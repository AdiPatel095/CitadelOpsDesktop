package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestProductionDefinitionLabelUsesOfficialGameNameForTools(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":614,"name":"internalMeadMace","type":"meadMace","slotTypes":"tool"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"meadMace_name":"Mead Mace"}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if label := productionDefinitionLabel(gameData, language, "tools", 614); label != "Mead Mace" {
		t.Fatalf("tool label = %q, want Mead Mace", label)
	}
}

func TestProductionDefinitionLabelIncludesOfficialTroopLevel(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489,"name":"inventedInternalLabel","type":"veteranSwordsman","level":6}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"veteranSwordsman_name":"Veteran Swordsman"}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if label := productionDefinitionLabel(gameData, language, "units", 489); label != "Veteran Swordsman (level 6)" {
		t.Fatalf("troop label = %q, want official name and level", label)
	}
	if label := productionDefinitionLabel(gameData, nil, "units", 489); label != "unit 489 (level 6)" {
		t.Fatalf("unlocalized troop label = %q, want an honest numeric fallback", label)
	}
}

func TestPlanProductionEnqueueUsesDefaultSessionKeyBeforeObservation(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489,"type":"veteranSwordsman","level":6}],"constructionItems":[],
		"viplevels":[{"vipLevelID":"10","recruitmentBonusSlots":"3","productionBonusSlots":"3"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"veteranSwordsman_name":"Veteran Swordsman"}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID:         77,
		KingdomID:  4,
		Production: map[int]State.ProductionQueue{0: {LineID: 0, ObservedAt: time.Now().UTC()}},
	}
	gameState.Player.VIP = State.VIPState{Level: 10}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData, Language: language,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("production enqueue steps = %d, want focus, capacity guard, and enqueue command", len(plan.Steps))
	}
	var payload struct {
		SessionKey int `json:"SK"`
		KingdomID  int `json:"SID"`
	}
	if err := json.Unmarshal(plan.Steps[len(plan.Steps)-1].Command.Payload, &payload); err != nil {
		t.Fatalf("decode production payload: %v", err)
	}
	if payload.SessionKey != defaultProductionSessionKey {
		t.Fatalf("session key = %d, want %d", payload.SessionKey, defaultProductionSessionKey)
	}
	if payload.KingdomID != 4 {
		t.Fatalf("production kingdom id = %d, want 4", payload.KingdomID)
	}
	if plan.Summary != "Queue 110 Veteran Swordsman (level 6) at castle 77" {
		t.Fatalf("production summary = %q", plan.Summary)
	}
}

func TestPlanProductionEnqueueDoesNotCountActiveStackAgainstQueueSlots(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {
				LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC(),
				Active: &State.QueueItem{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 110},
				Queued: make([]State.QueueItem, 4),
			},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110}`))
	if err != nil {
		t.Fatalf("plan production enqueue: %v", err)
	}
	if len(plan.Steps) != 3 || plan.Steps[2].Opcode != "bup" {
		t.Fatalf("production enqueue steps = %#v, want focus, capacity guard, and enqueue command", plan.Steps)
	}
}

func TestPlanProductionEnqueueRejectsDefinitionMissingFromLiveQueueableCatalog(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":515},{"wodID":516}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		},
		QueueableObservedAt: now,
		QueueableProduction: map[int][]State.DefinitionRef{
			0: {{Collection: "units", ID: 516}},
		},
	}
	_, err = planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":515,"amount":110}`))
	if !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "unit 515 is not currently available") {
		t.Fatalf("unavailable production definition error=%v", err)
	}
}

func TestPlanProductionEnqueueFillsEveryAvailableQueueSlotWithOneFocus(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC()},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110,"fillAvailable":true}`))
	if err != nil {
		t.Fatalf("plan production fill: %v", err)
	}
	if len(plan.Steps) != 11 {
		t.Fatalf("production fill steps = %d, want focus and five guarded enqueue commands", len(plan.Steps))
	}
	if plan.Steps[0].Opcode != "jaa" {
		t.Fatalf("first production fill opcode = %q, want jaa", plan.Steps[0].Opcode)
	}
	for stack := 0; stack < 5; stack++ {
		guardStep := plan.Steps[1+stack*2]
		if guardStep.Action != "production.enqueue.verify_capacity" || guardStep.ResumePolicy != Intent.ResumeRebuild {
			t.Fatalf("production capacity guard %d = %#v", stack+1, guardStep)
		}
		var guard productionQueueCapacityGuard
		if err := json.Unmarshal(guardStep.ActionArguments, &guard); err != nil {
			t.Fatal(err)
		}
		if guard.ExpectedFreeSlots != 5-stack {
			t.Fatalf("production capacity guard %d expects %d free slots, want %d", stack+1, guard.ExpectedFreeSlots, 5-stack)
		}
		commandStep := plan.Steps[2+stack*2]
		if commandStep.Opcode != "bup" || commandStep.AwaitOpcode != "bup" {
			t.Fatalf("production fill command %d = %#v, want awaited bup", stack+1, commandStep)
		}
	}
}

func TestPlanProductionEnqueueFillOnlyUsesFreeQueueSlots(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC(), Queued: make([]State.QueueItem, 4)},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110,"fillAvailable":true}`))
	if err != nil {
		t.Fatalf("plan production fill: %v", err)
	}
	if len(plan.Steps) != 3 || plan.Steps[2].Opcode != "bup" {
		t.Fatalf("production fill steps = %#v, want focus, capacity guard, and one enqueue command", plan.Steps)
	}
}

