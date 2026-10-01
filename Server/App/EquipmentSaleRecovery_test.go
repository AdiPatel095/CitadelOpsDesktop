package App

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// Sanitized CIT-13 fixture: three unworn non-relic storage items.
var cit13SaleItems = []State.EquipmentInstanceID{6558434871, 6558434872, 6558434873}

type equipmentSaleEngineSender struct {
	pipeline        *Ingest.Pipeline
	opcodes         []string
	indeterminateAt int // 1-based SEQ send that never gets a reply
	seqSends        int
	geiItems        []State.EquipmentInstanceID
}

func (*equipmentSaleEngineSender) Ready() bool                  { return true }
func (*equipmentSaleEngineSender) Namespace() string            { return "EmpireEx_21" }
func (*equipmentSaleEngineSender) CorrelatesResponses() bool    { return true }
func (*equipmentSaleEngineSender) ConnectionGeneration() uint64 { return 1 }

func (sender *equipmentSaleEngineSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	sender.opcodes = append(sender.opcodes, command.Opcode)
	if _, err := sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: command.Opcode, Payload: command.Payload,
		ReceivedAt: time.Now().UTC(), CausationOperationID: metadata.OperationID,
	}); err != nil {
		return err
	}
	responsePayload := json.RawMessage(`{"gcu":{"C1":1000},"esl":{"E":10,"TE":100}}`)
	if command.Opcode == "seq" {
		sender.seqSends++
		if sender.seqSends == sender.indeterminateAt {
			return Outbound.MarkIndeterminate(errors.New("seq response timed out"))
		}
	}
	if command.Opcode == "gei" {
		responsePayload = cit13StoragePayload(sender.geiItems)
	}
	code := 0
	_, err = sender.pipeline.HandleFrame(ctx, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: command.Opcode, ResponseCode: &code,
		ReceivedAt: time.Now().UTC(), Payload: responsePayload, ResponseToken: metadata.ResponseToken,
	})
	return err
}

func cit13StoragePayload(ids []State.EquipmentInstanceID) json.RawMessage {
	rows := make([][]any, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, []any{int64(id), 1, 0, 2, 100, []any{}, 100})
	}
	payload, _ := json.Marshal(map[string]any{"I": rows})
	return payload
}

func cit13SaleState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		Status: "connected", LoggedIn: true, SocketReady: true, Namespace: "EmpireEx_21",
	}
	code := 0
	for _, opcode := range []string{"gei", "ggm"} {
		gameState.Observations[opcode] = State.ProtocolObservation{
			Opcode: opcode, LastDirection: "inbound", LastCode: &code, LastSeenAt: now, LastSuccessfulInboundAt: now,
		}
	}
	for _, id := range cit13SaleItems {
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, DefinitionID: 100, Slot: 1, RarityID: 2}
	}
	return gameState
}

