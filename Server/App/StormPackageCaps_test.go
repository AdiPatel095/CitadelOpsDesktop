package App

import (
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestStormPackageCapPlannerSkipsBlockedAndKeepsOtherPackages(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"packages":[{"packageID":3119,"comment1":"Fixture capped","comment2":"Luna's trade boat","packageType":"resource","packagePriceAquamarine":10,"stock":4},{"packageID":245,"comment1":"Fixture eligible","comment2":"Luna's trade boat","packageType":"resource","packagePriceAquamarine":10,"stock":4}]}`), GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[910040] = State.CastleState{ID: 910040, KingdomID: 4, Resources: map[State.ResourceID]State.ResourceBalance{GameData.StormAquamarineID: {Amount: 1000}}}
	state.Inventory.ConstructionOffersCastleID = 910040
	state.Inventory.ConstructionOffersKingdomID = 4
	state.Inventory.ConstructionOffersObservedAt = time.Now().UTC()
	state.BlockStormPackage(910040, -1, 3119, 4, time.Now().UTC())
	args := json.RawMessage(`{"castleId":910040,"purchases":[{"productId":3119,"amount":4},{"productId":245,"amount":1}]}`)
	plan, err := planStormShopPurchase(t.Context(), Intent.PlanningContext{State: state, GameData: data}, args)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, step := range plan.Steps {
		if step.Opcode == "sbp" {
			count++
			var payload struct {
				PID int64 `json:"PID"`
				PC2 int64 `json:"PC2"`
			}
			if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload.PID != 245 || payload.PC2 != -1 {
				t.Fatalf("unsafe purchase: %s", step.Command.Payload)
			}
			if step.FinalDispatchAction != "storm.shop.guard" {
				t.Fatal("missing final-dispatch guard")
			}
		}
	}
	if count != 1 {
		t.Fatalf("SBP count=%d", count)
	}
	blockedArgs := json.RawMessage(`{"castleId":910040,"productId":3119,"amount":4}`)
	if _, err := planStormShopPurchase(t.Context(), Intent.PlanningContext{State: state, GameData: data}, blockedArgs); err == nil {
		t.Fatal("second SBP planned after 237")
	}
	if _, _, _, err := stormShopPurchaseContext(Intent.PlanningContext{State: state, GameData: data}, blockedArgs, true); err == nil {
		t.Fatal("queued purchase passed guard after 237")
	}
	for _, step := range plan.Steps {
		if step.Action == "storm.shop.guard" {
			if _, _, _, err := stormShopPurchaseContext(Intent.PlanningContext{State: state, GameData: data}, step.ActionArguments, true); err != nil {
				t.Fatalf("batch guard still includes blocked package: %v", err)
			}
		}
	}
	state.Castles[910041] = State.CastleState{ID: 910041, KingdomID: 4, Resources: state.Castles[910040].Resources}
	if _, err := planStormShopPurchase(t.Context(), Intent.PlanningContext{State: state, GameData: data}, json.RawMessage(`{"castleId":910041,"productId":3119,"amount":1}`)); err != nil {
		t.Fatalf("new occurrence remains blocked: %v", err)
	}
}

// Full planner/engine/reducer replay: 48 entries at cap, 43 omitting it, 237,
// then another purchase request in the same occurrence. No second SBP sends.
func TestStormPackageCapEndToEndReplayNoSecondSBP(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"packages":[{"packageID":3119,"comment1":"Fixture package","comment2":"Luna's trade boat","packageType":"currency","packagePriceAquamarine":10,"stock":4}]}`), GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	initial := State.NewGameState()
	initial.Castles[910040] = State.CastleState{ID: 910040, KingdomID: 4, Focused: true, Resources: map[State.ResourceID]State.ResourceBalance{GameData.StormAquamarineID: {Amount: 1000}}}
	store := State.NewStore(&initial)
	reducers := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(reducers); err != nil {
		t.Fatal(err)
	}
	provider := khanTauntGameDataProvider{store: data}
	pipeline := Ingest.NewPipeline(store, provider, reducers)
	sender := &stormCapReplaySender{pipeline: pipeline}
	for _, history := range []struct {
		size    int
		include bool
	}{{48, true}, {43, false}} {
		if _, err := pipeline.HandleRawAt(t.Context(), `%xt%EmpireEx_21%gbc%1%0%{"CID":910040,"KID":4}%`, Protocol.DirectionOutbound, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		payload := stormCapReplayHistory(history.size, history.include)
		if _, err := pipeline.HandleRawAt(t.Context(), fmt.Sprintf("%%xt%%gbc%%1%%0%%%s%%", payload), Protocol.DirectionInbound, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "fixture.storm.cap", Effect: Intent.EffectWrite, Planner: planStormShopPurchase}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, store, provider, sender, pipeline)
	if err := engine.RegisterAction("storm.shop.guard", func(_ context.Context, args json.RawMessage) error {
		_, _, _, err := stormShopPurchaseContext(Intent.PlanningContext{State: store.ReadOnlyView(), GameData: data}, args, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`{"castleId":910040,"productId":3119,"amount":4}`)
	first := engine.Submit(t.Context(), Intent.Request{ID: "fixture-cap-first", Name: "fixture.storm.cap", Arguments: args})
	if sender.purchases != 1 || first.Status != Intent.StatusPartiallySucceeded || len(first.Evidence) != 1 || first.Evidence[0].Kind != "sbp_cap_rejection" {
		t.Fatalf("first status=%s evidence=%d purchases=%d error=%s", first.Status, len(first.Evidence), sender.purchases, first.Error)
	}
	var evidence struct {
		GBC struct {
			PL []json.RawMessage `json:"PL"`
		} `json:"gbc"`
	}
	if err := json.Unmarshal(first.Evidence[0].Data, &evidence); err != nil {
		t.Fatal(err)
	}
	if len(evidence.GBC.PL) != 43 {
		t.Fatalf("wrong relied-on history length %d", len(evidence.GBC.PL))
	}
	second := engine.Submit(t.Context(), Intent.Request{ID: "fixture-cap-second", Name: "fixture.storm.cap", Arguments: args})
	if sender.purchases != 1 || second.Status != Intent.StatusFailed || second.Failure == nil || second.Failure.ExplanationDescriptor == nil || second.Failure.ExplanationDescriptor.Key != "server.storm.package_cap_blocked" {
		t.Fatalf("second status=%s purchases=%d error=%s", second.Status, sender.purchases, second.Error)
	}
}

func stormCapReplayHistory(size int, include bool) json.RawMessage {
	products := make([]map[string]int64, 0, size)
	if include {
		products = append(products, map[string]int64{"PID": 3119, "AMT": 4})
	}
	for len(products) < size {
		products = append(products, map[string]int64{"PID": 920000 + int64(len(products)), "AMT": 1})
	}
	payload, _ := json.Marshal(map[string]any{"PL": products})
	return payload
}

type stormCapReplaySender struct {
	pipeline  *Ingest.Pipeline
	purchases int
}

func (*stormCapReplaySender) Ready() bool       { return true }
func (*stormCapReplaySender) Namespace() string { return "EmpireEx_21" }
func (sender *stormCapReplaySender) Send(ctx context.Context, raw []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	frame, err := Protocol.Decode(string(raw), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err := sender.pipeline.HandleRawAt(ctx, string(raw), Protocol.DirectionOutbound, time.Now().UTC()); err != nil {
		return err
	}
	code := 0
	payload := json.RawMessage(`{}`)
	switch frame.Opcode {
	case "jaa":
		payload = json.RawMessage(`{"KID":4,"gca":{"A":[1,17,23,910040]}}`)
	case "gbc":
		payload = stormCapReplayHistory(43, false)
	case "sbp":
		sender.purchases++
		code = 237
	}
	_, err = sender.pipeline.HandleRawAt(ctx, fmt.Sprintf("%%xt%%%s%%1%%%d%%%s%%", frame.Opcode, code, payload), Protocol.DirectionInbound, time.Now().UTC())
	return err
}
