package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	allianceRosterRefreshInterval = 5 * time.Minute
	allianceRefreshAwaitInterval  = 10 * time.Second
	autoBirdNextMetricKey         = "nextBirdUnixMs"
	autoBirdNextCastleMetricKey   = "nextBirdCastleId"
	autoBirdCastleReturnMetricKey = "birdReturnUnixMs."
)

type reserveSetting struct {
	ID     State.UnitID `json:"id"`
	Amount int64        `json:"amount"`
}

type autoBirdSettings struct {
	Settings   map[string][]reserveSetting `json:"settings"`
	MinDelay   int                         `json:"minDelay"`
	MaxDelay   int                         `json:"maxDelay"`
	MinSend    int64                       `json:"minSend"`
	MinRPTDays int                         `json:"minRPTDays"`
}

type autoBirdPreset struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	Settings   map[string][]reserveSetting `json:"settings"`
	MinDelay   int                         `json:"minDelay"`
	MaxDelay   int                         `json:"maxDelay"`
	MinSend    int64                       `json:"minSend"`
	MinRPTDays *int                        `json:"minRPTDays"`
}

type autoBirdConfiguration struct {
	Version        int              `json:"version"`
	ActivePresetID string           `json:"activePresetId"`
	IgnoreSettings autoBirdSettings `json:"ignoreSettings"`
	Presets        struct {
		Presets []autoBirdPreset `json:"presets"`
	} `json:"presets"`

	ResolvedPresetID string    `json:"-"`
	PresetValidUntil time.Time `json:"-"`
}

type autoStationConfiguration struct {
	LeadTimeSec      int                         `json:"leadTimeSec"`
	RecallWhenClear  bool                        `json:"recallWhenClear"`
	MinRPTDays       int                         `json:"minRPTDays"`
	OpenGateFallback bool                        `json:"openGateFallback"`
	Settings         map[string][]reserveSetting `json:"settings"`
}

type AutoBirdPolicy struct{}

func NewAutoBirdPolicy() *AutoBirdPolicy { return &AutoBirdPolicy{} }

func (*AutoBirdPolicy) ID() string { return "autoBird" }

func (*AutoBirdPolicy) EnabledKey() string { return "auto_bird" }

func (*AutoBirdPolicy) WakeDomains() []string {
	return []string{"alliance", "movement-snapshot", "movements", "player-protection", "stationing", "units"}
}

func (*AutoBirdPolicy) WakeSections() []string {
	return []string{"automation.autoBird", "automation.autoFortress"}
}

func (*AutoBirdPolicy) WakeEnabledControls() []string { return []string{"auto_fortress"} }

