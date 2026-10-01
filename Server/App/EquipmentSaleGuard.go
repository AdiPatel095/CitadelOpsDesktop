package App

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

const equipmentSaleGuardAction = "equipment.sale.guard"

// equipmentSaleGuardRequest identifies one SEQ or SGE step of a frozen sale
// plan. Exactly one of EquipmentID or GemID is set.
type equipmentSaleGuardRequest struct {
	EquipmentID   State.EquipmentInstanceID `json:"equipmentId,omitempty"`
	GemID         int64                     `json:"gemId,omitempty"`
	RelicGem      bool                      `json:"relicGem,omitempty"`
	Category      string                    `json:"category"`
	SellLookItems bool                      `json:"sellLookItems,omitempty"`
	SellPost2026  bool                      `json:"sellPost2026,omitempty"`
	KeepStars     int                       `json:"keepStars,omitempty"`
}

func withEquipmentSaleGuard(step Intent.Step, request equipmentSaleGuardRequest) Intent.Step {
	arguments, _ := json.Marshal(request)
	step.FinalDispatchAction = equipmentSaleGuardAction
	step.FinalDispatchArguments = arguments
	return step
}

// guardEquipmentSale runs immediately before each SEQ/SGE reaches the
// transport. The frozen item must still be an unworn storage item matching the
// sale's protected boundary, and it must not already have a sale dispatched
// after the storage snapshot the plan was built from (an unresolved SEQ, for
// example a timeout, is never re-dispatched until storage is refreshed).
func (application *Application) guardEquipmentSale(_ context.Context, arguments json.RawMessage) error {
	var request equipmentSaleGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("equipment state is unavailable"), Localization.New("server.app.equipment_state_is_unavailable.59c90ca3", "equipment state is unavailable", nil))
	}
	return validateEquipmentSaleDispatch(application.State.ReadOnlyView(), request)
}

func validateEquipmentSaleDispatch(gameState State.GameState, request equipmentSaleGuardRequest) error {
	if request.EquipmentID > 0 {
		item, exists := gameState.Inventory.Equipment[request.EquipmentID]
		if !exists || item.WearerKind != "" || !equipmentMatchesSale(
			item, request.Category, request.SellLookItems, request.SellPost2026, request.KeepStars,
		) {
			return Localization.WithError(fmt.Errorf(
				"%w: equipment %d is no longer an unworn storage item eligible for this sale",
				Intent.ErrPlanStale, request.EquipmentID,
			), Localization.New("server.app.intent_plan_became_stale.388ea395", "intent plan became stale before dispatch: equipment {p1} is no longer an unworn storage item eligible for this sale", Localization.Params{"p1": fmt.Sprintf("%d", request.EquipmentID)}))
		}
		storageAt := storageSnapshotAt(gameState, "gei")
		for _, pending := range State.PendingCommandRequests(&gameState, "seq") {
			if pending.EquipmentID == request.EquipmentID && !pending.SentAt.Before(storageAt) {
				return Localization.WithError(fmt.Errorf(
					"%w: equipment %d already has an unresolved sale", Intent.ErrPlanStale, request.EquipmentID,
				), Localization.New("server.app.intent_plan_became_stale.fb9e1e91", "intent plan became stale before dispatch: equipment {p1} already has an unresolved sale", Localization.Params{"p1": fmt.Sprintf("%d", request.EquipmentID)}))
			}
		}
		return nil
	}
	if request.GemID <= 0 {
		return Localization.WithError(fmt.Errorf("equipment sale guard requires an equipment or gem id"), Localization.New("server.app.equipment_sale_guard_requires.3a0db83d", "equipment sale guard requires an equipment or gem id", nil))
	}
	available := int64(0)
	if request.RelicGem {
		gem, exists := gameState.Inventory.Gems[State.GemInstanceID(request.GemID)]
		if exists && gem.WearerKind == "" && gemMatchesSale(gem, request.Category, request.KeepStars) {
			available = 1
		}
	} else if EquipmentDomain.MatchesNonRelicGemStackSale(State.GemID(request.GemID), request.SellPost2026) {
		available = gameState.Inventory.GemStacks[State.GemID(request.GemID)]
	}
	if available <= 0 {
		return Localization.WithError(fmt.Errorf(
			"%w: gem %d is no longer in storage for this sale", Intent.ErrPlanStale, request.GemID,
		), Localization.New("server.app.intent_plan_became_stale.d99b47c6", "intent plan became stale before dispatch: gem {p1} is no longer in storage for this sale", Localization.Params{"p1": fmt.Sprintf("%d", request.GemID)}))
	}
	storageAt := storageSnapshotAt(gameState, "ggm")
	unresolved := int64(0)
	for _, pending := range State.PendingCommandRequests(&gameState, "sge") {
		if pending.GemID == request.GemID && pending.RelicGem == request.RelicGem && !pending.SentAt.Before(storageAt) {
			unresolved++
		}
	}
	if unresolved >= available {
		return Localization.WithError(fmt.Errorf(
			"%w: gem %d already has an unresolved sale", Intent.ErrPlanStale, request.GemID,
		), Localization.New("server.app.intent_plan_became_stale.fbfef31a", "intent plan became stale before dispatch: gem {p1} already has an unresolved sale", Localization.Params{"p1": fmt.Sprintf("%d", request.GemID)}))
	}
	return nil
}

func storageSnapshotAt(gameState State.GameState, opcode string) time.Time {
	observation, found := gameState.Observations[opcode]
	if !found {
		return time.Time{}
	}
	return observation.SuccessfulInboundAt()
}
