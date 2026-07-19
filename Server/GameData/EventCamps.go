package GameData

import (
	"sort"
	"strconv"
)

type EventCampDefinition struct {
	ID                       int64
	EventID                  int64
	DifficultyID             int64
	AreaTypeID               int
	CampLevel                int
	VictoryCount             int64
	CooldownSec              int64
	SkipCost                 int64
	MaximumDefenseTroopCount int64
}

func (store *Store) EventCamp(id int64) (EventCampDefinition, bool) {
	if store == nil || id <= 0 {
		return EventCampDefinition{}, false
	}
	catalog, err := store.Catalog("eventAutoScalingCamps")
	if err != nil {
		return EventCampDefinition{}, false
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return EventCampDefinition{}, false
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return EventCampDefinition{}, false
	}
	definition := decodeEventCamp(record)
	return definition, definition.ID == id
}

func (store *Store) EventCampProgression(eventID, difficultyID int64, areaTypeID int) []EventCampDefinition {
	if store == nil || eventID <= 0 || difficultyID <= 0 || areaTypeID <= 0 {
		return nil
	}
	catalog, err := store.Catalog("eventAutoScalingCamps")
	if err != nil {
		return nil
	}
	result := make([]EventCampDefinition, 0, 10)
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		definition := decodeEventCamp(record)
		if definition.EventID == eventID && definition.DifficultyID == difficultyID && definition.AreaTypeID == areaTypeID {
			result = append(result, definition)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].VictoryCount != result[right].VictoryCount {
			return result[left].VictoryCount < result[right].VictoryCount
		}
		return result[left].ID < result[right].ID
	})
	return result
}

func decodeEventCamp(record Record) EventCampDefinition {
	id, _ := record.Int64("eventAutoScalingCampID")
	eventID, _ := record.Int64("eventID")
	difficultyID, _ := record.Int64("difficultyID")
	areaTypeID, _ := record.Int64("areaType")
	campLevel, _ := record.Int64("camplevel")
	victoryCount, _ := record.Int64("countVictory")
	cooldownSec, _ := record.Int64("coolDown")
	skipCost, _ := record.Int64("skipCosts")
	maximumDefenseTroopCount, _ := record.Int64("maxTroopCapacityDefense")
	return EventCampDefinition{
		ID: id, EventID: eventID, DifficultyID: difficultyID, AreaTypeID: int(areaTypeID),
		CampLevel: int(campLevel), VictoryCount: victoryCount, CooldownSec: cooldownSec,
		SkipCost: skipCost, MaximumDefenseTroopCount: maximumDefenseTroopCount,
	}
}
