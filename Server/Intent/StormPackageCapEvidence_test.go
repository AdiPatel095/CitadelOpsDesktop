package Intent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestStormPackageCapEvidenceSanitizesWireAndPreservesHistory(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 9
	state.Session.ConnectionGeneration = 11
	state.Inventory.ConstructionOffersCastleID = 910040
	state.Inventory.ConstructionOffersKingdomID = 4
	state.Inventory.ConstructionOffersObservedAt = time.Now().UTC()
	state.Inventory.ConstructionOffers = map[State.PackageID]int64{910126: 4, 910127: 1}
	buffer := &operationEvidenceBuffer{}
	ctx := context.WithValue(t.Context(), operationEvidenceContextKey{}, buffer)
	code := 237
	command := Protocol.Command{Opcode: "sbp", Payload: json.RawMessage(`{"PID":910126,"BT":3,"TID":-1,"AMT":4,"KID":4,"AID":-1,"PC2":-1,"BA":0,"PWR":0,"_PO":-1,"private":"fixture-secret"}`)}
	response := Protocol.Frame{Opcode: "sbp", ResponseCode: &code, Payload: json.RawMessage(`{"PID":910126,"AMT":4,"private":"fixture-secret"}`), Raw: "fixture-secret", ReceivedAt: time.Now().UTC()}
	if err := recordStormPackageCapEvidence(ctx, command, response, state, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	items := buffer.drain()
	if len(items) != 1 || items[0].Kind != "sbp_cap_rejection" {
		t.Fatalf("evidence=%+v", items)
	}
	if strings.Contains(string(items[0].Data), "fixture-secret") || strings.Contains(string(items[0].Data), "private") {
		t.Fatal("private wire content leaked")
	}
	var evidence struct {
		OccurrenceKey string `json:"occurrenceKey"`
		Generation    uint64 `json:"generation"`
		GBC           struct {
			PL []struct {
				PID int64 `json:"PID"`
				AMT int64 `json:"AMT"`
			} `json:"PL"`
		} `json:"gbc"`
		Code int `json:"responseCode"`
	}
	if err := json.Unmarshal(items[0].Data, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.OccurrenceKey != "910040:-1" || evidence.Generation != 9 || len(evidence.GBC.PL) != 2 || evidence.GBC.PL[0].AMT != 4 || evidence.Code != 237 {
		t.Fatalf("evidence=%s", items[0].Data)
	}
}

func TestStormPackageCapEvidenceRecordedBeforeLaneLock(t *testing.T) {
	state := State.NewGameState()
	state.Inventory.ConstructionOffersCastleID = 910040
	state.Inventory.ConstructionOffersKingdomID = 4
	state.Inventory.ConstructionOffersObservedAt = time.Now().UTC()
	store := State.NewStore(&state)
	pipeline := Ingest.NewPipeline(store, nil, Ingest.NewRegistry())
	capSender := &stormCapEvidenceSender{pipeline: pipeline}
	registry := NewRegistry()
	if err := registry.Register(Definition{Name: "fixture.cap", Effect: EffectWrite, Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
		return Plan{Steps: []Step{{Opcode: "sbp", AwaitOpcode: "sbp", CaptureResponse: true, SuccessCodes: []int{0}, Payload: json.RawMessage(`{"PID":910126,"BT":3,"TID":-1,"AMT":4,"KID":4,"AID":-1,"PC2":-1}`)}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(registry, store, nil, capSender, pipeline)
	receipt := engine.Submit(t.Context(), Request{ID: "fixture-cap-operation", Name: "fixture.cap", Actor: "automation:autoStormShop", AutomationLane: "autoStormShop"})
	if receipt.Status != StatusFailed || len(receipt.Evidence) != 1 || receipt.Evidence[0].Kind != "sbp_cap_rejection" {
		t.Fatalf("receipt=%+v", receipt)
	}
	if engine.AutomationLaneLock("autoStormShop").Code != 237 {
		t.Fatal("237 backstop lock changed")
	}
	if strings.Contains(string(receipt.Evidence[0].Data), "fixture-only") {
		t.Fatal("receipt evidence retained private field")
	}
}

type stormCapEvidenceSender struct{ pipeline *Ingest.Pipeline }

func (*stormCapEvidenceSender) Ready() bool       { return true }
func (*stormCapEvidenceSender) Namespace() string { return "EmpireEx_21" }
func (sender *stormCapEvidenceSender) Send(ctx context.Context, _ []byte) error {
	_, err := sender.pipeline.HandleRawAt(ctx, `%xt%sbp%1%237%{"private":"fixture-only"}%`, Protocol.DirectionInbound, time.Now().UTC())
	return err
}
