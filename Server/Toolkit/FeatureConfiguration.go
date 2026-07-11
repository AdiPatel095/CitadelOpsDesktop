package Toolkit

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	featureview "CitadelDesktop/Server/GameFeatures/FeatureView"
	settingsview "CitadelDesktop/Server/GameFeatures/SettingsView"
	"CitadelDesktop/Server/Models"
	stsettings "CitadelDesktop/Server/Models/Settings"
)

type featureConfigureInput struct {
	FeatureID string          `json:"featureId"`
	Config    json.RawMessage `json:"config"`
}

type autoBirdToolConfig struct {
	MinDelay       int                 `json:"minDelay"`
	MaxDelay       int                 `json:"maxDelay"`
	MinSend        int                 `json:"minSend"`
	MinRPTDays     int                 `json:"minRPTDays"`
	IgnoreByCastle map[int]map[int]int `json:"ignoreByCastle"`
}

type schedulerToolConfig struct {
	MinAttackDelay       *float64                              `json:"minAttackDelay,omitempty"`
	MaxAttackDelay       *float64                              `json:"maxAttackDelay,omitempty"`
	UpgradeEreDelayMs    *int                                  `json:"upgradeEreDelayMs,omitempty"`
	UpgradeCoinThreshold *float64                              `json:"upgradeCoinThreshold,omitempty"`
	ManualFocusIdleSec   *int                                  `json:"manualFocusIdleSec,omitempty"`
	TabPriorities        map[string]stsettings.TabPriority     `json:"tabPriorities,omitempty"`
	FeatureSchedules     map[string]stsettings.FeatureSchedule `json:"featureSchedules,omitempty"`
}

func registerFeatureConfigurationTool(harness *Harness) error {
	return harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.feature.configure",
			Description: "Validate and persist a canonical automation configuration. Config shapes match state.settings fields; auto_bird uses minDelay, maxDelay, minSend, minRPTDays, and ignoreByCastle. scheduler accepts a partial patch.",
			InputSchema: objectSchema(map[string]interface{}{
				"featureId": enumProperty(
					"Configuration owner.",
					"auto_bird", "auto_station", "recruit_troops", "auto_tool", "auto_sceat_resources",
					"auto_hospital", "auto_tci", "auto_beri_world", "scheduler",
				),
				"config": map[string]interface{}{
					"type":        "object",
					"description": "Complete canonical feature config, except scheduler which is a partial patch.",
				},
			}, "featureId", "config"),
			Effect: EffectControl,
			Tags:   []string{"feature", "configuration"},
		},
		Handler: configureFeature,
	})
}

