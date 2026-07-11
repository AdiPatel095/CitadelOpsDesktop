// Package ProtocolTestHarness verifies captured game-protocol examples against
// parser and command-builder implementations without opening a websocket.
package ProtocolTestHarness

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SchemaVersion = 1

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type MatchMode string

const (
	MatchSemanticJSON MatchMode = "semantic_json"
	MatchExactBytes   MatchMode = "exact_bytes"
	MatchNoBody       MatchMode = "no_body"
)

type CaseKind string

const (
	CaseCommand     CaseKind = "command"
	CaseLiveCommand CaseKind = "live_command"
	CaseParser      CaseKind = "parser"
)

// Example is one validated JSON body from the repository capture corpus.
type Example struct {
	Direction Direction
	Name      string
	Path      string
	Body      json.RawMessage
}

// Corpus resolves the historical JSONExamples corpus after its inbound and
// outbound examples were split into separate directories.
type Corpus struct {
	InboundDir  string
	OutboundDir string
}

func NewCorpus(repositoryRoot string) Corpus {
	return Corpus{
		InboundDir:  filepath.Join(repositoryRoot, "Logs", "RecvCommandsJSON"),
		OutboundDir: filepath.Join(repositoryRoot, "Logs", "SendCommandsJSON"),
	}
}

func (c Corpus) Load(direction Direction, name string) (Example, error) {
	base, err := fixtureBaseName(name)
	if err != nil {
		return Example{}, err
	}
	directory, err := c.directory(direction)
	if err != nil {
		return Example{}, err
	}
	path := filepath.Join(directory, base+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Example{}, fmt.Errorf("read %s fixture %q: %w", direction, base, err)
	}
	raw = bytes.TrimSpace(raw)
	if _, err := canonicalJSON(raw); err != nil {
		return Example{}, fmt.Errorf("invalid %s fixture %q (%s): %w", direction, base, path, err)
	}
	return Example{
		Direction: direction,
		Name:      base,
		Path:      path,
		Body:      append(json.RawMessage(nil), raw...),
	}, nil
}

// Names lists available fixture basenames without loading large capture bodies.
func (c Corpus) Names(direction Direction) ([]string, error) {
	directory, err := c.directory(direction)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read %s fixture directory: %w", direction, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, err := fixtureBaseName(base); err == nil {
			names = append(names, base)
		}
	}
	sort.Strings(names)
	return names, nil
}

func (c Corpus) directory(direction Direction) (string, error) {
	switch direction {
	case DirectionInbound:
		return c.InboundDir, nil
	case DirectionOutbound:
		return c.OutboundDir, nil
	default:
		return "", fmt.Errorf("unknown fixture direction %q", direction)
	}
}

func fixtureBaseName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if strings.EqualFold(filepath.Ext(name), ".json") {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	if name == "" {
		return "", fmt.Errorf("fixture name is empty")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "", fmt.Errorf("invalid fixture name %q", name)
	}
	return name, nil
}

// Frame is the standard %xt%EmpireEx_N%opcode%sequence%body% envelope.
type Frame struct {
	Token    string
	Opcode   string
	Sequence int
	Body     json.RawMessage
	HasBody  bool
}

func ParseFrame(payload string) (Frame, error) {
	const prefix = "%xt%"
	if !strings.HasPrefix(payload, prefix) {
		return Frame{}, fmt.Errorf("frame is missing %q prefix", prefix)
	}
	rest := strings.TrimPrefix(payload, prefix)
	token, rest, ok := strings.Cut(rest, "%")
	if !ok || token == "" {
		return Frame{}, fmt.Errorf("frame is missing protocol token")
	}
	opcode, rest, ok := strings.Cut(rest, "%")
	if !ok || opcode == "" {
		return Frame{}, fmt.Errorf("frame is missing opcode")
	}
	sequenceText, tail, ok := strings.Cut(rest, "%")
	if !ok {
		return Frame{}, fmt.Errorf("frame is missing sequence terminator")
	}
	sequence, err := strconv.Atoi(sequenceText)
	if err != nil {
		return Frame{}, fmt.Errorf("invalid frame sequence %q", sequenceText)
	}
	frame := Frame{Token: token, Opcode: opcode, Sequence: sequence}
	if tail == "" {
		return frame, nil
	}
	if !strings.HasSuffix(tail, "%") {
		return Frame{}, fmt.Errorf("frame body is missing trailing delimiter")
	}
	body := strings.TrimSuffix(tail, "%")
	if _, err := canonicalJSON([]byte(body)); err != nil {
		return Frame{}, fmt.Errorf("invalid frame JSON body: %w", err)
	}
	frame.HasBody = true
	frame.Body = json.RawMessage(body)
	return frame, nil
}

