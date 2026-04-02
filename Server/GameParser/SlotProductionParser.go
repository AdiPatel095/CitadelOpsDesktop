package GameParser

import (
	"CitadelDesktop/Server/Models/Castle"
	gamestate "CitadelDesktop/Server/Models/GameState"
	"encoding/json"
	"reflect"
)

func jsonIntFromMap(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func slotFromPMap(p map[string]interface{}) castle.BarracksProductionSlot {
	return castle.BarracksProductionSlot{
		WID:  jsonIntFromMap(p, "WID"),
		TUA:  jsonIntFromMap(p, "TUA"),
		RCT:  jsonIntFromMap(p, "RCT"),
		ICT:  jsonIntFromMap(p, "ICT"),
		PID:  jsonIntFromMap(p, "PID"),
		SPID: jsonIntFromMap(p, "SPID"),
	}
}

func activeSlotFromPS(ps map[string]interface{}) *castle.BarracksProductionSlot {
	if len(ps) == 0 {
		return nil
	}
	wid := jsonIntFromMap(ps, "WID")
	if wid == 0 {
		return nil
	}
	s := slotFromPMap(ps)
	return &s
}

func queuedFromQS(qs []interface{}) []castle.BarracksProductionSlot {
	var out []castle.BarracksProductionSlot
	for _, raw := range qs {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		pRaw, ok := item["P"]
		if !ok || pRaw == nil {
			continue
		}
		p, ok := pRaw.(map[string]interface{})
		if !ok || jsonIntFromMap(p, "WID") == 0 {
			continue
		}
		out = append(out, slotFromPMap(p))
	}
	return out
}

func parseSPLObject(splObj map[string]interface{}) (q castle.BarracksProductionQueue, ok bool) {
	if splObj == nil {
		return
	}
	lid := jsonIntFromMap(splObj, "LID")
	psRaw, hasPS := splObj["PS"]
	qsRaw, hasQS := splObj["QS"]
	if !hasPS && !hasQS {
		return
	}
	q.LID = lid
	q.TCT = jsonIntFromMap(splObj, "TCT")
	if psMap, okm := psRaw.(map[string]interface{}); okm {
		q.Active = activeSlotFromPS(psMap)
	}
	if qsArr, okq := qsRaw.([]interface{}); okq {
		q.Queued = queuedFromQS(qsArr)
	}
	ok = true
	return
}

func extractSPLRoot(data string) (map[string]interface{}, bool) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(data), &root); err != nil || root == nil {
		return nil, false
	}
	if spl, ok := root["spl"].(map[string]interface{}); ok {
		return spl, true
	}
	if _, hasPS := root["PS"]; hasPS {
		return root, true
	}
	if _, hasQS := root["QS"]; hasQS {
		return root, true
	}
	return nil, false
}

// ApplySlotProductionFromSPLJSON parses **spl** or **bup** JSON (latter nests under "spl") and
// stores slots on the focused castle under **spl**.LID so recruit (LID 0) and workshop (LID 1) do not clobber each other.
func ApplySlotProductionFromSPLJSON(gs *gamestate.GameState, data string) bool {
	splObj, ok := extractSPLRoot(data)
	if !ok {
		return false
	}
	q, ok := parseSPLObject(splObj)
	if !ok {
		return false
	}
	cid := gs.CastleFocus.CastleAID
	if cid <= 0 || !gs.IsKnownPlayerCastleID(cid) {
		return false
	}
	c := gs.GetCastleByID(cid)
	if c == nil {
		return false
	}
	lid := q.LID
	if c.SlotProductionByLID == nil {
		c.SlotProductionByLID = make(map[int]*castle.BarracksProductionQueue)
	}
	prev := c.SlotProductionByLID[lid]
	if barracksQueuesEqual(prev, &q) {
		return false
	}
	qcopy := q
	c.SlotProductionByLID[lid] = &qcopy
	return true
}

func barracksQueuesEqual(a, b *castle.BarracksProductionQueue) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