func (*AutoBirdPolicy) Evaluate(_ context.Context, snapshot Snapshot) (decision Decision, err error) {
	releaseWaitingCastle := false
	if snapshot.PolicyConfigurationChanged {
		for _, operation := range snapshot.State.Stationing {
			if operation.Purpose == "autoBird" && operation.Phase == State.StationingPhaseWaiting &&
				operation.NextAttemptAt != nil && operation.NextAttemptAt.After(snapshot.Now) {
				releaseWaitingCastle = true
				break
			}
		}
	}
	if refresh, required := stationProtectionRefreshDecision(snapshot); required && !releaseWaitingCastle {
		return withAutoBirdSchedule(snapshot, refresh, time.Time{}), nil
	}
	defer func() {
		if err == nil {
			decision = capAtPlayerProtectionRefresh(snapshot, decision)
			if decision.Request != nil && strings.HasPrefix(decision.Request.Name, "auto_bird.") {
				var args map[string]json.RawMessage
				if json.Unmarshal(decision.Request.Arguments, &args) == nil {
					var id State.CastleID
					if json.Unmarshal(args["sourceCastleId"], &id) == nil && id > 0 {
						args["controlRevision"], _ = json.Marshal(snapshot.State.AutoBirdControl(id).UpdatedAt)
						decision.Request.Arguments, _ = json.Marshal(args)
					}
				}
			}
		}
	}()
	settings := defaultAutoBirdConfiguration()
	decodeSection(snapshot.Configuration, "automation.autoBird", &settings)
	schedule := resolveWeeklySchedule(snapshot.Configuration, "autoBird", snapshot.Now)
	if !schedule.Allowed {
		next := schedule.Next
		if next.IsZero() {
			next = snapshot.Now.Add(allianceRosterRefreshInterval)
		}
		return withAutoBirdSchedule(snapshot, Decision{
			Status: "scheduled", Detail: "Outside the configured weekly schedule", DetailDescriptor: Localization.New("server.automation.outside_the_configured_weekly.e8fa033d", "Outside the configured weekly schedule", nil), NextCheckAt: next,
		}, time.Time{}), nil
	}
	selectedPresetID := strings.TrimSpace(settings.ActivePresetID)
	if schedule.SlotOptionsEnabled {
		var valid bool
		selectedPresetID, valid = autoBirdSchedulePresetID(schedule.Options)
		if !valid {
			return withAutoBirdSchedule(snapshot, Decision{
				Status: "waiting", Detail: "The active schedule period has no valid Auto Bird preset", DetailDescriptor: Localization.New("server.automation.the_active_schedule_period.2bb8233b", "The active schedule period has no valid Auto Bird preset", nil),
				NextCheckAt: schedule.ValidUntil,
			}, time.Time{}), nil
		}
	}
	if selectedPresetID != "" {
		preset, valid := findAutoBirdPreset(settings.Presets.Presets, selectedPresetID)
		if !valid {
			return withAutoBirdSchedule(snapshot, Decision{
				Status: "waiting", Detail: "The selected Auto Bird preset no longer exists or is duplicated", DetailDescriptor: Localization.New("server.automation.the_selected_auto_bird.8c450082", "The selected Auto Bird preset no longer exists or is duplicated", nil),
				NextCheckAt: schedule.ValidUntil,
			}, time.Time{}), nil
		}
		minimumRPTDays := 3
		if preset.MinRPTDays != nil {
			minimumRPTDays = *preset.MinRPTDays
		}
		settings.IgnoreSettings = autoBirdSettings{
			Settings: preset.Settings, MinDelay: preset.MinDelay, MaxDelay: preset.MaxDelay,
			MinSend: preset.MinSend, MinRPTDays: minimumRPTDays,
		}
	}
	settings.ResolvedPresetID = selectedPresetID
	settings.PresetValidUntil = schedule.ValidUntil
	minimumDelay := clampInt(settings.IgnoreSettings.MinDelay, 1, 12)
	maximumDelay := clampInt(settings.IgnoreSettings.MaxDelay, minimumDelay, 12)
	if snapshot.State.Player.ProtectionMode.PreparingOrActive(snapshot.Now) {
		delayHours := deterministicDelayHours(minimumDelay, maximumDelay, snapshot.State.Revision, 0)
		nextCheck := snapshot.Now.Add(time.Duration(delayHours) * time.Hour)
		protectionEndsAt := snapshot.State.Player.ProtectionMode.Until().Add(time.Second)
		if protectionEndsAt.Before(nextCheck) {
			nextCheck = protectionEndsAt
		}
		return withAutoBirdSchedule(snapshot, Decision{
			Status: "protected", Detail: "Protection Mode is preparing or active; Auto Bird stationing is disabled", DetailDescriptor: Localization.New("server.automation.protection_mode_is_preparing.89ac07e7", "Protection Mode is preparing or active; Auto Bird stationing is disabled", nil),
			NextCheckAt: nextCheck,
		}, protectionEndsAt), nil
	}
	if snapshot.State.Alliance.ID <= 0 {
		return withAutoBirdSchedule(snapshot, Decision{
			Status: "waiting", Detail: "The current alliance is not known, so castle bird cycles cannot run AIN", DetailDescriptor: Localization.New("server.automation.the_current_alliance_is.628c9e4f", "The current alliance is not known, so castle bird cycles cannot run AIN", nil),
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, time.Time{}), nil
	}
	threats, _, _, _ := incomingThreats(snapshot.State, snapshot.Now)
	castleIDs := sortedCastleIDs(snapshot.State.Castles)
	if len(castleIDs) == 0 {
		return withAutoBirdSchedule(snapshot, Decision{
			Status: "waiting", Detail: "No owned castles are available for Auto Bird", DetailDescriptor: Localization.New("server.automation.no_owned_castles_are.25db1f63", "No owned castles are available for Auto Bird", nil),
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, time.Time{}), nil
	}
	allianceHoldings := protectedHoldings(snapshot.State.Alliance, 0)
	var nextCheck time.Time
	eligibleCastles := make([]State.CastleID, 0, len(castleIDs))
	for _, castleID := range castleIDs {
		if snapshot.State.AutoBirdPaused(castleID, snapshot.Now) {
			if until := snapshot.State.AutoBirdControl(castleID).PausedUntil; until != nil {
				nextCheck = earlierTime(nextCheck, *until)
			}
			continue
		}
		eligibleCastles = append(eligibleCastles, castleID)
	}
	castleIDs = eligibleCastles

	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		operation, exists := snapshot.State.Stationing[autoBirdTrackingID(castle.ID)]
		if !exists || operation.Purpose != "autoBird" ||
			operation.Phase != State.StationingPhaseDispatchReady {
			continue
		}
		if _, threatened := threats[castle.ID]; threatened ||
			(!snapshot.State.AutoBirdControl(castle.ID).RescanRequested && hasActiveAllianceStationMovement(snapshot.State, castle.ID, allianceHoldings, snapshot.Now)) {
			continue
		}
		target, targetAvailable := SelectAutoBirdHolding(
			snapshot.State.Alliance, castle, settings.IgnoreSettings.MinRPTDays,
		)
		targetStale := snapshot.PolicyConfigurationChanged ||
			operation.PresetID != settings.ResolvedPresetID ||
			operation.AllianceObservedAt.IsZero() ||
			snapshot.Now.Sub(operation.AllianceObservedAt) > allianceRosterRefreshInterval ||
			!targetAvailable || target.CastleID != operation.TargetCastleID
		if targetStale {
			return withAutoBirdSchedule(snapshot, autoBirdDiscoverDecision(
				castle, settings, snapshot.Now, "Refresh changed or expired Auto Bird target", castleDecisionDescriptor("bird_refresh_target", castle, nil),
			), time.Time{}), nil
		}
		if operation.UnitsObservedAt.IsZero() ||
			snapshot.Now.Sub(operation.UnitsObservedAt) > 30*time.Second {
			return withAutoBirdSchedule(snapshot, autoBirdPrepareDecision(
				castle, settings, snapshot.Now, "Refresh expired Auto Bird troop inventory", castleDecisionDescriptor("bird_refresh_expired", castle, nil),
			), time.Time{}), nil
		}
		arguments := autoBirdCycleArguments(castle.ID, settings)
		return withAutoBirdSchedule(snapshot, Decision{
			Status: "ready",
			Detail: fmt.Sprintf(
				"Dispatch %d freshly inventoried troops from %s to bird target %d",
				sumStationOperationUnits(operation.Units), castleName(castle), operation.TargetCastleID,
			), DetailDescriptor: castleDecisionDescriptor("bird_dispatch", castle, Localization.Params{"troops": sumStationOperationUnits(operation.Units), "target": fmt.Sprint(operation.TargetCastleID)}),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "auto_bird.dispatch", Arguments: arguments},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, time.Time{}), nil
	}

	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		operation, exists := snapshot.State.Stationing[autoBirdTrackingID(castle.ID)]
		if !exists || operation.Purpose != "autoBird" ||
			operation.Phase != State.StationingPhaseTargetReady {
			continue
		}
		if _, threatened := threats[castle.ID]; threatened ||
			(!snapshot.State.AutoBirdControl(castle.ID).RescanRequested && hasActiveAllianceStationMovement(snapshot.State, castle.ID, allianceHoldings, snapshot.Now)) {
			continue
		}
		target, targetAvailable := SelectAutoBirdHolding(
			snapshot.State.Alliance, castle, settings.IgnoreSettings.MinRPTDays,
		)
		if snapshot.PolicyConfigurationChanged ||
			operation.PresetID != settings.ResolvedPresetID ||
			operation.AllianceObservedAt.IsZero() ||
			snapshot.Now.Sub(operation.AllianceObservedAt) > allianceRosterRefreshInterval ||
			!targetAvailable || target.CastleID != operation.TargetCastleID {
			return withAutoBirdSchedule(snapshot, autoBirdDiscoverDecision(
				castle, settings, snapshot.Now, "Refresh changed or expired Auto Bird target", castleDecisionDescriptor("bird_refresh_target", castle, nil),
			), time.Time{}), nil
		}
		return withAutoBirdSchedule(snapshot, autoBirdPrepareDecision(
			castle, settings, snapshot.Now, "Refresh the latest troop inventory", castleDecisionDescriptor("bird_refresh_inventory", castle, nil),
		), time.Time{}), nil
	}

	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		operation, exists := snapshot.State.Stationing[autoBirdTrackingID(castle.ID)]
		if !exists || operation.Purpose != "autoBird" || operation.Phase != State.StationingPhaseAway ||
			operation.ExpectedReturnAt != nil {
			continue
		}
		if operation.NextAttemptAt != nil && operation.NextAttemptAt.After(snapshot.Now) {
			nextCheck = earlierTime(nextCheck, *operation.NextAttemptAt)
			continue
		}
		arguments := autoBirdCycleArguments(castle.ID, settings)
		return withAutoBirdSchedule(snapshot, Decision{
			Status: "reconciling",
			Detail: fmt.Sprintf("Refresh movement timing for %s without relaunching it", castleName(castle)), DetailDescriptor: Localization.New("server.automation.refresh_movement_timing_for.385cd686", "Refresh movement timing for {p0} without relaunching it", Localization.Params{"p0": fmt.Sprintf("%s", castleName(castle))}),
			NextCheckAt:         snapshot.Now.Add(autoBirdMovementRetryInterval),
			Request:             &Intent.Request{Name: "auto_bird.reconcile", Arguments: arguments},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, time.Time{}), nil
	}

	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		if _, threatened := threats[castle.ID]; threatened {
			nextCheck = earlierTime(nextCheck, snapshot.Now.Add(30*time.Second))
			continue
		}
		operation, tracked := snapshot.State.Stationing[autoBirdTrackingID(castle.ID)]
		if (!snapshot.State.AutoBirdControl(castle.ID).RescanRequested && hasActiveAllianceStationMovement(snapshot.State, castle.ID, allianceHoldings, snapshot.Now)) ||
			tracked && operation.Purpose == "autoBird" && operation.Phase == "" &&
				operation.ActiveInState(snapshot.State, snapshot.Now) {
			continue
		}
		if tracked && operation.Purpose == "autoBird" {
			switch operation.Phase {
			case State.StationingPhaseAway:
				if operation.ExpectedReturnAt != nil && operation.ExpectedReturnAt.After(snapshot.Now) {
					nextCheck = earlierTime(nextCheck, *operation.ExpectedReturnAt)
					continue
				}
			case State.StationingPhaseWaiting:
				if snapshot.PolicyConfigurationChanged {
					return withAutoBirdSchedule(snapshot, autoBirdDiscoverDecision(
						castle, settings, snapshot.Now, "Restart after relevant automation settings changed", castleDecisionDescriptor("bird_restart_settings", castle, nil),
					), time.Time{}), nil
				}
				if operation.PresetID != settings.ResolvedPresetID {
					return withAutoBirdSchedule(snapshot, autoBirdDiscoverDecision(
						castle, settings, snapshot.Now, "Restart after the Auto Bird preset changed", castleDecisionDescriptor("bird_restart_preset", castle, nil),
					), time.Time{}), nil
				}
				// AIN and JAA observations can advance while another castle runs its
				// independent cycle. The explicit per-castle retry remains authoritative
				// so those shared timestamps cannot restart this castle in a tight loop.
				if operation.NextAttemptAt != nil && operation.NextAttemptAt.After(snapshot.Now) {
					nextCheck = earlierTime(nextCheck, *operation.NextAttemptAt)
					continue
				}
			case State.StationingPhaseDispatchReady:
				continue
			case State.StationingPhaseTargetReady:
				continue
			}
		}
		if tracked && operation.Purpose != "autoBird" && operation.ActiveInState(snapshot.State, snapshot.Now) {
			continue
		}
		return withAutoBirdSchedule(snapshot, autoBirdDiscoverDecision(
			castle, settings, snapshot.Now, "Run this castle's independent AIN target discovery", castleDecisionDescriptor("bird_discover", castle, nil),
		), time.Time{}), nil
	}
	if nextCheck.IsZero() {
		if expectedAt, _ := earliestAutoBirdReturn(expectedAutoBirdReturns(snapshot.State, snapshot.Now)); expectedAt.After(snapshot.Now) {
			nextCheck = expectedAt
		} else {
			nextCheck = snapshot.Now.Add(allianceRosterRefreshInterval)
		}
	}
	return withAutoBirdSchedule(snapshot, Decision{
		Status: "idle", Detail: "Each castle is independently waiting for troops, a target, or its bird return", DetailDescriptor: Localization.New("server.automation.each_castle_is_independently.0d0d1581", "Each castle is independently waiting for troops, a target, or its bird return", nil),
		NextCheckAt: nextCheck,
	}, time.Time{}), nil
}

