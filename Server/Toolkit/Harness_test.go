package Toolkit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Models"
)

func TestDefaultHarnessDiscoveryAndReadOnlyPolicy(t *testing.T) {
	harness, err := NewDefaultHarness()
	if err != nil {
		t.Fatal(err)
	}
	definitions := harness.Definitions()
	if len(definitions) != 13 {
		t.Fatalf("tool definitions=%d, want 13", len(definitions))
	}
	for _, definition := range definitions {
		if !json.Valid(definition.InputSchema) {
			t.Fatalf("tool %s has invalid schema", definition.Name)
		}
	}
	manifest := harness.Manifest()
	if manifest.ContractVersion != ContractVersion || manifest.Toolkit != "citadel_ops" || len(manifest.Tools) != len(definitions) {
		t.Fatalf("invalid manifest: %+v", manifest)
	}

	stateResult := harness.Execute(context.Background(), Call{
		Name:      "citadel.state.read",
		Arguments: json.RawMessage(`{"scope":"summary"}`),
	})
	if !stateResult.OK {
		t.Fatalf("state read failed: %+v", stateResult.Error)
	}

	previewResult := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.preview",
		Arguments: json.RawMessage(`{"name":"jca","arguments":{"castleId":1,"kingdomId":0}}`),
	})
	if !previewResult.OK || !strings.Contains(string(previewResult.Content), "%jca%") {
		t.Fatalf("command preview failed: ok=%v error=%+v content=%s", previewResult.OK, previewResult.Error, previewResult.Content)
	}

	sendResult := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.send",
		Arguments: json.RawMessage(`{"name":"jca","arguments":{"castleId":1,"kingdomId":0}}`),
	})
	if sendResult.OK || sendResult.Error == nil || sendResult.Error.Code != "unauthorized" {
		t.Fatalf("read-only send result=%+v", sendResult)
	}
	if sendResult.Effect != EffectGameQuery {
		t.Fatalf("resolved effect=%q, want %q", sendResult.Effect, EffectGameQuery)
	}
}

func TestCommandTraceToolReturnsRedactedTelemetry(t *testing.T) {
	previous := Automation.CommandTraces
	Automation.CommandTraces = Automation.NewCommandTraceTracker(8)
	defer func() { Automation.CommandTraces = previous }()
	Automation.CommandTraces.RecordNativeSent([]byte(`%xt%EmpireEx_21%jca%1%{"CID":7654321,"KID":2}%`))
	Automation.CommandTraces.RecordQueued(Automation.Command{
		ID:           3,
		BrokerID:     81,
		HarnessID:    82,
		SubmissionID: 83,
		Owner:        Automation.OwnerToolkit,
		Surface:      Automation.CommandSurfaceToolkit,
		Opcode:       "gam",
		Payload:      []byte(`%xt%EmpireEx_21%gam%1%{}%`),
	})

	harness, err := NewDefaultHarness()
	if err != nil {
		t.Fatal(err)
	}
	recent := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.trace",
		Arguments: json.RawMessage(`{"mode":"recent","opcode":"jca","limit":5}`),
	})
	if !recent.OK || !strings.Contains(string(recent.Content), "object{CID:integer,KID:integer}") {
		t.Fatalf("recent command trace failed: ok=%v error=%+v content=%s", recent.OK, recent.Error, recent.Content)
	}
	if strings.Contains(string(recent.Content), `"CID":7654321`) {
		t.Fatalf("trace tool exposed a payload value: %s", recent.Content)
	}
	if strings.Contains(string(recent.Content), "0001-01-01") {
		t.Fatalf("trace tool exposed unset timestamps: %s", recent.Content)
	}
	byReceipt := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.trace",
		Arguments: json.RawMessage(`{"mode":"recent","brokerId":81,"harnessId":82,"submissionId":83}`),
	})
	if !byReceipt.OK || !strings.Contains(string(byReceipt.Content), `"opcode":"gam"`) || strings.Contains(string(byReceipt.Content), `"opcode":"jca"`) {
		t.Fatalf("receipt identity trace lookup failed: ok=%v error=%+v content=%s", byReceipt.OK, byReceipt.Error, byReceipt.Content)
	}

	variants := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.trace",
		Arguments: json.RawMessage(`{"mode":"variants","opcode":"jca"}`),
	})
	if !variants.OK || !strings.Contains(string(variants.Content), `"count":1`) {
		t.Fatalf("command variants failed: ok=%v error=%+v content=%s", variants.OK, variants.Error, variants.Content)
	}
}

