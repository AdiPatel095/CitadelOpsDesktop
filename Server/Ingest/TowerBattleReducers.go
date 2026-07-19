package Ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const towerMapTypeID = 2

// reduceSuccessfulTowerBattle records only confirmed player victories from a
// battle summary. The summary identifies the target, but not its cooldown;
// Auto Towers therefore follows this with a one-tile map refresh and stores
// the map's returned cooldown as the authoritative value.
func reduceSuccessfulTowerBattle(
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
		return nil, false, fmt.Errorf("decode tower battle summary: %w", err)
	}
	if summary.Target.TypeID != towerMapTypeID || !battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) ||
		!battleSummaryAttackerWon(summary.Participants) {
		return nil, false, nil
	}
	reportID := int64(summary.ReportID)
	if reportID <= 0 {
		reportID = int64(summary.MessageID)
	}
	key := towerCooldownKey(State.KingdomID(summary.Target.KingdomID), summary.Target.X, summary.Target.Y)
	if gameState.TowerCooldowns == nil {
		gameState.TowerCooldowns = map[string]State.TowerCooldownState{}
	}
	if current, exists := gameState.TowerCooldowns[key]; exists && current.ReportID == reportID && reportID > 0 {
		return nil, false, nil
	}
	gameState.TowerCooldowns[key] = State.TowerCooldownState{
		KingdomID: State.KingdomID(summary.Target.KingdomID), X: summary.Target.X, Y: summary.Target.Y,
		ReportID: reportID, LastSuccessfulBattleAt: frame.ReceivedAt, PendingCooldownRefresh: true,
	}
	domains := []string{"tower-cooldowns"}
	if recordRBCTestVictory(gameState, State.KingdomID(summary.Target.KingdomID), summary.Target.X, summary.Target.Y, reportID) {
		domains = append(domains, "nomad-camps")
	}
	if recordKhanVictory(gameState, State.KingdomID(summary.Target.KingdomID), summary.Target.X, summary.Target.Y, reportID) {
		domains = append(domains, "khan")
	}
	return domains, true, nil
}

func recordKhanVictory(gameState *State.GameState, kingdomID State.KingdomID, x, y int, reportID int64) bool {
	khan := &gameState.Khan
	if khan.RunID == "" || khan.KingdomID != kingdomID || khan.TargetX != x || khan.TargetY != y ||
		khan.VictoriesConfirmed >= khan.AttacksLaunched || reportID > 0 && khan.LastReportID == reportID {
		return false
	}
	khan.VictoriesConfirmed++
	khan.LastReportID = reportID
	return true
}

func recordRBCTestVictory(gameState *State.GameState, kingdomID State.KingdomID, x, y int, reportID int64) bool {
	test := gameState.NomadCamps.RBCTest
	if test == nil || test.KingdomID != kingdomID || test.TargetX != x || test.TargetY != y ||
		test.ExpectedAttacks <= 0 || test.VictoriesConfirmed >= test.AttacksLaunched ||
		reportID > 0 && test.LastReportID == reportID {
		return false
	}
	test.VictoriesConfirmed++
	test.LastReportID = reportID
	return true
}

func battleSummaryHasOwnAttacker(participants [][]json.RawMessage, playerID State.PlayerID) bool {
	for _, participant := range participants {
		if len(participant) < 2 {
			continue
		}
		if State.PlayerID(rowInt(participant, 0)) == playerID && rowInt(participant, 1) == 0 {
			return true
		}
	}
	return false
}

func battleSummaryAttackerWon(participants [][]json.RawMessage) bool {
	attackerPresent, defenderPresent := false, false
	var attackerSurvivors, defenderSurvivors int64
	for _, participant := range participants {
		if len(participant) < 4 {
			continue
		}
		survivors := rowInt(participant, 2) + rowInt(participant, 3)
		if survivors < 0 {
			survivors = 0
		}
		switch rowInt(participant, 1) {
		case 0:
			attackerPresent = true
			attackerSurvivors += survivors
		case 1:
			defenderPresent = true
			defenderSurvivors += survivors
		}
	}
	return attackerPresent && defenderPresent && attackerSurvivors > 0 && defenderSurvivors == 0
}

func refreshTowerCooldownFromMap(gameState *State.GameState, observation State.MapObservation) bool {
	if observation.TypeID != towerMapTypeID || gameState.TowerCooldowns == nil {
		return false
	}
	key := towerCooldownKey(observation.KingdomID, observation.X, observation.Y)
	current, exists := gameState.TowerCooldowns[key]
	if !exists || current.LastSuccessfulBattleAt.After(observation.ObservedAt) {
		return false
	}
	next := current
	next.CooldownRemaining = observation.TowerCooldownRemaining
	next.CooldownObservedAt = observation.ObservedAt
	next.PendingCooldownRefresh = false
	if next == current {
		return false
	}
	gameState.TowerCooldowns[key] = next
	return true
}

func towerCooldownKey(kingdomID State.KingdomID, x, y int) string {
	return fmt.Sprintf("%d:%d:%d", kingdomID, x, y)
}
