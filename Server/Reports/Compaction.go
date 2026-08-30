package Reports

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"CitadelDesktop/Server/History"
)

type BattleHistoryCompaction struct {
	Kept       int
	Discarded  int
	Duplicates int
}

func IsPvPBattleReport(report BattleReport) bool {
	return realBattlePlayer(report.Attacker) && realBattlePlayer(report.Defender)
}

func realBattlePlayer(combatant *BattleCombatant) bool {
	return combatant != nil && combatant.PlayerID > 0 && !combatant.Dummy
}

func CompactBattleHistory(history *History.Store) (BattleHistoryCompaction, error) {
	result := BattleHistoryCompaction{}
	if history == nil {
		return result, nil
	}
	rows, err := history.Read(History.CollectionBattleReports, time.Time{}, battleReportAnalyticsLimit)
	if err != nil {
		return result, fmt.Errorf("read battle history for PvP compaction: %w", err)
	}
	if len(rows) >= battleReportAnalyticsLimit {
		return result, fmt.Errorf("battle history reached the %d-row safe compaction limit", battleReportAnalyticsLimit)
	}

	kept := make([]json.RawMessage, 0, len(rows))
	byLID := make(map[int64]int)
	byID := make(map[string]int)
	for _, row := range rows {
		var report BattleReport
		if json.Unmarshal(row, &report) != nil || !IsPvPBattleReport(report) {
			result.Discarded++
			continue
		}
		index := -1
		if report.LID > 0 {
			index = byLID[report.LID] - 1
		} else if id := strings.TrimSpace(report.ID); id != "" {
			index = byID[id] - 1
		}
		if index >= 0 {
			kept[index] = append(json.RawMessage(nil), row...)
			result.Duplicates++
			continue
		}
		kept = append(kept, append(json.RawMessage(nil), row...))
		position := len(kept)
		if report.LID > 0 {
			byLID[report.LID] = position
		} else if id := strings.TrimSpace(report.ID); id != "" {
			byID[id] = position
		}
	}
	result.Kept = len(kept)
	if result.Discarded == 0 && result.Duplicates == 0 {
		return result, nil
	}
	if err := history.Replace(History.CollectionBattleReports, kept); err != nil {
		return BattleHistoryCompaction{}, fmt.Errorf("compact battle history to PvP reports: %w", err)
	}
	return result, nil
}

func PurgeBattleHistoryByLIDs(history *History.Store, lids []int64) (int, error) {
	if history == nil || len(lids) == 0 {
		return 0, nil
	}
	confirmed := make(map[int64]struct{}, len(lids))
	for _, lid := range lids {
		if lid > 0 {
			confirmed[lid] = struct{}{}
		}
	}
	if len(confirmed) == 0 {
		return 0, nil
	}
	removed, err := history.RemoveWhere(History.CollectionBattleReports, func(row json.RawMessage) bool {
		var report BattleReport
		if json.Unmarshal(row, &report) != nil {
			return false
		}
		_, exists := confirmed[report.LID]
		return exists
	})
	if err != nil {
		return 0, fmt.Errorf("purge cloud-confirmed PvP battle history: %w", err)
	}
	return removed, nil
}
