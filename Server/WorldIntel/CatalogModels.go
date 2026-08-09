package WorldIntel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	CatalogSchemaVersion  = 1
	OfficialCatalogSource = "ggs-official-items"
	MaximumCatalogRows    = 30_000
	MaximumCatalogBytes   = 8 << 20
)

type CatalogDatasetSnapshot struct {
	SchemaVersion     int             `json:"schemaVersion"`
	SnapshotID        string          `json:"snapshotId"`
	Source            string          `json:"source"`
	SourceVersion     string          `json:"sourceVersion"`
	SourceURL         string          `json:"sourceUrl"`
	SourceDigest      string          `json:"sourceDigest"`
	DatasetKey        string          `json:"datasetKey"`
	DatasetLabel      string          `json:"datasetLabel"`
	Category          string          `json:"category"`
	DatasetDigest     string          `json:"datasetDigest"`
	Fields            []string        `json:"fields"`
	Rows              json.RawMessage `json:"rows"`
	RowCount          int             `json:"rowCount"`
	CapturedAt        time.Time       `json:"capturedAt"`
	CollectorPlayerID int64           `json:"collectorPlayerId"`
}

type CatalogDatasetSummary struct {
	DatasetKey       string    `json:"datasetKey"`
	DatasetLabel     string    `json:"datasetLabel"`
	Category         string    `json:"category"`
	Source           string    `json:"source"`
	SourceVersion    string    `json:"sourceVersion"`
	SourceURL        string    `json:"sourceUrl"`
	DatasetDigest    string    `json:"datasetDigest"`
	Fields           []string  `json:"fields"`
	RowCount         int       `json:"rowCount"`
	CapturedAt       time.Time `json:"capturedAt"`
	ContributorCount int64     `json:"contributorCount"`
}

type CatalogDatasetCatalogResponse struct {
	Source   string                  `json:"source"`
	Datasets []CatalogDatasetSummary `json:"datasets"`
}

type CatalogDatasetResponse struct {
	CatalogDatasetSummary
	Rows    json.RawMessage         `json:"rows"`
	History []CatalogDatasetSummary `json:"history"`
}

type CatalogIngestResponse struct {
	Accepted   bool   `json:"accepted"`
	SnapshotID string `json:"snapshotId"`
}

func FinalizeCatalogSnapshot(snapshot CatalogDatasetSnapshot) (CatalogDatasetSnapshot, error) {
	snapshot.SchemaVersion = CatalogSchemaVersion
	snapshot.SnapshotID = ""
	snapshot.Source = strings.ToLower(strings.TrimSpace(snapshot.Source))
	snapshot.SourceVersion = cleanText(snapshot.SourceVersion, 64)
	snapshot.SourceURL = strings.TrimSpace(snapshot.SourceURL)
	snapshot.SourceDigest = strings.ToLower(strings.TrimSpace(snapshot.SourceDigest))
	snapshot.DatasetKey = cleanCatalogKey(snapshot.DatasetKey)
	snapshot.DatasetLabel = cleanText(snapshot.DatasetLabel, 120)
	snapshot.Category = strings.ToLower(cleanCatalogKey(snapshot.Category))
	snapshot.CapturedAt = snapshot.CapturedAt.UTC().Truncate(time.Second)
	snapshot.Fields = normalizeCatalogFields(snapshot.Fields)

	var compact bytes.Buffer
	if err := json.Compact(&compact, snapshot.Rows); err != nil {
		return CatalogDatasetSnapshot{}, fmt.Errorf("rows must be valid JSON: %w", err)
	}
	snapshot.Rows = append(json.RawMessage(nil), compact.Bytes()...)
	var rows []json.RawMessage
	if err := json.Unmarshal(snapshot.Rows, &rows); err != nil {
		return CatalogDatasetSnapshot{}, fmt.Errorf("rows must be a JSON array: %w", err)
	}
	snapshot.RowCount = len(rows)
	datasetDigest := sha256.Sum256(snapshot.Rows)
	snapshot.DatasetDigest = hex.EncodeToString(datasetDigest[:])
	if err := ValidateCatalogSnapshot(snapshot); err != nil {
		return CatalogDatasetSnapshot{}, err
	}
	identity := struct {
		SchemaVersion int      `json:"schemaVersion"`
		Source        string   `json:"source"`
		SourceVersion string   `json:"sourceVersion"`
		SourceURL     string   `json:"sourceUrl"`
		SourceDigest  string   `json:"sourceDigest"`
		DatasetKey    string   `json:"datasetKey"`
		DatasetLabel  string   `json:"datasetLabel"`
		Category      string   `json:"category"`
		DatasetDigest string   `json:"datasetDigest"`
		Fields        []string `json:"fields"`
	}{
		snapshot.SchemaVersion, snapshot.Source, snapshot.SourceVersion, snapshot.SourceURL,
		snapshot.SourceDigest, snapshot.DatasetKey, snapshot.DatasetLabel, snapshot.Category,
		snapshot.DatasetDigest, snapshot.Fields,
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return CatalogDatasetSnapshot{}, fmt.Errorf("encode catalog snapshot identity: %w", err)
	}
	digest := sha256.Sum256(encoded)
	snapshot.SnapshotID = hex.EncodeToString(digest[:])
	return snapshot, nil
}

