package Ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// coolingDownResponseCode is the official client enum COOLING_DOWN (ggs.dll).
const coolingDownResponseCode = 95

// Only Auto Fortress (ABI/CRA) and Auto Storm (CRA) consume the registry.
func attackRejectionTargetType(typeID int) bool {
	return typeID == State.MapTypeKingdomFortress || typeID == stormIslandMapTypeID || typeID == stormFortMapTypeID
}

// reduceAttackTargetCommand captures the target of an outbound ABI/CRA so the
// reply (which carries no target) can be attributed.
func reduceAttackTargetCommand(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &body); err != nil {
		return nil, false, fmt.Errorf("decode %s attack target: %w", frame.Opcode, err)
	}
	if _, hasX := body["TX"]; !hasX {
		return nil, false, nil
	}
	if _, hasY := body["TY"]; !hasY {
		return nil, false, nil
	}
	if !gameState.RecordPendingCommandRequest(State.PendingCommandRequest{
		Opcode: frame.Opcode, OperationID: frame.CausationOperationID, SentAt: frame.ReceivedAt,
		KingdomID: State.KingdomID(rawMapInt(body, "KID")), TargetX: rawMapInt(body, "TX"), TargetY: rawMapInt(body, "TY"),
	}) {
		return nil, false, nil
	}
	return []string{"command-context"}, true, nil
}

// reduceAttackTargetResponse records ABI/CRA 95 for a fortress or Storm target
// and clears the target's record after a confirmed CRA 0. Every reply resolves
// its pending identity; an uncorrelated reply records nothing (fail closed:
// the lane lock still applies).
func reduceAttackTargetResponse(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil {
		return nil, false, nil
	}
	before := len(gameState.CommandContext.PendingRequests)
	request, found := gameState.TakePendingCommandRequest(frame.Opcode, frame.CausationOperationID, frame.ReceivedAt)
	changed := before != len(gameState.CommandContext.PendingRequests)
	domains := []string{"command-context"}
	if !found {
		return domains, changed, nil
	}
	typeID, observation := attackRejectionTarget(gameState, request)
	switch {
	case frameResponseCodeIs(frame, coolingDownResponseCode) && attackRejectionTargetType(typeID):
		personal := typeID == State.MapTypeKingdomFortress && gameState.Player.ID > 0 &&
			observation.FortressDefeaterPlayerID == gameState.Player.ID
		if _, recorded := State.RecordAttackTargetRejection(gameState, State.AttackTargetRejection{
			KingdomID: request.KingdomID, TargetTypeID: typeID, X: request.TargetX, Y: request.TargetY,
			Opcode: frame.Opcode, Code: coolingDownResponseCode, OperationID: request.OperationID,
			ObservedAt: frame.ReceivedAt, Personal: personal,
		}); recorded {
			changed = true
			domains = append(domains, "attack-analytics")
		}
	case frame.Opcode == "cra" && frameResponseCodeIs(frame, 0):
		if State.ClearAttackTargetRejection(gameState, request.KingdomID, typeID, request.TargetX, request.TargetY) {
			changed = true
			domains = append(domains, "attack-analytics")
		}
	}
	return domains, changed, nil
}

// attackRejectionTarget resolves the target type from the attack dialog when
// it names the same coordinates, else from the current map observation.
func attackRejectionTarget(gameState *State.GameState, request State.PendingCommandRequest) (int, State.MapObservation) {
	observation, found := gameState.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	dialog := gameState.AttackDialog
	if dialog.KingdomID == request.KingdomID && dialog.Target.X == request.TargetX && dialog.Target.Y == request.TargetY &&
		dialog.Target.TypeID > 0 {
		return dialog.Target.TypeID, observation
	}
	if found {
		return observation.TypeID, observation
	}
	return 0, observation
}

// reducePackagePurchaseDispatch records every outbound event-package SBP so
// that purchase counters observed before it cannot authorize another purchase
// (a timeout never proves the outcome). The Storm Luna shop has its own
// accounting and is excluded.
func reducePackagePurchaseDispatch(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		ProductID wireInt64 `json:"PID"`
		TableID   wireInt64 `json:"TID"`
		Amount    wireInt64 `json:"AMT"`
		KingdomID wireInt64 `json:"KID"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode package purchase dispatch: %w", err)
	}
	if payload.ProductID <= 0 ||
		payload.KingdomID == GameData.StormKingdomID && payload.TableID == GameData.StormLunaShopTableID {
		return nil, false, nil
	}
	if !gameState.MarkInventoryPackagePurchaseDispatched(State.PackagePurchaseDispatch{
		PackageID: State.PackageID(payload.ProductID), Amount: int64(payload.Amount),
		OperationID: frame.CausationOperationID, SentAt: frame.ReceivedAt,
	}) {
		return nil, false, nil
	}
	return []string{"inventory", "construction-offers"}, true, nil
}
