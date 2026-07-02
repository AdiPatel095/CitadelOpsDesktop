package GameParser

import (
	"encoding/json"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	gamestate "CitadelDesktop/Server/Models/GameState"
)

type gpcQueueableEnvelope struct {
	GPC gpcQueueablePayload  `json:"gpc"`
	A   []gpcQueueableCastle `json:"A"`
}

type gpcQueueablePayload struct {
	A []gpcQueueableCastle `json:"A"`
}

type gpcQueueableCastle struct {
	AID int `json:"AID"`
	U   struct {
		Unlocked []int `json:"U"`
	} `json:"U"`
}

var (
	gpcToolIDSetOnce sync.Once
	gpcToolIDSet     map[int]struct{}
)

func buildGPCToolIDSet() {
	gpcToolIDSet = make(map[int]struct{})

	b, err := serverdata.ReadToolsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		WodID int `json:"wodID"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		if r.WodID > 0 {
			gpcToolIDSet[r.WodID] = struct{}{}
		}
	}
}

func queueableToolIDSet() map[int]struct{} {
	gpcToolIDSetOnce.Do(buildGPCToolIDSet)
	return gpcToolIDSet
}

func ApplyGPCQueueableFromPayload(gs *gamestate.GameState, data string) bool {
	if gs == nil {
		return false
	}
	var envelope gpcQueueableEnvelope
	if err := json.Unmarshal([]byte(data), &envelope); err != nil {
		return false
	}
	rows := envelope.GPC.A
	if len(rows) == 0 {
		rows = envelope.A
	}
	if len(rows) == 0 {
		return false
	}

	toolsByID := queueableToolIDSet()
	if len(toolsByID) == 0 {
		return false
	}
	changed := false
	for _, row := range rows {
		if row.AID <= 0 {
			continue
		}
		c := gs.GetCastleByID(row.AID)
		if c == nil {
			continue
		}
		unitIDs, toolIDs := splitGPCQueueableIDs(row.U.Unlocked, toolsByID)
		if !c.QueueableIDsLoaded {
			c.QueueableIDsLoaded = true
			changed = true
		}
		if !sameIntSlice(c.QueueableUnitIDs, unitIDs) {
			c.QueueableUnitIDs = unitIDs
			changed = true
		}
		if !sameIntSlice(c.QueueableToolIDs, toolIDs) {
			c.QueueableToolIDs = toolIDs
			changed = true
		}
	}
	return changed
}

func splitGPCQueueableIDs(ids []int, toolsByID map[int]struct{}) ([]int, []int) {
	unitSet := make(map[int]struct{})
	toolSet := make(map[int]struct{})
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := toolsByID[id]; ok {
			toolSet[id] = struct{}{}
			continue
		}
		unitSet[id] = struct{}{}
	}
	return sortedIDsFromSet(unitSet), sortedIDsFromSet(toolSet)
}

func sameIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func setFromIDs(ids []int) map[int]struct{} {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