func ValidateFinalizedCatalogSnapshot(snapshot CatalogDatasetSnapshot) error {
	provided := strings.ToLower(strings.TrimSpace(snapshot.SnapshotID))
	if !validCatalogSHA256(provided) {
		return fmt.Errorf("snapshotId must be a SHA-256 digest")
	}
	rebuilt, err := FinalizeCatalogSnapshot(snapshot)
	if err != nil {
		return err
	}
	if rebuilt.SnapshotID != provided {
		return fmt.Errorf("snapshotId does not match the catalog payload")
	}
	return nil
}

func ValidateCatalogSnapshot(snapshot CatalogDatasetSnapshot) error {
	if snapshot.SchemaVersion != CatalogSchemaVersion || snapshot.Source != OfficialCatalogSource {
		return fmt.Errorf("catalog source contract is invalid")
	}
	if snapshot.SourceVersion == "" || !validOfficialItemsURL(snapshot.SourceURL) ||
		!validCatalogSHA256(snapshot.SourceDigest) || !validCatalogSHA256(snapshot.DatasetDigest) {
		return fmt.Errorf("official catalog provenance is invalid")
	}
	if snapshot.DatasetKey == "" || snapshot.DatasetLabel == "" || snapshot.Category == "" ||
		len(snapshot.DatasetKey) > 80 || len(snapshot.DatasetLabel) > 120 || len(snapshot.Category) > 80 {
		return fmt.Errorf("catalog dataset metadata is invalid")
	}
	if snapshot.CapturedAt.IsZero() || snapshot.CollectorPlayerID <= 0 || len(snapshot.Rows) == 0 || len(snapshot.Rows) > MaximumCatalogBytes {
		return fmt.Errorf("catalog capture provenance or payload is invalid")
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(snapshot.Rows, &rows); err != nil || len(rows) == 0 || len(rows) > MaximumCatalogRows || len(rows) != snapshot.RowCount {
		return fmt.Errorf("catalog row count is invalid")
	}
	for index, row := range rows {
		trimmed := bytes.TrimSpace(row)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return fmt.Errorf("rows[%d] must be an object", index)
		}
	}
	return nil
}

func validOfficialItemsURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || strings.ToLower(parsed.Hostname()) != "empire-html5.goodgamestudios.com" {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	return strings.HasPrefix(path, "/default/items/items_v") && strings.HasSuffix(path, ".json")
}

func validCatalogSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func cleanCatalogKey(value string) string {
	value = strings.TrimSpace(value)
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return ""
	}
	return value
}

func normalizeCatalogFields(fields []string) []string {
	seen := make(map[string]struct{}, len(fields))
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = cleanText(field, 120)
		if field == "" {
			continue
		}
		if _, found := seen[field]; found {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	sort.Strings(result)
	return result
}
