package Khan

import "CitadelDesktop/Server/State"

func DefenseToolDeficits(castle State.CastleState, preset DefensePreset) map[State.UnitID]int64 {
	desired := map[State.UnitID]int64{}
	available := map[State.UnitID]int64{}
	addSlots := func(target map[State.UnitID]int64, slots []State.DefenseToolSlot) {
		for _, slot := range slots {
			if slot.DefinitionID > 0 && slot.Amount > 0 {
				target[slot.DefinitionID] += slot.Amount
			}
		}
	}
	for _, section := range []State.DefenseWallSection{preset.Wall.Left, preset.Wall.Middle, preset.Wall.Right} {
		addSlots(desired, section.ToolSlots)
	}
	for _, slots := range [][]State.DefenseToolSlot{
		preset.Moat.LeftToolSlots, preset.Moat.MiddleToolSlots, preset.Moat.RightToolSlots,
	} {
		addSlots(desired, slots)
	}
	for toolID, amount := range castle.Defense.Inventory {
		if amount > 0 {
			available[toolID] += amount
		}
	}
	for _, section := range []State.DefenseWallSection{castle.Defense.Wall.Left, castle.Defense.Wall.Middle, castle.Defense.Wall.Right} {
		addSlots(available, section.ToolSlots)
	}
	for _, slots := range [][]State.DefenseToolSlot{
		castle.Defense.Moat.LeftToolSlots, castle.Defense.Moat.MiddleToolSlots, castle.Defense.Moat.RightToolSlots,
	} {
		addSlots(available, slots)
	}
	result := map[State.UnitID]int64{}
	for toolID, amount := range desired {
		if deficit := amount - available[toolID]; deficit > 0 {
			result[toolID] = deficit
		}
	}
	return result
}
