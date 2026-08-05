package Automation

import (
	"fmt"
	"time"

	"CitadelDesktop/Server/GameData"
)

const beriBoosterGateCheckInterval = 5 * time.Minute

func beriGallantryBoosterGate(snapshot Snapshot, settings beriSettings) *Decision {
	if !settings.RequireActiveGallantryBooster {
		return nil
	}
	detail := fmt.Sprintf(
		"Waiting for an active Gallantry points booster (boi ID %d)",
		GameData.GallantryPointsBoosterID,
	)
	if snapshot.State.Market.BoostersObservedAt.IsZero() {
		detail = fmt.Sprintf(
			"Waiting for authoritative Gallantry points booster status (boi ID %d)",
			GameData.GallantryPointsBoosterID,
		)
	} else if booster, exists := snapshot.State.Market.Boosters[GameData.GallantryPointsBoosterID]; exists && booster.ActiveAt(snapshot.Now) {
		return nil
	}
	return &Decision{
		Status: "gated", Detail: detail,
		NextCheckAt: snapshot.Now.Add(beriBoosterGateCheckInterval),
	}
}
