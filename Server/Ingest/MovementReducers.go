package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func newMovementReducer(authoritative bool) Reducer {
	var snapshotMu sync.Mutex
	var lastSnapshotFrame time.Time
	return func(
		_ context.Context,
		frame Protocol.Frame,
		gameState *State.GameState,
		gameData *GameData.Store,
	) ([]string, bool, error) {
		if !frameSucceeded(frame) || len(frame.Payload) == 0 {
			return nil, false, nil
		}
		items, fullSnapshot, err := movementItems(frame.Payload)
		if err != nil {
			return nil, false, err
		}
		if authoritative && !fullSnapshot {
			return nil, false, fmt.Errorf("gam response does not contain a movement array")
		}
		before := gameState.Movements
		next := make(map[State.MovementID]State.MovementState, len(before)+len(items))
		for id, movement := range before {
			next[id] = movement
		}
		if authoritative {
			snapshotMu.Lock()
			reset := lastSnapshotFrame.IsZero() || frame.ReceivedAt.Sub(lastSnapshotFrame) > 250*time.Millisecond
			lastSnapshotFrame = frame.ReceivedAt
			snapshotMu.Unlock()
			// A single gam request can return a populated movement frame followed
			// by an empty frame. Only the first frame starts a new authoritative
			// snapshot; later frames in the same response window extend it.
			if reset {
				next = make(map[State.MovementID]State.MovementState, len(items))
				// Live gam replies are scoped and can omit owned movements that
				// appeared in another reply. Retain active commander movements
				// until an explicit removal, a newer observation, or their
				// conservative return time releases them.
				for id, movement := range before {
					if movement.CommanderID != nil && movementBelongsToCurrentPlayer(gameState, movement) &&
						State.CommanderMovementActiveAt(movement, frame.ReceivedAt) {
						next[id] = movement
					}
				}
			}
		}
		for _, raw := range items {
			movement, ok := parseMovement(raw, frame.ReceivedAt, gameData)
			if !ok {
				continue
			}
			next[movement.ID] = movement
		}
		khanChanged := reconcileKhanTaunts(gameState, next, frame.ReceivedAt, authoritative)
		movementChanged := !reflect.DeepEqual(before, next)
		if authoritative {
			gameState.MovementSnapshot.Version++
			gameState.MovementSnapshot.ObservedAt = frame.ReceivedAt
		}
		if !movementChanged && !authoritative && !khanChanged {
			return nil, false, nil
		}
		gameState.Movements = next
		syncCommanderAvailability(gameState)
		domains := []string{"movements", "commanders", "movement-snapshot"}
		if khanChanged {
			domains = append(domains, "khan", "event-scores")
		}
		return domains, true, nil
	}
}

