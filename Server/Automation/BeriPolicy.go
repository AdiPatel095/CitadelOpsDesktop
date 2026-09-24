package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoBeriWorldSection                    = "automation.autoBeriWorld"
	defaultBeriTroopTransportTimeSkipID     = "MS5"
	beriTroopTransportFallbackCheckInterval = time.Minute
)

type BeriPolicy struct {
	pendingTransportSignature          string
	nextTransportFallbackAt            time.Time
	transportFallbackRefreshPending    bool
	transportFallbackRefreshObservedAt time.Time
	skipChainArmed                     bool
	skipChainArmedTimeSkipID           string
	skipChainActive                    bool
	skipChainAwaitingResponse          bool
	skipChainTimeSkipID                string
	skipChainObservedAt                time.Time
	skipChainRemainingSec              int
}

type beriSettings struct {
	MinTroopsToTransfer           int64                  `json:"minTroopsToTransfer"`
	SourceCastleID                State.CastleID         `json:"sourceCastleId"`
	TroopSpaceCheckIntervalSec    int                    `json:"troopSpaceCheckIntervalSec"`
	PresetID                      string                 `json:"presetId"`
	AttackCheckIntervalSec        int                    `json:"attackCheckIntervalSec"`
	DailyAttackLimit              int64                  `json:"dailyAttackLimit"`
	HorseTravelBoostID            int                    `json:"horseTravelBoostId"`
	ToolMinimums                  map[State.UnitID]int64 `json:"toolMinimums"`
	Build                         beriBuildSettings      `json:"build"`
	RequireActiveGallantryBooster bool                   `json:"requireActiveGallantryBooster"`
	UseTroopTransportTimeSkips    bool                   `json:"useTroopTransportTimeSkips"`
	TroopTransportTimeSkipID      string                 `json:"troopTransportTimeSkipId"`
}

func NewBeriPolicy() *BeriPolicy { return &BeriPolicy{} }

func (*BeriPolicy) ID() string { return "autoBeriWorld" }

func (*BeriPolicy) EnabledKey() string { return "auto_beri_world" }

func (*BeriPolicy) WakeDomains() []string {
	return []string{"beri", "boosters", "castles", "currencies", "events", "event-scores", "kingdom-transport", "units"}
}

func (*BeriPolicy) WakeSections() []string {
	return []string{autoBeriWorldSection, AttackPresets.ConfigurationSection}
}

