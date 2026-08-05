package Reports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type battleResearchIntentRecorder struct {
	requests chan Intent.Request
}

func (recorder *battleResearchIntentRecorder) Submit(_ context.Context, request Intent.Request) Intent.Receipt {
	recorder.requests <- request
	return Intent.Receipt{Intent: request.Name, Actor: request.Actor, Status: Intent.StatusSucceeded}
}

func TestBattleResearchRequiresExplicitCurrentConsent(t *testing.T) {
	for name, configuration := range map[string]BattleResearchConfiguration{
		"disabled":         {Enabled: false, ConsentVersion: BattleResearchConsentVersion},
		"outdated consent": {Enabled: true, ConsentVersion: BattleResearchConsentVersion - 1},
		"current opt in":   {Enabled: true, ConsentVersion: BattleResearchConsentVersion},
	} {
		want := name == "current opt in"
		if got := configuration.Active(); got != want {
			t.Fatalf("%s Active() = %t, want %t", name, got, want)
		}
	}
	if !Outbound.YieldsToAutomationLock(battleResearchActor) {
		t.Fatal("battle research actor bypasses Bot Lock")
	}
}

func TestBattleResearchPollsMovementsOnlyForUnmatchedTrials(t *testing.T) {
	now := time.Now().UTC()
	manager := &BattleResearchManager{trials: map[string]BattleResearchTrial{}}
	if manager.needsMovementPoll(now) {
		t.Fatal("idle battle research requested movement polling")
	}
	manager.trials["waiting"] = BattleResearchTrial{
		ID: "waiting", Phase: "formation_captured",
		Formation: BattleResearchFormation{CapturedAt: now.Add(-time.Minute)},
	}
	if !manager.needsMovementPoll(now) {
		t.Fatal("unmatched current trial did not request movement polling")
	}
	trial := manager.trials["waiting"]
	trial.Movement = &State.MovementState{ID: 1}
	manager.trials["waiting"] = trial
	if manager.needsMovementPoll(now) {
		t.Fatal("matched trial continued movement polling")
	}
}

func TestParseBattleResearchFormationPreservesExactLanesAndSupport(t *testing.T) {
	capturedAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	formation, err := parseBattleResearchFormation(json.RawMessage(`{
		"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"LID":7,
		"A":[{"L":{"U":[[216,10]],"T":[[500,2]]},"M":{"U":[[217,20]]},"R":{"U":[[218,30]]}}],
		"RW":[[227,5]],"AST":[900,-1]
	}`), capturedAt)
	if err != nil {
		t.Fatal(err)
	}
	if formation.CommanderID != 7 || formation.TargetX != 10 || len(formation.Waves) != 1 ||
		formation.Waves[0].Left.Units[0] != (ResearchPair{WodID: 216, Amount: 10}) ||
		formation.Waves[0].Left.Tools[0] != (ResearchPair{WodID: 500, Amount: 2}) ||
		formation.SupportTroops[0].Amount != 5 || len(formation.SupportToolIDs) != 1 ||
		battleResearchFormationTroops(formation) != 65 {
		t.Fatalf("unexpected formation: %#v", formation)
	}
}

