package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestRecruitPolicyWakesOnLivePlayerGloryTitleChanges(t *testing.T) {
	if got, want := NewRecruitPolicy().WakeDomains(), []string{"production", "subscriptions", "glory-title", "kingdom-transport", "alliance-help"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Auto Recruit wake domains=%v, want %v", got, want)
	}
	if got, want := NewToolPolicy().WakeDomains(), []string{"production", "subscriptions", "kingdom-transport"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Auto Tool wake domains=%v, want %v", got, want)
	}
}

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

func TestRecruitPolicySkipsCurrentRetainedUnfocusableCastle(t *testing.T) {
	now := time.Date(2026, 8, 26, 23, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.ChangedAt = now.Add(-time.Minute)
	snapshot.State.KingdomTransport.ObservedAt = now
	snapshot.State.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{
		KingdomID: 10, Unlocked: true, Created: false,
	}
	castle := snapshot.State.Castles[77]
	castle.KingdomID = 10
	snapshot.State.Castles[77] = castle
	decision, err := NewRecruitPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil ||
		decision.Detail != "Configured production castles are not focusable in the current kingdom session" {
		t.Fatalf("retained unfocusable recruit castle decision=%#v err=%v", decision, err)
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

func TestRecruitPolicyResolvesSavedUnitToHighestQueueableUpgrade(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"global","globalItems":[{"id":515,"minId":513,"maxId":516,"amount":25}],
		"castles":{"77":{"enabled":true,"items":[]}}
	}`)
	castle := snapshot.State.Castles[77]
	castle.QueueableObservedAt = now
	castle.QueueableProduction = map[int][]State.DefinitionRef{
		0: {
			{Collection: "units", ID: 513},
			{Collection: "units", ID: 516},
		},
	}
	snapshot.State.Castles[77] = castle

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("upgraded recruit decision=%#v err=%v", decision, err)
	}
	if arguments := productionIntentArguments(t, decision); arguments.DefinitionID != 516 {
		t.Fatalf("resolved recruit definition=%d, want highest queueable family member 516", arguments.DefinitionID)
	}
}

func TestRecruitPolicyResolvesLegacyScheduledUnitAfterUpgrade(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"global","globalItems":[],"castles":{"77":{"enabled":true}}
	}`)
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
		"featureSchedules":{"autoRecruit":{
			"enabled":true,"timeZone":"UTC","slotOptionsEnabled":true,
			"slots":[{"day":4,"startMinute":0,"endMinute":1440,
				"options":{"unitID":515}}]
		}}
	}`)
	castle := snapshot.State.Castles[77]
	castle.QueueableObservedAt = now
	castle.QueueableProduction = map[int][]State.DefinitionRef{
		0: {{Collection: "units", ID: 516}},
	}
	snapshot.State.Castles[77] = castle

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("scheduled upgraded recruit decision=%#v err=%v", decision, err)
	}
	arguments := productionIntentArguments(t, decision)
	if arguments.DefinitionID != 516 || arguments.ScheduledDefinitionID != 516 {
		t.Fatalf("scheduled upgraded recruit arguments=%#v, want resolved definition 516", arguments)
	}
}

func TestRecruitPolicyWaitsWhenNoConfiguredUnitFamilyMemberIsQueueable(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"global","globalItems":[{"id":515,"amount":25}],
		"castles":{"77":{"enabled":true,"items":[]}}
	}`)
	castle := snapshot.State.Castles[77]
	castle.QueueableObservedAt = now
	castle.QueueableProduction = map[int][]State.DefinitionRef{
		0: {{Collection: "units", ID: 489}},
	}
	snapshot.State.Castles[77] = castle

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Detail != "No enabled castle can currently produce the configured unit family" {
		t.Fatalf("unavailable recruit family decision=%#v, want fail-closed wait", decision)
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

func TestToolPolicyRefreshesPreviousSessionQueueBeforeEnqueue(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	gameData := productionPolicyGameData(t, `{
		"versionInfo":[],
		"buildings":[{"wodID":256,"name":"Workshop","level":"4"}],
		"units":[{"wodID":614,"name":"Workshop","toolCategory":"Basic"}],
		"constructionItems":[]
	}`)
	gameState := State.NewGameState()
	gameState.Session.Generation = 4
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 256},
		},
		Production: map[int]State.ProductionQueue{
			1: {LineID: 1, Capacity: 5, ObservedAt: now.Add(-2 * time.Minute)},
		},
	}
	decision, err := NewToolPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoTool": json.RawMessage(`{"mode":"global","globalItems":[{"id":614}],"castles":{"77":{"enabled":true}}}`),
		}},
		GameData: gameData,
		Now:      now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "game.focus_castle" {
		t.Fatalf("stale tool queue decision = %#v err=%v", decision, err)
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
	observeOwnAllianceHelpList(&snapshot.State, now)
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

func TestRecruitPolicyWaitsForCurrentAllianceHelpListBeforeAHR(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.Generation = 4
	snapshot.State.Session.ChangedAt = now.Add(-time.Minute)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{ProductionID: 201, Amount: 5, AllianceHelpAvailable: true}
	queue.Queued = []State.QueueItem{
		{ProductionID: 202, Amount: 5}, {ProductionID: 203, Amount: 5},
		{ProductionID: 204, Amount: 5}, {ProductionID: 205, Amount: 5},
		{ProductionID: 206, Amount: 5},
	}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle

	policy := NewRecruitPolicy()
	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "waiting" || decision.Request != nil ||
		decision.Detail != "Waiting for the current recruitment alliance-help request list" {
		t.Fatalf("unobserved recruitment help decision = %#v", decision)
	}

	observeOwnAllianceHelpList(&snapshot.State, now)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("observed recruitment help decision = %#v err=%v", decision, err)
	}
}

