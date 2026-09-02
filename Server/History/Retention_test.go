package History

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestCompactPlayerSamplesKeepsOneUTCHourlyRecordingPerPlayer(t *testing.T) {
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
	if !report.Complete || report.ScannedRows != 17 || report.DeletedRows != 7 || report.KeptRows != 10 {
		t.Fatalf("retention report = %+v, want 17 scanned, 7 deleted, 10 kept", report)
	}
	assertPlayerSamples(t, store, 1, []int64{
		time.Date(2026, 8, 10, 0, 5, 0, 0, time.UTC).Unix(),
		time.Date(2026, 8, 10, 23, 55, 0, 0, time.UTC).Unix(),
		time.Date(2026, 8, 18, 10, 1, 0, 0, time.UTC).Unix(),
		time.Date(2026, 8, 18, 11, 2, 0, 0, time.UTC).Unix(),
		now.Add(-2 * time.Hour).Unix(),
	})
	assertPlayerSamples(t, store, 2, []int64{time.Date(2026, 8, 18, 10, 2, 0, 0, time.UTC).Unix()})
	assertPlayerSamples(t, store, 3, []int64{
		now.Add(-30*24*time.Hour + time.Minute).Unix(),
		now.Add(-7 * 24 * time.Hour).Unix(),
		now.Add(-24 * time.Hour).Unix(),
	})
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !containsLine(contents, "future-format-line") {
		t.Fatal("retention discarded an unrecognized history line")
	}
}

