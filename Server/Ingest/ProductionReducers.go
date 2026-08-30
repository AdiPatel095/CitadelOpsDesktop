package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type productionWireProduct struct {
	DefinitionID      wireInt64 `json:"WID"`
	Amount            wireInt64 `json:"TUA"`
	RuntimeSec        int       `json:"RCT"`
	ProductionID      wireInt64 `json:"PID"`
	StackProductionID wireInt64 `json:"SPID"`
	HelpRequested     *bool     `json:"RAH"`
}

type productionWireSlot struct {
	Product productionWireProduct `json:"P"`
	Slot    struct {
		RentalUntil int `json:"RUT"`
		VIP         int `json:"VIP"`
	} `json:"SI"`
}

type productionWireSnapshot struct {
	Active  productionWireProduct `json:"PS"`
	Queued  []productionWireSlot  `json:"QS"`
	Compact [][]json.RawMessage   `json:"PIDL"`
	LineID  int                   `json:"LID"`
}

type productionHelpSnapshot struct {
	LineID             int
	Complete           bool
	Items              map[int64]bool
	PriorProductionIDs map[int64]struct{}
}

func reduceProductionSnapshot(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	castleID, castle, ok := focusedCastle(gameState)
	if !ok {
		return nil, false, nil
	}
	recruitmentHelpOutstanding := State.HasOutstandingRecruitmentAllianceHelpRequest(*gameState, castleID)
	preserveRequestedHelp := State.OwnAllianceHelpStateCurrent(*gameState)
	castle, ok = gameState.MutableCastleParts(castleID, State.CastlePartProduction)
	if !ok {
		return nil, false, nil
	}
	changed, helpSnapshot, err := applyProductionSnapshot(
		frame.Payload, castleID, &castle, frame.ReceivedAt, recruitmentHelpOutstanding,
		preserveRequestedHelp, gameState.Session.ChangedAt, gameState.SubscriptionScope(),
	)
	if err != nil {
		return nil, false, err
	}
	helpChanged := reconcileProductionHelpSnapshot(gameState, castleID, helpSnapshot)
	if !changed && !helpChanged {
		return nil, false, nil
	}
	domains := []string{}
	if changed {
		gameState.SetCastleParts(castleID, castle, State.CastlePartProduction)
		domains = append(domains, "castles", "production")
	}
	if helpChanged {
		domains = append(domains, "alliance-help")
	}
	return domains, true, nil
}

func reduceEmbeddedProductionSnapshots(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	castleID, castle, ok := focusedCastle(gameState)
	if !ok {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode embedded production snapshots: %w", err)
	}
	castle, ok = gameState.MutableCastleParts(castleID, State.CastlePartProduction)
	if !ok {
		return nil, false, nil
	}
	changed := false
	helpChanged := false
	recruitmentHelpOutstanding := State.HasOutstandingRecruitmentAllianceHelpRequest(*gameState, castleID)
	preserveRequestedHelp := State.OwnAllianceHelpStateCurrent(*gameState)
	for key, raw := range root {
		if !strings.HasPrefix(key, "spl") || key == "spl" || len(raw) == 0 {
			continue
		}
		updated, helpSnapshot, err := applyProductionSnapshot(
			raw, castleID, &castle, frame.ReceivedAt, recruitmentHelpOutstanding,
			preserveRequestedHelp, gameState.Session.ChangedAt, gameState.SubscriptionScope(),
		)
		if err != nil {
			return nil, false, err
		}
		if updated {
			changed = true
		}
		if reconcileProductionHelpSnapshot(gameState, castleID, helpSnapshot) {
			helpChanged = true
		}
	}
	if !changed && !helpChanged {
		return nil, false, nil
	}
	domains := []string{}
	if changed {
		gameState.SetCastleParts(castleID, castle, State.CastlePartProduction)
		domains = append(domains, "castles", "production")
	}
	if helpChanged {
		domains = append(domains, "alliance-help")
	}
	return domains, true, nil
}