func TestRecruitPolicyRefreshesStaleSlotsBeforeAHRThenUsesOpenSlot(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.Generation = 4
	snapshot.State.Session.ChangedAt = now.Add(-time.Hour)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	elapsed := now.Add(-time.Minute)
	queue.ObservedAt = snapshot.State.Session.ChangedAt.Add(-time.Second)
	queue.Active = &State.QueueItem{ProductionID: 201, Amount: 5, AllianceHelpAvailable: true, CompletesAt: &elapsed}
	queue.Queued = []State.QueueItem{
		{ProductionID: 202, Amount: 5}, {ProductionID: 203, Amount: 5},
		{ProductionID: 204, Amount: 5}, {ProductionID: 205, Amount: 5},
		{ProductionID: 206, Amount: 5},
	}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	observeOwnAllianceHelpList(&snapshot.State, now)
	policy := NewRecruitPolicy()

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "game.focus_castle" ||
		!decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
		t.Fatalf("stale full queue decision = %#v err=%v", decision, err)
	}
	var refresh struct {
		CastleID State.CastleID `json:"castleId"`
		Refresh  bool           `json:"refresh"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &refresh); err != nil || refresh.CastleID != 77 || !refresh.Refresh {
		t.Fatalf("stale queue refresh arguments = %#v err=%v", refresh, err)
	}

	refreshedAt := now.Add(time.Minute)
	futureCompletion := now.Add(time.Hour)
	queue.ObservedAt = refreshedAt
	queue.Active.CompletesAt = &futureCompletion
	castle.ContextSnapshotObservedAt = refreshedAt
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	snapshot.Now = refreshedAt.Add(time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("current full queue decision = %#v err=%v", decision, err)
	}

	snapshot.State.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77}, OwnObservedGeneration: 4, ObservedAt: refreshedAt,
	}
	queue.Active.AllianceHelpRequested = true
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil && decision.Request.Name == "alliance.help.request" {
		t.Fatalf("outstanding help was requested twice: decision=%#v err=%v", decision, err)
	}

	queue.Queued = queue.Queued[:4]
	queue.ObservedAt = refreshedAt.Add(time.Minute)
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	snapshot.Now = queue.ObservedAt.Add(time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("refreshed open queue decision = %#v err=%v", decision, err)
	}
}

func TestRecruitPolicyRotatesAcrossStaleCastles(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.Generation = 4
	snapshot.State.Session.ChangedAt = now.Add(-time.Minute)
	first := snapshot.State.Castles[77]
	first.Production[0] = State.ProductionQueue{LineID: 0, Capacity: 5, ObservedAt: now.Add(-2 * time.Minute)}
	snapshot.State.Castles[77] = first
	second := first
	second.ID = 88
	second.Name = "Second"
	snapshot.State.Castles[88] = second
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"global","checkIntervalSec":300,"globalItems":[{"id":489}],
		"castles":{"77":{"enabled":true},"88":{"enabled":true}}
	}`)
	policy := NewRecruitPolicy()
	selected := make([]State.CastleID, 0, 2)
	for range 2 {
		decision, err := policy.Evaluate(t.Context(), snapshot)
		if err != nil || decision.Request == nil || decision.Request.Name != "game.focus_castle" {
			t.Fatalf("stale rotation decision = %#v err=%v", decision, err)
		}
		var refresh struct {
			CastleID State.CastleID `json:"castleId"`
		}
		if err := json.Unmarshal(decision.Request.Arguments, &refresh); err != nil {
			t.Fatal(err)
		}
		selected = append(selected, refresh.CastleID)
	}
	if selected[0] == selected[1] {
		t.Fatalf("stale queue refresh did not rotate castles: %v", selected)
	}
}

