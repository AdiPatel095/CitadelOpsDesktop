package App

import (
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const shopPurchaseCounterMaximumAge = 2 * time.Minute

type eventBackedSBPRequest struct {
	PackageID      State.PackageID
	TableID        int64
	Amount         int64
	Stock          int64
	MaxBuyPerClick int64
}

// validateEventBackedSBP binds an SBP to the exact current live route and its
// intended owned destination. Dispatch-ready validation additionally requires
// current-session castle focus and a fresh counter for finite stock.
func validateEventBackedSBP(
	input Intent.PlanningContext,
	source State.CastleState,
	request eventBackedSBPRequest,
	now time.Time,
	dispatchReady bool,
) (int64, error) {
	if input.GameData == nil {
		return 0, fmt.Errorf("official game data is unavailable")
	}
	if request.PackageID <= 0 || request.TableID <= 0 || request.Amount <= 0 || source.ID <= 0 {
		return 0, fmt.Errorf("event-backed shop purchase has an invalid package, table, amount, or destination")
	}
	route, active := input.State.ActiveShopForPackage(request.PackageID, now)
	if !active {
		return 0, fmt.Errorf("%w: package %d has no current live shop advertisement", Intent.ErrPlanStale, request.PackageID)
	}
	if route.EventID != request.TableID {
		return 0, fmt.Errorf(
			"%w: package %d is advertised by shop table %d, not requested table %d",
			Intent.ErrPlanStale, request.PackageID, route.EventID, request.TableID,
		)
	}
	if err := input.GameData.ValidateEventShopDestination(
		int64(request.PackageID), route.EventID, int64(source.KingdomID), source.SlotType,
	); err != nil {
		return 0, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	if request.MaxBuyPerClick > 0 && request.Amount > request.MaxBuyPerClick {
		return 0, fmt.Errorf("package %d amount exceeds per-click maximum %d", request.PackageID, request.MaxBuyPerClick)
	}
	if dispatchReady {
		protocol := input.ProtocolContext
		if !source.Focused || protocol.SessionGeneration == 0 || protocol.ConnectionGeneration == 0 ||
			protocol.SessionGeneration != input.State.Session.Generation ||
			protocol.ConnectionGeneration != input.State.Session.ConnectionGeneration ||
			protocol.FocusedCastleID != source.ID || protocol.FocusSubcontext != State.FocusSubcontextCastle ||
			protocol.FocusEpoch == 0 {
			return 0, fmt.Errorf(
				"%w: package %d lost current-session castle focus for destination %d",
				Intent.ErrPlanStale, request.PackageID, source.ID,
			)
		}
	}
	if request.Stock <= 0 {
		return 0, nil
	}
	offers, observedAt, found := input.State.ConstructionOffersFor(source.ID, source.KingdomID)
	if !found || observedAt.IsZero() || observedAt.After(now) || now.Sub(observedAt) >= shopPurchaseCounterMaximumAge {
		if dispatchReady {
			return 0, fmt.Errorf(
				"%w: package purchase counters are not fresh for castle %d",
				Intent.ErrPlanStale, source.ID,
			)
		}
		return 0, nil
	}
	return offers[request.PackageID], nil
}
