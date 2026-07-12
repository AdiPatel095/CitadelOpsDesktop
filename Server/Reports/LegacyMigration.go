package Reports

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

func MigrateLegacyHistory(dataDir string, history *History.Store, ownPlayerID State.PlayerID) error {
	if history == nil {
		return nil
	}
	_, spyErr := history.ImportLegacySource(
		filepath.Join(dataDir, "SpyReports.jsonl"),
		History.CollectionSpyReports,
		decodeLegacySpyReports,
	)
	_, battleErr := history.ImportLegacySource(
		filepath.Join(dataDir, "BattleReports.jsonl"),
		History.CollectionBattleReports,
		decodeLegacyBattleReports(ownPlayerID),
	)
	return errors.Join(spyErr, battleErr)
}

func decodeLegacySpyReports(reader io.Reader, sink History.ImportSink) error {
	return scanLegacyReportLines(reader, func(raw json.RawMessage) error {
		var canonical struct {
			ID                   string          `json:"id"`
			Status               string          `json:"status"`
			Target               json.RawMessage `json:"target"`
			Castle               json.RawMessage `json:"castle"`
			CapturedAtUnixMillis int64           `json:"capturedAtUnixMillis"`
		}
		if json.Unmarshal(raw, &canonical) == nil && canonical.ID != "" && canonical.Status != "" && len(canonical.Target) > 0 && len(canonical.Castle) > 0 {
			return sink(History.ImportRecord{CapturedAt: unixMillis(canonical.CapturedAtUnixMillis), Payload: raw})
		}
		var legacy struct {
			MID                  int64           `json:"mid"`
			CapturedAtUnixMillis int64           `json:"capturedAtUnixMillis"`
			BSD                  json.RawMessage `json:"bsd"`
		}
		if json.Unmarshal(raw, &legacy) != nil || len(legacy.BSD) == 0 {
			return nil
		}
		report, err := ParseSpyCapture(State.SpyReportCapture{
			MessageID: legacy.MID, Payload: legacy.BSD,
			CapturedAt: unixMillis(legacy.CapturedAtUnixMillis),
		})
		if err != nil {
			return nil
		}
		payload, err := json.Marshal(report)
		if err != nil {
			return err
		}
		return sink(History.ImportRecord{CapturedAt: unixMillis(legacy.CapturedAtUnixMillis), Payload: payload})
	})
}

func decodeLegacyBattleReports(ownPlayerID State.PlayerID) History.LegacySourceDecoder {
	return func(reader io.Reader, sink History.ImportSink) error {
		return scanLegacyReportLines(reader, func(raw json.RawMessage) error {
			var canonical struct {
				ID         string          `json:"id"`
				ReportID   string          `json:"reportID"`
				Metrics    json.RawMessage `json:"metrics"`
				DateMillis int64           `json:"dateMs"`
			}
			if json.Unmarshal(raw, &canonical) == nil && canonical.ID != "" && canonical.ReportID != "" && len(canonical.Metrics) > 0 {
				return sink(History.ImportRecord{CapturedAt: unixMillis(canonical.DateMillis), Payload: raw})
			}
			var legacy struct {
				MID                  int64           `json:"mid"`
				LID                  int64           `json:"lid"`
				BattleKey            string          `json:"battleKey"`
				CapturedAtUnixMillis int64           `json:"capturedAtUnixMillis"`
				BLD                  json.RawMessage `json:"bld"`
				BLM                  json.RawMessage `json:"blm"`
				BLS                  json.RawMessage `json:"bls"`
				Wire                 struct {
					BLD json.RawMessage `json:"bld"`
					BLM json.RawMessage `json:"blm"`
					BLS json.RawMessage `json:"bls"`
				} `json:"wire"`
			}
			if json.Unmarshal(raw, &legacy) != nil {
				return nil
			}
			summary := firstRaw(legacy.BLD, legacy.Wire.BLD)
			waves := firstRaw(legacy.BLM, legacy.Wire.BLM)
			details := firstRaw(legacy.BLS, legacy.Wire.BLS)
			if len(summary) == 0 || len(details) == 0 {
				return nil
			}
			capturedAt := unixMillis(legacy.CapturedAtUnixMillis)
			report, err := ParseBattleCapture(State.BattleReportCapture{
				MessageID: legacy.MID, ReportID: legacy.LID, BattleKey: legacy.BattleKey,
				Summary: summary, Waves: waves, Details: details, CapturedAt: capturedAt,
			}, ownPlayerID)
			if err != nil {
				return nil
			}
			payload, err := json.Marshal(report)
			if err != nil {
				return err
			}
			return sink(History.ImportRecord{CapturedAt: capturedAt, Payload: payload})
		})
	}
}

func scanLegacyReportLines(reader io.Reader, visit func(json.RawMessage) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		raw := json.RawMessage(append([]byte(nil), scanner.Bytes()...))
		if !json.Valid(raw) {
			continue
		}
		if err := visit(raw); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func firstRaw(primary json.RawMessage, fallback json.RawMessage) json.RawMessage {
	if len(primary) > 0 && string(primary) != "null" {
		return primary
	}
	return fallback
}

func unixMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}
