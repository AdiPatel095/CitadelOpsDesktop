package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	advisorNomadEventID   = 72
	advisorSamuraiEventID = 80
	advisorNomadTypeID    = 27
	advisorSamuraiTypeID  = 29
)

type advisorUnitMovement struct {
	AdvisorType  int64 `json:"AAT"`
	AttackNumber int   `json:"AAN"`
	AttackCount  int   `json:"AAC"`
	LaunchState  int   `json:"AAL"`
	Leader       struct {
		ID State.CommanderID `json:"ID"`
	} `json:"L"`
}

type advisorMovement struct {
	MovementID State.MovementID  `json:"MID"`
	Direction  int               `json:"D"`
	KingdomID  State.KingdomID   `json:"KID"`
	Source     []json.RawMessage `json:"SA"`
	Target     []json.RawMessage `json:"TA"`
}

type advisorMovementEnvelope struct {
	Movement advisorMovement     `json:"M"`
	Units    advisorUnitMovement `json:"UM"`
}

func reduceAdvisorActivation(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		AdvisorType int `json:"AAT"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode advisor activation: %w", err)
	}
	if payload.AdvisorType != 1 {
		return nil, false, nil
	}
	eventID, found := activeAdvisorEvent(*gameState)
	if !found {
		return nil, false, nil
	}
	score := gameState.EventScores.ByEvent[eventID]
	if score.AdvisorActive {
		return nil, false, nil
	}
	score.AdvisorActive = true
	if score.AdvisorCurrencyID == 0 {
		score.AdvisorCurrencyID = advisorTokenCurrency(eventID)
	}
	gameState.EventScores.ByEvent[eventID] = score
	return []string{"advisor", "events", "event-scores"}, true, nil
}

func reduceAdvisorOverview(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		AdvisorType int64             `json:"AAT"`
		Count       int               `json:"C"`
		Gains       []json.RawMessage `json:"G"`
		Costs       []json.RawMessage `json:"L"`
		UnitsLost   int64             `json:"LU"`
		ToolsLost   int64             `json:"LT"`
		Wins        int64             `json:"W"`
		Defeats     int64             `json:"D"`
		Pending     int64             `json:"P"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode advisor overview: %w", err)
	}
	if payload.AdvisorType != 1 {
		return nil, false, nil
	}
	summary := State.AdvisorSummaryState{
		AdvisorType: int(payload.AdvisorType), Count: payload.Count,
		Gains: advisorResourceRows(payload.Gains), Costs: advisorResourceRows(payload.Costs),
		UnitsLost: payload.UnitsLost, ToolsLost: payload.ToolsLost,
		Wins: payload.Wins, Defeats: payload.Defeats, PendingAttacks: payload.Pending,
		ObservedAt: frame.ReceivedAt.UTC(),
	}
	if reflect.DeepEqual(gameState.Advisor.Summary, summary) {
		return nil, false, nil
	}
	gameState.Advisor.Summary = summary
	if eventID, found := activeAdvisorEvent(*gameState); found {
		currentAttack := int64(0)
		if gameState.Advisor.Run != nil && gameState.Advisor.Run.EventID == eventID {
			currentAttack = int64(gameState.Advisor.Run.CurrentAttack)
		}
		State.UpdateAdvisorEventActivity(
			gameState, eventID, frame.ReceivedAt, currentAttack, summary.Wins, summary.Defeats,
			summary.UnitsLost, summary.ToolsLost,
		)
	}
	return []string{"advisor", "event-scores"}, true, nil
}

func reduceAdvisorMovement(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	envelopes, err := advisorMovementEnvelopes(frame.Payload)
	if err != nil {
		return nil, false, fmt.Errorf("decode advisor movement: %w", err)
	}
	changed := false
	for _, envelope := range envelopes {
		if envelope.Units.AdvisorType != 1 || envelope.Units.AttackCount < 1 || envelope.Units.AttackNumber < 1 {
			continue
		}
		if applyAdvisorMovement(frame.Opcode, frame.ReceivedAt, envelope, gameState) {
			changed = true
		}
	}
	if !changed {
		return nil, false, nil
	}
	return []string{"advisor", "event-scores"}, true, nil
}