func defaultAutoBirdConfiguration() autoBirdConfiguration {
	settings := autoBirdConfiguration{}
	settings.IgnoreSettings.Settings = map[string][]reserveSetting{}
	settings.IgnoreSettings.MinDelay = 6
	settings.IgnoreSettings.MaxDelay = 12
	settings.IgnoreSettings.MinRPTDays = 3
	return settings
}

func autoBirdSchedulePresetID(options map[string]any) (string, bool) {
	raw, exists := options["presetId"]
	if !exists {
		return "", false
	}
	presetID, valid := raw.(string)
	presetID = strings.TrimSpace(presetID)
	return presetID, valid && presetID != ""
}

func findAutoBirdPreset(presets []autoBirdPreset, presetID string) (autoBirdPreset, bool) {
	presetID = strings.TrimSpace(presetID)
	var selected autoBirdPreset
	matches := 0
	for _, preset := range presets {
		if strings.TrimSpace(preset.ID) != presetID {
			continue
		}
		selected = preset
		matches++
	}
	return selected, matches == 1
}

const autoBirdMovementRetryInterval = time.Minute

func autoBirdDiscoverDecision(
	castle State.CastleState,
	settings autoBirdConfiguration,
	now time.Time,
	reason string,
	descriptors ...*Localization.Message,
) Decision {
	return Decision{
		Status: "discovering",
		Detail: fmt.Sprintf("%s for %s", reason, castleName(castle)), DetailDescriptor: Localization.First(descriptors),
		NextCheckAt:         now.Add(30 * time.Second),
		Request:             &Intent.Request{Name: "auto_bird.discover", Arguments: autoBirdCycleArguments(castle.ID, settings)},
		ReevaluateOnSuccess: true,
		ReevaluateOnStale:   true,
	}
}

func autoBirdPrepareDecision(
	castle State.CastleState,
	settings autoBirdConfiguration,
	now time.Time,
	reason string,
	descriptors ...*Localization.Message,
) Decision {
	return Decision{
		Status: "preparing",
		Detail: fmt.Sprintf("%s for %s", reason, castleName(castle)), DetailDescriptor: Localization.First(descriptors),
		NextCheckAt:         now.Add(30 * time.Second),
		Request:             &Intent.Request{Name: "auto_bird.prepare", Arguments: autoBirdCycleArguments(castle.ID, settings)},
		ReevaluateOnSuccess: true,
		ReevaluateOnStale:   true,
	}
}

