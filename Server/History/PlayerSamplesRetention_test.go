package History

import (
	"encoding/json"
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
	}{
		{"missing", nil, false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited},
		{"invalid json", json.RawMessage(`{"version":`), false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited},
		{"unknown version", json.RawMessage(`{"version":2,"retention":"none"}`), false, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetentionUnlimited},
		{"unknown retention", json.RawMessage(`{"version":1,"retention":"forever"}`), true, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days},
		{"local unlimited", json.RawMessage(`{"version":1,"retention":"unlimited"}`), false, PlayerSamplesRetentionUnlimited, PlayerSamplesRetentionUnlimited, PlayerSamplesRetentionUnlimited},
		{"hosted unlimited", json.RawMessage(`{"version":1,"retention":"unlimited"}`), true, PlayerSamplesRetentionUnlimited, PlayerSamplesRetention30Days, PlayerSamplesRetention30Days},
		{"hosted none", json.RawMessage(`{"version":1,"retention":"none"}`), true, PlayerSamplesRetentionNone, PlayerSamplesRetentionNone, PlayerSamplesRetention30Days},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := ResolvePlayerSamplesRetention(test.raw, test.hosted)
			if policy.Configured != test.configured || policy.Effective != test.effective ||
				policy.Maximum != test.maximum || policy.Hosted != test.hosted {
				t.Fatalf("policy = %+v", policy)
			}
			if len(policy.Options) != 7 {
				t.Fatalf("option count = %d, want 7", len(policy.Options))
			}
			for _, option := range policy.Options {
				if option.Value == "" || option.Label == "" || option.Description == "" {
					t.Fatalf("incomplete option: %+v", option)
				}
			}
		})
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
}
