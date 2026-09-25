package Automation

import (
	"CitadelDesktop/Server/Localization"
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
	var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.waiting_for_an_active.ef746652", "Waiting for an active Gallantry points booster (boi ID {p0})", Localization.Params{"p0": fmt.Sprintf("%d", GameData.GallantryPointsBoosterID)})
	if snapshot.State.Market.BoostersObservedAt.IsZero() {
		detail = fmt.Sprintf(
			"Waiting for authoritative Gallantry points booster status (boi ID %d)",
			GameData.GallantryPointsBoosterID,
		)
		detailLocalizationMessage = Localization.New("server.automation.waiting_for_authoritative_gallantry.e36e641d", "Waiting for authoritative Gallantry points booster status (boi ID {p0})", Localization.Params{"p0": fmt.Sprintf("%d", GameData.GallantryPointsBoosterID)})
	} else if booster, exists := snapshot.State.Market.Boosters[GameData.GallantryPointsBoosterID]; exists && booster.ActiveAt(snapshot.Now) {
		return nil
	}
	return &Decision{
		Status: "gated", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
		NextCheckAt: snapshot.Now.Add(beriBoosterGateCheckInterval),
	}
}