func autoBirdCycleArguments(castleID State.CastleID, settings autoBirdConfiguration) json.RawMessage {
	reserves := settings.IgnoreSettings.Settings[strconv.FormatInt(int64(castleID), 10)]
	values := map[string]any{
		"sourceCastleId":    castleID,
		"trackingId":        autoBirdTrackingID(castleID),
		"presetId":          settings.ResolvedPresetID,
		"minimumRPTDays":    settings.IgnoreSettings.MinRPTDays,
		"minimumDelayHours": clampInt(settings.IgnoreSettings.MinDelay, 1, 12),
		"maximumDelayHours": clampInt(
			settings.IgnoreSettings.MaxDelay,
			clampInt(settings.IgnoreSettings.MinDelay, 1, 12),
			12,
		),
		"minimumSend": max(int64(0), settings.IgnoreSettings.MinSend),
		"reserves":    stationReserveUnits(reserves),
	}
	if !settings.PresetValidUntil.IsZero() {
		values["presetValidUntil"] = settings.PresetValidUntil.UTC()
	}
	arguments, _ := json.Marshal(values)
	return arguments
}

func autoBirdTrackingID(castleID State.CastleID) string {
	return "autoBird:" + strconv.FormatInt(int64(castleID), 10)
}

func sumStationOperationUnits(units map[State.UnitID]int64) int64 {
	var total int64
	for _, amount := range units {
		total += max(int64(0), amount)
	}
	return total
}

func earlierTime(current, candidate time.Time) time.Time {
	if candidate.IsZero() || !current.IsZero() && !candidate.Before(current) {
		return current
	}
	return candidate
}

type AutoStationPolicy struct{}

func NewAutoStationPolicy() *AutoStationPolicy { return &AutoStationPolicy{} }

func (*AutoStationPolicy) ID() string { return "autoStation" }

func (*AutoStationPolicy) EnabledKey() string { return "auto_station" }

func (*AutoStationPolicy) WakeDomains() []string {
	return []string{"alliance", "movement-snapshot", "movements", "player-protection", "stationing", "units"}
}

func (*AutoStationPolicy) WakeSections() []string { return []string{"automation.autoStation"} }

