package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type BeriPolicy struct{}

type beriSettings struct {
	MinTroopsToTransfer        int64          `json:"minTroopsToTransfer"`
	BeriCastleID               State.CastleID `json:"beriCastleId"`
	TransferTroopID            State.UnitID   `json:"transferTroopId"`
	SourceCastleID             State.CastleID `json:"sourceCastleId"`
	WireCastleID               int64          `json:"wireCastleId"`
	TroopSpaceCheckIntervalSec int            `json:"troopSpaceCheckIntervalSec"`
}

func NewBeriPolicy() *BeriPolicy { return &BeriPolicy{} }

func (*BeriPolicy) ID() string { return "autoBeriWorld" }

func (*BeriPolicy) EnabledKey() string { return "auto_beri_world" }

func (*BeriPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := beriSettings{WireCastleID: -1, TroopSpaceCheckIntervalSec: 30}
	decodeSection(snapshot.Configuration, "automation.autoBeriWorld", &settings)
	checkSeconds := settings.TroopSpaceCheckIntervalSec
	if checkSeconds < 5 {
		checkSeconds = 30
	} else if checkSeconds > 3600 {
		checkSeconds = 3600
	}
	interval := time.Duration(checkSeconds) * time.Second
	if settings.BeriCastleID <= 0 || settings.TransferTroopID <= 0 {
		return Decision{
			Status: "waiting", Detail: "Configure a Berimond castle and transfer troop",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	if _, exists := snapshot.State.Castles[beriSourceCastle(snapshot.State, settings.SourceCastleID)]; !exists {
		return Decision{
			Status: "waiting", Detail: "Waiting for the Berimond source castle to be observed",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	capacityExpired := snapshot.State.Beri.ObservedAt.IsZero() ||
		!snapshot.State.Beri.ConsumedAt.Before(snapshot.State.Beri.ObservedAt) ||
		snapshot.Now.Sub(snapshot.State.Beri.ObservedAt) >= interval
	if capacityExpired {
		arguments, _ := json.Marshal(map[string]any{"beriCastleId": settings.BeriCastleID})
		return Decision{
			Status: "ready", Detail: "Refresh Berimond troop-transfer capacity",
			NextCheckAt: snapshot.Now.Add(time.Second),
			Request:     &Intent.Request{Name: "beri.capacity.refresh", Arguments: arguments},
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
	sourceID := beriSourceCastle(snapshot.State, settings.SourceCastleID)
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": sourceID, "wireCastleId": settings.WireCastleID,
		"unitId": settings.TransferTroopID, "amount": available,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Transfer %d troops to Berimond", available),
		NextCheckAt: snapshot.Now.Add(interval), Metrics: map[string]float64{"availableTroops": float64(available)},
		Request: &Intent.Request{Name: "beri.transfer", Arguments: arguments},
	}, nil
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
