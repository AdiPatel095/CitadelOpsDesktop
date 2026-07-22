package History

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const legacyImportMarkerVersion = 1

type ImportRecord struct {
	CapturedAt time.Time
	Payload    json.RawMessage
}

type ImportSink func(ImportRecord) error

type LegacySourceDecoder func(io.Reader, ImportSink) error

type legacySourceSignature struct {
	Size             int64 `json:"size"`
	ModifiedUnixNano int64 `json:"modifiedUnixNano"`
}

type legacyImportMarker struct {
	Version int                              `json:"version"`
	Sources map[string]legacySourceSignature `json:"sources"`
}

func (store *Store) ImportLegacySource(
	sourcePath string,
	collection string,
	decode LegacySourceDecoder,
) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("history store is unavailable")
	}
	if decode == nil {
		return 0, fmt.Errorf("legacy history decoder is required")
	}
	destinationPath, err := store.collectionPath(collection)
	if err != nil {
		return 0, err
	}
	sourceInfo, err := os.Stat(sourcePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("inspect legacy history %s: %w", sourcePath, err)
	}
	if sourceInfo.IsDir() {
		return 0, fmt.Errorf("legacy history source %s is a directory", sourcePath)
	}
	signature := signatureFor(sourceInfo)
	sourceKey, err := filepath.Abs(sourcePath)
	if err != nil {
		sourceKey = filepath.Clean(sourcePath)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	marker, err := readLegacyImportMarker(store.directory)
	if err != nil {
		return 0, err
	}
	if marker.Sources[sourceKey] == signature {
		return 0, nil
	}

	identities, err := existingHistoryIdentities(destinationPath, collection)
	if err != nil {
		return 0, err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("open legacy history %s: %w", sourcePath, err)
	}
	defer source.Close()
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open %s history: %w", collection, err)
	}
	writer := bufio.NewWriterSize(destination, 256*1024)
	imported := 0
	sink := func(record ImportRecord) error {
		if len(record.Payload) == 0 || !json.Valid(record.Payload) {
			return nil
		}
		identity := historyIdentity(collection, record.Payload)
		if _, exists := identities[identity]; exists {
			return nil
		}
		capturedAt := record.CapturedAt.UTC()
		if capturedAt.IsZero() {
			capturedAt = time.Now().UTC()
		}
		line, err := json.Marshal(entry{
			SchemaVersion: SchemaVersion,
			CapturedAt:    capturedAt,
			Payload:       record.Payload,
		})
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(line, '\n')); err != nil {
			return err
		}
		identities[identity] = struct{}{}
		imported++
		return nil
	}
	decodeErr := decode(source, sink)
	flushErr := writer.Flush()
	syncErr := destination.Sync()
	closeErr := destination.Close()
	if decodeErr != nil {
		return imported, decodeErr
	}
	if flushErr != nil {
		return imported, flushErr
	}
	if syncErr != nil {
		return imported, syncErr
	}
	if closeErr != nil {
		return imported, closeErr
	}

	afterInfo, err := os.Stat(sourcePath)
	if err == nil && signatureFor(afterInfo) == signature {
		marker.Sources[sourceKey] = signature
		if err := writeLegacyImportMarker(store.directory, marker); err != nil {
			return imported, err
		}
	}
	return imported, nil
}

func existingHistoryIdentities(path string, collection string) (map[string]struct{}, error) {
	identities := map[string]struct{}{}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return identities, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		var item entry
		if json.Unmarshal(scanner.Bytes(), &item) != nil || item.SchemaVersion < 1 || item.SchemaVersion > SchemaVersion || len(item.Payload) == 0 {
			continue
		}
		identities[historyIdentity(collection, item.Payload)] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return identities, nil
}

func historyIdentity(collection string, payload json.RawMessage) string {
	var fields struct {
		ID            string `json:"id"`
		ReportID      string `json:"reportID"`
		MID           int64  `json:"mid"`
		LID           int64  `json:"lid"`
		UID           int64  `json:"uid"`
		PlayerID      int64  `json:"playerId"`
		TimestampUnix int64  `json:"timestampUnix"`
	}
	if json.Unmarshal(payload, &fields) == nil {
		switch collection {
		case CollectionPlayerSamples:
			if fields.TimestampUnix > 0 {
				if fields.UID > 0 {
					return "player-uid:" + strconv.FormatInt(fields.UID, 10) + ":" + strconv.FormatInt(fields.TimestampUnix, 10)
				}
				return "player:" + strconv.FormatInt(fields.PlayerID, 10) + ":" + strconv.FormatInt(fields.TimestampUnix, 10)
			}
		case CollectionSpyReports:
			if fields.ID != "" {
				return "spy:" + fields.ID
			}
			if fields.MID > 0 {
				return "spy:" + strconv.FormatInt(fields.MID, 10)
			}
		case CollectionBattleReports:
			if fields.ReportID != "" {
				return "battle:" + fields.ReportID
			}
			if fields.ID != "" {
				return "battle:" + fields.ID
			}
			if fields.MID > 0 {
				return "battle:" + strconv.FormatInt(fields.MID, 10) + ":" + strconv.FormatInt(fields.LID, 10)
			}
		}
	}
	sum := sha256.Sum256(bytes.TrimSpace(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func signatureFor(info os.FileInfo) legacySourceSignature {
	return legacySourceSignature{Size: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano()}
}

func readLegacyImportMarker(directory string) (legacyImportMarker, error) {
	marker := legacyImportMarker{Version: legacyImportMarkerVersion, Sources: map[string]legacySourceSignature{}}
	contents, err := os.ReadFile(filepath.Join(directory, "LegacyImports.json"))
	if errors.Is(err, os.ErrNotExist) {
		return marker, nil
	}
	if err != nil {
		return marker, fmt.Errorf("read legacy history marker: %w", err)
	}
	if err := json.Unmarshal(contents, &marker); err != nil {
		return marker, fmt.Errorf("decode legacy history marker: %w", err)
	}
	if marker.Version != legacyImportMarkerVersion {
		return marker, fmt.Errorf("unsupported legacy history marker version %d", marker.Version)
	}
	if marker.Sources == nil {
		marker.Sources = map[string]legacySourceSignature{}
	}
	return marker, nil
}

func writeLegacyImportMarker(directory string, marker legacyImportMarker) error {
	contents, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	temporary, err := os.CreateTemp(directory, ".LegacyImports-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, "LegacyImports.json")); err != nil {
		return fmt.Errorf("save legacy history marker: %w", err)
	}
	return nil
}
