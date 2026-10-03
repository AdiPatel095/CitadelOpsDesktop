package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestStormPackageCapPolicyStatusAndOtherPackages(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	castle := State.CastleState{ID: 910040, KingdomID: 4, Resources: map[State.ResourceID]State.ResourceBalance{GameData.StormAquamarineID: {Amount: 1000}}}
	state.Castles[castle.ID] = castle
	state.Inventory.ConstructionOffersCastleID = castle.ID
	state.Inventory.ConstructionOffersKingdomID = 4
	state.Inventory.ConstructionOffersObservedAt = now
	state.BlockStormPackage(castle.ID, -1, 3119, 4, now)
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"packages":[{"packageID":3119,"comment1":"Fixture capped","comment2":"Luna's trade boat","packageType":"resource","packagePriceAquamarine":10,"stock":4},{"packageID":245,"comment1":"Fixture eligible","comment2":"Luna's trade boat","packageType":"resource","packagePriceAquamarine":10,"stock":4}]}`), GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	settings := defaultAutoStormSettings()
	settings.Aquamarine.Purchases = []autoStormShopPurchase{{PackageID: 3119, TargetPurchases: 4}}
	snapshot := Snapshot{State: state, GameData: data, Now: now}
	first, complete, _, err := evaluateAutoStormShop(snapshot, settings, castle, map[string]float64{})
	if err != nil || complete || first == nil || first.Request != nil || first.Status != "waiting" || first.DetailDescriptor == nil || first.DetailDescriptor.Key != "server.storm.package_cap_blocked" {
		t.Fatalf("cap status=%+v complete=%v err=%v", first, complete, err)
	}
	second, _, _, _ := evaluateAutoStormShop(snapshot, settings, castle, map[string]float64{})
	if first.Detail != second.Detail || first.DetailDescriptor.FallbackText != first.Detail {
		t.Fatal("cap note is not stable/bound for deduplication")
	}
	settings.Aquamarine.Purchases = append(settings.Aquamarine.Purchases, autoStormShopPurchase{PackageID: 245, TargetPurchases: 1})
	decision, _, _, err := evaluateAutoStormShop(snapshot, settings, castle, map[string]float64{})
	if err != nil || decision == nil || decision.Request == nil {
		t.Fatalf("eligible package blocked: %+v err=%v", decision, err)
	}
	if decision.DetailDescriptor == nil || len(decision.DetailDescriptor.Context) != 1 || decision.DetailDescriptor.Context[0].Key != "server.storm.package_cap_blocked" {
		t.Fatal("eligible work lost the skipped-package note")
	}
	var args struct {
		Purchases []autoStormShopPurchaseLine `json:"purchases"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if len(args.Purchases) != 1 || args.Purchases[0].ProductID != 245 {
		t.Fatalf("unsafe purchases=%+v", args.Purchases)
	}
}
