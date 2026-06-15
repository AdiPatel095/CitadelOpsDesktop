package GameParser

import "CitadelDesktop/Server/Models"

func enrichGCAConstructionBuildingsLevels(rows []Models.GCAConstructionBuilding) {
	for i := range rows {
		for j := range rows[i].Slots {
			if lvl, ok := ConstructionItemLevelByCID(rows[i].Slots[j].CID); ok {
				rows[i].Slots[j].Level = lvl
			}
		}
	}
}
