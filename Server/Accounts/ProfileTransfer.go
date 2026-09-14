package Accounts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Runtime"
)

// ProfileExport is a private control receipt. The archive remains in a private
// worker directory and never appears in a portal response or public object URL.
type ProfileExport struct {
	Receipt      Runtime.ProfileArchiveReceipt `json:"receipt"`
	ArchiveBytes int64                         `json:"archiveBytes"`
}

type ProfileRestoreRequest struct {
	Receipt       Runtime.ProfileArchiveReceipt `json:"receipt"`
	TargetEpoch   uint64                        `json:"targetEpoch"`
	Configuration Configuration.Snapshot        `json:"configuration"`
}

func (o *Orchestrator) handoverSchema() int {
	if o.handoverTransport {
		return 1
	}
	return 0
}

func (o *Orchestrator) stoppedExportFence(identity Runtime.ProfileTransferIdentity) (SourceProfileFence, error) {
	o.supervisor.mu.RLock()
	closed := o.supervisor.closed
	o.supervisor.mu.RUnlock()
	if closed {
		return SourceProfileFence{}, errors.New("supervisor is closed")
	}
	fence, ok := o.supervisor.sourceFence(AccountID(identity.RuntimeID))
	if !ok || !fence.SourceStopped || fence.Identity != identity || identity.SourceCellID != o.cellID || !o.supervisor.runtimeHandoverFenced(AccountID(identity.RuntimeID)) {
		return SourceProfileFence{}, errors.New("exact stopped source fence is required")
	}
	return fence, nil
}

// ExportSourceProfile persists one immutable export after the source is
// stopped. Retries verify the saved export instead of re-capturing a profile
// whose durable files could have changed after the original stop receipt.
func (o *Orchestrator) ExportSourceProfile(ctx context.Context, identity Runtime.ProfileTransferIdentity) (ProfileExport, error) {
	if _, err := o.PrepareSourceHandover(ctx, identity); err != nil {
		return ProfileExport{}, err
	}
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	s := o.supervisor
	s.rebindMu.Lock()
	defer s.rebindMu.Unlock()
	fence, err := o.stoppedExportFence(identity)
	if err != nil {
		return ProfileExport{}, err
	}
	directory := filepath.Join(s.config.DataRoot, "Transfers", "Exports")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return ProfileExport{}, err
	}
	final := filepath.Join(directory, identity.OperationID)
	if _, err := os.Lstat(final); os.IsNotExist(err) {
		stage, err := os.MkdirTemp(directory, ".export-*")
		if err != nil {
			return ProfileExport{}, err
		}
		defer os.RemoveAll(stage) // Only this call's newly-created temporary output.
		file, err := os.OpenFile(filepath.Join(stage, "profile.tar"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return ProfileExport{}, err
		}
		receipt, captureErr := Runtime.WriteProfileArchive(ctx, filepath.Join(s.config.DataRoot, filepath.FromSlash(fence.ProfileDirectory)), file, identity)
		info, statErr := file.Stat()
		if err := errors.Join(captureErr, statErr, file.Sync(), file.Close()); err != nil {
			return ProfileExport{}, err
		}
		if receipt.ProfileID != fence.ProfileID {
			return ProfileExport{}, errors.New("stopped source profile changed")
		}
		export := ProfileExport{receipt, info.Size()}
		raw, err := json.Marshal(export)
		if err != nil {
			return ProfileExport{}, err
		}
		metadata, err := os.OpenFile(filepath.Join(stage, "receipt.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return ProfileExport{}, err
		}
		_, writeErr := metadata.Write(raw)
		if err := errors.Join(writeErr, metadata.Sync(), metadata.Close()); err != nil {
			return ProfileExport{}, err
		}
		if err := syncProfileParent(stage); err != nil {
			return ProfileExport{}, err
		}
		if err := ctx.Err(); err != nil {
			return ProfileExport{}, err
		}
		if err := os.Rename(stage, final); err != nil {
			return ProfileExport{}, err
		}
	} else if err != nil {
		return ProfileExport{}, err
	}
	export, file, err := o.openProfileExport(ctx, identity)
	if err != nil {
		return ProfileExport{}, err
	}
	if err := file.Close(); err != nil {
		return ProfileExport{}, err
	}
	if err := syncProfileParent(directory); err != nil {
		return ProfileExport{}, err
	}
	// Persist the newly created Exports/Transfers directory entries too.
	if err := syncProfileParent(filepath.Dir(directory)); err != nil {
		return ProfileExport{}, err
	}
	if err := syncProfileParent(s.config.DataRoot); err != nil {
		return ProfileExport{}, err
	}
	return export, nil
}

// Caller serializes handovers. The returned descriptor is positioned at zero
// and refers to the exact bounded file whose digest was checked here.
func (o *Orchestrator) openProfileExport(ctx context.Context, identity Runtime.ProfileTransferIdentity) (ProfileExport, *os.File, error) {
	fence, err := o.stoppedExportFence(identity)
	if err != nil {
		return ProfileExport{}, nil, err
	}
	directory := filepath.Join(o.supervisor.config.DataRoot, "Transfers", "Exports", identity.OperationID)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		return ProfileExport{}, nil, errors.New("export directory is missing or unsafe")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return ProfileExport{}, nil, err
	}
	defer root.Close()
	for _, name := range []string{"receipt.json", "profile.tar"} {
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			return ProfileExport{}, nil, errors.New("export contains an unsafe file")
		}
	}
	metadata, err := root.Open("receipt.json")
	if err != nil {
		return ProfileExport{}, nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(metadata, (16<<10)+1))
	closeErr := metadata.Close()
	if readErr != nil || closeErr != nil || len(raw) > 16<<10 {
		return ProfileExport{}, nil, errors.New("invalid export metadata")
	}
	var export ProfileExport
	if err := json.Unmarshal(raw, &export); err != nil {
		return ProfileExport{}, nil, errors.New("invalid export receipt")
	}
	canonical, err := json.Marshal(export)
	if err != nil || !bytes.Equal(raw, canonical) || export.Receipt.Identity != identity || export.Receipt.ProfileID != fence.ProfileID ||
		export.Receipt.SchemaVersion != 1 || !validConfigurationDigest(export.Receipt.SHA256) || export.ArchiveBytes < 1 || export.ArchiveBytes > Runtime.MaxProfileArchiveBytes {
		return ProfileExport{}, nil, errors.New("export receipt does not match stopped source")
	}
	file, err := root.Open("profile.tar")
	if err != nil {
		return ProfileExport{}, nil, err
	}
	hash := sha256.New()
	count, err := io.Copy(hash, &profileTransferReader{ctx, io.LimitReader(file, Runtime.MaxProfileArchiveBytes+1)})
	if err != nil || count != export.ArchiveBytes || hex.EncodeToString(hash.Sum(nil)) != export.Receipt.SHA256 {
		file.Close()
		return ProfileExport{}, nil, errors.New("immutable export integrity failed")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return ProfileExport{}, nil, err
	}
	return export, file, nil
}

type profileTransferReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *profileTransferReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func (o *Orchestrator) handleProfileExport(writer http.ResponseWriter, request *http.Request) {
	var identity Runtime.ProfileTransferIdentity
	if decodeControlJSON(writer, request, &identity) != nil {
		return
	}
	export, err := o.ExportSourceProfile(request.Context(), identity)
	if err != nil {
		writeControlError(writer, http.StatusConflict, "profile_export_not_ready")
		return
	}
	writeControlJSON(writer, http.StatusOK, export)
}

func (o *Orchestrator) handleProfileDownload(writer http.ResponseWriter, request *http.Request) {
	var expected ProfileExport
	if decodeControlJSON(writer, request, &expected) != nil {
		return
	}
	o.reconcileMu.Lock()
	defer o.reconcileMu.Unlock()
	o.supervisor.rebindMu.Lock()
	defer o.supervisor.rebindMu.Unlock()
	export, file, err := o.openProfileExport(request.Context(), expected.Receipt.Identity)
	if err != nil {
		writeControlError(writer, http.StatusConflict, "profile_export_not_ready")
		return
	}
	defer file.Close()
	if export != expected {
		writeControlError(writer, http.StatusConflict, "profile_export_mismatch")
		return
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Length", strconv.FormatInt(export.ArchiveBytes, 10))
	writer.Header().Set("X-Citadel-Profile-SHA256", export.Receipt.SHA256)
	writer.WriteHeader(http.StatusOK)
	_, _ = io.CopyN(writer, &profileTransferReader{request.Context(), file}, export.ArchiveBytes)
}

func (o *Orchestrator) handleProfileRestore(writer http.ResponseWriter, request *http.Request) {
	media, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || media != "multipart/form-data" || parameters["boundary"] == "" {
		writeControlError(writer, http.StatusUnsupportedMediaType, "profile_multipart_required")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, Runtime.MaxProfileArchiveBytes+maximumConfigurationSyncBytes+(64<<10))
	parts := multipart.NewReader(request.Body, parameters["boundary"])
	metadata, err := parts.NextRawPart()
	if err != nil || metadata.FormName() != "metadata" || metadata.FileName() != "" || metadata.Header.Get("Content-Transfer-Encoding") != "" {
		writeControlError(writer, http.StatusBadRequest, "profile_metadata_required")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(metadata, maximumConfigurationSyncBytes+1))
	if err != nil || len(raw) > maximumConfigurationSyncBytes {
		writeControlError(writer, http.StatusBadRequest, "profile_metadata_invalid")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input ProfileRestoreRequest
	if err := decoder.Decode(&input); err != nil {
		writeControlError(writer, http.StatusBadRequest, "profile_metadata_invalid")
		return
	}
	if requireJSONEOF(decoder) != nil {
		writeControlError(writer, http.StatusBadRequest, "profile_metadata_invalid")
		return
	}
	archive, err := parts.NextRawPart()
	if err != nil || archive.FormName() != "archive" || archive.Header.Get("Content-Transfer-Encoding") != "" {
		writeControlError(writer, http.StatusBadRequest, "profile_archive_required")
		return
	}
	profile, err := o.RestoreTargetProfile(request.Context(), input.Receipt, input.TargetEpoch, input.Configuration, archive)
	if err != nil {
		writeControlError(writer, http.StatusConflict, "profile_restore_not_ready")
		return
	}
	// A retry may find a durable generation and intentionally skip reading the
	// archive. Close/NextPart drains only within the total bounded request body.
	if err := archive.Close(); err != nil {
		writeControlError(writer, http.StatusBadRequest, "profile_archive_invalid")
		return
	}
	if _, err := parts.NextRawPart(); err != io.EOF {
		writeControlError(writer, http.StatusBadRequest, "profile_extra_part")
		return
	}
	writeControlJSON(writer, http.StatusOK, profile)
}

func (o *Orchestrator) handleProfileActivate(writer http.ResponseWriter, request *http.Request) {
	var profile TargetProfile
	if decodeControlJSON(writer, request, &profile) != nil {
		return
	}
	active, err := o.ActivateTargetProfile(request.Context(), profile)
	if err != nil {
		writeControlError(writer, http.StatusConflict, "profile_activation_not_ready")
		return
	}
	writeControlJSON(writer, http.StatusOK, active)
}
