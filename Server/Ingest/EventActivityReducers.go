package Ingest

import (
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/State"
)

const eventReportLaunchMatchWindow = 45 * time.Minute

type eventBattleSummary struct {
	MessageID    wireInt64           `json:"MID"`
	ReportID     wireInt64           `json:"LID"`
	Participants [][]json.RawMessage `json:"PBI"`
	Target       struct {
		TypeID    int       `json:"AT"`
		KingdomID wireInt64 `json:"K"`
		X         int       `json:"X"`
		Y         int       `json:"Y"`
	} `json:"AI"`
}

func reconcileEventBattleActivity(gameState *State.GameState, capture *State.BattleReportCapture) (bool, error) {
	if gameState == nil || capture == nil || gameState.Player.ID <= 0 || len(capture.Summary) == 0 || len(capture.Details) == 0 {
		return false, nil
	}
	var summary eventBattleSummary
	if err := json.Unmarshal(capture.Summary, &summary); err != nil {
		return false, fmt.Errorf("decode event battle activity summary: %w", err)
	}
	reportID := int64(summary.ReportID)
	if reportID <= 0 {
		reportID = capture.ReportID
	}
	if reportID <= 0 {
		reportID = int64(summary.MessageID)
	}
	if reportID <= 0 {
		reportID = capture.MessageID
	}
	if reportID <= 0 {
		return false, nil
	}
	role := ""
	if battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) {
		role = "attacker"
	} else if battleSummaryHasOwnDefender(summary.Participants, gameState.Player.ID) {
		role = "defender"
	}
	if role == "" {
		return false, nil
	}
	observedAt := capture.CapturedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	eventID, pendingIndex, record, found := matchingEventAttack(
		gameState, State.KingdomID(summary.Target.KingdomID), summary.Target.TypeID,
		summary.Target.X, summary.Target.Y, role, observedAt,
	)
	if !found {
		return false, nil
	}
	activity := gameState.EventScores.ActivityByEvent[eventID]
	for _, processed := range activity.ProcessedReportIDs {
		if processed == reportID {
			return false, nil
		}
	}
	activity.PendingAttacks = append(activity.PendingAttacks[:pendingIndex], activity.PendingAttacks[pendingIndex+1:]...)
	activity.ProcessedReportIDs = append(activity.ProcessedReportIDs, reportID)
	if len(activity.ProcessedReportIDs) > 4_096 {
		activity.ProcessedReportIDs = append([]int64(nil), activity.ProcessedReportIDs[len(activity.ProcessedReportIDs)-4_096:]...)
	}
	totals := State.EventCombatTotalsFor(&activity, record.Kind)
	if totals == nil {
		return false, nil
	}
	featureID := State.EventActivityFeature(record.Kind)
	if featureID == "" {
		return false, nil
	}
	capture.AutomationFeature = featureID
	capture.MovementID = record.MovementID
	capture.EventID = eventID
	capture.EventActivity = record.Kind
	capture.EventOccurrenceEndsAt = activity.OccurrenceEndsAt
	if capture.ToolsUsed == 0 {
		capture.ToolsUsed = ownBattleToolsUsed(capture.Details, gameState.Player.ID)
	}
	advisorAlreadyObserved := record.Kind == State.EventActivityAdvisor &&
		!activity.AdvisorObservedAt.IsZero() && !activity.AdvisorObservedAt.Before(record.ArrivesAt)
	if !advisorAlreadyObserved {
		totals.Battles++
		won := battleSummaryAttackerWon(summary.Participants)
		lost := battleSummaryDefenderWon(summary.Participants)
		if role == "defender" {
			won, lost = lost, won
		}
		if won {
			totals.Victories++
		} else if lost {
			totals.Defeats++
		}
		totals.TroopLosses += ownBattleTroopLosses(summary.Participants, gameState.Player.ID)
		totals.ToolsUsed += capture.ToolsUsed
		totals.Loot += ownBattleLoot(summary.Participants, gameState.Player.ID)
	}
	gameState.EventScores.ActivityByEvent[eventID] = activity
	return true, nil
}

