package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/PrivateMetrics"
	"CitadelDesktop/Server/RiftTemplates"
)

func TestDefaultAutoBeriWorldStableLevel(t *testing.T) {
	var configuration struct {
		Build struct {
			StableLevel int `json:"stableLevel"`
		} `json:"build"`
		DailyAttackLimit int64 `json:"dailyAttackLimit"`
	}
	if err := json.Unmarshal(defaultConfiguration()["automation.autoBeriWorld"], &configuration); err != nil {
		t.Fatalf("decode default Auto Beri configuration: %v", err)
	}
	if configuration.Build.StableLevel != 5 {
		t.Fatalf("stableLevel = %d, want 5", configuration.Build.StableLevel)
	}
	if configuration.DailyAttackLimit != 0 {
		t.Fatalf("dailyAttackLimit = %d, want disabled default 0", configuration.DailyAttackLimit)
	}
}

func TestDefaultAutoEquipmentCleanupInterval(t *testing.T) {
	var configuration struct {
		Version          int `json:"version"`
		CheckIntervalSec int `json:"checkIntervalSec"`
	}
	if err := json.Unmarshal(defaultConfiguration()["automation.autoEquipmentCleanup"], &configuration); err != nil {
		t.Fatalf("decode default Auto Equipment Cleanup configuration: %v", err)
	}
	if configuration.Version != 1 || configuration.CheckIntervalSec != 60 {
		t.Fatalf("Auto Equipment Cleanup defaults = %+v", configuration)
	}
}

func TestDefaultAutoKhanRageControlsAreDisabled(t *testing.T) {
	var configuration struct {
		MaxRageChain             int64 `json:"maxRageChain"`
		RequireActiveRageBooster bool  `json:"requireActiveRageBooster"`
	}
	if err := json.Unmarshal(defaultConfiguration()["automation.autoKhan"], &configuration); err != nil {
		t.Fatalf("decode default Auto Khan configuration: %v", err)
	}
	if configuration.MaxRageChain != 0 || configuration.RequireActiveRageBooster {
		t.Fatalf("Auto Khan rage defaults = %+v", configuration)
	}
}

func TestDefaultPresetDocumentsAllowSectionScopedFirstSave(t *testing.T) {
	for _, section := range []string{"attacks.presets", "defense.presets"} {
		var document struct {
			Version int               `json:"version"`
			Presets []json.RawMessage `json:"presets"`
		}
		if err := json.Unmarshal(defaultConfiguration()[section], &document); err != nil {
			t.Fatalf("decode default %s: %v", section, err)
		}
		if document.Version != 1 || len(document.Presets) != 0 {
			t.Fatalf("default %s = %+v", section, document)
		}
	}
}

func TestDefaultRiftTemplateDocumentIsAccountOwnedAndEmpty(t *testing.T) {
	document, err := RiftTemplates.Decode(defaultConfiguration()[RiftTemplates.ConfigurationSection])
	if err != nil {
		t.Fatal(err)
	}
	if document.Version != 1 || len(document.Launches) != 0 || len(document.DeletedLaunchIDs) != 0 {
		t.Fatalf("default Rift templates = %+v", document)
	}
}

func TestDefaultPlayerSamplesRetention(t *testing.T) {
	var configuration History.PlayerSamplesConfiguration
	if err := json.Unmarshal(defaultConfiguration()[History.PlayerSamplesConfigurationSection], &configuration); err != nil {
		t.Fatalf("decode default player samples retention: %v", err)
	}
	if configuration.Version != 1 || configuration.Retention != History.PlayerSamplesRetention30Days {
		t.Fatalf("player samples retention defaults = %+v", configuration)
	}
}

func TestApplicationPlayerSamplesRetentionPolicyAppliesHostedCap(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		History.PlayerSamplesConfigurationSection: json.RawMessage(`{"version":1,"retention":"137d","recordingIntervalSeconds":300}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Configuration: configuration}
	local := application.playerSamplesRetentionPolicy()
	if local.Configured != History.PlayerSamplesRetention("137d") ||
		local.Effective != History.PlayerSamplesRetention("137d") ||
		local.RecordingIntervalSeconds != 300 || local.Maximum != History.PlayerSamplesRetentionUnlimited || local.Hosted {
		t.Fatalf("local player samples retention = %+v", local)
	}
	application.BackgroundOnly = true
	hosted := application.playerSamplesRetentionPolicy()
	if hosted.Configured != History.PlayerSamplesRetention("137d") ||
		hosted.Effective != History.PlayerSamplesRetention30Days ||
		hosted.RecordingIntervalSeconds != 300 || !hosted.Hosted {
		t.Fatalf("hosted player samples retention = %+v", hosted)
	}
}

func TestDesktopApplicationRejectsHostedPrivateMetricsPublishing(t *testing.T) {
	client, err := PrivateMetrics.NewClient(PrivateMetrics.ClientConfig{
		Endpoint: "https://backend.example/internal/private-metrics",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		client    *PrivateMetrics.Client
		placement *PrivateMetrics.Placement
	}{
		{name: "backend client", client: client},
		{name: "placement grant", placement: &PrivateMetrics.Placement{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(context.Background(), Config{
				DataDir: t.TempDir(), PrivateMetricsClient: test.client,
				PrivateMetricsPlacement: test.placement,
			})
			if err == nil || !strings.Contains(err.Error(), "requires hosted background mode") {
				t.Fatalf("desktop private metrics error = %v", err)
			}
		})
	}
}
