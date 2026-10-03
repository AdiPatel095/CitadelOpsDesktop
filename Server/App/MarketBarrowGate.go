package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"fmt"
)

func marketBarrowsRequired(input Intent.PlanningContext, source State.CastleID, amount int64) (int, error) {
	row, found := input.State.Market.Castles[source]
	if !found || input.GameData == nil || !input.State.Market.CaravanLevelLoaded || amount <= 0 {
		return 0, marketBarrowPlanningUnavailable(source)
	}
	effects := make([]GameData.MarketEffect, 0, len(row.AreaEffects))
	for _, effect := range row.AreaEffects {
		effects = append(effects, GameData.MarketEffect{EffectID: effect.EffectID, Values: effect.Values})
	}
	capacity, err := input.GameData.MarketCapacity(input.State.Market.CaravanLevel, effects)
	if err != nil || capacity.CapacityPerBarrow <= 0 {
		return 0, marketBarrowPlanningUnavailable(source)
	}
	perBarrow := int64(capacity.CapacityPerBarrow)
	required := amount / perBarrow
	if amount%perBarrow != 0 {
		required++
	}
	return int(required), nil
}

// Commit A uses the existing message; commit B adopts CIT-118's typed wait.
func marketBarrowPlanningUnavailable(source State.CastleID) error {
	return Localization.WithError(fmt.Errorf("source castle %d has no observed available market barrows", source), Localization.New("server.app.source_castle_p_has.f0ba5631", "source castle {p0} has no observed available market barrows", Localization.Params{"p0": fmt.Sprint(source)}))
}
