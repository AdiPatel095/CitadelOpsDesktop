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
	commands []Protocol.Frame
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
	sender.commands = append(sender.commands, command)
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
		{"battle summary expired", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bls", GameCode: code(225)}}, true},
		{"battle summary code contains 225", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bls", GameCode: code(2250)}}, false},
		{"message id contains 225", Intent.Receipt{Error: "BLS report 1225 unavailable", Failure: &Intent.FailurePresentation{GameOpcode: "bls", GameCode: code(500)}}, false},
		{"expired text only", Intent.Receipt{Error: "response code 225 for BLS: report deleted"}, false},
		{"opcode contains bls", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "other-bls", GameCode: code(225)}}, false},
		{"battle waves expired is not terminal", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "blm", GameCode: code(225)}}, false},
		{"battle details expired is not terminal", Intent.Receipt{Failure: &Intent.FailurePresentation{GameOpcode: "bld", GameCode: code(225)}}, false},
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

// Replay the BLS/225 sequence from the 2026-10-03 opcode investigation,
// replacing message IDs with synthetic values. The planner mirrors
// App.planBattleReportSummary's sender payload: MID and IM=0.
func TestManagerReplaysExpiredBattleReportSummaries(t *testing.T) {
	for _, test := range []struct {
		name       string
		code       int
		wantStatus string
	}{
		{"captured expired response", 225, "unavailable"},
		{"deleted message unchanged", 66, "unavailable"},
		{"code contains 225", 2250, "error"},
		{"message id contains 225", 500, "error"},
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
			messageIDs := []int64{101, 102, 1225}
			for _, messageID := range messageIDs {
				gameState.Reports.Notices[messageID] = State.ReportNotice{
					MessageID: messageID, TypeID: 6, BattleKey: "battle#synthetic", Status: "pending",
				}
			}
			store := State.NewStore(&gameState)
			registry := Ingest.NewRegistry()
			if err := Ingest.RegisterCoreReducers(registry); err != nil {
				t.Fatal(err)
			}
			pipeline := Ingest.NewPipeline(store, nil, registry)
			sender := &reportResponseSender{pipeline: pipeline, code: test.code}
			intents := Intent.NewRegistry()
			if err := intents.Register(Intent.Definition{Name: "report.battle.summary", Effect: Intent.EffectRead,
				Planner: func(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
					var request struct {
						MessageID int64 `json:"messageId"`
					}
					if err := json.Unmarshal(arguments, &request); err != nil {
						return Intent.Plan{}, err
					}
					payload, _ := json.Marshal(struct {
						MessageID int64 `json:"MID"`
						InboxMode int   `json:"IM"`
					}{request.MessageID, 0})
					return Intent.Plan{
						Summary: fmt.Sprintf("Fetch battle report summary %d", request.MessageID),
						Steps: []Intent.Step{{
							Name: "Fetch battle report summary", Opcode: "bls", AwaitOpcode: "bls", TimeoutMillis: 1000, SuccessCodes: []int{0},
							Command: Protocol.Command{Opcode: "bls", Payload: payload},
						}},
					}, nil
				}}); err != nil {
				t.Fatal(err)
			}
			engine := Intent.NewEngine(intents, store, nil, sender, pipeline)
			manager := NewManager(store, history, engine)
			for range messageIDs {
				manager.processNext(t.Context())
			}
			counts := map[int64]int{}
			for _, command := range sender.commands {
				var payload map[string]int64
				if err := json.Unmarshal(command.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if command.Opcode != "bls" || len(payload) != 2 || payload["IM"] != 0 {
					t.Fatalf("unexpected BLS sender payload: %#v", command)
				}
				counts[payload["MID"]]++
			}
			snapshot := store.ReadOnlyView()
			for _, messageID := range messageIDs {
				notice, _ := snapshot.LookupReportNotice(messageID)
				if counts[messageID] != 1 || notice.Status != test.wantStatus {
					t.Fatalf("report %d: sends=%d status=%q, want one send and %q", messageID, counts[messageID], notice.Status, test.wantStatus)
				}
				next, retrying := manager.nextAttempt[messageID]
				if test.wantStatus == "unavailable" {
					if retrying {
						t.Fatalf("expired report %d retained retry at %v", messageID, next)
					}
					// Even a stale, already-due retry cannot refetch a retired notice.
					manager.nextAttempt[messageID] = time.Now().Add(-reportRetryDelay)
				} else if !retrying || !next.After(time.Now()) {
					t.Fatalf("nonterminal report %d lost its retry: %v", messageID, next)
				}
			}
			for range 5 {
				manager.processNext(t.Context())
			}
			if sender.sends != len(messageIDs) {
				t.Fatalf("reports were fetched again: %d sends", sender.sends)
			}
			if automations := store.ReadOnlyView().Automations; len(automations) != 0 {
				t.Fatalf("report failure locked an automation lane: %#v", automations)
			}
		})
	}
}
