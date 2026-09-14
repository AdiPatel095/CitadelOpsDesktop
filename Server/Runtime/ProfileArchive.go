package Runtime

// Profile archives are a stopped-profile transport primitive, not a handover
// executor. The caller must durably fence the source, acknowledge shutdown,
// freeze canonical writes, and authenticate both endpoints before using them.
// Restore always creates a new directory; it never overwrites a live profile.

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	profileArchiveSchema         = 1
	profileArchiveManifest       = "citadelops-profile-manifest.json"
	maxProfileFiles              = 100000
	maxProfileBytes        int64 = 2 << 30
	maxManifestBytes       int64 = 32 << 20
	maxArchiveBytes        int64 = maxProfileBytes + maxManifestBytes + (128 << 20)
	// MaxProfileArchiveBytes is the shared bound for private transfer transports.
	MaxProfileArchiveBytes int64 = maxArchiveBytes
)

// ProfileTransferIdentity binds every archive to one operation and exact
// source configuration. It contains identifiers/digests only, never passwords.
type ProfileTransferIdentity struct {
	OperationID           string `json:"operationId"`
	AccountID             string `json:"accountId"`
	RuntimeID             string `json:"runtimeId"`
	TenantID              string `json:"tenantId"`
	SourceCellID          string `json:"sourceCellId"`
	SourceEpoch           uint64 `json:"sourceEpoch"`
	ConfigurationRevision uint64 `json:"configurationRevision"`
	ConfigurationDigest   string `json:"configurationDigest"`
}

type profileArchiveEntry struct {
	Name      string `json:"name"`
	Directory bool   `json:"directory,omitempty"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256,omitempty"`
}

type profileArchiveDocument struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Identity      ProfileTransferIdentity `json:"identity"`
	ProfileID     string                  `json:"profileId"`
	Entries       []profileArchiveEntry   `json:"entries"`
}

type ProfileArchiveReceipt struct {
	Identity      ProfileTransferIdentity `json:"identity"`
	SchemaVersion int                     `json:"schemaVersion"`
	ProfileID     string                  `json:"profileId"`
	SHA256        string                  `json:"sha256"`
	Files         int                     `json:"files"`
	Bytes         int64                   `json:"bytes"`
}

func archiveNameValid(name string) bool {
	return len(name) <= 4096 && name != "." && fs.ValidPath(name) && path.Clean(name) == name &&
		!strings.ContainsAny(name, "\\:\x00") && name != "Runtime/Profile.lock"
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func (identity ProfileTransferIdentity) valid() bool {
	for _, value := range []string{identity.OperationID, identity.AccountID, identity.RuntimeID, identity.TenantID, identity.SourceCellID} {
		if value == "" || len(value) > 128 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/\\\x00\r\n") {
			return false
		}
	}
	return identity.SourceEpoch > 0 && identity.ConfigurationRevision > 0 && validDigest(identity.ConfigurationDigest)
}

type archiveContextReader struct {
	ctx    context.Context
	reader io.Reader
}

type archiveBoundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (writer *archiveBoundedWriter) Write(value []byte) (int, error) {
	if int64(len(value)) > writer.remaining {
		return 0, errors.New("profile archive exceeds transfer limit")
	}
	count, err := writer.writer.Write(value)
	writer.remaining -= int64(count)
	return count, err
}

func (reader archiveContextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

// Unix hard links could refer to sensitive files outside the profile. Hosted
// archives are supported only where the filesystem exposes a link count.
func archiveRegularFile(info fs.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return false
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return false
	}
	links := value.FieldByName("Nlink")
	return links.IsValid() && links.CanUint() && links.Uint() == 1
}

