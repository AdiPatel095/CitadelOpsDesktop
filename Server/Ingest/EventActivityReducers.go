package Ingest

import (
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/State"
)

const (
	eventReportLaunchMatchWindow = 45 * time.Minute
	eventReportPreImpactSkew     = 5 * time.Second
)

type eventBattleSummary struct {
	MessageID    wireInt64           `json:"MID"`
	ReportID     wireInt64           `json:"LID"`
	Participants [][]json.RawMessage `json:"PBI"`
	Target       struct {
		TypeID    int             `json:"AT"`
		KingdomID State.KingdomID `json:"K"`
		X         int             `json:"X"`
		Y         int             `json:"Y"`
	} `json:"AI"`
	targetIdentityKnown bool
}

func (summary *eventBattleSummary) UnmarshalJSON(raw []byte) error {
	var wire struct {
		MessageID    wireInt64           `json:"MID"`
		ReportID     wireInt64           `json:"LID"`
		Participants [][]json.RawMessage `json:"PBI"`
		Target       struct {
			TypeID    json.RawMessage `json:"AT"`
			KingdomID json.RawMessage `json:"K"`
			X         json.RawMessage `json:"X"`
			Y         json.RawMessage `json:"Y"`
		} `json:"AI"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	summary.MessageID, summary.ReportID, summary.Participants = wire.MessageID, wire.ReportID, wire.Participants
	kingdomID, kingdomKnown := rawJSONInt64(wire.Target.KingdomID)
	typeID, typeKnown := rawJSONInt64(wire.Target.TypeID)
	x, xKnown := rawJSONInt64(wire.Target.X)
	y, yKnown := rawJSONInt64(wire.Target.Y)
	if !kingdomKnown || kingdomID < 0 || !typeKnown || typeID < 0 || !xKnown || x < 0 || !yKnown || y < 0 ||
		int64(int(typeID)) != typeID || int64(int(x)) != x || int64(int(y)) != y {
		return nil
	}
	summary.Target.TypeID = int(typeID)
	summary.Target.KingdomID = State.KingdomID(kingdomID)
	summary.Target.X, summary.Target.Y = int(x), int(y)
	summary.targetIdentityKnown = true
	return nil
}

func eventBattleTargetIdentity(summary eventBattleSummary) (State.KingdomID, int, int, int, bool) {
	if !summary.targetIdentityKnown {
		return 0, 0, 0, 0, false
	}
	return summary.Target.KingdomID, summary.Target.TypeID, summary.Target.X, summary.Target.Y, true
}

func battleParticipantIdentity(participant []json.RawMessage) (State.PlayerID, int, bool) {
	if len(participant) < 2 {
		return 0, 0, false
	}
	playerID, playerKnown := rawJSONInt64(participant[0])
	role, roleKnown := rawJSONInt64(participant[1])
	if !playerKnown || !roleKnown || role < 0 || role > 1 {
		return 0, 0, false
	}
	return State.PlayerID(playerID), int(role), true
}

func battleParticipantPlayerID(participant []json.RawMessage) (State.PlayerID, bool) {
	if len(participant) == 0 {
		return 0, false
	}
	playerID, known := rawJSONInt64(participant[0])
	return State.PlayerID(playerID), known
}

func reconcileEventBattleActivity(gameState *State.GameState, capture *State.BattleReportCapture) (bool, error) {
	return reconcileEventBattleActivityForMovement(gameState, capture, 0)
}

func reconcileEventBattleActivityForMovement(
	gameState *State.GameState,
	capture *State.BattleReportCapture,
	movementID State.MovementID,
) (bool, error) {
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
	kingdomID, targetTypeID, targetX, targetY, targetKnown := eventBattleTargetIdentity(summary)
	if !targetKnown {
		return false, nil
	}
	observedAt := battleCaptureOccurredAt(gameState, *capture)
	eventID, pendingIndex, record, found := matchingEventAttack(
		gameState, kingdomID, targetTypeID, targetX, targetY, role, observedAt, movementID,
	)
	if !found {
		return false, nil
	}
	activity, _ := gameState.MutableEventActivity(eventID)
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
	gameState.SetEventActivity(eventID, activity)
	return true, nil
}

// ReconcileRetainedBattleCapturesForRecoveredLaunch replays attribution only
// for already-retained, complete reports that can match the newly recovered
// movement. Battle-detail reducers intentionally ignore identical payloads, so
// without this pass a report received before launch recovery would remain
// permanently unattributed.
func ReconcileRetainedBattleCapturesForRecoveredLaunch(
	gameState *State.GameState,
	movementID State.MovementID,
) (bool, error) {
	if gameState == nil || movementID == 0 {
		return false, nil
	}
	var launch State.EventAttackRecord
	foundLaunch := false
	gameState.RangeEventActivities(func(_ int64, activity State.EventActivityState) bool {
		for _, candidate := range activity.PendingAttacks {
			if candidate.MovementID == movementID {
				launch, foundLaunch = candidate, true
				return false
			}
		}
		return true
	})
	if !foundLaunch {
		return false, nil
	}
	var selected State.BattleReportCapture
	var selectedDistance time.Duration
	gameState.RangeBattleReportCaptures(func(_ int64, capture State.BattleReportCapture) bool {
		if len(capture.Summary) == 0 || len(capture.Details) == 0 || capture.MovementID != 0 {
			return true
		}
		var summary eventBattleSummary
		if json.Unmarshal(capture.Summary, &summary) != nil ||
			!battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) ||
			!eventBattleTargetMatches(summary, launch.KingdomID, launch.TargetTypeID, launch.TargetX, launch.TargetY) {
			return true
		}
		impactAt := launch.ArrivesAt
		if impactAt.IsZero() {
			impactAt = launch.LaunchedAt
		}
		occurredAt := battleCaptureOccurredAt(gameState, capture)
		delta := occurredAt.Sub(impactAt)
		if delta < -eventReportPreImpactSkew || delta > eventReportLaunchMatchWindow {
			return true
		}
		distance := delta
		if distance < 0 {
			distance = -distance
		}
		if selected.MessageID == 0 ||
			distance < selectedDistance || distance == selectedDistance && capture.MessageID < selected.MessageID {
			selected, selectedDistance = capture, distance
		}
		return true
	})
	if selected.MessageID == 0 {
		return false, nil
	}
	analyticsChanged, err := reconcileAttackFeatureBattleReportForMovement(gameState, &selected, movementID)
	if err != nil {
		return false, err
	}
	eventChanged, err := reconcileEventBattleActivityForMovement(gameState, &selected, movementID)
	if err != nil {
		return false, err
	}
	if !analyticsChanged && !eventChanged {
		return false, nil
	}
	gameState.SetBattleReportCapture(selected.MessageID, selected)
	return true, nil
}

// InvasionReservationReportCandidate reports whether one complete,
// unattributed own-attacker report might belong to a durable invasion
// reservation. It is only a temporary archival hold signal; source and
// commander identity are unavailable here, so it must never prove or account
// a launch by itself. Every plausible report is held because the movement's
// eventual impact time is not known until exact recovery.
func InvasionReservationReportCandidate(
	gameState State.GameState,
	reservation State.InvasionTargetReservation,
	capture State.BattleReportCapture,
) bool {
	if gameState.Player.ID <= 0 || !reservation.CommanderKnown || reservation.EventID <= 0 || reservation.OccurrenceEndsAt.IsZero() ||
		reservation.ReservedAt.IsZero() || reservation.TargetTypeID <= 0 ||
		reservation.SourceCastleID <= 0 || !reservation.SourceKnown || reservation.OperationID == "" ||
		capture.MessageID <= 0 || capture.MovementID != 0 || capture.AutomationFeature != "" ||
		len(capture.Summary) == 0 || len(capture.Details) == 0 ||
		!capture.EventOccurrenceEndsAt.IsZero() &&
			!State.SameEventOccurrence(capture.EventOccurrenceEndsAt, reservation.OccurrenceEndsAt) {
		return false
	}
	var summary eventBattleSummary
	if json.Unmarshal(capture.Summary, &summary) != nil ||
		!battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) ||
		!eventBattleTargetMatches(summary, reservation.KingdomID, reservation.TargetTypeID, reservation.X, reservation.Y) {
		return false
	}
	occurredAt := battleCaptureOccurredAt(&gameState, capture)
	return !occurredAt.Before(reservation.ReservedAt.Add(-2*time.Second)) &&
		!occurredAt.After(reservation.OccurrenceEndsAt.Add(eventReportLaunchMatchWindow))
}

func eventBattleTargetMatches(
	summary eventBattleSummary,
	kingdomID State.KingdomID,
	targetTypeID int,
	x int,
	y int,
) bool {
	observedKingdom, observedType, observedX, observedY, valid := eventBattleTargetIdentity(summary)
	return valid && observedKingdom == kingdomID && observedX == x && observedY == y &&
		(targetTypeID <= 0 || observedType == targetTypeID)
}

func matchingEventAttack(
	gameState *State.GameState,
	kingdomID State.KingdomID,
	targetTypeID, targetX, targetY int,
	role string,
	observedAt time.Time,
	movementID State.MovementID,
) (int64, int, State.EventAttackRecord, bool) {
	bestEventID, bestIndex := int64(0), -1
	bestDistance := eventReportLaunchMatchWindow + time.Second
	var best State.EventAttackRecord
	gameState.RangeEventActivities(func(eventID int64, activity State.EventActivityState) bool {
		for index, record := range activity.PendingAttacks {
			defense := record.Kind == State.EventActivityKhanDefense
			if movementID != 0 && record.MovementID != movementID {
				continue
			}
			if defense != (role == "defender") || record.KingdomID != kingdomID ||
				record.TargetX != targetX || record.TargetY != targetY {
				continue
			}
			if record.TargetTypeID > 0 && record.TargetTypeID != targetTypeID {
				continue
			}
			impactAt := record.ArrivesAt
			if impactAt.IsZero() {
				impactAt = record.LaunchedAt
			}
			delta := observedAt.Sub(impactAt)
			if delta < -eventReportPreImpactSkew || delta > eventReportLaunchMatchWindow {
				continue
			}
			distance := delta
			if distance < 0 {
				distance = -distance
			}
			if distance >= bestDistance {
				continue
			}
			bestEventID, bestIndex, best, bestDistance = eventID, index, record, distance
		}
		return true
	})
	return bestEventID, bestIndex, best, bestIndex >= 0
}

func battleSummaryHasOwnDefender(participants [][]json.RawMessage, playerID State.PlayerID) bool {
	for _, participant := range participants {
		observedPlayerID, role, valid := battleParticipantIdentity(participant)
		if valid && observedPlayerID == playerID && role == 1 {
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
		_, role, valid := battleParticipantIdentity(participant)
		if !valid {
			continue
		}
		survivors := max(int64(0), rowInt(participant, 2)+rowInt(participant, 3))
		switch role {
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
		observedPlayerID, _, valid := battleParticipantIdentity(participant)
		if len(participant) < 4 || !valid || observedPlayerID != playerID {
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
		observedPlayerID, role, valid := battleParticipantIdentity(participant)
		if len(participant) < 5 || !valid || observedPlayerID != playerID || role != 0 {
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
			if json.Unmarshal(rawParticipant, &participant) != nil {
				continue
			}
			observedPlayerID, valid := battleParticipantPlayerID(participant)
			if !valid || observedPlayerID != playerID {
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
		observedPlayerID, valid := battleParticipantPlayerID(row)
		if !valid || observedPlayerID != playerID {
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
