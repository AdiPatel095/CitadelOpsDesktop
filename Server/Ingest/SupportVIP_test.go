package Ingest

import (
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestSupportVIPObservationTracksUsedQuotaSessionAndWake(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 3
	state.Session.ConnectionGeneration = 4
	code := 0
	now := time.Now().UTC()
	frame := Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "vip", ResponseCode: &code, ReceivedAt: now, Payload: json.RawMessage(`{"VP":9000,"VRL":5,"VRS":3600,"UPG":7}`)}
	domains, changed, err := reduceVIPInfo(t.Context(), frame, &state, nil)
	if err != nil || !changed || !slices.Contains(domains, "vip") {
		t.Fatalf("domains=%v changed=%v err=%v", domains, changed, err)
	}
	vip := state.Player.VIP
	if vip.UsedPremiumCommanders != 7 || vip.Generation != 3 || vip.ConnectionGeneration != 4 || !vip.ObservedAt.Equal(now) {
		t.Fatalf("vip=%+v", vip)
	}
	// Identical values are a new authoritative observation and must wake waiters.
	frame.ReceivedAt = now.Add(time.Second)
	_, changed, err = reduceVIPInfo(t.Context(), frame, &state, nil)
	if err != nil || !changed {
		t.Fatal("fresh unchanged VIP did not wake")
	}
	// Frames ordered before the accepted observation cannot restore spent quota.
	frame.ReceivedAt = now
	frame.Payload = json.RawMessage(`{"VP":9000,"VRL":5,"VRS":3600,"UPG":0}`)
	_, changed, err = reduceVIPInfo(t.Context(), frame, &state, nil)
	if err != nil || changed || state.Player.VIP.UsedPremiumCommanders != 7 {
		t.Fatal("older observation restored quota")
	}
	raw, err := json.Marshal(state.Player.VIP)
	if err != nil {
		t.Fatal(err)
	}
	var persisted State.VIPState
	json.Unmarshal(raw, &persisted)
	if !persisted.ObservedAt.IsZero() || persisted.Generation != 0 || persisted.ConnectionGeneration != 0 {
		t.Fatal("VIP authority persisted")
	}
}

func TestSupportVIPInvalidObservationNeverAuthorizesQuota(t *testing.T) {
	for _, raw := range []string{`{}`, `{"VP":9000,"VRS":3600}`, `{"VP":9000,"VRS":3600,"UPG":-1}`, `{"VP":9000,"VRS":3600,"UPG":"0"}`, `{"VP":9000,"VRS":3600,"UPG":null}`} {
		state := State.NewGameState()
		state.Session.ConnectionGeneration = 1
		changed, err := applyVIPInfo(json.RawMessage(raw), &state, time.Now())
		if err != nil || !changed || state.Player.VIP.ConnectionGeneration != 0 {
			t.Fatalf("invalid observation accepted: %s", raw)
		}
	}
}
