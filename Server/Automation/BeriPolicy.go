package Automation

import (
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
	refillConfigSignature              string
	refillScope                        string
	refillRemaining                    int64
	refillActive                       bool
	refillCandidate                    beriRefillCandidate
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

// A refill may use several one-unit shipments. The minimum free-capacity
// threshold starts a batch; only a confirmed shipment can unlock its bounded
// remainder when the next refreshed capacity falls below that threshold.
type beriRefillCandidate struct {
	unitID            State.UnitID
	amount            int64
	capacityBefore    int64
	campStockBefore   int64
	donorStockBefore  int64
	proposedAt        time.Time
	pendingObservedAt time.Time
}

func (policy *BeriPolicy) resetBeriRefill() {
	policy.refillConfigSignature = ""
	policy.refillScope = ""
	policy.refillRemaining = 0
	policy.refillActive = false
	policy.refillCandidate = beriRefillCandidate{}
}

func beriRefillConfigurationSignature(snapshot Snapshot) string {
	return fmt.Sprintf("%d:%s:%s", snapshot.Configuration.Revision,
		snapshot.Configuration.Sections[autoBeriWorldSection],
		snapshot.Configuration.Sections[AttackPresets.ConfigurationSection])
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
	configSignature := beriRefillConfigurationSignature(snapshot)
	if policy.refillScope != "" && policy.refillConfigSignature != configSignature {
		policy.resetBeriRefill()
	}
	if decision, locked := limitedEventGate(
		snapshot.State, snapshot.Now, []int64{GameData.BerimondEventID}, "Battle for Berimond",
	); locked {
		policy.resetBeriRefill()
		return decision, nil
	}
	if decision := beriGallantryBoosterGate(snapshot, settings); decision != nil {
		policy.resetBeriRefill()
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
		policy.resetBeriRefill()
		return Decision{
			Status: "waiting", Detail: "Choose a Berimond attack preset before transferring troops",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if snapshot.GameData == nil {
		policy.resetBeriRefill()
		return Decision{
			Status: "waiting", Detail: "Official unit provision data is unavailable",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	beriCamp, found := beriCastle(snapshot.State)
	if !found {
		policy.resetBeriRefill()
		return Decision{
			Status: "waiting", Detail: "Waiting for an owned Berimond camp",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	beriCastleID := beriCamp.ID
	if unlock, observed := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(10)]; observed && !unlock.Unlocked {
		policy.resetBeriRefill()
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	sourceID := beriSourceCastle(snapshot.State, settings.SourceCastleID)
	refillScope := fmt.Sprintf("%s|%d|%d", configSignature, beriCastleID, sourceID)
	if policy.refillScope != "" && policy.refillScope != refillScope {
		policy.resetBeriRefill()
	}
	source, exists := snapshot.State.Castles[sourceID]
	if !exists {
		policy.resetBeriRefill()
		return Decision{
			Status: "waiting", Detail: "Waiting for the Berimond source castle to be observed",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if source.KingdomID != 0 {
		policy.resetBeriRefill()
		return Decision{
			Status: "waiting", Detail: "The Berimond troop source must be a Great Empire castle",
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
		detail := "Refresh Berimond transfer capacity, donor troops and camp troops"
		return Decision{
			Status:              "ready",
			Detail:              detail,
			NextCheckAt:         snapshot.Now.Add(time.Second),
			Request:             &Intent.Request{Name: "beri.capacity.refresh", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	if pendingAt := policy.refillCandidate.pendingObservedAt; !pendingAt.IsZero() {
		if !snapshot.State.KingdomTransport.ObservedAt.After(pendingAt) {
			arguments, _ := json.Marshal(map[string]any{})
			return Decision{
				Status: "ready", Detail: "Confirm the Berimond troop transport has arrived",
				NextCheckAt:         snapshot.Now.Add(time.Second),
				Request:             &Intent.Request{Name: "troops.kingdom.refresh", Arguments: arguments},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, nil
		}
		if !beriCamp.UnitsObservedAt.After(pendingAt) {
			arguments, _ := json.Marshal(map[string]any{"castleId": beriCastleID, "refresh": true})
			return Decision{
				Status: "ready", Detail: "Refresh Berimond camp troops after the confirmed transfer",
				NextCheckAt:         snapshot.Now.Add(time.Second),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, nil
		}
		if !source.UnitsObservedAt.After(pendingAt) || !snapshot.State.Beri.ObservedAt.After(pendingAt) {
			arguments, _ := json.Marshal(map[string]any{"beriCastleId": beriCastleID, "sourceCastleId": sourceID})
			return Decision{
				Status: "ready", Detail: "Refresh donor troops and capacity after the confirmed Berimond transfer",
				NextCheckAt:         snapshot.Now.Add(time.Second),
				Request:             &Intent.Request{Name: "beri.capacity.refresh", Arguments: arguments},
				ReevaluateOnSuccess: true, ReevaluateOnStale: true,
			}, nil
		}
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
	policy.confirmBeriRefillArrival(snapshot, source, beriCamp)
	if policy.refillActive && policy.refillRemaining <= 0 {
		policy.resetBeriRefill()
	}
	if available < minimum && !policy.refillActive {
		return Decision{
			Status: "idle", Detail: fmt.Sprintf("Berimond capacity %d is below the configured minimum %d", available, minimum),
			NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
		}, nil
	}
	transferCapacity := snapshot.State.Beri
	if policy.refillActive {
		if transferCapacity.AvailableTroops > policy.refillRemaining {
			transferCapacity.AvailableTroops = policy.refillRemaining
		}
	}
	preset, err := beriAttackPreset(snapshot, settings)
	if err != nil {
		policy.resetBeriRefill()
		return Decision{Status: "waiting", Detail: err.Error(), NextCheckAt: nextCheck}, nil
	}
	unitID, amount, reason := beriProportionalTransfer(
		preset, source.Units.Stationed, beriCamp.Units.Stationed,
		transferCapacity, snapshot.GameData,
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
				Status: "waiting", Detail: "Choose a valid Berimond troop transport skip from MS1 through MS7",
				NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
			}, nil
		}
		timeSkipCurrency := currencyIDForJSONKey(snapshot.GameData, timeSkipID)
		if timeSkipCurrency <= 0 {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Official %s Berimond transport skip data is unavailable", timeSkipID),
				NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
			}, nil
		}
		if snapshot.State.Player.Currencies[timeSkipCurrency] < 1 {
			return Decision{
				Status: "waiting", Detail: fmt.Sprintf("Waiting for a %s skip for the Berimond troop transport", timeSkipID),
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
	policy.refillScope = refillScope
	policy.refillConfigSignature = configSignature
	if !policy.refillActive {
		policy.refillRemaining = available
	}
	policy.refillCandidate = beriRefillCandidate{
		unitID: unitID, amount: amount,
		capacityBefore: available, campStockBefore: beriCamp.Units.Stationed[unitID],
		donorStockBefore: source.Units.Stationed[unitID], proposedAt: snapshot.Now,
	}
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

func (policy *BeriPolicy) confirmBeriRefillArrival(snapshot Snapshot, source, camp State.CastleState) {
	candidate := policy.refillCandidate
	if candidate.pendingObservedAt.IsZero() {
		return
	}
	// A pending KUT alone cannot authorize another shipment. The refreshed
	// inventories and capacity must account for its exact proposed amount.
	campStock := camp.Units.Stationed[candidate.unitID]
	donorStock := source.Units.Stationed[candidate.unitID]
	if campStock < candidate.campStockBefore || campStock-candidate.campStockBefore < candidate.amount ||
		donorStock < 0 || donorStock > candidate.donorStockBefore || candidate.donorStockBefore-donorStock < candidate.amount ||
		snapshot.State.Beri.AvailableTroops > candidate.capacityBefore-candidate.amount {
		policy.resetBeriRefill()
		return
	}
	policy.refillRemaining -= candidate.amount
	policy.refillCandidate = beriRefillCandidate{}
	if policy.refillRemaining <= 0 {
		policy.resetBeriRefill()
		return
	}
	policy.refillActive = true
}

func (policy *BeriPolicy) beriPendingTroopTransportDecision(snapshot Snapshot, settings beriSettings) (*Decision, bool) {
	for _, transport := range snapshot.State.KingdomTransport.PendingUnits {
		if transport.KingdomID != State.KingdomID(10) {
			continue
		}
		candidate := &policy.refillCandidate
		matchingCandidate := candidate.amount > 0 &&
			snapshot.State.KingdomTransport.ObservedAt.After(candidate.proposedAt) &&
			len(transport.Units) == 1 && transport.Units[0].UnitID == candidate.unitID &&
			transport.Units[0].Amount == candidate.amount
		if matchingCandidate {
			if candidate.pendingObservedAt.IsZero() {
				candidate.pendingObservedAt = snapshot.State.KingdomTransport.ObservedAt
			}
		} else {
			policy.resetBeriRefill()
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
					Status: "waiting", Detail: "The refreshed Berimond troop transport is completing",
					NextCheckAt: policy.nextTransportFallbackAt, Metrics: metrics,
				}, true
			}
			arguments, _ := json.Marshal(map[string]any{})
			policy.transportFallbackRefreshPending = true
			policy.transportFallbackRefreshObservedAt = observedAt
			policy.nextTransportFallbackAt = snapshot.Now.Add(beriTroopTransportFallbackCheckInterval)
			return &Decision{
				Status: "ready", Detail: "Confirm the arriving Berimond troop transfer",
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
				Status: "waiting", Detail: fmt.Sprintf("Waiting for the current Berimond troop transport (%d seconds)", remaining),
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		timeSkipID, valid := beriTroopTransportTimeSkipID(settings.TroopTransportTimeSkipID)
		if !valid {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: "Choose a valid Berimond troop transport skip from MS1 through MS7",
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		if snapshot.GameData == nil {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: "Official transport skip data is unavailable",
				NextCheckAt: nextCheck, Metrics: metrics,
			}, true
		}
		timeSkipCurrency := currencyIDForJSONKey(snapshot.GameData, timeSkipID)
		if timeSkipCurrency <= 0 {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: fmt.Sprintf("Official %s Berimond transport skip data is unavailable", timeSkipID),
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
					Status:      "waiting",
					Detail:      "The previous Berimond troop transport skip did not produce a confirmed timer reduction; waiting for the next fallback refresh",
					NextCheckAt: policy.nextTransportFallbackAt, Metrics: metrics,
				}, true
			}
			policy.skipChainAwaitingResponse = false
		}
		if snapshot.State.Player.Currencies[timeSkipCurrency] < 1 {
			policy.clearBeriSkipChain()
			return &Decision{
				Status: "waiting", Detail: fmt.Sprintf("Waiting for a %s skip while the Berimond troop transport is travelling", timeSkipID),
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
				Status: "waiting", Detail: fmt.Sprintf("Berimond troop transport is still travelling; next fallback check in %d seconds", secondsUntilCheck),
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
				Status: "ready", Detail: "Refresh the Berimond troop transport before retrying its selected skip",
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
		),
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
		return AttackPresets.Preset{}, fmt.Errorf("the selected Berimond attack preset no longer exists")
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