func TestBuildBattleResearchPredictionProducesVersionedResolutionShape(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[
			{"wodID":1,"meleeAttack":"100","meleeDefence":"20","rangeDefence":"20"},
			{"wodID":2,"meleeAttack":"10","meleeDefence":"50","rangeDefence":"50"}
		],
		"effects":[{"effectID":61,"effectTypeID":23,"capID":23,"name":"offensiveMeleeBonus"}],
		"effectCaps":[{"capID":23,"maxTotalBonus":"90"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	formation, err := parseBattleResearchFormation(json.RawMessage(`{
		"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"LID":7,
		"A":[{"L":{"U":[[1,10]]},"M":{},"R":{}}],"RW":[],"AST":[]
	}`), time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	contextSnapshot := BattleResearchAttackerContext{
		Equipment: []State.EquipmentInstance{{
			ID: 10, Effects: State.EquipmentEffects{{DefinitionID: 61, Values: []float64{20}}},
		}},
	}
	spy := SpyReport{
		Status: "success", TotalTroops: 5,
		Setup:     []SpySection{{Index: 0, Name: "Left flank", Total: 5, Units: []UnitCount{{WodID: 2, Amount: 5}}}},
		Castellan: &SpyCastellan{},
	}
	prediction, err := BuildBattleResearchPrediction(
		formation, contextSnapshot, State.MovementState{TargetTypeID: 4}, spy, gameData,
		time.Date(2026, 8, 5, 12, 1, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if prediction.ModelVersion != BattleResearchModelVersion || prediction.PredictedResult != "Victory" ||
		prediction.AttackerMeleeBonus != 20 || prediction.AttackerSent != 10 || prediction.DefenderObserved != 5 ||
		len(prediction.Waves) != 1 || prediction.Waves[0].Left.Winner != "attacker" ||
		prediction.Courtyard.Winner != "attacker" || prediction.UnitStatCoverage != 1 {
		t.Fatalf("unexpected prediction: %#v", prediction)
	}
}

func TestBattleResearchStoreRoundTripsTrial(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	trial := BattleResearchTrial{
		Version: 1, ID: "trial", Phase: "prediction_saved", PlayerID: 7,
		CreatedAt: now, UpdatedAt: now, UploadState: "not-ready",
		Formation:  BattleResearchFormation{TargetX: 10, TargetY: 20},
		Prediction: &BattlePrediction{ModelVersion: BattleResearchModelVersion, GeneratedAt: now},
	}
	if err := store.SaveBattleResearchTrial(context.Background(), trial); err != nil {
		t.Fatal(err)
	}
	trials, err := store.ListBattleResearchTrials(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(trials) != 1 || trials[0].ID != trial.ID || trials[0].Prediction == nil ||
		trials[0].Prediction.ModelVersion != BattleResearchModelVersion {
		t.Fatalf("round trip = %#v", trials)
	}
}

func TestCloudClientUploadsCompleteBattleResearchBundle(t *testing.T) {
	var received struct {
		TrialID        string          `json:"trialID"`
		BattleReportID string          `json:"battleReportID"`
		MovementID     int64           `json:"movementID"`
		Payload        json.RawMessage `json:"payload"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Citadel-Report-Key") != "key" {
			t.Errorf("upload key = %q", request.Header.Get("X-Citadel-Report-Key"))
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(30 * time.Second)
	movement := State.MovementState{ID: 55, ArrivesAt: &arrivesAt}
	trial := BattleResearchTrial{
		Version: 1, ID: "trial", Phase: "complete", PlayerID: 7, Movement: &movement,
		PreSpy:     &BattleResearchSpySnapshot{CapturedAt: now.Add(-time.Minute)},
		Prediction: &BattlePrediction{ModelVersion: BattleResearchModelVersion, GeneratedAt: now},
		Battle:     &BattleResearchOutcome{CapturedAt: now.Add(30 * time.Second), Report: BattleReport{ID: "10-20", DateMs: arrivesAt.UnixMilli()}},
		PostSpy:    &BattleResearchSpySnapshot{CapturedAt: now.Add(time.Minute)}, CompletedAt: now.Add(time.Minute),
	}
	client := NewCloudClient(CloudConfig{Client: server.Client(), TrainingURL: server.URL, UploadKey: "key"})
	if err := client.UploadBattleResearch(context.Background(), trial); err != nil {
		t.Fatal(err)
	}
	if received.TrialID != "trial" || received.BattleReportID != "10-20" || received.MovementID != 55 || !json.Valid(received.Payload) {
		t.Fatalf("received = %#v", received)
	}
}

func TestBattleResearchManagerCompletesAndUploadsForwardTrial(t *testing.T) {
	store, err := OpenSQLiteStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	uploads := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/reports/battle-training" {
			t.Errorf("upload path = %q", request.URL.Path)
		}
		uploads <- struct{}{}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	recorder := &battleResearchIntentRecorder{requests: make(chan Intent.Request, 4)}
	manager := &BattleResearchManager{
		intents: recorder, store: store,
		cloud:  NewCloudClient(CloudConfig{Client: server.Client(), TrainingURL: server.URL + "/reports/battle-training"}),
		trials: map[string]BattleResearchTrial{},
	}
	launchedAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	arrivesAt := launchedAt.Add(10 * time.Minute)
	commanderID := State.CommanderID(77)
	movement := State.MovementState{
		ID: 55, TypeID: 0, Direction: 0, OwnerPlayerID: 7, TargetPlayerID: 9,
		SourceTypeID: 4, SourceCastleID: 100, TargetTypeID: 4, TargetCastleID: 200,
		CommanderID: &commanderID, KingdomID: 0, SourceX: 1, SourceY: 2,
		TargetX: 10, TargetY: 20, StartedAt: launchedAt, ObservedAt: launchedAt.Add(5 * time.Second),
		ArrivesAt: &arrivesAt,
	}
	trial := BattleResearchTrial{
		Version: 1, ID: "trial-lifecycle", Phase: "formation_captured", AccountUID: 6,
		WorldID: "US1", PlayerID: 7, CreatedAt: launchedAt, UpdatedAt: launchedAt,
		Formation: BattleResearchFormation{
			CapturedAt: launchedAt, SourceX: 1, SourceY: 2, TargetX: 10, TargetY: 20,
			KingdomID: 0, CommanderID: commanderID,
		},
		UploadState: "not-ready",
	}
	manager.trials[trial.ID] = trial
	snapshot := State.NewGameState()
	snapshot.Player.ID = 7
	snapshot.Castles[100] = State.CastleState{ID: 100, SlotType: 4, X: 1, Y: 2}
	snapshot.Movements[movement.ID] = movement
	manager.reconcileTrials(
		context.Background(), snapshot,
		BattleResearchConfiguration{Enabled: true, ConsentVersion: BattleResearchConsentVersion, SpyCount: 1},
		launchedAt.Add(5*time.Second),
	)
	preRequest := waitForBattleResearchIntent(t, recorder.requests)
	if preRequest.Name != "spy.launch" || preRequest.Actor != battleResearchActor {
		t.Fatalf("pre-spy request = %+v", preRequest)
	}
	var spyArguments struct {
		SourceCastleID int64 `json:"sourceCastleId"`
		TargetX        int   `json:"targetX"`
		TargetY        int   `json:"targetY"`
		KingdomID      int64 `json:"kingdomId"`
		SpyCount       int   `json:"spyCount"`
	}
	if err := json.Unmarshal(preRequest.Arguments, &spyArguments); err != nil ||
		spyArguments.SourceCastleID != 100 || spyArguments.TargetX != 10 || spyArguments.TargetY != 20 ||
		spyArguments.KingdomID != 0 || spyArguments.SpyCount != 1 {
		t.Fatalf("pre-spy arguments = %+v, error = %v", spyArguments, err)
	}
	preCapturedAt := launchedAt.Add(20 * time.Second)
	manager.handleSpyReport(context.Background(), SpyReport{
		ID: "pre", MID: 1, CapturedAtUnixMillis: preCapturedAt.UnixMilli(), Status: "success",
		Target: Player{ID: 9}, Castle: Castle{ID: 200, KingdomID: 0, X: 10, Y: 20},
		Setup: []SpySection{{Index: 0, Units: []UnitCount{{WodID: 2, Amount: 5}}}},
	}, State.SpyReportCapture{MessageID: 1, CapturedAt: preCapturedAt})
	manager.mu.Lock()
	updated := manager.trials[trial.ID]
	updated.Prediction = &BattlePrediction{
		ModelVersion: BattleResearchModelVersion, GeneratedAt: preCapturedAt.Add(time.Second),
		PredictedResult: "Victory",
	}
	updated.Phase = "prediction_saved"
	manager.trials[trial.ID] = updated
	manager.mu.Unlock()
	battleCapturedAt := arrivesAt.Add(2 * time.Second)
	manager.handleBattleReport(context.Background(), BattleReport{
		ID: "10-20", MovementID: int64(movement.ID), DateMs: arrivesAt.UnixMilli(),
		Role: "attacker", Result: "Victory", Attacker: &BattleCombatant{PlayerID: 7},
		Metrics: BattleMetrics{AttackerLost: 3, DefenderLost: 5},
	}, State.BattleReportCapture{MessageID: 10, ReportID: 20, MovementID: movement.ID, CapturedAt: battleCapturedAt})
	manager.reconcileTrials(
		context.Background(), snapshot,
		BattleResearchConfiguration{Enabled: true, ConsentVersion: BattleResearchConsentVersion, SpyCount: 1},
		battleCapturedAt.Add(time.Second),
	)
	postRequest := waitForBattleResearchIntent(t, recorder.requests)
	if postRequest.Name != "spy.launch" || postRequest.Actor != battleResearchActor {
		t.Fatalf("post-spy request = %+v", postRequest)
	}
	postCapturedAt := battleCapturedAt.Add(5 * time.Second)
	manager.handleSpyReport(context.Background(), SpyReport{
		ID: "post", MID: 2, CapturedAtUnixMillis: postCapturedAt.UnixMilli(), Status: "success",
		Target: Player{ID: 9}, Castle: Castle{ID: 200, KingdomID: 0, X: 10, Y: 20},
		Setup: []SpySection{{Index: 0}},
	}, State.SpyReportCapture{MessageID: 2, CapturedAt: postCapturedAt})
	manager.uploadNext(context.Background(), postCapturedAt.Add(time.Second))
	select {
	case <-uploads:
	case <-time.After(time.Second):
		t.Fatal("completed battle research trial was not uploaded")
	}
	manager.mu.RLock()
	completed := manager.trials[trial.ID]
	manager.mu.RUnlock()
	if completed.Phase != "complete" || completed.PreSpy == nil || completed.Battle == nil || completed.PostSpy == nil ||
		completed.UploadState != "uploaded" || completed.Battle.Report.Metrics.AttackerLost != 3 {
		t.Fatalf("completed trial = %#v", completed)
	}
}

func waitForBattleResearchIntent(t *testing.T, requests <-chan Intent.Request) Intent.Request {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for battle research intent")
		return Intent.Request{}
	}
}