func (*AutoStationPolicy) Evaluate(_ context.Context, snapshot Snapshot) (decision Decision, err error) {
	if refresh, required := stationProtectionRefreshDecision(snapshot); required {
		return refresh, nil
	}
	defer func() {
		if err == nil {
			decision = capAtPlayerProtectionRefresh(snapshot, decision)
		}
	}()
	settings := autoStationConfiguration{
		LeadTimeSec: 60, RecallWhenClear: true, MinRPTDays: 3, Settings: map[string][]reserveSetting{},
	}
	decodeSection(snapshot.Configuration, "automation.autoStation", &settings)
	settings.LeadTimeSec = clampInt(settings.LeadTimeSec, 60, 3600)
	threats, threatCount, earliestImpact, _ := incomingThreats(snapshot.State, snapshot.Now)
	metrics := stationMetrics(threatCount, earliestImpact)
	protectionMode := snapshot.State.Player.ProtectionMode.PreparingOrActive(snapshot.Now)
	if threatCount > 0 {
		if protectionMode {
			return protectionModeOpenGateDecision(snapshot, settings, threats, threatCount, earliestImpact, metrics), nil
		}
		needsRoster := false
		for id := range threats {
			active, _, _, _ := trackedStationRemainder(snapshot, snapshot.State.Castles[id], nil)
			if !active {
				needsRoster = true
			}
		}
		if decision, refresh := allianceRosterRefreshDecision(snapshot, "Incoming attack detected; refreshing alliance roster before evacuation", Localization.New("server.automation.incoming_attack_detected_refreshing.143eccd4", "Incoming attack detected; refreshing alliance roster before evacuation", nil)); refresh && needsRoster {
			decision.Status = "threat"
			decision.Metrics = metrics
			if decision.Request != nil {
				for _, id := range sortedThreatCastleIDs(threats) {
					castle := snapshot.State.Castles[id]
					if castle.KingdomID == 0 && threats[id].Earliest.Sub(snapshot.Now) <= time.Duration(settings.LeadTimeSec)*time.Second {
						args, _ := json.Marshal(map[string]any{"castleId": id, "requireIncomingAttack": true, "autoStation": true})
						decision.FailureFallback = &Intent.Request{Name: "defense.open_gate", Arguments: args}
						break
					}
				}
			}
			return decision, nil
		}
		protectedTargets := protectedHoldings(snapshot.State.Alliance, settings.MinRPTDays)
		remaining := earliestImpact.Sub(snapshot.Now)
		if remaining > time.Duration(settings.LeadTimeSec)*time.Second {
			next := earliestImpact.Add(-time.Duration(settings.LeadTimeSec) * time.Second)
			if next.Before(snapshot.Now.Add(2 * time.Second)) {
				next = snapshot.Now.Add(2 * time.Second)
			}
			return Decision{
				Status: "threat", Detail: fmt.Sprintf("%d incoming attack(s); evacuation window opens in %s", threatCount, roundedDuration(remaining-time.Duration(settings.LeadTimeSec)*time.Second)), DetailDescriptor: Localization.New("server.automation.station_evacuation_window", "{attacks, number} incoming attack(s); evacuation window opens in {seconds, number} seconds", Localization.Params{"attacks": threatCount, "seconds": int64(roundedDuration(remaining-time.Duration(settings.LeadTimeSec)*time.Second) / time.Second)}),
				NextCheckAt: next, Metrics: metrics,
			}, nil
		}
		fallbackThreats := map[State.CastleID]threatWindow{}
		unresolved := false
		var nextWindow time.Time
		for _, castleID := range sortedThreatCastleIDs(threats) {
			castle := snapshot.State.Castles[castleID]
			window := threats[castleID]
			if window.Earliest.Sub(snapshot.Now) > time.Duration(settings.LeadTimeSec)*time.Second {
				unresolved = true
				nextWindow = minTime(nextWindow, window.Earliest.Add(-time.Duration(settings.LeadTimeSec)*time.Second))
				continue
			}
			reserves := settings.Settings[strconv.FormatInt(int64(castle.ID), 10)]
			if active, fresh, remaining, known := trackedStationRemainder(snapshot, castle, reserves); active {
				if !fresh {
					return refreshTrackedStationInventory(snapshot, castle), nil
				}
				if known && remaining == 0 {
					continue
				}
				unresolved = true
				if settings.OpenGateFallback && known {
					fallbackThreats[castleID] = window
				}
				continue
			}
			target, found := nearestHolding(protectedTargets, castle)
			units := stationableUnits(snapshot, castle, settings.Settings[strconv.FormatInt(int64(castle.ID), 10)])
			if !found || len(units) == 0 {
				unresolved = true
				if settings.OpenGateFallback {
					fallbackThreats[castleID] = window
				}
				continue
			}
			trackingID := "autoStation:" + strconv.FormatInt(int64(castle.ID), 10)
			arguments, _ := json.Marshal(map[string]any{
				"sourceCastleId": castle.ID, "targetCastleId": target.CastleID, "delayHours": 1,
				"purpose": "autoStation", "trackingId": trackingID, "safeAfterUnix": window.Latest.Unix(), "units": units, "minimumRPTDays": settings.MinRPTDays,
			})
			decision := Decision{
				Status:              "evacuating",
				Detail:              fmt.Sprintf("Evacuating %d troops from %s", sumStationUnits(units), castleName(castle)),
				DetailDescriptor:    castleDecisionDescriptor("station_evacuate", castle, Localization.Params{"troops": sumStationUnits(units)}),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Metrics:             metrics,
				Request:             &Intent.Request{Name: "troops.station", Arguments: arguments},
				ReevaluateOnSuccess: true,
			}
			if castle.KingdomID == 0 {
				fallbackArguments, _ := json.Marshal(map[string]any{
					"castleId": castle.ID, "requireIncomingAttack": true, "autoStation": true,
				})
				decision.FailureFallback = &Intent.Request{
					Name: "defense.open_gate", Arguments: fallbackArguments,
				}
				decision.FailureDetail = fmt.Sprintf(
					"Stationing failed; Open Gate fallback completed for %s", castleName(castle),
				)
			}
			return decision, nil
		}
		if len(fallbackThreats) > 0 {
			gate := protectionModeOpenGateDecision(snapshot, settings, fallbackThreats, threatCount, earliestImpact, metrics)
			if gate.Request != nil {
				return gate, nil
			}
			if len(fallbackThreats) == len(threats) {
				return gate, nil
			}
		}
		if unresolved {
			if nextWindow.IsZero() {
				nextWindow = snapshot.Now.Add(10 * time.Second)
			}
			return Decision{Status: "threat", Detail: "Some threatened castles cannot station troops or are outside their evacuation window", NextCheckAt: nextWindow, Metrics: metrics}, nil
		}
		return Decision{
			Status: "protected", Detail: fmt.Sprintf("%d incoming attack(s); eligible troops are already protected", threatCount), DetailDescriptor: Localization.New("server.automation.p_incoming_attack_s.100a15f6", "{p0} incoming attack(s); eligible troops are already protected", Localization.Params{"p0": threatCount}),
			NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		}, nil
	}

	for _, trackingID := range sortedStationingIDs(snapshot.State.Stationing) {
		operation := snapshot.State.Stationing[trackingID]
		if operation.Purpose != "autoStation" {
			continue
		}
		movement, active := trackedStationMovement(snapshot.State, operation)
		if !active || movement.Direction != 0 || !settings.RecallWhenClear {
			continue
		}
		if operation.SafeAfter != nil && snapshot.Now.Before(operation.SafeAfter.Add(5*time.Second)) {
			return Decision{
				Status: "protected", Detail: "Troops remain stationed until the final observed attack has landed", DetailDescriptor: Localization.New("server.automation.troops_remain_stationed_until.6560a1d7", "Troops remain stationed until the final observed attack has landed", nil),
				NextCheckAt: operation.SafeAfter.Add(5 * time.Second), Metrics: metrics,
			}, nil
		}
		if operation.SafeAfter != nil && !snapshot.State.MovementSnapshot.ObservedAt.After(*operation.SafeAfter) {
			return Decision{
				Status: "waiting",
				Detail: "Confirming a fresh movement snapshot before recall", DetailDescriptor: Localization.New("server.automation.confirming_a_fresh_movement.33a67299", "Confirming a fresh movement snapshot before recall", nil),
				NextCheckAt:         snapshot.Now.Add(3 * time.Second),
				Metrics:             metrics,
				Request:             &Intent.Request{Name: "game.refresh_movements", Arguments: json.RawMessage(`{}`)},
				ReevaluateOnSuccess: true,
			}, nil
		}
		arguments, _ := json.Marshal(map[string]any{"movementId": movement.ID})
		return Decision{
			Status: "recalling",
			Detail: fmt.Sprintf("Recalling protected troops from castle %d", operation.SourceCastleID), DetailDescriptor: Localization.New("server.automation.recalling_protected_troops_from.b58ab2f6", "Recalling protected troops from castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", operation.SourceCastleID)}),
			NextCheckAt:         snapshot.Now.Add(5 * time.Second),
			Metrics:             metrics,
			Request:             &Intent.Request{Name: "movement.recall", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, nil
	}
	if protectionMode {
		detail := "Protection Mode is preparing or active; Auto Station will use Open Gates instead of stationing"
		detailLocalizationMessage := Localization.New("server.automation.protection_mode_is_preparing.9107f9a8", "Protection Mode is preparing or active; Auto Station will use Open Gates instead of stationing", nil)
		return Decision{
			Status: "protected", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
			EventDriven: true, Metrics: metrics,
		}, nil
	}
	return Decision{
		Status: "armed", Detail: "Monitoring canonical movement snapshots for incoming attacks", DetailDescriptor: Localization.New("server.automation.monitoring_canonical_movement_snapshots.e588c775", "Monitoring canonical movement snapshots for incoming attacks", nil),
		EventDriven: true, Metrics: metrics,
	}, nil
}

