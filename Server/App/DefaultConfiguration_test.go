package App

import (
	"encoding/json"
	"testing"
)

func TestDefaultAutoBeriWorldStableLevel(t *testing.T) {
	var configuration struct {
		Build struct {
			StableLevel int `json:"stableLevel"`
		} `json:"build"`
	}
	if err := json.Unmarshal(defaultConfiguration()["automation.autoBeriWorld"], &configuration); err != nil {
		t.Fatalf("decode default Auto Beri configuration: %v", err)
	}
	if configuration.Build.StableLevel != 5 {
		t.Fatalf("stableLevel = %d, want 5", configuration.Build.StableLevel)
	}
}
