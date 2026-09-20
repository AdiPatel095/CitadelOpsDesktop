package Ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const capturedKhanNPCLeader = `{"WID":2,"GID":109,"AIE":[],"VIS":1001,"GEM":[],"GASAIDS":[],"SIDS":[],"AE":[]}`

func khanNPCMovementFixture(leader string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{
		"M":{"MID":87591337,"PT":115,"TT":400,"D":0,"TID":15825042,"T":20,"KID":0,"OID":-801,
			"TA":[1,212,941,15246649,15825042],"SA":[35,216,932,-1,-1]},
		"UM":{"PWD":0,"TWD":0,"L":%s}
	}`, leader))
}

func TestParseMovementAcceptsCapturedKhanNPCLeaderWithoutCommander(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	movement, ok := parseMovement(khanNPCMovementFixture(capturedKhanNPCLeader), now, nil)
	if !ok {
		t.Fatal("captured Khan NPC movement was rejected")
	}
	if movement.LeaderWID == nil || *movement.LeaderWID != 2 || movement.LeaderID != nil ||
		movement.LeaderDLID != nil || movement.CommanderID != nil {
		t.Fatalf("Khan NPC leader identity = %#v", movement)
	}
	if movement.ID != 87591337 || movement.TypeID != 20 || movement.SourceTypeID != 35 ||
		movement.OwnerPlayerID != -801 || movement.TargetPlayerID != 15825042 ||
		movement.TargetCastleID != 15246649 || movement.ArrivesAt == nil ||
		!movement.ArrivesAt.Equal(now.Add(285*time.Second)) {
		t.Fatalf("captured Khan movement fields = %#v", movement)
	}
	encoded, err := json.Marshal(movement)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"leaderWid":2`) {
		t.Fatalf("NPC leader identity missing from state contract: %s", encoded)
	}
}

func TestKhanNPCLeaderValidationFailsClosed(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	for _, leader := range []string{
		`{}`, `{"GID":109}`, `{"WID":null}`, `{"WID":"2"}`, `{"WID":1.5}`,
		`{"WID":true}`, `{"WID":[]}`, `{"WID":0}`, `{"WID":-1}`,
		`{"WID":9223372036854775808}`,
		`{"ID":null,"WID":2}`, `{"ID":"7","WID":2}`,
		`{"DLID":null,"WID":2}`, `{"DLID":"-14","WID":2}`,
		`{"ID":7,"WID":null}`, `{"DLID":-14,"WID":false}`,
	} {
		t.Run(leader, func(t *testing.T) {
			if movement, ok := parseMovement(khanNPCMovementFixture(leader), now, nil); ok {
				t.Fatalf("malformed Khan NPC leader parsed as %#v", movement)
			}
		})
	}

	nonKhan := json.RawMessage(`{
		"M":{"MID":51,"PT":1,"TT":60,"D":0,"T":0,"KID":0,"OID":1,"TID":99,
			"SA":[0,10,11,100,1],"TA":[0,20,21,300,99]},
		"UM":{"L":{"WID":2}}
	}`)
	if movement, ok := parseMovement(nonKhan, now, nil); ok {
		t.Fatalf("WID rescued a non-Khan movement: %#v", movement)
	}
}

