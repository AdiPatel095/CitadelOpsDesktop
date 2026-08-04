package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackPresets"
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
	MinTroopsToTransfer        int64                  `json:"minTroopsToTransfer"`
	BeriCastleID               State.CastleID         `json:"beriCastleId"`
	TransferTroopID            State.UnitID           `json:"transferTroopId"`
	SourceCastleID             State.CastleID         `json:"sourceCastleId"`
	WireCastleID               int64                  `json:"wireCastleId"`
	TroopSpaceCheckIntervalSec int                    `json:"troopSpaceCheckIntervalSec"`
	PresetID                   string                 `json:"presetId"`
	AttackCheckIntervalSec     int                    `json:"attackCheckIntervalSec"`
	HorseTravelBoostID         int                    `json:"horseTravelBoostId"`
	ToolMinimums               map[State.UnitID]int64 `json:"toolMinimums"`
	UseTroopTransportTimeSkips bool                   `json:"useTroopTransportTimeSkips"`
	TroopTransportTimeSkipID   string                 `json:"troopTransportTimeSkipId"`
}

func NewBeriPolicy() *BeriPolicy { return &BeriPolicy{} }

func (*BeriPolicy) ID() string { return "autoBeriWorld" }

func (*BeriPolicy) EnabledKey() string { return "auto_beri_world" }

func (*BeriPolicy) WakeDomains() []string {
	return []string{"beri", "castles", "currencies", "kingdom-transport", "units"}
}

func (*BeriPolicy) WakeSections() []string { return []string{autoBeriWorldSection} }

func (policy *BeriPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := beriSettings{
		WireCastleID: -1, TroopSpaceCheckIntervalSec: 30,
		TroopTransportTimeSkipID: defaultBeriTroopTransportTimeSkipID,
	}
	decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings)
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
	if settings.TransferTroopID <= 0 {
		return Decision{
			Status: "waiting", Detail: "Configure the troop type transferred to Berimond",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if snapshot.GameData == nil {
		return Decision{
			Status: "waiting", Detail: "Official unit provision data is unavailable",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	usesFood, err := snapshot.GameData.UnitUsesFoodSupply(settings.TransferTroopID)
	if err != nil {
		return Decision{
			Status:      "waiting",
			Detail:      fmt.Sprintf("Waiting for the official provision type of unit %d: %v", settings.TransferTroopID, err),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if !usesFood {
		return Decision{
			Status: "waiting", Detail: "Choose a troop that consumes Food; Mead and Beef troops are not eligible for Berimond transfer",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	beriCastleID := settings.BeriCastleID
	if beriCastleID > 0 {
		castle, exists := snapshot.State.Castles[beriCastleID]
		if !exists {
			return Decision{
				Status: "waiting", Detail: "Waiting for the configured Berimond camp to be observed",
				NextCheckAt: snapshot.Now.Add(interval),
			}, nil
		}
		if castle.KingdomID != State.KingdomID(10) {
			return Decision{
				Status: "waiting", Detail: "The configured Berimond castle is not an owned Berimond camp",
				NextCheckAt: snapshot.Now.Add(interval),
			}, nil
		}
	} else {
		if castle, found := beriCastle(snapshot.State); found {
			beriCastleID = castle.ID
		}
	}
	if beriCastleID <= 0 {
		return Decision{
			Status: "waiting", Detail: "Waiting for an owned Berimond camp",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if unlock, observed := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(10)]; observed && !unlock.Unlocked {
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	sourceID := beriSourceCastle(snapshot.State, settings.SourceCastleID)
	source, exists := snapshot.State.Castles[sourceID]
	if !exists {
		return Decision{
			Status: "waiting", Detail: "Waiting for the Berimond source castle to be observed",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if source.KingdomID != 0 {
		return Decision{
			Status: "waiting", Detail: "The Berimond troop source must be a Great Empire castle",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	capacityExpired := snapshot.State.Beri.ObservedAt.IsZero() ||
		!snapshot.State.Beri.ConsumedAt.Before(snapshot.State.Beri.ObservedAt) ||
		snapshot.Now.Sub(snapshot.State.Beri.ObservedAt) >= interval
	if capacityExpired {
		arguments, _ := json.Marshal(map[string]any{"beriCastleId": beriCastleID})
		return Decision{
			Status:              "ready",
			Detail:              "Refresh Berimond troop-transfer capacity",
			NextCheckAt:         snapshot.Now.Add(time.Second),
			Request:             &Intent.Request{Name: "beri.capacity.refresh", Arguments: arguments},
			ReevaluateOnSuccess: true, ReevaluateOnStale: true,
		}, nil
	}
	available := snapshot.State.Beri.AvailableTroops
	if exact, exists := snapshot.State.Beri.TroopsByUnit[settings.TransferTroopID]; exists {
		available = exact
	}
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
			Status: "idle", Detail: fmt.Sprintf("Berimond capacity %d is below the configured minimum %d", available, minimum),
			NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
		}, nil
	}
	sourceUnitsCurrent := !source.UnitsObservedAt.IsZero() &&
		!source.UnitsObservedAt.After(snapshot.Now) &&
		snapshot.Now.Sub(source.UnitsObservedAt) < interval
	if sourceUnitsCurrent && source.Units.Stationed[settings.TransferTroopID] < available {
		return Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Waiting for transfer troops: source castle has %d of unit %d; Berimond has room for %d",
				source.Units.Stationed[settings.TransferTroopID], settings.TransferTroopID, available,
			),
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
		"sourceCastleId": sourceID, "targetCastleId": beriCastleID, "wireCastleId": settings.WireCastleID,
		"unitId": settings.TransferTroopID, "amount": available,
		"useTimeSkip": settings.UseTroopTransportTimeSkips, "timeSkipId": timeSkipID,
	})
	policy.skipChainArmed = settings.UseTroopTransportTimeSkips
	if policy.skipChainArmed {
		policy.skipChainArmedTimeSkipID = timeSkipID
	}
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Transfer %d troops to Berimond", available),
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
