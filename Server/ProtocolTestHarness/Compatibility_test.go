package ProtocolTestHarness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	protocoltest "CitadelDesktop/Server/ProtocolTestHarness"
	"CitadelDesktop/Server/ResponseRegistry"
)

func TestCapturedProtocolCompatibility(t *testing.T) {
	repositoryRoot, packageDir := compatibilityPaths(t)
	manifest, err := protocoltest.LoadManifest(filepath.Join(packageDir, "TestData", "Contracts.json"))
	if err != nil {
		t.Fatal(err)
	}
	harness := protocoltest.New(repositoryRoot)
	registerCommandAdapters(t, harness)
	registerParserAdapters(t, harness)

	results, err := harness.Verify(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	contractCount := len(manifest.Commands) + len(manifest.LiveCommands) + len(manifest.Parsers)
	if len(results) != contractCount {
		t.Fatalf("results=%d, contracts=%d", len(results), contractCount)
	}
	for _, result := range results {
		result := result
		t.Run(string(result.Kind)+"/"+result.Name, func(t *testing.T) {
			if !result.Passed() {
				t.Fatal(result.String())
			}
		})
	}
}

func TestOutboundCorpusJSONHealth(t *testing.T) {
	repositoryRoot, _ := compatibilityPaths(t)
	corpus := protocoltest.NewCorpus(repositoryRoot)
	names, err := corpus.Names(protocoltest.DirectionOutbound)
	if err != nil {
		t.Fatal(err)
	}
	knownCorrupt := map[string]bool{
		"cds": true,
		"lli": true,
	}
	seenCorrupt := make(map[string]bool, len(knownCorrupt))
	for _, name := range names {
		_, err := corpus.Load(protocoltest.DirectionOutbound, name)
		if knownCorrupt[name] {
			seenCorrupt[name] = true
			if err == nil {
				t.Errorf("fixture %q is now valid; remove it from the corruption quarantine", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("fixture %q: %v", name, err)
		}
	}
	for name := range knownCorrupt {
		if !seenCorrupt[name] {
			t.Errorf("quarantined fixture %q no longer exists; remove it from the corruption quarantine", name)
		}
	}
}

type rawCommandBuilder func(context.Context, json.RawMessage) (string, error)

func registerCommandAdapters(t *testing.T, harness *protocoltest.Harness) {
	t.Helper()
	adapters := map[string]rawCommandBuilder{
		"eeq": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				EquipmentID float64 `json:"EID"`
				LeaderID    float64 `json:"LID"`
				Equip       int     `json:"E"`
			}](raw)
			if err != nil {
				return "", err
			}
			if body.Equip != 0 && body.Equip != 1 {
				return "", fmt.Errorf("E must be 0 or 1")
			}
			return GameCommands.EEQPayload(body.EquipmentID, body.LeaderID, body.Equip == 1), nil
		},
		"jaa": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				X       int `json:"PX"`
				Y       int `json:"PY"`
				Kingdom int `json:"KID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.JAAPayload(body.X, body.Y, body.Kingdom), nil
		},
		"jca": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				Castle  int `json:"CID"`
				Kingdom int `json:"KID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.JCAPayload(body.Castle, body.Kingdom), nil
		},
		"sdi": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				TargetX int `json:"TX"`
				TargetY int `json:"TY"`
				SourceX int `json:"SX"`
				SourceY int `json:"SY"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.SDIPayload(body.TargetX, body.TargetY, body.SourceX, body.SourceY), nil
		},
		"gam": func(_ context.Context, raw json.RawMessage) (string, error) {
			if _, err := decodeBody[struct{}](raw); err != nil {
				return "", err
			}
			return GameCommands.GAMPayload(), nil
		},
		"ebu": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				WID  int `json:"WID"`
				X    int `json:"X"`
				Y    int `json:"Y"`
				R    int `json:"R"`
				PWR  int `json:"PWR"`
				PO   int `json:"PO"`
				DOID int `json:"DOID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.EBUWithParamsPayload(body.WID, body.X, body.Y, body.R, body.PWR, body.PO, body.DOID), nil
		},
		"bup": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				LineID   int `json:"LID"`
				WID      int `json:"WID"`
				Amount   int `json:"AMT"`
				PO       int `json:"PO"`
				PWR      int `json:"PWR"`
				SK       int `json:"SK"`
				SID      int `json:"SID"`
				CastleID int `json:"AID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.BUPPayload(body.LineID, body.WID, body.Amount, body.PO, body.PWR, body.SK, body.SID, body.CastleID), nil
		},
		"boi": func(_ context.Context, raw json.RawMessage) (string, error) {
			if _, err := decodeBody[struct{}](raw); err != nil {
				return "", err
			}
			return GameCommands.BOIPayload(), nil
		},
		"bsd": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				MessageID int64 `json:"MID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.BSDPayload(body.MessageID), nil
		},
		"cmi": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				Scope   int `json:"S"`
				Kingdom int `json:"KID"`
			}](raw)
			if err != nil {
				return "", err
			}
			if body.Scope != 1 || body.Kingdom != -1 {
				return "", fmt.Errorf("fixture fixed fields do not match CMIPayload")
			}
			return GameCommands.CMIPayload(), nil
		},
		"crin": func(_ context.Context, raw json.RawMessage) (string, error) {
			if _, err := decodeBody[struct{}](raw); err != nil {
				return "", err
			}
			return GameCommands.CRINPayload(), nil
		},
		"rpc": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				OID      int `json:"OID"`
				CID      int `json:"CID"`
				SlotID   int `json:"SID"`
				Mode     int `json:"M"`
				Kingdom  int `json:"KID"`
				CastleID int `json:"AID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.RPCPayload(body.OID, body.CID, body.SlotID, body.Mode, body.Kingdom, body.CastleID), nil
		},
		"gbc": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				CastleID int `json:"CID"`
				Kingdom  int `json:"KID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.GBCPayload(body.CastleID, body.Kingdom), nil
		},
		"sin": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[string](raw)
			if err != nil {
				return "", err
			}
			if body != "" {
				return "", fmt.Errorf("bodyless sin fixture must be an empty string")
			}
			return GameCommands.SINPayload(), nil
		},
		"gaa": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				Kingdom int `json:"KID"`
				AX1     int `json:"AX1"`
				AY1     int `json:"AY1"`
				AX2     int `json:"AX2"`
				AY2     int `json:"AY2"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.GAAPayload(body.Kingdom, body.AX1, body.AY1, body.AX2, body.AY2), nil
		},
		"hru": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				Amount int `json:"A"`
				Unit   int `json:"U"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.HRUPayload(body.Unit, body.Amount), nil
		},
		"hdu": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				Amount int `json:"A"`
				Unit   int `json:"U"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.HDUPayload(body.Unit, body.Amount), nil
		},
		"ain": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				AllianceID int `json:"AID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.AINPayload(body.AllianceID), nil
		},
		"csm": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				SourceID int `json:"SID"`
				TargetX  int `json:"TX"`
				TargetY  int `json:"TY"`
				SpyCount int `json:"SC"`
				SpyType  int `json:"ST"`
				Percent  int `json:"SE"`
				HBW      int `json:"HBW"`
				Kingdom  int `json:"KID"`
				PTT      int `json:"PTT"`
				Delay    int `json:"SD"`
			}](raw)
			if err != nil {
				return "", err
			}
			if body.SpyType != 0 || body.Percent != 100 || body.HBW != -1 || body.Kingdom != 0 || body.PTT != 1 || body.Delay != 0 {
				return "", fmt.Errorf("fixture fixed fields do not match CSMPayload")
			}
			return GameCommands.CSMPayload(body.SourceID, body.TargetX, body.TargetY, body.SpyCount), nil
		},
		"gdi": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				PlayerID int `json:"PID"`
			}](raw)
			if err != nil {
				return "", err
			}
			return GameCommands.GDIPayload(body.PlayerID), nil
		},
		"dcl": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				CastleDetail int `json:"CD"`
			}](raw)
			if err != nil {
				return "", err
			}
			if body.CastleDetail != 0 {
				return "", fmt.Errorf("fixture fixed fields do not match DCLRefreshPayload")
			}
			return GameCommands.DCLRefreshPayload(), nil
		},
		"gei": func(_ context.Context, raw json.RawMessage) (string, error) {
			if _, err := decodeBody[struct{}](raw); err != nil {
				return "", err
			}
			return GameCommands.GEIPayload(), nil
		},
		"ggm": func(_ context.Context, raw json.RawMessage) (string, error) {
			if _, err := decodeBody[struct{}](raw); err != nil {
				return "", err
			}
			return GameCommands.GGMPayload(), nil
		},
		"kpi": func(_ context.Context, raw json.RawMessage) (string, error) {
			if _, err := decodeBody[struct{}](raw); err != nil {
				return "", err
			}
			return GameCommands.KPIPayload(), nil
		},
		"seq": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				EquipmentID float64 `json:"EID"`
				LeaderID    int     `json:"LID"`
				Exchange    int     `json:"EX"`
				LoadoutID   int     `json:"LFID"`
			}](raw)
			if err != nil {
				return "", err
			}
			if body.LeaderID != -1 || body.Exchange != 0 || body.LoadoutID != -1 {
				return "", fmt.Errorf("fixture fixed fields do not match SEQPayload")
			}
			return GameCommands.SEQPayload(body.EquipmentID), nil
		},
		"sge": func(_ context.Context, raw json.RawMessage) (string, error) {
			body, err := decodeBody[struct {
				GemID     float64 `json:"GID"`
				RelicGem  int     `json:"RGEM"`
				LoadoutID int     `json:"LFID"`
			}](raw)
			if err != nil {
				return "", err
			}
			if body.LoadoutID != -1 {
				return "", fmt.Errorf("fixture fixed fields do not match SGE payloads")
			}
			switch body.RelicGem {
			case 0:
				return GameCommands.SGENonRelicGemPayload(body.GemID), nil
			case 1:
				return GameCommands.SGERelicGemPayload(body.GemID), nil
			default:
				return "", fmt.Errorf("RGEM must be 0 or 1")
			}
		},
	}
	for name, adapter := range adapters {
		builder := func(ctx context.Context, input protocoltest.BuildInput) (string, error) {
			return adapter(ctx, input.Body)
		}
		if err := harness.RegisterBuilder(name, admittedByCommandHarness(name, builder)); err != nil {
			t.Fatal(err)
		}
	}
}

