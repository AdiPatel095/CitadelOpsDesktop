package App

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestAdvisorActivationRequiresExplicitPaidTokenConfirmation(t *testing.T) {
	gameState := State.NewGameState()
	gameState.EventScores.ActiveEventID = 72
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{
		EventID: 72, DifficultyID: 308, RemainingSec: 7_200, AdvisorCurrencyID: 77, ObservedAt: time.Now().UTC(),
	}
	gameState.Player.Currencies[77] = 1
	input := Intent.PlanningContext{State: gameState}

	if _, err := planAdvisorActivation(t.Context(), input, json.RawMessage(`{"eventId":72}`)); err == nil || !strings.Contains(err.Error(), "confirmedTokenSpend=true") {
		t.Fatalf("advisor activation did not require explicit confirmation: %v", err)
	}
	gameState.Player.Currencies[77] = 0
	if _, err := planAdvisorActivation(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"eventId":72,"confirmedTokenSpend":true}`)); err == nil || !strings.Contains(err.Error(), "neither is available") {
		t.Fatalf("advisor activation did not require an owned token: %v", err)
	}
	gameState.Player.Currencies[76] = 1
	plan, err := planAdvisorActivation(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"eventId":72,"confirmedTokenSpend":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Opcode != "aa" || string(plan.Steps[0].Command.Payload) != `{"AAT":1}` {
		t.Fatalf("unexpected advisor activation plan: %#v", plan)
	}
}

func TestAdvisorAttackAddsWireFieldsAndGuardsEveryRequestedCopy(t *testing.T) {
	gameState, gameData, request := advisorIntentFixture(t)
	gameState.DailyAttacks = State.DailyAttackState{Count: 10_000, ServerThreshold: 3_500, ObservedAt: time.Now().UTC()}
	arguments, _ := json.Marshal(request)
	input := Intent.PlanningContext{State: gameState, GameData: gameData}

	plan, err := planAdvisorAttack(t.Context(), input, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var deferred *Intent.Step
	for index := range plan.Steps {
		if plan.Steps[index].Action == "attack.daily_limit.guard" {
			t.Fatal("advisor attack was incorrectly subjected to the normal CRA daily limit")
		}
		if plan.Steps[index].Resolver == "advisor.attack.build" {
			deferred = &plan.Steps[index]
			break
		}
	}
	if deferred == nil || deferred.CommandDependencies == nil || deferred.CommandDependencies.Opcode != "cra" {
		t.Fatalf("advisor CRA was not deferred behind fresh attack context: %#v", plan.Steps)
	}
	resolved, err := (&Application{}).resolveAdvisorAttackStep(t.Context(), input, deferred.ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	var body advisorAttackBody
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.AttackCount != 3 || body.Mode != 0 || body.AdvisorType != 1 || body.Leader != 1 || body.PremiumTravel != 1 {
		t.Fatalf("unexpected advisor CRA body: %#v", body)
	}
	if !strings.Contains(string(resolved.Command.Payload), `"AAC":3`) ||
		!strings.Contains(string(resolved.Command.Payload), `"AASM":0`) ||
		!strings.Contains(string(resolved.Command.Payload), `"AAT":1`) {
		t.Fatalf("advisor fields are absent from CRA: %s", resolved.Command.Payload)
	}

	short := gameState.Castles[1]
	short.Units.Stationed[88] = 20
	gameState.Castles[1] = short
	_, err = (&Application{}).resolveAdvisorAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, deferred.ResolverArguments,
	)
	if err == nil || !strings.Contains(err.Error(), "3 commander(s) require 30") {
		t.Fatalf("advisor resolver did not reserve tools for all three attacks: %v", err)
	}
}

func TestAdvisorAttackSharesHigherAndLowerFamilyTiersAcrossRepeatedAttacks(t *testing.T) {
	gameState, gameData, request := advisorIntentFixture(t)
	source := gameState.Castles[1]
	source.Units.Stationed[76] = 180
	source.Units.Stationed[77] = 120
	gameState.Castles[1] = source
	anchor := int64(76)
	request.Preset.UseTroopFamilies = true
	request.Preset.Waves[0].Middle.Troops = []AttackPresets.Slot{{ItemID: &anchor, Quantity: 100}}
	arguments, _ := json.Marshal(request)
	input := Intent.PlanningContext{State: gameState, GameData: gameData}

	plan, err := planAdvisorAttack(t.Context(), input, arguments)
	if err != nil {
		t.Fatal(err)
	}
	var deferred Intent.Step
	for _, step := range plan.Steps {
		if step.Resolver == "advisor.attack.build" {
			deferred = step
			break
		}
	}
	resolved, err := (&Application{}).resolveAdvisorAttackStep(t.Context(), input, deferred.ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	var body advisorAttackBody
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	units := body.Waves[0].Middle.Units
	if units[0] != (attackPair{77, 40}) || units[1] != (attackPair{76, 60}) {
		t.Fatalf("advisor family allocation = %#v, want 40 tier-77 then 60 tier-76 per repeated attack", units)
	}
}

func TestAdvisorAttackRequiresAndEnforcesRubyHorseBudget(t *testing.T) {
	gameState, gameData, request := advisorIntentFixture(t)
	request.HorseTravelBoostID = 1008
	arguments, _ := json.Marshal(request)
	if _, err := planAdvisorAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err == nil || !strings.Contains(err.Error(), "positive rubyCostPerAttack") {
		t.Fatalf("ruby horse was accepted without a configured cost: %v", err)
	}

	request.RubyCostPerAttack = 100
	request.MinimumRubyReserve = 25
	gameState.Player.Resources[2] = 224
	arguments, _ = json.Marshal(request)
	if _, err := planAdvisorAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err == nil || !strings.Contains(err.Error(), "rubies") {
		t.Fatalf("ruby horse ignored the all-attack ruby budget: %v", err)
	}

	gameState.Player.Resources[2] = 325
	if _, err := planAdvisorAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err != nil {
		t.Fatalf("sufficient ruby budget was rejected: %v", err)
	}
}

func advisorIntentFixture(t *testing.T) (State.GameState, *GameData.Store, advisorAttackRequest) {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":76,"upgradeWodID":77},
			{"wodID":77,"downgradeWodID":76},
			{"wodID":88,"slotTypes":"attack"}
		],
		"resources":[{"resourceID":1,"JSONKey":"C1"},{"resourceID":2,"JSONKey":"C2"}],
		"currencies":[{"currencyID":22,"JSONKey":"PTT"}],
		"effects":[],
		"effectCaps":[],
		"eventAutoScalingDifficulties":[{"difficultyID":201,"eventID":80,"difficultyTypeID":1,"isLocked":0}],
		"eventAutoScalingCamps":[
			{"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,"camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":600}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{77: 300, 88: 30}, Traveling: map[State.UnitID]int64{}},
	}
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true}
	gameState.Player.Resources[1] = 100_000
	gameState.Player.Currencies[22] = 3
	gameState.Player.Currencies[1005] = 3
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ActiveEventID = 80
	gameState.EventScores.ByEvent[80] = State.ScalableEventScore{
		EventID: 80, DifficultyID: 201, RemainingSec: 7_200, AdvisorActive: true,
		AdvisorCurrencyID: 78, ObservedAt: now,
	}
	gameState.NomadCamps.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"99:100":  {KingdomID: 0, X: 99, Y: 100, TypeID: 29, ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9, ObservedAt: now},
		"100:99":  {KingdomID: 0, X: 100, Y: 99, TypeID: 29, ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9, ObservedAt: now},
		"100:101": {KingdomID: 0, X: 100, Y: 101, TypeID: 29, ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9, ObservedAt: now},
		"101:100": {KingdomID: 0, X: 101, Y: 100, TypeID: 29, ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9, ObservedAt: now},
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 1, KingdomID: 0,
		Target: State.AttackDialogTarget{
			TypeID: 29, X: 101, Y: 100, ObjectID: 5001, EventCampID: 5001, EventCampVictoryCount: 9,
		},
		ActiveEffects: []State.AttackDialogEffect{}, ObservedAt: now,
	}
	unitID, toolID := int64(77), int64(88)
	request := advisorAttackRequest{
		nomadTargetRequest: nomadTargetRequest{
			SourceCastleID: 1, EventID: 80, DifficultyID: 201, KingdomID: 0,
			TargetTypeID: 29, TargetX: 101, TargetY: 100, EventCampID: 5001,
		},
		Preset: AttackPresets.Preset{ID: "camp", Name: "Camp", Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{
				Troops: []AttackPresets.Slot{{ItemID: &unitID, Quantity: 100}},
				Tools:  []AttackPresets.Slot{{ItemID: &toolID, Quantity: 10}},
			},
		}}},
		CommanderID: 1, AttackCount: 3, MinimumRemainingSec: 1_800,
		CoinCostPerAttack: 500, TimeSkipReserve: map[string]int64{}, HorseTravelBoostID: -1,
	}
	return gameState, gameData, request
}
