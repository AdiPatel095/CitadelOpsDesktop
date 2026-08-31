package History

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestResolvePlayerSamplesRetentionDefaultsAndHostedCap(t *testing.T) {
	tests := []struct {
		name       string
		raw        json.RawMessage
		hosted     bool
		configured PlayerSamplesRetention
		effective  PlayerSamplesRetention
		maximum    PlayerSamplesRetention
		interval   int64
	}{
		{"missing", nil, false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited, 3600},
		{"invalid json", json.RawMessage(`{"version":`), false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited, 3600},
		{"unknown version", json.RawMessage(`{"version":2,"retention":"none"}`), false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited, 3600},
		{"unknown retention", json.RawMessage(`{"version":1,"retention":"forever"}`), true, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, 3600},
		{"local unlimited", json.RawMessage(`{"version":1,"retention":"unlimited"}`), false, PlayerSamplesRetentionUnlimited, PlayerSamplesRetentionUnlimited, PlayerSamplesRetentionUnlimited, 3600},
		{"hosted unlimited", json.RawMessage(`{"version":1,"retention":"unlimited"}`), true, PlayerSamplesRetentionUnlimited, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, 3600},
		{"local custom days", json.RawMessage(`{"version":1,"retention":"100d","recordingIntervalSeconds":300}`), false, PlayerSamplesRetention100Days, PlayerSamplesRetention100Days, PlayerSamplesRetentionUnlimited, 300},
		{"hosted custom days", json.RawMessage(`{"version":1,"retention":"100d","recordingIntervalSeconds":60}`), true, PlayerSamplesRetention100Days, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, 60},
		{"invalid interval defaults", json.RawMessage(`{"version":1,"retention":"30d","recordingIntervalSeconds":120}`), false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited, 3600},
		{"hosted none", json.RawMessage(`{"version":1,"retention":"none"}`), true, PlayerSamplesRetentionNone, PlayerSamplesRetentionNone, PlayerSamplesRetention30Days, 3600},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := ResolvePlayerSamplesRetention(test.raw, test.hosted)
			if policy.Configured != test.configured || policy.Effective != test.effective ||
				policy.Maximum != test.maximum || policy.Hosted != test.hosted ||
				policy.RecordingIntervalSeconds != test.interval {
				t.Fatalf("policy = %+v", policy)
			}
			if len(policy.Options) != 8 {
				t.Fatalf("option count = %d, want 8", len(policy.Options))
			}
			if len(policy.RecordingIntervalOptions) != 6 {
				t.Fatalf("recording interval option count = %d, want 6", len(policy.RecordingIntervalOptions))
			}
			for _, option := range policy.Options {
				if option.Days > 0 && option.Recordings != option.Days*PlayerSamplesRecordingsPerDay(test.interval) {
					t.Fatalf("option recordings = %+v for interval %d", option, test.interval)
				}
			}
			for _, option := range policy.Options {
				if option.Value == "" || option.Label == "" || option.Description == "" {
					t.Fatalf("incomplete option: %+v", option)
				}
			}
		})
	}
}

func TestPlayerSamplesRecordingIntervalsUseExactWhitelist(t *testing.T) {
	want := []int64{60, 300, 600, 900, 1800, 3600}
	for _, seconds := range want {
		if !ValidPlayerSamplesRecordingIntervalSeconds(seconds) {
			t.Fatalf("recording interval %d was rejected", seconds)
		}
		if got := PlayerSamplesRecordingsPerDay(seconds); got != 24*60*60/seconds {
			t.Fatalf("recordings per day for %d = %d", seconds, got)
		}
	}
	for _, seconds := range []int64{-1, 0, 30, 120, 7200} {
		if ValidPlayerSamplesRecordingIntervalSeconds(seconds) {
			t.Fatalf("invalid recording interval %d was accepted", seconds)
		}
	}
}

func TestPlayerSamplesRetentionDuration(t *testing.T) {
	duration, unlimited := PlayerSamplesRetentionDuration(PlayerSamplesRetention1Year)
	if unlimited || duration != 365*24*time.Hour {
		t.Fatalf("1y duration = %s unlimited=%t", duration, unlimited)
	}
	duration, unlimited = PlayerSamplesRetentionDuration(PlayerSamplesRetentionUnlimited)
	if !unlimited || duration != 0 {
		t.Fatalf("unlimited duration = %s unlimited=%t", duration, unlimited)
	}
	duration, unlimited = PlayerSamplesRetentionDuration("invalid")
	if unlimited || duration != 30*24*time.Hour {
		t.Fatalf("invalid duration = %s unlimited=%t", duration, unlimited)
	}
	duration, unlimited = PlayerSamplesRetentionDuration("100d")
	if unlimited || duration != 100*24*time.Hour {
		t.Fatalf("100d duration = %s unlimited=%t", duration, unlimited)
	}
}

func TestPlayerSamplesRetentionAcceptsArbitrarySafeWholeDays(t *testing.T) {
	for _, days := range []int64{1, 100, 365, MaxPlayerSamplesRetentionDays} {
		value, valid := PlayerSamplesRetentionForDays(days)
		if !valid {
			t.Fatalf("%d days was rejected", days)
		}
		parsed, valid := PlayerSamplesRetentionDays(value)
		if !valid || parsed != days || !ValidPlayerSamplesRetention(value) {
			t.Fatalf("%d days round trip = %q, %d, %t", days, value, parsed, valid)
		}
	}
	tooLarge := PlayerSamplesRetention(strconv.FormatInt(MaxPlayerSamplesRetentionDays+1, 10) + "d")
	for _, value := range []PlayerSamplesRetention{"0d", "-1d", "1.5d", "forever", tooLarge} {
		if ValidPlayerSamplesRetention(value) {
			t.Fatalf("invalid retention %q was accepted", value)
		}
	}
}