func protectionModeOpenGateDecision(
	snapshot Snapshot,
	settings autoStationConfiguration,
	threats map[State.CastleID]threatWindow,
	threatCount int,
	earliestImpact time.Time,
	metrics map[string]float64,
) Decision {
	leadTime := time.Duration(settings.LeadTimeSec) * time.Second
	remaining := earliestImpact.Sub(snapshot.Now)
	if remaining > leadTime {
		next := earliestImpact.Add(-leadTime)
		if next.Before(snapshot.Now.Add(2 * time.Second)) {
			next = snapshot.Now.Add(2 * time.Second)
		}
		return Decision{
			Status: "threat", Detail: fmt.Sprintf("%d incoming attack(s); Open Gate safety window opens in %s", threatCount, roundedDuration(remaining-leadTime)), DetailDescriptor: Localization.New("server.automation.station_gate_window", "{attacks, number} incoming attack(s); Open Gate safety window opens in {seconds, number} seconds", Localization.Params{"attacks": threatCount, "seconds": int64(roundedDuration(remaining-leadTime) / time.Second)}),
			NextCheckAt: next, Metrics: metrics,
		}
	}

	uncovered := make([]State.CastleID, 0, len(threats))
	for _, castleID := range sortedThreatCastleIDs(threats) {
		castle := snapshot.State.Castles[castleID]
		reserves := settings.Settings[strconv.FormatInt(int64(castle.ID), 10)]
		if snapshot.State.Player.ProtectionMode.PreparingOrActive(snapshot.Now) {
			reserves = nil
		}
		if active, fresh, remaining, known := trackedStationRemainder(snapshot, castle, reserves); active {
			if !fresh {
				return refreshTrackedStationInventory(snapshot, castle)
			}
			if known && remaining == 0 {
				continue
			}
			if !known {
				return Decision{Status: "waiting", Detail: "Waiting for official troop data before checking the evacuation remainder", NextCheckAt: snapshot.Now.Add(10 * time.Second), Metrics: metrics}
			}
		}
		gateUntil := castle.Defense.OpenGateUntil
		if gateUntil != nil && gateUntil.After(threats[castleID].Latest) {
			continue
		}
		uncovered = append(uncovered, castleID)
	}
	if len(uncovered) == 0 {
		return Decision{
			Status: "protected", Detail: fmt.Sprintf("%d incoming attack(s); troops are evacuated or game-reported gates cover the final attack", threatCount),
			NextCheckAt: snapshot.Now.Add(10 * time.Second), Metrics: metrics,
		}
	}

	var nextGateExpiry time.Time
	unsupportedCastle := State.CastleID(0)
	for _, castleID := range uncovered {
		castle := snapshot.State.Castles[castleID]
		if window := threats[castleID]; window.Earliest.Sub(snapshot.Now) > leadTime {
			nextGateExpiry = minTime(nextGateExpiry, window.Earliest.Add(-leadTime))
			continue
		}
		if castle.KingdomID != 0 {
			if unsupportedCastle == 0 {
				unsupportedCastle = castleID
			}
			continue
		}
		if gateUntil := castle.Defense.OpenGateUntil; gateUntil != nil && gateUntil.After(snapshot.Now) {
			nextGateExpiry = minTime(nextGateExpiry, gateUntil.Add(time.Second))
			continue
		}
		arguments, _ := json.Marshal(map[string]any{
			"castleId": castle.ID, "requireIncomingAttack": true, "autoStation": true, "requireProtectionMode": snapshot.State.Player.ProtectionMode.PreparingOrActive(snapshot.Now),
		})
		return Decision{
			Status:              "threat",
			Detail:              fmt.Sprintf("Opening gates at %s because troops cannot be stationed safely", castleName(castle)),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Metrics:             metrics,
			Request:             &Intent.Request{Name: "defense.open_gate", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}
	}
	if unsupportedCastle > 0 {
		return Decision{
			Status: "threat", Detail: fmt.Sprintf("Protection Mode suppresses stationing; Open Gates is not capture-confirmed for castle %d's kingdom", unsupportedCastle), DetailDescriptor: Localization.New("server.automation.protection_mode_suppresses_stationing.028adab4", "Protection Mode suppresses stationing; Open Gates is not capture-confirmed for castle {p0}'s kingdom", Localization.Params{"p0": unsupportedCastle}),
			NextCheckAt: snapshot.Now.Add(30 * time.Second), Metrics: metrics,
		}
	}
	return Decision{
		Status: "threat", Detail: "The current Open Gate period ends before the final incoming attack; waiting to reopen it", DetailDescriptor: Localization.New("server.automation.the_current_open_gate.6abebca2", "The current Open Gate period ends before the final incoming attack; waiting to reopen it", nil),
		NextCheckAt: nextGateExpiry, Metrics: metrics,
	}
}

type stationUnit struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type threatWindow struct {
	CastleID State.CastleID
	Count    int
	Earliest time.Time
	Latest   time.Time
}

func protectedHoldings(alliance State.AllianceState, minimumRPTDays int) []State.AllianceHolding {
	minimum := clampInt(minimumRPTDays, 0, 30) * 86_400
	protection := make(map[State.PlayerID]int, len(alliance.Members))
	for _, member := range alliance.Members {
		protection[member.PlayerID] = member.ReturnProtectionSec
	}
	result := make([]State.AllianceHolding, 0, len(alliance.Holdings))
	for _, holding := range alliance.Holdings {
		if !allianceStationSlot(holding.SlotType) || holding.CastleID <= 0 || holding.X < 0 || holding.Y < 0 {
			continue
		}
		if minimum > 0 && protection[holding.PlayerID] <= minimum {
			continue
		}
		result = append(result, holding)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].KingdomID != result[right].KingdomID {
			return result[left].KingdomID < result[right].KingdomID
		}
		return result[left].CastleID < result[right].CastleID
	})
	return result
}

func nearestHolding(holdings []State.AllianceHolding, castle State.CastleState) (State.AllianceHolding, bool) {
	best := State.AllianceHolding{}
	bestDistance := 0
	for _, holding := range holdings {
		if holding.KingdomID != castle.KingdomID || holding.CastleID == castle.ID || holding.X == castle.X && holding.Y == castle.Y {
			continue
		}
		distance := absoluteInt(holding.X-castle.X) + absoluteInt(holding.Y-castle.Y)
		if best.CastleID == 0 || distance < bestDistance || distance == bestDistance && holding.CastleID < best.CastleID {
			best = holding
			bestDistance = distance
		}
	}
	return best, best.CastleID > 0
}

func SelectAutoBirdHolding(
	alliance State.AllianceState,
	castle State.CastleState,
	minimumRPTDays int,
) (State.AllianceHolding, bool) {
	return nearestHolding(protectedHoldings(alliance, minimumRPTDays), castle)
}

func incomingThreats(gameState State.GameState, now time.Time) (map[State.CastleID]threatWindow, int, time.Time, time.Time) {
	result := map[State.CastleID]threatWindow{}
	count := 0
	var earliest time.Time
	var latest time.Time
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if !State.IsIncomingPlayerAttack(gameState, movement, now) {
			return true
		}
		impact := movement.ArrivesAt
		window := result[movement.TargetCastleID]
		window.CastleID = movement.TargetCastleID
		window.Count++
		if window.Earliest.IsZero() || impact.Before(window.Earliest) {
			window.Earliest = *impact
		}
		if window.Latest.IsZero() || impact.After(window.Latest) {
			window.Latest = *impact
		}
		result[movement.TargetCastleID] = window
		count++
		if earliest.IsZero() || impact.Before(earliest) {
			earliest = *impact
		}
		if latest.IsZero() || impact.After(latest) {
			latest = *impact
		}
		return true
	})
	return result, count, earliest, latest
}