func TestCompactPlayerSamplesIfDueRunsDaily(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 30, 0, 0, time.UTC)
	if _, ran, err := store.CompactPlayerSamplesIfDue(now); err != nil || !ran {
		t.Fatalf("first compaction ran=%t err=%v, want run", ran, err)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDue(now.Add(23 * time.Hour)); err != nil || ran {
		t.Fatalf("early compaction ran=%t err=%v, want skip", ran, err)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDue(now.Add(24 * time.Hour)); err != nil || !ran {
		t.Fatalf("daily compaction ran=%t err=%v, want run", ran, err)
	}
}

func TestCompactPlayerSamplesIfDueSkipsRoutineUnlimitedScans(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	minutePolicy := PlayerSamplesStoragePolicy{
		Retention:                PlayerSamplesRetentionUnlimited,
		RecordingIntervalSeconds: 60,
	}
	if _, ran, err := store.CompactPlayerSamplesIfDueWithPolicy(now, minutePolicy); err != nil || !ran {
		t.Fatalf("initial unlimited compaction ran=%t err=%v, want run", ran, err)
	}
	if _, ran, err := store.CompactPlayerSamplesIfDueWithPolicy(now.Add(365*24*time.Hour), minutePolicy); err != nil || ran {
		t.Fatalf("routine unlimited compaction ran=%t err=%v, want skip", ran, err)
	}
	fiveMinutePolicy := PlayerSamplesStoragePolicy{
		Retention:                PlayerSamplesRetentionUnlimited,
		RecordingIntervalSeconds: 5 * 60,
	}
	if _, ran, err := store.CompactPlayerSamplesIfDueWithPolicy(now.Add(365*24*time.Hour), fiveMinutePolicy); err != nil || !ran {
		t.Fatalf("changed unlimited cadence compaction ran=%t err=%v, want run", ran, err)
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
	if !report.Unlimited || !report.RetentionCutoff.IsZero() || report.DeletedRows != 0 {
		t.Fatalf("unlimited retention report = %+v", report)
	}
	assertPlayerSamples(t, store, 1, []int64{
		now.Add(-400 * 24 * time.Hour).Unix(),
		now.Add(-200 * 24 * time.Hour).Unix(),
		now.Add(-200*24*time.Hour + time.Hour).Unix(),
	})

	report, err = store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention1Year)
	if err != nil {
		t.Fatal(err)
	}
	if report.Unlimited || !report.RetentionCutoff.Equal(now.Add(-365*24*time.Hour)) || report.DeletedRows != 1 {
		t.Fatalf("one-year retention report = %+v", report)
	}
	assertPlayerSamples(t, store, 1, []int64{
		now.Add(-200 * 24 * time.Hour).Unix(),
		now.Add(-200*24*time.Hour + time.Hour).Unix(),
	})
}

func TestCompactPlayerSamplesFiniteDaysMatchHourlyRecordingProjection(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	rows := make([]json.RawMessage, 0, 365*24+1)
	for hoursAgo := 365 * 24; hoursAgo >= 0; hoursAgo-- {
		row, marshalErr := json.Marshal(PlayerSample{
			TimestampUnix: now.Add(-time.Duration(hoursAgo) * time.Hour).Unix(),
			PlayerID:      1,
			Might:         float64(hoursAgo),
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		rows = append(rows, row)
	}
	if err := store.Replace(CollectionPlayerSamples, rows); err != nil {
		t.Fatal(err)
	}

	report, err := store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention1Year)
	if err != nil {
		t.Fatal(err)
	}
	if report.KeptRows != 365*24 || report.DeletedRows != 1 {
		t.Fatalf("365-day report = %+v, want 8,760 retained recordings", report)
	}
	report, err = store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention100Days)
	if err != nil {
		t.Fatal(err)
	}
	if report.KeptRows != 100*24 || report.DeletedRows != (365-100)*24 {
		t.Fatalf("100-day report = %+v, want 2,400 retained recordings", report)
	}
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

func TestCapturePlayerSampleStoresAtMostOneRecordingPerUTCHourAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Date(2026, 8, 29, 12, 10, 0, 0, time.UTC)
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompactPlayerSamplesWithRetention(now, PlayerSamplesRetention100Days); err != nil {
		t.Fatal(err)
	}
	first := PlayerSample{TimestampUnix: now.Unix(), PlayerID: 1, Might: 10}
	if appended, _, err := store.CapturePlayerSampleWithRetention(first, now, PlayerSamplesRetention100Days); err != nil || !appended {
		t.Fatalf("first capture appended=%t err=%v", appended, err)
	}
	duplicate := PlayerSample{TimestampUnix: now.Add(40 * time.Minute).Unix(), PlayerID: 1, Might: 20}
	if appended, _, err := store.CapturePlayerSampleWithRetention(duplicate, now.Add(40*time.Minute), PlayerSamplesRetention100Days); err != nil || appended {
		t.Fatalf("same-hour capture appended=%t err=%v", appended, err)
	}
	nextHour := PlayerSample{TimestampUnix: now.Add(time.Hour).Unix(), PlayerID: 1, Might: 30}
	if appended, _, err := store.CapturePlayerSampleWithRetention(nextHour, now.Add(time.Hour), PlayerSamplesRetention100Days); err != nil || !appended {
		t.Fatalf("next-hour capture appended=%t err=%v", appended, err)
	}

	reopened, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.CompactPlayerSamplesWithRetention(now.Add(time.Hour), PlayerSamplesRetention100Days); err != nil {
		t.Fatal(err)
	}
	afterRestart := PlayerSample{TimestampUnix: now.Add(time.Hour + 20*time.Minute).Unix(), PlayerID: 1, Might: 40}
	if appended, _, err := reopened.CapturePlayerSampleWithRetention(afterRestart, now.Add(time.Hour+20*time.Minute), PlayerSamplesRetention100Days); err != nil || appended {
		t.Fatalf("restart same-hour capture appended=%t err=%v", appended, err)
	}
	assertPlayerSamples(t, reopened, 1, []int64{first.TimestampUnix, nextHour.TimestampUnix})
}

func TestCapturePlayerSampleUsesConfiguredIntervalAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	now := time.Date(2026, 8, 29, 12, 1, 0, 0, time.UTC)
	policy := PlayerSamplesStoragePolicy{
		Retention:                PlayerSamplesRetention100Days,
		RecordingIntervalSeconds: 5 * 60,
	}
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompactPlayerSamplesWithPolicy(now, policy); err != nil {
		t.Fatal(err)
	}
	first := PlayerSample{TimestampUnix: now.Unix(), PlayerID: 1, Might: 10}
	if appended, _, err := store.CapturePlayerSampleWithPolicy(first, now, policy); err != nil || !appended {
		t.Fatalf("first capture appended=%t err=%v", appended, err)
	}
	if appended, _, err := store.CapturePlayerSampleWithPolicy(PlayerSample{
		TimestampUnix: now.Add(3 * time.Minute).Unix(), PlayerID: 1, Might: 20,
	}, now.Add(3*time.Minute), policy); err != nil || appended {
		t.Fatalf("same five-minute bucket appended=%t err=%v", appended, err)
	}

	reopened, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.CompactPlayerSamplesWithPolicy(now.Add(3*time.Minute), policy); err != nil {
		t.Fatal(err)
	}
	if appended, _, err := reopened.CapturePlayerSampleWithPolicy(PlayerSample{
		TimestampUnix: now.Add(3 * time.Minute).Unix(), PlayerID: 1, Might: 30,
	}, now.Add(3*time.Minute), policy); err != nil || appended {
		t.Fatalf("restart duplicate appended=%t err=%v", appended, err)
	}
	next := PlayerSample{TimestampUnix: time.Date(2026, 8, 29, 12, 5, 0, 0, time.UTC).Unix(), PlayerID: 1, Might: 40}
	if appended, _, err := reopened.CapturePlayerSampleWithPolicy(next, time.Unix(next.TimestampUnix, 0), policy); err != nil || !appended {
		t.Fatalf("next bucket appended=%t err=%v", appended, err)
	}
	assertPlayerSamples(t, reopened, 1, []int64{first.TimestampUnix, next.TimestampUnix})
}

