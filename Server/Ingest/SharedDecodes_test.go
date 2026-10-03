package Ingest

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

func referenceAdvisorMovementEnvelopes(raw json.RawMessage) ([]advisorMovementEnvelope, error) {
	var payload struct {
		Attack    *advisorMovementEnvelope `json:"AAM"`
		Movement  json.RawMessage          `json:"A"`
		Movements json.RawMessage          `json:"M"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	result := make([]advisorMovementEnvelope, 0, 3)
	if payload.Attack != nil {
		result = append(result, *payload.Attack)
	}
	if len(payload.Movement) > 0 && payload.Movement[0] == '{' {
		var movement advisorMovementEnvelope
		if err := json.Unmarshal(payload.Movement, &movement); err != nil {
			return nil, err
		}
		result = append(result, movement)
	}
	if len(payload.Movements) > 0 && string(payload.Movements) != "null" {
		var movements []advisorMovementEnvelope
		if payload.Movements[0] == '[' {
			if err := json.Unmarshal(payload.Movements, &movements); err != nil {
				return nil, err
			}
		} else {
			var movement advisorMovementEnvelope
			if err := json.Unmarshal(payload.Movements, &movement); err != nil {
				return nil, err
			}
			movements = append(movements, movement)
		}
		result = append(result, movements...)
	}
	return result, nil
}
func referenceMapSnapshotPayload(raw json.RawMessage) (json.RawMessage, [][]json.RawMessage, bool, error) {
	var payload struct {
		KingdomID json.RawMessage `json:"KID"`
		Nodes     json.RawMessage `json:"AI"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, false, err
	}
	nodes, valid := decodeRows(payload.Nodes)
	return payload.KingdomID, nodes, valid, nil
}
func referencePlayerTitleOwners(raw json.RawMessage) ([]playerTitleOwnerWire, error) {
	var payload struct {
		Owners []playerTitleOwnerWire `json:"O"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode movement owners for player titles: %w", err)
	}
	return payload.Owners, nil
}

var sharedDecodeSeeds = []string{
	`{}`, `null`, `[]`, `5`, `"x"`, ``, `{"KID":0,"AI":[]}`, `{"KID":0,"AI":null}`,
	`{"KID":0,"AI":5}`, `{"KID":"0","AI":[[2,1,2]]}`, `{"KID":0,"AI":[5]}`, `{"AI":[null]}`,
	`{"kid":0,"ai":[[2,1,2]]}`, `{"KID":0,"kid":1,"AI":[]}`, `{"KID":0,"AI":[]}`,
	`{"AAM":null}`, `{"AAM":{}}`, `{"AAM":5}`, `{"AAM":{"M":{"MID":1}}}`, `{"aam":{}}`,
	`{"A":{}}`, `{"A":[]}`, `{"A":null}`, `{"A":5}`, `{"a":{}}`, `{"M":[]}`, `{"M":{}}`,
	`{"M":"x"}`, `{"M":null}`, `{"M":[{}]}`, `{"M":[5]}`, `{"m":[{}]}`, `{"M":[null]}`,
	`{"O":[]}`, `{"O":null}`, `{"O":"x"}`, `{"O":{}}`, `{"O":[{}]}`, `{"O":[5]}`, `{"o":[{}]}`,
	`{"O":[{"OID":424242,"PRE":116,"SUF":31}]}`, `{"AAM":{},"A":{},"M":[{}]}`, `{"O":[{"OID":"x"}]}`,
}

func sharedError(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// Duplicate top-level typed members can merge in struct decoding; the accepted
// serializer contract excludes them (AD-W2-3). RawMessage duplicates also skip.
func duplicateTopLevelKeys(raw []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err = dec.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok {
			return false
		}
		if seen[key] {
			return true
		}
		seen[key] = true
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			return false
		}
	}
	return false
}
func compareSharedDecodes(t *testing.T, raw []byte) {
	t.Helper()
	if duplicateTopLevelKeys(raw) {
		t.Skip("duplicate top-level keys excluded by AD-W2-3")
	}
	frame := Protocol.Frame{Payload: raw}.WithPayloadView()
	k, n, ok, e := mapSnapshotPayload(frame)
	wk, wn, wok, we := referenceMapSnapshotPayload(raw)
	if !reflect.DeepEqual(k, wk) || !reflect.DeepEqual(n, wn) || ok != wok || sharedError(e) != sharedError(we) {
		t.Fatalf("map payload %s: %v/%v/%v/%v != %v/%v/%v/%v", raw, k, n, ok, e, wk, wn, wok, we)
	}
	a, e := advisorMovementEnvelopes(frame)
	wa, we := referenceAdvisorMovementEnvelopes(raw)
	if !reflect.DeepEqual(a, wa) || sharedError(e) != sharedError(we) {
		t.Fatalf("advisor %s: %#v/%v != %#v/%v", raw, a, e, wa, we)
	}
	o, e := playerTitleOwners(frame)
	wo, we := referencePlayerTitleOwners(raw)
	if !reflect.DeepEqual(o, wo) || sharedError(e) != sharedError(we) {
		t.Fatalf("titles %s: %#v/%v != %#v/%v", raw, o, e, wo, we)
	}
}
func TestSharedDecodesMatchPreviousDecodes(t *testing.T) {
	for _, raw := range sharedDecodeSeeds {
		t.Run(raw, func(t *testing.T) { compareSharedDecodes(t, []byte(raw)) })
	}
}
func FuzzSharedDecodes(f *testing.F) {
	for _, raw := range sharedDecodeSeeds {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) { compareSharedDecodes(t, raw) })
}
func TestPlayerProtectionLeavesTheSharedRootUntouched(t *testing.T) {
	code := 0
	frame := Protocol.Frame{Opcode: "jaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Now().UTC(), Payload: json.RawMessage(`{"uap":{"PMS":1,"PMT":600},"gca":{"O":{"OID":424242,"RPT":600}}}`)}.WithPayloadView()
	state := State.NewGameState()
	state.Player.ID = 424242
	if _, _, err := reducePlayerProtectionMode(t.Context(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	got, err := frame.PayloadRoot()
	var want map[string]json.RawMessage
	_ = json.Unmarshal(frame.Payload, &want)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("shared root changed", got, err)
	}
}
func sharedPipeline(t *testing.T) *Pipeline {
	t.Helper()
	raw, err := os.ReadFile("testdata/ingest_session_gamedata.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Player.ID = 424242
	state.Session = State.SessionState{LoggedIn: true, Generation: 1, ConnectionGeneration: 1}
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	return NewPipeline(State.NewStore(&state), staticGameDataProvider{data}, registry)
}
func TestSharedRootDecodedOncePerCommit(t *testing.T) {
	for _, tc := range []struct {
		op, payload string
		rows        int
	}{{"gaa", `{"KID":0,"AI":[[2,100,100,-1,845,60,0]]}`, 1}, {"gam", `{"M":[],"O":[{"OID":424242,"PRE":116,"SUF":31}]}`, 0}, {"jaa", `{"KID":0,"gca":{"A":[1,100,100,100001,424242],"O":{"OID":424242,"RPT":600}},"spl":{"LID":0,"QS":[]},"sin":[],"gcu":{}}`, 0}} {
		t.Run(tc.op, func(t *testing.T) {
			p := sharedPipeline(t)
			var captured Protocol.Frame
			entry := p.registry.inboundReducers[tc.op]
			original := entry.steps[0].reducer
			entry.steps[0].reducer = func(ctx context.Context, f Protocol.Frame, s *State.GameState, g *GameData.Store) ([]string, bool, error) {
				captured = f
				return original(ctx, f, s, g)
			}
			p.registry.inboundReducers[tc.op] = entry
			if _, err := p.HandleRawAt(t.Context(), fmt.Sprintf("%%xt%%%s%%1%%0%%%s%%", tc.op, tc.payload), Protocol.DirectionInbound, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			stats := captured.PayloadViewStats()
			if stats.RootDecodes != 1 || stats.RowDecodes != tc.rows {
				t.Fatalf("stats=%+v", stats)
			}
		})
	}
}
func TestPublishedFramesCarryNoView(t *testing.T) {
	p := sharedPipeline(t)
	sub, cancel := p.SubscribeFrames(2)
	defer cancel()
	watch, cw := p.Watch("gaa", p.state.Revision())
	defer cw()
	wire, cww := p.WatchWire("gaa")
	defer cww()
	f, err := Protocol.Decode(`%xt%gaa%1%0%{"KID":0,"AI":[[2,100,100,-1,845,60,0]]}%`, Protocol.DirectionInbound, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	committed, err := p.HandleFrame(t.Context(), f)
	if err != nil {
		t.Fatal(err)
	}
	waited, err := p.WaitCommitted(t.Context(), committed.IngressID)
	if err != nil {
		t.Fatal(err)
	}
	for name, ch := range map[string]<-chan Protocol.CommittedFrame{"subscriber": sub, "watch": watch, "wire": wire} {
		select {
		case got := <-ch:
			if got.Frame.HasPayloadView() {
				t.Fatal(name, "retained view")
			}
		case <-time.After(time.Second):
			t.Fatal(name, "missing frame")
		}
	}
	if committed.Frame.HasPayloadView() || waited.Frame.HasPayloadView() {
		t.Fatal("return retained view")
	}
}
func TestObserveTransportFrameMatchesDecodeTransportFrameAt(t *testing.T) {
	p := sharedPipeline(t)
	at := time.Now().UTC()
	raw := `%xt%gaa%1%0%{"KID":0,"AI":[]}%`
	f, err := Protocol.Decode(raw, Protocol.DirectionInbound, at)
	if err != nil {
		t.Fatal(err)
	}
	a := p.ObserveTransportFrame(f, " token ", " operation ")
	b, err := p.DecodeTransportFrameAt(raw, Protocol.DirectionInbound, at, " token ", " operation ")
	if err != nil {
		t.Fatal(err)
	}
	a.IngressID = 0
	b.IngressID = 0
	a.Frame = a.Frame.WithoutPayloadView()
	b.Frame = b.Frame.WithoutPayloadView()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%#v != %#v", a, b)
	}
}
