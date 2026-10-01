package Reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const planFailureCode = -2

type reportResponseSender struct {
	pipeline *Ingest.Pipeline
	code     int
	sends    int
}

func (*reportResponseSender) Ready() bool                  { return true }
func (*reportResponseSender) Namespace() string            { return "EmpireEx_21" }
func (*reportResponseSender) CorrelatesResponses() bool    { return true }
func (*reportResponseSender) ConnectionGeneration() uint64 { return 1 }

func (sender *reportResponseSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.sends++
	if sender.code < 0 {
		return errors.New("game websocket became unavailable before the report was requested")
	}
	code := sender.code
	_, err = sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: command.Opcode, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(), ResponseToken: Outbound.MetadataFromContext(ctx).ResponseToken,
	})
	return err
}

// The engine and receipt shape are the real ones; the planner mirrors
// App.planSpyReportFetch, whose summary contains the message ID.
func newReportFetchEngine(t *testing.T, store *State.Store, code int) (*Intent.Engine, *reportResponseSender) {
	t.Helper()
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	sender := &reportResponseSender{pipeline: pipeline, code: code}
	intents := Intent.NewRegistry()
	if err := intents.Register(Intent.Definition{Name: "report.spy.fetch", Effect: Intent.EffectRead,
		Planner: func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
			var request struct {
				MessageID int64 `json:"messageId"`
			}
			if err := json.Unmarshal(arguments, &request); err != nil {
				return Intent.Plan{}, err
			}
			if code == planFailureCode {
				return Intent.Plan{}, fmt.Errorf("spy report %d was deleted from the local notice index", request.MessageID)
			}
			payload, _ := json.Marshal(map[string]int64{"MID": request.MessageID})
			return Intent.Plan{
				Summary: fmt.Sprintf("Fetch spy report %d", request.MessageID),
				Steps: []Intent.Step{{
					Name: "Fetch spy report", Opcode: "bsd", AwaitOpcode: "bsd", TimeoutMillis: 1000, SuccessCodes: []int{0},
					Command: Protocol.Command{Opcode: "bsd", Payload: payload},
				}},
			}, nil
		}}); err != nil {
		t.Fatal(err)
	}
	return Intent.NewEngine(intents, store, nil, sender, pipeline), sender
}

func TestManagerClassifiesSpyReportUnavailabilityFromStructuredResponse(t *testing.T) {
	for _, test := range []struct {
		name       string
		messageID  int64
		code       int
		wantStatus string
	}{
		{"generic failure with 130 in message id", 2221968130, 500, "error"},
		{"generic failure with 130 and 66 in message id", 1300066, 500, "error"},
		{"generic failure with 66 and 130 in message id", 66130, 500, "error"},
		{"planning error text with 130 and deleted", 2221968130, planFailureCode, "error"},
		{"transport error text with unavailable", 66130, -1, "error"},
		{"no spy data", 2221968130, 130, "unavailable"},
		{"deleted message", 1300066, 66, "unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			history, err := History.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			gameState := State.NewGameState()
			gameState.Session = State.SessionState{
				Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
				Status: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_21",
			}
			gameState.Reports.Notices[test.messageID] = State.ReportNotice{MessageID: test.messageID, TypeID: 3, Status: "pending"}
			store := State.NewStore(&gameState)
			engine, sender := newReportFetchEngine(t, store, test.code)
			manager := NewManager(store, history, engine)

			before := time.Now()
			manager.processNext(t.Context())
			accessorState1 := store.ReadOnlyView()
			notice, _ := accessorState1.LookupReportNotice(test.messageID)
			wantSends := 1
			if test.code == planFailureCode {
				wantSends = 0
			}
			if sender.sends != wantSends || notice.Status != test.wantStatus {
				t.Fatalf("sends=%d status=%q, want %q", sender.sends, notice.Status, test.wantStatus)
			}
			next, retrying := manager.nextAttempt[test.messageID]
			if test.wantStatus == "error" {
				if !retrying || next.Before(before.Add(reportRetryDelay-time.Second)) {
					t.Fatalf("generic failure retry = %v scheduled=%t", next, retrying)
				}
			} else if retrying {
				t.Fatalf("unavailable report kept a retry at %v", next)
			}
			manager.processNext(t.Context())
			if sender.sends != wantSends {
				t.Fatalf("report was fetched again immediately: %d sends", sender.sends)
			}
			if automations := store.ReadOnlyView().Automations; len(automations) != 0 {
				t.Fatalf("report fetch failure locked a lane: %#v", automations)
			}
		})
	}
}

func TestReportFetchUnavailableIgnoresReceiptText(t *testing.T) {
	code := func(value int) *int { return &value }
	for _, test := range []struct {
		name    string
		receipt Intent.Receipt
		want    bool
	}{
		{"text only", Intent.Receipt{Error: "response code 130 for BSD: report 66 unavailable and deleted"}, false},
		{"other opcode 130", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "blm", GameCode: code(130)}}, false},
		{"bsd 130", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bsd", GameCode: code(130)}}, true},
		{"bsd 66", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "BSD", GameCode: code(66)}}, true},
		{"bsd other", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bsd", GameCode: code(1300066)}}, false},
		{"battle summary 66 keeps terminal handling", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bls", GameCode: code(66)}}, true},
		{"battle summary other", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bls", GameCode: code(130)}}, false},
		{"battle waves 66 keeps terminal handling", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "blm", GameCode: code(66)}}, true},
		{"battle details 66 keeps terminal handling", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bld", GameCode: code(66)}}, true},
		{"no code", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bsd"}}, false},
	} {
		if got := reportFetchUnavailable(test.receipt); got != test.want {
			t.Errorf("%s: unavailable=%t want %t", test.name, got, test.want)
		}
	}
}
