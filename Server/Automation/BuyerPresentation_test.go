package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
	"strings"
	"testing"
)

func TestBuyerSpecialistWholeVariantsAndCopy(t *testing.T) {
	for _, id := range []int{0, 1, 2, 3, 4, 5, 6, 8, 10} {
		specialist, ok := GameData.AutoBuyerSpecialistByID(id)
		if !ok {
			t.Fatal(id)
		}
		for _, variant := range []string{"renew", "balance", "cost", "floor", "ceiling"} {
			params := Localization.Params{"days": 14, "cost": int64(625), "minimum": 14}
			message := buyerSpecialistDescriptor(variant, specialist, params)
			if message == nil || strings.Contains(message.Fallback, "{role}") || message.Params["days"] != 14 {
				t.Fatalf("%d/%s: %+v", id, variant, message)
			}
			message.Params["days"] = 99
			if params["days"] != 14 {
				t.Fatal("descriptor modified caller parameters")
			}
		}
	}
	if buyerSpecialistDescriptor("renew", GameData.AutoBuyerSpecialist{ID: 999}, nil) != nil {
		t.Fatal("unknown role invented")
	}
}

func TestBuyerPackageIdentityAndCurrencyProvenance(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[{"wodID":489,"name":"elitecrossbowman"}],"resources":[{"resourceID":2,"name":"currency2","JSONKey":"C2"}]}`), GameData.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"elitecrossbowman_name":"Veteran Crossbowman","gold":"Rubies"}`), GameData.LanguageMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{GameData: store, Language: language}
	product := GameData.AutoBuyerPackage{PackageID: 102, PackageType: "soldier", UnitID: 489, UnitAmount: 50, Name: "English bundle text", Price: GameData.AutoBuyerPrice{ResourceID: 2, Name: "currency2", Amount: 10}}
	message := buyerPackagePurchaseDescriptor(snapshot, product, 3, false)
	if message == nil || message.GameParams["unit"].Key != "elitecrossbowman_name" || message.GameParams["currency"].Key != "gold" || message.Params["amount"] != int64(3) || message.Params["cost"] != int64(30) {
		t.Fatalf("atomic package: %+v", message)
	}
	message.GameParams["unit"] = Localization.GameParam{Key: "mutated"}
	again := buyerPackagePurchaseDescriptor(snapshot, product, 3, false)
	if again.GameParams["unit"].Key == "mutated" {
		t.Fatal("shared descriptor state")
	}
	product.PackageType = "bundle"
	message = buyerPackagePurchaseDescriptor(snapshot, product, 3, false)
	if message == nil || message.Params["packageID"] != "102" || message.GameParams["unit"].Key != "" || strings.Contains(message.Fallback, "English bundle") {
		t.Fatalf("compound package identity: %+v", message)
	}
	product.PackageType = "soldier"
	product.UnitID = 999
	message = buyerPackagePurchaseDescriptor(snapshot, product, 3, false)
	if message == nil || message.Params["packageID"] != "102" {
		t.Fatalf("missing official noun lacks ID variant: %+v", message)
	}
	snapshot.Language = nil
	if buyerPackagePurchaseDescriptor(snapshot, product, 3, false) != nil {
		t.Fatal("unknown price hidden inside partial translation")
	}
}
