package GameData

import (
	"CitadelDesktop/Server/Localization"
	"fmt"
	"math"
	"sort"
	"time"

	"CitadelDesktop/Server/State"
)

// AutoBuyerFeastSource is a fully observed positive-net food source. Callers
// use FoodAmount for deterministic ranking and NetFoodPerHour for user-facing
// evidence; neither capacity nor a saved feast source participates.
type AutoBuyerFeastSource struct {
	Castle         State.CastleState
	FoodAmount     int64
	NetFoodPerHour float64
}

// AutoBuyerFeastSourceStatus explains why automatic selection cannot yet
// produce a source. StaleCandidateIDs are sorted so policy refreshes make
// bounded, deterministic progress.
type AutoBuyerFeastSourceStatus struct {
	CandidateIDs             []State.CastleID
	StaleCandidateIDs        []State.CastleID
	ContextStaleCandidateIDs []State.CastleID
	BalanceStaleCandidateIDs []State.CastleID
	EligibleCount            int
}

func (store *Store) SelectAutoBuyerFeastSource(
	gameState State.GameState,
	now time.Time,
	maxAge time.Duration,
) (AutoBuyerFeastSource, AutoBuyerFeastSourceStatus, error) {
	status := AutoBuyerFeastSourceStatus{}
	if store == nil {
		return AutoBuyerFeastSource{}, status, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.game_data.buyer_unavailable", "Official game data is unavailable", nil))
	}
	resourceIDs, err := store.FoodResourceIDs()
	if err != nil {
		return AutoBuyerFeastSource{}, status, err
	}
	foodID := resourceIDs["F"]
	ids := make([]State.CastleID, 0, len(gameState.Castles))
	for id := range gameState.Castles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })

	eligible := make([]AutoBuyerFeastSource, 0, len(ids))
	for _, id := range ids {
		castle := gameState.Castles[id]
		if !AutoBuyerFeastCastleUsable(gameState, id, castle) {
			continue
		}
		status.CandidateIDs = append(status.CandidateIDs, id)
		contextStale := !autoBuyerFeastObservationFresh(castle.ContextSnapshotObservedAt, gameState.Session.ChangedAt, now, maxAge)
		balanceStale := !autoBuyerFeastObservationFresh(castle.FoodBalanceObservedAt, gameState.Session.ChangedAt, now, maxAge) ||
			!autoBuyerFeastObservationFresh(castle.FoodEconomyObservedAt, gameState.Session.ChangedAt, now, maxAge)
		if contextStale || balanceStale {
			status.StaleCandidateIDs = append(status.StaleCandidateIDs, id)
			if contextStale {
				status.ContextStaleCandidateIDs = append(status.ContextStaleCandidateIDs, id)
			}
			if balanceStale {
				status.BalanceStaleCandidateIDs = append(status.BalanceStaleCandidateIDs, id)
			}
			continue
		}
		balance, found := castle.Resources[foodID]
		if !found || math.IsNaN(balance.Amount) || math.IsInf(balance.Amount, 0) || balance.Amount < 0 || balance.Amount >= float64(math.MaxInt64) {
			continue
		}
		consumption, estimateErr := store.EstimateFoodConsumption(castle)
		if estimateErr != nil {
			continue
		}
		rate, found := consumption.ByResource[foodID]
		if !found || rate.NetPerHour == nil || math.IsNaN(*rate.NetPerHour) || math.IsInf(*rate.NetPerHour, 0) || *rate.NetPerHour <= 0 {
			continue
		}
		eligible = append(eligible, AutoBuyerFeastSource{
			Castle: castle, FoodAmount: int64(math.Floor(balance.Amount)), NetFoodPerHour: *rate.NetPerHour,
		})
	}
	status.EligibleCount = len(eligible)
	if len(status.StaleCandidateIDs) > 0 || len(eligible) == 0 {
		return AutoBuyerFeastSource{}, status, nil
	}
	sort.Slice(eligible, func(left, right int) bool {
		if eligible[left].FoodAmount != eligible[right].FoodAmount {
			return eligible[left].FoodAmount > eligible[right].FoodAmount
		}
		return eligible[left].Castle.ID < eligible[right].Castle.ID
	})
	return eligible[0], status, nil
}

func AutoBuyerFeastCastleUsable(gameState State.GameState, key State.CastleID, castle State.CastleState) bool {
	return key > 0 && castle.ID == key && castle.KingdomID >= 0 && castle.SlotType > 0 &&
		!State.CastleFocusKnownUnavailable(gameState, castle)
}

func autoBuyerFeastObservationFresh(observedAt, sessionChangedAt, now time.Time, maxAge time.Duration) bool {
	return !now.IsZero() && maxAge > 0 && !observedAt.IsZero() && !observedAt.After(now) &&
		(sessionChangedAt.IsZero() || !observedAt.Before(sessionChangedAt)) && now.Sub(observedAt) < maxAge
}
