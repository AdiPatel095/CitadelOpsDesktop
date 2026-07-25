package Reports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

func TestCloudOutboxUploadsOnlyPvPAndPurgesAfterConfirmation(t *testing.T) {
	ctx := t.Context()
	dataDir := t.TempDir()
	history, err := History.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenSQLiteStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	battleTime := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	pvp := BattleReport{
		ID: "10-20", ReportID: "10-20", AccountUID: 44, PlayerID: 1,
		MID: 10, LID: 20, OccurredAt: battleTime.Format(time.RFC3339), DateMs: battleTime.UnixMilli(),
		Result: "Victory", Role: "attacker",
		Attacker: &BattleCombatant{PlayerID: 1, Name: "Player", Alliance: "Alliance"},
		Defender: &BattleCombatant{PlayerID: 2, Name: "Opponent", Alliance: "Other"},
	}
	nonPvP := BattleReport{
		ID: "11-21", ReportID: "11-21", AccountUID: 44, PlayerID: 1,
		MID: 11, LID: 21, OccurredAt: battleTime.Add(time.Minute).Format(time.RFC3339),
		DateMs: battleTime.Add(time.Minute).UnixMilli(), Result: "Victory", Role: "attacker",
		Attacker: &BattleCombatant{PlayerID: 1, Name: "Player", Alliance: "Alliance"},
		Defender: &BattleCombatant{Dummy: true, Name: "NPC"},
	}
	if err := history.Append(History.CollectionBattleReports, pvp); err != nil {
		t.Fatal(err)
	}
	if err := history.Append(History.CollectionBattleReports, nonPvP); err != nil {
		t.Fatal(err)
	}

	snapshot := State.NewGameState()
	snapshot.Player.ID = 1
	snapshot.Player.AllianceID = 90
	snapshot.Alliance = State.AllianceState{
		ID: 90, Name: "Alliance",
		Members: []State.AllianceMember{{PlayerID: 1, Name: "Player"}},
	}
	if err := BackfillBattleHistory(ctx, history, store, snapshot); err != nil {
		t.Fatal(err)
	}
	compaction, err := CompactBattleHistory(history)
	if err != nil {
		t.Fatal(err)
	}
	if compaction.Kept != 1 || compaction.Discarded != 1 {
		t.Fatalf("compaction = %#v", compaction)
	}
	backfill, err := BackfillCloudOutbox(ctx, history, store, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if backfill.Queued != 1 {
		t.Fatalf("cloud outbox backfill = %#v", backfill)
	}

	var mu sync.Mutex
	remote := make([]string, 0, 1)
	uploaded := make([]cloudBattleReportEnvelope, 0, 1)
	cloud := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch request.Method {
		case http.MethodGet:
			_ = json.NewEncoder(writer).Encode(map[string]any{"reports": remote})
		case http.MethodPost:
			var batch []cloudBattleReportEnvelope
			if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			uploaded = append(uploaded, batch...)
			for _, report := range batch {
				remote = append(remote, report.Payload)
			}
			writer.WriteHeader(http.StatusCreated)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(cloud.Close)

	state := State.NewStore(snapshot)
	client := NewCloudClient(CloudConfig{UploadURL: cloud.URL, FetchURL: cloud.URL})
	uploader := NewCloudUploader(state, history, store, client)
	processed, err := uploader.processNext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !processed {
		t.Fatal("cloud outbox did not process a report")
	}

	mu.Lock()
	if len(uploaded) != 1 || uploaded[0].LID != 20 {
		t.Fatalf("uploaded reports = %#v", uploaded)
	}
	if uploaded[0].AttackerAllianceID != 90 {
		t.Fatalf("attacker alliance ID = %d", uploaded[0].AttackerAllianceID)
	}
	mu.Unlock()
	pending, err := store.PendingCloudReports(ctx, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("confirmed outbox reports remain = %#v", pending)
	}
	localReports, err := history.Read(History.CollectionBattleReports, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(localReports) != 0 {
		t.Fatalf("cloud-confirmed local reports remain = %d", len(localReports))
	}
	analytics, err := store.Recent(ctx, BattleReportQuery{AccountUID: 44, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(analytics) != 1 || analytics[0].LID != 21 {
		t.Fatalf("SQLite PvE analytics reports = %#v", analytics)
	}
}

func TestNewPvPCloudPayloadUsesLegacyCaptureShape(t *testing.T) {
	battleTime := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	report := BattleReport{
		ID: "10-20", MID: 10, LID: 20, DateMs: battleTime.UnixMilli(), Result: "Victory",
		Attacker: &BattleCombatant{PlayerID: 1},
		Defender: &BattleCombatant{PlayerID: 2},
	}
	capture := State.BattleReportCapture{
		MessageID: 10, ReportID: 20, BattleKey: "battle#key", CapturedAt: battleTime.Add(time.Minute),
		Summary: json.RawMessage(`{"MID":10,"LID":20}`),
		Waves:   json.RawMessage(`{"LID":20,"W":[]}`),
		Details: json.RawMessage(`{"LID":20,"Y":[]}`),
	}
	envelope, eligible, err := buildCloudEnvelopeFromCapture(report, capture)
	if err != nil {
		t.Fatal(err)
	}
	if !eligible {
		t.Fatal("PvP report was not eligible for cloud upload")
	}
	var payload legacyCloudBattleCapture
	if err := json.Unmarshal([]byte(envelope.Payload), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || payload.Source != "local" || payload.MID != 10 || payload.LID != 20 {
		t.Fatalf("legacy payload identity = %#v", payload)
	}
	if len(payload.BLS) == 0 || len(payload.BLM) == 0 || len(payload.BLD) == 0 {
		t.Fatalf("legacy payload omitted report frames = %#v", payload)
	}
}