func advisorMovementEnvelopes(raw json.RawMessage) ([]advisorMovementEnvelope, error) {
	var payload struct {
		Attack    *advisorMovementEnvelope  `json:"AAM"`
		Movement  *advisorMovementEnvelope  `json:"A"`
		Movements []advisorMovementEnvelope `json:"M"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	result := make([]advisorMovementEnvelope, 0, len(payload.Movements)+2)
	if payload.Attack != nil {
		result = append(result, *payload.Attack)
	}
	if payload.Movement != nil {
		result = append(result, *payload.Movement)
	}
	result = append(result, payload.Movements...)
	return result, nil
}

func applyAdvisorMovement(opcode string, observedAt time.Time, envelope advisorMovementEnvelope, gameState *State.GameState) bool {
	sourceRow, targetRow := envelope.Movement.Source, envelope.Movement.Target
	if envelope.Movement.Direction == 1 {
		sourceRow, targetRow = targetRow, sourceRow
	}
	targetTypeID := int(advisorRowInt(targetRow, 0))
	eventID := advisorEventForTarget(targetTypeID)
	if eventID == 0 {
		eventID, _ = activeAdvisorEvent(*gameState)
	}
	if eventID == 0 {
		return false
	}

	next := State.AdvisorRunState{
		EventID: eventID, EventEndsAt: advisorEventEndsAt(*gameState, eventID),
		SourceCastleID: State.CastleID(advisorRowInt(sourceRow, 3)),
		KingdomID:      envelope.Movement.KingdomID, TargetTypeID: targetTypeID,
		TargetX: int(advisorRowInt(targetRow, 1)), TargetY: int(advisorRowInt(targetRow, 2)),
		CommanderID: envelope.Units.Leader.ID, MovementID: envelope.Movement.MovementID,
		RequestedAttacks: envelope.Units.AttackCount, CurrentAttack: envelope.Units.AttackNumber,
		LaunchState: envelope.Units.LaunchState, Status: "running",
		StartedAt: observedAt.UTC(), LastAttackAt: observedAt.UTC(), UpdatedAt: observedAt.UTC(),
	}
	if envelope.Units.LaunchState != 0 {
		next.Status = "cancelled"
	} else if envelope.Movement.Direction == 1 && envelope.Units.AttackNumber >= envelope.Units.AttackCount {
		next.Status = "completed"
	}
	previous := gameState.Advisor.Run
	if previous != nil && advisorSameRunOccurrence(*previous, next) {
		next.StartedAt = previous.StartedAt
		if next.StartedAt.IsZero() {
			next.StartedAt = observedAt.UTC()
		}
		if opcode != "cra" || previous.CurrentAttack == next.CurrentAttack {
			next.LastAttackAt = previous.LastAttackAt
		}
		if previous.Status == "cancelled" && opcode != "cra" {
			next.Status = previous.Status
			next.LaunchState = previous.LaunchState
			next.CurrentAttack = max(previous.CurrentAttack, next.CurrentAttack)
		}
	}
	if previous != nil && advisorRunsEqual(*previous, next) {
		return false
	}
	gameState.Advisor.Run = &next
	if opcode == "cra" && envelope.Movement.Direction == 0 && envelope.Movement.MovementID > 0 {
		record := State.EventAttackRecord{
			MovementID: envelope.Movement.MovementID, Kind: State.EventActivityAdvisor,
			KingdomID: envelope.Movement.KingdomID, TargetTypeID: targetTypeID,
			TargetX: next.TargetX, TargetY: next.TargetY, LaunchedAt: observedAt.UTC(),
		}
		if movement, found := gameState.Movements[envelope.Movement.MovementID]; found && movement.ArrivesAt != nil {
			record.ArrivesAt = movement.ArrivesAt.UTC()
		}
		State.RecordEventAttackLaunch(gameState, eventID, record)
		activity := gameState.EventScores.ActivityByEvent[eventID]
		activity.Advisor.Launches = max(activity.Advisor.Launches, int64(next.CurrentAttack))
		gameState.EventScores.ActivityByEvent[eventID] = activity
	}
	return true
}

func advisorSameRunOccurrence(left State.AdvisorRunState, right State.AdvisorRunState) bool {
	if left.EventID != right.EventID || left.RequestedAttacks != right.RequestedAttacks {
		return false
	}
	if left.EventEndsAt.IsZero() || right.EventEndsAt.IsZero() {
		return true
	}
	delta := left.EventEndsAt.Sub(right.EventEndsAt)
	return delta >= -10*time.Minute && delta <= 10*time.Minute
}

func advisorEventEndsAt(gameState State.GameState, eventID int64) time.Time {
	score, found := gameState.EventScores.ByEvent[eventID]
	if !found || score.ObservedAt.IsZero() || score.RemainingSec <= 0 {
		return time.Time{}
	}
	return score.ObservedAt.Add(time.Duration(score.RemainingSec) * time.Second).UTC()
}

func advisorRunsEqual(left State.AdvisorRunState, right State.AdvisorRunState) bool {
	left.UpdatedAt, right.UpdatedAt = time.Time{}, time.Time{}
	return left == right
}

func advisorResourceRows(rows []json.RawMessage) map[string]int64 {
	result := map[string]int64{}
	for _, raw := range rows {
		var row []json.RawMessage
		if json.Unmarshal(raw, &row) != nil || len(row) < 2 {
			continue
		}
		var key string
		if json.Unmarshal(row[0], &key) != nil || key == "" {
			continue
		}
		if amount, ok := rawInt64(row[1]); ok {
			result[key] = amount
		}
	}
	return result
}

func advisorRowInt(row []json.RawMessage, index int) int64 {
	if index < 0 || index >= len(row) {
		return 0
	}
	value, _ := rawInt64(row[index])
	return value
}

func advisorEventForTarget(targetTypeID int) int64 {
	switch targetTypeID {
	case advisorNomadTypeID:
		return advisorNomadEventID
	case advisorSamuraiTypeID:
		return advisorSamuraiEventID
	default:
		return 0
	}
}

func activeAdvisorEvent(gameState State.GameState) (int64, bool) {
	if eventID := gameState.EventScores.ActiveEventID; eventID == advisorNomadEventID || eventID == advisorSamuraiEventID {
		if _, found := gameState.EventScores.ByEvent[eventID]; found {
			return eventID, true
		}
	}
	selected := int64(0)
	var observedAt time.Time
	for _, eventID := range []int64{advisorNomadEventID, advisorSamuraiEventID} {
		score, found := gameState.EventScores.ByEvent[eventID]
		if found && score.RemainingSec > 0 && (selected == 0 || score.ObservedAt.After(observedAt)) {
			selected, observedAt = eventID, score.ObservedAt
		}
	}
	return selected, selected != 0
}

func advisorTokenCurrency(eventID int64) State.CurrencyID {
	if eventID == advisorNomadEventID {
		return 77
	}
	if eventID == advisorSamuraiEventID {
		return 78
	}
	return 0
}
