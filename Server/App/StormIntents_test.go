package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestStormMapScanWindowsCoverSixHundredBySixHundredInThirtySixRequests(t *testing.T) {
	windows := stormMapScanWindows(State.StormMapBounds{X1: 0, Y1: 0, X2: 605, Y2: 605})
	if len(windows) != 36 {
		t.Fatalf("window count = %d, want 36", len(windows))
	}
	if first := windows[0]; first != (towerMapWindow{X1: 0, Y1: 0, X2: 100, Y2: 100}) {
		t.Fatalf("first window = %#v", first)
	}
	if last := windows[len(windows)-1]; last != (towerMapWindow{X1: 505, Y1: 505, X2: 605, Y2: 605}) {
		t.Fatalf("last window = %#v", last)
	}
	for _, window := range windows {
		if width, height := window.X2-window.X1+1, window.Y2-window.Y1+1; width > 101 || height > 101 {
			t.Fatalf("oversized GAA window = %#v", window)
		}
	}
}

func TestPlanStormMapScanRejectsSecondAttemptInsideSixHours(t *testing.T) {
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 300, Y: 300, Focused: true}
	state.Storm.Map = State.StormMapState{
		SourceCastleID: 40,
		LastAttemptAt:  time.Now().UTC(),
		Targets:        map[string]State.MapObservation{},
	}
	request := json.RawMessage(`{
		"sourceCastleId":40,"fullMap":true,
		"bounds":{"x1":0,"y1":0,"x2":605,"y2":605}
	}`)
	if _, err := planStormMapScan(context.Background(), Intent.PlanningContext{State: state}, request); err == nil ||
		!strings.Contains(err.Error(), "one attempt every six hours") {
		t.Fatalf("second full-map attempt error = %v", err)
	}
}

func TestCaptureStormScanBuildsAuthoritativeMapState(t *testing.T) {
	startedAt := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.ServerURL = "storm-test.example"
	state.Player.ID = 99
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: stormIntentKingdomID, X: 300, Y: 300, Focused: true}
	state.Map[stormIntentKingdomID] = map[string]State.MapObservation{
		"20:20": {
			KingdomID: stormIntentKingdomID, X: 20, Y: 20, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 7, ObservedAt: startedAt.Add(-time.Second),
		},
		"100:100": {
			KingdomID: stormIntentKingdomID, X: 100, Y: 100, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 8, ObservedAt: startedAt.Add(time.Second),
		},
		"600:600": {
			KingdomID: stormIntentKingdomID, X: 600, Y: 600, TypeID: 12,
			ObservedAt: startedAt.Add(2 * time.Second),
		},
	}
	application := &Application{State: State.NewStore(state)}
	request, err := json.Marshal(stormMapScanRequest{
		SourceCastleID: 40,
		FullMap:        true,
		Bounds:         State.StormMapBounds{X1: 0, Y1: 0, X2: 605, Y2: 605},
		ScanStartedAt:  startedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.beginStormScan(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := application.captureStormScan(context.Background(), request); err != nil {
		t.Fatal(err)
	}

	snapshot := application.State.Snapshot()
	if _, stale := snapshot.Map[stormIntentKingdomID]["20:20"]; stale {
		t.Fatal("stale Storm fort survived authoritative sweep")
	}
	if len(snapshot.Storm.Map.Targets) != 1 || snapshot.Storm.Map.Targets["100:100"].StormIsleID != 8 {
		t.Fatalf("authoritative targets = %#v", snapshot.Storm.Map.Targets)
	}
	if snapshot.Storm.Map.WindowCount != 36 || snapshot.Storm.Map.CoveredBounds.X2 != 605 ||
		snapshot.Storm.Map.NextBounds.X2 != 706 || snapshot.Storm.Map.NextBounds.Y2 != 706 {
		t.Fatalf("Storm map coverage = %#v", snapshot.Storm.Map)
	}
	if !snapshot.Storm.Map.LastAttemptAt.Equal(startedAt) || snapshot.Storm.Map.LastCompletedAt.IsZero() {
		t.Fatalf("Storm scan timing = %#v", snapshot.Storm.Map)
	}
}

func TestStormAttackContextEnforcesMinimumFortWins(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":7,"type":"DUNGEON","dungeonlevel":40,"countVictories":"0#1#5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4}
	state.Map[4] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 4, X: 101, Y: 102, TypeID: stormIntentFortMapTypeID,
			StormIsleID: 7, StormVictoryCount: 4, ObservedAt: time.Now().UTC(),
		},
	}
	input := Intent.PlanningContext{State: state, GameData: gameData}
	request := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":5,"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err == nil || !strings.Contains(err.Error(), "below the required minimum") {
		t.Fatalf("minimum-win guard error = %v", err)
	}
	request = json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":4,"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err != nil {
		t.Fatalf("fort at minimum wins rejected: %v", err)
	}
	readyAt := time.Now().UTC().Add(-time.Minute)
	target := state.Map[4]["101:102"]
	target.StormVictoryCount = 0
	target.StormCooldownRemaining = 60
	target.StormReadyAt = readyAt
	target.ObservedAt = readyAt.Add(-time.Minute)
	state.Map[4]["101:102"] = target
	input.State = state
	request = json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":25,"targetX":101,"targetY":102,
		"stormIsleId":7,"minimumVictoryCount":5,"preset":{"id":"fort","name":"Fort","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err != nil {
		t.Fatalf("fort readyAt was not allowed to refresh authoritative wins: %v", err)
	}
}