var protocolTokenMu sync.Mutex

func admittedByCommandHarness(name string, builder protocoltest.Builder) protocoltest.Builder {
	return func(ctx context.Context, input protocoltest.BuildInput) (string, error) {
		if input.Opcode != name {
			return "", fmt.Errorf("adapter %q received opcode %q", name, input.Opcode)
		}
		var payload string
		var err error
		func() {
			protocolTokenMu.Lock()
			defer protocolTokenMu.Unlock()
			previousToken := ResponseRegistry.EmpireExToken
			ResponseRegistry.EmpireExToken = input.Token
			defer func() { ResponseRegistry.EmpireExToken = previousToken }()
			payload, err = builder(ctx, input)
		}()
		if err != nil {
			return "", err
		}
		broker := Automation.NewCommandBroker()
		receipt := Automation.NewCommandHarness(broker).Dispatch(ctx, Automation.CommandSubmission{
			ContractVersion: Automation.CommandContractVersion,
			Command:         name,
			Intent:          "protocol_compatibility",
			Frames: []Automation.CommandFrame{
				{Payload: []byte(payload)},
			},
		})
		if !receipt.Accepted {
			return "", fmt.Errorf("command harness rejected frame: %s: %s", receipt.Code, receipt.Message)
		}
		if len(receipt.Frames) != 1 || receipt.Frames[0].Opcode != name {
			return "", fmt.Errorf("command harness receipt does not preserve opcode %q: %+v", name, receipt.Frames)
		}
		return payload, nil
	}
}