func TestContextCommandPlansFromCastleState(t *testing.T) {
	gameState := Models.GetGameState()
	originalCastle := gameState.Castle.MainCastle
	originalFocus := gameState.CastleFocus
	defer func() {
		gameState.Castle.MainCastle = originalCastle
		gameState.CastleFocus = originalFocus
	}()
	gameState.Castle.MainCastle = Models.PlayerCastleInfo{
		Name:         "Highgarden",
		Aid:          4242,
		MapKingdomID: 0,
		MapX:         1200,
		MapY:         1300,
	}
	gameState.CastleFocus.CastleAID = 4242

	harness, err := NewDefaultHarness()
	if err != nil {
		t.Fatal(err)
	}
	result := harness.Execute(context.Background(), Call{
		Name: "citadel.context_command.plan",
		Arguments: json.RawMessage(`{
			"name":"focus_castle",
			"arguments":{"castle":{"key":"mainCastle"}}
		}`),
	})
	if !result.OK {
		t.Fatalf("context plan failed: %+v", result.Error)
	}
	var plan ContextCommandPlan
	if err := json.Unmarshal(result.Content, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.PlanID == "" || plan.Command != "focus_castle" || len(plan.Steps) != 1 || plan.Steps[0].Castle == nil {
		t.Fatalf("unexpected context plan: %+v", plan)
	}
	if plan.Steps[0].Castle.CastleID != 4242 || plan.Steps[0].Castle.MapX != 1200 || plan.Steps[0].Castle.MapY != 1300 {
		t.Fatalf("castle context was not resolved: %+v", plan.Steps[0].Castle)
	}

	executeResult := harness.Execute(context.Background(), Call{
		Name: "citadel.context_command.execute",
		Arguments: json.RawMessage(`{
			"name":"focus_castle",
			"arguments":{"castle":{"key":"mainCastle"}}
		}`),
	})
	if executeResult.OK || executeResult.Error == nil || executeResult.Error.Code != "unauthorized" {
		t.Fatalf("read-only contextual execute result=%+v", executeResult)
	}
	if executeResult.Effect != EffectGameQuery {
		t.Fatalf("resolved effect=%q, want %q", executeResult.Effect, EffectGameQuery)
	}
}

func TestContextCommandCatalogCoverage(t *testing.T) {
	commands, err := newCommandSpecs()
	if err != nil {
		t.Fatal(err)
	}
	contextCommands, err := newContextCommandSpecs(commands)
	if err != nil {
		t.Fatal(err)
	}
	if len(contextCommands) != 10 {
		t.Fatalf("context command specs=%d, want 10", len(contextCommands))
	}
	for _, name := range []string{
		"refresh_account", "focus_castle", "queue_production", "heal_wounded", "discard_wounded",
		"send_market_resource", "send_kingdom_resource", "store_decoration", "place_item", "open_tci_offers",
	} {
		if _, ok := contextCommands[name]; !ok {
			t.Errorf("missing contextual command %q", name)
		}
	}
}

func TestContextQueueProductionDerivesCastleStackAmount(t *testing.T) {
	gameState := Models.GetGameState()
	originalCastle := gameState.Castle.MainCastle
	defer func() {
		gameState.Castle.MainCastle = originalCastle
	}()
	gameState.Castle.MainCastle = Models.PlayerCastleInfo{
		Name:               "Highgarden",
		Aid:                4242,
		MapKingdomID:       0,
		MapX:               1200,
		MapY:               1300,
		BuildingRowsLoaded: true,
		BDRows: []Models.BuildingData{
			{BuildingID: 160, OID: 9001, Level: 1},
		},
		QueueableIDsLoaded: true,
		QueueableUnitIDs:   []int{602},
	}

	harness, err := NewDefaultHarness()
	if err != nil {
		t.Fatal(err)
	}
	result := harness.Execute(context.Background(), Call{
		Name: "citadel.context_command.plan",
		Arguments: json.RawMessage(`{
			"name":"queue_production",
			"arguments":{"castle":{"id":4242},"kind":"troop","itemId":602}
		}`),
	})
	if !result.OK {
		t.Fatalf("context production plan failed: %+v", result.Error)
	}
	var plan ContextCommandPlan
	if err := json.Unmarshal(result.Content, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Kind != "focus_castle" || plan.Steps[1].Command == nil || plan.Steps[1].Command.Name != "bup" {
		t.Fatalf("unexpected production plan steps: %+v", plan.Steps)
	}
	var purchase productionPurchaseArgs
	if err := json.Unmarshal(plan.Steps[1].Command.Arguments, &purchase); err != nil {
		t.Fatal(err)
	}
	if purchase.Amount != 80 || purchase.CastleID != 4242 || purchase.LineID != 0 {
		t.Fatalf("resolved production arguments=%+v, want amount=80 castle=4242 line=0", purchase)
	}
}

func TestStateAwaitUsesParserVersionCursor(t *testing.T) {
	harness, err := NewDefaultHarness()
	if err != nil {
		t.Fatal(err)
	}
	stateKey := "toolkit-test"
	before := Automation.StateSnapshot(stateKey).Version
	Automation.ObserveState(stateKey)
	result := harness.Execute(context.Background(), Call{
		Name: "citadel.state.await",
		Arguments: json.RawMessage(`{
			"stateKey":"toolkit-test",
			"afterVersion":` + jsonNumber(before) + `,
			"timeoutMs":100
		}`),
	})
	if !result.OK {
		t.Fatalf("state await failed: %+v", result.Error)
	}
}

func TestCommandCatalogCoversPayloadBuilders(t *testing.T) {
	specs, err := newCommandSpecs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 130 {
		t.Fatalf("command specs=%d, want 130", len(specs))
	}
	for _, name := range []string{
		"castle_focus", "cra", "cds", "rpc", "ubc", "gbc", "sbp",
		"crin", "crst", "crun", "crsk", "crm", "kgt", "kingdom_resource_msk",
		"bls", "blm", "bld", "bsd", "ssi", "mfs",
	} {
		if _, ok := specs[name]; !ok {
			t.Errorf("missing command spec %q", name)
		}
	}
}

func TestOutboundExampleCorpusHasExplicitCommandCoverage(t *testing.T) {
	specs, err := newCommandSpecs()
	if err != nil {
		t.Fatal(err)
	}
	coveredOpcodes := make(map[string]bool)
	for _, spec := range specs {
		for _, opcode := range strings.Split(spec.definition.Opcode, "|") {
			coveredOpcodes[opcode] = true
		}
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate toolkit test source")
	}
	examplePattern := filepath.Join(filepath.Dir(sourceFile), "..", "..", "Logs", "SendCommandsJSON", "*.json")
	examples, err := filepath.Glob(examplePattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) != 96 {
		t.Fatalf("outbound examples=%d, want 96", len(examples))
	}
	for _, example := range examples {
		opcode := strings.TrimSuffix(filepath.Base(example), filepath.Ext(example))
		if coveredOpcodes[opcode] {
			continue
		}
		if _, excluded := intentionallyUnexposedOutboundExamples[opcode]; excluded {
			continue
		}
		t.Errorf("outbound example %q has no command spec or explicit exclusion", opcode)
	}
}

func TestCapturedOpcodePreviewAndStrictWireFields(t *testing.T) {
	harness, err := NewDefaultHarness()
	if err != nil {
		t.Fatal(err)
	}
	result := harness.Execute(context.Background(), Call{
		Name: "citadel.command.preview",
		Arguments: json.RawMessage(`{
			"name":"cat",
			"arguments":{
				"A":[[493,4770]],"BPC":0,"HBW":-1,"KID":0,"LID":0,
				"PTT":1,"SD":0,"SX":212,"SY":939,"TX":212,"TY":941,"WT":0
			}
		}`),
	})
	if !result.OK || !strings.Contains(string(result.Content), `%cat%1%`) {
		t.Fatalf("captured preview failed: ok=%v error=%+v content=%s", result.OK, result.Error, result.Content)
	}

	invalid := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.preview",
		Arguments: json.RawMessage(`{"name":"cat","arguments":{"A":[[493,1]]}}`),
	})
	if invalid.OK || invalid.Error == nil || invalid.Error.Code != "invalid_arguments" {
		t.Fatalf("incomplete captured command result=%+v", invalid)
	}

	empty := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.preview",
		Arguments: json.RawMessage(`{"name":"alb","arguments":{}}`),
	})
	if !empty.OK || !strings.Contains(string(empty.Content), `%alb%1%{}%`) {
		t.Fatalf("empty captured preview failed: ok=%v error=%+v content=%s", empty.OK, empty.Error, empty.Content)
	}
}