func profileManifest(ctx context.Context, directory string, root *os.Root, identity ProfileTransferIdentity, profileID string) (profileArchiveDocument, error) {
	document := profileArchiveDocument{SchemaVersion: profileArchiveSchema, Identity: identity, ProfileID: profileID}
	var total int64
	err := filepath.WalkDir(directory, func(full string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(directory, full)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if name == "." || name == "Runtime/Profile.lock" {
			return nil
		}
		if !archiveNameValid(name) || len(document.Entries) >= maxProfileFiles {
			return errors.New("profile contains an unsupported path or too many entries")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := profileArchiveEntry{Name: name, Directory: info.IsDir()}
		if !info.IsDir() {
			if !archiveRegularFile(info) {
				return errors.New("profile contains a link or unsupported file")
			}
			file, err := root.Open(name)
			if err != nil {
				return err
			}
			opened, statErr := file.Stat()
			if statErr != nil || !archiveRegularFile(opened) || !os.SameFile(info, opened) {
				file.Close()
				return errors.New("profile file changed during capture")
			}
			if opened.Size() < 0 || opened.Size() > maxProfileBytes-total {
				file.Close()
				return errors.New("profile exceeds transfer size limit")
			}
			hash := sha256.New()
			count, copyErr := io.Copy(hash, io.LimitReader(archiveContextReader{ctx, file}, opened.Size()+1))
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
			if count != opened.Size() {
				return errors.New("profile file changed during capture")
			}
			item.Size, item.SHA256 = count, hex.EncodeToString(hash.Sum(nil))
			total += count
		}
		document.Entries = append(document.Entries, item)
		return nil
	})
	return document, err
}

// WriteProfileArchive requires a pre-existing, stopped profile. It obtains the
// same exclusive lease as the application and holds it through both passes.
// The writer should be a private staging file; discard it if this returns error.
func WriteProfileArchive(ctx context.Context, directory string, writer io.Writer, identity ProfileTransferIdentity) (ProfileArchiveReceipt, error) {
	var receipt ProfileArchiveReceipt
	if !identity.valid() {
		return receipt, errors.New("invalid profile transfer identity")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return receipt, errors.New("profile directory must exist without a symlink")
	}
	// Do not manufacture identity for an empty/wrong profile path.
	root, err := os.OpenRoot(directory)
	if err != nil {
		return receipt, err
	}
	defer root.Close()
	for _, name := range []string{"Runtime", "Runtime/ProfileID", "Runtime/Profile.lock"} {
		info, statErr := root.Lstat(name)
		if os.IsNotExist(statErr) && name == "Runtime/Profile.lock" {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || (name == "Runtime" && !info.IsDir()) ||
			(name != "Runtime" && !archiveRegularFile(info)) {
			return receipt, errors.New("profile lease paths must not contain links or special files")
		}
	}
	identityFile, err := root.Open("Runtime/ProfileID")
	if err != nil {
		return receipt, errors.New("profile has no durable identity")
	}
	rawID, readErr := io.ReadAll(io.LimitReader(identityFile, 129))
	identityFile.Close()
	if readErr != nil || len(rawID) > 128 || strings.TrimSpace(string(rawID)) == "" {
		return receipt, errors.New("invalid durable profile identity")
	}
	lease, err := AcquireProfileLease(directory)
	if err != nil {
		return receipt, err
	}
	defer lease.Close()
	if lease.ProfileID != strings.TrimSpace(string(rawID)) {
		return receipt, errors.New("profile identity changed")
	}
	document, err := profileManifest(ctx, directory, root, identity, lease.ProfileID)
	if err != nil {
		return receipt, err
	}
	manifest, err := json.Marshal(document)
	if err != nil || int64(len(manifest)) > maxManifestBytes {
		return receipt, errors.New("profile manifest exceeds limit")
	}
	hash := sha256.New()
	archive := tar.NewWriter(&archiveBoundedWriter{writer: io.MultiWriter(writer, hash), remaining: maxArchiveBytes})
	if err := archive.WriteHeader(&tar.Header{Name: profileArchiveManifest, Mode: 0600, Size: int64(len(manifest)), Typeflag: tar.TypeReg}); err != nil {
		return receipt, err
	}
	if _, err := archive.Write(manifest); err != nil {
		return receipt, err
	}
	for _, item := range document.Entries {
		if err := ctx.Err(); err != nil {
			return receipt, err
		}
		header := &tar.Header{Name: item.Name, Mode: 0600, Size: item.Size, Typeflag: tar.TypeReg}
		if item.Directory {
			header.Typeflag, header.Mode = tar.TypeDir, 0700
		}
		if err := archive.WriteHeader(header); err != nil {
			return receipt, err
		}
		if item.Directory {
			continue
		}
		file, err := root.Open(item.Name)
		if err != nil {
			return receipt, err
		}
		info, err := file.Stat()
		if err != nil || !archiveRegularFile(info) || info.Size() != item.Size {
			file.Close()
			return receipt, errors.New("profile file changed during capture")
		}
		fileHash := sha256.New()
		count, copyErr := io.Copy(io.MultiWriter(archive, fileHash), io.LimitReader(archiveContextReader{ctx, file}, item.Size+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return receipt, errors.Join(copyErr, closeErr)
		}
		if count != item.Size || hex.EncodeToString(fileHash.Sum(nil)) != item.SHA256 {
			return receipt, errors.New("profile changed during capture")
		}
		receipt.Files++
		receipt.Bytes += count
	}
	if err := archive.Close(); err != nil {
		return ProfileArchiveReceipt{}, err
	}
	receipt.Identity, receipt.SchemaVersion, receipt.ProfileID, receipt.SHA256 = identity, profileArchiveSchema, lease.ProfileID, hex.EncodeToString(hash.Sum(nil))
	return receipt, nil
}

// RestoreProfileArchive validates into a newly-created stopped staging profile.
// The caller still must fence placement, bind the player directory, and verify
// schema/configuration compatibility before atomically adopting it or logging in.
func RestoreProfileArchive(ctx context.Context, reader io.Reader, destination string, expected ProfileTransferIdentity, digest string) (receipt ProfileArchiveReceipt, err error) {
	if !expected.valid() || !validDigest(digest) {
		return receipt, errors.New("invalid expected transfer identity or digest")
	}
	if err = os.Mkdir(destination, 0700); err != nil {
		return receipt, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(destination)
		}
	}() // Only the fresh staging directory created above.
	root, err := os.OpenRoot(destination)
	if err != nil {
		return receipt, err
	}
	defer root.Close()
	hash := sha256.New()
	limited := &io.LimitedReader{R: archiveContextReader{ctx, reader}, N: maxArchiveBytes + 1}
	stream := io.TeeReader(limited, hash)
	archive := tar.NewReader(stream)
	header, err := archive.Next()
	if err != nil || header.Name != profileArchiveManifest || header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > maxManifestBytes {
		return receipt, errors.New("invalid profile manifest header")
	}
	raw, err := io.ReadAll(archive)
	if err != nil {
		return receipt, err
	}
	var document profileArchiveDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&document); err != nil {
		return receipt, errors.New("invalid profile manifest")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return receipt, errors.New("trailing profile manifest data")
	}
	if document.SchemaVersion != profileArchiveSchema || document.Identity != expected || document.ProfileID == "" || len(document.ProfileID) > 128 || len(document.Entries) > maxProfileFiles {
		return receipt, errors.New("unsupported or mismatched profile manifest")
	}
	seen := map[string]bool{}
	var total int64
	for _, item := range document.Entries {
		if !archiveNameValid(item.Name) || seen[item.Name] || item.Size < 0 || item.Size > maxProfileBytes-total ||
			(item.Directory && (item.Size != 0 || item.SHA256 != "")) || (!item.Directory && !validDigest(item.SHA256)) {
			return receipt, errors.New("invalid profile entry")
		}
		seen[item.Name] = true
		total += item.Size
		header, err = archive.Next()
		if err != nil || header.Name != item.Name || header.Size != item.Size || header.Linkname != "" {
			return receipt, errors.New("profile entry mismatch")
		}
		if item.Directory {
			if header.Typeflag != tar.TypeDir {
				return receipt, errors.New("profile entry type mismatch")
			}
			if err = root.Mkdir(item.Name, 0700); err != nil {
				return receipt, err
			}
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return receipt, errors.New("profile links and special files are forbidden")
		}
		file, openErr := root.OpenFile(item.Name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if openErr != nil {
			return receipt, openErr
		}
		fileHash := sha256.New()
		count, copyErr := io.Copy(io.MultiWriter(file, fileHash), archive)
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil {
			return receipt, errors.Join(copyErr, syncErr, closeErr)
		}
		if count != item.Size || hex.EncodeToString(fileHash.Sum(nil)) != item.SHA256 {
			return receipt, errors.New("profile entry checksum mismatch")
		}
		receipt.Files++
		receipt.Bytes += count
	}
	if _, nextErr := archive.Next(); nextErr != io.EOF {
		return receipt, errors.New("unexpected or truncated archive entry")
	}
	// tar permits terminal zero padding; concatenated archives or other trailing
	// data are not accepted, even if an attacker supplies their own digest.
	buffer := make([]byte, 32768)
	for {
		count, readErr := stream.Read(buffer)
		for _, value := range buffer[:count] {
			if value != 0 {
				return receipt, errors.New("trailing archive data")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return receipt, readErr
		}
	}
	if limited.N == 0 || hex.EncodeToString(hash.Sum(nil)) != digest {
		return receipt, errors.New("profile archive checksum or size mismatch")
	}
	identityFile, err := root.Open("Runtime/ProfileID")
	if err != nil {
		return receipt, errors.New("restored profile identity is missing")
	}
	rawID, readErr := io.ReadAll(io.LimitReader(identityFile, 129))
	identityFile.Close()
	if readErr != nil || strings.TrimSpace(string(rawID)) != document.ProfileID {
		return receipt, errors.New("restored profile identity mismatch")
	}
	// Flush directory entries as well as file contents before the caller adopts
	// the stage. All directories were created by this operation, without links.
	for index := len(document.Entries) - 1; index >= 0; index-- {
		item := document.Entries[index]
		if !item.Directory {
			continue
		}
		if err = syncProfileDirectory(filepath.Join(destination, filepath.FromSlash(item.Name))); err != nil {
			return receipt, err
		}
	}
	if err = syncProfileDirectory(destination); err != nil {
		return receipt, err
	}
	receipt.Identity, receipt.SchemaVersion, receipt.ProfileID, receipt.SHA256 = expected, profileArchiveSchema, document.ProfileID, digest
	return receipt, nil
}

func syncProfileDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}

func (receipt ProfileArchiveReceipt) String() string {
	return fmt.Sprintf("profile archive schema=%d files=%d bytes=%d sha256=%s", receipt.SchemaVersion, receipt.Files, receipt.Bytes, receipt.SHA256)
}
