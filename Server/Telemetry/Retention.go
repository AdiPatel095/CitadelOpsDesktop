package Telemetry

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	logRetentionDuration      = 48 * time.Hour
	logRetentionSweepInterval = time.Hour
	logCompactionBufferSize   = 4 * 1024 * 1024
)

type logRetentionResult struct {
	filesDeleted   int
	filesCompacted int
	bytesRemoved   int64
}

type persistentLogPath struct {
	path       string
	modifiedAt time.Time
}

func (store *Store) startRetention() {
	store.retentionLifecycleMu.Lock()
	defer store.retentionLifecycleMu.Unlock()
	if store.retentionStarted || store.retentionStopped {
		return
	}
	store.retentionStarted = true
	go store.runRetention()
}

func (store *Store) wakeRetention() {
	store.retentionLifecycleMu.Lock()
	started := store.retentionStarted && !store.retentionStopped
	store.retentionLifecycleMu.Unlock()
	if !started {
		return
	}
	select {
	case store.retentionWake <- struct{}{}:
	default:
	}
}

func (store *Store) stopRetention() {
	store.retentionLifecycleMu.Lock()
	started := store.retentionStarted
	if !store.retentionStopped {
		store.retentionStopped = true
		if started {
			close(store.retentionStop)
		}
	}
	done := store.retentionDone
	store.retentionLifecycleMu.Unlock()
	if started {
		<-done
	}
}

func (store *Store) runRetention() {
	defer close(store.retentionDone)
	ticker := time.NewTicker(logRetentionSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-store.retentionStop:
			return
		case <-store.retentionWake:
		case <-ticker.C:
		}
		select {
		case <-store.retentionStop:
			return
		default:
		}
		result, err := store.pruneLogs(time.Now().Add(-logRetentionDuration))
		if err != nil {
			log.Printf("[telemetry-retention] cleanup incomplete: %v", err)
		}
		if result.bytesRemoved > 0 {
			log.Printf(
				"[telemetry-retention] retained 48 hours; removed %d bytes from %d deleted and %d compacted log files",
				result.bytesRemoved,
				result.filesDeleted,
				result.filesCompacted,
			)
		}
	}
}

func (store *Store) pruneLogs(cutoff time.Time) (logRetentionResult, error) {
	result := logRetentionResult{}
	if store == nil || cutoff.IsZero() {
		return result, nil
	}
	store.flushPersistence()
	store.retentionFileMu.Lock()
	defer store.retentionFileMu.Unlock()
	store.fileMu.Lock()
	directory := store.channelsDir
	activePaths := make(map[string]struct{}, len(store.filePaths))
	for _, path := range store.filePaths {
		activePaths[filepath.Clean(path)] = struct{}{}
	}
	store.fileMu.Unlock()
	if directory == "" {
		return result, nil
	}

	var cleanupErrors []error
	for _, channel := range knownChannels {
		legacyPath := filepath.Join(directory, channel.ID+".log")
		if _, active := activePaths[filepath.Clean(legacyPath)]; !active {
			removed, deleted, compacted, err := pruneLegacyLog(legacyPath, cutoff)
			result.bytesRemoved += removed
			if deleted {
				result.filesDeleted++
			}
			if compacted {
				result.filesCompacted++
			}
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("prune %s: %w", legacyPath, err))
			}
		}

		channelDirectory := filepath.Join(directory, channel.ID)
		entries, err := os.ReadDir(channelDirectory)
		if err != nil {
			if !os.IsNotExist(err) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("read %s: %w", channelDirectory, err))
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			path := filepath.Join(channelDirectory, entry.Name())
			if _, active := activePaths[filepath.Clean(path)]; active {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("stat %s: %w", path, err))
				continue
			}
			if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
				continue
			}
			expired, err := rotatedLogExpired(path, cutoff)
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("inspect %s: %w", path, err))
				continue
			}
			if !expired {
				continue
			}
			if err := os.Remove(path); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove %s: %w", path, err))
				continue
			}
			result.filesDeleted++
			result.bytesRemoved += info.Size()
		}
	}
	return result, errors.Join(cleanupErrors...)
}

func rotatedLogExpired(path string, cutoff time.Time) (bool, error) {
	lines, err := tailNonEmptyLines(path, 100)
	if err != nil {
		return false, err
	}
	for index := len(lines) - 1; index >= 0; index-- {
		if observedAt, ok := persistentLogTimestamp(lines[index]); ok {
			return observedAt.Before(cutoff), nil
		}
	}
	return false, nil
}

