package Ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceSuccessfulNomadCampBattle(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 || gameState.Player.ID <= 0 {
		return nil, false, nil
	}
	var summary struct {
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
	if err := json.Unmarshal(frame.Payload, &summary); err != nil {
		return nil, false, fmt.Errorf("decode Nomad/Samurai camp battle summary: %w", err)
	}
	if !isRegularEventCampType(summary.Target.TypeID) || !battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) ||
		!battleSummaryAttackerWon(summary.Participants) {
		return nil, false, nil
	}
	reportID := int64(summary.ReportID)
	if reportID <= 0 {
		reportID = int64(summary.MessageID)
	}
	kingdomID := State.KingdomID(summary.Target.KingdomID)
	key := nomadCampCooldownKey(kingdomID, summary.Target.X, summary.Target.Y)
	if gameState.NomadCamps.Cooldowns == nil {
		gameState.NomadCamps.Cooldowns = map[string]State.NomadCampCooldownState{}
	}
	if current, exists := gameState.NomadCamps.Cooldowns[key]; exists && current.ReportID == reportID && reportID > 0 {
		return nil, false, nil
	}
	gameState.NomadCamps.Cooldowns[key] = State.NomadCampCooldownState{
		KingdomID: kingdomID, X: summary.Target.X, Y: summary.Target.Y, ReportID: reportID,
		LastSuccessfulBattleAt: frame.ReceivedAt, PendingCooldownRefresh: true,
	}
	return []string{"nomad-camps"}, true, nil
}

func refreshNomadCampCooldownFromMap(gameState *State.GameState, observation State.MapObservation) bool {
	if !isRegularEventCampType(observation.TypeID) && observation.TypeID != khanCampMapTypeID ||
		gameState.NomadCamps.Cooldowns == nil {
		return false
	}
	reportChanged := refreshKhanCooldownReportsFromMap(gameState, observation)
	key := nomadCampCooldownKey(observation.KingdomID, observation.X, observation.Y)
	current, exists := gameState.NomadCamps.Cooldowns[key]
	if !exists || current.LastSuccessfulBattleAt.After(observation.ObservedAt) {
		return reportChanged
	}
	next := current
	next.CooldownRemaining = observation.EventCampCooldownRemaining
	next.CooldownObservedAt = observation.ObservedAt
	next.PendingCooldownRefresh = false
	if next == current {
		return reportChanged
	}
	gameState.NomadCamps.Cooldowns[key] = next
	return true
}

func refreshKhanCooldownReportsFromMap(gameState *State.GameState, observation State.MapObservation) bool {
	if gameState == nil || observation.TypeID != khanCampMapTypeID ||
		len(gameState.Khan.CooldownReports) == 0 {
		return false
	}
	changed := false
	for reportID, current := range gameState.Khan.CooldownReports {
		if !current.ResolvedAt.IsZero() || current.KingdomID != observation.KingdomID ||
			current.X != observation.X || current.Y != observation.Y ||
			current.LandedAt.After(observation.ObservedAt) {
			continue
		}
		next := current
		next.CooldownRemaining = observation.EventCampCooldownRemaining
		next.CooldownObservedAt = observation.ObservedAt.UTC()
		if next.CooldownRemaining == current.CooldownRemaining &&
			next.CooldownObservedAt.Equal(current.CooldownObservedAt) {
			continue
		}
		gameState.Khan.CooldownReports[reportID] = next
		changed = true
	}
	return changed
}

func nomadCampCooldownKey(kingdomID State.KingdomID, x, y int) string {
	return fmt.Sprintf("%d:%d:%d", kingdomID, x, y)
}
