package Ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// Response codes whose reconciliation is owned by these reducers. They only
// reconcile state; the lane lock for every nonzero reply stays in the engine.
const (
	packageNotHelpableResponseCode = 269 // official client PACKAGE_NOT_HELPABLE
	recruitmentListHelpType        = 6   // official client ALLIANCE_HELP_RECRUITMENT_LIST
)

func frameResponseCodeIs(frame Protocol.Frame, code int) bool {
	return frame.ResponseCode != nil && *frame.ResponseCode == code
}

// reduceEquipmentSaleCommand captures an outbound SEQ/SGE identity and marks
// storage snapshots received before this send as stale, whatever the outcome.
func reduceEquipmentSaleCommand(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	request := State.PendingCommandRequest{
		Opcode: frame.Opcode, OperationID: frame.CausationOperationID, SentAt: frame.ReceivedAt,
	}
	switch frame.Opcode {
	case "seq":
		var command struct {
			EquipmentID wireInt64 `json:"EID"`
		}
		if err := json.Unmarshal(frame.Payload, &command); err != nil {
			return nil, false, fmt.Errorf("decode equipment sale command: %w", err)
		}
		if command.EquipmentID <= 0 {
			return nil, false, nil
		}
		request.EquipmentID = State.EquipmentInstanceID(command.EquipmentID)
	case "sge":
		var command struct {
			GemID    wireInt64 `json:"GID"`
			RelicGem wireInt64 `json:"RGEM"`
		}
		if err := json.Unmarshal(frame.Payload, &command); err != nil {
			return nil, false, fmt.Errorf("decode gem sale command: %w", err)
		}
		if command.GemID <= 0 {
			return nil, false, nil
		}
		request.GemID = int64(command.GemID)
		request.RelicGem = command.RelicGem == 1
	default:
		return nil, false, nil
	}
	recorded := gameState.RecordPendingCommandRequest(request)
	mutated := gameState.MarkInventoryEquipmentMutated(frame.ReceivedAt)
	if !recorded && !mutated {
		return nil, false, nil
	}
	return []string{"command-context", "inventory"}, true, nil
}

// reduceEquipmentSaleResponse removes exactly the sold instance after a
// correlated SEQ 0. Any other code (including 214) or an uncorrelated reply
// only resolves the pending identity; it never infers a sale.
func reduceEquipmentSaleResponse(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil {
		return nil, false, nil
	}
	before := len(gameState.CommandContext.PendingRequests)
	request, found := gameState.TakePendingCommandRequest("seq", frame.CausationOperationID, frame.ReceivedAt)
	changed := before != len(gameState.CommandContext.PendingRequests)
	domains := []string{"command-context"}
	if found && frameResponseCodeIs(frame, 0) && request.EquipmentID > 0 {
		if _, exists := gameState.Inventory.Equipment[request.EquipmentID]; exists {
			// Gems carried by the sold instance are reconciled by the next ggm.
			gameState.DeleteInventoryEquipment(request.EquipmentID)
			changed = true
			domains = append(domains, "inventory", "equipment")
		}
	}
	return domains, changed, nil
}

// reduceGemSaleResponse resolves a pending SGE identity. Stack counts and
// relic gem instances are reconciled by the next ggm snapshot.
func reduceGemSaleResponse(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil {
		return nil, false, nil
	}
	before := len(gameState.CommandContext.PendingRequests)
	gameState.TakePendingCommandRequest("sge", frame.CausationOperationID, frame.ReceivedAt)
	return []string{"command-context"}, before != len(gameState.CommandContext.PendingRequests), nil
}

// reduceAllianceHelpRequestCommand captures an outbound AHR with the castle
// focused when it was sent. Recruitment-list help is castle scoped.
func reduceAllianceHelpRequestCommand(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var command struct {
		ID   wireInt64 `json:"ID"`
		Type wireInt64 `json:"T"`
	}
	if err := json.Unmarshal(frame.Payload, &command); err != nil {
		return nil, false, fmt.Errorf("decode alliance help request command: %w", err)
	}
	castleID, _, _ := focusedCastle(gameState)
	if !gameState.RecordPendingCommandRequest(State.PendingCommandRequest{
		Opcode: "ahr", OperationID: frame.CausationOperationID, SentAt: frame.ReceivedAt,
		HelpType: int(command.Type), HelpID: int64(command.ID), CastleID: castleID,
	}) {
		return nil, false, nil
	}
	return []string{"command-context"}, true, nil
}

// reduceAllianceHelpRequestResponse records AHR 269 for the castle whose
// recruitment-list request it answers. The 273 path and hospital help are
// unaffected; every reply resolves its pending identity.
func reduceAllianceHelpRequestResponse(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil {
		return nil, false, nil
	}
	before := len(gameState.CommandContext.PendingRequests)
	request, found := gameState.TakePendingCommandRequest("ahr", frame.CausationOperationID, frame.ReceivedAt)
	changed := before != len(gameState.CommandContext.PendingRequests)
	domains := []string{"command-context"}
	if found && frameResponseCodeIs(frame, packageNotHelpableResponseCode) &&
		request.HelpType == recruitmentListHelpType && request.CastleID > 0 &&
		State.RecordRecruitmentHelpIneligibility(gameState, request.CastleID, request.OperationID, frame.ReceivedAt) {
		changed = true
		domains = append(domains, "alliance-help")
	}
	return domains, changed, nil
}

// storageSnapshotSaleOpcodes maps a storage refresh to the sale it resolves.
var storageSnapshotSaleOpcodes = map[string]string{"gei": "seq", "ggm": "sge"}

// reduceStorageSnapshotCommand records an outbound GEI/GGM so its reply can be
// tied to the moment the refresh was requested.
func reduceStorageSnapshotCommand(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if _, tracked := storageSnapshotSaleOpcodes[frame.Opcode]; !tracked ||
		!gameState.RecordPendingCommandRequest(State.PendingCommandRequest{
			Opcode: frame.Opcode, OperationID: frame.CausationOperationID, SentAt: frame.ReceivedAt,
		}) {
		return nil, false, nil
	}
	return []string{"command-context"}, true, nil
}

// reduceStorageSnapshotSaleResolution drops unresolved SEQ/SGE identities that
// were sent before the answered GEI/GGM request: the game processes one
// connection in order, so the snapshot already reflects those sales. This
// stops a lost sale reply from shifting FIFO attribution onto a later sale.
func reduceStorageSnapshotSaleResolution(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	saleOpcode, tracked := storageSnapshotSaleOpcodes[frame.Opcode]
	if !tracked || frame.ResponseCode == nil {
		return nil, false, nil
	}
	before := len(gameState.CommandContext.PendingRequests)
	request, found := gameState.TakePendingCommandRequest(frame.Opcode, frame.CausationOperationID, frame.ReceivedAt)
	if found && frameResponseCodeIs(frame, 0) {
		gameState.DropPendingCommandRequestsBefore(saleOpcode, request.SentAt)
	}
	return []string{"command-context"}, before != len(gameState.CommandContext.PendingRequests), nil
}
