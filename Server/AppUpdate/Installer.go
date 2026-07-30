package AppUpdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func (manager *Manager) downloadAndInstall(ctx context.Context, downloadURL string, expectedSHA256 string) (returnErr error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	response, err := manager.config.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > manager.config.MaxBytes {
		return fmt.Errorf("application update exceeds %d bytes", manager.config.MaxBytes)
	}
	if err := manager.validateDownloadURL(response.Request.URL.String()); err != nil {
		return err
	}
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(executablePath), ".CitadelOps-update-*")
	if err != nil {
		return fmt.Errorf("create update file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	hash := sha256.New()
	reader := &progressReader{
		reader: io.LimitReader(response.Body, manager.config.MaxBytes+1),
		total:  response.ContentLength,
		onProgress: func(progress int) {
			manager.update(func(snapshot *Snapshot) {
				snapshot.Progress = progress
				snapshot.Stage = "Downloading update..."
			})
		},
	}
	written, err := io.Copy(io.MultiWriter(temporary, hash), reader)
	if err != nil {
		return fmt.Errorf("download application update: %w", err)
	}
	if written > manager.config.MaxBytes {
		return fmt.Errorf("application update exceeds %d bytes", manager.config.MaxBytes)
	}
	if written < 1<<20 {
		return fmt.Errorf("application update is unexpectedly small (%d bytes)", written)
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if expectedSHA256 != "" && !strings.EqualFold(strings.TrimSpace(expectedSHA256), actualSHA256) {
		return fmt.Errorf("application update checksum mismatch")
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync application update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close application update: %w", err)
	}
	if err := validateExecutableFile(temporaryPath); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(temporaryPath, 0o755); err != nil {
			return fmt.Errorf("make application update executable: %w", err)
		}
	}
	manager.update(func(snapshot *Snapshot) {
		snapshot.Status = "installing"
		snapshot.Stage = "Installing update..."
		snapshot.Progress = 96
	})
	if err := replaceExecutable(executablePath, temporaryPath); err != nil {
		return err
	}
	return nil
}

type progressReader struct {
	reader     io.Reader
	total      int64
	read       int64
	last       int
	onProgress func(int)
}

func (reader *progressReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.read += int64(count)
	if reader.total > 0 && reader.onProgress != nil {
		progress := 1 + int(float64(reader.read)/float64(reader.total)*94)
		if progress > 95 {
			progress = 95
		}
		if progress > reader.last {
			reader.last = progress
			reader.onProgress(progress)
		}
	}
	return count, err
}

func validateExecutableFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("read application update header: %w", err)
	}
	valid := false
	switch runtime.GOOS {
	case "windows":
		valid = header[0] == 'M' && header[1] == 'Z'
	case "darwin":
		magic := hex.EncodeToString(header)
		valid = magic == "cffaedfe" || magic == "feedfacf" || magic == "cafebabe" || magic == "bebafeca"
	case "linux":
		valid = string(header) == "\x7fELF"
	}
	if !valid {
		return fmt.Errorf("download is not a %s executable", runtime.GOOS)
	}
	return nil
}

func replaceExecutable(executablePath string, updatePath string) error {
	backupPath := oldExecutablePath(executablePath)
	_ = os.Remove(backupPath)
	if err := os.Rename(executablePath, backupPath); err != nil {
		return fmt.Errorf("back up current executable: %w", err)
	}
	if err := os.Rename(updatePath, executablePath); err != nil {
		_ = os.Rename(backupPath, executablePath)
		return fmt.Errorf("install application update: %w", err)
	}
	if directory, err := os.Open(filepath.Dir(executablePath)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func CleanupOldExecutable() error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	err = os.Remove(oldExecutablePath(executablePath))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func oldExecutablePath(executablePath string) string {
	extension := filepath.Ext(executablePath)
	base := strings.TrimSuffix(executablePath, extension)
	return base + "_old" + extension
}
