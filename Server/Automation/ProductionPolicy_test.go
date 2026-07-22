package Automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestRecruitPolicyInfersStackCapacityFromBarracksAndConstructionItems(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	arguments := productionIntentArguments(t, decision)
	if arguments.CastleID != 77 || arguments.DefinitionID != 489 || arguments.Amount != 190 || !arguments.FillAvailable {
		t.Fatalf("unexpected intent arguments: %#v", arguments)
	}
}

func TestRecruitPolicyAddsActiveSubscriptionStackBonus(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Subscriptions[1] = State.SubscriptionState{TypeID: 1, RemainingSec: 60}
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if arguments := productionIntentArguments(t, decision); arguments.Amount != 230 {
		t.Fatalf("recruit stack amount = %d, want 230", arguments.Amount)
	}
}

func TestRecruitPolicyDoesNotReuseStaleOversizedStack(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	castle.ConstructionSlots[1] = nil
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 36}
	queue.Queued = []State.QueueItem{
		{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 180},
	}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	snapshot.State.Subscriptions[1] = State.SubscriptionState{TypeID: 1, RemainingSec: 60}

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if arguments := productionIntentArguments(t, decision); arguments.Amount != 150 {
		t.Fatalf("recruit stack amount = %d, want current inferred amount 150", arguments.Amount)
	}
}

func TestRecruitPolicyWaitsWithoutCalculatedStackCapacity(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Queued = []State.QueueItem{
		{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 180},
	}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	snapshot.GameData = nil

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request != nil || decision.Detail != "Waiting for the official building stack capacity" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestRecruitPolicyRotatesGlobalCastlesInDisplayOrder(t *testing.T) {
	now := time.Date(2026, 7, 21, 14, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	for _, castle := range []State.CastleState{
		{ID: 1, Name: "Castle Amos_Bur", KingdomID: 4},
		{ID: 902, Name: "Tycho", KingdomID: 0},
		{ID: 900, Name: "Baltimore", KingdomID: 0},
		{ID: 901, Name: "Ganymede", KingdomID: 0},
	} {
		castle.Production = map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		}
		gameState.Castles[castle.ID] = castle
	}
	snapshot := Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.recruitTroops": json.RawMessage(`{
				"mode":"global","globalItems":[{"id":489,"amount":25}],
				"castles":{"1":{"enabled":true},"900":{"enabled":true},"901":{"enabled":true},"902":{"enabled":true}}
			}`),
		}},
		Now: now,
	}
	policy := NewRecruitPolicy()
	for index, want := range []State.CastleID{900, 901, 902, 1, 900} {
		decision, err := policy.Evaluate(context.Background(), snapshot)
		if err != nil || decision.Request == nil || decision.Request.Name != "production.enqueue" {
			t.Fatalf("evaluation %d: decision=%#v err=%v", index+1, decision, err)
		}
		if got := productionIntentArguments(t, decision).CastleID; got != want {
			t.Fatalf("evaluation %d castle = %d, want %d", index+1, got, want)
		}
	}
}

func TestRecruitPolicyDoesNotCountActiveStackAgainstQueueSlots(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 110}
	queue.Queued = make([]State.QueueItem, 4)
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestToolPolicyInfersStackCapacityWithoutCountingActiveStackAgainstQueueSlots(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameData := productionPolicyGameData(t, `{
		"versionInfo":[],
		"buildings":[{"wodID":256,"name":"Workshop","level":"4"}],
		"units":[{"wodID":614,"name":"Workshop","toolCategory":"Basic"}],
		"constructionItems":[]
	}`)
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 256},
		},
		Production: map[int]State.ProductionQueue{
			1: {
				LineID: 1, Capacity: 5, ObservedAt: now,
				Active: &State.QueueItem{Definition: State.DefinitionRef{Collection: "tools", ID: 614}, Amount: 80},
				Queued: make([]State.QueueItem, 4),
			},
		},
	}
	decision, err := NewToolPolicy().Evaluate(context.Background(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoTool": json.RawMessage(`{
				"mode":"global","checkIntervalSec":300,"globalItems":[{"id":614,"amount":0}],
				"castles":{"77":{"enabled":true,"items":[]}}
			}`),
		}},
		GameData: gameData,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if arguments := productionIntentArguments(t, decision); arguments.Amount != 80 {
		t.Fatalf("tool stack amount = %d, want 80", arguments.Amount)
	}
}

func TestProductionPoliciesUseVIPQueueCapacityWhenTheQueueOmitsIt(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	castle.Production[0] = State.ProductionQueue{LineID: 0, ObservedAt: now}
	snapshot.State.Castles[77] = castle
	snapshot.State.Player.VIP = State.VIPState{Level: 10}
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate recruit policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected recruit decision: %#v", decision)
	}

	gameData := productionPolicyGameData(t, `{
		"versionInfo":[],
		"buildings":[{"wodID":256,"name":"Workshop","level":"4"}],
		"units":[{"wodID":614,"name":"Workshop","toolCategory":"Basic"}],
		"constructionItems":[],
		"viplevels":[{"vipLevelID":"10","productionBonusSlots":"3","recruitmentBonusSlots":"3"}]
	}`)
	toolState := State.NewGameState()
	toolState.Player.VIP = State.VIPState{Level: 10}
	toolState.Castles[77] = State.CastleState{
		ID: 77,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 256},
		},
		Production: map[int]State.ProductionQueue{1: {LineID: 1, ObservedAt: now}},
	}
	decision, err = NewToolPolicy().Evaluate(context.Background(), Snapshot{
		State: toolState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoTool": json.RawMessage(`{"mode":"global","globalItems":[{"id":614}],"castles":{"77":{"enabled":true}}}`),
		}},
		GameData: gameData,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("evaluate tool policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected tool decision: %#v", decision)
	}
}

func TestRecruitPolicyRequestsAllianceHelpAfterFillingTheQueue(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{ProductionID: 201, Amount: 5}
	queue.Queued = []State.QueueItem{{ProductionID: 202, Amount: 5}, {ProductionID: 203, Amount: 5}, {ProductionID: 204, Amount: 5}, {ProductionID: 205, Amount: 5}, {ProductionID: 206, Amount: 5}}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if productionID := allianceHelpProductionID(t, decision); productionID != 201 {
		t.Fatalf("production id = %d, want 201", productionID)
	}
}

func TestRecruitPolicyDoesNotRequestAllianceHelpBelowMinimumStack(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{ProductionID: 201, Amount: 4}
	queue.Queued = []State.QueueItem{
		{ProductionID: 202, Amount: 4}, {ProductionID: 203, Amount: 4}, {ProductionID: 204, Amount: 4},
		{ProductionID: 205, Amount: 4}, {ProductionID: 206, Amount: 4},
	}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request != nil {
		t.Fatalf("unexpected alliance-help request: %#v", decision)
	}
}

func TestRecruitPolicyPropagatesPerCastleScheduleKey(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"perCastle","checkIntervalSec":300,
		"castles":{"77":{"enabled":true,"items":[{"id":489,"amount":25}]}}
	}`)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("per-castle enqueue decision: %#v err=%v", decision, err)
	}
	if decision.ScheduleKey != "autoRecruit:77" {
		t.Fatalf("per-castle enqueue schedule key = %q", decision.ScheduleKey)
	}

	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{ProductionID: 201, Amount: 5, AllianceHelpAvailable: true}
	queue.Queued = make([]State.QueueItem, queue.Capacity)
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	decision, err = NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("per-castle alliance-help decision: %#v err=%v", decision, err)
	}
	if decision.ScheduleKey != "autoRecruit:77" {
		t.Fatalf("per-castle alliance-help schedule key = %q", decision.ScheduleKey)
	}
}

