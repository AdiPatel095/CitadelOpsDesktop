package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models"
)

func TestApplyMarketInfoAndBooster(t *testing.T) {
	gs := Models.GetGameState()
	gs.Reset()
	gs.Castle.MainCastle.Aid = 15246649

	if !ApplyMarketBoosterFromJSON(`{"BO":[{"L":21,"ID":11,"RT":364738326}]}`) {
		t.Fatal("boi should change unloaded market booster state")
	}
	if !ApplyMarketInfoFromJSON(`{"C":[{"CID":15246649,"KID":0,"TC":125,"AC":123,"G":569960,"AE":[[90,[15.0],"RH"],[90,[10.0],"PS"]]}]}`) {
		t.Fatal("cmi should change empty market state")
	}

	snapshot := gs.MarketTransportSnapshot()
	if !snapshot.CaravanLevelLoaded || snapshot.CaravanLevel != 21 {
		t.Fatalf("caravan level = %d loaded=%v, want 21 true", snapshot.CaravanLevel, snapshot.CaravanLevelLoaded)
	}
	if len(snapshot.Castles) != 1 || snapshot.Castles[0].TotalBarrows != 125 || snapshot.Castles[0].AvailableBarrows != 123 {
		t.Fatalf("market castles = %#v", snapshot.Castles)
	}
	if gs.Castle.MainCastle.Amount.GlassAmount != 569960 || gs.Castle.MainCastle.MarketBarrowsAvailable != 123 {
		t.Fatalf("main market/resource state not applied: %#v", gs.Castle.MainCastle)
	}
}
