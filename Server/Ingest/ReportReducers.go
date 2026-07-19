package Ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const maxAutomaticSpyNoticeAge = 6 * time.Hour

func reduceReportNotices(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyReportNotices(frame.Payload, frame.ReceivedAt, gameState)
	return []string{"reports"}, changed, err
}

func applyReportNotices(raw json.RawMessage, observedAt time.Time, gameState *State.GameState) (bool, error) {
	var payload struct {
		Messages [][]json.RawMessage `json:"MSG"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode report notices: %w", err)
	}
	if gameState.Reports.Notices == nil {
		gameState.Reports.Notices = map[int64]State.ReportNotice{}
	}
	changed := false
	for _, row := range payload.Messages {
		if len(row) < 2 {
			continue
		}
		messageID, ok := rawInt64(row[0])
		if !ok || messageID <= 0 {
			continue
		}
		typeID, _ := rawInt64(row[1])
		if typeID != 3 && typeID != 6 {
			continue
		}
		notice := State.ReportNotice{
			MessageID: messageID, TypeID: int(typeID), Status: "pending", ObservedAt: observedAt,
		}
		if typeID == 3 {
			notice.OwnedByPlayer = len(row) <= 4
		}
		if len(row) > 2 {
			notice.BattleKey = rowString(row, 2)
		}
		if len(row) > 4 {
			if value, exists := rawInt64(row[4]); exists {
				if typeID == 3 {
					notice.OwnedByPlayer = value <= 0
				} else if value > 0 {
					notice.ReportID = value
				}
			}
		}
		if len(row) > 5 {
			notice.AgeSec, _ = rawInt64(row[5])
		}
		if typeID == 3 && (notice.AgeSec < 0 || notice.AgeSec >= int64(maxAutomaticSpyNoticeAge/time.Second)) {
			notice.Status = "expired"
		}
		if current, exists := gameState.Reports.Notices[messageID]; exists {
			if current.Status == "archived" || current.Status == "unavailable" {
				notice.Status = current.Status
			}
			if current.ReportID > 0 && notice.ReportID == 0 {
				notice.ReportID = current.ReportID
			}
		}
		if gameState.Reports.Notices[messageID] != notice {
			gameState.Reports.Notices[messageID] = notice
			changed = true
		}
	}
	return changed, nil
}

func reduceSpyReportCapture(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var header struct {
		MessageID wireInt64 `json:"MID"`
	}
	if err := json.Unmarshal(frame.Payload, &header); err != nil {
		return nil, false, fmt.Errorf("decode spy report capture: %w", err)
	}
	if header.MessageID <= 0 {
		return nil, false, nil
	}
	if notice := gameState.Reports.Notices[int64(header.MessageID)]; notice.Status == "archived" {
		return nil, false, nil
	}
	if gameState.Reports.SpyCaptures == nil {
		gameState.Reports.SpyCaptures = map[int64]State.SpyReportCapture{}
	}
	current := gameState.Reports.SpyCaptures[int64(header.MessageID)]
	if bytes.Equal(current.Payload, frame.Payload) {
		return nil, false, nil
	}
	gameState.Reports.SpyCaptures[int64(header.MessageID)] = State.SpyReportCapture{
		MessageID: int64(header.MessageID), Payload: append(json.RawMessage(nil), frame.Payload...), CapturedAt: frame.ReceivedAt,
	}
	return []string{"reports"}, true, nil
}

func reduceBattleSummaryCapture(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var header struct {
		MessageID wireInt64 `json:"MID"`
		ReportID  wireInt64 `json:"LID"`
	}
	if err := json.Unmarshal(frame.Payload, &header); err != nil {
		return nil, false, fmt.Errorf("decode battle summary capture: %w", err)
	}
	if header.MessageID <= 0 {
		return nil, false, nil
	}
	if notice := gameState.Reports.Notices[int64(header.MessageID)]; notice.Status == "archived" {
		return nil, false, nil
	}
	ensureReportMaps(gameState)
	capture := gameState.Reports.BattleCaptures[int64(header.MessageID)]
	capture.MessageID = int64(header.MessageID)
	if header.ReportID > 0 {
		capture.ReportID = int64(header.ReportID)
	}
	if notice := gameState.Reports.Notices[capture.MessageID]; notice.BattleKey != "" {
		capture.BattleKey = notice.BattleKey
	}
	if bytes.Equal(capture.Summary, frame.Payload) {
		return nil, false, nil
	}
	capture.Summary = append(json.RawMessage(nil), frame.Payload...)
	capture.CapturedAt = frame.ReceivedAt
	gameState.Reports.BattleCaptures[capture.MessageID] = capture
	return []string{"reports"}, true, nil
}

func reduceBattleWaveCapture(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var header struct {
		ReportID wireInt64 `json:"LID"`
	}
	if err := json.Unmarshal(frame.Payload, &header); err != nil {
		return nil, false, fmt.Errorf("decode battle wave capture: %w", err)
	}
	reportID := int64(header.ReportID)
	if reportID <= 0 {
		reportID = gameState.Reports.ActiveBattleReport
	}
	if reportID <= 0 {
		return nil, false, nil
	}
	messageID, capture, exists := battleCaptureByReportID(gameState, reportID)
	if !exists || bytes.Equal(capture.Waves, frame.Payload) {
		return nil, false, nil
	}
	capture.Waves = append(json.RawMessage(nil), frame.Payload...)
	capture.CapturedAt = frame.ReceivedAt
	gameState.Reports.BattleCaptures[messageID] = capture
	return []string{"reports"}, true, nil
}

func reduceBattleDetailCapture(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var header struct {
		ReportID wireInt64 `json:"LID"`
	}
	if err := json.Unmarshal(frame.Payload, &header); err != nil {
		return nil, false, fmt.Errorf("decode battle detail capture: %w", err)
	}
	if header.ReportID <= 0 {
		header.ReportID = wireInt64(gameState.Reports.ActiveBattleReport)
	}
	messageID, capture, exists := battleCaptureByReportID(gameState, int64(header.ReportID))
	if !exists || bytes.Equal(capture.Details, frame.Payload) {
		return nil, false, nil
	}
	capture.Details = append(json.RawMessage(nil), frame.Payload...)
	capture.CapturedAt = frame.ReceivedAt
	gameState.Reports.BattleCaptures[messageID] = capture
	stormChanged, err := reconcileStormIslandBattleReport(gameState, capture)
	if err != nil {
		return nil, false, err
	}
	domains := []string{"reports"}
	if stormChanged {
		domains = append(domains, "storm")
	}
	return domains, true, nil
}

func reconcileStormIslandBattleReport(gameState *State.GameState, capture State.BattleReportCapture) (bool, error) {
	if gameState.Player.ID <= 0 || len(capture.Summary) == 0 || len(capture.Details) == 0 {
		return false, nil
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
	if err := json.Unmarshal(capture.Summary, &summary); err != nil {
		return false, fmt.Errorf("decode Storm island battle summary: %w", err)
	}
	if summary.Target.TypeID != 24 || State.KingdomID(summary.Target.KingdomID) != State.KingdomID(GameData.StormKingdomID) ||
		!battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) {
		return false, nil
	}
	key := State.StormIslandReturnKey(State.KingdomID(summary.Target.KingdomID), summary.Target.X, summary.Target.Y)
	operation, exists := gameState.Storm.IslandReturns[key]
	if !exists || operation.Status != State.StormIslandReturnAwaitingReport {
		return false, nil
	}
	battleAt := capture.CapturedAt
	if notice, found := gameState.Reports.Notices[capture.MessageID]; found && !notice.ObservedAt.IsZero() {
		battleAt = notice.ObservedAt
		if notice.AgeSec > 0 {
			battleAt = battleAt.Add(-time.Duration(notice.AgeSec) * time.Second)
		}
	}
	if !operation.LaunchedAt.IsZero() && battleAt.Add(2*time.Second).Before(operation.LaunchedAt) {
		return false, nil
	}
	if !battleSummaryAttackerWon(summary.Participants) {
		delete(gameState.Storm.IslandReturns, key)
		return true, nil
	}
	survivors, found, err := stormIslandBattleSurvivors(capture.Details, gameState.Player.ID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	operation.Survivors = survivors
	if len(operation.UnitsToReturn()) == 0 {
		delete(gameState.Storm.IslandReturns, key)
		return true, nil
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
	operation.ReportID = reportID
	operation.Status = State.StormIslandReturnReady
	operation.ReportedAt = capture.CapturedAt
	gameState.Storm.IslandReturns[key] = operation
	return true, nil
}

func stormIslandBattleSurvivors(raw json.RawMessage, playerID State.PlayerID) (map[State.UnitID]int64, bool, error) {
	var payload struct {
		Units [][]json.RawMessage `json:"Y"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Storm island battle details: %w", err)
	}
	survivors := map[State.UnitID]int64{}
	found := false
	for _, row := range payload.Units {
		if len(row) < 2 || State.PlayerID(rowInt(row, 0)) != playerID {
			continue
		}
		found = true
		for _, rawUnit := range row[1:] {
			var unit []json.RawMessage
			if err := json.Unmarshal(rawUnit, &unit); err != nil || len(unit) < 2 {
				continue
			}
			unitID := State.UnitID(rowInt(unit, 0))
			amount := rowInt(unit, 1)
			lost := rowInt(unit, 2)
			if lost < 0 {
				lost = -lost
			}
			remaining := amount - lost
			if unitID > 0 && remaining > 0 {
				survivors[unitID] += remaining
			}
		}
	}
	return survivors, found, nil
}

