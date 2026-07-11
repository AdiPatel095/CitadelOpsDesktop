package ProtocolTestHarness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHarnessVerifiesSemanticExactAndParserContracts(t *testing.T) {
	corpus := testCorpus(t, map[string]string{
		"outbound/example.json":  `{"B":2,"A":1}`,
		"outbound/bodyless.json": `{}`,
		"inbound/example.json":   `{"items":[3,4]}`,
	})
	harness := NewWithCorpus(corpus)
	mustRegisterBuilder(t, harness, "semantic", func(_ context.Context, _ BuildInput) (string, error) {
		return `%xt%EmpireEx_21%example%1%{"A":1,"B":2}%`, nil
	})
	mustRegisterBuilder(t, harness, "exact", func(_ context.Context, _ BuildInput) (string, error) {
		return `%xt%EmpireEx_21%example%1%{"B":2,"A":1}%`, nil
	})
	mustRegisterBuilder(t, harness, "bodyless", func(_ context.Context, _ BuildInput) (string, error) {
		return `%xt%EmpireEx_21%bodyless%1%`, nil
	})
	mustRegisterBuilder(t, harness, "live", func(_ context.Context, input BuildInput) (string, error) {
		if input.Token != "EmpireEx_49" || input.Opcode != "live" {
			return "", fmt.Errorf("unexpected build envelope: %+v", input)
		}
		return `%xt%EmpireEx_49%live%1%{"ID":7}%`, nil
	})
	mustRegisterParser(t, harness, "count", func(_ context.Context, raw json.RawMessage) (interface{}, error) {
		var input struct {
			Items []int `json:"items"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return nil, err
		}
		return map[string]int{"count": len(input.Items)}, nil
	})

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Commands: []CommandContract{
			{Name: "semantic", Generation: "test/v1", Adapter: "semantic", Fixture: "example", Match: MatchSemanticJSON},
			{
				Name:          "exact",
				Generation:    "test/v1",
				Adapter:       "exact",
				Fixture:       "example",
				Match:         MatchExactBytes,
				ExpectedFrame: `%xt%EmpireEx_21%example%1%{"B":2,"A":1}%`,
			},
			{Name: "bodyless", Generation: "test/v1", Adapter: "bodyless", Fixture: "bodyless", Match: MatchNoBody},
		},
		LiveCommands: []LiveCommandContract{
			{
				Name:               "live",
				Generation:         "test/live",
				Adapter:            "live",
				CapturedAt:         "2026-07-11T12:00:00-04:00",
				Source:             "Logs/channels/app_send.log",
				SourceRequestLine:  10,
				SourceResponseLine: 11,
				RequestFrame:       `%xt%EmpireEx_49%live%1%{"ID":7}%`,
				Response: LiveResponseEvidence{
					Opcode:      "live",
					Status:      0,
					DelayMS:     12,
					FrameBytes:  24,
					FrameSHA256: strings.Repeat("0", 64),
				},
			},
		},
		Parsers: []ParserContract{
			{Name: "parser", Generation: "test/v1", Adapter: "count", Fixture: "example", Expected: json.RawMessage(`{"count":2}`)},
		},
	}
	results, err := harness.Verify(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 5 {
		t.Fatalf("results=%d, want 5", len(results))
	}
	for _, result := range results {
		if !result.Passed() {
			t.Error(result.String())
		}
	}
}

func TestHarnessReportsMissingAdapterWithoutStoppingLaterCases(t *testing.T) {
	corpus := testCorpus(t, map[string]string{
		"outbound/example.json": `{}`,
	})
	harness := NewWithCorpus(corpus)
	mustRegisterBuilder(t, harness, "present", func(_ context.Context, _ BuildInput) (string, error) {
		return `%xt%EmpireEx_21%example%1%{}%`, nil
	})
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Commands: []CommandContract{
			{Name: "missing", Generation: "test/v1", Adapter: "missing", Fixture: "example", Match: MatchSemanticJSON},
			{Name: "present", Generation: "test/v1", Adapter: "present", Fixture: "example", Match: MatchSemanticJSON},
		},
	}
	results, err := harness.Verify(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Passed() || !results[1].Passed() {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestCorpusRejectsCorruptJSONExample(t *testing.T) {
	corpus := testCorpus(t, map[string]string{
		"outbound/corrupt.json": "\x00\x00",
	})
	_, err := corpus.Load(DirectionOutbound, "corrupt")
	if err == nil || !strings.Contains(err.Error(), "invalid outbound fixture") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseFrameAllowsPercentInsideJSONStrings(t *testing.T) {
	frame, err := ParseFrame(`%xt%EmpireEx_21%acm%1%{"M":"100% ready"}%`)
	if err != nil {
		t.Fatal(err)
	}
	if !frame.HasBody || string(frame.Body) != `{"M":"100% ready"}` {
		t.Fatalf("frame=%+v", frame)
	}
}

func TestManifestRejectsUnknownSchema(t *testing.T) {
	err := (Manifest{SchemaVersion: SchemaVersion + 1}).Validate()
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol contract schema") {
		t.Fatalf("error=%v", err)
	}
}

func testCorpus(t *testing.T, files map[string]string) Corpus {
	t.Helper()
	root := t.TempDir()
	inbound := filepath.Join(root, "inbound")
	outbound := filepath.Join(root, "outbound")
	for _, directory := range []string{inbound, outbound} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Corpus{InboundDir: inbound, OutboundDir: outbound}
}

func mustRegisterBuilder(t *testing.T, harness *Harness, name string, builder Builder) {
	t.Helper()
	if err := harness.RegisterBuilder(name, builder); err != nil {
		t.Fatal(err)
	}
}

func mustRegisterParser(t *testing.T, harness *Harness, name string, parser Parser) {
	t.Helper()
	if err := harness.RegisterParser(name, parser); err != nil {
		t.Fatal(err)
	}
}
