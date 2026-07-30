package State

import "testing"

func TestDefenseStateSnapshotIsDeepCopied(t *testing.T) {
	state := NewGameState()
	state.Castles[10] = CastleState{
		ID: 10,
		Defense: CastleDefenseState{
			Inventory:     map[UnitID]int64{501: 9},
			RangedUnitIDs: []UnitID{11},
			MeleeUnitIDs:  []UnitID{21},
			Wall:          DefenseWallState{Left: DefenseWallSection{ToolSlots: []DefenseToolSlot{{DefinitionID: 501, Amount: 2}}}},
			Keep:          DefenseKeepState{PrimaryToolSlots: []DefenseToolSlot{{DefinitionID: -1}}},
			Moat:          DefenseMoatState{LeftToolSlots: []DefenseToolSlot{{DefinitionID: 502, Amount: 1}}},
		},
	}
	store := NewStore(state)
	snapshot := store.Snapshot()
	castle := snapshot.Castles[10]
	castle.Defense.Inventory[501] = 0
	castle.Defense.RangedUnitIDs[0] = 99
	castle.Defense.Wall.Left.ToolSlots[0].Amount = 0
	castle.Defense.Keep.PrimaryToolSlots[0].DefinitionID = 501
	castle.Defense.Moat.LeftToolSlots[0].Amount = 0
	snapshot.Castles[10] = castle

	unchanged := store.Snapshot().Castles[10].Defense
	if unchanged.Inventory[501] != 9 || unchanged.RangedUnitIDs[0] != 11 ||
		unchanged.Wall.Left.ToolSlots[0].Amount != 2 || unchanged.Keep.PrimaryToolSlots[0].DefinitionID != -1 ||
		unchanged.Moat.LeftToolSlots[0].Amount != 1 {
		t.Fatalf("defense state snapshot aliases store data: %#v", unchanged)
	}
}