// Manifest is the durable compatibility ledger. Implementations are bound by
// adapter name in Go, while historical expectations stay data-driven.
type Manifest struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Commands      []CommandContract     `json:"commands"`
	LiveCommands  []LiveCommandContract `json:"liveCommands,omitempty"`
	Parsers       []ParserContract      `json:"parsers"`
}

type CommandContract struct {
	Name             string    `json:"name"`
	Generation       string    `json:"generation"`
	Adapter          string    `json:"adapter"`
	Fixture          string    `json:"fixture"`
	Match            MatchMode `json:"match"`
	ExpectedToken    string    `json:"expectedToken,omitempty"`
	ExpectedOpcode   string    `json:"expectedOpcode,omitempty"`
	ExpectedSequence int       `json:"expectedSequence,omitempty"`
	ExpectedFrame    string    `json:"expectedFrame,omitempty"`
}

type ParserContract struct {
	Name       string          `json:"name"`
	Generation string          `json:"generation"`
	Adapter    string          `json:"adapter"`
	Fixture    string          `json:"fixture"`
	Expected   json.RawMessage `json:"expected"`
}

// LiveCommandContract is a curated successful request/response observation
// extracted from app_send.log. Only the outbound frame is retained; response
// contents are represented by non-sensitive evidence.
type LiveCommandContract struct {
	Name               string               `json:"name"`
	Generation         string               `json:"generation"`
	Adapter            string               `json:"adapter"`
	CapturedAt         string               `json:"capturedAt"`
	Source             string               `json:"source"`
	SourceRequestLine  int                  `json:"sourceRequestLine"`
	SourceResponseLine int                  `json:"sourceResponseLine"`
	RequestFrame       string               `json:"requestFrame"`
	Response           LiveResponseEvidence `json:"response"`
}

type LiveResponseEvidence struct {
	Opcode      string `json:"opcode"`
	Status      int    `json:"status"`
	DelayMS     int64  `json:"delayMs"`
	FrameBytes  int    `json:"frameBytes"`
	FrameSHA256 string `json:"frameSha256"`
}

func LoadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read protocol contract manifest: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode protocol contract manifest: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Manifest{}, fmt.Errorf("decode protocol contract manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported protocol contract schema %d (want %d)", m.SchemaVersion, SchemaVersion)
	}
	seen := make(map[string]CaseKind, len(m.Commands)+len(m.LiveCommands)+len(m.Parsers))
	for i, contract := range m.Commands {
		if err := validateCaseIdentity(CaseCommand, contract.Name, contract.Generation, contract.Adapter, contract.Fixture, seen); err != nil {
			return fmt.Errorf("commands[%d]: %w", i, err)
		}
		mode := contract.Match
		if mode == "" {
			mode = MatchSemanticJSON
		}
		switch mode {
		case MatchSemanticJSON, MatchNoBody:
		case MatchExactBytes:
			if contract.ExpectedFrame == "" {
				return fmt.Errorf("commands[%d] %q: exact_bytes requires expectedFrame", i, contract.Name)
			}
		default:
			return fmt.Errorf("commands[%d] %q: unknown match mode %q", i, contract.Name, mode)
		}
	}
	for i, contract := range m.LiveCommands {
		if err := validateLiveCommandContract(contract, seen); err != nil {
			return fmt.Errorf("liveCommands[%d]: %w", i, err)
		}
	}
	for i, contract := range m.Parsers {
		if err := validateCaseIdentity(CaseParser, contract.Name, contract.Generation, contract.Adapter, contract.Fixture, seen); err != nil {
			return fmt.Errorf("parsers[%d]: %w", i, err)
		}
		if len(bytes.TrimSpace(contract.Expected)) == 0 {
			return fmt.Errorf("parsers[%d] %q: expected snapshot is required", i, contract.Name)
		}
		if _, err := canonicalJSON(contract.Expected); err != nil {
			return fmt.Errorf("parsers[%d] %q: invalid expected snapshot: %w", i, contract.Name, err)
		}
	}
	return nil
}

