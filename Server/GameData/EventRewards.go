package GameData

import (
	"strconv"
	"strings"
)

type EventRewardProgress struct {
	Reached   int
	Total     int
	NextScore int64
}

func (store *Store) ScalableEventRewardProgress(eventID, difficultyID, leagueID, rewardSetID, score int64) EventRewardProgress {
	if store == nil || eventID <= 0 || difficultyID <= 0 || leagueID <= 0 {
		return EventRewardProgress{}
	}
	catalog, err := store.Catalog("leaguetypeevents")
	if err != nil {
		return EventRewardProgress{}
	}
	var fallback Record
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil || recordInt(record, "eventID") != eventID || recordInt(record, "leaguetypeID") != leagueID {
			continue
		}
		if subtype := recordInt(record, "subType"); subtype != 0 {
			continue
		}
		if fallback == nil {
			fallback = record
		}
		if rewardSetID <= 0 || recordInt(record, "rewardSetID") == rewardSetID {
			fallback = record
			break
		}
	}
	if fallback == nil {
		return EventRewardProgress{}
	}
	thresholds := eventIntList(fallback, "difficultyScalingNeededPointsForRewards")
	difficulties := eventIntList(fallback, "difficultyIDforMaxPoints")
	maximums := eventIntList(fallback, "difficultyMaxPoints")
	maximum := int64(0)
	for index, candidate := range difficulties {
		if candidate == difficultyID && index < len(maximums) {
			maximum = maximums[index]
			break
		}
	}
	if len(thresholds) == 0 || maximum <= 0 {
		return EventRewardProgress{}
	}
	progress := EventRewardProgress{}
	for _, threshold := range thresholds {
		if threshold <= 0 || threshold > maximum {
			continue
		}
		progress.Total++
		if score >= threshold {
			progress.Reached++
			continue
		}
		if progress.NextScore == 0 {
			progress.NextScore = threshold
		}
	}
	return progress
}

func eventIntList(record Record, field string) []int64 {
	value, ok := record.String(field)
	if !ok {
		return nil
	}
	parts := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == '#'
	})
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil {
			result = append(result, value)
		}
	}
	return result
}

func recordInt(record Record, field string) int64 {
	value, _ := record.Int64(field)
	return value
}
