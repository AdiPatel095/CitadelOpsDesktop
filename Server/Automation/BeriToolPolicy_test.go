package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriToolPolicyUsesIndependentSharedFeatureControls(t *testing.T) {
	policy := NewBeriToolPolicy()
	if policy.ID() != "autoBeriWorldTools" || policy.ActorID() != "autoBeriWorld" ||
		policy.ScheduleKey() != "autoBeriWorld" || policy.EnabledKey() != "auto_beri_world" {
		t.Fatalf(
			"unexpected feature controls: id=%q actor=%q schedule=%q enabled=%q",
			policy.ID(), policy.ActorID(), policy.ScheduleKey(), policy.EnabledKey(),
		)
	}
}

func TestBeriToolPolicyRefreshesThenBuysExactScalingLadderShortage(t *testing.T) {
	now := time.Date(2026, 7, 29, 23, 30, 0, 0, time.UTC)
	snapshot := beriToolTestSnapshot(t, now, `{"toolMinimums":{"614":10,"611":5,"620":5}}`)
	policy := NewBeriToolPolicy()

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tools.refresh" {
		t.Fatalf("refresh decision: %#v err=%v", decision, err)
	}

	castle := snapshot.State.Castles[989]
	castle.UnitsObservedAt = now
	castle.Units.Stationed = map[State.UnitID]int64{
		GameData.BerimondScalingLadderToolID: 1,
		GameData.BerimondBatteringRamToolID:  5,
		GameData.BerimondMantletToolID:       5,
	}
	snapshot.State.Castles[castle.ID] = castle
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tools.purchase" {
		t.Fatalf("purchase decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		CastleID  State.CastleID `json:"castleId"`
		PackageID int64          `json:"packageId"`
		ToolID    State.UnitID   `json:"toolId"`
		Amount    int64          `json:"amount"`
		Minimum   int64          `json:"minimum"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.CastleID != 989 || arguments.PackageID != 28 ||
		arguments.ToolID != GameData.BerimondScalingLadderToolID ||
		arguments.Amount != 9 || arguments.Minimum != 10 {
		t.Fatalf("purchase arguments = %#v", arguments)
	}
}

func TestBeriToolPolicyDoesNotDispatchAnUnaffordableBatch(t *testing.T) {
	now := time.Date(2026, 7, 29, 23, 30, 0, 0, time.UTC)
	snapshot := beriToolTestSnapshot(t, now, `{"toolMinimums":{"614":10}}`)
	castle := snapshot.State.Castles[989]
	castle.UnitsObservedAt = now
	castle.Units.Stationed = map[State.UnitID]int64{GameData.BerimondScalingLadderToolID: 1}
	snapshot.State.Castles[castle.ID] = castle
	snapshot.State.Player.Resources[1] = 1_979

	decision, err := NewBeriToolPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "1980 coins") {
		t.Fatalf("coin decision: %#v err=%v", decision, err)
	}
}

func TestBeriToolPolicyCapsEachPurchaseAtGameBatchLimit(t *testing.T) {
	now := time.Date(2026, 7, 29, 23, 30, 0, 0, time.UTC)
	snapshot := beriToolTestSnapshot(t, now, `{"toolMinimums":{"614":2501}}`)
	snapshot.State.Player.Resources[1] = 1_000_000
	castle := snapshot.State.Castles[989]
	castle.UnitsObservedAt = now
	castle.Units.Stationed = map[State.UnitID]int64{GameData.BerimondScalingLadderToolID: 1}
	snapshot.State.Castles[castle.ID] = castle

	decision, err := NewBeriToolPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.tools.purchase" {
		t.Fatalf("capped purchase decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		Amount int64 `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.Amount != GameData.BerimondArmorerMaxPurchaseAmount || !decision.ReevaluateOnSuccess {
		t.Fatalf("purchase amount = %d, want game cap %d", arguments.Amount, GameData.BerimondArmorerMaxPurchaseAmount)
	}

	castle.Units.Stationed[GameData.BerimondScalingLadderToolID] = 2_001
	snapshot.Now = now.Add(time.Second)
	castle.UnitsObservedAt = snapshot.Now
	snapshot.State.Castles[castle.ID] = castle
	decision, err = NewBeriToolPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("follow-up purchase decision: %#v err=%v", decision, err)
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.Amount != 500 {
		t.Fatalf("final purchase amount = %d, want remaining 500", arguments.Amount)
	}
}

func beriToolTestSnapshot(t *testing.T, now time.Time, settings string) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":611,"typ":"Attack"},
			{"wodID":614,"typ":"Attack"},
			{"wodID":620,"typ":"Attack"}
		],
		"packages":[
			{"packageID":28,"packageType":"tool","comment1":"Scaling ladder","comment2":"Armorer","packagePriceC1":220,"unitID":614,"unitAmount":1,"minLevel":4},
			{"packageID":32,"packageType":"tool","comment1":"Battering ram","comment2":"Armorer","packagePriceC1":430,"unitID":611,"unitAmount":1,"minLevel":4},
			{"packageID":36,"packageType":"tool","comment1":"Mantlet","comment2":"Armorer","packagePriceC1":810,"unitID":620,"unitAmount":1,"minLevel":17}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Player.Resources[1] = 100_000
	gameState.Castles[989] = State.CastleState{ID: 989, KingdomID: 10}
	gameState.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{
		KingdomID: 10, Unlocked: true, Created: true,
	}
	return Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			autoBeriWorldSection: json.RawMessage(settings),
		}},
	}
}