func TestKhanNPCMovementCaptureSupportsGAMAndAAM(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name          string
		authoritative bool
		payload       json.RawMessage
	}{
		{name: "gam", authoritative: true, payload: json.RawMessage(fmt.Sprintf(`{"M":[%s]}`, khanNPCMovementFixture(capturedKhanNPCLeader)))},
		{name: "aam", authoritative: false, payload: json.RawMessage(fmt.Sprintf(`{"AAM":%s}`, khanNPCMovementFixture(capturedKhanNPCLeader)))},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := khanNPCMovementState(now)
			code := 0
			_, changed, err := newMovementReducer(test.authoritative)(t.Context(), Protocol.Frame{
				Opcode: test.name, Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: now, Payload: test.payload,
			}, &gameState, nil)
			if err != nil || !changed {
				t.Fatalf("%s capture: changed=%t err=%v", test.name, changed, err)
			}
			movement, found := gameState.LookupMovement(87591337)
			if !found || movement.CommanderID != nil || movement.LeaderWID == nil || *movement.LeaderWID != 2 {
				t.Fatalf("%s captured movement = %#v", test.name, movement)
			}
			if !gameState.Commanders[2].Available {
				t.Fatal("NPC WID occupied the same-numbered player commander")
			}
			if gameState.Khan.TauntsObserved != 1 || len(gameState.Khan.Taunts) != 1 {
				t.Fatalf("%s Khan counters = %#v", test.name, gameState.Khan)
			}
			activity, found := gameState.LookupEventActivity(72)
			if !found || activity.KhanDefense.Launches != 1 || len(activity.PendingAttacks) != 1 {
				t.Fatalf("%s Khan defense activity = %#v", test.name, activity)
			}
		})
	}
}