func matchingEventAttack(
	gameState *State.GameState,
	kingdomID State.KingdomID,
	targetTypeID, targetX, targetY int,
	role string,
	observedAt time.Time,
) (int64, int, State.EventAttackRecord, bool) {
	bestEventID, bestIndex := int64(0), -1
	bestDistance := eventReportLaunchMatchWindow + time.Second
	var best State.EventAttackRecord
	for eventID, activity := range gameState.EventScores.ActivityByEvent {
		for index, record := range activity.PendingAttacks {
			defense := record.Kind == State.EventActivityKhanDefense
			if defense != (role == "defender") || record.KingdomID != kingdomID ||
				record.TargetX != targetX || record.TargetY != targetY {
				continue
			}
			if record.TargetTypeID > 0 && targetTypeID > 0 && record.TargetTypeID != targetTypeID {
				continue
			}
			impactAt := record.ArrivesAt
			if impactAt.IsZero() {
				impactAt = record.LaunchedAt
			}
			distance := observedAt.Sub(impactAt)
			if distance < 0 {
				distance = -distance
			}
			if distance > eventReportLaunchMatchWindow || distance >= bestDistance {
				continue
			}
			bestEventID, bestIndex, best, bestDistance = eventID, index, record, distance
		}
	}
	return bestEventID, bestIndex, best, bestIndex >= 0
}

func battleSummaryHasOwnDefender(participants [][]json.RawMessage, playerID State.PlayerID) bool {
	for _, participant := range participants {
		if len(participant) >= 2 && State.PlayerID(rowInt(participant, 0)) == playerID && rowInt(participant, 1) == 1 {
			return true
		}
	}
	return false
}

func battleSummaryDefenderWon(participants [][]json.RawMessage) bool {
	attackerPresent, defenderPresent := false, false
	var attackerSurvivors, defenderSurvivors int64
	for _, participant := range participants {
		if len(participant) < 4 {
			continue
		}
		survivors := max(int64(0), rowInt(participant, 2)+rowInt(participant, 3))
		switch rowInt(participant, 1) {
		case 0:
			attackerPresent = true
			attackerSurvivors += survivors
		case 1:
			defenderPresent = true
			defenderSurvivors += survivors
		}
	}
	return attackerPresent && defenderPresent && attackerSurvivors == 0 && defenderSurvivors > 0
}

func ownBattleTroopLosses(participants [][]json.RawMessage, playerID State.PlayerID) int64 {
	var losses int64
	for _, participant := range participants {
		if len(participant) < 4 || State.PlayerID(rowInt(participant, 0)) != playerID {
			continue
		}
		lost := rowInt(participant, 3)
		if lost < 0 {
			lost = -lost
		}
		losses += lost
	}
	return losses
}

func ownBattleLoot(participants [][]json.RawMessage, playerID State.PlayerID) int64 {
	var loot int64
	for _, participant := range participants {
		if len(participant) < 5 || State.PlayerID(rowInt(participant, 0)) != playerID || rowInt(participant, 1) != 0 {
			continue
		}
		var resources [][]json.RawMessage
		if json.Unmarshal(participant[4], &resources) != nil {
			continue
		}
		for _, resource := range resources {
			amount := rowInt(resource, 1)
			if len(resource) >= 2 && rowString(resource, 0) != "" && amount > 0 {
				loot += amount
			}
		}
	}
	return loot
}

func ownBattleToolsUsed(raw json.RawMessage, playerID State.PlayerID) int64 {
	var payload struct {
		Waves   []json.RawMessage   `json:"W"`
		Support [][]json.RawMessage `json:"S"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return 0
	}
	var used int64
	for _, rawWave := range payload.Waves {
		var participants []json.RawMessage
		if json.Unmarshal(rawWave, &participants) != nil {
			continue
		}
		for _, rawParticipant := range participants {
			var participant []json.RawMessage
			if json.Unmarshal(rawParticipant, &participant) != nil || len(participant) < 2 ||
				State.PlayerID(rowInt(participant, 0)) != playerID {
				continue
			}
			for _, rawLane := range participant[1:] {
				var lane []json.RawMessage
				if json.Unmarshal(rawLane, &lane) != nil || len(lane) < 2 {
					continue
				}
				used += usedToolRows(lane[1])
			}
		}
	}
	for _, row := range payload.Support {
		if len(row) < 2 || State.PlayerID(rowInt(row, 0)) != playerID {
			continue
		}
		for _, rawTool := range row[1:] {
			used += usedToolRow(rawTool)
		}
	}
	return used
}

func usedToolRows(raw json.RawMessage) int64 {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return 0
	}
	var used int64
	for _, row := range rows {
		used += usedToolRow(row)
	}
	return used
}

func usedToolRow(raw json.RawMessage) int64 {
	var row []json.RawMessage
	if json.Unmarshal(raw, &row) != nil || len(row) < 3 {
		return 0
	}
	value := rowInt(row, 2)
	if value < 0 {
		return -value
	}
	return 0
}
