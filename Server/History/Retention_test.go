package History

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestCompactPlayerSamplesKeepsUTCTiersPerPlayer(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	samples := []PlayerSample{
		{TimestampUnix: now.Add(-31 * 24 * time.Hour).Unix(), PlayerID: 1, Might: 1},
		{TimestampUnix: time.Date(2026, 8, 10, 0, 5, 0, 0, time.UTC).Unix(), PlayerID: 1, Might: 2},
		{TimestampUnix: time.Date(2026, 8, 10, 23, 55, 0, 0, time.UTC).Unix(), PlayerID: 1, Might: 3},
		{TimestampUnix: time.Date(2026, 8, 18, 10, 1, 0, 0, time.UTC).Unix(), PlayerID: 1, Might: 4},
		{TimestampUnix: time.Date(2026, 8, 18, 10, 59, 0, 0, time.UTC).Unix(), PlayerID: 1, Might: 5},
		{TimestampUnix: time.Date(2026, 8, 18, 11, 2, 0, 0, time.UTC).Unix(), PlayerID: 1, Might: 6},
		{TimestampUnix: now.Add(-2 * time.Hour).Unix(), PlayerID: 1, Might: 7},
		{TimestampUnix: now.Add(-2*time.Hour + time.Minute).Unix(), PlayerID: 1, Might: 8},
		{TimestampUnix: time.Date(2026, 8, 18, 10, 2, 0, 0, time.UTC).Unix(), PlayerID: 2, Might: 9},
		{TimestampUnix: time.Date(2026, 8, 18, 10, 40, 0, 0, time.UTC).Unix(), PlayerID: 2, Might: 10},
		{TimestampUnix: now.Add(-30 * 24 * time.Hour).Unix(), PlayerID: 3, Might: 11},
		{TimestampUnix: now.Add(-30*24*time.Hour + time.Minute).Unix(), PlayerID: 3, Might: 12},
		{TimestampUnix: now.Add(-7 * 24 * time.Hour).Unix(), PlayerID: 3, Might: 13},
		{TimestampUnix: now.Add(-7*24*time.Hour + time.Minute).Unix(), PlayerID: 3, Might: 14},
		{TimestampUnix: now.Add(-24 * time.Hour).Unix(), PlayerID: 3, Might: 15},
		{TimestampUnix: now.Add(-24*time.Hour + time.Minute).Unix(), PlayerID: 3, Might: 16},
	}
	for _, sample := range samples {
		if err := store.Append(CollectionPlayerSamples, sample); err != nil {
			t.Fatal(err)
		}
	}
	// An unreadable line is preserved rather than treated as disposable data.
	path := filepath.Join(dataDir, "History", "PlayerSamples.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("future-format-line\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := store.CompactPlayerSamples(now)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.ScannedRows != 17 || report.DeletedRows != 6 || report.KeptRows != 11 {
		t.Fatalf("retention report = %+v, want 17 scanned, 6 deleted, 11 kept", report)
	}
	assertPlayerSamples(t, store, 1, []int64{
		time.Date(2026, 8, 10, 0, 5, 0, 0, time.UTC).Unix(),
		time.Date(2026, 8, 18, 10, 1, 0, 0, time.UTC).Unix(),
		time.Date(2026, 8, 18, 11, 2, 0, 0, time.UTC).Unix(),
		now.Add(-2 * time.Hour).Unix(),
		now.Add(-2*time.Hour + time.Minute).Unix(),
	})
	assertPlayerSamples(t, store, 2, []int64{time.Date(2026, 8, 18, 10, 2, 0, 0, time.UTC).Unix()})
	assertPlayerSamples(t, store, 3, []int64{
		now.Add(-30 * 24 * time.Hour).Unix(),
		now.Add(-7 * 24 * time.Hour).Unix(),
		now.Add(-24 * time.Hour).Unix(),
		now.Add(-24*time.Hour + time.Minute).Unix(),
	})
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(contents, "future-format-line") {
		t.Fatal("retention discarded an unrecognized history line")
	}
}

func TestCompactPlayerSamplesIfDueRunsHourly(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	if _, ran, err := store.CompactPlayerSamplesIfDue(now); err != nil || !ran {
		t.Fatalf("first compaction ran=%t err=%v, want run", ran, err)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDue(now.Add(59 * time.Minute)); err != nil || ran {
		t.Fatalf("early compaction ran=%t err=%v, want skip", ran, err)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDue(now.Add(time.Hour)); err != nil || !ran {
		t.Fatalf("hourly compaction ran=%t err=%v, want run", ran, err)
	}
}

func TestCompactPlayerSamplesSupportsYearAndUnlimitedRetention(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	for _, sample := range []PlayerSample{
		{TimestampUnix: now.Add(-400 * 24 * time.Hour).Unix(), PlayerID: 1, Might: 1},
		{TimestampUnix: now.Add(-200 * 24 * time.Hour).Unix(), PlayerID: 1, Might: 2},
		{TimestampUnix: now.Add(-200*24*time.Hour + time.Hour).Unix(), PlayerID: 1, Might: 3},
	} {
		if err := store.Append(CollectionPlayerSamples, sample); err != nil {
			t.Fatal(err)
		}
	}

	report, err := store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetentionUnlimited)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Unlimited || !report.RetentionCutoff.IsZero() || report.DeletedRows != 1 {
		t.Fatalf("unlimited retention report = %+v", report)
	}
	assertPlayerSamples(t, store, 1, []int64{
		now.Add(-400 * 24 * time.Hour).Unix(),
		now.Add(-200 * 24 * time.Hour).Unix(),
	})

	report, err = store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention1Year)
	if err != nil {
		t.Fatal(err)
	}
	if report.Unlimited || !report.RetentionCutoff.Equal(now.Add(-365*24*time.Hour)) || report.DeletedRows != 1 {
		t.Fatalf("one-year retention report = %+v", report)
	}
	assertPlayerSamples(t, store, 1, []int64{now.Add(-200 * 24 * time.Hour).Unix()})
}

func TestCompactPlayerSamplesNoneClearsAllRowsAndPolicyChangesRunImmediately(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	if err := store.Append(CollectionPlayerSamples, PlayerSample{
		TimestampUnix: now.Unix(), PlayerID: 1, Might: 1,
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, "History", "PlayerSamples.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("future-format-line\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, ran, err := store.CompactPlayerSamplesIfDueWithRetention(now, PlayerSamplesRetentionUnlimited); err != nil || !ran {
		t.Fatalf("initial compaction ran=%t err=%v, want run", ran, err)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDueWithRetention(now.Add(time.Minute), PlayerSamplesRetentionUnlimited); err != nil || ran {
		t.Fatalf("same-policy compaction ran=%t err=%v, want skip", ran, err)
	}
	report, ran, err := store.CompactPlayerSamplesIfDueWithRetention(now.Add(2*time.Minute), PlayerSamplesRetentionNone)
	if err != nil || !ran {
		t.Fatalf("changed-policy compaction ran=%t err=%v, want run", ran, err)
	}
	if !report.Complete || report.ScannedRows != 2 || report.DeletedRows != 2 || report.KeptRows != 0 {
		t.Fatalf("none retention report = %+v", report)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 0 {
		t.Fatalf("none retention left %d bytes: %q", len(contents), contents)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDueWithRetention(now.Add(3*time.Minute), PlayerSamplesRetentionNone); err != nil || ran {
		t.Fatalf("repeated none compaction ran=%t err=%v, want skip", ran, err)
	}
}

func TestCapturePlayerSampleCannotCrossAppliedRetentionBoundary(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)

	t.Run("stale capture after none apply is rejected", func(t *testing.T) {
		store, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention30Days); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompactPlayerSamplesWithRetention(now.Add(time.Second), PlayerSamplesRetentionNone); err != nil {
			t.Fatal(err)
		}
		appended, _, err := store.CapturePlayerSampleWithRetention(PlayerSample{
			TimestampUnix: now.Add(2 * time.Second).Unix(), PlayerID: 1, Might: 10,
		}, now.Add(2*time.Second), PlayerSamplesRetention30Days)
		if err != nil {
			t.Fatal(err)
		}
		if appended {
			t.Fatal("stale 30-day capture appended after no-history apply")
		}
		assertPlayerSamples(t, store, 1, nil)
	})

	t.Run("none apply clears an earlier capture", func(t *testing.T) {
		store, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention30Days); err != nil {
			t.Fatal(err)
		}
		appended, _, err := store.CapturePlayerSampleWithRetention(PlayerSample{
			TimestampUnix: now.Add(time.Second).Unix(), PlayerID: 1, Might: 10,
		}, now.Add(time.Second), PlayerSamplesRetention30Days)
		if err != nil {
			t.Fatal(err)
		}
		if !appended {
			t.Fatal("matching 30-day capture was not appended")
		}
		if _, err := store.CompactPlayerSamplesWithRetention(now.Add(2*time.Second), PlayerSamplesRetentionNone); err != nil {
			t.Fatal(err)
		}
		assertPlayerSamples(t, store, 1, nil)
	})
}

func TestResolvedCompactionReadsPolicyInsideHistoryLock(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	if err := store.Append(CollectionPlayerSamples, PlayerSample{
		TimestampUnix: now.Unix(), PlayerID: 1, Might: 10,
	}); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	retention := PlayerSamplesRetention30Days
	started := make(chan struct{})
	done := make(chan struct{})
	var report PlayerSamplesRetentionReport
	var compactErr error
	go func() {
		close(started)
		report, compactErr = store.CompactPlayerSamplesWithResolvedRetention(now, func() PlayerSamplesRetention {
			return retention
		})
		close(done)
	}()
	<-started
	retention = PlayerSamplesRetentionNone
	store.mu.Unlock()
	<-done
	if compactErr != nil {
		t.Fatal(compactErr)
	}
	if report.Retention != PlayerSamplesRetentionNone || report.DeletedRows != 1 {
		t.Fatalf("resolved compaction report = %+v", report)
	}
	assertPlayerSamples(t, store, 1, nil)
}

func TestResolvedMaintenanceRetriesFailedPolicyChange(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	if _, err := store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention30Days); err != nil {
		t.Fatal(err)
	}
	historyDirectory := filepath.Join(dataDir, "History")
	if err := os.RemoveAll(historyDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyDirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompactPlayerSamplesWithResolvedRetention(now.Add(time.Minute), func() PlayerSamplesRetention {
		return PlayerSamplesRetentionNone
	}); err == nil {
		t.Fatal("broken history path unexpectedly accepted retention change")
	}
	if err := os.Remove(historyDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(historyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	report, ran, err := store.CompactPlayerSamplesIfDueResolved(now.Add(2*time.Minute), func() PlayerSamplesRetention {
		return PlayerSamplesRetentionNone
	})
	if err != nil || !ran {
		t.Fatalf("retry ran=%t err=%v, want successful retry", ran, err)
	}
	if report.Retention != PlayerSamplesRetentionNone || !report.Complete {
		t.Fatalf("retry report = %+v", report)
	}
}

func assertPlayerSamples(t *testing.T, store *Store, playerID State.PlayerID, want []int64) {
	t.Helper()
	samples, err := store.PlayerSamplesForPlayer(time.Time{}, 100, playerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != len(want) {
		t.Fatalf("player %d sample count = %d, want %d: %+v", playerID, len(samples), len(want), samples)
	}
	for index := range want {
		if samples[index].TimestampUnix != want[index] {
			t.Fatalf("player %d timestamps[%d] = %d, want %d", playerID, index, samples[index].TimestampUnix, want[index])
		}
	}
}

func containsLine(contents []byte, line string) bool {
	for _, candidate := range splitLines(string(contents)) {
		if candidate == line {
			return true
		}
	}
	return false
}

func splitLines(value string) []string {
	result := []string{}
	start := 0
	for index, character := range value {
		if character != '\n' {
			continue
		}
		result = append(result, value[start:index])
		start = index + 1
	}
	if start < len(value) {
		result = append(result, value[start:])
	}
	return result
}