func reduceMovementRemoval(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		ID wireInt64 `json:"MID"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode movement removal: %w", err)
	}
	id := State.MovementID(payload.ID)
	if _, exists := gameState.Movements[id]; !exists {
		return nil, false, nil
	}
	delete(gameState.Movements, id)
	khanChanged := resolveKhanTaunt(gameState, id, frame.ReceivedAt)
	syncCommanderAvailability(gameState)
	domains := []string{"movements", "commanders"}
	if khanChanged {
		domains = append(domains, "khan", "event-scores")
	}
	return domains, true, nil
}

// ReconcileExpiredMovements removes ordinary movements whose observed arrival
// or return time has passed and refreshes commander availability. Station waits
// remain authoritative until a later game snapshot changes or removes them.
func ReconcileExpiredMovements(gameState *State.GameState, now time.Time) bool {
	if gameState == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	changed := false
	for id, movement := range gameState.Movements {
		if movementActiveAt(movement, now) || movement.CommanderID != nil &&
			movementBelongsToCurrentPlayer(gameState, movement) && State.CommanderMovementActiveAt(movement, now) {
			continue
		}
		delete(gameState.Movements, id)
		resolveKhanTaunt(gameState, id, now)
		changed = true
	}
	for id, commander := range gameState.Commanders {
		available := commanderAvailableAt(gameState, id, now)
		if commander.Available == available {
			continue
		}
		commander.Available = available
		gameState.Commanders[id] = commander
		changed = true
	}
	return changed
}

func resolveKhanTaunt(gameState *State.GameState, movementID State.MovementID, resolvedAt time.Time) bool {
	if gameState == nil || gameState.Khan.Taunts == nil {
		return false
	}
	taunt, found := gameState.Khan.Taunts[movementID]
	if !found {
		return false
	}
	delete(gameState.Khan.Taunts, movementID)
	recordResolvedKhanTaunt(gameState, taunt)
	gameState.Khan.TauntsResolved++
	if resolvedAt.After(gameState.Khan.LastTauntResolvedAt) {
		gameState.Khan.LastTauntResolvedAt = resolvedAt.UTC()
	}
	return true
}

func movementActiveAt(movement State.MovementState, now time.Time) bool {
	if movement.Direction == 0 && movement.WaitSeconds > 0 {
		return true
	}
	completesAt := movement.ProjectedCompletionAt()
	return completesAt == nil || completesAt.IsZero() || completesAt.After(now)
}

// movementBelongsToCurrentPlayer keeps globally observed movement data available
// for target intelligence and defensive automation without allowing a foreign
// leader id to mark one of the current player's commanders unavailable.
func movementBelongsToCurrentPlayer(gameState *State.GameState, movement State.MovementState) bool {
	return gameState != nil && State.MovementOwnedByCurrentPlayer(*gameState, movement)
}

func movementItems(raw json.RawMessage) ([]json.RawMessage, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, false, fmt.Errorf("decode movements: %w", err)
	}
	if rawItems, exists := root["M"]; exists {
		var items []json.RawMessage
		if json.Unmarshal(rawItems, &items) == nil {
			return items, true, nil
		}
		var movement map[string]json.RawMessage
		if json.Unmarshal(rawItems, &movement) == nil {
			return []json.RawMessage{raw}, false, nil
		}
	}
	for _, wrapperName := range []string{"A", "AAM"} {
		wrapper, exists := root[wrapperName]
		if !exists {
			continue
		}
		var movement map[string]json.RawMessage
		if json.Unmarshal(wrapper, &movement) == nil {
			return []json.RawMessage{wrapper}, false, nil
		}
	}
	return nil, false, nil
}

func parseMovement(raw json.RawMessage, observedAt time.Time, gameData *GameData.Store) (State.MovementState, bool) {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil {
		return State.MovementState{}, false
	}
	var details struct {
		ID        wireInt64         `json:"MID"`
		Progress  int               `json:"PT"`
		Travel    int               `json:"TT"`
		Direction int               `json:"D"`
		TypeID    int               `json:"T"`
		KingdomID wireInt64         `json:"KID"`
		OwnerID   wireInt64         `json:"OID"`
		TargetID  wireInt64         `json:"TID"`
		Source    []json.RawMessage `json:"SA"`
		Target    []json.RawMessage `json:"TA"`
	}
	if json.Unmarshal(item["M"], &details) != nil || details.ID <= 0 {
		return State.MovementState{}, false
	}
	movement := State.MovementState{
		ID: State.MovementID(details.ID), TypeID: details.TypeID, Direction: details.Direction,
		OwnerPlayerID: State.PlayerID(details.OwnerID), TargetPlayerID: State.PlayerID(details.TargetID),
		KingdomID: State.KingdomID(details.KingdomID), TravelSeconds: details.Travel,
		ProgressSeconds: details.Progress, ObservedAt: observedAt.UTC(), Units: map[State.UnitID]int64{},
	}
	movement.StartedAt = observedAt.UTC().Add(-time.Duration(max(0, details.Progress)) * time.Second)
	var spyDetails struct {
		Count int `json:"SC"`
	}
	if json.Unmarshal(item["S"], &spyDetails) == nil && spyDetails.Count > 0 {
		movement.SpyCount = spyDetails.Count
	}
	if len(details.Source) > 4 {
		movement.SourceTypeID = int(rowInt(details.Source, 0))
		movement.SourceX = int(rowInt(details.Source, 1))
		movement.SourceY = int(rowInt(details.Source, 2))
		movement.SourceCastleID = State.CastleID(rowInt(details.Source, 3))
		if movement.OwnerPlayerID == 0 {
			movement.OwnerPlayerID = State.PlayerID(rowInt(details.Source, 4))
		}
	}
	if len(details.Target) > 4 {
		movement.TargetTypeID = int(rowInt(details.Target, 0))
		movement.TargetX = int(rowInt(details.Target, 1))
		movement.TargetY = int(rowInt(details.Target, 2))
		movement.TargetCastleID = State.CastleID(rowInt(details.Target, 3))
		if targetPlayerID := rowInt(details.Target, 4); targetPlayerID != 0 {
			movement.TargetPlayerID = State.PlayerID(targetPlayerID)
		}
	}
	var unitMovement struct {
		WaitSeconds int `json:"TWD"`
		Leader      struct {
			ID *wireInt64 `json:"ID"`
		} `json:"L"`
	}
	if json.Unmarshal(item["UM"], &unitMovement) == nil {
		movement.WaitSeconds = max(0, unitMovement.WaitSeconds)
		if unitMovement.Leader.ID != nil && *unitMovement.Leader.ID >= 0 {
			commanderID := State.CommanderID(*unitMovement.Leader.ID)
			movement.CommanderID = &commanderID
		}
	}
	for id, amount := range decodeUnitCounts(item["A"]) {
		movement.Units[State.UnitID(id)] = amount
	}
	var market struct {
		Barrows int                 `json:"C"`
		Goods   [][]json.RawMessage `json:"G"`
	}
	if json.Unmarshal(item["MM"], &market) == nil {
		movement.MarketBarrows = market.Barrows
		for _, good := range market.Goods {
			if len(good) < 2 {
				continue
			}
			jsonKey := rowString(good, 0)
			definitionID, found := officialDefinitionID(gameData, "resources", "resourceID", jsonKey)
			amount, exists := rawFloat64(good[1])
			if !found || !exists || amount <= 0 {
				continue
			}
			movement.MarketGoods = append(movement.MarketGoods, State.KingdomTransportGood{
				ResourceID: State.ResourceID(definitionID), Amount: amount,
			})
		}
	}
	completion := movement.StartedAt.Add(time.Duration(max(0, details.Travel)) * time.Second)
	if details.Direction == 0 {
		movement.ArrivesAt = &completion
	} else {
		movement.ReturnsAt = &completion
	}
	return movement, true
}

func reconcileKhanTaunts(
	gameState *State.GameState,
	movements map[State.MovementID]State.MovementState,
	observedAt time.Time,
	authoritative bool,
) bool {
	if gameState.Khan.Taunts == nil {
		gameState.Khan.Taunts = map[State.MovementID]State.KhanTauntState{}
	}
	changed := false
	for _, movement := range movements {
		if !isKhanTauntMovement(gameState, movement) || movement.ReturnsAt == nil || movement.ReturnsAt.IsZero() {
			continue
		}
		next := State.KhanTauntState{
			MovementID: movement.ID, ObservedAt: observedAt.UTC(), ImpactAt: movement.ReturnsAt.UTC(),
		}
		current, exists := gameState.Khan.Taunts[movement.ID]
		if exists {
			next.ObservedAt = current.ObservedAt
		}
		if !exists || current != next {
			gameState.Khan.Taunts[movement.ID] = next
			changed = true
		}
		if !exists {
			gameState.Khan.TauntsObserved++
			launchedAt := movement.ObservedAt
			if launchedAt.IsZero() {
				launchedAt = observedAt
			}
			State.RecordEventAttackLaunch(gameState, 72, State.EventAttackRecord{
				MovementID: movement.ID, Kind: State.EventActivityKhanDefense, KingdomID: movement.KingdomID,
				TargetTypeID: movement.TargetTypeID, TargetX: movement.TargetX, TargetY: movement.TargetY,
				LaunchedAt: launchedAt.UTC(), ArrivesAt: movement.ReturnsAt.UTC(),
			})
		}
	}
	if !authoritative {
		return changed
	}
	for movementID, taunt := range gameState.Khan.Taunts {
		if _, present := movements[movementID]; present || taunt.ImpactAt.After(observedAt.Add(2*time.Second)) {
			continue
		}
		delete(gameState.Khan.Taunts, movementID)
		recordResolvedKhanTaunt(gameState, taunt)
		gameState.Khan.TauntsResolved++
		if observedAt.After(gameState.Khan.LastTauntResolvedAt) {
			gameState.Khan.LastTauntResolvedAt = observedAt.UTC()
		}
		changed = true
	}
	return changed
}

func recordResolvedKhanTaunt(gameState *State.GameState, taunt State.KhanTauntState) {
	if gameState == nil || taunt.MovementID <= 0 {
		return
	}
	for _, current := range gameState.Khan.ResolvedTaunts {
		if current.MovementID == taunt.MovementID {
			return
		}
	}
	gameState.Khan.ResolvedTaunts = append(gameState.Khan.ResolvedTaunts, taunt)
	if len(gameState.Khan.ResolvedTaunts) > 256 {
		gameState.Khan.ResolvedTaunts = append(
			[]State.KhanTauntState(nil), gameState.Khan.ResolvedTaunts[len(gameState.Khan.ResolvedTaunts)-256:]...,
		)
	}
}

func isKhanTauntMovement(gameState *State.GameState, movement State.MovementState) bool {
	if movement.Direction != 1 || movement.TypeID != towerMapTypeID || movement.SourceTypeID != towerMapTypeID ||
		movement.SourceCastleID >= 0 || movement.KingdomID != 0 {
		return false
	}
	main, found := greatEmpireMainCastle(gameState)
	if !found || movement.TargetX != main.X || movement.TargetY != main.Y {
		return false
	}
	if movement.TargetCastleID > 0 && movement.TargetCastleID != main.ID {
		return false
	}
	if gameState.Khan.TargetX != 0 || gameState.Khan.TargetY != 0 {
		return movement.SourceX == gameState.Khan.TargetX && movement.SourceY == gameState.Khan.TargetY
	}
	_, active := gameState.EventScores.ByEvent[72]
	return active
}

func greatEmpireMainCastle(gameState *State.GameState) (State.CastleState, bool) {
	for _, castle := range gameState.Castles {
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return castle, true
		}
	}
	return State.CastleState{}, false
}