func TestRecruitPolicyDoesNotRefocusWhenCurrentCastleSnapshotOmittedQueue(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.Generation = 4
	snapshot.State.Session.ChangedAt = now.Add(-time.Hour)
	castle := snapshot.State.Castles[77]
	castle.ContextSnapshotObservedAt = now
	queue := castle.Production[0]
	queue.ObservedAt = now.Add(-time.Minute)
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle

	decision, err := NewRecruitPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("current castle snapshot omission caused a refocus loop: %#v", decision)
	}
}

func TestRecruitPolicyDoesNotRepeatOutstandingCastleWideAllianceHelp(t *testing.T) {
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.Generation = 4
	snapshot.State.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77}, OwnObservedGeneration: 4, ObservedAt: now,
	}
	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Active = &State.QueueItem{ProductionID: 201, Amount: 5, AllianceHelpRequested: true}
	queue.Queued = []State.QueueItem{
		{ProductionID: 202, Amount: 5}, {ProductionID: 203, Amount: 5},
		{ProductionID: 204, Amount: 5}, {ProductionID: 205, Amount: 5},
		{ProductionID: 206, Amount: 5},
	}
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	decision, err := NewRecruitPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil && decision.Request.Name == "alliance.help.request" {
		t.Fatalf("outstanding castle-wide help was requested again: %#v", decision)
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
	observeOwnAllianceHelpList(&snapshot.State, now)
	decision, err = NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("per-castle alliance-help decision: %#v err=%v", decision, err)
	}
	if decision.ScheduleKey != "autoRecruit:77" {
		t.Fatalf("per-castle alliance-help schedule key = %q", decision.ScheduleKey)
	}
}

func TestRecruitPolicyStrictlyRotatesPerCastleUnitsAfterSuccess(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	raw := json.RawMessage(`{
		"mode":"perCastle","checkIntervalSec":300,
		"castles":{"77":{"enabled":true,"cursor":1,"items":[
			{"id":489,"amount":25},{"id":490,"amount":30},{"id":491,"amount":35}
		]}}
	}`)
	snapshot.Configuration.Sections["automation.recruitTroops"] = raw

	policy := NewRecruitPolicy()
	wantUnits := []int64{490, 491, 489, 490}
	wantExpectedCursors := []int{1, 2, 0, 1}
	wantCursors := []int{2, 0, 1, 2}
	for index, wantUnit := range wantUnits {
		snapshot.Configuration.Sections["automation.recruitTroops"] = raw
		decision, err := policy.Evaluate(context.Background(), snapshot)
		if err != nil || decision.Request == nil || decision.Request.Name != "production.enqueue" {
			t.Fatalf("rotation %d decision=%#v err=%v", index+1, decision, err)
		}
		arguments := productionIntentArguments(t, decision)
		if arguments.DefinitionID != wantUnit || arguments.FillAvailable {
			t.Fatalf("rotation %d arguments=%#v, want unit %d and one stack", index+1, arguments, wantUnit)
		}
		if decision.FollowUp == nil || decision.FollowUp.Name != "config.update" {
			t.Fatalf("rotation %d did not defer cursor advancement until success: %#v", index+1, decision)
		}
		var update struct {
			Value         json.RawMessage  `json:"value"`
			ExpectedValue *json.RawMessage `json:"expectedValue"`
		}
		if err := json.Unmarshal(decision.FollowUp.Arguments, &update); err != nil {
			t.Fatal(err)
		}
		if update.ExpectedValue == nil {
			t.Fatalf("rotation %d cursor update was not conditional", index+1)
		}
		var expected productionSettings
		if err := json.Unmarshal(*update.ExpectedValue, &expected); err != nil {
			t.Fatal(err)
		}
		if cursor := expected.Castles["77"].Cursor; cursor != wantExpectedCursors[index] {
			t.Fatalf("rotation %d expected cursor=%d, want %d", index+1, cursor, wantExpectedCursors[index])
		}
		var updated productionSettings
		if err := json.Unmarshal(update.Value, &updated); err != nil {
			t.Fatal(err)
		}
		if cursor := updated.Castles["77"].Cursor; cursor != wantCursors[index] {
			t.Fatalf("rotation %d cursor=%d, want %d", index+1, cursor, wantCursors[index])
		}
		raw = update.Value
	}
}