func newEquipmentSaleEngine(t *testing.T, gameState State.GameState, sender *equipmentSaleEngineSender) (*Intent.Engine, *Application, *State.Store) {
	t.Helper()
	stateStore := State.NewStore(&gameState)
	ingestRegistry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(ingestRegistry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, nil, ingestRegistry)
	sender.pipeline = pipeline
	intentRegistry := Intent.NewRegistry()
	if err := intentRegistry.Register(Intent.Definition{Name: "equipment.sell", Effect: Intent.EffectWrite, Planner: planEquipmentSell}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(intentRegistry, stateStore, nil, sender, pipeline)
	application := &Application{State: stateStore, Intents: engine, Ingest: pipeline}
	if err := engine.RegisterAction(equipmentSaleGuardAction, application.guardEquipmentSale); err != nil {
		t.Fatal(err)
	}
	return engine, application, stateStore
}

func cit13SaleRequest(id string) Intent.Request {
	return Intent.Request{
		ID: id, Name: "equipment.sell", Actor: "automation:autoEquipmentCleanup", AutomationLane: "autoEquipmentCleanup",
		Arguments: json.RawMessage(`{"category":"non_relic_equipment"}`),
	}
}

// Two confirmed sales, then a control-plane interruption, then reevaluation.
func TestEquipmentSaleInterruptionNeverRedispatchesConfirmedSales(t *testing.T) {
	sender := &equipmentSaleEngineSender{}
	engine, application, stateStore := newEquipmentSaleEngine(t, cit13SaleState(time.Now().UTC()), sender)
	beforeStep := 0
	engine.SetExecutionGate(func(_ context.Context, _ Intent.Request, _ Intent.Plan, point Intent.ExecutionPoint) error {
		if point != Intent.ExecutionBeforeStep {
			return nil
		}
		beforeStep++
		if beforeStep == 3 {
			return errors.New("control plane interrupted the operation")
		}
		return nil
	})
	receipt := engine.Submit(t.Context(), cit13SaleRequest("cit13-sale-interrupted"))
	if receipt.Status != Intent.StatusPartiallySucceeded || !slices.Equal(receipt.CompletedStepIndexes, []int{0, 1}) {
		t.Fatalf("interrupted sale receipt = status %s completed %v err %s", receipt.Status, receipt.CompletedStepIndexes, receipt.Error)
	}
	if !slices.Equal(sender.opcodes, []string{"seq", "seq"}) {
		t.Fatalf("dispatched opcodes = %v", sender.opcodes)
	}
	view := stateStore.ReadOnlyView()
	for _, sold := range cit13SaleItems[:2] {
		if _, found := view.Inventory.Equipment[sold]; found {
			t.Fatalf("confirmed sold item %d remains in inventory", sold)
		}
	}
	if _, found := view.Inventory.Equipment[cit13SaleItems[2]]; !found {
		t.Fatal("unsold item was removed")
	}
	if _, err := planEquipmentSell(t.Context(), Intent.PlanningContext{State: view}, json.RawMessage(`{"category":"non_relic_equipment"}`)); err == nil ||
		!strings.Contains(err.Error(), "storage is stale") {
		t.Fatalf("reevaluation planned from pre-sale storage: %v", err)
	}
	for _, sold := range cit13SaleItems[:2] {
		arguments, _ := json.Marshal(equipmentSaleGuardRequest{EquipmentID: sold, Category: "non_relic_equipment"})
		if err := application.guardEquipmentSale(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) {
			t.Fatalf("final guard allowed re-dispatch of sold item %d: %v", sold, err)
		}
	}
	if len(view.Automations) != 0 {
		t.Fatalf("interruption created a lane lock: %#v", view.Automations)
	}

	// Refresh storage: the server reports only the remaining item.
	code := 0
	if _, err := application.Ingest.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gei", ResponseCode: &code, ReceivedAt: time.Now().UTC(),
		Payload: cit13StoragePayload(cit13SaleItems[2:]),
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := planEquipmentSell(t.Context(), Intent.PlanningContext{State: stateStore.ReadOnlyView()}, json.RawMessage(`{"category":"non_relic_equipment"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "seq" || plan.Steps[1].Opcode != "gei" ||
		!strings.Contains(string(plan.Steps[0].Command.Payload), "6558434873") ||
		plan.Steps[0].FinalDispatchAction != equipmentSaleGuardAction {
		t.Fatalf("replanned sale = %#v", plan.Steps)
	}
}

// A timed-out SEQ is never inferred as a sale and is never re-dispatched until
// a storage snapshot newer than it resolves the outcome.
func TestEquipmentSaleTimeoutBlocksRedispatchUntilStorageRefresh(t *testing.T) {
	sender := &equipmentSaleEngineSender{indeterminateAt: 1}
	engine, application, stateStore := newEquipmentSaleEngine(t, cit13SaleState(time.Now().UTC()), sender)
	receipt := engine.Submit(t.Context(), cit13SaleRequest("cit13-sale-timeout"))
	if receipt.Status != Intent.StatusIndeterminate || len(receipt.CompletedStepIndexes) != 0 {
		t.Fatalf("timed-out sale receipt = status %s completed %v err %s", receipt.Status, receipt.CompletedStepIndexes, receipt.Error)
	}
	view := stateStore.ReadOnlyView()
	if len(view.Inventory.Equipment) != len(cit13SaleItems) {
		t.Fatalf("timeout inferred a sale: %#v", view.Inventory.Equipment)
	}
	if _, err := planEquipmentSell(t.Context(), Intent.PlanningContext{State: view}, json.RawMessage(`{"category":"non_relic_equipment"}`)); err == nil {
		t.Fatal("next batch was planned before a storage refresh")
	}
	arguments, _ := json.Marshal(equipmentSaleGuardRequest{EquipmentID: cit13SaleItems[0], Category: "non_relic_equipment"})
	if err := application.guardEquipmentSale(t.Context(), arguments); !errors.Is(err, Intent.ErrPlanStale) ||
		!strings.Contains(err.Error(), "unresolved sale") {
		t.Fatalf("final guard allowed re-dispatch of an unresolved sale: %v", err)
	}

	// The refresh proves the sale did not happen; the item becomes sellable again.
	code := 0
	if _, err := application.Ingest.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gei", ResponseCode: &code, ReceivedAt: time.Now().UTC().Add(time.Millisecond),
		Payload: cit13StoragePayload(cit13SaleItems),
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.guardEquipmentSale(t.Context(), arguments); err != nil {
		t.Fatalf("refreshed storage still blocked the item: %v", err)
	}
}

func TestEquipmentSaleGuardKeepsProtectedBoundary(t *testing.T) {
	gameState := cit13SaleState(time.Now().UTC())
	gameState.Inventory.Equipment[7] = State.EquipmentInstance{ID: 7, DefinitionID: 100, Slot: 1, RarityID: 2, WearerKind: "commander", WearerID: 5}
	gameState.Inventory.Equipment[8] = State.EquipmentInstance{ID: 8, DefinitionID: 100, Slot: 1, RarityID: 5}
	gameState.Inventory.Equipment[9] = State.EquipmentInstance{ID: 9, DefinitionID: 100, Slot: 5, RarityID: 2}
	gameState.Inventory.GemStacks[20] = 1
	gameState.Inventory.GemStacks[500] = 1
	for name, request := range map[string]equipmentSaleGuardRequest{
		"worn":        {EquipmentID: 7, Category: "non_relic_equipment"},
		"relic":       {EquipmentID: 8, Category: "non_relic_equipment"},
		"look":        {EquipmentID: 9, Category: "non_relic_equipment"},
		"missing":     {EquipmentID: 10, Category: "non_relic_equipment"},
		"post2026":    {GemID: 500, Category: "non_relic_gems"},
		"empty gem":   {GemID: 21, Category: "non_relic_gems"},
		"relic gem":   {GemID: 99, RelicGem: true, Category: "relic1_gems"},
		"no identity": {Category: "non_relic_equipment"},
	} {
		if err := validateEquipmentSaleDispatch(gameState, request); err == nil {
			t.Errorf("%s sale passed the final guard", name)
		}
	}
	if err := validateEquipmentSaleDispatch(gameState, equipmentSaleGuardRequest{EquipmentID: cit13SaleItems[0], Category: "non_relic_equipment"}); err != nil {
		t.Fatalf("eligible item refused: %v", err)
	}
	if err := validateEquipmentSaleDispatch(gameState, equipmentSaleGuardRequest{GemID: 20, Category: "non_relic_gems"}); err != nil {
		t.Fatalf("eligible gem stack refused: %v", err)
	}
	gameState.RecordPendingCommandRequest(State.PendingCommandRequest{Opcode: "sge", SentAt: time.Now().UTC().Add(time.Second), GemID: 20})
	if err := validateEquipmentSaleDispatch(gameState, equipmentSaleGuardRequest{GemID: 20, Category: "non_relic_gems"}); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("stack with every unit pending was re-dispatched: %v", err)
	}
}
