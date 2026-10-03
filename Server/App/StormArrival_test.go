package App

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type stormArrivalCountingSender struct{ sends int }

func (*stormArrivalCountingSender) Ready() bool                  { return true }
func (*stormArrivalCountingSender) Namespace() string            { return "EmpireEx_21" }
func (*stormArrivalCountingSender) CorrelatesResponses() bool    { return true }
func (*stormArrivalCountingSender) ConnectionGeneration() uint64 { return 1 }
func (sender *stormArrivalCountingSender) Send(context.Context, []byte) error {
	sender.sends++
	return errors.New("synthetic sender must not dispatch a late move")
}

func stormArrivalApplicationFixture(t *testing.T) (*Application, State.GameState, time.Time) {
	t.Helper()
	raw, err := os.ReadFile("../Automation/testdata/storm_arrival_kingdoms_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]json.RawMessage
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog["units"] = json.RawMessage(`[{"wodID":10}]`)
	catalog["resources"] = json.RawMessage(`[{"resourceID":12,"JSONKey":"MEAD"}]`)
	catalog["isles"] = json.RawMessage(`[{"IsleID":7,"type":"DUNGEON","dungeonlevel":40,"maxCountVictories":10,"countVictories":"0#1#2#3#4#5#6#7#8#9"}]`)
	raw, _ = json.Marshal(catalog)
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "Items-vfixture.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{CacheDir: directory, VersionURL: "offline://fixture"})
	if err := manager.LoadCache(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Player.ID = 71001
	state.Session = State.SessionState{Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_21", ChangedAt: now.Add(-time.Hour)}
	fundTravelTicketsForTest(&state)
	source := kingdomTroopIntentCastle(81001, 0, "Synthetic donor")
	source.Units.Stationed[10] = 20
	storm := kingdomTroopIntentCastle(82001, 4, "Synthetic Storm castle")
	storm.X, storm.Y, storm.Focused = 100, 100, true
	storm.Units.Stationed[10] = 20
	state.Castles[source.ID], state.Castles[storm.ID] = source, storm
	state.Commanders[11] = State.CommanderState{ID: 11, Available: true}
	state.Map[4] = map[string]State.MapObservation{"110:100": {KingdomID: 4, TypeID: 25, X: 110, Y: 100, StormIsleID: 7, ObservedAt: now}}
	state.AttackDialog = State.AttackDialogState{SourceCastleID: storm.ID, KingdomID: 4,
		Target: State.AttackDialogTarget{TypeID: 25, X: 110, Y: 100, StormIsleID: 7}, ObservedAt: now}
	fundStormArrivalForTest(&state, now)
	state.KingdomTransport.ObservedAt = now
	for key, observation := range state.Storm.TravelObservations {
		observation.TargetX, observation.TargetY, observation.TravelSeconds = 110, 100, 180
		observation.SlowestSecondsPerTile = 18
		state.Storm.TravelObservations[key] = observation
	}
	return &Application{GameData: manager}, state, now
}

func TestStormArrivalLateCRAAndKUTSendNothingAndCreateNoLaneLock(t *testing.T) {
	for _, kind := range []string{"CRA", "KUT"} {
		t.Run(kind, func(t *testing.T) {
			application, state, now := stormArrivalApplicationFixture(t)
			row := state.KingdomTransport.Unlocks[4]
			row.EventEndsAt = now.Add(45 * time.Second)
			state.KingdomTransport.Unlocks[4] = row
			for key, observation := range state.Storm.TravelObservations {
				observation.EventEndsAt = row.EventEndsAt
				state.Storm.TravelObservations[key] = observation
			}
			store := State.NewStore(&state)
			data, _ := application.GameData.Current()
			intents := Intent.NewRegistry()
			planner := planStormAttack
			arguments := json.RawMessage(`{"sourceCastleId":82001,"kingdomId":4,"targetTypeId":25,"targetX":110,"targetY":100,"stormIsleId":7,"preset":{"id":"synthetic","name":"Synthetic","waves":[]}}`)
			if kind == "KUT" {
				planner = planKingdomTroopShipment
				arguments = json.RawMessage(`{"sourceCastleId":81001,"targetCastleId":82001,"targetKingdomId":4,"units":[{"unitId":10,"amount":1}]}`)
			}
			if err := intents.Register(Intent.Definition{Name: "synthetic.arrival", Effect: Intent.EffectLaunch, Planner: planner}); err != nil {
				t.Fatal(err)
			}
			registry := Ingest.NewRegistry()
			if err := Ingest.RegisterCoreReducers(registry); err != nil {
				t.Fatal(err)
			}
			pipeline := Ingest.NewPipeline(store, resourceIntentGameDataProvider{data}, registry)
			sender := &stormArrivalCountingSender{}
			engine := Intent.NewEngine(intents, store, resourceIntentGameDataProvider{data}, sender, pipeline)
			receipt := engine.Submit(t.Context(), Intent.Request{ID: "synthetic-" + kind, Name: "synthetic.arrival", Actor: "automation:autoStorm", AutomationLane: "autoStorm", Arguments: arguments})
			if sender.sends != 0 || receipt.Status != Intent.StatusFailed || !strings.Contains(receipt.Error, "cannot arrive") {
				t.Fatalf("late %s: sends=%d receipt=%+v", kind, sender.sends, receipt)
			}
			if engine.AutomationLaneLock("autoStorm").Active(time.Now().UTC()) || receipt.Failure != nil && receipt.Failure.SafetyLock != nil {
				t.Fatal("local timing block created a lane lock")
			}
		})
	}
}

func TestStormArrivalExistingDispatchGuardsRecheckDeadline(t *testing.T) {
	application, state, now := stormArrivalApplicationFixture(t)
	row := state.KingdomTransport.Unlocks[4]
	row.EventEndsAt = now.Add(10 * time.Minute)
	state.KingdomTransport.Unlocks[4] = row
	for key, observation := range state.Storm.TravelObservations {
		observation.EventEndsAt = row.EventEndsAt
		state.Storm.TravelObservations[key] = observation
	}
	application.State = State.NewStore(&state)
	attack, _ := json.Marshal(resolvedStormAttackRequest{stormAttackRequest: stormAttackRequest{SourceCastleID: 82001, KingdomID: 4, TargetTypeID: 25, TargetX: 110, TargetY: 100, StormIsleID: 7}, CommanderID: 11})
	if err := application.guardStormAttack(t.Context(), attack); err != nil {
		t.Fatal(err)
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(current *State.GameState) ([]string, bool, error) {
		row := current.KingdomTransport.Unlocks[4]
		row.EventEndsAt = time.Now().UTC().Add(2 * time.Minute)
		current.KingdomTransport.Unlocks[4] = row
		return []string{"kingdom-transport"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.guardStormAttack(t.Context(), attack); err == nil || !strings.Contains(err.Error(), "cannot arrive") {
		t.Fatalf("CRA final timing guard=%v", err)
	}
	guard := json.RawMessage(`{"targetKingdomId":4,"transportKind":"troop"}`)
	if err := application.verifyKingdomTransportAvailable(t.Context(), guard); err == nil || !strings.Contains(err.Error(), "cannot arrive") {
		t.Fatalf("KUT final timing guard=%v", err)
	}
}
