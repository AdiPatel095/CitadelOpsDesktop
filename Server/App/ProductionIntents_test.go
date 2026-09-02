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
	if len(plan.Steps) != 4 {
		t.Fatalf("production enqueue steps = %d, want focus, guarded BUP, and causal AHR", len(plan.Steps))
	}
	var guard productionQueueCapacityGuard
	if err := json.Unmarshal(plan.Steps[1].ActionArguments, &guard); err != nil {
		t.Fatalf("decode production guard: %v", err)
	}
	if !guard.RequireNewerQueue || guard.QueueObservedAt.IsZero() {
		t.Fatalf("focus-changing production plan lacks a newer-queue guard: %#v", guard)
	}
	var payload struct {
		SessionKey int `json:"SK"`
		KingdomID  int `json:"SID"`
	}
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &payload); err != nil {
		t.Fatalf("decode production payload: %v", err)
	}
	if payload.SessionKey != defaultProductionSessionKey {
		t.Fatalf("session key = %d, want %d", payload.SessionKey, defaultProductionSessionKey)
	}
	if payload.KingdomID != 4 {
		t.Fatalf("production kingdom id = %d, want 4", payload.KingdomID)
	}
	if plan.Steps[3].Resolver != "production.enqueue.alliance_help.build" ||
		!containsString(plan.Claims, "alliance-help") {
		t.Fatalf("recruitment BUP plan lacks one causal AHR tail: steps=%#v claims=%#v", plan.Steps, plan.Claims)
	}
	if plan.Summary != "Queue 110 Veteran Swordsman (level 6) at castle 77" {
		t.Fatalf("production summary = %q", plan.Summary)
	}
}

