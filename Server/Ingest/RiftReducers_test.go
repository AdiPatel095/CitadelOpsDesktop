package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestRiftReducersCaptureTemplateAndTravelTime(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{
		"10:20": {KingdomID: 0, X: 10, Y: 20, TypeID: riftObservationTypeID},
	}
	outbound := Protocol.Frame{
		Opcode: "cra", Direction: Protocol.DirectionOutbound, ReceivedAt: time.Unix(1000, 0).UTC(),
		Payload: json.RawMessage(`{"LID":5,"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"AV":1,"HBW":-1,"PTT":1,"A":[{"L":{"U":[[300,11]]}}]}`),
	}
	_, changed, err := reduceRiftLaunchCapture(context.Background(), outbound, &gameState, nil)
	if err != nil || !changed || len(gameState.Rift.Launches) != 1 {
		t.Fatalf("capture launch: changed=%v launches=%#v err=%v", changed, gameState.Rift.Launches, err)
	}
	code := 0
	inbound := Protocol.Frame{
		Opcode: "cra", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: time.Unix(1001, 0).UTC(),
		Payload: json.RawMessage(`{"M":{"MID":99,"TT":321,"PT":0,"D":0,"T":0,"KID":0,"OID":7,"SA":[0,1,2,100,7],"TA":[0,10,20,0,0]},"A":[[300,11]]}`),
	}
	_, changed, err = reduceRiftLaunchAck(context.Background(), inbound, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("capture ack: changed=%v err=%v", changed, err)
	}
	for _, launch := range gameState.Rift.Launches {
		if launch.OneWayTTSeconds != 321 || launch.LastSuccessAtUnix != 1001 || gameState.Rift.PendingLaunchID != "" {
			t.Fatalf("unexpected launch: %#v pending=%q", launch, gameState.Rift.PendingLaunchID)
		}
	}
}

func TestRiftReducerKeepsDeletedTemplateHiddenUntilNewCapture(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{
		"10:20": {KingdomID: 0, X: 10, Y: 20, TypeID: riftObservationTypeID},
	}
	outbound := Protocol.Frame{
		Opcode: "cra", Direction: Protocol.DirectionOutbound, ReceivedAt: time.Unix(1000, 0).UTC(),
		Payload: json.RawMessage(`{"LID":5,"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"A":[{"L":{"U":[[300,11]]}}]}`),
	}
	_, changed, err := reduceRiftLaunchCapture(context.Background(), outbound, &gameState, nil)
	if err != nil || !changed || len(gameState.Rift.Launches) != 1 {
		t.Fatalf("initial capture: changed=%v launches=%#v err=%v", changed, gameState.Rift.Launches, err)
	}
	var launchID string
	for id := range gameState.Rift.Launches {
		launchID = id
	}
	delete(gameState.Rift.Launches, launchID)
	gameState.Rift.PendingLaunchID = ""
	gameState.Rift.DeletedLaunchIDs[launchID] = time.Unix(2000, 0).UnixMilli()

	_, changed, err = reduceRiftLaunchCapture(context.Background(), outbound, &gameState, nil)
	if err != nil || changed || len(gameState.Rift.Launches) != 0 {
		t.Fatalf("deleted capture replay: changed=%v launches=%#v err=%v", changed, gameState.Rift.Launches, err)
	}

	newCapture := outbound
	newCapture.ReceivedAt = time.Unix(3000, 0).UTC()
	_, changed, err = reduceRiftLaunchCapture(context.Background(), newCapture, &gameState, nil)
	if err != nil || !changed || len(gameState.Rift.Launches) != 1 {
		t.Fatalf("new capture: changed=%v launches=%#v err=%v", changed, gameState.Rift.Launches, err)
	}
	if _, deleted := gameState.Rift.DeletedLaunchIDs[launchID]; deleted {
		t.Fatalf("new capture retained deletion marker for %q", launchID)
	}
}
