package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// Sanitized structural replay of the Oct 1 cap rejection. Account and castle identities
// are invented; public package constants are retained under CIT-83; preserve list lengths, cap, omission and wire field names.
func TestStormPackageCapReplay48Then43(t *testing.T) {
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"packages":[{"packageID":3119,"comment1":"Fixture resource","comment2":"Luna's trade boat","packageType":"resource","packagePriceAquamarine":10,"stock":4}]}`), GameData.SourceMetadata{ItemVersion: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Castles[910040] = State.CastleState{ID: 910040, KingdomID: 4, Focused: true, Resources: map[State.ResourceID]State.ResourceBalance{GameData.StormAquamarineID: {Amount: 10000}}}
	state.Inventory.ConstructionOffersCastleID = 910040
	state.Inventory.ConstructionOffersKingdomID = 4
	at := time.Now().UTC()
	code := 0
	history := func(size int, include bool, stamp time.Time) {
		products := make([]map[string]int64, 0, size)
		if include {
			products = append(products, map[string]int64{"PID": 3119, "AMT": 4})
		}
		for len(products) < size {
			products = append(products, map[string]int64{"PID": 920000 + int64(len(products)), "AMT": 1})
		}
		payload, _ := json.Marshal(map[string]any{"PL": products})
		if _, _, err := reduceConstructionOffers(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "gbc", ResponseCode: &code, Payload: payload, ReceivedAt: stamp}, &state, data); err != nil {
			t.Fatal(err)
		}
	}
	history(48, true, at)
	if state.Inventory.ConstructionOffers[3119] != 4 {
		t.Fatal("initial cap missing")
	}
	history(43, false, at.Add(time.Second))
	if _, exists := state.Inventory.ConstructionOffers[3119]; exists {
		t.Fatal("replay did not omit package")
	}
	command := Protocol.Frame{Direction: Protocol.DirectionOutbound, Opcode: "sbp", Payload: json.RawMessage(`{"PID":3119,"BT":3,"TID":-1,"AMT":4,"KID":4,"AID":-1,"PC2":-1}`), ReceivedAt: at.Add(2 * time.Second)}
	if _, changed, err := reduceStormShopCommand(t.Context(), command, &state, data); err != nil || !changed {
		t.Fatalf("command changed=%v err=%v", changed, err)
	}
	code = 237
	if _, changed, err := reduceStormShopResponse(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "sbp", ResponseCode: &code, Payload: json.RawMessage(`{}`), ReceivedAt: at.Add(3 * time.Second)}, &state, data); err != nil || !changed {
		t.Fatalf("response changed=%v err=%v", changed, err)
	}
	code = 0
	history(43, false, at.Add(4*time.Second))
	if !state.StormPackageBlocked(910040, -1, 3119, at.Add(5*time.Second)) {
		t.Fatal("omission lifted cap block")
	}
	if state.Castles[910040].Resources[GameData.StormAquamarineID].Amount != 10000 {
		t.Fatal("rejection spent resources")
	}
	for _, malformed := range []string{
		`{"PL":[{"PID":3119}]}`, `{"PL":[{"PID":3119,"AMT":null}]}`,
		`{"PL":[{"PID":3119,"AMT":"invalid"}]}`, `{"PL":[{"PID":3119,"AMT":1.5}]}`,
		`{"PL":[{"PID":3119,"AMT":-1}]}`,
	} {
		if _, _, err := reduceConstructionOffers(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "gbc", ResponseCode: &code, Payload: json.RawMessage(malformed), ReceivedAt: at.Add(5 * time.Second)}, &state, data); err != nil {
			t.Fatal(err)
		}
		if !state.StormPackageBlocked(910040, -1, 3119, at.Add(5*time.Second)) {
			t.Fatal("missing or malformed count lifted cap block")
		}
	}
	payload := json.RawMessage(`{"PL":[{"PID":3119,"AMT":3}]}`)
	if _, _, err := reduceConstructionOffers(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "gbc", ResponseCode: &code, Payload: payload, ReceivedAt: at.Add(6 * time.Second)}, &state, data); err != nil {
		t.Fatal(err)
	}
	if state.StormPackageBlocked(910040, -1, 3119, at.Add(7*time.Second)) {
		t.Fatal("explicit below-cap history did not lift block")
	}
}
