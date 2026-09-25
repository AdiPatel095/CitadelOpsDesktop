package App

import (
	"CitadelDesktop/Server/Localization"
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
		return 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if request.PackageID <= 0 || request.TableID <= 0 || request.Amount <= 0 || source.ID <= 0 {
		return 0, Localization.WithError(fmt.Errorf("event-backed shop purchase has an invalid package, table, amount, or destination"), Localization.New("server.app.event_backed_shop_purchase.564a69a1", "event-backed shop purchase has an invalid package, table, amount, or destination", nil))
	}
	route, active := input.State.ActiveShopForPackage(request.PackageID, now)
	if !active {
		return 0, Localization.WithError(fmt.Errorf("%w: package %d has no current live shop advertisement", Intent.ErrPlanStale, request.PackageID), Localization.New("server.app.intent_plan_became_stale.0195a075", "intent plan became stale before dispatch: package {p1} has no current live shop advertisement", Localization.Params{"p1": fmt.Sprintf("%d", request.PackageID)}))
	}
	if route.EventID != request.TableID {
		return 0, Localization.WithError(fmt.Errorf(
			"%w: package %d is advertised by shop table %d, not requested table %d",
			Intent.ErrPlanStale, request.PackageID, route.EventID, request.TableID,
		), Localization.New("server.app.intent_plan_became_stale.f3c23ec9", "intent plan became stale before dispatch: package {p1} is advertised by shop table {p2}, not requested table {p3}", Localization.Params{"p1": fmt.Sprintf("%d", request.PackageID), "p2": fmt.Sprintf("%d", route.EventID), "p3": fmt.Sprintf("%d", request.TableID)}))
	}
	if err := input.GameData.ValidateEventShopDestination(
		int64(request.PackageID), route.EventID, int64(source.KingdomID), source.SlotType,
	); err != nil {
		return 0, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	if request.MaxBuyPerClick > 0 && request.Amount > request.MaxBuyPerClick {
		return 0, Localization.WithError(fmt.Errorf("package %d amount exceeds per-click maximum %d", request.PackageID, request.MaxBuyPerClick), Localization.New("server.app.package_p_amount_exceeds.d71536f5", "package {p0} amount exceeds per-click maximum {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": request.MaxBuyPerClick}))
	}
	if dispatchReady {
		protocol := input.ProtocolContext
		if !source.Focused || protocol.SessionGeneration == 0 || protocol.ConnectionGeneration == 0 ||
			protocol.SessionGeneration != input.State.Session.Generation ||
			protocol.ConnectionGeneration != input.State.Session.ConnectionGeneration ||
			protocol.FocusedCastleID != source.ID || protocol.FocusSubcontext != State.FocusSubcontextCastle ||
			protocol.FocusEpoch == 0 {
			return 0, Localization.WithError(fmt.Errorf(
				"%w: package %d lost current-session castle focus for destination %d",
				Intent.ErrPlanStale, request.PackageID, source.ID,
			), Localization.New("server.app.intent_plan_became_stale.28e62842", "intent plan became stale before dispatch: package {p1} lost current-session castle focus for destination {p2}", Localization.Params{"p1": fmt.Sprintf("%d", request.PackageID), "p2": fmt.Sprintf("%d", source.ID)}))
		}
	}
	if request.Stock <= 0 {
		return 0, nil
	}
	offers, observedAt, found := input.State.ConstructionOffersFor(source.ID, source.KingdomID)
	if !found || observedAt.IsZero() || observedAt.After(now) || now.Sub(observedAt) >= shopPurchaseCounterMaximumAge {
		if dispatchReady {
			return 0, Localization.WithError(fmt.Errorf(
				"%w: package purchase counters are not fresh for castle %d",
				Intent.ErrPlanStale, source.ID,
			), Localization.New("server.app.intent_plan_became_stale.ad53b564", "intent plan became stale before dispatch: package purchase counters are not fresh for castle {p1}", Localization.Params{"p1": fmt.Sprintf("%d", source.ID)}))
		}
		return 0, nil
	}
	return offers[request.PackageID], nil
}
