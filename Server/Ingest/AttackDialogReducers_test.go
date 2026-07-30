package Ingest

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestReduceAttackDialogStoresSourceTargetAndActiveEffects(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 12, 14, 30, 0, 0, time.UTC)
	_, changed, err := reduceAttackDialog(t.Context(), Protocol.Frame{
		Opcode: "adi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"KID":2,"SCID":100,
			"gaa":{"AI":[34,205,938,70,-1,0]},
			"AE":[[66,[48],"CI"],[503,[65],"CI"],[99,[],"ignored"],[0,[5],"ignored"]]
		}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("attack dialog: changed=%t err=%v", changed, err)
	}
	dialog := gameState.AttackDialog
	if dialog.SourceCastleID != 100 || dialog.KingdomID != 2 || dialog.Target.TypeID != 34 || dialog.Target.X != 205 || dialog.Target.Y != 938 || dialog.Target.ObjectID != 70 {
		t.Fatalf("unexpected dialog metadata: %#v", dialog)
	}
	if !dialog.ObservedAt.Equal(observedAt) || len(dialog.ActiveEffects) != 2 {
		t.Fatalf("unexpected dialog effects: %#v", dialog)
	}
	if dialog.ActiveEffects[0].EffectID != 66 || dialog.ActiveEffects[0].Source != "CI" || len(dialog.ActiveEffects[0].Values) != 1 || dialog.ActiveEffects[0].Values[0] != 48 {
		t.Fatalf("unexpected first effect: %#v", dialog.ActiveEffects[0])
	}
}

func TestReduceAttackDialogStoresAuthoritativeRBCTowerProgression(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Now().UTC()
	_, changed, err := reduceAttackDialog(t.Context(), Protocol.Frame{
		Opcode: "adi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":0,"SCID":100,"gaa":{"AI":[2,101,102,-1,845,0,0]},"AE":[]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("RBC attack dialog: changed=%t err=%v", changed, err)
	}
	target := gameState.AttackDialog.Target
	if target.TypeID != towerMapTypeID || target.X != 101 || target.Y != 102 || target.ObjectID != -1 ||
		target.TowerVictoryCount != 845 || target.TowerCooldownRemaining != 0 {
		t.Fatalf("unexpected RBC dialog target: %#v", target)
	}
}

func TestReduceAttackDialogStoresKhanCooldown(t *testing.T) {
	gameState := State.NewGameState()
	battleAt := time.Now().UTC().Add(-2 * time.Second)
	gameState.Map[0] = map[string]State.MapObservation{
		"939:1123": {
			KingdomID: 0, TypeID: khanCampMapTypeID, X: 939, Y: 1123,
			EventCampID: 1145, Level: 105, ObservedAt: time.Now().UTC().Add(-time.Minute),
		},
	}
	gameState.NomadCamps.Cooldowns["0:939:1123"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 939, Y: 1123, LastSuccessfulBattleAt: battleAt, PendingCooldownRefresh: true,
	}
	code := 0
	observedAt := time.Now().UTC()
	domains, changed, err := reduceAttackDialog(t.Context(), Protocol.Frame{
		Opcode: "adi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"KID":0,"SCID":100,
			"gaa":{"AI":[35,939,1123,3352,-1,194,360,1825,17,1146,200,200,85]},
			"AE":[]
		}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("Khan attack dialog: changed=%t err=%v", changed, err)
	}
	target := gameState.AttackDialog.Target
	if target.TypeID != khanCampMapTypeID || target.X != 939 || target.Y != 1123 ||
		target.ObjectID != 1146 || target.EventCampID != 1146 || target.EventCampCooldownRemaining != 194 {
		t.Fatalf("unexpected Khan dialog target: %#v", target)
	}
	observation := gameState.Map[0]["939:1123"]
	if observation.TypeID != khanCampMapTypeID || observation.EventCampID != 1146 ||
		observation.EventCampCooldownRemaining != 194 || observation.Level != 105 ||
		!observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected tracked Khan cooldown: %#v", observation)
	}
	if !slices.Contains(domains, "map") || !slices.Contains(domains, "nomad-camps") {
		t.Fatalf("Khan cooldown update domains = %v", domains)
	}
	cooldown := gameState.NomadCamps.Cooldowns["0:939:1123"]
	if cooldown.PendingCooldownRefresh || cooldown.CooldownRemaining != 194 ||
		!cooldown.CooldownObservedAt.Equal(observedAt) {
		t.Fatalf("attack dialog did not refresh Khan cooldown tracker: %#v", cooldown)
	}
}

func TestReduceAttackDialogRefreshesTrackedStormOpportunity(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[{"IsleID":4,"type":"VILLAGEWOOD","dungeonlevel":70,"globalCooldown":115200,"occupationTime":14400}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Map[4] = map[string]State.MapObservation{
		"100:101": {KingdomID: 4, X: 100, Y: 101, TypeID: stormIslandMapTypeID, StormIsleID: 4},
	}
	gameState.Storm.Map.Targets = map[string]State.MapObservation{
		"100:101": gameState.Map[4]["100:101"],
	}
	code := 0
	observedAt := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
	_, changed, err := reduceAttackDialog(t.Context(), Protocol.Frame{
		Opcode: "adi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":4,"SCID":40,"gaa":{"AI":[24,100,101,3319,-403,0,0,0,4,100]},"AE":[]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("Storm attack dialog: changed=%t err=%v", changed, err)
	}
	target := gameState.AttackDialog.Target
	if target.OwnerID != -403 || !target.StormReadyAt.Equal(observedAt) ||
		!target.StormExpiresAt.Equal(observedAt.Add(100*time.Second)) {
		t.Fatalf("Storm dialog target = %#v", target)
	}
	tracked := gameState.Storm.Map.Targets["100:101"]
	if tracked.OwnerID != -403 || !tracked.StormReadyAt.Equal(observedAt) ||
		!tracked.StormExpiresAt.Equal(observedAt.Add(100*time.Second)) {
		t.Fatalf("tracked Storm opportunity = %#v", tracked)
	}
}