func TestCapturedOpcodeDefinitionsBuildTheirOutboundExamples(t *testing.T) {
	specs, err := newCommandSpecs()
	if err != nil {
		t.Fatal(err)
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate toolkit test source")
	}
	exampleDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "Logs", "SendCommandsJSON")
	rawCaptureOnlyExamples := map[string]json.RawMessage{
		"acm":  json.RawMessage(`{"M":"test message"}`),
		"ama":  json.RawMessage(`{"AID":190489}`),
		"aqs":  json.RawMessage(`{"EID":137}`),
		"bse":  json.RawMessage(`{}`),
		"bss":  json.RawMessage(`{"ID":262}`),
		"crca": json.RawMessage(`{"KID":0,"AID":15246649,"OID":4105,"S":0,"ST":"queue"}`),
		"csp":  json.RawMessage(`{}`),
		"ctr":  json.RawMessage(`{"TRQ":0}`),
		"eup":  json.RawMessage(`{"OID":1841,"PWR":0,"PO":-1}`),
		"gcc":  json.RawMessage(`{}`),
		"grc":  json.RawMessage(`{"AID":15252150,"KID":2}`),
		"mcu":  json.RawMessage(`{"LID":0,"S":2,"ST":"queue"}`),
		"msb":  json.RawMessage(`{"OID":1841,"MST":"MS7"}`),
		"qsc":  json.RawMessage(`{"QID":3490}`),
		"sii":  json.RawMessage(`{}`),
		"usg":  json.RawMessage(`{}`),
		"vln":  json.RawMessage(`{"NOM":"CapturedName"}`),
	}
	if len(rawCaptureOnlyExamples) != 17 {
		t.Fatalf("raw-capture-only examples=%d, want 17", len(rawCaptureOnlyExamples))
	}
	definitions := capturedOpcodeDefinitions()
	if len(definitions) != 70 {
		t.Fatalf("captured opcode definitions=%d, want 70", len(definitions))
	}
	for _, definition := range definitions {
		definition := definition
		t.Run(definition.Name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(exampleDir, definition.Opcode+".json"))
			if err != nil {
				var captured bool
				raw, captured = rawCaptureOnlyExamples[definition.Opcode]
				if !captured {
					t.Fatal(err)
				}
			}
			spec, ok := specs[definition.Name]
			if !ok {
				t.Fatalf("missing command spec %q", definition.Name)
			}
			payloads, err := spec.build(raw)
			if err != nil {
				t.Fatalf("example does not satisfy mapped schema: %v", err)
			}
			if len(payloads) != 1 || !strings.Contains(payloads[0], "%"+definition.Opcode+"%1%") {
				t.Fatalf("unexpected payloads: %q", payloads)
			}
		})
	}
}

