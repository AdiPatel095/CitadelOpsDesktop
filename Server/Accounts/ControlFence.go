package Accounts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gofrs/flock"
)

const controlEpochHeader = "X-Citadel-Control-Epoch"
const controlFenceFile = "controller-fence.json"

type controlFenceDocument struct {
	SchemaVersion int    `json:"schemaVersion"`
	CellID        string `json:"cellId"`
	Epoch         uint64 `json:"epoch"`
}

func readControlFence(directory, cellID string) (uint64, error) {
	path := filepath.Join(directory, controlFenceFile)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return 0, errors.New("invalid controller fence file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document controlFenceDocument
	if err := decoder.Decode(&document); err != nil {
		return 0, err
	}
	var trailing any
	canonical, err := json.Marshal(document)
	if err != nil || decoder.Decode(&trailing) != io.EOF || !bytes.Equal(raw, canonical) ||
		document.SchemaVersion != 1 || document.CellID != cellID || document.Epoch == 0 || document.Epoch > uint64(1<<63-1) {
		return 0, errors.New("invalid controller fence journal")
	}
	return document.Epoch, nil
}

func writeControlFence(directory, cellID string, epoch uint64) error {
	raw, err := json.Marshal(controlFenceDocument{SchemaVersion: 1, CellID: cellID, Epoch: epoch})
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".controller-fence-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(raw)
	syncErr, closeErr := file.Sync(), file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(directory, controlFenceFile)); err != nil {
		return err
	}
	parent, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(parent.Sync(), parent.Close())
}

// withControlFence serializes complete control mutations across HTTP handlers
// and even overlapping processes sharing the same cell data root. Once an
// authenticated fenced controller has written a positive epoch, legacy/unfenced
// commands are rejected permanently. Timeouts never reset this high-water mark.
func (orchestrator *Orchestrator) withControlFence(writer http.ResponseWriter, request *http.Request, next http.Handler) {
	values := request.Header.Values(controlEpochHeader)
	var epoch uint64
	if len(values) > 0 {
		var err error
		if len(values) != 1 {
			writeControlError(writer, http.StatusBadRequest, "invalid_control_epoch")
			return
		}
		epoch, err = strconv.ParseUint(values[0], 10, 63)
		if err != nil || epoch == 0 || strconv.FormatUint(epoch, 10) != values[0] {
			writeControlError(writer, http.StatusBadRequest, "invalid_control_epoch")
			return
		}
	}
	directory := filepath.Join(orchestrator.supervisor.config.DataRoot, "Accounts")
	if err := os.MkdirAll(directory, 0700); err != nil {
		writeControlError(writer, http.StatusServiceUnavailable, "control_fence_unavailable")
		return
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		writeControlError(writer, http.StatusServiceUnavailable, "control_fence_unavailable")
		return
	}
	lockPath := filepath.Join(directory, "controller-fence.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() || err != nil && !os.IsNotExist(err) {
		writeControlError(writer, http.StatusServiceUnavailable, "control_fence_unavailable")
		return
	}
	lock := flock.New(lockPath)
	defer lock.Close()
	// Bound lock queueing even when the caller supplied no deadline.
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !locked {
		writeControlError(writer, http.StatusServiceUnavailable, "control_fence_busy")
		return
	}
	defer lock.Unlock()
	persisted, err := readControlFence(directory, orchestrator.cellID)
	if err != nil {
		writeControlError(writer, http.StatusServiceUnavailable, "control_fence_unavailable")
		return
	}
	// An fsync error must not allow this process to acknowledge an older epoch.
	minimum := orchestrator.controlEpoch.Load()
	if persisted > minimum {
		minimum = persisted
		orchestrator.controlEpoch.Store(persisted)
	}
	if epoch < minimum {
		writeControlError(writer, http.StatusConflict, "stale_control_epoch")
		return
	}
	if epoch > 0 {
		orchestrator.controlEpoch.Store(epoch)
		// Retry persistence on equal epochs too: after a prior directory-fsync
		// failure, reading the renamed file alone is not durable acknowledgment.
		if err := writeControlFence(directory, orchestrator.cellID, epoch); err != nil {
			writeControlError(writer, http.StatusServiceUnavailable, "control_fence_unavailable")
			return
		}
	}
	next.ServeHTTP(writer, request)
}
