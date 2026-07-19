package GameData

import "CitadelDesktop/Server/State"

// DungeonDefinition is the official robber-baron or kingdom-tower row for a
// specific kingdom and completed-victory count.
type DungeonDefinition struct {
	KingdomID   State.KingdomID
	VictoryCount int64
	LordID       int64
	SkipCost     int64
}

func (store *Store) Dungeon(kingdomID State.KingdomID, victoryCount int64) (DungeonDefinition, bool) {
	if store == nil {
		return DungeonDefinition{}, false
	}
	catalog, err := store.Catalog("dungeons")
	if err != nil {
		return DungeonDefinition{}, false
	}
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		rowKingdomID, hasKingdomID := record.Int64("kID")
		rowVictoryCount, hasVictoryCount := record.Int64("countVictories")
		if !hasKingdomID || !hasVictoryCount || State.KingdomID(rowKingdomID) != kingdomID || rowVictoryCount != victoryCount {
			continue
		}
		lordID, _ := record.Int64("lordID")
		skipCost, _ := record.Int64("skipCosts")
		return DungeonDefinition{
			KingdomID: kingdomID, VictoryCount: victoryCount, LordID: lordID, SkipCost: skipCost,
		}, true
	}
	return DungeonDefinition{}, false
}
