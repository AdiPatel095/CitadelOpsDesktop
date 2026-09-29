package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAttackCoolingDownRepliesDeferOnlyTheCorrelatedTarget(t *testing.T) {
	base := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Session.Generation = 3
	gameState.Map[1] = map[string]State.MapObservation{
		"101:100": {KingdomID: 1, X: 101, Y: 100, TypeID: State.MapTypeKingdomFortress, FortressDefeaterPlayerID: 42, ObservedAt: base},
		"300:300": {KingdomID: 1, X: 300, Y: 300, TypeID: 2, ObservedAt: base},
	}
	gameState.Map[4] = map[string]State.MapObservation{
		"50:60": {KingdomID: 4, X: 50, Y: 60, TypeID: State.MapTypeStormFort, ObservedAt: base},
	}
	store, pipeline := commandCorrelationPipeline(t, gameState)
	send := func(opcode, payload string, at time.Time) {
		handleCorrelationFrame(t, pipeline, Protocol.Frame{
			Direction: Protocol.DirectionOutbound, Opcode: opcode, Payload: json.RawMessage(payload), ReceivedAt: at,
			CausationOperationID: "op-" + opcode,
		})
	}
	reply := func(opcode string, code int, payload string, at time.Time) {
		frame := Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: opcode, ResponseCode: &code, ReceivedAt: at}
		if payload != "" {
			frame.Payload = json.RawMessage(payload)
		}
		handleCorrelationFrame(t, pipeline, frame)
	}

	// ABI 95 on a fortress this player defeated: personal deferral.
	send("abi", `{"SX":100,"SY":100,"TX":101,"TY":100,"KID":1}`, base)
	reply("abi", 95, "", base.Add(100*time.Millisecond))
	fortress, found := State.AttackTargetRejectedAt(store.ReadOnlyView(), 1, State.MapTypeKingdomFortress, 101, 100, base.Add(time.Second))
	if !found || !fortress.Personal || fortress.Until.Sub(fortress.ObservedAt) != State.AttackTargetRejectionPersonalBase ||
		fortress.OperationID != "op-abi" {
		t.Fatalf("fortress rejection = %#v found=%t", fortress, found)
	}

	// CRA 95 on a Storm fort defers it; CRA 95 on an unrelated tower does not.
	send("cra", `{"SX":1,"SY":1,"TX":50,"TY":60,"KID":4,"LID":5}`, base.Add(time.Second))
	reply("cra", 95, "", base.Add(time.Second+100*time.Millisecond))
	send("cra", `{"SX":1,"SY":1,"TX":300,"TY":300,"KID":1,"LID":5}`, base.Add(2*time.Second))
	reply("cra", 95, "", base.Add(2*time.Second+100*time.Millisecond))
	view := store.ReadOnlyView()
	if _, found := State.AttackTargetRejectedAt(view, 4, State.MapTypeStormFort, 50, 60, base.Add(3*time.Second)); !found {
		t.Fatal("Storm CRA 95 did not defer the fort")
	}
	if len(view.AttackAnalytics.RejectedTargets) != 2 || len(view.CommandContext.PendingRequests) != 0 {
		t.Fatalf("registry = %#v pending=%#v", view.AttackAnalytics.RejectedTargets, view.CommandContext.PendingRequests)
	}

	// A fresh map zero does not clear the rejection.
	ok := 0
	handleCorrelationFrame(t, pipeline, Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "gaa", ResponseCode: &ok, ReceivedAt: base.Add(3 * time.Second),
		Payload: json.RawMessage(`{"KID":1,"AI":[[11,101,100,-1,45,0,42,0]]}`),
	})
	if _, found := State.AttackTargetRejectedAt(store.ReadOnlyView(), 1, State.MapTypeKingdomFortress, 101, 100, base.Add(4*time.Second)); !found {
		t.Fatal("fresh GAA zero erased an active rejection")
	}

	// A confirmed CRA 0 clears the Storm fort; an own victory clears the fortress.
	send("cra", `{"SX":1,"SY":1,"TX":50,"TY":60,"KID":4,"LID":5}`, base.Add(5*time.Second))
	reply("cra", 0, `{}`, base.Add(5*time.Second+100*time.Millisecond))
	reply("bls", 0, `{"MID":101,"LID":202,"PBI":[[42,0,1700,-10],[-220,1,135,-135]],"AI":{"AT":11,"K":1,"X":101,"Y":100}}`, base.Add(6*time.Second))
	if rejected := store.ReadOnlyView().AttackAnalytics.RejectedTargets; len(rejected) != 0 {
		t.Fatalf("confirmed launch/victory left rejections: %#v", rejected)
	}
}

func TestPackagePurchaseDispatchIsRecordedExceptStormLunaShop(t *testing.T) {
	base := time.Now().UTC()
	gameState := State.NewGameState()
	store, pipeline := commandCorrelationPipeline(t, gameState)
	handleCorrelationFrame(t, pipeline, Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "sbp", ReceivedAt: base, CausationOperationID: "buy-1",
		Payload: json.RawMessage(`{"PID":3857,"BT":0,"TID":94,"AMT":50,"KID":0,"AID":-1,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}`),
	})
	dispatch := store.ReadOnlyView().Inventory.LastPackagePurchaseDispatch
	if dispatch.PackageID != 3857 || dispatch.Amount != 50 || dispatch.OperationID != "buy-1" || !dispatch.SentAt.Equal(base) {
		t.Fatalf("package dispatch = %#v", dispatch)
	}
	handleCorrelationFrame(t, pipeline, Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "sbp", ReceivedAt: base.Add(time.Second),
		Payload: json.RawMessage(`{"PID":5,"BT":3,"TID":-1,"AMT":1,"KID":4,"AID":-1}`),
	})
	if got := store.ReadOnlyView().Inventory.LastPackagePurchaseDispatch; !got.SentAt.Equal(base) {
		t.Fatalf("Storm Luna purchase replaced the package dispatch: %#v", got)
	}
}