func validateLiveCommandContract(contract LiveCommandContract, seen map[string]CaseKind) error {
	name := strings.TrimSpace(contract.Name)
	if name == "" {
		return fmt.Errorf("case name is required")
	}
	if previous, ok := seen[name]; ok {
		return fmt.Errorf("duplicate case name %q (already used by %s)", name, previous)
	}
	seen[name] = CaseLiveCommand
	if strings.TrimSpace(contract.Generation) == "" {
		return fmt.Errorf("case %q: generation is required", name)
	}
	if strings.TrimSpace(contract.Adapter) == "" {
		return fmt.Errorf("case %q: adapter is required", name)
	}
	if _, err := time.Parse(time.RFC3339Nano, contract.CapturedAt); err != nil {
		return fmt.Errorf("case %q: capturedAt must be RFC3339: %w", name, err)
	}
	source := filepath.Clean(strings.TrimSpace(contract.Source))
	if source == "." || filepath.IsAbs(source) || source == ".." || strings.HasPrefix(source, ".."+string(filepath.Separator)) {
		return fmt.Errorf("case %q: source must be a repository-relative path", name)
	}
	if contract.SourceRequestLine <= 0 || contract.SourceResponseLine <= contract.SourceRequestLine {
		return fmt.Errorf("case %q: source line numbers are invalid", name)
	}
	frame, err := ParseFrame(contract.RequestFrame)
	if err != nil {
		return fmt.Errorf("case %q: invalid requestFrame: %w", name, err)
	}
	if !frame.HasBody {
		return fmt.Errorf("case %q: live requestFrame must contain a JSON body", name)
	}
	responseOpcode := strings.TrimSpace(contract.Response.Opcode)
	if responseOpcode == "" || responseOpcode != frame.Opcode {
		return fmt.Errorf("case %q: response opcode %q does not match request opcode %q", name, responseOpcode, frame.Opcode)
	}
	if contract.Response.Status != 0 {
		return fmt.Errorf("case %q: only successful response status 0 may be curated", name)
	}
	if contract.Response.DelayMS < 0 || contract.Response.FrameBytes <= 0 {
		return fmt.Errorf("case %q: response timing/size evidence is invalid", name)
	}
	digest, err := hex.DecodeString(contract.Response.FrameSHA256)
	if err != nil || len(digest) != 32 {
		return fmt.Errorf("case %q: response frameSha256 must be a 64-character SHA-256 digest", name)
	}
	return nil
}

func validateCaseIdentity(
	kind CaseKind,
	name string,
	generation string,
	adapter string,
	fixture string,
	seen map[string]CaseKind,
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("case name is required")
	}
	if previous, ok := seen[name]; ok {
		return fmt.Errorf("duplicate case name %q (already used by %s)", name, previous)
	}
	seen[name] = kind
	if strings.TrimSpace(generation) == "" {
		return fmt.Errorf("case %q: generation is required", name)
	}
	if strings.TrimSpace(adapter) == "" {
		return fmt.Errorf("case %q: adapter is required", name)
	}
	if _, err := fixtureBaseName(fixture); err != nil {
		return fmt.Errorf("case %q: %w", name, err)
	}
	return nil
}

type BuildInput struct {
	Body   json.RawMessage
	Token  string
	Opcode string
}

type Builder func(context.Context, BuildInput) (string, error)

type Parser func(context.Context, json.RawMessage) (interface{}, error)

type Harness struct {
	corpus   Corpus
	builders map[string]Builder
	parsers  map[string]Parser
}

func New(repositoryRoot string) *Harness {
	return NewWithCorpus(NewCorpus(repositoryRoot))
}

func NewWithCorpus(corpus Corpus) *Harness {
	return &Harness{
		corpus:   corpus,
		builders: make(map[string]Builder),
		parsers:  make(map[string]Parser),
	}
}