func (policy *BeriPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := beriSettings{
		TroopSpaceCheckIntervalSec: 30,
		TroopTransportTimeSkipID:   defaultBeriTroopTransportTimeSkipID,
	}
	decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings)
	if decision, locked := limitedEventGate(
		snapshot.State, snapshot.Now, []int64{GameData.BerimondEventID}, "Battle for Berimond",
	); locked {
		return decision, nil
	}
	if decision := beriGallantryBoosterGate(snapshot, settings); decision != nil {
		return *decision, nil
	}
	checkSeconds := settings.TroopSpaceCheckIntervalSec
	if checkSeconds < 5 {
		checkSeconds = 30
	} else if checkSeconds > 3600 {
		checkSeconds = 3600
	}
	interval := time.Duration(checkSeconds) * time.Second
	if decision, pending := policy.beriPendingTroopTransportDecision(snapshot, settings); pending {
		return *decision, nil
	}
	if strings.TrimSpace(settings.PresetID) == "" {
		return Decision{
			Status: "waiting", Detail: "Choose a Berimond attack preset before transferring troops",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if snapshot.GameData == nil {
		return Decision{
			Status: "waiting", Detail: "Official unit provision data is unavailable", DetailDescriptor: Localization.New("server.automation.official_unit_provision_data.985609ec", "Official unit provision data is unavailable", nil),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	beriCamp, found := beriCastle(snapshot.State)
	if !found {
		return Decision{
			Status: "waiting", Detail: "Waiting for an owned Berimond camp", DetailDescriptor: Localization.New("server.automation.waiting_for_an_owned.deab064e", "Waiting for an owned Berimond camp", nil),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	beriCastleID := beriCamp.ID
	if unlock, observed := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(10)]; observed && !unlock.Unlocked {
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked", DetailDescriptor: Localization.New("server.automation.the_battle_for_berimond.a9f4b97a", "The Battle for Berimond is not currently unlocked", nil),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	sourceID := beriSourceCastle(snapshot.State, settings.SourceCastleID)
	source, exists := snapshot.State.Castles[sourceID]
	if !exists {
		return Decision{
			Status: "waiting", Detail: "Waiting for the Berimond source castle to be observed", DetailDescriptor: Localization.New("server.automation.waiting_for_the_berimond.a3a5544f", "Waiting for the Berimond source castle to be observed", nil),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if source.KingdomID != 0 {
		return Decision{
			Status: "waiting", Detail: "The Berimond troop source must be a Great Empire castle", DetailDescriptor: Localization.New("server.automation.the_berimond_troop_source.5db4cc3d", "The Berimond troop source must be a Great Empire castle", nil),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	sourceUnitsCurrent := !source.UnitsObservedAt.IsZero() &&
		!source.UnitsObservedAt.After(snapshot.Now) &&
		snapshot.Now.Sub(source.UnitsObservedAt) < interval
	campUnitsCurrent := !beriCamp.UnitsObservedAt.IsZero() &&
		!beriCamp.UnitsObservedAt.After(snapshot.Now) &&
		snapshot.Now.Sub(beriCamp.UnitsObservedAt) < interval
	if !campUnitsCurrent {
		arguments, _ := json.Marshal(map[string]any{"castleId": beriCastleID, "refresh": true})
		return Decision{
			Status: "ready", Detail: "Refresh Berimond camp troops before balancing the preset mix",
			NextCheckAt:         snapshot.Now.Add(time.Second),
			Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	capacityExpired := snapshot.State.Beri.ObservedAt.IsZero() ||
		!snapshot.State.Beri.ConsumedAt.Before(snapshot.State.Beri.ObservedAt) ||
		snapshot.Now.Sub(snapshot.State.Beri.ObservedAt) >= interval
	if capacityExpired || !sourceUnitsCurrent {
		arguments, _ := json.Marshal(map[string]any{
			"beriCastleId": beriCastleID, "sourceCastleId": sourceID,
		})
		detail := "Refresh Berimond troop-transfer capacity and selected donor inventory"
		var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.refresh_berimond_troop_transfer.7d177693", "Refresh Berimond troop-transfer capacity and selected donor inventory", nil)
		return Decision{
			Status: "ready",
			Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
			NextCheckAt:         snapshot.Now.Add(time.Second),
			Request:             &Intent.Request{Name: "beri.capacity.refresh", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	available := snapshot.State.Beri.AvailableTroops
	minimum := settings.MinTroopsToTransfer
	if minimum < 1 {
		minimum = 1
	}
	nextCheck := snapshot.State.Beri.ObservedAt.Add(interval)
	if !nextCheck.After(snapshot.Now) {
		nextCheck = snapshot.Now.Add(interval)
	}
	if available < minimum {
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Berimond capacity %d is below the configured minimum %d", available, minimum), DetailDescriptor: Localization.New("server.automation.berimond_capacity_p_is.07c22864", "Berimond capacity {p0} is below the configured minimum {p1}", Localization.Params{"p0": available, "p1": minimum}),
			NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
		}, nil
	}
	preset, err := beriAttackPreset(snapshot, settings)
	if err != nil {
		return Decision{Status: "waiting", Detail: err.Error(), NextCheckAt: nextCheck}, nil
	}
	unitID, amount, reason := beriProportionalTransfer(
		preset, source.Units.Stationed, beriCamp.Units.Stationed,
		snapshot.State.Beri, snapshot.GameData,
	)
	if reason != "" {
		return Decision{
			Status: "waiting", Detail: reason,
			NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
		}, nil
	}
	timeSkipID, validTimeSkip := beriTroopTransportTimeSkipID(settings.TroopTransportTimeSkipID)
	if settings.UseTroopTransportTimeSkips {
		if !validTimeSkip {
			return Decision{
				Status: "waiting", Detail: "Choose a valid Berimond troop transport skip from MS1 through MS7", DetailDescriptor: Localization.New("server.automation.choose_a_valid_berimond.2ffc2dbb", "Choose a valid Berimond troop transport skip from MS1 through MS7", nil),
				NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
			}, nil
		}
		timeSkipCurrency := currencyIDForJSONKey(snapshot.GameData, timeSkipID)
		if timeSkipCurrency <= 0 {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Official %s Berimond transport skip data is unavailable", timeSkipID), DetailDescriptor: Localization.New("server.automation.official_p_berimond_transport.5ebad39b", "Official {p0} Berimond transport skip data is unavailable", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipID)}),
				NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
			}, nil
		}
		if snapshot.State.Player.Currencies[timeSkipCurrency] < 1 {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Waiting for a %s skip for the Berimond troop transport", timeSkipID), DetailDescriptor: Localization.New("server.automation.waiting_for_a_p.20acc779", "Waiting for a {p0} skip for the Berimond troop transport", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipID)}),
				NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
			}, nil
		}
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": sourceID, "targetCastleId": beriCastleID,
		"unitId": unitID, "amount": amount,
		"configurationRevision": snapshot.Configuration.Revision,
		"donorUnitsObservedAt":  source.UnitsObservedAt, "campUnitsObservedAt": beriCamp.UnitsObservedAt,
		"useTimeSkip": settings.UseTroopTransportTimeSkips, "timeSkipId": timeSkipID,
	})
	policy.skipChainArmed = settings.UseTroopTransportTimeSkips
	if policy.skipChainArmed {
		policy.skipChainArmedTimeSkipID = timeSkipID
	}
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Transfer %d troops of unit %d to Berimond", amount, unitID),
		NextCheckAt: snapshot.Now.Add(interval), Metrics: map[string]float64{"availableTroops": float64(available)},
		Request:             &Intent.Request{Name: "beri.transfer", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func (policy *BeriPolicy) beriPendingTroopTransportDecision(snapshot Snapshot, settings beriSettings) (*Decision, bool) {
	for _, transport := range snapshot.State.KingdomTransport.PendingUnits {
		if transport.KingdomID != State.KingdomID(10) {
			continue
		}
		signature := beriTroopTransportSignature(transport)
		startArmedChain := false
		armedTimeSkipID := ""
		if policy.pendingTransportSignature != signature {
			startArmedChain = policy.skipChainArmed
			armedTimeSkipID = policy.skipChainArmedTimeSkipID
			policy.clearBeriSkipChain()
			policy.pendingTransportSignature = signature
			policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
			policy.transportFallbackRefreshPending = false
			policy.transportFallbackRefreshObservedAt = time.Time{}
		} else if policy.nextTransportFallbackAt.IsZero() {
			policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
		}
		policy.skipChainArmed = false
		remaining := transport.RemainingSec
		observedAt := snapshot.State.KingdomTransport.ObservedAt
		if !observedAt.IsZero() && snapshot.Now.After(observedAt) {
			remaining = max(0, remaining-int(snapshot.Now.Sub(observedAt)/time.Second))
		}
		freshAfterFallbackRefresh := policy.transportFallbackRefreshPending &&
			((policy.transportFallbackRefreshObservedAt.IsZero() && !observedAt.IsZero()) ||
				observedAt.After(policy.transportFallbackRefreshObservedAt))
		if freshAfterFallbackRefresh {
			policy.transportFallbackRefreshPending = false
			policy.transportFallbackRefreshObservedAt = time.Time{}
		}
		metrics := map[string]float64{
			"troopTransferRemainingSec": float64(remaining),
			"troopTransferStacks":       float64(len(transport.Units)),
		}
		if remaining <= 0 {
			policy.clearBeriSkipChain()
			if freshAfterFallbackRefresh {
				policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
				return &Decision{
					Status: "waiting", Detail: "The refreshed Berimond troop transport is completing", DetailDescriptor: Localization.New("server.automation.the_refreshed_berimond_troop.92542b8d", "The refreshed Berimond troop transport is completing", nil),
					NextCheckAt: policy.nextTransportFallbackAt, Metrics: metrics,
				}, true
			}
			arguments, _ := json.Marshal(map[string]any{})
			policy.transportFallbackRefreshPending = true
			policy.transportFallbackRefreshObservedAt = observedAt
			policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
			return &Decision{
				Status: "ready", Detail: "Confirm the arriving Berimond troop transfer", DetailDescriptor: Localization.New("server.automation.confirm_the_arriving_berimond.cf8b3cb7", "Confirm the arriving Berimond troop transfer", nil),
				NextCheckAt: snapshot.Now.Add(beriTroopTransportFallbackCheckInterval), Metrics: metrics,
				Request:             &Intent.Request{Name: "troops.kingdom.refresh", Arguments: arguments},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, true
		}

		nextCheck := snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
		arrivalCheck := snapshot.Now.Add(time.Duration(remaining) * time.Second)
		if arrivalCheck.Before(nextCheck) {
			nextCheck = arrivalCheck
		}
		if !policy.nextTransportFallbackAt.IsZero() && policy.nextTransportFallbackAt.Before(nextCheck) {
			nextCheck = policy.nextTransportFallbackAt
		}
		if !settings.UseTroopTransportTimeSkips {
			policy.clearBeriSkipChain()
			policy.transportFallbackRefreshPending = false
			policy.transportFallbackRefreshObservedAt = time.Time{}
			return &Decision{
				Status: "waiting", Detail: fmt.Sprintf("Waiting for the current Berimond troop transport (%d seconds)", remaining), DetailDescriptor: Localization.New("server.automation.waiting_for_the_current.b6efda1b", "Waiting for the current Berimond troop transport ({p0} seconds)", Localization.Params{"p0": remaining}),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		timeSkipID, valid := beriTroopTransportTimeSkipID(settings.TroopTransportTimeSkipID)
		if !valid {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: "Choose a valid Berimond troop transport skip from MS1 through MS7", DetailDescriptor: Localization.New("server.automation.choose_a_valid_berimond.2ffc2dbb", "Choose a valid Berimond troop transport skip from MS1 through MS7", nil),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		if snapshot.GameData == nil {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: "Official transport skip data is unavailable", DetailDescriptor: Localization.New("server.automation.official_transport_skip_data.d5ab9ed5", "Official transport skip data is unavailable", nil),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		timeSkipCurrency := currencyIDForJSONKey(snapshot.GameData, timeSkipID)
		if timeSkipCurrency <= 0 {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: fmt.Sprintf("Official %s Berimond transport skip data is unavailable", timeSkipID), DetailDescriptor: Localization.New("server.automation.official_p_berimond_transport.5ebad39b", "Official {p0} Berimond transport skip data is unavailable", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipID)}),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		if (startArmedChain && armedTimeSkipID == timeSkipID) || freshAfterFallbackRefresh {
			policy.skipChainActive = true
			policy.skipChainAwaitingResponse = false
			policy.skipChainTimeSkipID = timeSkipID
			policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
		}
		if policy.skipChainActive && policy.skipChainTimeSkipID != timeSkipID {
			policy.clearBeriSkipChain()
		}
		if policy.skipChainActive && policy.skipChainAwaitingResponse {
			if !beriTroopSkipReducedRemaining(
				policy.skipChainObservedAt,
				policy.skipChainRemainingSec,
				observedAt,
				transport.RemainingSec,
			) {
				policy.clearBeriSkipChain()
				policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
				return &Decision{
					Status: "waiting",
					Detail: "The previous Berimond troop transport skip did not produce a confirmed timer reduction; waiting for the next fallback refresh", DetailDescriptor: Localization.New("server.automation.the_previous_berimond_troop.23665c53", "The previous Berimond troop transport skip did not produce a confirmed timer reduction; waiting for the next fallback refresh", nil),
					NextCheckAt: policy.nextTransportFallbackAt, Metrics: metrics,
				}, true
			}
			policy.skipChainAwaitingResponse = false
		}
		if snapshot.State.Player.Currencies[timeSkipCurrency] < 1 {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: fmt.Sprintf("Waiting for a %s skip while the Berimond troop transport is travelling", timeSkipID), DetailDescriptor: Localization.New("server.automation.waiting_for_a_p.3f86413c", "Waiting for a {p0} skip while the Berimond troop transport is travelling", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipID)}),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		if policy.skipChainActive {
			return policy.beriTroopSkipDecision(
				snapshot, transport, observedAt, remaining, timeSkipID, timeSkipCurrency, metrics,
			), true
		}
		if !freshAfterFallbackRefresh && policy.nextTransportFallbackAt.After(snapshot.Now) {
			secondsUntilCheck := max(0, int(nextCheck.Sub(snapshot.Now)/time.Second))
			return &Decision{
				Status: "waiting", Detail: fmt.Sprintf("Berimond troop transport is still travelling; next fallback check in %d seconds", secondsUntilCheck), DetailDescriptor: Localization.New("server.automation.berimond_troop_transport_is.1cf1086a", "Berimond troop transport is still travelling; next fallback check in {p0} seconds", Localization.Params{"p0": secondsUntilCheck}),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		if !freshAfterFallbackRefresh {
			policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
		}
		nextCheck = policy.nextTransportFallbackAt
		if arrivalCheck.Before(nextCheck) {
			nextCheck = arrivalCheck
		}
		if !freshAfterFallbackRefresh {
			arguments, _ := json.Marshal(map[string]any{})
			policy.transportFallbackRefreshPending = true
			policy.transportFallbackRefreshObservedAt = observedAt
			return &Decision{
				Status: "ready", Detail: "Refresh the Berimond troop transport before retrying its selected skip", DetailDescriptor: Localization.New("server.automation.refresh_the_berimond_troop.9bc29084", "Refresh the Berimond troop transport before retrying its selected skip", nil),
				NextCheckAt: nextCheck, Metrics: metrics,
				Request:             &Intent.Request{Name: "troops.kingdom.refresh", Arguments: arguments},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, true
		}
	}
	policy.resetBeriTransportTracking()
	return nil, false
}

func (policy *BeriPolicy) beriTroopSkipDecision(
	snapshot Snapshot,
	transport State.KingdomUnitTransport,
	observedAt time.Time,
	remaining int,
	timeSkipID string,
	timeSkipCurrency State.CurrencyID,
	metrics map[string]float64,
) *Decision {
	skipSeconds := kingdomTimeSkipSeconds[timeSkipID]
	skipsNeeded := 1
	if skipSeconds > 0 {
		skipsNeeded = (remaining + skipSeconds - 1) / skipSeconds
		metrics["selectedTimeSkipSec"] = float64(skipSeconds)
		metrics["timeSkipsNeeded"] = float64(skipsNeeded)
	}
	available := int(snapshot.State.Player.Currencies[timeSkipCurrency])
	metrics["timeSkipsAvailable"] = float64(available)
	policy.skipChainActive = true
	policy.skipChainAwaitingResponse = true
	policy.skipChainTimeSkipID = timeSkipID
	policy.skipChainObservedAt = observedAt
	policy.skipChainRemainingSec = transport.RemainingSec
	arguments, _ := json.Marshal(map[string]any{
		"targetKingdomId": State.KingdomID(10), "timeSkipId": timeSkipID, "minimumRemaining": 0,
	})
	return &Decision{
		Status: "ready",
		Detail: fmt.Sprintf(
			"Apply one %s to the Berimond troop transport (%d selected skips needed, %d available)",
			timeSkipID, skipsNeeded, available,
		), DetailDescriptor: Localization.New("server.automation.apply_one_p_to.6bccaf98", "Apply one {p0} to the Berimond troop transport ({p1} selected skips needed, {p2} available)", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipID), "p1": skipsNeeded, "p2": available}),
		NextCheckAt: snapshot.Now.Add(beriTroopTransportFallbackCheckInterval), Metrics: metrics,
		Request:             &Intent.Request{Name: "troops.kingdom.skip", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}
}

func beriTroopSkipReducedRemaining(
	previousObservedAt time.Time,
	previousRemainingSec int,
	currentObservedAt time.Time,
	currentRemainingSec int,
) bool {
	if previousObservedAt.IsZero() || currentObservedAt.IsZero() || !currentObservedAt.After(previousObservedAt) {
		return false
	}
	elapsedSec := max(0, int(currentObservedAt.Sub(previousObservedAt)/time.Second))
	naturalRemaining := max(0, previousRemainingSec-elapsedSec)
	return currentRemainingSec < naturalRemaining
}

func (policy *BeriPolicy) clearBeriSkipChain() {
	policy.skipChainArmed = false
	policy.skipChainArmedTimeSkipID = ""
	policy.skipChainActive = false
	policy.skipChainAwaitingResponse = false
	policy.skipChainTimeSkipID = ""
	policy.skipChainObservedAt = time.Time{}
	policy.skipChainRemainingSec = 0
}

func (policy *BeriPolicy) resetBeriTransportTracking() {
	policy.pendingTransportSignature = ""
	policy.nextTransportFallbackAt = time.Time{}
	policy.transportFallbackRefreshPending = false
	policy.transportFallbackRefreshObservedAt = time.Time{}
	policy.clearBeriSkipChain()
}

func beriTroopTransportSignature(transport State.KingdomUnitTransport) string {
	units := append([]State.KingdomTransportUnit(nil), transport.Units...)
	sort.Slice(units, func(left, right int) bool {
		if units[left].UnitID == units[right].UnitID {
			return units[left].Amount < units[right].Amount
		}
		return units[left].UnitID < units[right].UnitID
	})
	var signature strings.Builder
	fmt.Fprintf(&signature, "%d", transport.KingdomID)
	for _, unit := range units {
		fmt.Fprintf(&signature, "|%d:%d", unit.UnitID, unit.Amount)
	}
	return signature.String()
}

func beriTroopTransportTimeSkipID(raw string) (string, bool) {
	timeSkipID := strings.ToUpper(strings.TrimSpace(raw))
	if timeSkipID == "" {
		timeSkipID = defaultBeriTroopTransportTimeSkipID
	}
	switch timeSkipID {
	case "MS1", "MS2", "MS3", "MS4", "MS5", "MS6", "MS7":
		return timeSkipID, true
	default:
		return "", false
	}
}

func beriCastle(gameState State.GameState) (State.CastleState, bool) {
	ids := make([]State.CastleID, 0, len(gameState.Castles))
	for id := range gameState.Castles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		castle := gameState.Castles[id]
		if castle.KingdomID == State.KingdomID(10) {
			return castle, true
		}
	}
	return State.CastleState{}, false
}

func beriAttackPreset(snapshot Snapshot, settings beriSettings) (AttackPresets.Preset, error) {
	document, err := AttackPresets.Decode(snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
	if err != nil {
		return AttackPresets.Preset{}, err
	}
	preset, found := AttackPresets.Find(document, strings.TrimSpace(settings.PresetID))
	if !found {
		return AttackPresets.Preset{}, Localization.WithError(fmt.Errorf("the selected Berimond attack preset no longer exists"), Localization.New("server.automation.the_selected_berimond_attack.cc0f853f", "the selected Berimond attack preset no longer exists", nil))
	}
	return preset, nil
}

func beriSourceCastle(gameState State.GameState, requested State.CastleID) State.CastleID {
	if requested > 0 {
		return requested
	}
	ids := make([]State.CastleID, 0, len(gameState.Castles))
	for id := range gameState.Castles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		castle := gameState.Castles[id]
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return id
		}
	}
	return 0
}