func TestPlanProductionEnqueueRejectsUntrustworthyQueueSnapshots(t *testing.T) {
	now := time.Now().UTC()
	changedAt := now.Add(-time.Minute)
	elapsed := now.Add(-time.Second)
	for _, test := range []struct {
		name  string
		queue State.ProductionQueue
	}{
		{name: "previous session", queue: State.ProductionQueue{LineID: 0, Capacity: 5, ObservedAt: changedAt.Add(-time.Second)}},
		{name: "elapsed active stack", queue: State.ProductionQueue{
			LineID: 0, Capacity: 5, ObservedAt: now, Active: &State.QueueItem{CompletesAt: &elapsed},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Session.Generation = 7
			gameState.Session.ChangedAt = changedAt
			gameState.Castles[77] = State.CastleState{
				ID: 77, Focused: true, Production: map[int]State.ProductionQueue{0: test.queue},
			}
			_, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{State: gameState},
				json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110}`))
			if !errors.Is(err, Intent.ErrPlanStale) {
				t.Fatalf("stale queue plan error = %v, want stale", err)
			}
		})
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
	if len(plan.Steps) != 4 || plan.Steps[2].Opcode != "bup" ||
		plan.Steps[3].Resolver != "production.enqueue.alliance_help.build" {
		t.Fatalf("production enqueue steps = %#v, want focus, guarded BUP, and causal AHR", plan.Steps)
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
	if len(plan.Steps) != 12 {
		t.Fatalf("production fill steps = %d, want focus, five guarded BUPs, and one AHR", len(plan.Steps))
	}
	if plan.Steps[0].Opcode != "jaa" {
		t.Fatalf("first production fill opcode = %q, want jaa", plan.Steps[0].Opcode)
	}
	for stack := 0; stack < 5; stack++ {
		stepOffset := 1 + stack*2
		guardStep := plan.Steps[stepOffset]
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
		commandStep := plan.Steps[stepOffset+1]
		if commandStep.Opcode != "bup" || commandStep.AwaitOpcode != "bup" ||
			commandStep.ResponseBarrier != Intent.ResponseBarrierCommitted {
			t.Fatalf("production fill command %d = %#v, want awaited bup", stack+1, commandStep)
		}
	}
	if plan.Steps[len(plan.Steps)-1].Resolver != "production.enqueue.alliance_help.build" {
		t.Fatalf("recruitment AHR was not emitted once after the final BUP: %#v", plan.Steps)
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
	if len(plan.Steps) != 4 || plan.Steps[2].Opcode != "bup" ||
		plan.Steps[3].Resolver != "production.enqueue.alliance_help.build" {
		t.Fatalf("production fill steps = %#v, want focus, one guarded BUP, and one AHR", plan.Steps)
	}
}

func TestPlanRecruitmentBUPAlwaysRequestsAllianceHelpAfterBatch(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	now := time.Now().UTC()
	gameState.Session.Generation = 7
	gameState.Session.ConnectionGeneration = 3
	gameState.Session.ChangedAt = now.Add(-2 * time.Minute)
	gameState.Player.ID = 501
	gameState.AllianceHelpRequests = State.AllianceHelpRequestState{
		RecruitmentCastleIDs: []State.CastleID{77},
		ObservedAt:           now, OwnObservedGeneration: 7,
		OwnRecruitmentRequests: []State.RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 77, Progress: 1, MaximumHelpers: 3, ObservedAt: now},
		},
		OwnRecruitmentObservedGeneration: 7,
	}
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: now},
		},
	}
	input := Intent.PlanningContext{
		State: gameState, GameData: gameData,
		ProtocolContext: State.ProtocolContextState{
			SessionGeneration: 7, ConnectionGeneration: 3,
			FocusedCastleID: 77, FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 11,
			RecruitmentBUPCastleID: 77, RecruitmentBUPFocusEpoch: 9,
			RecruitmentBUPSerial: 2, RecruitmentAHRCoveredSerial: 2,
			RecruitmentAHRFocusCovered: true,
		},
	}
	arguments := json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110}`)

	plan, err := planProductionEnqueue(t.Context(), input, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Action != "production.enqueue.verify_capacity" ||
		plan.Steps[1].Opcode != "bup" ||
		plan.Steps[2].Resolver != "production.enqueue.alliance_help.build" {
		t.Fatalf("recruitment BUP steps=%#v, want guard, BUP, then one AHR", plan.Steps)
	}
	if !containsString(plan.Claims, "alliance-help") {
		t.Fatalf("recruitment BUP claims=%#v, want alliance-help serialization", plan.Claims)
	}
	step, err := (&Application{}).resolveRecruitmentBUPAllianceHelpStep(
		t.Context(), input, plan.Steps[2].ResolverArguments,
	)
	if err != nil {
		t.Fatalf("pending lifecycle or prior focus epoch suppressed BUP AHR: %v", err)
	}
	var payload struct {
		RequestID int64 `json:"ID"`
		Type      int   `json:"T"`
	}
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RequestID != 0 || payload.Type != allianceHelpRecruitmentType ||
		step.ResponseIdentity.PlayerID != 501 || step.ResponseIdentity.CastleID != 77 {
		t.Fatalf("BUP AHR step=%#v payload=%#v", step, payload)
	}
}

func TestPlanToolProductionDoesNotQueueRecruitmentAllianceHelp(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":614,"name":"internalMeadMace","type":"meadMace","slotTypes":"tool"}],
		"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			1: {LineID: 1, Capacity: 1, ObservedAt: time.Now().UTC()},
		},
	}
	plan, err := planProductionEnqueue(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":1,"definitionId":614,"amount":25}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[2].Opcode != "bup" {
		t.Fatalf("tool production steps = %#v, want only focus, guard, and BUP", plan.Steps)
	}
	if containsString(plan.Claims, "alliance-help") {
		t.Fatalf("tool BUP claimed recruitment alliance help: %#v", plan.Claims)
	}
	for _, step := range plan.Steps {
		if step.Action == "production.enqueue.mark_help_due" ||
			step.Action == "production.enqueue.mark_help_covered" ||
			step.Resolver == "production.enqueue.alliance_help.build" {
			t.Fatalf("tool BUP queued recruitment alliance help: %#v", plan.Steps)
		}
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
	if len(plan.Steps) != 4 || plan.Steps[1].Action != "production.enqueue.verify_capacity" ||
		plan.Steps[3].Resolver != "production.enqueue.alliance_help.build" {
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

func TestVerifyProductionQueueCapacityRequiresPostFocusObservation(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	gameState := State.NewGameState()
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = now.Add(-time.Hour)
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: observedAt},
		},
	}
	arguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 0, DefinitionID: 489, ExpectedFreeSlots: 1,
		QueueObservedAt: observedAt, RequireNewerQueue: true,
	})
	application := &Application{State: State.NewStore(gameState)}
	if err := application.verifyProductionQueueCapacityAt(arguments, now); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("unchanged post-focus queue error = %v, want stale", err)
	}

	castle := gameState.Castles[77]
	castle.ContextSnapshotObservedAt = observedAt.Add(30 * time.Second)
	gameState.Castles[77] = castle
	replannedArguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 0, DefinitionID: 489, ExpectedFreeSlots: 1,
	})
	application = &Application{State: State.NewStore(gameState)}
	if err := application.verifyProductionQueueCapacityAt(replannedArguments, now); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("replanned guard accepted a queue omitted by the committed castle snapshot: %v", err)
	}

	queue := castle.Production[0]
	queue.ObservedAt = castle.ContextSnapshotObservedAt
	castle.Production[0] = queue
	gameState.Castles[77] = castle
	application = &Application{State: State.NewStore(gameState)}
	if err := application.verifyProductionQueueCapacityAt(arguments, now); err != nil {
		t.Fatalf("new post-focus queue observation rejected: %v", err)
	}
}

