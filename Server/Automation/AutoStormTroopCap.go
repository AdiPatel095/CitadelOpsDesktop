package Automation

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

const autoStormTroopCapBaseline int64 = 5_000
const autoStormLegacyTroopDemandMultiplier int64 = 2

const (
	autoStormTroopCapBasisBaseline  = "baseline"
	autoStormTroopCapBasisResetRate = "reset_rate"
	autoStormTroopCapBasisReserve   = "reserve"
)

type AutoStormTroopCapPreview struct {
	Available             bool       `json:"available"`
	MaximumTroops         int64      `json:"maximumTroops"`
	TroopsPerAttack       int64      `json:"troopsPerAttack"`
	MinimumTroops         int64      `json:"minimumTroops"`
	BaselineTroops        int64      `json:"baselineTroops"`
	EnabledPresetCount    int        `json:"enabledPresetCount"`
	AveragePresetTroops   float64    `json:"averagePresetTroops"`
	ResetSessionAvailable bool       `json:"resetSessionAvailable"`
	ResetSessionStartedAt *time.Time `json:"resetSessionStartedAt,omitempty"`
	AttacksSinceReset     int64      `json:"attacksSinceReset"`
	AverageAttacksPerHour float64    `json:"averageAttacksPerHour"`
	RateBasedTroops       int64      `json:"rateBasedTroops"`
	CapBasis              string     `json:"capBasis"`
	// Deprecated rolling-history fields remain during the client transition.
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
	telemetry AttackLaunchCountsProvider,
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
		State: state, Configuration: configuration, GameData: gameData, Telemetry: telemetry, Now: now,
	}, settings)
}

func autoStormTroopCapPreview(snapshot Snapshot, settings autoStormSettings) (AutoStormTroopCapPreview, error) {
	if snapshot.Now.IsZero() {
		snapshot.Now = time.Now().UTC()
	}
	historyCount, measuredAttacks, troopsSent, averageHourlyTroops, bufferedTroops :=
		autoStormLegacyAttackDemand(snapshot.State, snapshot.Now)
	configured, detail, err := autoStormConfiguredTroops(snapshot, settings)
	if err != nil {
		return AutoStormTroopCapPreview{}, err
	}
	result := AutoStormTroopCapPreview{
		MaximumTroops:            autoStormTroopCapBaseline,
		TroopsPerAttack:          configured.maximum,
		MinimumTroops:            settings.TroopImport.MinimumTroops,
		BaselineTroops:           autoStormTroopCapBaseline,
		EnabledPresetCount:       configured.count,
		CapBasis:                 autoStormTroopCapBasisBaseline,
		HistoryHours:             autoStormTroopHistoryHours,
		AttacksInHistory:         historyCount,
		MeasuredAttacksInHistory: measuredAttacks,
		TroopsSentInHistory:      troopsSent,
		AverageTroopsPerHour:     averageHourlyTroops,
		BufferedTroops:           bufferedTroops,
		Detail:                   detail,
	}
	resetStartedAt := snapshot.State.DailyAttacks.SessionStartedAt.UTC()
	if !resetStartedAt.IsZero() {
		result.ResetSessionStartedAt = &resetStartedAt
	}
	if configured.count <= 0 {
		return result, nil
	}
	result.Available = true
	result.AveragePresetTroops = float64(configured.total) / float64(configured.count)
	feasibilityFloor := autoStormSaturatingAdd(configured.maximum, settings.TroopImport.MinimumTroops)
	if feasibilityFloor > result.MaximumTroops {
		result.MaximumTroops = feasibilityFloor
		result.CapBasis = autoStormTroopCapBasisReserve
	}
	if snapshot.Telemetry == nil {
		result.Detail = "Confirmed attack telemetry is unavailable"
		return result, nil
	}
	counts, resetAvailable := snapshot.Telemetry.AttackLaunchCountsSince(resetStartedAt, snapshot.Now)
	result.ResetSessionAvailable = resetAvailable
	if !resetAvailable {
		switch {
		case resetStartedAt.IsZero():
			result.Detail = "Waiting for the authoritative daily attack reset boundary"
		case resetStartedAt.After(snapshot.Now):
			result.Detail = "The authoritative daily attack reset boundary is in the future"
		default:
			result.Detail = "Confirmed Auto Storm attacks are unavailable for the current reset session"
		}
		return result, nil
	}
	result.AttacksSinceReset = int64(max(0, counts[Telemetry.ChannelAutoStorm]))
	result.AverageAttacksPerHour = float64(result.AttacksSinceReset) / float64(autoStormTroopHistoryHours)
	averageTroopsPerHour := result.AverageAttacksPerHour * result.AveragePresetTroops
	rateBasedTroops, err := autoStormCeilTroops(averageTroopsPerHour)
	if err != nil {
		return AutoStormTroopCapPreview{}, err
	}
	result.RateBasedTroops = rateBasedTroops
	if rateBasedTroops > result.MaximumTroops {
		result.MaximumTroops = rateBasedTroops
		result.CapBasis = autoStormTroopCapBasisResetRate
	}
	rateDetail := fmt.Sprintf(
		"%d confirmed Auto Storm attacks since reset / %d = %.2f attacks per hour; %.0f average troops across %d enabled presets; %d troop baseline; %d attack-plus-reserve floor",
		result.AttacksSinceReset,
		autoStormTroopHistoryHours,
		result.AverageAttacksPerHour,
		result.AveragePresetTroops,
		result.EnabledPresetCount,
		result.BaselineTroops,
		feasibilityFloor,
	)
	result.Detail = strings.Trim(strings.Join([]string{detail, rateDetail}, " · "), " ·")
	return result, nil
}

