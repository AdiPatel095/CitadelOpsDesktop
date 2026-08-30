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

func TestBuildCRACommandStepsAlwaysAssemblesSupportWave(t *testing.T) {
	source := State.CastleState{ID: 7}
	steps, err := buildCRACommandSteps(
		source,
		[]State.CommanderID{4},
		"Test attack",
		func(State.CommanderID) (json.RawMessage, error) {
			return json.RawMessage(`{"A":[{}]}`), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 {
		t.Fatalf("CRA steps = %d, want 3", len(steps))
	}
	var fields struct {
		Tools        []int64      `json:"AST"`
		Support      []attackPair `json:"RW"`
		SupportCount *int         `json:"ASCT"`
	}
	if err := json.Unmarshal(steps[2].Command.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields.Tools) != 3 || len(fields.Support) != 8 || fields.SupportCount == nil || *fields.SupportCount != 0 {
		t.Fatalf("assembled support fields = %#v", fields)
	}
	for _, itemID := range fields.Tools {
		if itemID != -1 {
			t.Fatalf("empty support tools = %#v", fields.Tools)
		}
	}
	for _, pair := range fields.Support {
		if pair != (attackPair{-1, 0}) {
			t.Fatalf("empty support troops = %#v", fields.Support)
		}
	}
}

func TestEnsureCRASupportPayloadPreservesSuppliedSupportWave(t *testing.T) {
	payload, err := ensureCRASupportPayload(json.RawMessage(`{
		"A":[{}],"AST":[400,-1,-1],"RW":[[524,2400],[-1,0]],"ASCT":2
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		Tools        []int64      `json:"AST"`
		Support      []attackPair `json:"RW"`
		SupportCount int          `json:"ASCT"`
	}
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields.Tools) != 3 || fields.Tools[0] != 400 ||
		len(fields.Support) != 2 || fields.Support[0] != (attackPair{524, 2400}) ||
		fields.SupportCount != 2 {
		t.Fatalf("supplied support fields changed: %#v", fields)
	}
}