func configureFeature(_ context.Context, raw json.RawMessage) (interface{}, error) {
	input, err := decodeStrict[featureConfigureInput](raw)
	if err != nil {
		return nil, err
	}
	if len(input.Config) == 0 || !json.Valid(input.Config) || input.Config[0] != '{' {
		return nil, toolError("invalid_arguments", "config must be a JSON object")
	}
	featureID := canonicalFeatureID(input.FeatureID)
	state := Models.GetSettingsState()

	switch featureID {
	case "auto_bird":
		config, decodeErr := decodeStrict[autoBirdToolConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if config.MinDelay < 0 || config.MaxDelay < config.MinDelay || config.MinSend < 0 || config.MinRPTDays < 0 || config.MinRPTDays > 30 {
			return nil, toolError("invalid_arguments", "auto_bird delay, minimum send, or RPT range is invalid")
		}
		if config.IgnoreByCastle == nil {
			config.IgnoreByCastle = map[int]map[int]int{}
		}
		if err := validateNestedAmounts(config.IgnoreByCastle); err != nil {
			return nil, err
		}
		if err := writeAutoBirdToolConfig(config); err != nil {
			return nil, toolError("persistence_failed", "auto_bird: %v", err)
		}
		state.AutoBirdDelay = stsettings.AutoBirdDelayConfig{
			MinDelay: config.MinDelay, MaxDelay: config.MaxDelay, MinSend: config.MinSend, MinRPTDays: config.MinRPTDays,
		}
		state.UpdateBirdIgnoreList(config.IgnoreByCastle)
	case "auto_station":
		config, decodeErr := decodeStrict[stsettings.AutoStationConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		config = config.Normalize()
		encoded, _ := json.Marshal(stsettings.AutoStationClientStateFromConfig(config))
		if err := stsettings.WriteAutoStationClientFile(encoded); err != nil {
			return nil, toolError("persistence_failed", "auto_station: %v", err)
		}
		state.UpdateAutoStationConfig(config)
	case "recruit_troops":
		config, decodeErr := decodeStrict[stsettings.RecruitTroopsConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		config = config.Normalize()
		if err := stsettings.WriteRecruitTroopsConfig(config); err != nil {
			return nil, toolError("persistence_failed", "recruit_troops: %v", err)
		}
		state.UpdateRecruitTroopsConfig(config)
		settingsview.NotifyRecruitTroopsSettingsChanged()
	case "auto_tool":
		config, decodeErr := decodeStrict[stsettings.AutoToolConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		config = config.Normalize()
		if err := stsettings.WriteAutoToolConfig(config); err != nil {
			return nil, toolError("persistence_failed", "auto_tool: %v", err)
		}
		state.UpdateAutoToolConfig(config)
		settingsview.NotifyAutoToolSettingsChanged()
	case "auto_sceat_resources":
		config, decodeErr := decodeStrict[stsettings.AutoSceatResConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		config = config.Normalize()
		if err := stsettings.WriteAutoSceatResConfig(config); err != nil {
			return nil, toolError("persistence_failed", "auto_sceat_resources: %v", err)
		}
		state.UpdateAutoSceatResConfig(config)
		settingsview.NotifyAutoSceatResSettingsChanged()
	case "auto_hospital":
		config, decodeErr := decodeStrict[stsettings.AutoHospitalConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		config = config.Normalize()
		if err := stsettings.WriteAutoHospitalConfig(config); err != nil {
			return nil, toolError("persistence_failed", "auto_hospital: %v", err)
		}
		state.UpdateAutoHospitalConfig(config)
		settingsview.NotifyAutoHospitalSettingsChanged()
	case "auto_tci":
		config, decodeErr := decodeStrict[stsettings.AutoTCIConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if config.Targets == nil {
			config.Targets = map[int]map[int]stsettings.AutoTCILevelTarget{}
		}
		for castleID, targets := range config.Targets {
			if castleID <= 0 {
				return nil, toolError("invalid_arguments", "auto_tci castle IDs must be positive")
			}
			for constructionItemID, target := range targets {
				if constructionItemID <= 0 {
					return nil, toolError("invalid_arguments", "auto_tci construction item IDs must be positive")
				}
				targets[constructionItemID] = target.Normalize()
			}
		}
		state.UpdateAutoTCIList(config.Targets)
	case "auto_beri_world":
		config, decodeErr := decodeStrict[stsettings.AutoBeriWorldConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if err := stsettings.WriteAutoBeriWorldConfig(config); err != nil {
			return nil, toolError("persistence_failed", "auto_beri_world: %v", err)
		}
		state.AutoBeriWorld = stsettings.ReadAutoBeriWorldConfig()
		featureview.SyncBeriCastleFromSettings()
		featureview.SyncKutSourceFromMainCastle()
	case "scheduler":
		config, decodeErr := decodeStrict[schedulerToolConfig](input.Config)
		if decodeErr != nil {
			return nil, decodeErr
		}
		applySchedulerToolConfig(state, config)
		if err := stsettings.PersistSchedulerSettings(state); err != nil {
			return nil, toolError("persistence_failed", "scheduler: %v", err)
		}
		settingsview.NotifyRecruitTroopsSettingsChanged()
		settingsview.NotifyAutoToolSettingsChanged()
		settingsview.NotifyAutoSceatResSettingsChanged()
		settingsview.NotifyAutoHospitalSettingsChanged()
	default:
		return nil, toolError("invalid_arguments", "feature %q has no configurable adapter", input.FeatureID)
	}

	return featureConfiguration(featureID), nil
}

func featureConfiguration(featureID string) interface{} {
	state := Models.GetSettingsState()
	switch featureID {
	case "auto_bird":
		return autoBirdToolConfig{
			MinDelay:       state.AutoBirdDelay.MinDelay,
			MaxDelay:       state.AutoBirdDelay.MaxDelay,
			MinSend:        state.AutoBirdDelay.MinSend,
			MinRPTDays:     state.AutoBirdDelay.MinRPTDays,
			IgnoreByCastle: state.BirdIgnoreList.Troops,
		}
	case "auto_station":
		return state.AutoStation
	case "recruit_troops":
		return state.RecruitTroopsList
	case "auto_tool":
		return state.AutoToolList
	case "auto_sceat_resources":
		return state.AutoSceatRes
	case "auto_hospital":
		return state.AutoHospital
	case "auto_tci":
		return state.AutoTCIList
	case "auto_beri_world":
		return state.AutoBeriWorld
	case "scheduler":
		return map[string]interface{}{
			"minAttackDelay":       state.MinAttackDelay,
			"maxAttackDelay":       state.MaxAttackDelay,
			"upgradeEreDelayMs":    state.UpgradeEreDelayMs,
			"upgradeCoinThreshold": state.UpgradeCoinThreshold,
			"manualFocusIdleSec":   state.ManualFocusIdleSec,
			"tabPriorities":        state.TabPriorities,
			"featureSchedules":     state.FeatureSchedules,
		}
	default:
		return map[string]interface{}{}
	}
}

func validateNestedAmounts(values map[int]map[int]int) error {
	for parentID, entries := range values {
		if parentID <= 0 {
			return toolError("invalid_arguments", "castle IDs must be positive")
		}
		for itemID, amount := range entries {
			if itemID <= 0 || amount < 0 {
				return toolError("invalid_arguments", "unit IDs must be positive and amounts must not be negative")
			}
		}
	}
	return nil
}

func writeAutoBirdToolConfig(config autoBirdToolConfig) error {
	var root map[string]interface{}
	if err := json.Unmarshal(stsettings.ReadAutoBirdClientFile(), &root); err != nil {
		if err := json.Unmarshal(stsettings.DefaultAutoBirdClientJSON(), &root); err != nil {
			return err
		}
	}
	settings := make(map[string]interface{}, len(config.IgnoreByCastle))
	for castleID, units := range config.IgnoreByCastle {
		unitIDs := make([]int, 0, len(units))
		for unitID := range units {
			unitIDs = append(unitIDs, unitID)
		}
		sort.Ints(unitIDs)
		rows := make([]map[string]int, 0, len(unitIDs))
		for _, unitID := range unitIDs {
			amount := units[unitID]
			rows = append(rows, map[string]int{"id": unitID, "amount": amount})
		}
		settings[strconv.Itoa(castleID)] = rows
	}
	root["version"] = 1
	root["ignoreSettings"] = map[string]interface{}{
		"settings":   settings,
		"minDelay":   config.MinDelay,
		"maxDelay":   config.MaxDelay,
		"minSend":    config.MinSend,
		"minRPTDays": config.MinRPTDays,
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return stsettings.WriteAutoBirdClientFile(append(encoded, '\n'))
}

func applySchedulerToolConfig(state *Models.SettingsState, config schedulerToolConfig) {
	if config.MinAttackDelay != nil {
		state.MinAttackDelay = *config.MinAttackDelay
	}
	if config.MaxAttackDelay != nil {
		state.MaxAttackDelay = *config.MaxAttackDelay
	}
	if config.UpgradeEreDelayMs != nil {
		state.UpgradeEreDelayMs = *config.UpgradeEreDelayMs
	}
	if config.UpgradeCoinThreshold != nil {
		state.UpgradeCoinThreshold = *config.UpgradeCoinThreshold
	}
	if config.ManualFocusIdleSec != nil {
		state.ManualFocusIdleSec = stsettings.ClampManualFocusIdleSec(*config.ManualFocusIdleSec)
	}
	if config.TabPriorities != nil {
		state.TabPriorities = config.TabPriorities
	}
	if config.FeatureSchedules != nil {
		state.FeatureSchedules = stsettings.NormalizeFeatureSchedules(config.FeatureSchedules)
	}
}