func autoStormLegacyAttackDemand(
	state State.GameState,
	now time.Time,
) (int64, int64, int64, float64, int64) {
	cutoff := now.Add(-autoStormTroopHistoryHours * time.Hour)
	troopsByMovement := map[State.MovementID]int64{}
	for _, records := range [][]State.AttackFeatureLaunch{
		state.AttackAnalytics.RecentAutoStormLaunches,
		state.AttackAnalytics.PendingAttacks,
	} {
		for _, record := range records {
			if record.MovementID <= 0 || record.FeatureID != State.AttackFeatureAutoStorm ||
				record.KingdomID != autoStormKingdomID || record.LaunchedAt.Before(cutoff) ||
				record.LaunchedAt.After(now) {
				continue
			}
			troopsByMovement[record.MovementID] = max(
				troopsByMovement[record.MovementID],
				max(int64(0), record.TroopCount),
			)
		}
	}
	troopsSent := int64(0)
	measuredAttacks := int64(0)
	for _, troopCount := range troopsByMovement {
		if troopCount <= 0 {
			continue
		}
		measuredAttacks++
		troopsSent = autoStormSaturatingAdd(troopsSent, troopCount)
	}
	averageHourly := float64(troopsSent) / float64(autoStormTroopHistoryHours)
	bufferedTroops := int64(math.Ceil(averageHourly * float64(autoStormLegacyTroopDemandMultiplier)))
	return int64(len(troopsByMovement)), measuredAttacks, troopsSent, averageHourly, bufferedTroops
}

type autoStormConfiguredTroopDemand struct {
	maximum int64
	total   int64
	count   int
}

func autoStormConfiguredTroops(snapshot Snapshot, settings autoStormSettings) (autoStormConfiguredTroopDemand, string, error) {
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return autoStormConfiguredTroopDemand{}, "", err
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
	result := autoStormConfiguredTroopDemand{}
	enabled := 0
	issues := make([]string, 0, len(configured))
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
			return autoStormConfiguredTroopDemand{}, "", err
		}
		if total <= 0 {
			issues = append(issues, fmt.Sprintf("The %s attack preset has no transferable troops", candidate.label))
			continue
		}
		result.count++
		result.maximum = max(result.maximum, total)
		if result.total > math.MaxInt64-total {
			return autoStormConfiguredTroopDemand{}, "", fmt.Errorf("enabled Storm preset troop total exceeds the supported range")
		}
		result.total += total
	}
	if enabled == 0 {
		return result, "Enable Storm forts or resource islands to calculate the cap", nil
	}
	return result, strings.Join(issues, " · "), nil
}

func autoStormCeilTroops(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt64) {
		return 0, fmt.Errorf("Auto Storm troop cap exceeds the supported range")
	}
	return int64(math.Ceil(value)), nil
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