func TestCapturedKhanNPCMovementDrivesPolicyResolutionAndFeatureStatsOnce(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	gameState := khanNPCMovementState(now)
	code := 0
	reducer := newMovementReducer(true)
	_, changed, err := reducer(t.Context(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(fmt.Sprintf(`{"M":[%s]}`, khanNPCMovementFixture(capturedKhanNPCLeader))),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("capture Khan retaliation: changed=%t err=%v", changed, err)
	}

	policySnapshot := khanNPCPolicySnapshot(t, gameState, now)
	decision, err := Automation.NewAutoKhanRagePolicy().Evaluate(t.Context(), policySnapshot)
	if err != nil || decision.Status != "resolving" ||
		decision.Detail != "Tracking 1 active Khan retaliation(s) through resolution" ||
		decision.Metrics["tauntsObserved"] != 1 || decision.Metrics["activeTaunts"] != 1 {
		t.Fatalf("active Khan policy decision = %#v err=%v", decision, err)
	}

	_, changed, err = reduceMovementRemoval(t.Context(), Protocol.Frame{
		Opcode: "mrm", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: now.Add(286 * time.Second), Payload: json.RawMessage(`{"MID":87591337}`),
	}, &gameState, nil)
	if err != nil || !changed || len(gameState.Khan.Taunts) != 0 || gameState.Khan.TauntsResolved != 1 {
		t.Fatalf("resolved Khan retaliation: changed=%t err=%v state=%#v", changed, err, gameState.Khan)
	}
	policySnapshot.State = gameState
	policySnapshot.Now = now.Add(286 * time.Second)
	decision, err = Automation.NewAutoKhanRagePolicy().Evaluate(t.Context(), policySnapshot)
	if err != nil || decision.Status != "idle" || decision.Metrics["tauntsObserved"] != 1 ||
		decision.Metrics["tauntsResolved"] != 1 || decision.Metrics["activeTaunts"] != 0 {
		t.Fatalf("resolved Khan policy decision = %#v err=%v", decision, err)
	}

	report := State.BattleReportCapture{
		MessageID: 501, ReportID: 601, OccurredAt: now.Add(285 * time.Second), CapturedAt: now.Add(286 * time.Second),
		Summary: json.RawMessage(`{"MID":501,"LID":601,"PBI":[[-801,0,50,-50],[15825042,1,100,-5]],"AI":{"AT":1,"K":0,"X":212,"Y":941}}`),
		Details: json.RawMessage(`{"LID":601,"W":[[[-801,[[],[]]],[15825042,[[[1,100,-5]],[[701,3,-3]]]]]]}`),
	}
	changed, err = reconcileEventBattleActivity(&gameState, &report)
	if err != nil || !changed {
		t.Fatalf("Khan defense report: changed=%t err=%v", changed, err)
	}
	changed, err = reconcileEventBattleActivity(&gameState, &report)
	if err != nil || changed {
		t.Fatalf("duplicate Khan defense report: changed=%t err=%v", changed, err)
	}
	activity, _ := gameState.LookupEventActivity(72)
	if activity.KhanDefense.Launches != 1 || activity.KhanDefense.Battles != 1 ||
		activity.KhanDefense.Victories != 1 || activity.KhanDefense.Defeats != 0 ||
		activity.KhanDefense.TroopLosses != 5 || activity.KhanDefense.ToolsUsed != 3 ||
		len(activity.PendingAttacks) != 0 || len(activity.ProcessedReportIDs) != 1 ||
		report.MovementID != 87591337 || report.AutomationFeature != State.AttackFeatureAutoKhan ||
		report.EventActivity != State.EventActivityKhanDefense {
		t.Fatalf("Khan Feature Stats/report attribution: activity=%#v report=%#v", activity, report)
	}
}

func TestMixedMalformedKhanGAMDoesNotAdvanceAuthoritativeFreshness(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	gameState := khanNPCMovementState(now)
	gameState.MovementSnapshot = State.MovementSnapshot{Version: 4, ObservedAt: now.Add(-time.Minute)}
	code := 0
	malformed := strings.Replace(string(khanNPCMovementFixture(capturedKhanNPCLeader)), `"PT":115`, `"PT":"115"`, 1)
	_, _, err := newMovementReducer(true)(t.Context(), Protocol.Frame{
		Opcode: "gam", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(fmt.Sprintf(`{"M":[%s,%s]}`, khanNPCMovementFixture(capturedKhanNPCLeader), malformed)),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gameState.MovementSnapshot.Version != 4 || !gameState.MovementSnapshot.ObservedAt.Equal(now.Add(-time.Minute)) {
		t.Fatalf("mixed malformed GAM advanced freshness: %#v", gameState.MovementSnapshot)
	}
}

func khanNPCMovementState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Player.ID = 15825042
	gameState.Commanders[2] = State.CommanderState{ID: 2, Available: true}
	gameState.Castles[15246649] = State.CastleState{
		ID: 15246649, KingdomID: 0, SlotType: 1, X: 212, Y: 941,
	}
	gameState.Khan.TargetX = 216
	gameState.Khan.TargetY = 932
	gameState.Khan.RageCampID = 1145
	gameState.Khan.RageCampRevision = 1
	gameState.Khan.RageCampObservedAt = now
	gameState.Khan.RageBalanceCampRevision = 1
	gameState.Khan.PlayerRage = 1_000
	gameState.Khan.PlayerRageCap = 1_740
	gameState.Khan.PlayerTotalRage = 1_000
	gameState.Khan.RageObservedAt = now
	gameState.EventScores.ActiveEventID = 72
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{
		EventID: 72, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.EventScores.ActivityByEvent[72] = State.EventActivityState{
		EventID: 72, OccurrenceEndsAt: now.Add(7_200 * time.Second), ObservedFrom: now.Add(-time.Hour),
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"216:932": {KingdomID: 0, TypeID: 35, X: 216, Y: 932, EventCampID: 1145, ObjectID: 1145, ObservedAt: now},
	}
	return gameState
}

func khanNPCPolicySnapshot(t *testing.T, gameState State.GameState, now time.Time) Automation.Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return Automation.Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoKhan": json.RawMessage(`{
				"version":1,"sourceCastleId":15246649,"attackPresetId":"camp","defensePresetId":"defense",
				"minimumRemainingSec":300,"checkIntervalSec":30,"openGateProtection":false
			}`),
			"defense.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"defense","name":"Defense",
				"wall":{"left":{},"middle":{},"right":{}},
				"moat":{"leftToolSlots":[],"middleToolSlots":[],"rightToolSlots":[]}}]
			}`),
		}},
	}
}