func TestVerifyProductionQueueCapacityRejectsElapsedActiveQueue(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Session.Generation = 7
	gameState.Session.ChangedAt = now.Add(-time.Hour)
	gameState.Castles[77] = State.CastleState{
		ID: 77, Focused: true,
		Production: map[int]State.ProductionQueue{
			0: {
				LineID: 0, Capacity: 5, ObservedAt: now.Add(-time.Minute),
				Active: &State.QueueItem{CompletesAt: &now},
			},
		},
	}
	arguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: 77, LineID: 0, DefinitionID: 489, ExpectedFreeSlots: 1,
	})
	application := &Application{State: State.NewStore(gameState)}
	if err := application.verifyProductionQueueCapacityAt(arguments, now); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("elapsed production queue guard error = %v, want stale", err)
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
	if len(plan.Steps) != 6 {
		t.Fatalf("title-gated production steps=%#v", plan.Steps)
	}
	for stack := 0; stack < 2; stack++ {
		stepOffset := 1 + stack*2
		guardStep := plan.Steps[stepOffset]
		commandStep := plan.Steps[stepOffset+1]
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
	for _, step := range plan.Steps {
		if len(step.StaleCodes) != 1 || step.StaleCodes[0] != 175 {
			t.Fatalf("hospital focus-sensitive step does not classify code 175 as stale: %#v", step)
		}
	}
	if plan.Summary != "Heal wounded units: 10 Veteran Swordsman (level 6) at castle 77" {
		t.Fatalf("hospital heal summary = %q", plan.Summary)
	}
}

func TestFocusSensitiveProductionPlannersRejectRetainedUnfocusableCastle(t *testing.T) {
	now := time.Date(2026, 8, 26, 23, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.KingdomTransport.ObservedAt = now
	state.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{
		KingdomID: 10, Unlocked: true, Created: false,
	}
	state.Castles[77] = State.CastleState{
		ID: 77, KingdomID: 10,
		Units: State.CastleUnits{Hospital: map[State.UnitID]int64{489: 10}},
	}
	if _, err := planProductionEnqueue(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":10}`),
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("production planner accepted retained unfocusable castle: %v", err)
	}
	if _, err := planHospitalHeal(
		t.Context(), Intent.PlanningContext{State: state},
		json.RawMessage(`{"castleId":77,"unitId":489,"amount":10}`),
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("hospital planner accepted retained unfocusable castle: %v", err)
	}
}

func TestObservedProductionStackHonorsScopedLearnedFloor(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Subscriptions = map[int]State.SubscriptionState{7: {TypeID: 7, RemainingSec: 3600}}
	queue := State.ProductionQueue{
		LineID:            0,
		Queued:            []State.QueueItem{{Definition: State.DefinitionRef{ID: 489}, Amount: 220}},
		LearnedStacks:     map[int64]int64{489: 260},
		LearnedStackScope: "sub:7",
	}
	if got := observedProductionStack(gameState, queue, 489); got != 260 {
		t.Fatalf("matching scope should floor at learned 260, got %d", got)
	}

	// After a subscription lapse the learned floor is stale and live stacks rule.
	gameState.Subscriptions = nil
	if got := observedProductionStack(gameState, queue, 489); got != 220 {
		t.Fatalf("stale scope must fall back to live stacks (220), got %d", got)
	}

	// A live stack larger than the learned floor always wins.
	gameState.Subscriptions = map[int]State.SubscriptionState{7: {TypeID: 7}}
	queue.Queued = append(queue.Queued, State.QueueItem{Definition: State.DefinitionRef{ID: 489}, Amount: 300})
	if got := observedProductionStack(gameState, queue, 489); got != 300 {
		t.Fatalf("live 300 should beat learned 260, got %d", got)
	}

	// Another unit's learned cap must not floor this unit: a different
	// definition with no history mimics the line (cold start) but ignores
	// unit 489's high-water mark.
	queue.Queued = []State.QueueItem{{Definition: State.DefinitionRef{ID: 512}, Amount: 132}}
	if got := observedProductionStack(gameState, queue, 512); got != 132 {
		t.Fatalf("unit 512 must not inherit unit 489's 260 floor, got %d", got)
	}
}