func TestRecruitPolicyWakesWhenPerCastleScheduleOpens(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"perCastle","checkIntervalSec":300,
		"castles":{"77":{"enabled":true,"items":[{"id":489,"amount":25}]}}
	}`)
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
		"featureSchedules":{"autoRecruit:77":{
			"enabled":true,"timeZone":"UTC","slots":[{"day":1,"startMinute":721,"endMinute":800}]
		}}
	}`)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request != nil {
		t.Fatalf("closed per-castle schedule decision: %#v err=%v", decision, err)
	}
	want := now.Add(time.Minute)
	if !decision.NextCheckAt.Equal(want) {
		t.Fatalf("per-castle schedule wake = %s, want %s", decision.NextCheckAt, want)
	}
}

type productionArguments struct {
	CastleID      State.CastleID `json:"castleId"`
	DefinitionID  int64          `json:"definitionId"`
	Amount        int64          `json:"amount"`
	FillAvailable bool           `json:"fillAvailable"`
}

func productionIntentArguments(t *testing.T, decision Decision) productionArguments {
	t.Helper()
	var arguments productionArguments
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatalf("decode intent arguments: %v", err)
	}
	return arguments
}

func recruitPolicySnapshot(t *testing.T, now time.Time) Snapshot {
	t.Helper()
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 1939},
		},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{
			1: {{DefinitionID: 14, Slot: 0, Level: 4}},
		},
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		},
	}
	return Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.recruitTroops": json.RawMessage(`{
				"mode":"global","checkIntervalSec":300,"globalItems":[{"id":489,"amount":0}],
				"castles":{"77":{"enabled":true,"items":[]}}
			}`),
		}},
		GameData: productionPolicyGameData(t, `{
			"versionInfo":[],
			"buildings":[{"wodID":1939,"stackSize":"110","constructionItemGroupIDs":"2"}],
			"units":[{"wodID":489}],
			"constructionItems":[{
				"constructionItemID":14,"constructionItemGroupID":"2","stackSize":"80",
				"lockRemoval":"SOLDIER_RECRUITMENT"
			}],
			"subscriptionsBuffs":[{"subscriptionTypeID":"1","effects":"189&40"}],
			"viplevels":[{"vipLevelID":"10","recruitmentBonusSlots":"3","productionBonusSlots":"3"}]
		}`),
		Now: now,
	}
}

func productionPolicyGameData(t *testing.T, raw string) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(raw), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatalf("decode game data: %v", err)
	}
	return store
}