func applyProductionSnapshot(
	raw json.RawMessage,
	castleID State.CastleID,
	castle *State.CastleState,
	observedAt time.Time,
	recruitmentHelpOutstanding bool,
	preserveRequestedHelp bool,
	sessionChangedAt time.Time,
	subscriptionScope string,
) (bool, productionHelpSnapshot, error) {
	helpSnapshot := productionHelpSnapshot{
		LineID: -1, Items: map[int64]bool{}, PriorProductionIDs: map[int64]struct{}{},
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, helpSnapshot, fmt.Errorf("decode production snapshot: %w", err)
	}
	if nested := root["spl"]; len(nested) > 0 {
		raw = nested
	}
	var wire productionWireSnapshot
	if err := json.Unmarshal(raw, &wire); err != nil {
		return false, helpSnapshot, fmt.Errorf("decode production snapshot fields: %w", err)
	}
	helpSnapshot.LineID = wire.LineID
	if wire.LineID < 0 || len(root["PS"]) == 0 && len(root["QS"]) == 0 && len(root["PIDL"]) == 0 && len(root["spl"]) == 0 {
		return false, helpSnapshot, nil
	}
	if nested := root["spl"]; len(nested) > 0 {
		var nestedRoot map[string]json.RawMessage
		_ = json.Unmarshal(nested, &nestedRoot)
		root = nestedRoot
	}
	ensureCastleMaps(castle)
	previousQueue := castle.Production[wire.LineID]
	if wire.LineID == 2 {
		if previousQueue.Active != nil && previousQueue.Active.ProductionID > 0 {
			helpSnapshot.PriorProductionIDs[previousQueue.Active.ProductionID] = struct{}{}
		}
		for _, item := range previousQueue.Queued {
			if item.ProductionID > 0 {
				helpSnapshot.PriorProductionIDs[item.ProductionID] = struct{}{}
			}
		}
	}
	preserveRequestedHelp = preserveRequestedHelp && !previousQueue.ObservedAt.IsZero() &&
		(sessionChangedAt.IsZero() || !previousQueue.ObservedAt.Before(sessionChangedAt))
	requestedHelp := requestedProductionHelp(previousQueue)
	helpItems := 0
	helpItemsObserved := 0
	observeHelp := func(product productionWireProduct, item State.QueueItem) {
		helpItems++
		if product.HelpRequested == nil {
			return
		}
		helpItemsObserved++
		if item.ProductionID > 0 {
			helpSnapshot.Items[item.ProductionID] = helpSnapshot.Items[item.ProductionID] || *product.HelpRequested
		}
	}
	reconcileItemHelp := func(item *State.QueueItem, product productionWireProduct) {
		if item == nil || product.HelpRequested != nil {
			return
		}
		if (preserveRequestedHelp && requestedHelp[item.ProductionID]) ||
			(wire.LineID == 0 && recruitmentHelpOutstanding) {
			item.AllianceHelpRequested = true
		}
	}
	queue := State.ProductionQueue{
		LineID: wire.LineID, ObservedAt: observedAt, Queued: []State.QueueItem{},
	}
	if item, exists := productionQueueItem(wire.LineID, wire.Active, observedAt, true); exists {
		reconcileItemHelp(&item, wire.Active)
		observeHelp(wire.Active, item)
		queue.Active = &item
	}
	if len(root["QS"]) > 0 {
		// QS lists every slot the player owns — base, VIP, rented, and slots
		// granted by capacity effects — one entry per slot whether or not a
		// unit occupies it. Purchasable-but-locked slots ride offer data, not
		// QS, so the slot count IS the queue capacity; counting only occupied
		// or rented entries silently discarded empty effect-granted slots.
		queue.Capacity = len(wire.Queued)
		for _, slot := range wire.Queued {
			if item, exists := productionQueueItem(wire.LineID, slot.Product, observedAt, false); exists {
				reconcileItemHelp(&item, slot.Product)
				observeHelp(slot.Product, item)
				queue.Queued = append(queue.Queued, item)
			}
		}
	}
	if len(root["PIDL"]) > 0 {
		queue.Capacity = 0
		queue.Queued = []State.QueueItem{}
		for _, row := range wire.Compact {
			if len(row) == 0 {
				continue
			}
			queue.Capacity++
			product := productionWireProduct{
				DefinitionID: wireInt64(rowInt(row, 0)), Amount: wireInt64(rowInt(row, 1)),
			}
			if len(row) > 2 {
				product.RuntimeSec = int(rowInt(row, 2))
			}
			if len(row) > 5 {
				product.ProductionID = wireInt64(rowInt(row, 5))
			}
			if len(row) > 4 {
				// Hospital PIDL does not include RAH. Its fifth value is the
				// server-applied alliance-help reduction; once positive, another
				// AHR for that job is rejected.
				helpRequested := rowInt(row, 4) > 0
				product.HelpRequested = &helpRequested
			}
			if item, exists := productionQueueItem(wire.LineID, product, observedAt, false); exists {
				reconcileItemHelp(&item, product)
				observeHelp(product, item)
				queue.Queued = append(queue.Queued, item)
			}
		}
	}
	helpSnapshot.Complete = (wire.LineID == 0 || wire.LineID == 2) && helpItemsObserved == helpItems
	// High-water stack learning: carry the largest stack ever observed per
	// unit definition forward across snapshots, but only while the
	// subscription set it was learned under still holds. A scope change
	// (subscription gained or lapsed) discards learned values so the line
	// re-learns from live stacks instead of overshooting a lapsed
	// entitlement. Keyed by definition because batch caps are per-unit —
	// one unit's learned cap must never floor another unit's stacks.
	if previous := castle.Production[wire.LineID]; previous.LearnedStackScope == subscriptionScope && len(previous.LearnedStacks) > 0 {
		queue.LearnedStacks = make(map[int64]int64, len(previous.LearnedStacks))
		for definitionID, amount := range previous.LearnedStacks {
			queue.LearnedStacks[definitionID] = amount
		}
	}
	queue.LearnedStackScope = subscriptionScope
	learnObservedStacks(&queue)
	if reflect.DeepEqual(castle.Production[wire.LineID], queue) {
		return false, helpSnapshot, nil
	}
	castle.Production[wire.LineID] = queue
	_ = castleID
	return true, helpSnapshot, nil
}