func TestPlanProductionFillTreatsAlreadyFullQueueAsSatisfied(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC(), Queued: make([]State.QueueItem, 5)},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110,"fillAvailable":true}`))
	if err != nil {
		t.Fatalf("plan satisfied production fill: %v", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("satisfied production fill steps = %#v, want no commands", plan.Steps)
	}
}

func TestVerifyProductionQueueCapacityRejectsStaleFullQueue(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			1: {LineID: 1, Capacity: 5, ObservedAt: time.Now().UTC(), Queued: make([]State.QueueItem, 5)},
		},
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 1, ExpectedFreeSlots: 2, FillAvailable: true,
	})
	err := application.verifyProductionQueueCapacity(t.Context(), arguments)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("capacity guard error = %v, want stale plan", err)
	}
}

func TestPlanProductionEnqueueCarriesScheduledSelectionIntoCapacityGuard(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":2069}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	validUntil := now.Add(time.Hour)
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		},
	}
	arguments, _ := json.Marshal(map[string]any{
		"castleId": 77, "lineId": 0, "definitionId": 2069, "amount": 110,
		"fillAvailable": false, "scheduledDefinitionId": 2069, "scheduleValidUntil": validUntil,
	})
	plan, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[1].Action != "production.enqueue.verify_capacity" {
		t.Fatalf("scheduled production steps=%#v", plan.Steps)
	}
	var guard productionQueueCapacityGuard
	if err := json.Unmarshal(plan.Steps[1].ActionArguments, &guard); err != nil {
		t.Fatal(err)
	}
	if guard.DefinitionID != 2069 || guard.ScheduledDefinitionID != 2069 || guard.ScheduleValidUntil == nil ||
		!guard.ScheduleValidUntil.Equal(validUntil) || guard.FillAvailable || guard.ExpectedFreeSlots != 1 {
		t.Fatalf("scheduled production guard=%#v", guard)
	}
}

func TestVerifyProductionQueueCapacityRejectsDefinitionThatBecameUnavailable(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		},
		QueueableObservedAt: now,
		QueueableProduction: map[int][]State.DefinitionRef{
			0: {{Collection: "units", ID: 516}},
		},
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 0, DefinitionID: 515, ExpectedFreeSlots: 1,
	})
	err := application.verifyProductionQueueCapacity(t.Context(), arguments)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("unavailable definition guard error=%v, want stale plan", err)
	}
}

func TestVerifyProductionQueueCapacityRejectsExpiredScheduledSelection(t *testing.T) {
	now := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		},
	}
	application := &Application{State: State.NewStore(gameState)}
	expiredAt := now.Add(-time.Second)
	arguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 0, ExpectedFreeSlots: 1,
		ScheduledDefinitionID: 2069, ScheduleValidUntil: &expiredAt,
	})
	err := application.verifyProductionQueueCapacityAt(arguments, now)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("expired schedule guard error=%v, want stale plan", err)
	}

	validUntil := now.Add(time.Hour)
	arguments, _ = json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 0, ExpectedFreeSlots: 1,
		ScheduledDefinitionID: 2069, ScheduleValidUntil: &validUntil,
	})
	if err := application.verifyProductionQueueCapacityAt(arguments, now); err != nil {
		t.Fatalf("active schedule guard error=%v", err)
	}
}

func TestProductionGloryTitleGuardRevalidatesLevel11AndFallback(t *testing.T) {
	gameData := productionGloryTitleGameData(t)
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 7
	gameState.Player.GloryTitleID = 31
	gameState.Player.GloryTitleAt = now
	gameState.Player.GloryTitleGen = 7

	if err := validateProductionGloryTitle(gameState, gameData, 493, 493, 31, false); err != nil {
		t.Fatalf("current title rejected level 11: %v", err)
	}
	if err := validateProductionGloryTitle(gameState, gameData, 493, 0, 0, false); err != nil {
		t.Fatalf("implicit level-11 title guard rejected current title: %v", err)
	}
	gameState.Player.GloryTitleID = 30
	if err := validateProductionGloryTitle(gameState, gameData, 493, 493, 31, false); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("lost title level-11 guard error=%v, want stale", err)
	}
	if err := validateProductionGloryTitle(gameState, gameData, 493, 0, 0, false); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("implicit lost-title level-11 guard error=%v, want stale", err)
	}
	if err := validateProductionGloryTitle(gameState, gameData, 238, 493, 31, true); err != nil {
		t.Fatalf("lost title rejected level-10 fallback: %v", err)
	}
	gameState.Player.GloryTitleID = 31
	if err := validateProductionGloryTitle(gameState, gameData, 238, 493, 31, true); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("restored title fallback guard error=%v, want stale", err)
	}
	gameState.Player.GloryTitleGen = 6
	if err := validateProductionGloryTitle(gameState, gameData, 493, 493, 31, false); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("old-connection title guard error=%v, want stale", err)
	}
}

func TestPlanProductionEnqueueDerivesGloryTitleGuardForDirectLevel11Request(t *testing.T) {
	gameData := productionGloryTitleGameData(t)
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 7
	gameState.Player.GloryTitleID = 31
	gameState.Player.GloryTitleAt = now
	gameState.Player.GloryTitleGen = 7
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 2, ObservedAt: now},
		},
	}

	plan, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":493,"amount":110,"fillAvailable":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 {
		t.Fatalf("title-gated production steps=%#v", plan.Steps)
	}
	for stack := 0; stack < 2; stack++ {
		guardStep := plan.Steps[1+stack*2]
		commandStep := plan.Steps[2+stack*2]
		if guardStep.Action != "production.enqueue.verify_capacity" || commandStep.Opcode != "bup" {
			t.Fatalf("title-gated stack %d is not individually guarded: %#v", stack+1, plan.Steps)
		}
		var guard productionQueueCapacityGuard
		if err := json.Unmarshal(guardStep.ActionArguments, &guard); err != nil {
			t.Fatal(err)
		}
		if guard.TitleGatedDefinitionID != 493 || guard.RequiredGloryTitleID != 31 ||
			guard.TitleLossFallback || guard.ExpectedFreeSlots != 2-stack {
			t.Fatalf("derived title guard %d=%#v", stack+1, guard)
		}
	}

	gameState.Player.GloryTitleID = 30
	_, err = planProductionEnqueue(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":493,"amount":110}`))
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("direct level-11 request after title loss error=%v, want stale", err)
	}
}

func productionGloryTitleGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[
			{"wodID":238,"type":"MeadBow","level":"10"},
			{"wodID":493,"type":"MeadBow","level":"11"}
		],
		"titles":[
			{"titleID":"30","type":"FAME","displayType":"suffix"},
			{"titleID":"31","previousTitleID":"30","type":"FAME","displayType":"suffix","effects":"46&493"}
		],
		"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

func TestPlanHospitalHealAlwaysRefreshesFocusedCastle(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489,"type":"veteranSwordsman","level":6}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	language, err := GameData.DecodeLanguage([]byte(`{"veteranSwordsman_name":"Veteran Swordsman"}`), GameData.LanguageMetadata{Language: "en"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Units: State.CastleUnits{Hospital: map[State.UnitID]int64{489: 10}},
	}
	plan, err := planHospitalHeal(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData, Language: language,
	}, json.RawMessage(`{"castleId":77,"unitId":489,"amount":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "jaa" || plan.Steps[1].Opcode != "hru" {
		t.Fatalf("hospital heal steps = %#v, want fresh jaa then hru", plan.Steps)
	}
	if plan.Summary != "Heal wounded units: 10 Veteran Swordsman (level 6) at castle 77" {
		t.Fatalf("hospital heal summary = %q", plan.Summary)
	}
}

func TestObservedProductionStackHonorsScopedLearnedFloor(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Subscriptions = map[int]State.SubscriptionState{7: {TypeID: 7, RemainingSec: 3600}}
	queue := State.ProductionQueue{
		LineID:            0,
		Queued:            []State.QueueItem{{Amount: 220}},
		LearnedStack:      260,
		LearnedStackScope: "sub:7",
	}
	if got := observedProductionStack(gameState, queue); got != 260 {
		t.Fatalf("matching scope should floor at learned 260, got %d", got)
	}

	// After a subscription lapse the learned floor is stale and live stacks rule.
	gameState.Subscriptions = nil
	if got := observedProductionStack(gameState, queue); got != 220 {
		t.Fatalf("stale scope must fall back to live stacks (220), got %d", got)
	}

	// A live stack larger than the learned floor always wins.
	gameState.Subscriptions = map[int]State.SubscriptionState{7: {TypeID: 7}}
	queue.Queued = append(queue.Queued, State.QueueItem{Amount: 300})
	if got := observedProductionStack(gameState, queue); got != 300 {
		t.Fatalf("live 300 should beat learned 260, got %d", got)
	}
}
