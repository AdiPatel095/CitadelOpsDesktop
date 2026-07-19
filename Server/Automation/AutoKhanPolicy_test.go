package Automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	KhanDomain "CitadelDesktop/Server/Khan"
	"CitadelDesktop/Server/State"
)

func TestAutoKhanPolicyProtectsMainThresholdButAllowsOutpostSource(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	mainSnapshot := autoKhanPolicySnapshot(t, now, 1)
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), mainSnapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.open_gate" {
		t.Fatalf("main threshold decision: %#v err=%v", decision, err)
	}
	if decision.Metrics["offensiveWallUnits"] != 1_000 {
		t.Fatalf("main threshold metrics = %#v", decision.Metrics)
	}

	outpostSnapshot := autoKhanPolicySnapshot(t, now, 2)
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), outpostSnapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("outpost attack decision: %#v err=%v", decision, err)
	}
	var request struct {
		SourceCastleID     State.CastleID `json:"sourceCastleId"`
		MainCastleID       State.CastleID `json:"mainCastleId"`
		OpenGateProtection bool           `json:"openGateProtection"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != 2 || request.MainCastleID != 1 || request.OpenGateProtection {
		t.Fatalf("outpost request = %#v", request)
	}
}

func TestAutoKhanPolicyYieldsToPlayerAttackAndAutoStationButNotKhanTaunt(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	playerImpact := now.Add(time.Minute)
	snapshot.State.Movements[10] = State.MovementState{
		ID: 10, Direction: 0, TypeID: 0, OwnerPlayerID: 999, TargetPlayerID: 158,
		SourceTypeID: 1, SourceCastleID: 999, TargetTypeID: 1, TargetCastleID: 1, ArrivesAt: &playerImpact,
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "yielding" || !strings.Contains(decision.Detail, "Auto Station") {
		t.Fatalf("player threat decision: %#v err=%v", decision, err)
	}

	delete(snapshot.State.Movements, 10)
	tauntImpact := now.Add(30 * time.Second)
	snapshot.State.Movements[11] = State.MovementState{
		ID: 11, Direction: 1, TypeID: 2, SourceTypeID: 2, SourceCastleID: -1,
		TargetCastleID: 1, SourceX: 210, SourceY: 942, TargetX: 212, TargetY: 941, ReturnsAt: &tauntImpact,
	}
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.attack" {
		t.Fatalf("Khan taunt was mistaken for a player attack: %#v err=%v", decision, err)
	}

	stationImpact := now.Add(45 * time.Second)
	snapshot.State.Movements[12] = State.MovementState{
		ID: 12, Direction: 0, SourceCastleID: 1, TargetCastleID: 900, ArrivesAt: &stationImpact,
	}
	snapshot.State.Stationing["autoStation:1"] = State.StationingOperation{
		ID: "autoStation:1", Purpose: "autoStation", SourceCastleID: 1, TargetCastleID: 900, MovementID: 12,
	}
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "yielding" || !strings.Contains(decision.Detail, "moving troops") {
		t.Fatalf("Auto Station movement decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyKeepsExpiredProtectionLockedUntilDefenseRecovers(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 1)
	expiredAt := now.Add(-time.Minute)
	snapshot.State.Khan.Protection = State.KhanProtectionState{
		Active: true, CastleID: 1, OffensiveWallUnits: 1_000, OffensiveUnitThreshold: 1_000,
		TriggeredAt: now.Add(-6*time.Hour - time.Minute), GateOpenUntil: expiredAt,
		Reason: "Add defense units to continue",
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "blocked" {
		t.Fatalf("expired unsafe protection decision: %#v err=%v", decision, err)
	}

	main := snapshot.State.Castles[1]
	main.Defense.Inventory[489] = 1_001
	main.Defense.ObservedAt = now
	main.Defense.InventoryObservedAt = now
	snapshot.State.Castles[1] = main
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.protection.clear" {
		t.Fatalf("recovered protection decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyBlocksAnUnsafeCurrentRun(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.Khan = State.KhanState{
		RunID: "unsafe", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		Launches: []State.KhanLaunchState{}, Taunts: map[State.MovementID]State.KhanTauntState{},
		SafetyError: "a later launch would overtake the previous hit",
	}
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "blocked" || !strings.Contains(decision.Detail, "unsafe arrival order") {
		t.Fatalf("unsafe chain decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanPolicyStopsAtNomadPointThreshold(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	snapshot := autoKhanPolicySnapshot(t, now, 2)
	snapshot.State.EventScores.ByEvent[autoKhanEventID] = State.ScalableEventScore{
		EventID: autoKhanEventID, PlayerScore: 50_000, RemainingSec: 10, ObservedAt: now,
	}
	snapshot.Configuration.Sections["automation.autoKhan"] = json.RawMessage(`{
		"version":1,"sourceCastleId":2,"attackPresetId":"camp","defensePresetId":"defense",
		"minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,
		"mapRefreshIntervalSec":30,"skipCooldowns":true,"timeSkipReserve":{},
		"openGateProtection":false,"offensiveUnitThreshold":1000,"nomadPointThreshold":50000
	}`)
	decision, err := NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "khan.point_limit.protect" {
		t.Fatalf("point-limit decision: %#v err=%v", decision, err)
	}

	main := snapshot.State.Castles[1]
	gateOpenUntil := now.Add(6 * time.Hour)
	main.Defense.OpenGateUntil = &gateOpenUntil
	snapshot.State.Castles[1] = main
	decision, err = NewAutoKhanPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "protected" {
		t.Fatalf("protected point-limit decision: %#v err=%v", decision, err)
	}
}

func TestAutoKhanDefenseToolShopRouteUsesCapturedLunaTable(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Storm.LunaShopTableID = 14
	route, active := autoKhanDefenseToolShopRoute(gameState, GameData.DefenseToolShopPackage{
		PackageID: 244, PriceScope: GameData.DefenseToolPriceCastleResource, PriceID: GameData.StormAquamarineID,
	}, time.Now().UTC())
	if !active || route.EventID != 14 {
		t.Fatalf("Luna route = %#v active=%t", route, active)
	}
}

func autoKhanPolicySnapshot(t *testing.T, now time.Time, sourceCastleID State.CastleID) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":215,"role":"melee","fightType":"0"},
			{"wodID":489,"role":"melee","fightType":"1"}
		],
		"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defenseRaw := json.RawMessage(`{
		"version":1,
		"presets":[{
			"id":"defense","name":"Defense",
			"wall":{
				"left":{"toolSlots":[],"unitPercent":33,"unitTypePercent":100},
				"middle":{"toolSlots":[],"unitPercent":34,"unitTypePercent":100},
				"right":{"toolSlots":[],"unitPercent":33,"unitTypePercent":100}
			},
			"moat":{"leftToolSlots":[],"middleToolSlots":[],"rightToolSlots":[]}
		}]
	}`)
	defensePreset, err := KhanDomain.DecodeDefensePreset(defenseRaw, "defense")
	if err != nil {
		t.Fatal(err)
	}
	main := State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, SlotType: 1, X: 212, Y: 941,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 3_000}, Traveling: map[State.UnitID]int64{}},
		Defense: State.CastleDefenseState{
			Wall: State.DefenseWallState{
				Left: defensePreset.Wall.Left, Middle: defensePreset.Wall.Middle, Right: defensePreset.Wall.Right,
				UnitCount: 2_000,
			},
			Moat: State.DefenseMoatState{
				LeftToolSlots:   defensePreset.Moat.LeftToolSlots,
				MiddleToolSlots: defensePreset.Moat.MiddleToolSlots,
				RightToolSlots:  defensePreset.Moat.RightToolSlots,
			},
			MeleeUnitIDs: []State.UnitID{489, 215}, RangedUnitIDs: []State.UnitID{},
			Inventory:  map[State.UnitID]int64{489: 1_000, 215: 2_000},
			ObservedAt: now, InventoryObservedAt: now,
		},
	}
	outpost := State.CastleState{
		ID: 2, Name: "Attack Outpost", KingdomID: 0, SlotType: 2, X: 220, Y: 945,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 3_000}, Traveling: map[State.UnitID]int64{}},
	}
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Player.Currencies[1006] = 10
	gameState.Castles[1] = main
	gameState.Castles[2] = outpost
	gameState.Commanders[1] = State.CommanderState{ID: 1, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	gameState.EventScores.ActiveEventID = autoKhanEventID
	gameState.EventScores.ByEvent[autoKhanEventID] = State.ScalableEventScore{
		EventID: autoKhanEventID, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"210:942": {
			KingdomID: 0, TypeID: autoKhanCampTypeID, X: 210, Y: 942,
			TowerVictoryCount: autoKhanCampVictoryCount, Level: 90, ObservedAt: now,
		},
	}
	return Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoKhan": json.RawMessage(fmt.Sprintf(`{
				"version":1,"sourceCastleId":%d,"attackPresetId":"camp","defensePresetId":"defense",
				"minimumRemainingSec":300,"checkIntervalSec":30,"defenseRefreshIntervalSec":30,
				"mapRefreshIntervalSec":30,"skipCooldowns":true,"timeSkipReserve":{},
				"openGateProtection":true,"offensiveUnitThreshold":1000
			}`, sourceCastleID)),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"camp","name":"Khan Camp","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":215,"quantity":100}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
			"defense.presets": defenseRaw,
		}},
	}
}
