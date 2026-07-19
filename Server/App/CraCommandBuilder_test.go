package App

import (
	"encoding/json"
	"strings"
	"testing"

	"CitadelDesktop/Server/State"
)

func TestCRAInventoryGuardCountsFormationAndSupportItems(t *testing.T) {
	payload, err := json.Marshal(attackBody{
		Waves: []attackWave{{
			Left: attackFlank{
				Units: []attackPair{{1, 11}},
				Tools: []attackPair{{2, 3}},
			},
		}},
		SupportTroops:      []attackPair{{3, 1}},
		AttackSupportTools: []int64{4, -1, -1},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := State.CastleState{ID: 7, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
		1: 22, 2: 2, 3: 2, 4: 2,
	}}}
	err = validateCRAInventoryPayloads([]json.RawMessage{payload, payload}, source)
	if err == nil || !strings.Contains(err.Error(), "item 2") || !strings.Contains(err.Error(), "requires 6") {
		t.Fatalf("fresh inventory guard error = %v", err)
	}

	source.Units.Stationed[2] = 6
	if err := validateCRAInventoryPayloads([]json.RawMessage{payload, payload}, source); err != nil {
		t.Fatalf("fresh inventory guard rejected available formation: %v", err)
	}
}
