package Ingest

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseMovementCapturesActiveSpyCount(t *testing.T) {
	raw := json.RawMessage(`{
		"M":{"MID":50,"PT":2,"TT":20,"D":0,"T":3,"KID":0,"OID":1,"TID":2,"SA":[0,10,11,100,1],"TA":[0,20,21,200,2]},
		"S":{"SC":7}
	}`)
	movement, ok := parseMovement(raw, time.Now().UTC())
	if !ok {
		t.Fatal("spy movement did not parse")
	}
	if movement.TypeID != 3 || movement.SpyCount != 7 || movement.SourceCastleID != 100 {
		t.Fatalf("spy movement = %+v", movement)
	}
}
