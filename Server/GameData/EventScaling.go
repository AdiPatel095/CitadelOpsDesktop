package GameData

import (
	"fmt"
	"strconv"
	"strings"
)

type ScalableEventDefinition struct {
	EventID             int64
	EventType           string
	Name                string
	LocalizationKey     string
	DifficultyID        int64
	DifficultyTypeID    int64
	DifficultyTypeName  string
	IsLocked            bool
	RentCurrency2Cost   int64
	UnlockAchievementID int64
}

func (store *Store) ScalableEvent(eventID int64, difficultyID int64) (ScalableEventDefinition, bool) {
	if store == nil || eventID <= 0 {
		return ScalableEventDefinition{}, false
	}
	difficulties, err := store.Catalog("eventAutoScalingDifficulties")
	if err != nil {
		return ScalableEventDefinition{}, false
	}
	var difficultyRaw []byte
	var found bool
	if difficultyID > 0 {
		difficultyRaw, found = difficulties.Find(strconv.FormatInt(difficultyID, 10))
	} else {
		difficultyRaw, found = difficulties.FindByField("eventID", strconv.FormatInt(eventID, 10))
	}
	if !found {
		return ScalableEventDefinition{}, false
	}
	difficulty, err := DecodeRecord(difficultyRaw)
	if err != nil {
		return ScalableEventDefinition{}, false
	}
	definitionEventID, ok := difficulty.Int64("eventID")
	if !ok || definitionEventID != eventID {
		return ScalableEventDefinition{}, false
	}

	definition := ScalableEventDefinition{
		EventID:         eventID,
		DifficultyID:    difficultyID,
		LocalizationKey: fmt.Sprintf("event_title_%d", eventID),
	}
	if difficultyID > 0 {
		definition.DifficultyTypeID, _ = difficulty.Int64("difficultyTypeID")
		locked, _ := difficulty.Int64("isLocked")
		definition.IsLocked = locked == 1
		definition.RentCurrency2Cost, _ = difficulty.Int64("rentC2Cost")
		if achievements, catalogErr := store.Catalog("achievements"); catalogErr == nil {
			if raw, achievementFound := achievements.FindByField("unlocksDifficulty", strconv.FormatInt(difficultyID, 10)); achievementFound {
				if achievement, decodeErr := DecodeRecord(raw); decodeErr == nil {
					definition.UnlockAchievementID, _ = achievement.Int64("achievementID")
				}
			}
		}
		if types, catalogErr := store.Catalog("eventAutoScalingDifficultyTypes"); catalogErr == nil {
			if raw, typeFound := types.Find(strconv.FormatInt(definition.DifficultyTypeID, 10)); typeFound {
				if difficultyType, decodeErr := DecodeRecord(raw); decodeErr == nil {
					definition.DifficultyTypeName, _ = difficultyType.String("name")
				}
			}
		}
	}
	if events, catalogErr := store.Catalog("events"); catalogErr == nil {
		if raw, eventFound := events.Find(strconv.FormatInt(eventID, 10)); eventFound {
			if event, decodeErr := DecodeRecord(raw); decodeErr == nil {
				definition.EventType, _ = event.String("eventType")
				definition.Name, _ = event.String("comment1")
			}
		}
	}
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.Name == "" {
		definition.Name = strings.TrimSpace(definition.EventType)
	}
	return definition, true
}
