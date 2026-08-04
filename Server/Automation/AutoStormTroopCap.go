package Automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type AutoStormTroopCapPreview struct {
	Available                bool    `json:"available"`
	MaximumTroops            int64   `json:"maximumTroops"`
	TroopsPerAttack          int64   `json:"troopsPerAttack"`
	MinimumTroops            int64   `json:"minimumTroops"`
	HistoryHours             int     `json:"historyHours"`
	AttacksInHistory         int64   `json:"attacksInHistory"`
	MeasuredAttacksInHistory int64   `json:"measuredAttacksInHistory"`
	TroopsSentInHistory      int64   `json:"troopsSentInHistory"`
	AverageTroopsPerHour     float64 `json:"averageTroopsPerHour"`
	BufferedTroops           int64   `json:"bufferedTroops"`
	Detail                   string  `json:"detail,omitempty"`
}

func PreviewAutoStormTroopCap(
	state State.GameState,
	configuration Configuration.Snapshot,
	gameData *GameData.Store,
	settingsJSON json.RawMessage,
	now time.Time,
) (AutoStormTroopCapPreview, error) {
	if gameData == nil {
		return AutoStormTroopCapPreview{}, fmt.Errorf("official game data is unavailable")
	}
	settings := defaultAutoStormSettings()
	if len(settingsJSON) == 0 {
		return AutoStormTroopCapPreview{}, fmt.Errorf("Auto Storm settings are required")
	}
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return AutoStormTroopCapPreview{}, fmt.Errorf("decode Auto Storm settings: %w", err)
	}
	normalizeAutoStormSettings(&settings)
	if settings.Version != 1 {
		return AutoStormTroopCapPreview{}, fmt.Errorf("unsupported Auto Storm settings version %d", settings.Version)
	}
	return autoStormTroopCapPreview(Snapshot{
		State: state, Configuration: configuration, GameData: gameData, Now: now,
	}, settings)
}

func autoStormTroopCapPreview(snapshot Snapshot, settings autoStormSettings) (AutoStormTroopCapPreview, error) {
	historyCount, measuredAttacks, troopsSent, averageHourlyTroops, bufferedTroops :=
		autoStormAttackDemand(snapshot.State, snapshot.Now)
	perAttackTroops, detail, err := autoStormMaximumConfiguredTroops(snapshot, settings)
	if err != nil {
		return AutoStormTroopCapPreview{}, err
	}
	result := AutoStormTroopCapPreview{
		Available:                perAttackTroops > 0,
		TroopsPerAttack:          perAttackTroops,
		MinimumTroops:            settings.TroopImport.MinimumTroops,
		HistoryHours:             autoStormTroopHistoryHours,
		AttacksInHistory:         historyCount,
		MeasuredAttacksInHistory: measuredAttacks,
		TroopsSentInHistory:      troopsSent,
		AverageTroopsPerHour:     averageHourlyTroops,
		BufferedTroops:           bufferedTroops,
		Detail:                   detail,
	}
	if perAttackTroops <= 0 {
		return result, nil
	}
	result.MaximumTroops = max(
		autoStormSaturatingAdd(perAttackTroops, settings.TroopImport.MinimumTroops),
		bufferedTroops,
	)
	return result, nil
}

func autoStormMaximumConfiguredTroops(snapshot Snapshot, settings autoStormSettings) (int64, string, error) {
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return 0, "", err
	}
	type configuredPreset struct {
		label                      string
		enabled                    bool
		presetID                   string
		defense                    []autoStormDefenseUnit
		includePresetSupportTroops bool
	}
	configured := []configuredPreset{
		{
			label: "fort", enabled: settings.Forts.Enabled, presetID: settings.Forts.PresetID,
			includePresetSupportTroops: true,
		},
		{
			label: "island", enabled: settings.Islands.Enabled, presetID: settings.Islands.PresetID,
			defense: settings.Islands.DefenseUnits,
		},
	}
	maximum := int64(0)
	issues := make([]string, 0, len(configured))
	enabled := 0
	for _, candidate := range configured {
		if !candidate.enabled {
			continue
		}
		enabled++
		preset, found := AttackPresets.Find(document, candidate.presetID)
		if !found {
			issues = append(issues, fmt.Sprintf("Choose a valid %s attack preset", candidate.label))
			continue
		}
		required, valid := autoStormPresetRequirements(
			preset, candidate.defense, candidate.includePresetSupportTroops,
		)
		if !valid {
			issues = append(issues, fmt.Sprintf("The %s attack preset has no valid units", candidate.label))
			continue
		}
		total, err := autoStormRequiredTroopTotal(snapshot.GameData, required)
		if err != nil {
			return 0, "", err
		}
		if total <= 0 {
			issues = append(issues, fmt.Sprintf("The %s attack preset has no transferable troops", candidate.label))
			continue
		}
		maximum = max(maximum, total)
	}
	if enabled == 0 {
		return 0, "Enable Storm forts or resource islands to calculate the cap", nil
	}
	return maximum, strings.Join(issues, " · "), nil
}

func autoStormRequiredTroopTotal(gameData *GameData.Store, required map[State.UnitID]int64) (int64, error) {
	total := int64(0)
	for _, unitID := range sortedAutoStormUnitIDs(required) {
		isTool, found := autoStormUnitIsTool(gameData, unitID)
		if !found {
			return 0, fmt.Errorf("official unit definition %d is unavailable", unitID)
		}
		if isTool {
			continue
		}
		total = autoStormSaturatingAdd(total, required[unitID])
	}
	return total, nil
}