func pruneLegacyLog(path string, cutoff time.Time) (int64, bool, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, false, nil
		}
		return 0, false, false, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return 0, false, false, nil
	}
	offset, recognized, err := logRetentionOffset(path, cutoff)
	if err != nil || !recognized || offset <= 0 {
		return 0, false, false, err
	}
	if offset >= info.Size() {
		if err := os.Remove(path); err != nil {
			return 0, false, false, err
		}
		return info.Size(), true, false, nil
	}
	if err := compactLogPrefix(path, offset); err != nil {
		return 0, false, false, err
	}
	return offset, false, true, nil
}

func logRetentionOffset(path string, cutoff time.Time) (int64, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, logCompactionBufferSize)
	var offset int64
	recognized := false
	for {
		lineStart := offset
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			if observedAt, ok := persistentLogTimestamp(line); ok {
				recognized = true
				if !observedAt.Before(cutoff) {
					return lineStart, true, nil
				}
			}
			offset += int64(len(line))
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return offset, recognized, nil
			}
			return 0, false, readErr
		}
	}
}

func persistentLogTimestamp(line string) (time.Time, bool) {
	const timestampLength = len("2006-01-02 15:04:05.000000")
	if len(line) < timestampLength {
		return time.Time{}, false
	}
	observedAt, err := time.ParseInLocation(
		"2006-01-02 15:04:05.000000",
		line[:timestampLength],
		time.Local,
	)
	return observedAt, err == nil
}

func compactLogPrefix(path string, offset int64) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if offset <= 0 || offset >= info.Size() {
		return fmt.Errorf("invalid compaction offset %d for %d-byte log", offset, info.Size())
	}
	buffer := make([]byte, logCompactionBufferSize)
	var written int64
	for source := offset; source < info.Size(); {
		readSize := min(int64(len(buffer)), info.Size()-source)
		count, readErr := file.ReadAt(buffer[:readSize], source)
		if count > 0 {
			position := 0
			for position < count {
				writeCount, writeErr := file.WriteAt(buffer[position:count], written+int64(position))
				if writeErr != nil {
					return writeErr
				}
				if writeCount == 0 {
					return io.ErrShortWrite
				}
				position += writeCount
			}
			source += int64(count)
			written += int64(count)
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if count == 0 {
			break
		}
	}
	if err := file.Truncate(written); err != nil {
		return err
	}
	return file.Sync()
}

func channelLogPathsNewest(channelsDirectory string, channel string) []string {
	if strings.TrimSpace(channelsDirectory) == "" || strings.TrimSpace(channel) == "" {
		return nil
	}
	directory := filepath.Join(channelsDirectory, channel)
	entries, _ := os.ReadDir(directory)
	paths := make([]persistentLogPath, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		paths = append(paths, persistentLogPath{
			path:       filepath.Join(directory, entry.Name()),
			modifiedAt: info.ModTime(),
		})
	}
	sort.Slice(paths, func(left int, right int) bool {
		if paths[left].modifiedAt.Equal(paths[right].modifiedAt) {
			return paths[left].path > paths[right].path
		}
		return paths[left].modifiedAt.After(paths[right].modifiedAt)
	})
	result := make([]string, 0, len(paths)+1)
	for _, path := range paths {
		result = append(result, path.path)
	}
	legacyPath := filepath.Join(channelsDirectory, channel+".log")
	if info, err := os.Stat(legacyPath); err == nil && info.Mode().IsRegular() {
		result = append(result, legacyPath)
	}
	return result
}

func tailNonEmptyLinesFromPaths(paths []string, limit int) ([]string, error) {
	if limit < 1 {
		return []string{}, nil
	}
	var lines []string
	var readError error
	readAny := false
	for _, path := range paths {
		remaining := limit - len(lines)
		if remaining <= 0 {
			break
		}
		persisted, err := tailNonEmptyLines(path, remaining)
		if err != nil {
			if !os.IsNotExist(err) {
				readError = errors.Join(readError, err)
			}
			continue
		}
		readAny = true
		lines = append(persisted, lines...)
	}
	if readAny {
		return tailLines(lines, limit), nil
	}
	return nil, readError
}