func (h *Harness) RegisterBuilder(name string, builder Builder) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("builder adapter name is required")
	}
	if builder == nil {
		return fmt.Errorf("builder adapter %q is nil", name)
	}
	if _, exists := h.builders[name]; exists {
		return fmt.Errorf("builder adapter %q is already registered", name)
	}
	h.builders[name] = builder
	return nil
}

func (h *Harness) RegisterParser(name string, parser Parser) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("parser adapter name is required")
	}
	if parser == nil {
		return fmt.Errorf("parser adapter %q is nil", name)
	}
	if _, exists := h.parsers[name]; exists {
		return fmt.Errorf("parser adapter %q is already registered", name)
	}
	h.parsers[name] = parser
	return nil
}

type CaseResult struct {
	Kind       CaseKind
	Name       string
	Generation string
	Adapter    string
	Fixture    string
	Err        error
}

func (r CaseResult) Passed() bool {
	return r.Err == nil
}

func (r CaseResult) String() string {
	identity := fmt.Sprintf("%s %q [%s] adapter=%q fixture=%q", r.Kind, r.Name, r.Generation, r.Adapter, r.Fixture)
	if r.Err == nil {
		return identity + ": passed"
	}
	return identity + ": " + r.Err.Error()
}

// Verify executes every contract and returns results in manifest order. A
// missing adapter is a case failure so one incomplete registration cannot hide
// failures in the remainder of the corpus.
func (h *Harness) Verify(ctx context.Context, manifest Manifest) ([]CaseResult, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	results := make([]CaseResult, 0, len(manifest.Commands)+len(manifest.LiveCommands)+len(manifest.Parsers))
	for _, contract := range manifest.Commands {
		result := CaseResult{
			Kind:       CaseCommand,
			Name:       contract.Name,
			Generation: contract.Generation,
			Adapter:    contract.Adapter,
			Fixture:    contract.Fixture,
		}
		if err := ctx.Err(); err != nil {
			result.Err = err
		} else {
			result.Err = h.verifyCommand(ctx, contract)
		}
		results = append(results, result)
	}
	for _, contract := range manifest.LiveCommands {
		result := CaseResult{
			Kind:       CaseLiveCommand,
			Name:       contract.Name,
			Generation: contract.Generation,
			Adapter:    contract.Adapter,
			Fixture:    contract.Source,
		}
		if err := ctx.Err(); err != nil {
			result.Err = err
		} else {
			result.Err = h.verifyLiveCommand(ctx, contract)
		}
		results = append(results, result)
	}
	for _, contract := range manifest.Parsers {
		result := CaseResult{
			Kind:       CaseParser,
			Name:       contract.Name,
			Generation: contract.Generation,
			Adapter:    contract.Adapter,
			Fixture:    contract.Fixture,
		}
		if err := ctx.Err(); err != nil {
			result.Err = err
		} else {
			result.Err = h.verifyParser(ctx, contract)
		}
		results = append(results, result)
	}
	return results, nil
}

func (h *Harness) verifyLiveCommand(ctx context.Context, contract LiveCommandContract) error {
	builder, ok := h.builders[contract.Adapter]
	if !ok {
		return fmt.Errorf("builder adapter is not registered")
	}
	expected, err := ParseFrame(contract.RequestFrame)
	if err != nil {
		return fmt.Errorf("parse curated request frame: %w", err)
	}
	payload, err := builder(ctx, BuildInput{
		Body:   append(json.RawMessage(nil), expected.Body...),
		Token:  expected.Token,
		Opcode: expected.Opcode,
	})
	if err != nil {
		return fmt.Errorf("build frame: %w", err)
	}
	if payload != contract.RequestFrame {
		return fmt.Errorf("live frame mismatch:\n got: %s\nwant: %s", payload, contract.RequestFrame)
	}
	return nil
}