func TestPlayerSamplesIntervalChangeCompactsOnlyWhenCadenceSlows(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	for minute := 0; minute < 60; minute++ {
		if err := store.Append(CollectionPlayerSamples, PlayerSample{
			TimestampUnix: now.Add(time.Duration(minute) * time.Minute).Unix(), PlayerID: 1, Might: float64(minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	minutePolicy := PlayerSamplesStoragePolicy{Retention: PlayerSamplesRetentionUnlimited, RecordingIntervalSeconds: 60}
	if report, err := store.CompactPlayerSamplesWithPolicy(now.Add(time.Hour), minutePolicy); err != nil || report.KeptRows != 60 {
		t.Fatalf("minute policy report=%+v err=%v", report, err)
	}
	fifteenMinutePolicy := PlayerSamplesStoragePolicy{Retention: PlayerSamplesRetentionUnlimited, RecordingIntervalSeconds: 15 * 60}
	if _, _, err := store.PlayerSamplesForPlayerBounded(time.Time{}, 100, 1, 60); err != nil {
		t.Fatal(err)
	}
	if report, err := store.CompactPlayerSamplesWithPolicy(now.Add(time.Hour), fifteenMinutePolicy); err != nil || report.KeptRows != 4 || report.DeletedRows != 56 {
		t.Fatalf("fifteen-minute policy report=%+v err=%v", report, err)
	}
	compacted, servedInterval, err := store.PlayerSamplesForPlayerBounded(time.Time{}, 100, 1, 15*60)
	if err != nil {
		t.Fatal(err)
	}
	if servedInterval != 15*60 || len(compacted) != 4 {
		t.Fatalf("bounded history after compaction interval=%d samples=%+v", servedInterval, compacted)
	}
	if report, err := store.CompactPlayerSamplesWithPolicy(now.Add(time.Hour), minutePolicy); err != nil || report.KeptRows != 4 || report.DeletedRows != 0 {
		t.Fatalf("return to minute policy report=%+v err=%v", report, err)
	}
}

func TestCapturePlayerSampleRejectsStaleRecordingInterval(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	applied := PlayerSamplesStoragePolicy{Retention: PlayerSamplesRetention30Days, RecordingIntervalSeconds: 5 * 60}
	if _, err := store.CompactPlayerSamplesWithPolicy(now, applied); err != nil {
		t.Fatal(err)
	}
	stale := PlayerSamplesStoragePolicy{Retention: PlayerSamplesRetention30Days, RecordingIntervalSeconds: 60}
	if appended, _, err := store.CapturePlayerSampleWithPolicy(PlayerSample{
		TimestampUnix: now.Unix(), PlayerID: 1, Might: 10,
	}, now, stale); err != nil || appended {
		t.Fatalf("stale interval capture appended=%t err=%v", appended, err)
	}
	assertPlayerSamples(t, store, 1, nil)
}

func TestPlayerSamplesForPlayerBoundedPreservesFullSpan(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	for minute := 0; minute < 12; minute++ {
		if err := store.Append(CollectionPlayerSamples, PlayerSample{
			TimestampUnix: start.Add(time.Duration(minute) * time.Minute).Unix(), PlayerID: 1, Might: float64(minute),
			Currencies: map[string]float64{
				fmt.Sprintf("resource:%d", minute+10): float64(minute + 100),
				fmt.Sprintf("currency:%d", minute+20): float64(minute + 200),
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	samples, intervalSeconds, err := store.PlayerSamplesForPlayerBounded(time.Time{}, 5, 1, 60)
	if err != nil {
		t.Fatal(err)
	}
	if intervalSeconds != 4*60 || len(samples) != 3 {
		t.Fatalf("bounded interval=%d samples=%d: %+v", intervalSeconds, len(samples), samples)
	}
	want := []int64{start.Unix(), start.Add(4 * time.Minute).Unix(), start.Add(11 * time.Minute).Unix()}
	for index := range want {
		if samples[index].TimestampUnix != want[index] {
			t.Fatalf("sample[%d]=%d, want %d", index, samples[index].TimestampUnix, want[index])
		}
	}
	if samples[1].Currencies["resource:14"] != 104 || samples[1].Currencies["currency:24"] != 204 ||
		samples[2].Currencies["resource:21"] != 111 || samples[2].Currencies["currency:31"] != 211 {
		t.Fatalf("bounded samples lost wallet maps: %+v", samples)
	}

	// Building the index must not make later policy-aware captures invisible.
	policy := PlayerSamplesStoragePolicy{
		Retention:                PlayerSamplesRetentionUnlimited,
		RecordingIntervalSeconds: 60,
	}
	next := PlayerSample{
		TimestampUnix: start.Add(12 * time.Minute).Unix(),
		PlayerID:      1,
		Might:         12,
	}
	if _, err := store.CompactPlayerSamplesWithPolicy(time.Unix(next.TimestampUnix, 0), policy); err != nil {
		t.Fatal(err)
	}
	if appended, _, err := store.CapturePlayerSampleWithPolicy(next, time.Unix(next.TimestampUnix, 0), policy); err != nil || !appended {
		t.Fatalf("indexed capture appended=%t err=%v", appended, err)
	}
	samples, intervalSeconds, err = store.PlayerSamplesForPlayerBounded(time.Time{}, 5, 1, 60)
	if err != nil {
		t.Fatal(err)
	}
	if intervalSeconds != 4*60 || len(samples) != 4 || samples[3].TimestampUnix != next.TimestampUnix {
		t.Fatalf("bounded samples after indexed append interval=%d samples=%+v", intervalSeconds, samples)
	}

	// A collection replacement changes every byte offset and must force a
	// rebuild before the next bounded read.
	replacement := PlayerSample{
		TimestampUnix: start.Add(30 * time.Minute).Unix(),
		PlayerID:      1,
		Might:         99,
	}
	replacementJSON, err := json.Marshal(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(CollectionPlayerSamples, []json.RawMessage{replacementJSON}); err != nil {
		t.Fatal(err)
	}
	samples, intervalSeconds, err = store.PlayerSamplesForPlayerBounded(time.Time{}, 5, 1, 60)
	if err != nil {
		t.Fatal(err)
	}
	if intervalSeconds != 60 || len(samples) != 1 || samples[0].Might != replacement.Might {
		t.Fatalf("bounded samples after replacement interval=%d samples=%+v", intervalSeconds, samples)
	}
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
