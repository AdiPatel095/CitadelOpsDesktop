package Khan

import (
	"testing"

	"CitadelDesktop/Server/State"
)

func TestDefenseToolDeficitsIncludeAssignedAndFreePresetTools(t *testing.T) {
	var preset DefensePreset
	preset.Wall.Left.ToolSlots = []State.DefenseToolSlot{{DefinitionID: 731, Amount: 6}}
	preset.Moat.MiddleToolSlots = []State.DefenseToolSlot{{DefinitionID: 731, Amount: 4}, {DefinitionID: 637, Amount: 5}}
	castle := State.CastleState{}
	castle.Defense.Inventory = map[State.UnitID]int64{731: 3, 637: 5}
	castle.Defense.Wall.Right.ToolSlots = []State.DefenseToolSlot{{DefinitionID: 731, Amount: 4}}

	deficits := DefenseToolDeficits(castle, preset)
	if deficits[731] != 3 {
		t.Fatalf("tool 731 deficit = %d, want 3", deficits[731])
	}
	if _, short := deficits[637]; short {
		t.Fatalf("tool 637 should be covered by free inventory: %#v", deficits)
	}
}
