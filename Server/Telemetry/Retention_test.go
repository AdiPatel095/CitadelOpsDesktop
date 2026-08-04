package Telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPruneLogsKeepsOnlyTheRollingWindow(t *testing.T) {
	dataDir := t.TempDir()
	channelsDir := filepath.Join(dataDir, "Logs", "channels")
	if err := os.MkdirAll(filepath.Join(channelsDir, ChannelWebSocketGame), 0o755); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.Local)
	oldLine := formatLine(cutoff.Add(-time.Minute), "SEND", "old", "expired") + "\n"
	keptLine := formatLine(cutoff, "SEND", "kept", "inside window") + "\n"
	recentLine := formatLine(cutoff.Add(time.Hour), "MATCH", "recent", "inside window") + "\n"
	legacyPath := filepath.Join(channelsDir, ChannelAppSend+".log")
	if err := os.WriteFile(legacyPath, []byte(oldLine+keptLine+recentLine), 0o644); err != nil {
		t.Fatal(err)
	}

	expiredPath := filepath.Join(channelsDir, ChannelWebSocketGame, "2026-07-29-1.log")
	expiredContents := []byte(formatLine(cutoff.Add(-time.Hour), "RECV", "gam", "expired game traffic") + "\n")
	if err := os.WriteFile(expiredPath, expiredContents, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(expiredPath, cutoff.Add(-time.Hour), cutoff.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	recentPath := filepath.Join(channelsDir, ChannelWebSocketGame, "2026-07-29-2.log")
	if err := os.WriteFile(recentPath, []byte(formatLine(cutoff.Add(time.Hour), "RECV", "gam", "recent game traffic")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recentPath, cutoff.Add(time.Hour), cutoff.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	store := NewStore(100)
	store.fileMu.Lock()
	store.channelsDir = channelsDir
	store.fileMu.Unlock()
	defer store.Close()
	result, err := store.pruneLogs(cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if result.filesDeleted != 1 || result.filesCompacted != 1 {
		t.Fatalf("retention result = %#v, want one deletion and one compaction", result)
	}
	if result.bytesRemoved != int64(len(expiredContents)+len(oldLine)) {
		t.Fatalf("removed bytes = %d, want %d", result.bytesRemoved, len(expiredContents)+len(oldLine))
	}
	if _, err := os.Stat(expiredPath); !os.IsNotExist(err) {
		t.Fatalf("expired game log still exists: %v", err)
	}
	if _, err := os.Stat(recentPath); err != nil {
		t.Fatalf("recent game log was removed: %v", err)
	}
	contents, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != keptLine+recentLine {
		t.Fatalf("compacted legacy log = %q, want %q", contents, keptLine+recentLine)
	}
}

func TestPersistentChannelsRotateAndTailAcrossFiles(t *testing.T) {
	store := NewStore(100)
	if err := store.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	first := formatLine(now.Add(-4*time.Hour), "INFO", "TRANSPORT", "first activity")
	second := formatLine(now, "INFO", "TRANSPORT", "second activity")
	third := formatLine(now.Add(time.Minute), "INFO", "TRANSPORT", "same rotation activity")
	store.mu.Lock()
	store.appendLocked(ChannelAutoBird, first, now.Add(-4*time.Hour))
	store.mu.Unlock()
	store.flushPersistence()
	store.mu.Lock()
	store.appendLocked(ChannelAutoBird, second, now)
	store.mu.Unlock()
	store.flushPersistence()
	store.mu.Lock()
	store.appendLocked(ChannelAutoBird, third, now.Add(time.Minute))
	store.mu.Unlock()
	store.flushPersistence()

	paths := channelLogPathsNewest(store.channelsDir, ChannelAutoBird)
	if len(paths) != 2 {
		t.Fatalf("rotated paths = %v, want two files", paths)
	}
	tail := strings.Join(store.Tail(ChannelAutoBird, 10), "\n")
	if !strings.Contains(tail, "first activity") || !strings.Contains(tail, "second activity") || !strings.Contains(tail, "same rotation activity") {
		t.Fatalf("tail across rotated files = %q", tail)
	}
	if strings.Index(tail, "first activity") > strings.Index(tail, "second activity") {
		t.Fatalf("tail is not chronological: %q", tail)
	}
}
