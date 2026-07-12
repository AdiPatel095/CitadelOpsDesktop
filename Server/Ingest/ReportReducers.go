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
		if len(row) > 2 {
			notice.BattleKey = rowString(row, 2)
		}
		if len(row) > 4 {
			if reportID, exists := rawInt64(row[4]); exists && reportID > 0 {
				notice.ReportID = reportID
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
	return []string{"reports"}, true, nil
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