func TestCommandSendUsesSelectedCommandEffect(t *testing.T) {
	harness, err := NewDefaultHarness(WithAllowedEffects(EffectRead, EffectGameAction))
	if err != nil {
		t.Fatal(err)
	}
	result := harness.Execute(context.Background(), Call{
		Name: "citadel.command.send",
		Arguments: json.RawMessage(`{
			"name":"sbp",
			"arguments":{
				"productId":1,"buildType":0,"typeId":116,"amount":1,
				"kingdomId":0,"castleId":-1,"priceCode2":-1,
				"buildAux":0,"power":0,"publicOrder":-1
			}
		}`),
	})
	if result.OK || result.Error == nil || result.Error.Code != "unauthorized" {
		t.Fatalf("destructive command result=%+v", result)
	}
	if result.Effect != EffectDestructive {
		t.Fatalf("resolved effect=%q, want %q", result.Effect, EffectDestructive)
	}
}

func TestExternalCommunicationRequiresSeparateAuthorization(t *testing.T) {
	harness, err := NewDefaultHarness(WithAllowedEffects(EffectRead, EffectDestructive))
	if err != nil {
		t.Fatal(err)
	}
	result := harness.Execute(context.Background(), Call{
		Name:      "citadel.command.send",
		Arguments: json.RawMessage(`{"name":"acm","arguments":{"M":"test"}}`),
	})
	if result.OK || result.Error == nil || result.Error.Code != "unauthorized" {
		t.Fatalf("external command result=%+v", result)
	}
	if result.Effect != EffectExternal {
		t.Fatalf("resolved effect=%q, want %q", result.Effect, EffectExternal)
	}
}

func jsonNumber(value uint64) string {
	data, _ := json.Marshal(value)
	return string(data)
}
