package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

// Explicit free-VIP context for pre-existing support/travel regression tests.
func supportCommanderTestInput(t *testing.T, input Intent.PlanningContext) Intent.PlanningContext {
	t.Helper()
	if input.State.Session.ConnectionGeneration == 0 {
		input.State.Session.ConnectionGeneration = 1
	}
	input.State.Player.VIP = State.VIPState{Points: 10, RemainingSec: 3600, ObservedAt: time.Now().UTC(), Generation: input.State.Session.Generation, ConnectionGeneration: input.State.Session.ConnectionGeneration}
	collections := map[string]json.RawMessage{"versionInfo": json.RawMessage(`[]`), "buildings": json.RawMessage(`[]`), "units": json.RawMessage(`[]`)}
	if input.GameData != nil {
		for _, name := range input.GameData.CatalogNames() {
			collections[name], _ = input.GameData.RawCollection(name)
		}
	}
	collections["viplevels"] = json.RawMessage(`[{"level":1,"thresholdMin":0,"thresholdMax":100,"freePremiumGeneralsPerDay":100}]`)
	collections["effectCaps"] = json.RawMessage(`[{"capID":99,"maxTotalBonus":100}]`)
	raw, _ := json.Marshal(collections)
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "invented-support-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	input.GameData = data
	if input.SupportCommanders == nil {
		input.SupportCommanders = newPremiumCommanderDispatchGate()
	}
	input.CommanderHolds = newCommanderLaunchHolds()
	input.OperationID = "invented-support-operation"
	return input
}
