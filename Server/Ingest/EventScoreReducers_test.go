package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestScalableEventScoresTrackSnapshotsAndPointUpdates(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	observedAt := time.Date(2026, 7, 13, 20, 23, 4, 0, time.UTC)
	code := 0

	_, changed, err := reduceScalableEventSnapshot(t.Context(), Protocol.Frame{
		Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"E":[
			{"EID":72,"RS":25616,"SP":{"OP":1500,"OR":25},"A":{"OP":184426,"OR":150},"EASE":1,"EDID":308,"PIDS":"10, 11,bad","RCKS":["GTO","STO","st","ST"],
				"AC":{"ACID":1147,"AR":10000,"PCRP":1740,"PTRP":52140}},
			{"EID":68,"RS":500,"PID":[12,13],"AC":0},
			{"EID":69,"RS":400,"PID":14,"A":[]}
		]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("event snapshot: changed=%t err=%v", changed, err)
	}
	score, found := gameState.ActiveScalableEventScore()
	if !found || score.EventID != 72 || score.DifficultyTypeName != "expertPlus" || score.PlayerScore != 1500 || score.AllianceScore != 184426 {
		t.Fatalf("unexpected event score: %#v found=%t", score, found)
	}
	if !gameState.ScalableEventScoreReached(72, 1500) || gameState.ScalableEventScoreReached(72, 1501) || !gameState.ActiveScalableEventScoreReached(1500) {
		t.Fatalf("threshold helper did not use the player score: %#v", gameState.EventScores)
	}
	if got := gameState.Invasion.FortifyCurrencies; len(got) != 3 || got[0] != "GTO" || got[1] != "STO" || got[2] != "ST" {
		t.Fatalf("invasion fortification currencies = %#v", got)
	}
	activity := gameState.EventScores.ActivityByEvent[72]
	if got := activity.FortifyCurrencies; len(got) != 3 || got[0] != "GTO" || got[1] != "STO" || got[2] != "ST" ||
		!activity.FortifyCurrenciesObservedAt.Equal(observedAt) {
		t.Fatalf("event fortification currency snapshot = %#v at %s", got, activity.FortifyCurrenciesObservedAt)
	}
	if gameState.Khan.RageCampID != 1147 || gameState.Khan.PlayerRage != 1740 ||
		gameState.Khan.PlayerRageCap != 1740 || gameState.Khan.PlayerTotalRage != 52140 ||
		!gameState.Khan.RageObservedAt.Equal(observedAt) {
		t.Fatalf("Khan rage snapshot = %#v", gameState.Khan)
	}
	for packageID, eventID := range map[State.PackageID]int64{10: 72, 11: 72, 12: 68, 13: 68, 14: 69} {
		route, active := gameState.ActiveShopForPackage(packageID, observedAt.Add(time.Second))
		if !active || route.EventID != eventID {
			t.Fatalf("package %d route = %#v active=%t", packageID, route, active)
		}
	}
	if _, active := gameState.EventAvailable(72, observedAt.Add(time.Second)); !active ||
		!gameState.EventScores.Inventory.ObservedAt.Equal(observedAt.Truncate(time.Minute)) {
		t.Fatalf("authoritative event inventory = %#v", gameState.EventScores.Inventory)
	}

	_, changed, err = reduceEventPoints(t.Context(), Protocol.Frame{
		Opcode: "pep", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Minute),
		Payload: json.RawMessage(`{"OP":[2200,190000],"OR":[18,140],"PT":[0],"EID":72}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("event points: changed=%t err=%v", changed, err)
	}
	score, _ = gameState.ActiveScalableEventScore()
	if score.PlayerScore != 2200 || score.AllianceScore != 190000 || score.PlayerRank != 18 || score.AllianceRank != 140 || score.RemainingSec != 25556 {
		t.Fatalf("point update was not applied: %#v", score)
	}

	_, changed, err = reduceKhanRagePoints(t.Context(), Protocol.Frame{
		Opcode: "rpr", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(2 * time.Minute),
		Payload: json.RawMessage(`{"EID":72,"PCRP":150,"PTRP":52290}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Khan.PlayerRage != 150 ||
		gameState.Khan.PlayerRageCap != 1740 || gameState.Khan.PlayerTotalRage != 52290 {
		t.Fatalf("Khan rage update: changed=%t state=%#v err=%v", changed, gameState.Khan, err)
	}

}

func TestScalableEventShopRoutesMergeCatalogAndLivePackages(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"events":[{"eventID":22,"packageIDs":"1123+1124","kIDs":"0","areaTypes":"1"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	observedAt := time.Date(2026, 9, 19, 22, 31, 32, 0, time.UTC)
	changed, err := applyScalableEventSnapshot(json.RawMessage(`{"E":[{"EID":22,"RS":293248}]}`), observedAt, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("event shop snapshot changed=%t err=%v", changed, err)
	}
	for _, packageID := range []State.PackageID{1123, 1124} {
		route, active := gameState.ActiveShopForPackage(packageID, observedAt.Add(time.Minute))
		if !active || route.EventID != 22 {
			t.Fatalf("package %d route=%+v active=%t", packageID, route, active)
		}
	}
	changed, err = applyScalableEventSnapshot(json.RawMessage(`{"E":[{"EID":22,"RS":293247,"PIDS":"2001"}]}`), observedAt.Add(time.Second), &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("event shop additions changed=%t err=%v", changed, err)
	}
	for _, packageID := range []State.PackageID{1123, 1124, 2001} {
		route, active := gameState.ActiveShopForPackage(packageID, observedAt.Add(time.Minute))
		if !active || route.EventID != 22 {
			t.Fatalf("merged package %d route=%+v active=%t", packageID, route, active)
		}
	}
}

func TestAICCampUpgradeInvalidatesOldCapUntilCoherentRageUpdate(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"eventAutoScalingCamps":[
			{"eventAutoScalingCampID":1114,"eventID":72,"difficultyID":310,"areaType":35,"playerRageCap":1620},
			{"eventAutoScalingCampID":1115,"eventID":72,"difficultyID":310,"areaType":35,"playerRageCap":1740}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 19, 19, 42, 13, 0, time.UTC)
	endsAt := now.Add(time.Hour)
	gameState := State.NewGameState()
	gameState.EventScores.ByEvent[72] = State.ScalableEventScore{
		EventID: 72, DifficultyID: 310, RemainingSec: 3600, ObservedAt: now,
	}
	gameState.EventScores.ActivityByEvent[72] = State.EventActivityState{
		EventID: 72, OccurrenceEndsAt: endsAt, ObservedFrom: now.Add(-time.Hour),
	}
	gameState.Khan.TargetX = 216
	gameState.Khan.TargetY = 932
	gameState.Khan.RageCampID = 1114
	gameState.Khan.RageCampRevision = 1
	gameState.Khan.RageCampObservedAt = now.Add(-time.Second)
	gameState.Khan.RageBalanceCampRevision = 1
	gameState.Khan.PlayerRage = 1620
	gameState.Khan.PlayerRageCap = 1620
	gameState.Khan.PlayerTotalRage = 43539
	gameState.Khan.RageObservedAt = now.Add(-time.Second)
	store := State.NewStore(gameState)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil || !registry.HasInbound("aic") {
		t.Fatalf("AIC reducer registration: %v", err)
	}
	pipeline := NewPipeline(store, staticGameDataProvider{store: gameData}, registry)
	code := 0
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "aic", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"AC":[{"X":216,"Y":932,"AR":1182,"ACID":1115,"EID":72,"ACVC":1}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan := store.ReadOnlyView().Khan
	if khan.RageCampID != 1115 || khan.PlayerRageCap != 1740 || khan.PlayerRage != 1620 ||
		khan.PlayerTotalRage != 43539 || khan.RageCampRevision != 2 || khan.RageBalanceCampRevision != 0 {
		t.Fatalf("camp update = %+v", khan)
	}
	occurrence, _ := store.ReadOnlyView().LookupEventOccurrence(72)
	if khan.FullRageTauntDue(occurrence) {
		t.Fatal("old full-bar observation survived the camp upgrade")
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "rpr", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Millisecond),
		Payload: json.RawMessage(`{"EID":72,"PCRP":1620,"PTRP":45422}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.PlayerRageCap != 1740 || khan.RageBalanceCampRevision != khan.RageCampRevision || khan.FullRageTauntDue(occurrence) {
		t.Fatalf("1620 rage incorrectly became ready against upgraded cap: %+v", khan)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "rpr", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(2 * time.Millisecond),
		Payload: json.RawMessage(`{"EID":72,"PCRP":1740,"PTRP":45542}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if !khan.FullRageTauntDue(occurrence) {
		t.Fatalf("coherent 1740 rage did not become ready: %+v", khan)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "aic", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(3 * time.Millisecond),
		Payload: json.RawMessage(`{"AC":[{"X":216,"Y":932,"AR":1182,"ACID":1115,"EID":72,"ACVC":1}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.RageBalanceCampRevision != khan.RageCampRevision || !khan.FullRageTauntDue(occurrence) {
		t.Fatalf("repeated same-camp AIC invalidated coherent rage: %+v", khan)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "aic", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(-time.Second),
		Payload: json.RawMessage(`{"AC":[{"X":216,"Y":932,"AR":1182,"ACID":1114,"EID":72,"ACVC":0}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.ReadOnlyView().Khan.RageCampID; got != 1115 {
		t.Fatalf("older AIC rolled camp back to %d", got)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "rpr", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(4 * time.Millisecond),
		Payload: json.RawMessage(`{"EID":72,"PCRP":1740}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.PlayerRage != 1740 || khan.PlayerTotalRage != 45542 || khan.RageBalanceCampRevision != 0 {
		t.Fatalf("missing RPR fields fabricated rage state: %+v", khan)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(5 * time.Millisecond),
		Payload: json.RawMessage(`{"E":[{"EID":72,"RS":3599,"EASE":1,"EDID":310,"AC":{"ACID":1115,"PCRP":1740,"PTRP":45542}}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.RageBalanceCampRevision != khan.RageCampRevision || !khan.FullRageTauntDue(occurrence) {
		t.Fatalf("fresh SEI did not restore coherent rage: %+v", khan)
	}
	beforeOtherEvent := khan
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "aic", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(6 * time.Millisecond),
		Payload: json.RawMessage(`{"AC":[{"X":100,"Y":200,"ACID":9999,"EID":73,"ACVC":1}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.RageCampID != beforeOtherEvent.RageCampID || khan.RageCampRevision != beforeOtherEvent.RageCampRevision ||
		khan.RageBalanceCampRevision != beforeOtherEvent.RageBalanceCampRevision {
		t.Fatalf("unrelated camp event changed Khan rage state: before=%+v after=%+v", beforeOtherEvent, khan)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "aic", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(7 * time.Millisecond),
		Payload: json.RawMessage(`{"AC":[{"X":216,"Y":932,"ACID":9999,"EID":72,"ACVC":1}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.RageCampID != 0 || khan.PlayerRageCap != 0 || khan.RageBalanceCampRevision != 0 ||
		khan.PlayerRage != 1740 || khan.PlayerTotalRage != 45542 {
		t.Fatalf("unknown camp did not fail closed while preserving rage amounts: %+v", khan)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "aic", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(8 * time.Millisecond),
		Payload: json.RawMessage(`{"AC":[{"X":216,"Y":932,"ACID":1115,"EID":72,"ACVC":1}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "rpr", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(9 * time.Millisecond),
		Payload: json.RawMessage(`{"EID":72,"PCRP":1740,"PTRP":45542}`),
	}); err != nil {
		t.Fatal(err)
	}
	khan = store.ReadOnlyView().Khan
	if khan.RageCampID != 1115 || khan.RageBalanceCampRevision != khan.RageCampRevision || !khan.FullRageTauntDue(occurrence) {
		t.Fatalf("valid AIC plus RPR did not recover rage readiness: %+v", khan)
	}
}

func TestScalableEventSnapshotReplacesAuthoritativeAvailability(t *testing.T) {
	gameState := State.NewGameState()
	gameData := scalableEventTestGameData(t)
	code := 0
	start := time.Date(2026, 8, 14, 8, 0, 4, 0, time.UTC)
	apply := func(at time.Time, payload string) {
		t.Helper()
		_, changed, err := reduceScalableEventSnapshot(t.Context(), Protocol.Frame{
			Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: at,
			Payload: json.RawMessage(payload),
		}, &gameState, gameData)
		if err != nil || !changed {
			t.Fatalf("event inventory at %s: changed=%t err=%v", at, changed, err)
		}
	}

	apply(start, `{"E":[{"EID":71,"RS":1800,"EASE":1},{"EID":3,"RS":3600}]}`)
	if _, active := gameState.EventAvailable(71, start); !active {
		t.Fatal("scalable event missing from authoritative inventory")
	}
	if _, active := gameState.EventAvailable(3, start); !active {
		t.Fatal("non-scalable Berimond event missing from authoritative inventory")
	}

	settled := start.Add(6 * time.Minute)
	apply(settled, `{"E":[]}`)
	if _, active := gameState.EventAvailable(71, settled); active {
		t.Fatal("event omitted from the replacement snapshot remained active")
	}
	if len(gameState.EventScores.Inventory.ActiveByEvent) != 0 ||
		!gameState.EventScores.Inventory.ObservedAt.Equal(settled.Truncate(time.Minute)) {
		t.Fatalf("replacement inventory = %#v", gameState.EventScores.Inventory)
	}
}

func TestTriggerEventSnapshotsBindLiveOfferAndBoostStatusToDailyWindow(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	observedAt := time.Date(2026, time.September, 2, 17, 0, 0, 0, time.UTC)
	code := 0
	_, changed, err := reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
		Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"TE":[
			{"TRID":610,"GE":[[2,1800,60]],"SGE":[]},
			{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":60}]}
		]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("global-effect TEI: changed=%t err=%v", changed, err)
	}
	wantEndsAt := observedAt.Add(30 * time.Minute).Truncate(time.Minute)
	effect := gameState.EventScores.Inventory.GlobalEffects[2]
	offer := gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2]
	if effect.GlobalEffectID != 2 || effect.Strength != 60 || !effect.EndsAt.Equal(wantEndsAt) ||
		offer.GlobalEffectID != 2 || offer.RubyCost != 2500 || offer.BonusValue != 60 {
		t.Fatalf("global-effect state = effect:%+v offer:%+v", effect, offer)
	}

	_, changed, err = reduceGlobalEffectBoosterInfo(t.Context(), Protocol.Frame{
		Opcode: "bie", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: observedAt.Add(time.Second), Payload: json.RawMessage(`{"GE":[]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("unboosted BIE: changed=%t err=%v", changed, err)
	}
	status := gameState.EventScores.Inventory.GlobalEffectBoosts[2]
	if status.Boosted || !status.OccurrenceEndsAt.Equal(wantEndsAt) {
		t.Fatalf("unboosted status = %+v", status)
	}

	_, changed, err = reduceGlobalEffectBoosterInfo(t.Context(), Protocol.Frame{
		Opcode: "bie", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: observedAt.Add(2 * time.Second), Payload: json.RawMessage(`{"GE":[2]}`),
	}, &gameState, gameData)
	if err != nil || !changed || !gameState.EventScores.Inventory.GlobalEffectBoosts[2].Boosted {
		t.Fatalf("boosted BIE: status=%+v changed=%t err=%v", gameState.EventScores.Inventory.GlobalEffectBoosts[2], changed, err)
	}
}

func TestGlobalEffectBoosterInfoRequiresExplicitCurrentCodeZeroArray(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	observedAt := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	code := 0
	base := State.NewGameState()
	base.Session.ConnectionGeneration = 7
	base.EventScores.Inventory.GlobalEffectsObservedAt = observedAt
	base.EventScores.Inventory.GlobalEffects[2] = State.GlobalEffectAvailability{
		GlobalEffectID: 2, Strength: 10, EndsAt: observedAt.Add(time.Hour),
	}
	base.EventScores.Inventory.GlobalEffectBoostsObservedAt = observedAt
	base.EventScores.Inventory.GlobalEffectBoosts[2] = State.GlobalEffectBoostState{
		GlobalEffectID: 2, Boosted: true, OccurrenceEndsAt: observedAt.Add(time.Hour),
		ObservedAt: observedAt, ConnectionGeneration: 7,
	}

	for name, payload := range map[string]string{
		"missing": `{}`,
		"null":    `{"GE":null}`,
		"object":  `{"GE":{}}`,
		"string":  `{"GE":"2"}`,
		"decimal": `{"GE":[2.5]}`,
		"zero":    `{"GE":[0]}`,
	} {
		t.Run(name, func(t *testing.T) {
			gameState := base
			gameState.EventScores.Inventory.GlobalEffects = cloneGlobalEffects(base.EventScores.Inventory.GlobalEffects)
			gameState.EventScores.Inventory.GlobalEffectBoosts = cloneGlobalEffectBoosts(base.EventScores.Inventory.GlobalEffectBoosts)
			_, changed, err := reduceGlobalEffectBoosterInfo(t.Context(), Protocol.Frame{
				Opcode: "bie", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt.Add(time.Second), Payload: json.RawMessage(payload),
			}, &gameState, gameData)
			if err == nil || changed || !gameState.EventScores.Inventory.GlobalEffectBoosts[2].Boosted {
				t.Fatalf("invalid BIE changed authority: changed=%t err=%v state=%+v", changed, err, gameState.EventScores.Inventory.GlobalEffectBoosts[2])
			}
		})
	}

	gameState := base
	_, changed, err := reduceGlobalEffectBoosterInfo(t.Context(), Protocol.Frame{
		Opcode: "bie", Direction: Protocol.DirectionInbound, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"GE":[]}`),
	}, &gameState, gameData)
	if err != nil || changed || !gameState.EventScores.Inventory.GlobalEffectBoosts[2].Boosted {
		t.Fatalf("nil response code manufactured BIE authority: changed=%t err=%v", changed, err)
	}

	gameState = base
	_, changed, err = reduceGlobalEffectBoosterInfo(t.Context(), Protocol.Frame{
		Opcode: "bie", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: observedAt.Add(-time.Second), Payload: json.RawMessage(`{"GE":[]}`),
	}, &gameState, gameData)
	if err != nil || changed || !gameState.EventScores.Inventory.GlobalEffectBoosts[2].Boosted {
		t.Fatalf("older BIE rebound to newer occurrence: changed=%t err=%v", changed, err)
	}
}

func TestGlobalEffectOccurrenceIdentitySurvivesCountdownJitterButRollsDaily(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	code := 0
	firstAt := time.Date(2026, time.September, 15, 12, 0, 10, 0, time.UTC)
	apply := func(at time.Time, remaining int) time.Time {
		t.Helper()
		_, _, err := reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
			Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: at,
			Payload: json.RawMessage(fmt.Sprintf(`{"TE":[{"TRID":610,"GE":[[2,%d,10]],"SGE":[]}]}`, remaining)),
		}, &gameState, gameData)
		if err != nil {
			t.Fatal(err)
		}
		return gameState.EventScores.Inventory.GlobalEffects[2].EndsAt
	}
	firstEnd := apply(firstAt, 50)
	jitteredEnd := apply(firstEnd.Add(10*time.Second), 50)
	if !jitteredEnd.Equal(firstEnd) {
		t.Fatalf("same occurrence changed after expiry jitter: first=%s next=%s", firstEnd, jitteredEnd)
	}
	nextEnd := apply(firstAt.Add(24*time.Hour), 50)
	if nextEnd.Equal(firstEnd) || !nextEnd.After(firstEnd.Add(23*time.Hour)) {
		t.Fatalf("daily rollover did not create a new occurrence: first=%s next=%s", firstEnd, nextEnd)
	}
}

func TestScalableEventSnapshotDoesNotEraseTriggerGlobalEffects(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	observedAt := time.Date(2026, time.September, 16, 17, 0, 0, 0, time.UTC)
	code := 0
	_, _, err := reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
		Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"TE":[
			{"TRID":610,"GE":[[2,1800,10]],"SGE":[]},
			{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":50}]}
		]}`),
	}, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	effect := gameState.EventScores.Inventory.GlobalEffects[2]
	offer := gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2]
	_, _, err = reduceScalableEventSnapshot(t.Context(), Protocol.Frame{
		Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"E":[{"EID":72,"RS":3600,"EASE":1,"EDID":308}]}`),
	}, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	if got := gameState.EventScores.Inventory.GlobalEffects[2]; got != effect {
		t.Fatalf("ordinary SEI replaced trigger effect: got=%+v want=%+v", got, effect)
	}
	if got := gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2]; got != offer {
		t.Fatalf("ordinary SEI replaced trigger offer: got=%+v want=%+v", got, offer)
	}
}

func TestGlobalEffectTriggerSnapshotStrictShapesAndElapsedRows(t *testing.T) {
	observedAt := time.Date(2026, time.September, 16, 17, 0, 0, 0, time.UTC)
	for name, payload := range map[string]string{
		"missing TE":       `{}`,
		"null TE":          `{"TE":null}`,
		"object TE":        `{"TE":{}}`,
		"missing GE":       `{"TE":[{"TRID":610}]}`,
		"null GE":          `{"TE":[{"TRID":610,"GE":null}]}`,
		"malformed GE row": `{"TE":[{"TRID":610,"GE":[[2,"soon",10]]}]}`,
		"overflow GE row":  `{"TE":[{"TRID":610,"GE":[[2,9223372036854775807,10]]}]}`,
		"missing GEB":      `{"TE":[{"TRID":612}]}`,
		"null GEB":         `{"TE":[{"TRID":612,"GEB":null}]}`,
		"malformed offer":  `{"TE":[{"TRID":612,"GEB":[{"GEID":2,"C2":"2500","BV":50}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeGlobalEffectTriggerSnapshot(json.RawMessage(payload), observedAt, nil); err == nil {
				t.Fatalf("malformed trigger snapshot accepted: %s", payload)
			}
		})
	}
	for name, payload := range map[string]string{
		"explicit empty TE": `{"TE":[]}`,
		"empty effects":     `{"TE":[{"TRID":610,"GE":[]}]}`,
		"empty offers":      `{"TE":[{"TRID":612,"GEB":[]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeGlobalEffectTriggerSnapshot(json.RawMessage(payload), observedAt, nil); err != nil {
				t.Fatalf("valid explicit empty trigger snapshot rejected: %v", err)
			}
		})
	}
	snapshot, err := decodeGlobalEffectTriggerSnapshot(json.RawMessage(`{"TE":[
		{"TRID":610,"GE":[[3,-1,-1],[2,1800,-1]]},
		{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":50}]}
	]}`), observedAt, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Effects) != 1 || snapshot.Effects[2].Strength != -1 || !snapshot.Effects[2].ActiveAt(observedAt) {
		t.Fatalf("elapsed/sentinel rows decoded incorrectly: %+v", snapshot.Effects)
	}
}

func TestGBDTriggerSnapshotAuthorityDistinguishesInvalidAbsentAndNoOffer(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	observedAt := time.Date(2026, time.September, 16, 17, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name              string
		payload           string
		wantBaseline      bool
		wantEffect2       bool
		wantOffer2        bool
		preservePriorData bool
	}{
		{name: "missing TEI", payload: `{"bie":{"GE":[]},"gcu":{"C2":10000}}`, preservePriorData: true},
		{name: "null TEI", payload: `{"tei":null,"bie":{"GE":[]},"gcu":{"C2":10000}}`, preservePriorData: true},
		{name: "malformed TE", payload: `{"tei":{"TE":null},"bie":{"GE":[]},"gcu":{"C2":10000}}`, preservePriorData: true},
		{name: "malformed GE", payload: `{"tei":{"TE":[{"TRID":610,"GE":null}]},"bie":{"GE":[]},"gcu":{"C2":10000}}`, preservePriorData: true},
		{name: "malformed GEB", payload: `{"tei":{"TE":[{"TRID":612,"GEB":{}}]},"bie":{"GE":[]},"gcu":{"C2":10000}}`, preservePriorData: true},
		{name: "explicit empty TE", payload: `{"tei":{"TE":[]},"bie":{"GE":[]},"gcu":{"C2":10000}}`, wantBaseline: true},
		{name: "effect 2 absent", payload: `{"tei":{"TE":[{"TRID":610,"GE":[[3,1800,5]]},{"TRID":612,"GEB":[{"GEID":3,"C2":500,"BV":5}]}]},"bie":{"GE":[]},"gcu":{"C2":10000}}`, wantBaseline: true},
		{name: "offer missing", payload: `{"tei":{"TE":[{"TRID":610,"GE":[[2,1800,10]]}]},"bie":{"GE":[]},"gcu":{"C2":10000}}`, wantBaseline: true, wantEffect2: true},
		{name: "available and offered", payload: `{"tei":{"TE":[{"TRID":610,"GE":[[2,1800,10]]},{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":50}]}]},"bie":{"GE":[]},"gcu":{"C2":10000}}`, wantBaseline: true, wantEffect2: true, wantOffer2: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Session.ConnectionGeneration = 11
			priorAt := observedAt.Add(-time.Hour)
			gameState.EventScores.Inventory.GlobalEffectsObservedAt = priorAt
			gameState.EventScores.Inventory.GlobalEffectReadObservedAt = priorAt
			gameState.EventScores.Inventory.GlobalEffectReadGeneration = 11
			gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = priorAt
			gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 11
			gameState.EventScores.Inventory.GlobalEffects[2] = State.GlobalEffectAvailability{GlobalEffectID: 2, Strength: 9, EndsAt: observedAt.Add(time.Hour)}
			gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = State.GlobalEffectBoosterOffer{GlobalEffectID: 2, RubyCost: 2500, BonusValue: 49}
			store := State.NewStore(gameState)
			registry := NewRegistry()
			if err := RegisterCoreReducers(registry); err != nil {
				t.Fatal(err)
			}
			pipeline := NewPipeline(store, staticGameDataProvider{store: gameData}, registry)
			code := 0
			if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
				Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt, Payload: json.RawMessage(test.payload),
			}); err != nil {
				t.Fatalf("GBD hydration failed: %v", err)
			}
			inventory := store.ReadOnlyView().EventScores.Inventory
			if test.wantBaseline != inventory.GlobalEffectBaselineObservedAt.Equal(observedAt) ||
				!inventory.GlobalEffectReadObservedAt.Equal(observedAt) {
				t.Fatalf("authority baseline=%s read=%s", inventory.GlobalEffectBaselineObservedAt, inventory.GlobalEffectReadObservedAt)
			}
			_, effect2 := inventory.GlobalEffects[2]
			_, offer2 := inventory.GlobalEffectBoosterOffers[2]
			if test.preservePriorData {
				if !effect2 || !offer2 || inventory.GlobalEffects[2].Strength != 9 || inventory.GlobalEffectBoosterOffers[2].BonusValue != 49 {
					t.Fatalf("invalid TEI replaced prior trigger data: %+v", inventory)
				}
			} else if effect2 != test.wantEffect2 || offer2 != test.wantOffer2 {
				t.Fatalf("effect2=%t offer2=%t inventory=%+v", effect2, offer2, inventory)
			}
		})
	}
}

func TestIncrementalTEIAndTEEUpdateOnlyNamedGlobalTrigger(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil || !registry.HasInbound("tei") || !registry.HasInbound("tee") {
		t.Fatalf("trigger reducers registered tei=%t tee=%t err=%v", registry.HasInbound("tei"), registry.HasInbound("tee"), err)
	}
	gameState := State.NewGameState()
	observedAt := time.Date(2026, time.September, 16, 17, 0, 0, 0, time.UTC)
	gameState.EventScores.Inventory.GlobalEffectsObservedAt = observedAt
	gameState.EventScores.Inventory.GlobalEffectReadObservedAt = observedAt
	gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = observedAt
	gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 4
	gameState.EventScores.Inventory.GlobalEffects[2] = State.GlobalEffectAvailability{GlobalEffectID: 2, Strength: 10, EndsAt: observedAt.Add(time.Hour)}
	gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = State.GlobalEffectBoosterOffer{GlobalEffectID: 2, RubyCost: 2500, BonusValue: 50}
	code := 0

	_, changed, err := reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
		Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"TE":[{"TRID":601,"X":1}]}`),
	}, &gameState, nil)
	if err != nil || changed || !gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt.Equal(observedAt) {
		t.Fatalf("unrelated TEI invalidated global baseline: changed=%t err=%v inventory=%+v", changed, err, gameState.EventScores.Inventory)
	}

	_, changed, err = reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
		Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(24 * time.Hour),
		Payload: json.RawMessage(`{"TE":[{"TRID":610,"GE":[[2,1800,12]],"SGE":[]}]}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.EventScores.Inventory.GlobalEffects[2].Strength != 12 ||
		gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2].BonusValue != 50 ||
		!gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt.IsZero() {
		t.Fatalf("incremental availability update=%+v changed=%t err=%v", gameState.EventScores.Inventory, changed, err)
	}

	baselineAt := observedAt.Add(24*time.Hour + time.Second)
	gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = baselineAt
	gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 4
	_, changed, err = reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
		Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: baselineAt,
		Payload: json.RawMessage(`{"TE":[{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":55}]}]}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.EventScores.Inventory.GlobalEffects[2].Strength != 12 ||
		gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2].BonusValue != 55 {
		t.Fatalf("incremental offer update=%+v changed=%t err=%v", gameState.EventScores.Inventory, changed, err)
	}

	gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = baselineAt
	gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 4
	_, changed, err = reduceGlobalEffectTriggerEnd(t.Context(), Protocol.Frame{
		Opcode: "tee", Direction: Protocol.DirectionInbound, ReceivedAt: baselineAt.Add(time.Second),
		Payload: json.RawMessage(`{"TRID":612}`),
	}, &gameState, nil)
	if err != nil || !changed || len(gameState.EventScores.Inventory.GlobalEffectBoosterOffers) != 0 ||
		gameState.EventScores.Inventory.GlobalEffects[2].Strength != 12 ||
		!gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt.IsZero() {
		t.Fatalf("TEE 612 removal=%+v changed=%t err=%v", gameState.EventScores.Inventory, changed, err)
	}

	_, changed, err = reduceGlobalEffectTriggerEnd(t.Context(), Protocol.Frame{
		Opcode: "tee", Direction: Protocol.DirectionInbound, ReceivedAt: baselineAt.Add(2 * time.Second),
		Payload: json.RawMessage(`{"TRID":601}`),
	}, &gameState, nil)
	if err != nil || changed || len(gameState.EventScores.Inventory.GlobalEffects) != 1 {
		t.Fatalf("unrelated TEE changed global state: changed=%t err=%v inventory=%+v", changed, err, gameState.EventScores.Inventory)
	}

	_, changed, err = reduceGlobalEffectTriggerEnd(t.Context(), Protocol.Frame{
		Opcode: "tee", Direction: Protocol.DirectionInbound, ReceivedAt: baselineAt.Add(3 * time.Second),
		Payload: json.RawMessage(`{"TRID":610}`),
	}, &gameState, nil)
	if err != nil || !changed || len(gameState.EventScores.Inventory.GlobalEffects) != 0 {
		t.Fatalf("TEE 610 removal=%+v changed=%t err=%v", gameState.EventScores.Inventory, changed, err)
	}
}

func TestMalformedStandaloneTEIInvalidatesPurchaseBaselineWithoutReplacingData(t *testing.T) {
	observedAt := time.Date(2026, time.September, 16, 17, 0, 0, 0, time.UTC)
	for name, payload := range map[string]string{
		"malformed root": `null`,
		"malformed GE":   `{"TE":[{"TRID":610,"GE":null}]}`,
		"malformed GEB":  `{"TE":[{"TRID":612,"GEB":[{"GEID":2,"C2":2500}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.EventScores.Inventory.GlobalEffectsObservedAt = observedAt
			gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt = observedAt
			gameState.EventScores.Inventory.GlobalEffectBaselineGeneration = 4
			gameState.EventScores.Inventory.GlobalEffects[2] = State.GlobalEffectAvailability{GlobalEffectID: 2, Strength: 10, EndsAt: observedAt.Add(time.Hour)}
			gameState.EventScores.Inventory.GlobalEffectBoosterOffers[2] = State.GlobalEffectBoosterOffer{GlobalEffectID: 2, RubyCost: 2500, BonusValue: 50}
			code := 0
			_, changed, err := reduceGlobalEffectTriggerSnapshot(t.Context(), Protocol.Frame{
				Opcode: "tei", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt.Add(time.Second), Payload: json.RawMessage(payload),
			}, &gameState, nil)
			if err != nil || !changed || !gameState.EventScores.Inventory.GlobalEffectBaselineObservedAt.IsZero() ||
				len(gameState.EventScores.Inventory.GlobalEffects) != 1 || len(gameState.EventScores.Inventory.GlobalEffectBoosterOffers) != 1 {
				t.Fatalf("malformed incremental TEI state=%+v changed=%t err=%v", gameState.EventScores.Inventory, changed, err)
			}
		})
	}
}

func TestGlobalEffectPurchaseEvidenceHandlesBothBIEAndAGBOrderings(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	for _, bieFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("bie-first-%t", bieFirst), func(t *testing.T) {
			observedAt := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
			endsAt := observedAt.Add(time.Hour)
			gameState := State.NewGameState()
			gameState.Session.ConnectionGeneration = 4
			gameState.Player.Resources[2] = 7500
			gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: observedAt.Add(time.Second), ConnectionGeneration: 4}
			gameState.EventScores.Inventory.GlobalEffectsObservedAt = observedAt
			gameState.EventScores.Inventory.GlobalEffects[2] = State.GlobalEffectAvailability{GlobalEffectID: 2, Strength: 10, EndsAt: endsAt}
			gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
				GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ExpiresAt: endsAt,
				QuotedRubyCost: 2500, RubyBefore: 10000, RubyBeforeObservedAt: observedAt,
				RequestOpcode: "agb", OperationID: "op-current", ResponseToken: "token-current",
				ConnectionGeneration: 4, DebitUnverified: true, Outcome: State.GlobalEffectPurchaseUnresolved,
			}
			code := 0
			bie := func(at time.Time, payload string) {
				t.Helper()
				_, _, err := reduceGlobalEffectBoosterInfo(t.Context(), Protocol.Frame{
					Opcode: "bie", Direction: Protocol.DirectionInbound, ResponseCode: &code,
					ReceivedAt: at, Payload: json.RawMessage(payload),
				}, &gameState, gameData)
				if err != nil {
					t.Fatal(err)
				}
			}
			ack := func() {
				t.Helper()
				_, _, err := reduceGlobalEffectPurchaseAcknowledgement(t.Context(), Protocol.Frame{
					Opcode: "agb", Direction: Protocol.DirectionInbound, ResponseCode: &code,
					ReceivedAt: observedAt.Add(2 * time.Second), ResponseToken: "token-current",
				}, &gameState, gameData)
				if err != nil {
					t.Fatal(err)
				}
			}
			if bieFirst {
				bie(observedAt.Add(time.Second), `{"GE":[2]}`)
				ack()
			} else {
				ack()
				if outcome := gameState.EventScores.Inventory.GlobalEffectPurchases[2].Outcome; outcome != State.GlobalEffectPurchaseAccepted {
					t.Fatalf("empty code-zero AGB was not accepted: %q", outcome)
				}
				bie(observedAt.Add(3*time.Second), `{"GE":[2]}`)
			}
			record := gameState.EventScores.Inventory.GlobalEffectPurchases[2]
			if record.Outcome != State.GlobalEffectPurchaseConfirmed || record.ResultCode == nil || *record.ResultCode != 0 ||
				!record.RubyAfterKnown || record.ObservedRubyChange != 2500 || !record.DebitUnverified {
				t.Fatalf("combined purchase evidence=%+v", record)
			}
			activationObservedAt := record.ActivationObservedAt
			bie(observedAt.Add(4*time.Second), `{"GE":[]}`)
			record = gameState.EventScores.Inventory.GlobalEffectPurchases[2]
			if !gameState.EventScores.Inventory.GlobalEffectBoosts[2].Boosted || record.Outcome != State.GlobalEffectPurchaseConfirmed ||
				!record.ActivationObservedAt.Equal(activationObservedAt) {
				t.Fatal("later empty BIE erased positive confirmation for the same occurrence")
			}
		})
	}
}

func TestGlobalEffectPurchaseAckMustMatchCurrentOperation(t *testing.T) {
	gameState := State.NewGameState()
	endsAt := time.Now().UTC().Add(time.Hour)
	gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
		GlobalEffectID: 2, OccurrenceEndsAt: endsAt, OperationID: "new-op", ResponseToken: "new-token",
		Outcome: State.GlobalEffectPurchaseUnresolved, DebitUnverified: true,
	}
	code := 0
	_, changed, err := reduceGlobalEffectPurchaseAcknowledgement(t.Context(), Protocol.Frame{
		Opcode: "agb", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(), ResponseToken: "old-token", CausationOperationID: "old-op",
	}, &gameState, scalableEventTestGameData(t))
	if err != nil || changed || gameState.EventScores.Inventory.GlobalEffectPurchases[2].Outcome != State.GlobalEffectPurchaseUnresolved {
		t.Fatalf("late prior AGB bound to current record: changed=%t err=%v record=%+v", changed, err, gameState.EventScores.Inventory.GlobalEffectPurchases[2])
	}
}

func TestGlobalEffectPurchaseExplicitRejectionIsDurableEvidence(t *testing.T) {
	gameState := State.NewGameState()
	endsAt := time.Now().UTC().Add(time.Hour)
	gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
		GlobalEffectID: 2, OccurrenceEndsAt: endsAt, OperationID: "rejected-op", ResponseToken: "rejected-token",
		Outcome: State.GlobalEffectPurchaseUnresolved, DebitUnverified: true,
	}
	code := 91
	domains, changed, err := reduceGlobalEffectPurchaseAcknowledgement(t.Context(), Protocol.Frame{
		Opcode: "agb", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(), ResponseToken: "rejected-token", CausationOperationID: "rejected-op",
	}, &gameState, scalableEventTestGameData(t))
	record := gameState.EventScores.Inventory.GlobalEffectPurchases[2]
	if err != nil || !changed || record.Outcome != State.GlobalEffectPurchaseRejected || record.ResultCode == nil || *record.ResultCode != 91 ||
		!slices.Contains(domains, globalEffectPurchaseDurabilityDomain) {
		t.Fatalf("explicit rejection evidence: changed=%t domains=%v err=%v record=%+v", changed, domains, err, record)
	}
}

func TestGlobalEffectPurchaseAcceptanceNeedsNewerBalanceEvidence(t *testing.T) {
	observedAt := time.Now().UTC().Truncate(time.Second)
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 3
	gameState.Player.Resources[2] = 10000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: observedAt, ConnectionGeneration: 3}
	gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
		GlobalEffectID: 2, OccurrenceEndsAt: observedAt.Add(time.Hour), OperationID: "accepted-op",
		ResponseToken: "accepted-token", RubyBefore: 10000, RubyBeforeObservedAt: observedAt,
		DispatchedAt: observedAt, ConnectionGeneration: 3, Outcome: State.GlobalEffectPurchaseUnresolved,
		DebitUnverified: true,
	}
	code := 0
	_, changed, err := reduceGlobalEffectPurchaseAcknowledgement(t.Context(), Protocol.Frame{
		Opcode: "agb", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: observedAt.Add(time.Second), ResponseToken: "accepted-token", CausationOperationID: "accepted-op",
	}, &gameState, scalableEventTestGameData(t))
	record := gameState.EventScores.Inventory.GlobalEffectPurchases[2]
	if err != nil || !changed || record.Outcome != State.GlobalEffectPurchaseAccepted || record.RubyAfterKnown || !record.DebitUnverified {
		t.Fatalf("pre-purchase balance was presented as after evidence: changed=%t err=%v record=%+v", changed, err, record)
	}
}

func TestGBDCapturedGlobalEffectBaselineAndMalformedBIEIsolation(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	capturedFixture, err := os.ReadFile("testdata/global_effect_gbd_sanitized.json")
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name         string
		bie          string
		wantBaseline bool
	}{
		{name: "captured empty GE is authoritative", bie: `{"GE":[]}`, wantBaseline: true},
		{name: "null BIE preserves other hydration without authority", bie: `null`, wantBaseline: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			observedAt := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
			rubyBalance := 10000
			gameState := State.NewGameState()
			gameState.Session.ConnectionGeneration = 9
			if test.wantBaseline {
				rubyBalance = 7500
				gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
					GlobalEffectID: 2, OccurrenceEndsAt: observedAt.Add(47990 * time.Second).Truncate(time.Minute),
					RubyBefore: 10000, RubyBeforeObservedAt: observedAt.Add(-time.Minute),
					ConnectionGeneration: 9, Outcome: State.GlobalEffectPurchaseAccepted,
					DebitUnverified: true,
				}
			}
			store := State.NewStore(gameState)
			pipeline := NewPipeline(store, staticGameDataProvider{store: gameData}, registry)
			code := 0
			payload := fmt.Sprintf(`{
				"gpi":{"UID":456,"PID":123,"PN":"Fixture Player"},
					"tei":{"TE":[
						{"TRID":610,"GE":[[2,1800,10]],"SGE":[]},
						{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":50}]}
				]},
				"bie":%s,
				"gcu":{"C2":%d}
			}`, test.bie, rubyBalance)
			if test.wantBaseline {
				payload = string(capturedFixture)
			}
			_, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
				Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt, Payload: json.RawMessage(payload),
			})
			if err != nil {
				t.Fatalf("GBD pipeline rejected fixture: %v", err)
			}
			snapshot := store.Snapshot()
			if snapshot.Player.ID != 123 || snapshot.Player.Resources[2] != float64(rubyBalance) ||
				!snapshot.Player.ResourceObservations[2].ObservedAt.Equal(observedAt) {
				t.Fatalf("valid GPI/GCU did not commit: player=%+v resources=%+v observations=%+v", snapshot.Player, snapshot.Player.Resources, snapshot.Player.ResourceObservations)
			}
			effect := snapshot.EventScores.Inventory.GlobalEffects[2]
			offer := snapshot.EventScores.Inventory.GlobalEffectBoosterOffers[2]
			if effect.Strength != 10 || offer.RubyCost != 2500 || offer.BonusValue != 50 ||
				!snapshot.EventScores.Inventory.GlobalEffectReadObservedAt.Equal(observedAt) {
				t.Fatalf("valid TEI/read did not commit: effect=%+v offer=%+v inventory=%+v", effect, offer, snapshot.EventScores.Inventory)
			}
			baseline := snapshot.EventScores.Inventory.GlobalEffectBaselineObservedAt
			if test.wantBaseline != baseline.Equal(observedAt) {
				t.Fatalf("baseline=%s want authoritative=%t", baseline, test.wantBaseline)
			}
			if test.wantBaseline {
				status, found := snapshot.EventScores.Inventory.GlobalEffectBoosts[2]
				if !found || status.Boosted || status.ConnectionGeneration != 9 {
					t.Fatalf("captured BIE status=%+v found=%t", status, found)
				}
				record := snapshot.EventScores.Inventory.GlobalEffectPurchases[2]
				if record.Outcome != State.GlobalEffectPurchaseAccepted || !record.RubyAfterKnown ||
					record.RubyAfter != 7500 || record.ObservedRubyChange != 2500 || !record.DebitUnverified {
					t.Fatalf("post-GBD accepted evidence=%+v", record)
				}
			}
		})
	}
}

func TestGBDNestedBIEConfirmationCrossesDurabilityFence(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)
	endsAt := observedAt.Add(30 * time.Minute)
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 6
	gameState.EventScores.Inventory.GlobalEffectPurchases[2] = State.GlobalEffectPurchaseRecord{
		GlobalEffectID: 2, OccurrenceEndsAt: endsAt, ExpiresAt: endsAt,
		OperationID: "missing-agb-ack", Outcome: State.GlobalEffectPurchaseUnresolved,
		DebitUnverified: true,
	}
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, staticGameDataProvider{store: gameData}, registry)
	fenceCalls := 0
	pipeline.SetDurabilityFence(func(_ context.Context, event State.Event) error {
		fenceCalls++
		if event.Patch == nil || store.ReadOnlyView().EventScores.Inventory.GlobalEffectPurchases[2].Outcome != State.GlobalEffectPurchaseConfirmed {
			t.Fatalf("durability fence ran before confirmed state: event=%+v", event)
		}
		return nil
	})
	code := 0
	_, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"tei":{"TE":[
				{"TRID":610,"GE":[[2,1800,10]],"SGE":[]},
				{"TRID":612,"GEB":[{"GEID":2,"C2":2500,"BV":50}]}
			]},
			"bie":{"GE":[2]},"gcu":{"C2":7500}
		}`),
	})
	if err != nil || fenceCalls != 1 {
		t.Fatalf("nested BIE durability fence calls=%d err=%v", fenceCalls, err)
	}
}

func cloneGlobalEffects(input map[int64]State.GlobalEffectAvailability) map[int64]State.GlobalEffectAvailability {
	result := make(map[int64]State.GlobalEffectAvailability, len(input))
	for id, value := range input {
		result[id] = value
	}
	return result
}

func cloneGlobalEffectBoosts(input map[int64]State.GlobalEffectBoostState) map[int64]State.GlobalEffectBoostState {
	result := make(map[int64]State.GlobalEffectBoostState, len(input))
	for id, value := range input {
		result[id] = value
	}
	return result
}

func TestScalableEventSnapshotCachesFirstCurrenciesForEachOccurrence(t *testing.T) {
	gameData := scalableEventTestGameData(t)
	gameState := State.NewGameState()
	startedAt := time.Date(2026, 7, 28, 14, 0, 0, 0, time.UTC)
	code := 0
	apply := func(observedAt time.Time, payload string) {
		t.Helper()
		_, changed, err := reduceScalableEventSnapshot(t.Context(), Protocol.Frame{
			Opcode: "sei", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
			Payload: json.RawMessage(payload),
		}, &gameState, gameData)
		if err != nil || !changed {
			t.Fatalf("event snapshot at %s: changed=%t err=%v", observedAt, changed, err)
		}
	}

	apply(startedAt, `{"E":[{"EID":71,"RS":7200,"EASE":1}]}`)
	if got := gameState.Invasion.FortifyCurrencies; len(got) != 0 {
		t.Fatalf("currencies before an authoritative list = %#v", got)
	}

	capturedAt := startedAt.Add(time.Minute)
	apply(capturedAt, `{"E":[{"EID":71,"RS":7140,"EASE":1,"RCKS":["GTO","ST"]}]}`)
	apply(startedAt.Add(2*time.Minute), `{"E":[{"EID":71,"RS":7080,"EASE":1,"RCKS":["GTO","KM"]}]}`)
	activity := gameState.EventScores.ActivityByEvent[71]
	if got := activity.FortifyCurrencies; len(got) != 2 || got[0] != "GTO" || got[1] != "ST" ||
		!activity.FortifyCurrenciesObservedAt.Equal(capturedAt) {
		t.Fatalf("cached first event currencies = %#v at %s", got, activity.FortifyCurrenciesObservedAt)
	}
	if got := gameState.Invasion.FortifyCurrencies; len(got) != 2 || got[0] != "GTO" || got[1] != "ST" {
		t.Fatalf("active event currencies changed during the occurrence = %#v", got)
	}

	nextOccurrenceAt := startedAt.Add(24 * time.Hour)
	apply(nextOccurrenceAt, `{"E":[{"EID":71,"RS":7200,"EASE":1,"RCKS":["GTO","KM"]}]}`)
	activity = gameState.EventScores.ActivityByEvent[71]
	if got := activity.FortifyCurrencies; len(got) != 2 || got[0] != "GTO" || got[1] != "KM" ||
		!activity.FortifyCurrenciesObservedAt.Equal(nextOccurrenceAt) ||
		!activity.ObservedFrom.Equal(nextOccurrenceAt) {
		t.Fatalf("new occurrence currencies = %#v, activity = %#v", got, activity)
	}
}

func scalableEventTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}],
		"events":[{"eventID":"72","comment1":"AllianceNomad Invasion","eventType":"AllianceNomadInvasion"}],
		"eventAutoScalingDifficulties":[{"difficultyID":"308","eventID":"72","difficultyTypeID":"8"}],
		"eventAutoScalingDifficultyTypes":[{"difficultyTypeID":"8","name":"expertPlus","sortOrder":"8"}],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":"1147","eventID":"72","difficultyID":"308",
			"areaType":"35","camplevel":"107","playerRageCap":"1740","rageNeededForLevelUp":"34440"
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}
