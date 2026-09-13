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
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

func TestFortressAttackBuildsOneFullDirewolfFlankWaveWithoutPremiumBooster(t *testing.T) {
	gameData := fortressIntentGameData(t)
	now := time.Now().UTC()
	gameState := fortressIntentState(now)
	arguments := json.RawMessage(`{
		"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,
		"commanderIds":[5],"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100
	}`)
	plan, err := planFortressAttack(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Admission == nil || plan.Admission.Module != "autoFortress" || plan.Admission.Class != Intent.AdmissionAttackLaunch {
		t.Fatalf("fortress admission = %#v", plan.Admission)
	}
	var deferred Intent.Step
	gaaCount, gaaIndex, launchIndex := 0, -1, -1
	for index, step := range plan.Steps {
		if step.Opcode == "gaa" {
			gaaCount++
			gaaIndex = index
			if step.ResponseBarrier != Intent.ResponseBarrierCommitted {
				t.Fatalf("fortress target refresh is not committed: %#v", step)
			}
			var payload struct {
				X1 int `json:"AX1"`
				Y1 int `json:"AY1"`
				X2 int `json:"AX2"`
				Y2 int `json:"AY2"`
			}
			if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.X1 != 101 || payload.Y1 != 100 || payload.X2 != 101 || payload.Y2 != 100 {
				t.Fatalf("fortress pre-CRA refresh is not 1x1: %#v", payload)
			}
		}
		if step.Resolver == "fortress.attack.build" {
			deferred = step
			launchIndex = index
		}
	}
	if gaaCount != 1 || gaaIndex < 0 || launchIndex < 0 || gaaIndex >= launchIndex {
		t.Fatalf("fortress plan must commit one 1x1 GAA before CRA: %#v", plan.Steps)
	}
	if deferred.Resolver == "" || deferred.CommandDependencies == nil || deferred.CommandDependencies.Opcode != "cra" {
		t.Fatalf("fortress plan has no guarded CRA resolver: %#v", plan.Steps)
	}
	gameState.AttackDialog = State.AttackDialogState{
		SourceCastleID: 10, KingdomID: 1, ObservedAt: time.Now().UTC(),
		Target: State.AttackDialogTarget{TypeID: State.MapTypeKingdomFortress, X: 101, Y: 100, Level: 45},
	}
	resolved, err := (&Application{}).resolveFortressAttackStep(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, deferred.ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Leader State.CommanderID `json:"LID"`
		Waves  []attackWave      `json:"A"`
	}
	if err := json.Unmarshal(resolved.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if resolved.Opcode != "cra" || body.Leader != 5 || len(body.Waves) != 1 {
		t.Fatalf("fortress CRA body = %#v", body)
	}
	wave := body.Waves[0]
	if wave.Left.Units[0][0] != GameData.DirewolfUnitID || wave.Left.Units[0][1] <= 0 ||
		wave.Right.Units[0][0] != GameData.DirewolfUnitID || wave.Right.Units[0][1] <= 0 ||
		wave.Middle.Units[0] != (attackPair{-1, 0}) {
		t.Fatalf("fortress formation is not one full Direwolf flank wave: %#v", wave)
	}
}

func TestFortressPersonalCooldownSurvivesReadyGlobalMapRow(t *testing.T) {
	gameData := fortressIntentGameData(t)
	now := time.Now().UTC()
	gameState := fortressIntentState(now)
	gameState.TowerCooldowns["1:101:100"] = State.TowerCooldownState{
		KingdomID: 1, TargetTypeID: State.MapTypeKingdomFortress, X: 101, Y: 100,
		LastSuccessfulBattleAt: now.Add(-24 * time.Hour), CooldownObservedAt: now, CooldownRemaining: 0,
	}
	_, _, _, _, err := fortressAttackContext(Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"kingdomId":1,"targetX":101,"targetY":100,
		"commanderIds":[5],"horseTravelBoostId":-1,"minimumCommanderSpeedBonus":100
	}`), now, true)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("personal five-day cooldown was not enforced: %v", err)
	}
}

func TestPlanFortressMapScanUsesOneAdaptiveFullMapAction(t *testing.T) {
	state := fortressIntentState(time.Now().UTC())
	source := state.Castles[10]
	source.Focused = true
	state.Castles[10] = source
	plan, err := planFortressMapScan(t.Context(), Intent.PlanningContext{State: state}, json.RawMessage(`{
		"sourceCastleId":10,"kingdomId":1
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "fortress.scan.full" || plan.Steps[0].Opcode != "" {
		t.Fatalf("Fortress full-map plan = %#v", plan.Steps)
	}
	var request map[string]any
	if err := json.Unmarshal(plan.Steps[0].ActionArguments, &request); err != nil {
		t.Fatal(err)
	}
	if _, retained := request["radius"]; retained {
		t.Fatalf("legacy Fortress radius leaked into the normalized request: %#v", request)
	}
	if request["scanStartedAt"] == nil || !strings.Contains(plan.Summary, "every fortress") {
		t.Fatalf("Fortress full-map metadata = %#v summary=%q", request, plan.Summary)
	}
}

func TestDiscoverFullFortressMapExpandsFarBeyondTheOldRadius(t *testing.T) {
	source := State.CastleState{X: 135, Y: 135}
	visited := map[fortressMapChunk]bool{}
	result, err := discoverFullFortressMap(t.Context(), source, func(_ context.Context, window towerMapWindow) (bool, error) {
		chunk := fortressMapChunk{X: window.X1 / fortressMapChunkSize, Y: window.Y1 / fortressMapChunkSize}
		visited[chunk] = true
		return chunk.Y == 1 && chunk.X >= 0 && chunk.X <= 6, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !visited[fortressMapChunk{X: 6, Y: 1}] {
		t.Fatalf("full-map discovery never reached a connected chunk 405+ tiles from the source: %#v", visited)
	}
	if len(result.ContentWindows) != 7 || len(result.FailedChunks) != 0 {
		t.Fatalf("Fortress discovery result = %#v", result)
	}
	for _, window := range result.Windows {
		if width, height := window.X2-window.X1+1, window.Y2-window.Y1+1; width != fortressMapChunkSize || height != fortressMapChunkSize {
			t.Fatalf("Fortress GAA window is not %dx%d: %#v", fortressMapChunkSize, fortressMapChunkSize, window)
		}
	}
}

func TestDiscoverFullFortressMapReportsFailedChunkAsIncomplete(t *testing.T) {
	source := State.CastleState{X: 135, Y: 135}
	failed := fortressMapChunk{X: 2, Y: 1}
	result, err := discoverFullFortressMap(t.Context(), source, func(_ context.Context, window towerMapWindow) (bool, error) {
		chunk := fortressMapChunk{X: window.X1 / fortressMapChunkSize, Y: window.Y1 / fortressMapChunkSize}
		if chunk == failed {
			return false, errors.New("test timeout")
		}
		return chunk == (fortressMapChunk{X: 1, Y: 1}), nil
	})
	if err == nil || !strings.Contains(err.Error(), "incomplete") || len(result.FailedChunks) != 1 || result.FailedChunks[0] != failed {
		t.Fatalf("failed Fortress chunk result = %#v err=%v", result, err)
	}
}

func TestFortressMapWindowWaitsForCommittedCorrelatedResponse(t *testing.T) {
	observer := newStormMapBurstTestObserver()
	sender := &stormMapBurstTestSender{
		observer: observer, expectedSends: 1,
		responsePayload: json.RawMessage(`{"KID":1,"AI":[[11,45,46,0,45,0,0,1]]}`),
	}
	hasContent, err := runFortressMapGAAWindow(
		Outbound.WithMetadata(t.Context(), Outbound.Metadata{OperationID: "fortress-map-test"}),
		sender, observer, nil, 1,
		towerMapWindow{X1: 0, Y1: 0, X2: 89, Y2: 89},
		"fortress-map-test/fortress-gaa/1",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !hasContent || len(sender.frames) != 1 || sender.frames[0].Opcode != "gaa" ||
		string(sender.frames[0].Payload) != `{"KID":1,"AX1":0,"AY1":0,"AX2":89,"AY2":89}` {
		t.Fatalf("Fortress GAA send = content:%t frames:%#v", hasContent, sender.frames)
	}
	if len(sender.metadata) != 1 || sender.metadata[0].ResponseToken != "fortress-map-test/fortress-gaa/1" ||
		sender.metadata[0].ResponseTimeoutMillis != int(fortressMapResponseTimeout/time.Millisecond) {
		t.Fatalf("Fortress GAA response metadata = %#v", sender.metadata)
	}
	observer.mu.Lock()
	waited := append([]uint64(nil), observer.waited...)
	observer.mu.Unlock()
	if len(waited) != 1 || waited[0] != 1 {
		t.Fatalf("Fortress GAA committed responses = %v", waited)
	}
}

func TestCaptureFullFortressMapRemovesOnlyStaleTargetsInsideScannedWindows(t *testing.T) {
	startedAt := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[10] = State.CastleState{ID: 10, KingdomID: 1, SlotType: 12}
	state.Map[1] = map[string]State.MapObservation{
		"10:10":   {KingdomID: 1, X: 10, Y: 10, TypeID: State.MapTypeKingdomFortress, ObservedAt: startedAt.Add(-time.Hour)},
		"20:20":   {KingdomID: 1, X: 20, Y: 20, TypeID: State.MapTypeKingdomFortress, ObservedAt: startedAt.Add(time.Second)},
		"200:200": {KingdomID: 1, X: 200, Y: 200, TypeID: State.MapTypeKingdomFortress, ObservedAt: startedAt.Add(-time.Hour)},
	}
	application := &Application{State: State.NewStore(state)}
	if err := application.captureFullFortressMap(
		fortressMapRequest{SourceCastleID: 10, KingdomID: 1, ScanStartedAt: startedAt},
		[]towerMapWindow{{X1: 0, Y1: 0, X2: 89, Y2: 89}},
	); err != nil {
		t.Fatal(err)
	}
	view := application.State.ReadOnlyView()
	if _, exists := view.LookupMapObservation(1, "10:10"); exists {
		t.Fatal("stale Fortress survived a completed scan of its window")
	}
	if _, exists := view.LookupMapObservation(1, "20:20"); !exists {
		t.Fatal("fresh Fortress was removed from a completed scan window")
	}
	if _, exists := view.LookupMapObservation(1, "200:200"); !exists {
		t.Fatal("Fortress outside completed scan windows was removed")
	}
}

func fortressIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":277,"type":"Elitetinoswolves","name":"Eventunit","comment1":"Nomad Shop"}],
		"effects":[
			{"effectID":2106,"name":"relicSpeedBonus","effectTypeID":15,"capID":1006},
			{"effectID":426,"name":"speedBonus","effectTypeID":15,"capID":99}
		],
		"effectCaps":[{"capID":1006,"maxTotalBonus":100}],
		"globalEffects":[{"ID":10,"globalEffectID":2,"name":"SpeedBoost","effects":"426&60","boostValue":60}],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func fortressIntentState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 1, SlotType: 12, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{GameData.DirewolfUnitID: 10_000}},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 5001}}
	gameState.Inventory.Equipment[5001] = State.EquipmentInstance{
		ID: 5001, Slot: 1, RarityID: 5, Effects: State.EquipmentEffects{
			{DefinitionID: 2106, Values: []float64{100}},
			{DefinitionID: 1, Values: []float64{1}},
			{DefinitionID: 2, Values: []float64{1}},
			{DefinitionID: 3, Values: []float64{1}},
		},
	}
	gameState.Map[1] = map[string]State.MapObservation{
		"101:100": {KingdomID: 1, X: 101, Y: 100, TypeID: State.MapTypeKingdomFortress, Level: 45, ObservedAt: now},
	}
	return gameState
}