func reduceBattleCommandContext(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	var command struct {
		ReportID wireInt64 `json:"LID"`
	}
	if len(frame.Payload) == 0 || json.Unmarshal(frame.Payload, &command) != nil || command.ReportID <= 0 {
		return nil, false, nil
	}
	if gameState.Reports.ActiveBattleReport == int64(command.ReportID) {
		return nil, false, nil
	}
	gameState.Reports.ActiveBattleReport = int64(command.ReportID)
	return []string{"reports"}, true, nil
}

func ensureReportMaps(gameState *State.GameState) {
	if gameState.Reports.Notices == nil {
		gameState.Reports.Notices = map[int64]State.ReportNotice{}
	}
	if gameState.Reports.SpyCaptures == nil {
		gameState.Reports.SpyCaptures = map[int64]State.SpyReportCapture{}
	}
	if gameState.Reports.BattleCaptures == nil {
		gameState.Reports.BattleCaptures = map[int64]State.BattleReportCapture{}
	}
}

func battleCaptureByReportID(gameState *State.GameState, reportID int64) (int64, State.BattleReportCapture, bool) {
	for messageID, capture := range gameState.Reports.BattleCaptures {
		if capture.ReportID == reportID {
			return messageID, capture, true
		}
	}
	return 0, State.BattleReportCapture{}, false
}