func (h *Harness) verifyCommand(ctx context.Context, contract CommandContract) error {
	builder, ok := h.builders[contract.Adapter]
	if !ok {
		return fmt.Errorf("builder adapter is not registered")
	}
	example, err := h.corpus.Load(DirectionOutbound, contract.Fixture)
	if err != nil {
		return err
	}
	token, opcode, sequence := commandEnvelope(contract, example.Name)
	payload, err := builder(ctx, BuildInput{
		Body:   append(json.RawMessage(nil), example.Body...),
		Token:  token,
		Opcode: opcode,
	})
	if err != nil {
		return fmt.Errorf("build frame: %w", err)
	}
	actual, err := ParseFrame(payload)
	if err != nil {
		return fmt.Errorf("parse built frame: %w", err)
	}
	if actual.Token != token || actual.Opcode != opcode || actual.Sequence != sequence {
		return fmt.Errorf(
			"envelope mismatch: got token=%q opcode=%q sequence=%d; want token=%q opcode=%q sequence=%d",
			actual.Token, actual.Opcode, actual.Sequence, token, opcode, sequence,
		)
	}
	mode := contract.Match
	if mode == "" {
		mode = MatchSemanticJSON
	}
	switch mode {
	case MatchSemanticJSON:
		if !actual.HasBody {
			return fmt.Errorf("built frame has no JSON body")
		}
		return compareJSON(example.Body, actual.Body)
	case MatchNoBody:
		if actual.HasBody {
			return fmt.Errorf("built frame unexpectedly has body %s", abbreviatedJSON(actual.Body))
		}
		return nil
	case MatchExactBytes:
		expected, err := ParseFrame(contract.ExpectedFrame)
		if err != nil {
			return fmt.Errorf("parse expectedFrame: %w", err)
		}
		if expected.Token != token || expected.Opcode != opcode || expected.Sequence != sequence {
			return fmt.Errorf("expectedFrame envelope does not match contract")
		}
		if !expected.HasBody {
			return fmt.Errorf("expectedFrame has no JSON body")
		}
		if err := compareJSON(example.Body, expected.Body); err != nil {
			return fmt.Errorf("expectedFrame does not represent fixture: %w", err)
		}
		if payload != contract.ExpectedFrame {
			return fmt.Errorf("exact frame mismatch:\n got: %s\nwant: %s", payload, contract.ExpectedFrame)
		}
		return nil
	default:
		return fmt.Errorf("unknown match mode %q", mode)
	}
}

func commandEnvelope(contract CommandContract, fixture string) (string, string, int) {
	token := strings.TrimSpace(contract.ExpectedToken)
	if token == "" {
		token = "EmpireEx_21"
	}
	opcode := strings.TrimSpace(contract.ExpectedOpcode)
	if opcode == "" {
		opcode = fixture
	}
	sequence := contract.ExpectedSequence
	if sequence == 0 {
		sequence = 1
	}
	return token, opcode, sequence
}

func (h *Harness) verifyParser(ctx context.Context, contract ParserContract) error {
	parser, ok := h.parsers[contract.Adapter]
	if !ok {
		return fmt.Errorf("parser adapter is not registered")
	}
	example, err := h.corpus.Load(DirectionInbound, contract.Fixture)
	if err != nil {
		return err
	}
	parsed, err := parser(ctx, append(json.RawMessage(nil), example.Body...))
	if err != nil {
		return fmt.Errorf("parse fixture: %w", err)
	}
	actual, err := json.Marshal(parsed)
	if err != nil {
		return fmt.Errorf("marshal parser snapshot: %w", err)
	}
	return compareJSON(contract.Expected, actual)
}

func compareJSON(expected, actual []byte) error {
	want, err := canonicalJSON(expected)
	if err != nil {
		return fmt.Errorf("invalid expected JSON: %w", err)
	}
	got, err := canonicalJSON(actual)
	if err != nil {
		return fmt.Errorf("invalid actual JSON: %w", err)
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("JSON mismatch:\n got: %s\nwant: %s", abbreviatedJSON(got), abbreviatedJSON(want))
	}
	return nil
}

func canonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected JSON after first value")
		}
		return err
	}
	return nil
}

func abbreviatedJSON(raw []byte) string {
	const limit = 1200
	if len(raw) <= limit {
		return string(raw)
	}
	return string(raw[:limit]) + fmt.Sprintf("... (%d bytes total)", len(raw))
}