func reduceProductionCommandContext(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var command struct {
		SessionKey int `json:"SK"`
	}
	if err := json.Unmarshal(frame.Payload, &command); err != nil {
		return nil, false, fmt.Errorf("decode production command context: %w", err)
	}
	if command.SessionKey <= 0 || gameState.CommandContext.ProductionSessionKey == command.SessionKey {
		return nil, false, nil
	}
	gameState.CommandContext.ProductionSessionKey = command.SessionKey
	observedAt := frame.ReceivedAt
	gameState.CommandContext.ProductionObservedAt = &observedAt
	return []string{"command-context"}, true, nil
}

func productionQueueItem(lineID int, product productionWireProduct, observedAt time.Time, active bool) (State.QueueItem, bool) {
	if product.DefinitionID <= 0 || product.Amount <= 0 {
		return State.QueueItem{}, false
	}
	collection := "units"
	if lineID == 1 {
		collection = "tools"
	}
	productionID := product.ProductionID
	if active && product.StackProductionID > 0 {
		productionID = product.StackProductionID
	}
	item := State.QueueItem{
		Definition:            State.DefinitionRef{Collection: collection, ID: int64(product.DefinitionID)},
		Amount:                int64(product.Amount),
		ProductionID:          int64(productionID),
		AllianceHelpAvailable: (lineID == 0 || lineID == 2) && productionID > 0,
		AllianceHelpRequested: product.HelpRequested != nil && *product.HelpRequested,
	}
	if active {
		startedAt := observedAt
		item.StartedAt = &startedAt
		if product.RuntimeSec > 0 {
			completesAt := observedAt.Add(time.Duration(product.RuntimeSec) * time.Second)
			item.CompletesAt = &completesAt
		}
	}
	return item, true
}

