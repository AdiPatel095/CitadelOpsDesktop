package GameData

import (
	"CitadelDesktop/Server/Localization"
	"testing"
)

func TestAutoBuyerCapabilityProjectionAndIsolation(t *testing.T) {
	store, err := DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"packages":[],"currencies":[],"resources":[{"resourceID":2,"JSONKey":"C2","name":"currency2"}],"feasts":[{"feastID":1,"duration":21600,"productionBoost":120,"costC2":250}]}`), SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.AutoBuyerCatalog()
	if err != nil {
		t.Fatal(err)
	}
	feast, found := store.AutoBuyerFeast(1)
	if !found {
		t.Fatal("missing ruby feast")
	}
	caps := []AutoBuyerCapability{catalog.TimedOffers, catalog.FeastAutomaticSource, catalog.SpecialistUpkeep, feast.AutomaticPurchase}
	keys := []string{"timed_offers", "feast_source", "specialist_upkeep", "ruby_feast"}
	supported := []bool{false, true, true, false}
	for i, capability := range caps {
		if capability.Supported != supported[i] || capability.ReasonDescriptor == nil || capability.ReasonDescriptor.Key != "server.game_data.buyer_capability."+keys[i] || capability.ReasonDescriptor.FallbackText != capability.Reason || capability.ReasonDescriptor.Fallback != capability.Reason {
			t.Fatalf("capability %d: %+v", i, capability)
		}
		capability.ReasonDescriptor.Key = "mutated"
	}
	catalog.Feasts[0].AutomaticPurchase.ReasonDescriptor.Key = "mutated"
	again, err := store.AutoBuyerCatalog()
	if err != nil {
		t.Fatal(err)
	}
	next, found := store.AutoBuyerFeast(1)
	if !found || again.TimedOffers.ReasonDescriptor.Key == "mutated" || again.FeastAutomaticSource.ReasonDescriptor.Key == "mutated" || again.SpecialistUpkeep.ReasonDescriptor.Key == "mutated" || again.Feasts[0].AutomaticPurchase.ReasonDescriptor.Key == "mutated" || next.AutomaticPurchase.ReasonDescriptor.Key == "mutated" {
		t.Fatal("catalog/getter descriptors share mutable state")
	}
}

func TestAutoBuyerCapabilityUnknownAndNestedCopy(t *testing.T) {
	source := AutoBuyerCapability{Reason: "External unknown reason"}
	if got := copyAutoBuyerCapability(source); got.ReasonDescriptor != nil || got.Reason != source.Reason {
		t.Fatal("unknown reason changed")
	}
	message := Localization.New("fixture", "Template {value}", Localization.Params{"value": "literal {admin}"})
	message.Context = []*Localization.Message{Localization.New("context", "Prefix {count}", Localization.Params{"count": 2})}
	source.ReasonDescriptor = Localization.Bind(message, source.Reason)
	copied := copyAutoBuyerCapability(source)
	copied.ReasonDescriptor.Params["value"] = "changed"
	copied.ReasonDescriptor.Context[0].Params["count"] = 9
	if source.ReasonDescriptor.Params["value"] != "literal {admin}" || source.ReasonDescriptor.Context[0].Params["count"] != 2 {
		t.Fatal("nested metadata shared")
	}
	source.Reason = "Changed reason without matching metadata"
	if copyAutoBuyerCapability(source).ReasonDescriptor != nil {
		t.Fatal("stale metadata conceals changed reason")
	}
}