type countSummary struct {
	UniqueIDs   int         `json:"uniqueIds"`
	TotalAmount int         `json:"totalAmount"`
	Selected    map[int]int `json:"selected"`
}

type constructionSlotSample struct {
	OID          int `json:"oid"`
	CID          int `json:"cid"`
	Slot         int `json:"slot"`
	RemainingSec int `json:"remainingSec"`
}

type constructionSummary struct {
	Buildings  int                     `json:"buildings"`
	Slots      int                     `json:"slots"`
	TimedSlots int                     `json:"timedSlots"`
	FirstOID   int                     `json:"firstOid"`
	FirstCID   int                     `json:"firstCid"`
	FirstTimed *constructionSlotSample `json:"firstTimed"`
	LastTimed  *constructionSlotSample `json:"lastTimed"`
}

type productSample struct {
	PID    int `json:"pid"`
	Amount int `json:"amount"`
}

type productSummary struct {
	Products    int           `json:"products"`
	TotalAmount int           `json:"totalAmount"`
	First       productSample `json:"first"`
	Last        productSample `json:"last"`
}

func registerParserAdapters(t *testing.T, harness *protocoltest.Harness) {
	t.Helper()
	adapters := map[string]protocoltest.Parser{
		"sin_summary": func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			counts, err := GameParser.ParseDecorationStorageCountsFromSINJSON(string(raw))
			if err != nil {
				return nil, err
			}
			return summarizeCounts(counts, 2944, 3200), nil
		},
		"construction_summary": func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			buildings := GameParser.ParseGCAConstructionFromGameJSON(string(raw))
			if len(buildings) == 0 {
				return nil, fmt.Errorf("no construction buildings parsed")
			}
			summary := constructionSummary{Buildings: len(buildings), FirstOID: buildings[0].OID}
			if len(buildings[0].Slots) > 0 {
				summary.FirstCID = buildings[0].Slots[0].CID
			}
			for _, building := range buildings {
				for _, slot := range building.Slots {
					summary.Slots++
					if slot.RemainingSec == nil {
						continue
					}
					sample := &constructionSlotSample{
						OID:          building.OID,
						CID:          slot.CID,
						Slot:         slot.S,
						RemainingSec: *slot.RemainingSec,
					}
					if summary.FirstTimed == nil {
						first := *sample
						summary.FirstTimed = &first
					}
					summary.LastTimed = sample
					summary.TimedSlots++
				}
			}
			return summary, nil
		},
		"construction_inventory_summary": func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			counts, ok := GameParser.ParseConstructionInventoryPairsFromRootJSON(string(raw))
			if !ok {
				return nil, fmt.Errorf("construction inventory was not parsed")
			}
			return summarizeCounts(counts, 537, 30130, 601), nil
		},
		"gbc_summary": func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			rows, err := GameParser.ParseGbcTrivialCIPLFromJSON(string(raw))
			if err != nil {
				return nil, err
			}
			if len(rows) == 0 {
				return nil, fmt.Errorf("no shop products parsed")
			}
			summary := productSummary{
				Products: len(rows),
				First:    productSample{PID: rows[0].PID, Amount: rows[0].AMT},
				Last:     productSample{PID: rows[len(rows)-1].PID, Amount: rows[len(rows)-1].AMT},
			}
			for _, row := range rows {
				summary.TotalAmount += row.AMT
			}
			return summary, nil
		},
	}
	for name, adapter := range adapters {
		if err := harness.RegisterParser(name, adapter); err != nil {
			t.Fatal(err)
		}
	}
}

func summarizeCounts(counts map[int]int, selected ...int) countSummary {
	summary := countSummary{
		UniqueIDs: len(counts),
		Selected:  make(map[int]int, len(selected)),
	}
	for _, amount := range counts {
		summary.TotalAmount += amount
	}
	for _, id := range selected {
		summary.Selected[id] = counts[id]
	}
	return summary
}

func decodeBody[T any](raw json.RawMessage) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, fmt.Errorf("unexpected JSON after body")
		}
		return value, err
	}
	return value, nil
}

func compatibilityPaths(t *testing.T) (string, string) {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate compatibility test source")
	}
	packageDir := filepath.Dir(sourceFile)
	return filepath.Clean(filepath.Join(packageDir, "..", "..")), packageDir
}
