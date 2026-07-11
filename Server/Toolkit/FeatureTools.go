package Toolkit

import (
	"context"
	"encoding/json"
	"strings"

	featureview "CitadelDesktop/Server/GameFeatures/FeatureView"
	settingsview "CitadelDesktop/Server/GameFeatures/SettingsView"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Scheduler"
)

// FeatureStatus separates persisted intent (configured) from the actual loop state (running).
type FeatureStatus struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Available        bool   `json:"available"`
	Configured       bool   `json:"configured"`
	Running          bool   `json:"running"`
	State            string `json:"state,omitempty"`
	Detail           string `json:"detail,omitempty"`
	NextWakeUpUnixMs int64  `json:"nextWakeUpUnixMs,omitempty"`
	ThreatCount      int    `json:"threatCount,omitempty"`
	NextImpactUnixMs int64  `json:"nextImpactUnixMs,omitempty"`
}

type featureSetEnabledInput struct {
	FeatureID string `json:"featureId"`
	Enabled   bool   `json:"enabled"`
}

func registerFeatureTools(harness *Harness) error {
	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.feature.list",
			Description: "List every controllable Citadel automation feature, including configured intent, actual runtime state, availability, and wake timing.",
			InputSchema: objectSchema(map[string]interface{}{}),
			Effect:      EffectRead,
			Tags:        []string{"feature", "read"},
		},
		Handler: func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			if _, err := decodeStrict[struct{}](raw); err != nil {
				return nil, err
			}
			return featureStatuses(), nil
		},
	}); err != nil {
		return err
	}

	featureIDs := []string{
		"game_session", "auto_bird", "auto_station", "recruit_troops", "auto_tool",
		"auto_sceat_resources", "auto_hospital", "auto_tci", "auto_beri_world",
	}
	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.feature.set_enabled",
			Description: "Idempotently start or stop one Citadel automation feature. This changes runtime behavior but does not replace feature configuration.",
			InputSchema: objectSchema(map[string]interface{}{
				"featureId": enumProperty("Feature identifier returned by citadel.feature.list.", featureIDs...),
				"enabled":   schemaProperty("boolean", "True starts the feature; false stops it."),
			}, "featureId", "enabled"),
			Effect: EffectControl,
			Tags:   []string{"feature", "control"},
		},
		Handler: setFeatureEnabled,
	}); err != nil {
		return err
	}
	return registerFeatureConfigurationTool(harness)
}

func featureStatuses() []FeatureStatus {
	settings := Models.GetSettingsState()
	connection := ResponseRegistry.GetGameConnectionStatus()
	stationEnabled, stationState, threatCount, nextImpact, stationDetail := featureview.GetAutoStationStatus()

	return []FeatureStatus{
		{
			ID:         "game_session",
			Name:       "Game session",
			Available:  true,
			Configured: settings.BotEnabled,
			Running:    connection.BrowserRunning || connection.SocketConnected,
			State:      string(connection.State),
			Detail:     connection.Detail,
		},
		{
			ID:               "auto_bird",
			Name:             "Auto Bird",
			Available:        true,
			Configured:       settings.AutoBirdEnabled,
			Running:          featureview.IsAutoBirdRunning(),
			NextWakeUpUnixMs: featureview.GetAutoBirdNextWakeUp(),
		},
		{
			ID:               "auto_station",
			Name:             "Auto Station",
			Available:        true,
			Configured:       settings.AutoStationEnabled,
			Running:          stationEnabled,
			State:            stationState,
			Detail:           stationDetail,
			ThreatCount:      threatCount,
			NextImpactUnixMs: nextImpact,
		},
		{
			ID:         "recruit_troops",
			Name:       "Auto Recruit",
			Available:  true,
			Configured: settings.RecruitTroopsEnabled,
			Running:    settingsview.IsRecruitTroopsRunning(),
		},
		{
			ID:         "auto_tool",
			Name:       "Auto Tool",
			Available:  true,
			Configured: settings.AutoToolEnabled,
			Running:    settingsview.IsAutoToolRunning(),
		},
		{
			ID:         "auto_sceat_resources",
			Name:       "Auto Sceat Resources",
			Available:  true,
			Configured: settings.AutoSceatResEnabled,
			Running:    settingsview.IsAutoSceatResRunning(),
		},
		{
			ID:         "auto_hospital",
			Name:       "Auto Hospital",
			Available:  true,
			Configured: settings.AutoHospitalEnabled,
			Running:    settingsview.IsAutoHospitalRunning(),
		},
		{
			ID:               "auto_tci",
			Name:             "Auto TCI",
			Available:        true,
			Configured:       settings.AutoTCIEnabled,
			Running:          featureview.IsAutoTCIRunning(),
			NextWakeUpUnixMs: featureview.GetAutoTCINextWakeUp(),
		},
		{
			ID:         "auto_beri_world",
			Name:       "Auto Beri World",
			Available:  false,
			Configured: settings.AutoBeriWorldEnabled,
			Running:    featureview.IsAutoBeriWorldRunning(),
			Detail:     "Disabled in the current feature implementation",
		},
	}
}

