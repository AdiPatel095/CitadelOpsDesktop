package App

import (
	"context"
	"encoding/json"
	"errors"
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
	if len(plan.Steps) != 7 {
		t.Fatalf("production fill steps = %d, want focus, capacity guard, and five enqueue commands", len(plan.Steps))
	}
	if plan.Steps[0].Opcode != "jaa" {
		t.Fatalf("first production fill opcode = %q, want jaa", plan.Steps[0].Opcode)
	}
	if plan.Steps[1].Action != "production.enqueue.verify_capacity" || plan.Steps[1].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("production capacity guard = %#v", plan.Steps[1])
	}
	for index, step := range plan.Steps[2:] {
		if step.Opcode != "bup" || step.AwaitOpcode != "bup" {
			t.Fatalf("production fill step %d = %#v, want awaited bup", index+2, step)
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
	if guard.ScheduledDefinitionID != 2069 || guard.ScheduleValidUntil == nil ||
		!guard.ScheduleValidUntil.Equal(validUntil) || guard.FillAvailable || guard.ExpectedFreeSlots != 1 {
		t.Fatalf("scheduled production guard=%#v", guard)
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
