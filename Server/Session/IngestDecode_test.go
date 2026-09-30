package Session

import (
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func referenceAllianceHelpFocusIdentity(frame Protocol.Frame) (int64, int64, bool) {
	if frame.Opcode != "jaa" || len(frame.Payload) == 0 ||
		frame.ResponseCode == nil || *frame.ResponseCode != 0 {
		return 0, 0, false
	}
	var response struct {
		Castle struct {
			Owner struct {
				PlayerID int64 `json:"OID"`
			} `json:"O"`
			Resources struct {
				CastleID int64 `json:"AID"`
			} `json:"grc"`
			Address []json.RawMessage `json:"A"`
		} `json:"gca"`
	}
	if json.Unmarshal(frame.Payload, &response) != nil {
		return 0, 0, false
	}
	playerID := response.Castle.Owner.PlayerID
	castleID := response.Castle.Resources.CastleID
	if playerID <= 0 && len(response.Castle.Address) > 4 {
		_ = json.Unmarshal(response.Castle.Address[4], &playerID)
	}
	if castleID <= 0 && len(response.Castle.Address) > 3 {
		_ = json.Unmarshal(response.Castle.Address[3], &castleID)
	}
	if playerID > 0 && castleID > 0 {
		return playerID, castleID, true
	}
	return 0, 0, false
}
func referenceGAACorrelation(transport *DirectWebSocketTransport, frame Protocol.Frame) string {
	opcode := "gaa"
	if opcode == "gaa" {
		matchedIndex := -1
		smallestArea := int64(0)
		ambiguous := false
		matchedCount := 0
		for index, pending := range transport.pending {
			if _, expected := pending.opcodes[opcode]; !expected ||
				!directResponseMatchesRequest(pending, frame) {
				continue
			}
			matchedCount++
			area := int64(pending.gaaX2-pending.gaaX1+1) * int64(pending.gaaY2-pending.gaaY1+1)
			if matchedIndex < 0 || area < smallestArea {
				matchedIndex, smallestArea, ambiguous = index, area, false
			} else if area == smallestArea {
				ambiguous = true
			}
		}
		if matchedIndex < 0 || ambiguous || matchedCount > 1 && !directGAAResponseHasCoordinates(frame) {
			return ""
		}
		pending := transport.pending[matchedIndex]
		transport.pending = append(transport.pending[:matchedIndex], transport.pending[matchedIndex+1:]...)
		return pending.token
	}
	return ""
}

func sessionDecodePipeline(t *testing.T) (*Ingest.Pipeline, *State.Store) {
	t.Helper()
	s := State.NewGameState()
	s.Player.ID = 424242
	s.Session = State.SessionState{LoggedIn: true, Generation: 1, ConnectionGeneration: 1}
	store := State.NewStore(s)
	reg := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(reg); err != nil {
		t.Fatal(err)
	}
	return Ingest.NewPipeline(store, nil, reg), store
}
func pendingTokens(pending []directPendingResponse) []string {
	var tokens []string
	for _, p := range pending {
		tokens = append(tokens, p.token)
	}
	return tokens
}
func TestDirectGAAAreaMatchesReferenceCorrelation(t *testing.T) {
	var payloads []string
	for _, kid := range []string{`0`, `1`, `null`, `"0"`, `0.0`} {
		for _, ai := range []string{`null`, `[]`, `[[2,100,100]]`, `[[2,101,100]]`, `[[2,99,100]]`, `[[2,-1,100]]`, `[[2,100.5,100]]`, `[[2,100]]`, `5`, `[[2,100,100],[2,102,100]]`} {
			payloads = append(payloads, `{"KID":`+kid+`,"AI":`+ai+`}`)
		}
	}
	payloads = append(payloads, `{}`, `{"KID":0}`, `{"AI":[]}`, `{"kid":0,"ai":[[2,100,100]]}`, `{"KID":0,"AI":[]}`, `[]`, `null`, `5`)
	combinations := 0
	for n := 1; n <= 4; n++ {
		for variant := 0; variant < 4; variant++ {
			pending := make([]directPendingResponse, n)
			for i := range pending {
				pending[i] = directPendingResponse{token: fmt.Sprint(i), opcodes: map[string]struct{}{"gaa": {}}, requestOpcode: "gaa", gaaScopeKnown: true, gaaKingdomID: 0, gaaX1: 100, gaaX2: 102 + i, gaaY1: 100, gaaY2: 100, expiresAt: time.Now().Add(time.Hour)}
				switch variant {
				case 1:
					pending[i].gaaX2 = 102
				case 2:
					if i == 0 {
						pending[i].gaaScopeKnown = false
					}
				case 3:
					if i == 0 {
						pending[i].requestOpcode = "other"
					}
				}
			}
			for _, raw := range payloads {
				for _, code := range []*int{nil, new(int), func() *int { v := 1; return &v }()} {
					f := Protocol.Frame{Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: code, Payload: json.RawMessage(raw)}.WithPayloadView()
					a := &DirectWebSocketTransport{pending: append([]directPendingResponse(nil), pending...)}
					b := &DirectWebSocketTransport{pending: append([]directPendingResponse(nil), pending...)}
					got := a.matchResponseToken(f)
					want := referenceGAACorrelation(b, f)
					if got != want || !reflect.DeepEqual(pendingTokens(a.pending), pendingTokens(b.pending)) {
						t.Fatalf("n=%d variant=%d payload=%s code=%v: %q/%v != %q/%v", n, variant, raw, code, got, pendingTokens(a.pending), want, pendingTokens(b.pending))
					}
					combinations++
				}
			}
		}
	}
	t.Logf("%d correlation combinations", combinations)
}
func TestDeliverInboundPassesTheDecodedFrame(t *testing.T) {
	transport := &DirectWebSocketTransport{frames: make(chan RawFrame, 1)}
	raw := decodeBenchGaa()
	if _, err := transport.registerPending(Outbound.Metadata{ResponseToken: "fixture", ResponseOpcodes: []string{"gaa"}}, `%xt%EmpireEx_21%gaa%1%{"KID":0,"AX1":100,"AY1":100,"AX2":399,"AY2":100}%`); err != nil {
		t.Fatal(err)
	}
	transport.deliverInbound(raw, 1)
	rf := drainOutbox(transport)[0]
	if rf.Decoded == nil {
		t.Fatal("missing decode")
	}
	want, err := Protocol.Decode(raw, Protocol.DirectionInbound, rf.ObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rf.Decoded.WithoutPayloadView(), want.WithoutPayloadView()) || rf.ResponseToken != "fixture" {
		t.Fatal("frame/token mismatch")
	}
	stats := rf.Decoded.PayloadViewStats()
	if stats.RootDecodes != 1 || stats.RowDecodes != 1 {
		t.Fatal(stats)
	}
	p, _ := sessionDecodePipeline(t)
	if _, err := p.CommitFrame(t.Context(), p.ObserveTransportFrame(*rf.Decoded, rf.ResponseToken, rf.CausationOperationID)); err != nil {
		t.Fatal(err)
	}
	if rf.Decoded.PayloadViewStats() != stats {
		t.Fatal("commit re-decoded", rf.Decoded.PayloadViewStats())
	}
}
func TestControllerUsesTheTransportsDecodedFrame(t *testing.T) {
	for _, decoded := range []bool{true, false} {
		t.Run(fmt.Sprint(decoded), func(t *testing.T) {
			transport := newPacingTransport()
			pipeline, store := sessionDecodePipeline(t)
			frames, cancelFrames := pipeline.SubscribeFrames(2)
			defer cancelFrames()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controller := NewController(ctx, transport, pipeline, store)
			if err := controller.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			at := time.Now().UTC()
			raw := RawFrame{Payload: `%xt%aaa%1%0%{}%`, Direction: Protocol.DirectionInbound, ObservedAt: at, ConnectionGeneration: 1}
			want := "aaa"
			if decoded {
				f, e := Protocol.Decode(`%xt%bbb%1%0%{}%`, Protocol.DirectionInbound, at)
				if e != nil {
					t.Fatal(e)
				}
				raw.Decoded = &f
				want = "bbb"
			}
			select {
			case transport.frames <- raw:
			case <-time.After(time.Second):
				t.Fatal("controller did not read")
			}
			select {
			case frame := <-frames:
				if frame.Frame.Opcode != want {
					t.Fatal(frame.Frame.Opcode, want)
				}
			case <-time.After(time.Second):
				t.Fatal("no committed frame")
			}
		})
	}
}
func TestAllianceHelpFocusIdentityMatchesReference(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `[]`, `5`, `{"gca":null}`, `{"gca":5}`, `{"gca":{"A":[1,2,3,100001,424242]}}`, `{"GCA":{"A":[1,2,3,100001,424242]}}`, `{"gca":{"O":{"OID":424242},"grc":{"AID":100001}}}`, `{"gca":{"O":{"OID":"x"},"A":[1,2,3,100001,424242]}}`, `{"gca":{},"GCA":{"A":[1,2,3,100001,424242]}}`} {
		code := 0
		f := Protocol.Frame{Opcode: "jaa", ResponseCode: &code, Payload: json.RawMessage(raw)}.WithPayloadView()
		p, c, ok := allianceHelpFocusIdentity(f)
		wp, wc, wok := referenceAllianceHelpFocusIdentity(f)
		if p != wp || c != wc || ok != wok {
			t.Fatal(raw, p, c, ok, wp, wc, wok)
		}
	}
}