func stationableUnits(snapshot Snapshot, castle State.CastleState, reserves []reserveSetting) []stationUnit {
	reserved := make(map[State.UnitID]int64, len(reserves))
	for _, item := range reserves {
		if item.ID > 0 && item.Amount > 0 {
			reserved[item.ID] = item.Amount
		}
	}
	unitIDs := make([]State.UnitID, 0, len(castle.Units.Stationed))
	for unitID := range castle.Units.Stationed {
		unitIDs = append(unitIDs, unitID)
	}
	sort.Slice(unitIDs, func(left, right int) bool { return unitIDs[left] < unitIDs[right] })
	result := make([]stationUnit, 0, len(unitIDs))
	if snapshot.GameData == nil {
		return result
	}
	for _, unitID := range unitIDs {
		available := castle.Units.Stationed[unitID] - reserved[unitID]
		if available <= 0 {
			continue
		}
		isTool, exists := snapshot.GameData.UnitIsTool(int64(unitID))
		if !exists || isTool {
			continue
		}
		result = append(result, stationUnit{UnitID: unitID, Amount: available})
	}
	return result
}

func stationReserveUnits(reserves []reserveSetting) []stationUnit {
	result := make([]stationUnit, 0, len(reserves))
	for _, item := range reserves {
		if item.ID <= 0 || item.Amount <= 0 {
			continue
		}
		result = append(result, stationUnit{UnitID: item.ID, Amount: item.Amount})
	}
	return result
}

func withAutoBirdSchedule(snapshot Snapshot, decision Decision, notBefore time.Time) Decision {
	expectedReturns := expectedAutoBirdReturns(snapshot.State, snapshot.Now)
	expectedAt, castleID := earliestAutoBirdReturn(expectedReturns)
	if expectedAt.After(snapshot.Now) {
		if decision.Metrics == nil {
			decision.Metrics = map[string]float64{}
		}
		decision.Metrics[autoBirdNextMetricKey] = float64(expectedAt.UnixMilli())
		if castleID > 0 {
			decision.Metrics[autoBirdNextCastleMetricKey] = float64(castleID)
		}
	}
	if len(expectedReturns) > 0 {
		if decision.Metrics == nil {
			decision.Metrics = map[string]float64{}
		}
		for expectedCastleID, returnAt := range expectedReturns {
			decision.Metrics[autoBirdCastleReturnMetricKey+strconv.FormatInt(int64(expectedCastleID), 10)] =
				float64(returnAt.UnixMilli())
		}
	}
	wakeAt := expectedAt
	if !notBefore.IsZero() && (wakeAt.IsZero() || wakeAt.Before(notBefore)) {
		wakeAt = notBefore
	}
	if wakeAt.After(snapshot.Now) {
		if decision.NextCheckAt.IsZero() || wakeAt.Before(decision.NextCheckAt) {
			decision.NextCheckAt = wakeAt
		}
	}
	periodEndsAt := resolveWeeklySchedule(snapshot.Configuration, "autoBird", snapshot.Now).ValidUntil
	if periodEndsAt.After(snapshot.Now) &&
		(decision.NextCheckAt.IsZero() || periodEndsAt.Before(decision.NextCheckAt)) {
		decision.NextCheckAt = periodEndsAt
	}
	return decision
}

func expectedAutoBirdReturns(gameState State.GameState, now time.Time) map[State.CastleID]time.Time {
	result := map[State.CastleID]time.Time{}
	for _, operation := range gameState.Stationing {
		if operation.Purpose != "autoBird" || operation.SourceCastleID <= 0 ||
			operation.ExpectedReturnAt == nil || !operation.ExpectedReturnAt.After(now) {
			continue
		}
		result[operation.SourceCastleID] = operation.ExpectedReturnAt.UTC()
	}
	actual := map[State.CastleID]time.Time{}
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		castleID, matches := autoBirdMovementCastle(gameState, movement)
		if !matches {
			return true
		}
		candidate := time.Time{}
		switch {
		case movement.Direction == 1 && movement.ReturnsAt != nil:
			candidate = movement.ReturnsAt.UTC()
		case movement.Direction == 0 && movement.ArrivesAt != nil && movement.WaitSeconds > 0:
			candidate = movement.ArrivesAt.UTC().Add(
				time.Duration(movement.WaitSeconds+max(0, movement.TravelSeconds)) * time.Second,
			)
		}
		current := actual[castleID]
		if candidate.After(now) && (current.IsZero() || candidate.After(current)) {
			actual[castleID] = candidate
		}
		return true
	})
	for castleID, candidate := range actual {
		result[castleID] = candidate
	}
	return result

}

func earliestAutoBirdReturn(expectedReturns map[State.CastleID]time.Time) (time.Time, State.CastleID) {
	var next time.Time
	var nextCastleID State.CastleID
	for castleID, candidate := range expectedReturns {
		if next.IsZero() || candidate.Before(next) || candidate.Equal(next) && castleID < nextCastleID {
			next = candidate
			nextCastleID = castleID
		}
	}
	return next, nextCastleID
}

func nextGameReportedAutoBirdReturn(gameState State.GameState, now time.Time) (time.Time, State.CastleID) {
	var next time.Time
	var nextCastleID State.CastleID
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.Direction != 1 || movement.ReturnsAt == nil {
			return true
		}
		castleID, matches := autoBirdMovementCastle(gameState, movement)
		if !matches {
			return true
		}
		candidate := movement.ReturnsAt.UTC()
		if candidate.After(now) && (next.IsZero() || candidate.Before(next) ||
			candidate.Equal(next) && castleID < nextCastleID) {
			next = candidate
			nextCastleID = castleID
		}
		return true
	})
	return next, nextCastleID
}

func autoBirdMovementCastle(gameState State.GameState, movement State.MovementState) (State.CastleID, bool) {
	matchedAutoStation := false
	for _, operation := range gameState.Stationing {
		if !operation.MatchesMovement(movement) {
			continue
		}
		switch operation.Purpose {
		case "autoBird":
			return operation.SourceCastleID, operation.SourceCastleID > 0
		case "autoStation":
			matchedAutoStation = true
		}
	}
	if matchedAutoStation {
		return 0, false
	}
	sourceCastleID := movement.SourceCastleID
	holdingCastleID := movement.TargetCastleID
	if movement.Direction == 1 {
		sourceCastleID = movement.TargetCastleID
		holdingCastleID = movement.SourceCastleID
	}
	if _, owned := gameState.Castles[sourceCastleID]; !owned {
		return 0, false
	}
	for _, holding := range gameState.Alliance.Holdings {
		if holding.CastleID == holdingCastleID && allianceStationSlot(holding.SlotType) {
			return sourceCastleID, true
		}
	}
	return 0, false
}

