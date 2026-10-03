package Reports

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

// fakeCloud is a backend that stores reports by LID and records what the client sends.
type fakeCloud struct {
	mu            sync.Mutex
	stored        map[int64]string
	preexisting   int
	hasLIDRoute   bool
	failUploads   int
	failLIDStatus int
	listCalls     int
	lidCalls      [][]int64
	uploadCalls   int
	bytesOut      int
	bytesIn       int
	keys          []string
}

func newFakeCloud(preexisting int, hasLIDRoute bool) *fakeCloud {
	return &fakeCloud{stored: map[int64]string{}, preexisting: preexisting, hasLIDRoute: hasLIDRoute}
}

func (cloud *fakeCloud) handler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cloud.mu.Lock()
		defer cloud.mu.Unlock()
		cloud.keys = append(cloud.keys, request.Header.Get("X-Citadel-Report-Key"))
		switch {
		case request.URL.Path == "/reports/battle/lids" && request.Method == http.MethodPost:
			if !cloud.hasLIDRoute {
				http.NotFound(writer, request)
				return
			}
			if cloud.failLIDStatus != 0 {
				http.Error(writer, "lookup failed", cloud.failLIDStatus)
				return
			}
			raw, _ := io.ReadAll(request.Body)
			cloud.bytesOut += len(raw)
			var body struct {
				LIDs []int64 `json:"lids"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			cloud.lidCalls = append(cloud.lidCalls, append([]int64(nil), body.LIDs...))
			present := []int64{}
			for _, lid := range body.LIDs {
				if _, found := cloud.stored[lid]; found {
					present = append(present, lid)
				}
			}
			sort.Slice(present, func(i, j int) bool { return present[i] < present[j] })
			response, _ := json.Marshal(map[string]any{"count": len(present), "lids": present})
			cloud.bytesIn += len(response)
			_, _ = writer.Write(response)
		case request.URL.Path == "/reports/battle" && request.Method == http.MethodGet:
			cloud.listCalls++
			reports := make([]string, 0, len(cloud.stored)+cloud.preexisting)
			for _, payload := range cloud.stored {
				reports = append(reports, payload)
			}
			for index := 0; index < cloud.preexisting; index++ {
				reports = append(reports, `{"lid":`+strconv.Itoa(1_000_000+index)+`}`)
			}
			response, _ := json.Marshal(map[string]any{"reports": reports})
			cloud.bytesIn += len(response)
			_, _ = writer.Write(response)
		case request.URL.Path == "/reports/battle" && request.Method == http.MethodPost:
			cloud.uploadCalls++
			if cloud.failUploads > 0 {
				cloud.failUploads--
				http.Error(writer, "unavailable", http.StatusServiceUnavailable)
				return
			}
			var batch []cloudBattleReportEnvelope
			if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			for _, report := range batch {
				cloud.stored[report.LID] = report.Payload
			}
			writer.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(writer, request)
		}
	})
}

type cloudFixture struct {
	uploader *CloudUploader
	client   *CloudClient
	store    *SQLiteStore
	history  *History.Store
	cloud    *fakeCloud
	lids     []int64
}

func newCloudFixture(t *testing.T, reports int, cloud *fakeCloud, key string) *cloudFixture {
	t.Helper()
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
	server := httptest.NewServer(cloud.handler())
	t.Cleanup(server.Close)
	battleTime := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	fixture := &cloudFixture{store: store, history: history, cloud: cloud}
	for index := 0; index < reports; index++ {
		lid := int64(20 + index)
		report := BattleReport{
			ID: strconv.FormatInt(10+int64(index), 10) + "-" + strconv.FormatInt(lid, 10), AccountUID: 44, PlayerID: 1,
			MID: 10 + int64(index), LID: lid, OccurredAt: battleTime.Format(time.RFC3339), DateMs: battleTime.UnixMilli() + int64(index),
			Result: "Victory", Role: "attacker",
			Attacker: &BattleCombatant{PlayerID: 1, Name: "Player", Alliance: "Alliance"},
			Defender: &BattleCombatant{PlayerID: 2, Name: "Opponent", Alliance: "Other"},
		}
		report.ReportID = report.ID
		envelope, eligible, err := buildCloudEnvelopeFromReport(report)
		if err != nil || !eligible {
			t.Fatalf("envelope for LID %d: eligible %t, %v", lid, eligible, err)
		}
		if err := store.QueueCloudReport(ctx, envelope); err != nil {
			t.Fatal(err)
		}
		fixture.lids = append(fixture.lids, lid)
	}
	snapshot := State.NewGameState()
	snapshot.Player.ID = 1
	fixture.client = NewCloudClient(CloudConfig{
		UploadURL: server.URL + "/reports/battle", FetchURL: server.URL + "/reports/battle", UploadKey: key,
		Client: &http.Client{Timeout: 5 * time.Second},
	})
	fixture.uploader = NewCloudUploader(State.NewStore(&snapshot), history, store, fixture.client)
	return fixture
}

func (fixture *cloudFixture) pending(t *testing.T) int {
	t.Helper()
	pending, err := fixture.store.PendingCloudReports(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return len(pending)
}

func TestUploaderChecksOnlyPendingLIDsBeforeAndAfterUpload(t *testing.T) {
	cloud := newFakeCloud(50_000, true) // a large history the uploader must not transfer
	fixture := newCloudFixture(t, 25, cloud, "report-key")

	processed, err := fixture.uploader.processNext(t.Context())
	if err != nil || !processed {
		t.Fatalf("processNext = %t, %v", processed, err)
	}
	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	if cloud.listCalls != 0 {
		t.Fatalf("the uploader downloaded the report list %d times", cloud.listCalls)
	}
	if len(cloud.lidCalls) != 2 || cloud.uploadCalls != 1 {
		t.Fatalf("LID checks = %d, uploads = %d; want a check before and after one upload", len(cloud.lidCalls), cloud.uploadCalls)
	}
	for index, asked := range cloud.lidCalls {
		sort.Slice(asked, func(i, j int) bool { return asked[i] < asked[j] })
		if len(asked) != 25 || asked[0] != fixture.lids[0] || asked[24] != fixture.lids[24] {
			t.Fatalf("check %d asked about %v, want exactly the 25 pending LIDs", index, asked)
		}
	}
	if cloud.bytesOut+cloud.bytesIn > 2048 {
		t.Fatalf("confirming a 25-report batch moved %d bytes (out %d, in %d); it must not scale with history", cloud.bytesOut+cloud.bytesIn, cloud.bytesOut, cloud.bytesIn)
	}
	for _, key := range cloud.keys {
		if key != "report-key" {
			t.Fatalf("request without the upload key: %v", cloud.keys)
		}
	}
	if fixture.pending(t) != 0 {
		t.Fatal("confirmed reports remain in the outbox")
	}
}

func TestUploaderSkipsUploadWhenTheCloudAlreadyHasTheBatch(t *testing.T) {
	cloud := newFakeCloud(0, true)
	fixture := newCloudFixture(t, 3, cloud, "")
	for _, lid := range fixture.lids {
		cloud.stored[lid] = `{"lid":` + strconv.FormatInt(lid, 10) + `}`
	}
	processed, err := fixture.uploader.processNext(t.Context())
	if err != nil || !processed {
		t.Fatalf("processNext = %t, %v", processed, err)
	}
	if cloud.uploadCalls != 0 || len(cloud.lidCalls) != 1 || fixture.pending(t) != 0 {
		t.Fatalf("uploads %d, checks %d, pending %d; want one check, no upload, empty outbox", cloud.uploadCalls, len(cloud.lidCalls), fixture.pending(t))
	}
}

func TestUploaderRetryAfterPartialFailureConverges(t *testing.T) {
	cloud := newFakeCloud(0, true)
	cloud.failUploads = 1
	fixture := newCloudFixture(t, 4, cloud, "")
	if processed, err := fixture.uploader.processNext(t.Context()); err == nil || processed {
		t.Fatalf("first attempt = %t, %v; want a failed upload", processed, err)
	}
	if fixture.pending(t) != 4 {
		t.Fatalf("a failed upload dropped outbox entries: %d pending", fixture.pending(t))
	}
	if processed, err := fixture.uploader.processNext(t.Context()); err != nil || !processed {
		t.Fatalf("retry = %t, %v", processed, err)
	}
	if fixture.pending(t) != 0 || len(cloud.stored) != 4 {
		t.Fatalf("after retry: %d pending, %d stored", fixture.pending(t), len(cloud.stored))
	}
}

func TestUploaderFallsBackToTheListOnAnOlderBackendAndReprobesLater(t *testing.T) {
	cloud := newFakeCloud(0, false)
	fixture := newCloudFixture(t, 2, cloud, "")
	clock := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	fixture.client.now = func() time.Time { return clock }

	if processed, err := fixture.uploader.processNext(t.Context()); err != nil || !processed {
		t.Fatalf("older backend: processNext = %t, %v", processed, err)
	}
	cloud.mu.Lock()
	listAfterFirst := cloud.listCalls
	cloud.mu.Unlock()
	if listAfterFirst < 1 {
		t.Fatal("the older backend was never asked for the list")
	}

	// Within the recheck window the route is not probed again.
	cloud.mu.Lock()
	cloud.hasLIDRoute = true
	cloud.mu.Unlock()
	clock = clock.Add(cloudLIDRouteRecheck - time.Minute)
	if _, err := fixture.client.RemoteLIDsOf(t.Context(), []int64{20}); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	if len(cloud.lidCalls) != 0 {
		t.Fatal("the presence route was probed inside the recheck window")
	}
	cloud.mu.Unlock()

	// After the window the rolled-out route is used.
	clock = clock.Add(2 * time.Minute)
	if _, err := fixture.client.RemoteLIDsOf(t.Context(), []int64{20}); err != nil {
		t.Fatal(err)
	}
	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	if len(cloud.lidCalls) != 1 {
		t.Fatalf("presence route calls after the window = %d, want 1", len(cloud.lidCalls))
	}
}

func TestLIDCheckErrorsDoNotFallBackToTheList(t *testing.T) {
	cloud := newFakeCloud(0, true)
	cloud.failLIDStatus = http.StatusInternalServerError
	fixture := newCloudFixture(t, 2, cloud, "")
	if processed, err := fixture.uploader.processNext(t.Context()); err == nil || processed {
		t.Fatalf("processNext = %t, %v; want the lookup error", processed, err)
	}
	if cloud.listCalls != 0 || cloud.uploadCalls != 0 || fixture.pending(t) != 2 {
		t.Fatalf("list %d, uploads %d, pending %d; a failed check must change nothing", cloud.listCalls, cloud.uploadCalls, fixture.pending(t))
	}
}

func TestCloudReportLIDsURLIsDerivedFromTheListURL(t *testing.T) {
	for input, want := range map[string]string{
		"https://citadelops.app/api/reports/battle":         "https://citadelops.app/api/reports/battle/lids",
		"https://citadelops.app/api/reports/battle/":        "https://citadelops.app/api/reports/battle/lids",
		"https://citadelops.app/api/reports/battle?limit=5": "https://citadelops.app/api/reports/battle/lids",
		"http://127.0.0.1:9000":                             "http://127.0.0.1:9000/lids",
	} {
		if got := cloudReportLIDsURL(input); got != want {
			t.Errorf("cloudReportLIDsURL(%q) = %q, want %q", input, got, want)
		}
	}
	client := NewCloudClient(CloudConfig{FetchURL: "https://example.test/api/reports/battle", UploadURL: "u", TrainingURL: "t"})
	if client.lidsURL != "https://example.test/api/reports/battle/lids" {
		t.Fatalf("derived client URL = %q", client.lidsURL)
	}
	custom := NewCloudClient(CloudConfig{FetchURL: "https://example.test/x", LIDsURL: "https://example.test/lids-route", UploadURL: "u", TrainingURL: "t"})
	if custom.lidsURL != "https://example.test/lids-route" {
		t.Fatalf("configured URL = %q", custom.lidsURL)
	}
}

func TestPendingCloudLIDsDeduplicatesAndSkipsInvalid(t *testing.T) {
	got := pendingCloudLIDs([]cloudBattleReportEnvelope{{LID: 5}, {LID: 5}, {LID: 0}, {LID: -1}, {LID: 9}})
	if len(got) != 2 || got[0] != 5 || got[1] != 9 {
		t.Fatalf("pendingCloudLIDs = %v", got)
	}
}