func reconcileProductionHelpSnapshot(
	gameState *State.GameState,
	castleID State.CastleID,
	snapshot productionHelpSnapshot,
) bool {
	if gameState == nil {
		return false
	}
	switch snapshot.LineID {
	case 0:
		requested := false
		for _, itemRequested := range snapshot.Items {
			if itemRequested {
				requested = true
				break
			}
		}
		if !requested && !snapshot.Complete {
			return false
		}
		return State.ReconcileOwnRecruitmentAllianceHelp(gameState, castleID, requested)
	case 2:
		if !snapshot.Complete {
			return false
		}
		changed := false
		for productionID := range snapshot.PriorProductionIDs {
			if _, stillPresent := snapshot.Items[productionID]; stillPresent {
				continue
			}
			if State.ReconcileOwnHospitalAllianceHelp(gameState, productionID, false) {
				changed = true
			}
		}
		for productionID, requested := range snapshot.Items {
			if State.ReconcileOwnHospitalAllianceHelp(gameState, productionID, requested) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

type allianceHelpWireRequest struct {
	ListID           wireInt64      `json:"LID"`
	PlayerID         State.PlayerID `json:"PID"`
	RequestType      int            `json:"TID"`
	Progress         wireInt64      `json:"P"`
	AlreadyConfirmed wireInt64      `json:"AC"`
	Optional         struct {
		CastleID      State.CastleID `json:"AID"`
		RecruitmentID int64          `json:"RID"`
		LineID        int            `json:"RLID"`
	} `json:"OP"`
}

func reduceAllianceHelpRequest(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	requests := []allianceHelpWireRequest{}
	if frame.Opcode == "ahl" {
		var list struct {
			Requests []allianceHelpWireRequest `json:"AHL"`
		}
		if err := json.Unmarshal(frame.Payload, &list); err != nil {
			return nil, false, fmt.Errorf("decode alliance help list: %w", err)
		}
		requests = list.Requests
	} else {
		var request allianceHelpWireRequest
		if err := json.Unmarshal(frame.Payload, &request); err != nil {
			return nil, false, fmt.Errorf("decode alliance help request: %w", err)
		}
		requests = append(requests, request)
	}
	changed := false
	if frame.Opcode == "ahl" {
		hospitalProductionIDs := ownHospitalAllianceHelpProductionIDs(requests, gameState.Player.ID)
		recruitmentCastleIDs := ownRecruitmentAllianceHelpCastleIDs(requests, gameState.Player.ID)
		ownRecruitmentRequests := ownRecruitmentAllianceHelpRequests(
			requests, gameState.Player.ID, gameState.Session.Generation, frame.ReceivedAt,
			gameData, gameState.AllianceHelpRequests,
		)
		pendingOtherListIDs := pendingOtherAllianceHelpListIDs(requests, gameState.Player.ID, gameData)
		next := State.AllianceHelpRequestState{
			HospitalProductionIDs:            hospitalProductionIDs,
			RecruitmentCastleIDs:             recruitmentCastleIDs,
			OwnRecruitmentRequests:           ownRecruitmentRequests,
			OwnRecruitmentObservedGeneration: gameState.Session.Generation,
			PendingOtherListIDs:              pendingOtherListIDs,
			ObservedAt:                       frame.ReceivedAt,
			OwnObservedGeneration:            gameState.Session.Generation,
			OthersObservedAt:                 frame.ReceivedAt,
			OthersObservedGeneration:         gameState.Session.Generation,
			LastHelpAllAt:                    gameState.AllianceHelpRequests.LastHelpAllAt,
			LastHelpAllGeneration:            gameState.AllianceHelpRequests.LastHelpAllGeneration,
		}
		if !reflect.DeepEqual(gameState.AllianceHelpRequests, next) {
			gameState.AllianceHelpRequests = next
			changed = true
		}
	}
	for _, request := range requests {
		if request.PlayerID <= 0 {
			continue
		}
		if request.PlayerID != gameState.Player.ID {
			if frame.Opcode != "ahl" && updatePendingOtherAllianceHelpRequest(
				&gameState.AllianceHelpRequests, request, gameState.Player.ID,
				gameState.Session.Generation, frame.ReceivedAt, gameData,
			) {
				changed = true
			}
			continue
		}
		if frame.Opcode != "ahl" {
			switch request.RequestType {
			case 2:
				if State.ReconcileOwnHospitalAllianceHelp(gameState, request.Optional.RecruitmentID, true) {
					changed = true
				}
			case 6:
				if State.ReconcileOwnRecruitmentAllianceHelp(gameState, request.Optional.CastleID, true) {
					changed = true
				}
				if updateOwnRecruitmentAllianceHelpRequest(
					&gameState.AllianceHelpRequests, request, gameState.Player.ID,
					gameState.Session.Generation, frame.ReceivedAt, gameData,
				) {
					changed = true
				}
			}
		}
		if request.Optional.CastleID <= 0 {
			continue
		}
		castle, exists := gameState.MutableCastleParts(request.Optional.CastleID, State.CastlePartProduction)
		if !exists {
			continue
		}
		lineID := request.Optional.LineID
		productionID := request.Optional.RecruitmentID
		switch request.RequestType {
		case 2:
			lineID = 2
		case 6:
			lineID = 0
			productionID = 0
		default:
			continue
		}
		queue, exists := castle.Production[lineID]
		if !exists {
			continue
		}
		if markAllianceHelpQueue(&queue, lineID == 0, productionID) {
			castle.Production[lineID] = queue
			gameState.SetCastleParts(request.Optional.CastleID, castle, State.CastlePartProduction)
			changed = true
		}
	}
	if !changed {
		return nil, false, nil
	}
	return []string{"alliance-help", "castles", "production"}, true, nil
}

func reduceAllianceHelpDelete(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		ListID wireInt64 `json:"LID"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode alliance help deletion: %w", err)
	}
	if payload.ListID <= 0 {
		return nil, false, nil
	}
	changed := removeOwnRecruitmentAllianceHelpRequest(
		&gameState.AllianceHelpRequests, int64(payload.ListID),
		gameState.Session.Generation, frame.ReceivedAt,
	)
	if gameState.AllianceHelpRequests.OthersObservedGeneration == gameState.Session.Generation &&
		removePendingOtherAllianceHelpListID(&gameState.AllianceHelpRequests, int64(payload.ListID)) {
		gameState.AllianceHelpRequests.OthersObservedAt = frame.ReceivedAt
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	return []string{"alliance-help"}, true, nil
}

func pendingOtherAllianceHelpListIDs(
	requests []allianceHelpWireRequest,
	playerID State.PlayerID,
	gameData *GameData.Store,
) []int64 {
	unique := map[int64]struct{}{}
	for _, request := range requests {
		if !otherAllianceHelpRequestActionable(request, playerID, gameData) {
			continue
		}
		unique[int64(request.ListID)] = struct{}{}
	}
	result := make([]int64, 0, len(unique))
	for listID := range unique {
		result = append(result, listID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func otherAllianceHelpRequestActionable(
	request allianceHelpWireRequest,
	playerID State.PlayerID,
	gameData *GameData.Store,
) bool {
	if playerID <= 0 || request.PlayerID <= 0 || request.PlayerID == playerID ||
		request.ListID <= 0 || request.AlreadyConfirmed != 0 {
		return false
	}
	maximum, found := gameData.AllianceHelpMaximumHelpers(request.RequestType)
	return found && int(request.Progress) < maximum
}

func updatePendingOtherAllianceHelpRequest(
	state *State.AllianceHelpRequestState,
	request allianceHelpWireRequest,
	playerID State.PlayerID,
	generation uint64,
	observedAt time.Time,
	gameData *GameData.Store,
) bool {
	if state == nil || generation == 0 || playerID <= 0 || request.PlayerID == playerID || request.ListID <= 0 {
		return false
	}
	changed := false
	if state.OthersObservedGeneration != generation {
		state.PendingOtherListIDs = []int64{}
		state.OthersObservedGeneration = generation
		changed = true
	}
	if otherAllianceHelpRequestActionable(request, playerID, gameData) {
		if addPendingOtherAllianceHelpListID(state, int64(request.ListID)) {
			changed = true
		}
	} else if removePendingOtherAllianceHelpListID(state, int64(request.ListID)) {
		changed = true
	}
	if changed {
		state.OthersObservedAt = observedAt
	}
	return changed
}

func addPendingOtherAllianceHelpListID(state *State.AllianceHelpRequestState, listID int64) bool {
	if state == nil || listID <= 0 {
		return false
	}
	for _, existingID := range state.PendingOtherListIDs {
		if existingID == listID {
			return false
		}
	}
	state.PendingOtherListIDs = append(state.PendingOtherListIDs, listID)
	sort.Slice(state.PendingOtherListIDs, func(left, right int) bool {
		return state.PendingOtherListIDs[left] < state.PendingOtherListIDs[right]
	})
	return true
}

func removePendingOtherAllianceHelpListID(state *State.AllianceHelpRequestState, listID int64) bool {
	if state == nil || listID <= 0 {
		return false
	}
	for index, existingID := range state.PendingOtherListIDs {
		if existingID != listID {
			continue
		}
		state.PendingOtherListIDs = append(
			state.PendingOtherListIDs[:index], state.PendingOtherListIDs[index+1:]...,
		)
		return true
	}
	return false
}

func ownHospitalAllianceHelpProductionIDs(requests []allianceHelpWireRequest, playerID State.PlayerID) []int64 {
	unique := map[int64]struct{}{}
	for _, request := range requests {
		if request.PlayerID == playerID && request.RequestType == 2 && request.Optional.RecruitmentID > 0 {
			unique[request.Optional.RecruitmentID] = struct{}{}
		}
	}
	result := make([]int64, 0, len(unique))
	for productionID := range unique {
		result = append(result, productionID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func ownRecruitmentAllianceHelpCastleIDs(
	requests []allianceHelpWireRequest,
	playerID State.PlayerID,
) []State.CastleID {
	unique := map[State.CastleID]struct{}{}
	for _, request := range requests {
		if request.PlayerID == playerID && request.RequestType == 6 && request.Optional.CastleID > 0 {
			unique[request.Optional.CastleID] = struct{}{}
		}
	}
	result := make([]State.CastleID, 0, len(unique))
	for castleID := range unique {
		result = append(result, castleID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func ownRecruitmentAllianceHelpRequests(
	requests []allianceHelpWireRequest,
	playerID State.PlayerID,
	generation uint64,
	observedAt time.Time,
	gameData *GameData.Store,
	previous State.AllianceHelpRequestState,
) []State.RecruitmentAllianceHelpRequest {
	prior := map[int64]State.RecruitmentAllianceHelpRequest{}
	if previous.OwnRecruitmentObservedGeneration == generation {
		for _, request := range previous.OwnRecruitmentRequests {
			if request.ListID > 0 {
				prior[request.ListID] = request
			}
		}
	}
	maximum, maximumKnown := gameData.AllianceHelpMaximumHelpers(6)
	seen := map[int64]struct{}{}
	result := make([]State.RecruitmentAllianceHelpRequest, 0, len(requests)+len(prior))
	for _, request := range requests {
		listID := int64(request.ListID)
		if request.PlayerID != playerID || request.RequestType != 6 || listID <= 0 ||
			request.Optional.CastleID <= 0 {
			continue
		}
		lifecycle := prior[listID]
		if lifecycle.CastleID != 0 && lifecycle.CastleID != request.Optional.CastleID {
			lifecycle = State.RecruitmentAllianceHelpRequest{}
		}
		lifecycle.ListID = listID
		lifecycle.CastleID = request.Optional.CastleID
		lifecycle.Progress = max(lifecycle.Progress, max(0, int(request.Progress)))
		if maximumKnown {
			lifecycle.MaximumHelpers = maximum
		}
		lifecycle.ObservedAt = observedAt
		lifecycle.RemovedAt = time.Time{}
		if lifecycle.MaximumHelpers > 0 && lifecycle.Progress >= lifecycle.MaximumHelpers &&
			lifecycle.CompletedAt.IsZero() && !observedAt.IsZero() {
			lifecycle.CompletedAt = observedAt
		}
		seen[listID] = struct{}{}
		result = append(result, lifecycle)
	}
	for listID, lifecycle := range prior {
		if _, retained := seen[listID]; retained || lifecycle.CompletedAt.IsZero() || observedAt.IsZero() ||
			!observedAt.Before(lifecycle.CompletedAt.Add(State.RecruitmentAllianceHelpCompletionGrace)) {
			continue
		}
		if lifecycle.RemovedAt.IsZero() {
			lifecycle.RemovedAt = observedAt
		}
		result = append(result, lifecycle)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ListID < result[right].ListID })
	return result
}

func updateOwnRecruitmentAllianceHelpRequest(
	state *State.AllianceHelpRequestState,
	request allianceHelpWireRequest,
	playerID State.PlayerID,
	generation uint64,
	observedAt time.Time,
	gameData *GameData.Store,
) bool {
	listID := int64(request.ListID)
	if state == nil || generation == 0 || request.PlayerID != playerID || request.RequestType != 6 ||
		listID <= 0 || request.Optional.CastleID <= 0 {
		return false
	}
	changed := false
	if state.OwnRecruitmentObservedGeneration != generation {
		state.OwnRecruitmentRequests = []State.RecruitmentAllianceHelpRequest{}
		state.OwnRecruitmentObservedGeneration = generation
		changed = true
	}
	index := -1
	for candidate := range state.OwnRecruitmentRequests {
		if state.OwnRecruitmentRequests[candidate].ListID == listID {
			index = candidate
			break
		}
	}
	lifecycle := State.RecruitmentAllianceHelpRequest{ListID: listID, CastleID: request.Optional.CastleID}
	if index >= 0 {
		lifecycle = state.OwnRecruitmentRequests[index]
		if lifecycle.CastleID != 0 && lifecycle.CastleID != request.Optional.CastleID {
			lifecycle = State.RecruitmentAllianceHelpRequest{ListID: listID}
		}
	}
	previous := lifecycle
	lifecycle.CastleID = request.Optional.CastleID
	lifecycle.Progress = max(lifecycle.Progress, max(0, int(request.Progress)))
	if maximum, found := gameData.AllianceHelpMaximumHelpers(6); found {
		lifecycle.MaximumHelpers = maximum
	}
	lifecycle.ObservedAt = observedAt
	lifecycle.RemovedAt = time.Time{}
	if lifecycle.MaximumHelpers > 0 && lifecycle.Progress >= lifecycle.MaximumHelpers &&
		lifecycle.CompletedAt.IsZero() && !observedAt.IsZero() {
		lifecycle.CompletedAt = observedAt
	}
	if index < 0 {
		state.OwnRecruitmentRequests = append(state.OwnRecruitmentRequests, lifecycle)
		sort.Slice(state.OwnRecruitmentRequests, func(left, right int) bool {
			return state.OwnRecruitmentRequests[left].ListID < state.OwnRecruitmentRequests[right].ListID
		})
		return true
	}
	if lifecycle != previous {
		state.OwnRecruitmentRequests[index] = lifecycle
		changed = true
	}
	return changed
}

func removeOwnRecruitmentAllianceHelpRequest(
	state *State.AllianceHelpRequestState,
	listID int64,
	generation uint64,
	removedAt time.Time,
) bool {
	if state == nil || listID <= 0 || generation == 0 ||
		state.OwnRecruitmentObservedGeneration != generation {
		return false
	}
	for index, request := range state.OwnRecruitmentRequests {
		if request.ListID != listID {
			continue
		}
		if request.CompletedAt.IsZero() {
			state.OwnRecruitmentRequests = append(
				state.OwnRecruitmentRequests[:index], state.OwnRecruitmentRequests[index+1:]...,
			)
			return true
		}
		if request.RemovedAt.IsZero() {
			state.OwnRecruitmentRequests[index].RemovedAt = removedAt
			return true
		}
		return false
	}
	return false
}

func markAllianceHelpQueue(queue *State.ProductionQueue, markAll bool, productionID int64) bool {
	if queue == nil {
		return false
	}
	changed := false
	if queue.Active != nil && (markAll || queue.Active.ProductionID == productionID) && !queue.Active.AllianceHelpRequested {
		queue.Active.AllianceHelpRequested = true
		changed = true
	}
	for index := range queue.Queued {
		if (markAll || queue.Queued[index].ProductionID == productionID) && !queue.Queued[index].AllianceHelpRequested {
			queue.Queued[index].AllianceHelpRequested = true
			changed = true
		}
	}
	return changed
}

// learnObservedStacks raises the per-definition high-water marks with every
// stack visible on the line right now (active production included).
func learnObservedStacks(queue *State.ProductionQueue) {
	learn := func(definitionID, amount int64) {
		if definitionID <= 0 || amount <= 0 || amount <= queue.LearnedStacks[definitionID] {
			return
		}
		if queue.LearnedStacks == nil {
			queue.LearnedStacks = map[int64]int64{}
		}
		queue.LearnedStacks[definitionID] = amount
	}
	if queue.Active != nil {
		learn(int64(queue.Active.Definition.ID), queue.Active.Amount)
	}
	for _, item := range queue.Queued {
		learn(int64(item.Definition.ID), item.Amount)
	}
}

func requestedProductionHelp(queue State.ProductionQueue) map[int64]bool {
	requested := map[int64]bool{}
	if queue.Active != nil && queue.Active.ProductionID > 0 && queue.Active.AllianceHelpRequested {
		requested[queue.Active.ProductionID] = true
	}
	for _, item := range queue.Queued {
		if item.ProductionID > 0 && item.AllianceHelpRequested {
			requested[item.ProductionID] = true
		}
	}
	return requested
}