func autoBirdStationActive(gameState State.GameState, castleID State.CastleID, now time.Time) bool {
	operation := gameState.Stationing["autoBird:"+strconv.FormatInt(int64(castleID), 10)]
	return operation.ActiveInState(gameState, now)
}

func hasActiveAllianceStationMovement(gameState State.GameState, castleID State.CastleID, holdings []State.AllianceHolding, now time.Time) bool {
	targets := make(map[State.CastleID]struct{}, len(holdings))
	for _, holding := range holdings {
		targets[holding.CastleID] = struct{}{}
	}
	active := false
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		// Cached support can outlive the list entry. Unknown timing remains
		// conservative, but a known completed return cannot block another cycle.
		release := State.StationMovementReleaseAt(movement)
		if movement.CommanderID != nil {
			if commanderRelease := State.CommanderMovementReleaseAt(movement); commanderRelease != nil && (release == nil || commanderRelease.After(*release)) {
				release = commanderRelease
			}
		}
		if release != nil && !release.After(now) {
			return true
		}
		_, outgoingTarget := targets[movement.TargetCastleID]
		_, returningSource := targets[movement.SourceCastleID]
		if (movement.Direction == 0 && movement.SourceCastleID == castleID && outgoingTarget) ||
			(movement.Direction == 1 && movement.TargetCastleID == castleID && returningSource) {
			active = true
			return false
		}
		return true
	})
	return active
}

func activeTrackedStation(gameState State.GameState, castleID State.CastleID, now time.Time) bool {
	operation := gameState.Stationing["autoStation:"+strconv.FormatInt(int64(castleID), 10)]
	return operation.ActiveInState(gameState, now)
}

func trackedStationMovement(gameState State.GameState, operation State.StationingOperation) (State.MovementState, bool) {
	if len(operation.MovementIDs) > 0 {
		for _, id := range operation.MovementIDs {
			if movement, exists := gameState.LookupMovement(id); exists && movement.Direction == 0 {
				return movement, true
			}
		}
		return State.MovementState{}, false
	}
	if operation.MovementID <= 0 {
		return State.MovementState{}, false
	}
	movement, exists := gameState.LookupMovement(operation.MovementID)
	if !exists {
		return State.MovementState{}, false
	}
	return movement, true
}

func allianceRefreshDecision(snapshot Snapshot, detail string, descriptors ...*Localization.Message) Decision {
	if snapshot.State.Alliance.ID <= 0 {
		return Decision{Status: "waiting", Detail: detail, DetailDescriptor: Localization.First(descriptors), NextCheckAt: snapshot.Now.Add(30 * time.Second)}
	}
	if observation, found := snapshot.State.Observations["ain"]; found && observation.LastDirection == "outbound" {
		nextCheck := observation.LastSeenAt.Add(allianceRefreshAwaitInterval)
		if nextCheck.After(snapshot.Now) {
			return Decision{Status: "waiting", Detail: "Waiting for the requested alliance roster", DetailDescriptor: Localization.New("server.automation.waiting_for_the_requested.9566f1bd", "Waiting for the requested alliance roster", nil), NextCheckAt: nextCheck}
		}
	}
	return Decision{
		Status: "waiting",
		Detail: detail, DetailDescriptor: Localization.First(descriptors),
		NextCheckAt:         snapshot.Now.Add(30 * time.Second),
		Request:             &Intent.Request{Name: "alliance.refresh", Arguments: json.RawMessage(`{}`)},
		ReevaluateOnSuccess: true,
	}
}

func allianceRosterRefreshDecision(snapshot Snapshot, detail string, descriptors ...*Localization.Message) (Decision, bool) {
	if snapshot.State.Alliance.ID <= 0 {
		return Decision{}, false
	}
	if !snapshot.State.Alliance.ObservedAt.IsZero() &&
		snapshot.Now.Sub(snapshot.State.Alliance.ObservedAt) < allianceRosterRefreshInterval {
		return Decision{}, false
	}
	return allianceRefreshDecision(snapshot, detail, descriptors...), true
}

func stationMetrics(threatCount int, earliestImpact time.Time) map[string]float64 {
	metrics := map[string]float64{"threatCount": float64(threatCount)}
	if !earliestImpact.IsZero() {
		metrics["nextImpactUnixMs"] = float64(earliestImpact.UnixMilli())
	}
	return metrics
}

func sortedCastleIDs(castles map[State.CastleID]State.CastleState) []State.CastleID {
	result := make([]State.CastleID, 0, len(castles))
	for id := range castles {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func sortedThreatCastleIDs(threats map[State.CastleID]threatWindow) []State.CastleID {
	result := make([]State.CastleID, 0, len(threats))
	for id := range threats {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func sortedStationingIDs(operations map[string]State.StationingOperation) []string {
	result := make([]string, 0, len(operations))
	for id := range operations {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func deterministicDelayHours(minimum, maximum int, revision uint64, castleID State.CastleID) int {
	if maximum <= minimum {
		return minimum
	}
	spread := uint64(maximum - minimum + 1)
	seed := revision*1_103_515_245 + uint64(castleID)*12_345
	return minimum + int(seed%spread)
}

func sumStationUnits(units []stationUnit) int64 {
	var total int64
	for _, item := range units {
		total += item.Amount
	}
	return total
}

func allianceStationSlot(slotType int) bool {
	switch slotType {
	case 0, 1, 3, 4, 5, 6, 12, 22:
		return true
	default:
		return false
	}
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func absoluteInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func roundedDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value.Round(time.Second)
}

// AutoBirdDispatchAllowed rechecks schedule and selected preset at the wire boundary.
func AutoBirdDispatchAllowed(configuration Configuration.Snapshot, preset string, now time.Time) bool {
	if !FeatureEnabledAt(configuration, "auto_bird", now) {
		return false
	}
	schedule := resolveWeeklySchedule(configuration, "autoBird", now)
	if !schedule.Allowed {
		return false
	}
	settings := defaultAutoBirdConfiguration()
	decodeSection(configuration, "automation.autoBird", &settings)
	selected := strings.TrimSpace(settings.ActivePresetID)
	if schedule.SlotOptionsEnabled {
		var ok bool
		selected, ok = autoBirdSchedulePresetID(schedule.Options)
		if !ok {
			return false
		}
	}
	return selected == preset
}
