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

const autoBeriWorldSection = "automation.autoBeriWorld"

type BeriPolicy struct{}

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
}

func NewBeriPolicy() *BeriPolicy { return &BeriPolicy{} }

func (*BeriPolicy) ID() string { return "autoBeriWorld" }

func (*BeriPolicy) EnabledKey() string { return "auto_beri_world" }

func (*BeriPolicy) WakeDomains() []string {
	return []string{"beri", "castles", "currencies", "kingdom-transport", "units"}
}

func (*BeriPolicy) WakeSections() []string { return []string{autoBeriWorldSection} }

func (*BeriPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := beriSettings{WireCastleID: -1, TroopSpaceCheckIntervalSec: 30}
	decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings)
	checkSeconds := settings.TroopSpaceCheckIntervalSec
	if checkSeconds < 5 {
		checkSeconds = 30
	} else if checkSeconds > 3600 {
		checkSeconds = 3600
	}
	interval := time.Duration(checkSeconds) * time.Second
	if settings.TransferTroopID <= 0 {
		return Decision{
			Status: "waiting", Detail: "Configure the troop type transferred to Berimond",
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
	if beriTroopTransportPending(snapshot.State) {
		return Decision{
			Status: "waiting", Detail: "Waiting for the current Berimond troop transport to settle",
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
	timeSkipCurrency := currencyIDForJSONKey(snapshot.GameData, "MS5")
	if timeSkipCurrency <= 0 {
		return Decision{
			Status: "waiting", Detail: "Official MS5 Berimond transport skip data is unavailable",
			NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
		}, nil
	}
	if snapshot.State.Player.Currencies[timeSkipCurrency] < 1 {
		return Decision{
			Status: "waiting", Detail: "Waiting for an MS5 skip for the Berimond troop transport",
			NextCheckAt: nextCheck, Metrics: map[string]float64{"availableTroops": float64(available)},
		}, nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": sourceID, "targetCastleId": beriCastleID, "wireCastleId": settings.WireCastleID,
		"unitId": settings.TransferTroopID, "amount": available,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Transfer %d troops to Berimond", available),
		NextCheckAt: snapshot.Now.Add(interval), Metrics: map[string]float64{"availableTroops": float64(available)},
		Request:             &Intent.Request{Name: "beri.transfer", Arguments: arguments},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func beriTroopTransportPending(gameState State.GameState) bool {
	for _, transport := range gameState.KingdomTransport.PendingUnits {
		if transport.KingdomID == State.KingdomID(10) {
			return true
		}
	}
	return false
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