func setFeatureEnabled(_ context.Context, raw json.RawMessage) (interface{}, error) {
	input, err := decodeStrict[featureSetEnabledInput](raw)
	if err != nil {
		return nil, err
	}
	featureID := canonicalFeatureID(input.FeatureID)
	settings := Models.GetSettingsState()

	switch featureID {
	case "game_session":
		settings.BotEnabled = input.Enabled
		if input.Enabled {
			Scheduler.GetScheduler().Start()
			ResponseRegistry.ReloadGameTab()
		} else {
			Scheduler.GetScheduler().Stop()
			ResponseRegistry.DisconnectGameWebSocket()
		}
	case "auto_bird":
		settings.AutoBirdEnabled = input.Enabled
		if input.Enabled {
			featureview.StartAutoBird()
		} else {
			featureview.StopAutoBird()
		}
	case "auto_station":
		settings.AutoStationEnabled = input.Enabled
		if input.Enabled {
			featureview.StartAutoStation()
		} else {
			featureview.StopAutoStation()
		}
	case "recruit_troops":
		settings.RecruitTroopsEnabled = input.Enabled
		if input.Enabled {
			settingsview.StartRecruitTroops()
		} else {
			settingsview.StopRecruitTroops()
		}
	case "auto_tool":
		settings.AutoToolEnabled = input.Enabled
		if input.Enabled {
			settingsview.StartAutoTool()
		} else {
			settingsview.StopAutoTool()
		}
	case "auto_sceat_resources":
		settings.AutoSceatResEnabled = input.Enabled
		if input.Enabled {
			settingsview.StartAutoSceatRes()
		} else {
			settingsview.StopAutoSceatRes()
		}
	case "auto_hospital":
		settings.AutoHospitalEnabled = input.Enabled
		if input.Enabled {
			settingsview.StartAutoHospital()
		} else {
			settingsview.StopAutoHospital()
		}
	case "auto_tci":
		settings.AutoTCIEnabled = input.Enabled
		if input.Enabled {
			featureview.StartAutoTCI()
		} else {
			featureview.StopAutoTCI()
		}
	case "auto_beri_world":
		if input.Enabled {
			return nil, toolError("feature_unavailable", "auto_beri_world is disabled in the current implementation")
		}
		settings.AutoBeriWorldEnabled = false
		featureview.StopAutoBeriWorld()
	default:
		return nil, toolError("invalid_arguments", "unknown featureId %q", input.FeatureID)
	}

	for _, status := range featureStatuses() {
		if status.ID == featureID {
			return status, nil
		}
	}
	return nil, toolError("not_found", "feature %q disappeared after update", featureID)
}

func canonicalFeatureID(value string) string {
	compact := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(value))
	switch compact {
	case "gamesession", "bot":
		return "game_session"
	case "autobird":
		return "auto_bird"
	case "autostation":
		return "auto_station"
	case "recruittroops", "autorecruit":
		return "recruit_troops"
	case "autotool":
		return "auto_tool"
	case "autosceatresources", "autosceatres":
		return "auto_sceat_resources"
	case "autohospital":
		return "auto_hospital"
	case "autotci":
		return "auto_tci"
	case "autoberiworld":
		return "auto_beri_world"
	default:
		return value
	}
}
