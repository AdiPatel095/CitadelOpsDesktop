package GameParser

import (
	"encoding/json"
	"testing"

	alliance "CitadelDesktop/Server/Models/Alliance"
	gamestate "CitadelDesktop/Server/Models/GameState"
)

func testGSWithCastle(aid int) *gamestate.GameState {
	gs := gamestate.GetGameState()
	gs.Reset()
	gs.Alliance.PlayerCastleLocations = []alliance.PlayerCastleLocation{{CastleID: aid, KingdomID: 1}}
	gs.Castle.MainCastle.Aid = float64(aid)
	return gs
}

func TestApplyCraftingFromCRINJSON_CAIObjectLikeJAA(t *testing.T) {
	const payload = `{
  "crai":{
    "CAI":{
      "CBI":[
        {
          "KID":0,
          "AID":16326717,
          "OID":293,
          "CQID":2,
          "QS":{"CRID":[],"BV":[],"RUT":[244047,244048]},
          "PS":{"CRID":[],"BV":[],"RUT":[244048]},
          "S":4,
          "WID":2177
        }
      ]
    }
  }
}`
	gs := testGSWithCastle(16326717)

	if !ApplyCraftingFromCRINJSON(gs, payload) {
		t.Fatal("expected crin parse to succeed for CAI object + crai wrapper")
	}
	c := gs.GetCastleByID(16326717)
	if c == nil || len(c.CraftingQueues) != 1 {
		t.Fatalf("crafting queues: castle=%v n=%d", c != nil, len(c.CraftingQueues))
	}
	q := c.CraftingQueues[0]
	if q.WID != 2177 || q.OID != 293 {
		t.Fatalf("wid/oid: %+v", q)
	}
	if len(q.PS.CRID) != 1 || q.PS.CRID[0] != 244048 {
		t.Fatalf("ps crid from RUT: %+v", q.PS.CRID)
	}
	if len(q.QS.CRID) != 2 || q.QS.CRID[0] != 244047 {
		t.Fatalf("qs crid from RUT: %+v", q.QS.CRID)
	}
}

func TestApplyCraftingFromCRINJSON_ArrayCAIUppercaseCRID(t *testing.T) {
	payload := `{"CAI":[{"CBI":[{"KID":0,"AID":99,"OID":1,"CQID":1,"WID":2174,"PS":{"CRID":[5],"BV":[2]},"QS":{"CRID":[6,7],"BV":[1,1]}}]}]}`
	gs := testGSWithCastle(99)

	if !ApplyCraftingFromCRINJSON(gs, payload) {
		t.Fatal("expected array CAI to parse")
	}
	c := gs.GetCastleByID(99)
	if len(c.CraftingQueues[0].PS.CRID) != 1 || c.CraftingQueues[0].PS.CRID[0] != 5 {
		t.Fatalf("ps: %+v", c.CraftingQueues[0].PS)
	}
}

func TestApplyCraftingFromCRINJSON_LowercaseKeys(t *testing.T) {
	payload := `{"cai":{"cbi":[{"kid":0,"aid":100,"oid":2,"cqid":1,"wid":3069,"ps":{"crid":[3],"bv":[1]},"qs":{"crid":[],"bv":[]}}]}}`
	gs := testGSWithCastle(100)

	if !ApplyCraftingFromCRINJSON(gs, payload) {
		t.Fatal("expected lowercase cai/cbi/crid to parse")
	}
	c := gs.GetCastleByID(100)
	if c.CraftingQueues[0].PS.CRID[0] != 3 {
		t.Fatalf("crid: %+v", c.CraftingQueues[0].PS)
	}
}

func TestApplyCraftingFromCRSTJSON_CBOWrapper(t *testing.T) {
	payload := `{"CBI":{"AID":101,"OID":5,"KID":0,"CQID":1,"WID":2177,"PS":{"CRID":[9],"BV":[]},"QS":{"CRID":[],"BV":[]}}}`
	gs := testGSWithCastle(101)

	if !ApplyCraftingFromCRSTJSON(gs, payload) {
		t.Fatal("crst CBI wrapper")
	}
	c := gs.GetCastleByID(101)
	if len(c.CraftingQueues) != 1 || c.CraftingQueues[0].PS.CRID[0] != 9 {
		t.Fatalf("%+v", c.CraftingQueues)
	}
}

func TestUnmarshalJSONObjectPayload_DoubleEncoded(t *testing.T) {
	inner := `{"CAI":{"CBI":[{"WID":1,"AID":102,"OID":1,"KID":0,"CQID":0,"PS":{},"QS":{}}]}}`
	wrapped, _ := json.Marshal(inner)
	m, err := unmarshalJSONObjectPayload(string(wrapped))
	if err != nil {
		t.Fatal(err)
	}
	if len(extractCBIEntries(m)) != 1 {
		t.Fatalf("%+v", m)
	}
}