func TestStormAttackContextUsesIslandReadyAndExpiryLabels(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":4,"type":"VILLAGEWOOD","dungeonlevel":70,"globalCooldown":115200,"occupationTime":14400}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4}
	target := State.MapObservation{
		KingdomID: 4, X: 101, Y: 102, TypeID: stormIntentIslandMapTypeID, OwnerID: -403,
		ObjectID: 777, StormIsleID: 4, StormCooldownRemaining: 3_600, StormReadyAt: now.Add(-time.Minute),
		StormExpiresAt: now.Add(time.Hour), ObservedAt: now.Add(-time.Minute),
	}
	state.Map[4] = map[string]State.MapObservation{"101:102": target}
	input := Intent.PlanningContext{State: state, GameData: gameData}
	request := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"targetTypeId":24,"targetX":101,"targetY":102,
		"stormIsleId":4,"preset":{"id":"island","name":"Island","waves":[]}
	}`)
	if _, _, _, _, err := stormAttackContext(input, request); err != nil {
		t.Fatalf("unoccupied island timer was treated as a cooldown: %v", err)
	}

	target.OwnerID = 99
	target.StormReadyAt = now.Add(time.Minute)
	state.Map[4]["101:102"] = target
	input.State = state
	if _, _, _, _, err := stormAttackContext(input, request); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("occupied island readyAt guard error = %v", err)
	}

	target.OwnerID = -403
	target.StormReadyAt = now.Add(-time.Minute)
	target.StormExpiresAt = now.Add(-time.Second)
	state.Map[4]["101:102"] = target
	input.State = state
	if _, _, _, _, err := stormAttackContext(input, request); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired island guard error = %v", err)
	}
}

func TestConsumeStormIslandTargetRecordsReportGatedReturn(t *testing.T) {
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4}
	state.Map[4] = map[string]State.MapObservation{
		"101:102": {KingdomID: 4, X: 101, Y: 102, TypeID: stormIntentIslandMapTypeID, ObjectID: 777},
	}
	state.Storm.Map.Targets["101:102"] = state.Map[4]["101:102"]
	application := &Application{State: State.NewStore(state)}
	arguments, err := json.Marshal(stormTargetConsumeRequest{
		SourceCastleID: 40, KingdomID: 4, TargetTypeID: stormIntentIslandMapTypeID,
		TargetX: 101, TargetY: 102, IslandObjectID: 777, LeaveBehind: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.consumeStormTarget(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	snapshot := application.State.Snapshot()
	operation := snapshot.Storm.IslandReturns[State.StormIslandReturnKey(4, 101, 102)]
	if operation.Status != State.StormIslandReturnAwaitingReport || operation.SourceCastleID != 40 ||
		operation.IslandObjectID != 777 || operation.LeaveBehind != 1 || operation.LaunchedAt.IsZero() {
		t.Fatalf("pending island return = %#v", operation)
	}
	if _, exists := snapshot.Map[4]["101:102"]; exists {
		t.Fatal("consumed island remained in the live map")
	}
}

func TestPlanStormIslandReturnUsesIslandAsSourceAndStormCastleAsDestination(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":10},{"wodID":12}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[40] = State.CastleState{ID: 40, KingdomID: 4, X: 200, Y: 300, Focused: true}
	key := State.StormIslandReturnKey(4, 101, 102)
	state.Storm.IslandReturns[key] = State.StormIslandReturnState{
		KingdomID: 4, SourceCastleID: 40, TargetX: 101, TargetY: 102,
		IslandObjectID: 777, ReportID: 202, Status: State.StormIslandReturnReady, LeaveBehind: 1,
		Survivors: map[State.UnitID]int64{10: 4, 12: 5}, ReportedAt: time.Now().UTC(),
	}
	request := json.RawMessage(`{
		"sourceCastleId":40,"kingdomId":4,"islandX":101,"islandY":102,
		"islandObjectId":777,"reportId":202,"units":[{"unitId":10,"amount":4},{"unitId":12,"amount":4}]
	}`)
	plan, err := planStormIslandReturn(context.Background(), Intent.PlanningContext{State: state, GameData: gameData}, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Opcode != "jaa" || plan.Steps[1].Opcode != "sdi" ||
		plan.Steps[2].Action != "storm.island.return.guard" || plan.Steps[3].Opcode != "cds" ||
		plan.Steps[4].Action != "storm.island.return.complete" {
		t.Fatalf("island return steps = %#v", plan.Steps)
	}
	var route struct {
		TargetX int `json:"TX"`
		TargetY int `json:"TY"`
		SourceX int `json:"SX"`
		SourceY int `json:"SY"`
	}
	if err := json.Unmarshal(plan.Steps[1].Command.Payload, &route); err != nil {
		t.Fatal(err)
	}
	if route.TargetX != 200 || route.TargetY != 300 || route.SourceX != 101 || route.SourceY != 102 {
		t.Fatalf("island return route = %#v", route)
	}
	var dispatch struct {
		SourceID int64      `json:"SID"`
		TargetX  int        `json:"TX"`
		TargetY  int        `json:"TY"`
		Wait     int        `json:"WT"`
		Units    [][2]int64 `json:"A"`
	}
	if err := json.Unmarshal(plan.Steps[3].Command.Payload, &dispatch); err != nil {
		t.Fatal(err)
	}
	if dispatch.SourceID != 777 || dispatch.TargetX != 200 || dispatch.TargetY != 300 || dispatch.Wait != 0 ||
		len(dispatch.Units) != 2 || dispatch.Units[0] != [2]int64{10, 4} || dispatch.Units[1] != [2]int64{12, 4} {
		t.Fatalf("island return dispatch = %#v", dispatch)
	}
}