func TestHostedRecruitPolicyAdvancesOperationalCursorWithoutConfigurationFollowUp(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.ConfigurationExternallyOwned = true
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"perCastle","checkIntervalSec":300,
		"castles":{"77":{"enabled":true,"cursor":0,"items":[
			{"id":489,"amount":25},{"id":490,"amount":30},{"id":491,"amount":35}
		]}}
	}`)
	snapshot.State.Automations["autoRecruit"] = State.AutomationState{
		ID:                 "autoRecruit",
		OperationalCursors: map[string]int{productionOperationalCursorKey("77"): 1},
	}

	decision, err := NewRecruitPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("hosted rotation decision=%#v err=%v", decision, err)
	}
	if arguments := productionIntentArguments(t, decision); arguments.DefinitionID != 490 || arguments.FillAvailable {
		t.Fatalf("hosted rotation arguments=%#v", arguments)
	}
	if decision.FollowUp != nil {
		t.Fatalf("hosted rotation attempted a configuration follow-up: %#v", decision.FollowUp)
	}
	if decision.OperationalCursor == nil ||
		decision.OperationalCursor.Key != productionOperationalCursorKey("77") ||
		decision.OperationalCursor.Value != 2 {
		t.Fatalf("hosted operational cursor=%#v", decision.OperationalCursor)
	}
}

func TestRecruitPolicySinglePerCastleUnitStillFillsAvailableSlots(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"perCastle","castles":{"77":{"enabled":true,"cursor":7,"items":[{"id":489,"amount":25}]}}
	}`)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("single-unit decision=%#v err=%v", decision, err)
	}
	if arguments := productionIntentArguments(t, decision); !arguments.FillAvailable || arguments.DefinitionID != 489 {
		t.Fatalf("single-unit arguments=%#v", arguments)
	}
	if decision.FollowUp != nil {
		t.Fatalf("single-unit configuration unexpectedly advanced a cursor: %#v", decision.FollowUp)
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

func TestRecruitPolicyRestoresStormScheduleAfterSeasonalCastleIDChange(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name             string
		kingdomMetadata  string
		rememberOldStorm bool
		queueableID      int64
		wantRequest      bool
	}{
		{
			name: "persisted kingdom identity", kingdomMetadata: `,"kingdomId":4`,
			queueableID: 489, wantRequest: true,
		},
		{
			name: "legacy Storm scan identity", rememberOldStorm: true,
			queueableID: 489, wantRequest: true,
		},
		{
			name: "live recruitability guard remains authoritative", rememberOldStorm: true,
			queueableID: 516, wantRequest: false,
		},
		{
			name:        "unknown missing castle is not guessed to be Storm",
			queueableID: 489, wantRequest: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := recruitPolicySnapshot(t, now)
			storm := snapshot.State.Castles[77]
			delete(snapshot.State.Castles, 77)
			storm.ID = 88
			storm.KingdomID = State.KingdomID(GameData.StormKingdomID)
			storm.Name = "New Storm Castle"
			storm.QueueableObservedAt = now
			storm.QueueableProduction = map[int][]State.DefinitionRef{
				0: {{Collection: "units", ID: test.queueableID}},
			}
			snapshot.State.Castles[88] = storm
			if test.rememberOldStorm {
				snapshot.State.Storm.LastScannedAt[77] = now.Add(-24 * time.Hour)
			}
			snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(fmt.Sprintf(`{
				"mode":"perCastle","checkIntervalSec":300,
				"castles":{"77":{"enabled":true%s,"items":[{"id":489,"amount":25}]}}
			}`, test.kingdomMetadata))
			snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
				"featureSchedules":{"autoRecruit:77":{
					"enabled":true,"timeZone":"UTC","slotOptionsEnabled":true,
					"slots":[{"day":1,"startMinute":700,"endMinute":800,"options":{"unitID":489}}]
				}}
			}`)

			decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if !test.wantRequest {
				if decision.Request != nil {
					t.Fatalf("unsafe Storm restoration decision=%#v", decision)
				}
				return
			}
			if decision.Request == nil || decision.Request.Name != "production.enqueue" {
				t.Fatalf("restored Storm decision=%#v, want production enqueue", decision)
			}
			arguments := productionIntentArguments(t, decision)
			if arguments.CastleID != 88 || arguments.DefinitionID != 489 ||
				arguments.ScheduledDefinitionID != 489 || decision.ScheduleKey != "autoRecruit:77" {
				t.Fatalf("restored Storm arguments=%#v schedule=%q", arguments, decision.ScheduleKey)
			}
		})
	}
}

func TestRecruitPolicyReevaluatesScheduledUnitForEveryOpenedSlot(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 30, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"perCastle","checkIntervalSec":3600,
		"castles":{"77":{"enabled":true,"items":[{"id":489,"amount":25}]}}
	}`)
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
		"featureSchedules":{"autoRecruit:77":{
			"enabled":true,"timeZone":"America/New_York","slotOptionsEnabled":true,"slots":[
				{"day":3,"startMinute":1200,"endMinute":1260,"options":{"unitID":2069}},
				{"day":3,"startMinute":1260,"endMinute":1440,"options":{"unitID":524}}
			]
		}}
	}`)

	policy := NewRecruitPolicy()
	decision, err := policy.Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("first scheduled decision=%#v err=%v", decision, err)
	}
	first := productionIntentArguments(t, decision)
	wantFirstUntil := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)
	if first.DefinitionID != 2069 || first.ScheduledDefinitionID != 2069 || first.FillAvailable ||
		first.ScheduleValidUntil == nil || !first.ScheduleValidUntil.Equal(wantFirstUntil) {
		t.Fatalf("first scheduled arguments=%#v, want one 2069 stack valid through %s", first, wantFirstUntil)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale || decision.FollowUp != nil {
		t.Fatalf("first scheduled continuation=%#v", decision)
	}

	castle := snapshot.State.Castles[77]
	queue := castle.Production[0]
	queue.Queued = append(queue.Queued, State.QueueItem{
		Definition: State.DefinitionRef{Collection: "units", ID: 2069}, Amount: first.Amount,
	})
	castle.Production[0] = queue
	snapshot.State.Castles[77] = castle
	snapshot.Now = now.Add(5 * time.Minute)
	decision, err = policy.Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("second scheduled decision=%#v err=%v", decision, err)
	}
	second := productionIntentArguments(t, decision)
	if second.DefinitionID != 2069 || second.ScheduledDefinitionID != 2069 || second.FillAvailable {
		t.Fatalf("second opened slot fell back from schedule: %#v", second)
	}

	snapshot.Now = wantFirstUntil.Add(5 * time.Minute)
	decision, err = policy.Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("next-period scheduled decision=%#v err=%v", decision, err)
	}
	third := productionIntentArguments(t, decision)
	if third.DefinitionID != 524 || third.ScheduledDefinitionID != 524 || third.FillAvailable {
		t.Fatalf("next-period scheduled arguments=%#v, want freshly resolved unit 524", third)
	}
}

func TestRecruitPolicyDoesNotFallBackWhenActiveScheduleSlotHasNoUnit(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 30, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"perCastle","castles":{"77":{"enabled":true,"items":[{"id":489,"amount":25}]}}
	}`)
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
		"featureSchedules":{"autoRecruit:77":{
			"enabled":true,"timeZone":"America/New_York","slotOptionsEnabled":true,
			"slots":[{"day":3,"startMinute":1200,"endMinute":1260}]
		}}
	}`)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Detail != "The active schedule slot has no valid unit" {
		t.Fatalf("missing scheduled unit decision=%#v, want fail-closed wait", decision)
	}
}

func TestRecruitPolicyUsesGlobalScheduledUnitWithoutStaticFallback(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 30, 0, 0, time.UTC)
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(`{
		"mode":"global","globalItems":[],"castles":{"77":{"enabled":true}}
	}`)
	snapshot.Configuration.Sections["scheduler"] = json.RawMessage(`{
		"featureSchedules":{"autoRecruit":{
			"enabled":true,"timeZone":"America/New_York","slotOptionsEnabled":true,
			"slots":[{"day":3,"startMinute":1200,"endMinute":1260,"options":{"unitID":2069}}]
		}}
	}`)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("global scheduled decision=%#v err=%v", decision, err)
	}
	arguments := productionIntentArguments(t, decision)
	if arguments.DefinitionID != 2069 || arguments.ScheduledDefinitionID != 2069 || arguments.FillAvailable {
		t.Fatalf("global scheduled arguments=%#v", arguments)
	}
}

func TestRecruitPolicyUsesTitleGatedLevel11WhileGloryTitleIsCurrent(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	snapshot := gloryTitleRecruitPolicySnapshot(t, now, 493, 31, 493, false)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("title-eligible decision=%#v err=%v", decision, err)
	}
	arguments := productionIntentArguments(t, decision)
	if arguments.DefinitionID != 493 || arguments.TitleGatedDefinitionID != 493 ||
		arguments.RequiredGloryTitleID != 31 || arguments.TitleLossFallback {
		t.Fatalf("title-eligible arguments=%#v", arguments)
	}
}

func TestRecruitPolicySoftPausesTitleGatedSlotByDefault(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	snapshot := gloryTitleRecruitPolicySnapshot(t, now, 493, 30, 238, false)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Detail != "Glory-title level 11 recruit slots are paused while the required title is lost" {
		t.Fatalf("title-loss default decision=%#v", decision)
	}
}

func TestRecruitPolicyRotationSkipsSoftPausedTitleGatedSlot(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	snapshot := gloryTitleRecruitPolicySnapshot(t, now, 493, 30, 515, false)
	raw := json.RawMessage(`{
		"mode":"perCastle","checkIntervalSec":300,"recruitLevel10OnTitleLoss":false,
		"castles":{"77":{"enabled":true,"cursor":0,"items":[
			{"id":493,"amount":25},{"id":515,"amount":25},{"id":516,"amount":25}
		]}}
	}`)
	snapshot.Configuration.Sections["automation.recruitTroops"] = raw

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil || decision.Request == nil {
		t.Fatalf("rotation skip decision=%#v err=%v", decision, err)
	}
	arguments := productionIntentArguments(t, decision)
	if arguments.DefinitionID != 515 || arguments.TitleGatedDefinitionID != 0 ||
		arguments.RequiredGloryTitleID != 0 || arguments.TitleLossFallback {
		t.Fatalf("rotation skip arguments=%#v", arguments)
	}
	if decision.Detail != "Queue Auto Recruit rotation unit 2 of 3 at castle 77" {
		t.Fatalf("rotation skip detail=%q", decision.Detail)
	}
	if decision.FollowUp == nil || decision.FollowUp.Name != "config.update" {
		t.Fatalf("rotation skip did not advance after the selected slot: %#v", decision)
	}
	var update struct {
		Value         json.RawMessage  `json:"value"`
		ExpectedValue *json.RawMessage `json:"expectedValue"`
	}
	if err := json.Unmarshal(decision.FollowUp.Arguments, &update); err != nil {
		t.Fatal(err)
	}
	if update.ExpectedValue == nil {
		t.Fatalf("rotation skip cursor update was not conditional: %#v", update)
	}
	var expected productionSettings
	if err := json.Unmarshal(*update.ExpectedValue, &expected); err != nil {
		t.Fatal(err)
	}
	if cursor := expected.Castles["77"].Cursor; cursor != 0 {
		t.Fatalf("rotation skip expected cursor=%d, want 0", cursor)
	}
	var updated productionSettings
	if err := json.Unmarshal(update.Value, &updated); err != nil {
		t.Fatal(err)
	}
	if cursor := updated.Castles["77"].Cursor; cursor != 2 {
		t.Fatalf("rotation skip cursor=%d, want 2", cursor)
	}
}

func TestRecruitPolicyCanUseExactLevel10FallbackAfterTitleLoss(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name           string
		configuredID   int64
		currentTitleID int64
		queueableID    int64
		wantID         int64
		wantTitleID    int64
	}{
		{name: "Protector of the North", configuredID: 489, currentTitleID: 29, queueableID: 227, wantID: 227, wantTitleID: 30},
		{name: "Valkyrie Sniper", configuredID: 493, currentTitleID: 30, queueableID: 238, wantID: 238, wantTitleID: 31},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := gloryTitleRecruitPolicySnapshot(
				t, now, test.configuredID, test.currentTitleID, test.queueableID, true,
			)
			decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
			if err != nil || decision.Request == nil {
				t.Fatalf("fallback decision=%#v err=%v", decision, err)
			}
			arguments := productionIntentArguments(t, decision)
			if arguments.DefinitionID != test.wantID ||
				arguments.TitleGatedDefinitionID != test.configuredID ||
				arguments.RequiredGloryTitleID != test.wantTitleID || !arguments.TitleLossFallback {
				t.Fatalf("fallback arguments=%#v", arguments)
			}
		})
	}
}

func TestRecruitPolicyAppliesGloryFallbackToLowerTierFamilyAnchor(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	paused := gloryTitleRecruitPolicySnapshot(t, now, 238, 30, 238, false)
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), paused)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Detail != "Glory-title level 11 recruit slots are paused while the required title is lost" {
		t.Fatalf("lower-tier family anchor bypassed disabled fallback: %#v", decision)
	}

	fallback := gloryTitleRecruitPolicySnapshot(t, now, 238, 30, 238, true)
	decision, err = NewRecruitPolicy().Evaluate(context.Background(), fallback)
	if err != nil || decision.Request == nil {
		t.Fatalf("lower-tier fallback decision=%#v err=%v", decision, err)
	}
	arguments := productionIntentArguments(t, decision)
	if arguments.DefinitionID != 238 || arguments.TitleGatedDefinitionID != 493 ||
		arguments.RequiredGloryTitleID != 31 || !arguments.TitleLossFallback {
		t.Fatalf("lower-tier fallback arguments=%#v", arguments)
	}

	titleRestored := gloryTitleRecruitPolicySnapshot(t, now, 238, 31, 493, true)
	decision, err = NewRecruitPolicy().Evaluate(context.Background(), titleRestored)
	if err != nil || decision.Request == nil {
		t.Fatalf("restored-title decision=%#v err=%v", decision, err)
	}
	arguments = productionIntentArguments(t, decision)
	if arguments.DefinitionID != 493 || arguments.TitleGatedDefinitionID != 493 ||
		arguments.RequiredGloryTitleID != 31 || arguments.TitleLossFallback {
		t.Fatalf("restored-title arguments=%#v", arguments)
	}
}

func TestRecruitPolicyWaitsWhenCurrentGloryTitleIsUnknown(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	snapshot := gloryTitleRecruitPolicySnapshot(t, now, 493, 30, 238, true)
	snapshot.State.Player.GloryTitleAt = time.Time{}

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Detail != "Waiting for the current player glory title before recruiting a title-gated level 11 unit" {
		t.Fatalf("unknown-title decision=%#v", decision)
	}
}

func TestRecruitPolicyDoesNotUseLevel10ForNonTitleAvailabilityFailure(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	snapshot := gloryTitleRecruitPolicySnapshot(t, now, 493, 31, 238, true)

	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Detail != "No enabled castle can currently produce the configured unit family" {
		t.Fatalf("eligible-but-unavailable decision=%#v", decision)
	}
}

type productionArguments struct {
	CastleID               State.CastleID `json:"castleId"`
	DefinitionID           int64          `json:"definitionId"`
	Amount                 int64          `json:"amount"`
	FillAvailable          bool           `json:"fillAvailable"`
	ScheduledDefinitionID  int64          `json:"scheduledDefinitionId"`
	ScheduleValidUntil     *time.Time     `json:"scheduleValidUntil"`
	TitleGatedDefinitionID int64          `json:"titleGatedDefinitionId"`
	RequiredGloryTitleID   int64          `json:"requiredGloryTitleId"`
	TitleLossFallback      bool           `json:"titleLossFallback"`
}

func gloryTitleRecruitPolicySnapshot(
	t *testing.T,
	now time.Time,
	configuredID int64,
	currentTitleID int64,
	queueableID int64,
	fallback bool,
) Snapshot {
	t.Helper()
	snapshot := recruitPolicySnapshot(t, now)
	snapshot.State.Session.ConnectionGeneration = 7
	snapshot.State.Player.GloryTitleID = currentTitleID
	snapshot.State.Player.GloryTitleAt = now
	snapshot.State.Player.GloryTitleGen = 7
	castle := snapshot.State.Castles[77]
	castle.QueueableObservedAt = now
	castle.QueueableProduction = map[int][]State.DefinitionRef{
		0: {{Collection: "units", ID: queueableID}},
	}
	snapshot.State.Castles[77] = castle
	snapshot.Configuration.Sections["automation.recruitTroops"] = json.RawMessage(fmt.Sprintf(`{
		"mode":"global","checkIntervalSec":300,"recruitLevel10OnTitleLoss":%t,
		"globalItems":[{"id":%d,"amount":0}],"castles":{"77":{"enabled":true,"items":[]}}
	}`, fallback, configuredID))
	snapshot.GameData = productionPolicyGameData(t, `{
		"versionInfo":[],
		"buildings":[{"wodID":1939,"stackSize":"110","constructionItemGroupIDs":"2"}],
		"units":[
			{"wodID":227,"type":"MeadMace","level":"10","upgradeWodID":"489"},
			{"wodID":489,"type":"MeadMace","level":"11","downgradeWodID":"227"},
			{"wodID":238,"type":"MeadBow","level":"10","upgradeWodID":"493"},
			{"wodID":493,"type":"MeadBow","level":"11","downgradeWodID":"238"},
			{"wodID":515,"type":"Militia","level":"1"},
			{"wodID":516,"type":"Spear","level":"1"}
		],
		"titles":[
			{"titleID":"29","type":"FAME","displayType":"suffix"},
			{"titleID":"30","previousTitleID":"29","type":"FAME","displayType":"suffix","effects":"46&489"},
			{"titleID":"31","previousTitleID":"30","type":"FAME","displayType":"suffix","effects":"46&493"}
		],
		"constructionItems":[{
			"constructionItemID":14,"constructionItemGroupID":"2","stackSize":"80",
			"lockRemoval":"SOLDIER_RECRUITMENT"
		}],
		"subscriptionsBuffs":[],"viplevels":[]
	}`)
	return snapshot
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
			"units":[
				{"wodID":489},
				{"wodID":513,"upgradeWodID":514},
				{"wodID":514,"downgradeWodID":513,"upgradeWodID":515},
				{"wodID":515,"downgradeWodID":514,"upgradeWodID":516},
				{"wodID":516,"downgradeWodID":515}
			],
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

func observeOwnAllianceHelpList(state *State.GameState, observedAt time.Time) {
	if state.Session.Generation == 0 {
		state.Session.Generation = 1
	}
	if state.Session.ChangedAt.IsZero() || !state.Session.ChangedAt.Before(observedAt) {
		state.Session.ChangedAt = observedAt.Add(-time.Second)
	}
	state.AllianceHelpRequests.OwnObservedGeneration = state.Session.Generation
	state.AllianceHelpRequests.ObservedAt = observedAt
}

func productionPolicyGameData(t *testing.T, raw string) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(raw), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatalf("decode game data: %v", err)
	}
	return store
}
