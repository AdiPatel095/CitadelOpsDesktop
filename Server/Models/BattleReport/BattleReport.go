package battlereport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Paths"
)

const (
	archiveFileName        = "BattleReports.jsonl"
	clientIDFileName       = "BattleReportClientID.txt"
	defaultCloudBackendURL = "https://citadelops.app/api"
	maxScanLineSize        = 32 * 1024 * 1024
	cloudUploadTimeout     = 20 * time.Second
)

type Capture struct {
	Version              int                    `json:"version"`
	ID                   string                 `json:"id"`
	ClientID             string                 `json:"clientID,omitempty"`
	Source               string                 `json:"source"`
	CapturedAtUnixMillis int64                  `json:"capturedAtUnixMillis"`
	MID                  int64                  `json:"mid"`
	LID                  int64                  `json:"lid"`
	NoticeType           int                    `json:"noticeType,omitempty"`
	BattleKey            string                 `json:"battleKey,omitempty"`
	SNERow               []interface{}          `json:"sneRow,omitempty"`
	SNE                  map[string]interface{} `json:"sne,omitempty"`
	BLS                  map[string]interface{} `json:"bls,omitempty"`
	BLM                  map[string]interface{} `json:"blm,omitempty"`
	BLD                  map[string]interface{} `json:"bld,omitempty"`
	Wire                 map[string]string      `json:"wire,omitempty"`
}

type cloudBattleReportPayload struct {
	BattleDateUnixMillis int64  `json:"battleDateUnixMillis"`
	MID                  int64  `json:"mid"`
	LID                  int64  `json:"lid"`
	AttackerPlayerID     int64  `json:"attackerPlayerID"`
	AttackerAllianceID   int64  `json:"attackerAllianceID"`
	DefenderPlayerID     int64  `json:"defenderPlayerID"`
	DefenderAllianceID   int64  `json:"defenderAllianceID"`
	AttackWon            bool   `json:"attackWon"`
	RichnessScore        int    `json:"richnessScore"`
	Payload              string `json:"payload"`
}

type ParsedReport struct {
	ID               string             `json:"id"`
	ReportID         string             `json:"reportID,omitempty"`
	MID              int64              `json:"mid,omitempty"`
	LID              int64              `json:"lid,omitempty"`
	BattleKey        string             `json:"battleKey,omitempty"`
	KingdomID        int                `json:"kingdomID,omitempty"`
	TargetName       string             `json:"targetName,omitempty"`
	CastleName       string             `json:"castleName,omitempty"`
	BattleType       string             `json:"battleType,omitempty"`
	OccurredAt       string             `json:"occurredAt,omitempty"`
	DateMs           int64              `json:"dateMs,omitempty"`
	Result           string             `json:"result,omitempty"`
	Role             string             `json:"role,omitempty"`
	Attacker         *Combatant         `json:"attacker,omitempty"`
	Defender         *Combatant         `json:"defender,omitempty"`
	Metrics          Metrics            `json:"metrics,omitempty"`
	Effects          []Effect           `json:"effects,omitempty"`
	CommanderEffects []Effect           `json:"commanderEffects,omitempty"`
	CastellanEffects []Effect           `json:"castellanEffects,omitempty"`
	TopUnits         []BattleItemDetail `json:"topUnits,omitempty"`
	SupportTools     []BattleItemDetail `json:"supportTools,omitempty"`
	Waves            []Wave             `json:"waves,omitempty"`
	RawCapture       *Capture           `json:"rawCapture,omitempty"`
}

type Combatant struct {
	PlayerID     int64  `json:"playerID,omitempty"`
	Name         string `json:"name,omitempty"`
	PlayerName   string `json:"playerName,omitempty"`
	AllianceID   int64  `json:"allianceID,omitempty"`
	Alliance     string `json:"alliance,omitempty"`
	AllianceName string `json:"allianceName,omitempty"`
	AllianceTag  string `json:"allianceTag,omitempty"`
	CastleName   string `json:"castleName,omitempty"`
	Role         string `json:"role,omitempty"`
}

type Metrics struct {
	AttackerSent      int64   `json:"attackerSent,omitempty"`
	AttackerLost      int64   `json:"attackerLost,omitempty"`
	AttackersKilled   int64   `json:"attackersKilled,omitempty"`
	DefenderStationed int64   `json:"defenderStationed,omitempty"`
	DefenderLost      int64   `json:"defenderLost,omitempty"`
	DefendersKilled   int64   `json:"defendersKilled,omitempty"`
	WallLosses        int64   `json:"wallLosses,omitempty"`
	CourtyardLosses   int64   `json:"courtyardLosses,omitempty"`
	AttackTradeRatio  float64 `json:"attackTradeRatio,omitempty"`
	DefenseTradeRatio float64 `json:"defenseTradeRatio,omitempty"`
}

type Effect struct {
	Code           string  `json:"code,omitempty"`
	Label          string  `json:"label,omitempty"`
	Name           string  `json:"name,omitempty"`
	Value          float64 `json:"value,omitempty"`
	FormattedValue string  `json:"formattedValue,omitempty"`
	DisplayText    string  `json:"displayText,omitempty"`
	Category       string  `json:"category,omitempty"`
	SortOrder      int     `json:"sortOrder,omitempty"`
	Side           string  `json:"side,omitempty"`
}

type BattleItemDetail struct {
	Side   string `json:"side,omitempty"`
	Phase  string `json:"phase,omitempty"`
	Lane   string `json:"lane,omitempty"`
	WodID  int64  `json:"wodID,omitempty"`
	Amount int64  `json:"amount,omitempty"`
	Lost   int64  `json:"lost,omitempty"`
	Used   int64  `json:"used,omitempty"`
}

type Wave struct {
	Index int        `json:"index,omitempty"`
	Wave  int        `json:"wave,omitempty"`
	Lanes []WaveLane `json:"lanes,omitempty"`
}

type WaveLane struct {
	Lane                string             `json:"lane,omitempty"`
	Result              string             `json:"result,omitempty"`
	AttackerLost        int64              `json:"attackerLost,omitempty"`
	DefenderLost        int64              `json:"defenderLost,omitempty"`
	AttackerStart       int64              `json:"attackerStart,omitempty"`
	DefenderStart       int64              `json:"defenderStart,omitempty"`
	AttackerUnitDetails []BattleItemDetail `json:"attackerUnitDetails,omitempty"`
	DefenderUnitDetails []BattleItemDetail `json:"defenderUnitDetails,omitempty"`
	AttackerToolDetails []BattleItemDetail `json:"attackerToolDetails,omitempty"`
	DefenderToolDetails []BattleItemDetail `json:"defenderToolDetails,omitempty"`
}

var archiveMu sync.Mutex

func RegisterStatsHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/battle-reports", handleListReports)
	mux.HandleFunc("GET /api/battleReports", handleListReports)
	mux.HandleFunc("GET /api/reports/battle", handleListReports)
	mux.HandleFunc("GET /api/battle-reports/cloud", handleListCloudReports)
	mux.HandleFunc("GET /api/battleReports/cloud", handleListCloudReports)
	mux.HandleFunc("POST /api/battle-reports", handlePostReport)
	mux.HandleFunc("POST /api/reports/battle", handlePostReport)
}

func RecordSNEPayload(payload string) ([]Capture, error) {
	captures, err := CapturesFromSNEPayload(payload)
	if err != nil {
		return nil, err
	}
	for _, capture := range captures {
		if err := AppendCapture(capture); err != nil {
			return captures, err
		}
	}
	return captures, nil
}

func CapturesFromSNEPayload(payload string) ([]Capture, error) {
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &root); err != nil {
		return nil, err
	}
	rows, ok := root["MSG"].([]interface{})
	if !ok {
		return []Capture{}, nil
	}
	captures := make([]Capture, 0, len(rows))
	for _, rawRow := range rows {
		row, ok := rawRow.([]interface{})
		if !ok {
			continue
		}
		capture, ok := CaptureFromSNERow(root, row)
		if !ok {
			continue
		}
		captures = append(captures, capture)
	}
	return captures, nil
}

func CaptureFromSNERow(root map[string]interface{}, row []interface{}) (Capture, bool) {
	mid, ok := int64FromValue(rowValue(row, 0))
	if !ok || mid <= 0 {
		return Capture{}, false
	}
	lid, _ := int64FromValue(rowValue(row, 4))
	if lid == 0 {
		lid = mid
	}
	noticeType, _ := int64FromValue(rowValue(row, 1))
	battleKey := stringFromValue(rowValue(row, 2))
	if !isSharedBattleReportNotice(noticeType, battleKey) {
		return Capture{}, false
	}
	capture := Capture{
		Version:              1,
		ID:                   captureID(mid, lid),
		Source:               "sne",
		CapturedAtUnixMillis: time.Now().UnixMilli(),
		MID:                  mid,
		LID:                  lid,
		NoticeType:           int(noticeType),
		BattleKey:            battleKey,
		SNERow:               row,
		SNE:                  root,
	}
	return capture, true
}

func isSharedBattleReportNotice(noticeType int64, battleKey string) bool {
	return noticeType == 6 && strings.Contains(battleKey, "#")
}

func AppendCapture(capture Capture) error {
	if err := normalizeCapture(&capture); err != nil {
		return err
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	existingCaptures, err := readCapturesFromArchive()
	if err != nil {
		return err
	}
	for _, existing := range existingCaptures {
		if sameReport(existing, capture) {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(archivePath()), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(archivePath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(capture)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func DeleteCapture(capture Capture) error {
	if err := normalizeCapture(&capture); err != nil {
		return err
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	captures, err := readCapturesFromArchive()
	if err != nil {
		return err
	}
	filtered := captures[:0]
	for _, existing := range captures {
		if !sameReport(existing, capture) {
			filtered = append(filtered, existing)
		}
	}
	return writeCapturesToArchive(filtered)
}

func UpsertCapture(capture Capture) error {
	if err := normalizeCapture(&capture); err != nil {
		return err
	}
	archiveMu.Lock()
	defer archiveMu.Unlock()
	captures, err := readCapturesFromArchive()
	if err != nil {
		return err
	}
	replaced := false
	for i := range captures {
		if sameReport(captures[i], capture) {
			captures[i] = mergeCapture(captures[i], capture)
			replaced = true
			break
		}
	}
	if !replaced {
		captures = append(captures, capture)
	}
	return writeCapturesToArchive(captures)
}

func UploadCaptureToCloud(capture Capture) error {
	if err := normalizeCapture(&capture); err != nil {
		return err
	}
	envelope, err := cloudBattleReportFromCapture(capture)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, battleReportUploadURL(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	applyReportKeyHeader(req)
	client := &http.Client{Timeout: cloudUploadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("cloud battle report upload failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func cloudBattleReportFromCapture(capture Capture) (cloudBattleReportPayload, error) {
	parsed := ParseCapture(&capture)
	if parsed.ID == "" || !ReportHasBothPlayers(parsed) {
		return cloudBattleReportPayload{}, errors.New("cloud battle report requires parsed attacker and defender")
	}
	if parsed.Attacker == nil || parsed.Defender == nil || parsed.Attacker.PlayerID <= 0 || parsed.Defender.PlayerID <= 0 {
		return cloudBattleReportPayload{}, errors.New("cloud battle report requires attackerPlayerID and defenderPlayerID")
	}
	battleDateUnixMillis := parsed.DateMs
	if battleDateUnixMillis <= 0 {
		battleDateUnixMillis = capture.CapturedAtUnixMillis
	}
	if battleDateUnixMillis <= 0 {
		return cloudBattleReportPayload{}, errors.New("cloud battle report requires battleDateUnixMillis")
	}
	richnessScore := ReportRichnessScore(capture)
	if richnessScore <= 0 {
		return cloudBattleReportPayload{}, errors.New("cloud battle report requires richnessScore")
	}
	payloadCapture := capture
	payloadCapture.ClientID = ""
	rawCapture, err := json.Marshal(payloadCapture)
	if err != nil {
		return cloudBattleReportPayload{}, err
	}
	return cloudBattleReportPayload{
		BattleDateUnixMillis: battleDateUnixMillis,
		MID:                  parsed.MID,
		LID:                  parsed.LID,
		AttackerPlayerID:     parsed.Attacker.PlayerID,
		AttackerAllianceID:   parsed.Attacker.AllianceID,
		DefenderPlayerID:     parsed.Defender.PlayerID,
		DefenderAllianceID:   parsed.Defender.AllianceID,
		AttackWon:            inferBinaryResult(parsed.Metrics) != "Defeat",
		RichnessScore:        richnessScore,
		Payload:              string(rawCapture),
	}, nil
}

func ReportRichnessScore(capture Capture) int {
	score := rawCaptureRichnessScore(capture)
	report := ParseCapture(&capture)
	score += parsedReportRichnessScore(report)
	return score
}

func rawCaptureRichnessScore(capture Capture) int {
	score := 0
	if len(capture.SNE) > 0 {
		score += 500
	}
	if len(capture.BLS) > 0 {
		score += 1000
	}
	if len(capture.BLM) > 0 {
		score += 1000
	}
	if len(capture.BLD) > 0 {
		score += 1000
	}
	return score
}

func parsedReportRichnessScore(report ParsedReport) int {
	score := 0
	if report.ID != "" {
		score += 25
	}
	if report.DateMs > 0 || report.OccurredAt != "" {
		score += 25
	}
	if report.BattleKey != "" || report.CastleName != "" || report.TargetName != "" {
		score += 25
	}
	score += combatantRichnessScore(report.Attacker)
	score += combatantRichnessScore(report.Defender)
	score += metricsRichnessScore(report.Metrics)
	score += effectsRichnessScore(report.CommanderEffects)
	score += effectsRichnessScore(report.CastellanEffects)
	score += battleItemsRichnessScore(report.TopUnits)
	score += battleItemsRichnessScore(report.SupportTools)
	score += wavesRichnessScore(report.Waves)
	score += defenderDetailRichnessScore(report)
	return score
}

func combatantRichnessScore(combatant *Combatant) int {
	if combatant == nil {
		return 0
	}
	score := 10
	if combatant.PlayerID > 0 {
		score += 50
	}
	if combatant.AllianceID > 0 {
		score += 25
	}
	if combatant.Name != "" || combatant.PlayerName != "" {
		score += 10
	}
	if combatant.Alliance != "" || combatant.AllianceName != "" || combatant.AllianceTag != "" {
		score += 10
	}
	if combatant.CastleName != "" || combatant.Role != "" {
		score += 5
	}
	return score
}

func metricsRichnessScore(metrics Metrics) int {
	score := 0
	for _, value := range []int64{
		metrics.AttackerSent,
		metrics.AttackerLost,
		metrics.AttackersKilled,
		metrics.DefenderStationed,
		metrics.DefenderLost,
		metrics.DefendersKilled,
		metrics.WallLosses,
		metrics.CourtyardLosses,
	} {
		if value != 0 {
			score += 8
		}
	}
	if metrics.AttackTradeRatio != 0 {
		score += 4
	}
	if metrics.DefenseTradeRatio != 0 {
		score += 4
	}
	return score
}

func effectsRichnessScore(effects []Effect) int {
	score := 0
	for _, effect := range effects {
		score += 5
		if effect.Code != "" {
			score += 2
		}
		if effect.Label != "" || effect.Name != "" {
			score += 2
		}
		if effect.Value != 0 || effect.FormattedValue != "" || effect.DisplayText != "" {
			score += 3
		}
		if effect.Category != "" || effect.Side != "" {
			score += 2
		}
	}
	return score
}

func battleItemsRichnessScore(items []BattleItemDetail) int {
	score := 0
	for _, item := range items {
		if item.WodID <= 0 {
			continue
		}
		score += 5
		if item.Side != "" {
			score++
		}
		if item.Phase != "" {
			score++
		}
		if item.Lane != "" {
			score++
		}
		if item.Amount != 0 {
			score += 2
		}
		if item.Lost != 0 || item.Used != 0 {
			score += 2
		}
	}
	return score
}

func wavesRichnessScore(waves []Wave) int {
	score := 0
	for _, wave := range waves {
		score += 10
		if wave.Index != 0 || wave.Wave != 0 {
			score += 2
		}
		for _, lane := range wave.Lanes {
			score += waveLaneRichnessScore(lane)
		}
	}
	return score
}

func waveLaneRichnessScore(lane WaveLane) int {
	score := 5
	if lane.Lane != "" || lane.Result != "" {
		score += 4
	}
	for _, value := range []int64{lane.AttackerLost, lane.DefenderLost, lane.AttackerStart, lane.DefenderStart} {
		if value != 0 {
			score += 3
		}
	}
	score += battleItemsRichnessScore(lane.AttackerUnitDetails)
	score += battleItemsRichnessScore(lane.DefenderUnitDetails)
	score += battleItemsRichnessScore(lane.AttackerToolDetails)
	score += battleItemsRichnessScore(lane.DefenderToolDetails)
	return score
}

func defenderDetailRichnessScore(report ParsedReport) int {
	score := 0
	score += defenderBattleItemDetailsRichnessScore(report.TopUnits, true)
	score += defenderBattleItemDetailsRichnessScore(report.SupportTools, true)
	for _, wave := range report.Waves {
		for _, lane := range wave.Lanes {
			if lane.DefenderStart != 0 || lane.DefenderLost != 0 {
				score += 25
			}
			score += defenderBattleItemDetailsRichnessScore(lane.DefenderUnitDetails, false)
			score += defenderBattleItemDetailsRichnessScore(lane.DefenderToolDetails, false)
		}
	}
	return score
}

func defenderBattleItemDetailsRichnessScore(items []BattleItemDetail, requireDefenderSide bool) int {
	score := 0
	for _, item := range items {
		if requireDefenderSide && item.Side != "defender" {
			continue
		}
		if item.WodID <= 0 && item.Amount == 0 && item.Lost == 0 && item.Used == 0 {
			continue
		}
		score += 15
		if item.Amount != 0 {
			score += 5
		}
		if item.Lost != 0 || item.Used != 0 {
			score += 5
		}
	}
	return score
}

func FetchCloudParsedReports(ctx context.Context) ([]ParsedReport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, battleReportFetchURL(), nil)
	if err != nil {
		return nil, err
	}
	applyReportKeyHeader(req)

	client := &http.Client{Timeout: cloudUploadTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("cloud battle report fetch failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()
	var payload interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	reports := parsedReportsFromCloudPayload(payload)
	filtered := reports[:0]
	for _, report := range reports {
		if report.ID != "" && ReportHasBothPlayers(report) {
			filtered = append(filtered, report)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].DateMs > filtered[j].DateMs
	})
	return filtered, nil
}

func ReadParsedReports() ([]ParsedReport, error) {
	captures, err := ReadCaptures()
	if err != nil {
		return nil, err
	}
	reports := make([]ParsedReport, 0, len(captures))
	for i := range captures {
		report := ParseCapture(&captures[i])
		if report.ID == "" || !ReportHasBothPlayers(report) {
			continue
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func ReadCaptures() ([]Capture, error) {
	return readCapturesFromArchive()
}

func readCapturesFromArchive() ([]Capture, error) {
	f, err := os.Open(archivePath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Capture{}, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxScanLineSize)
	var captures []Capture
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var capture Capture
		if err := json.Unmarshal([]byte(line), &capture); err != nil {
			continue
		}
		captures = append(captures, capture)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return captures, nil
}

func writeCapturesToArchive(captures []Capture) error {
	path := archivePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	for _, capture := range captures {
		if err := normalizeCapture(&capture); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
		line, err := json.Marshal(capture)
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func ParseCapture(capture *Capture) ParsedReport {
	if capture == nil {
		return ParsedReport{}
	}
	targetName := cleanBattleLocation(capture.BattleKey)
	report := ParsedReport{
		ID:         captureID(capture.MID, capture.LID),
		ReportID:   captureID(capture.MID, capture.LID),
		MID:        capture.MID,
		LID:        capture.LID,
		BattleKey:  capture.BattleKey,
		TargetName: targetName,
		CastleName: targetName,
		BattleType: "Castle battle",
		DateMs:     capture.CapturedAtUnixMillis,
		OccurredAt: timeFromUnixMs(capture.CapturedAtUnixMillis),
		Role:       "Unknown",
		Result:     "Unknown",
		RawCapture: capture,
	}
	applyBattleMeta(&report, capture.BLS)
	applyBattleItemSummaries(&report, capture.BLD)
	applyBattleWaves(&report, capture.BLD)
	applyBattleWaves(&report, capture.BLM)
	report.TopUnits = aggregateBattleItems(report.TopUnits)
	report.SupportTools = aggregateBattleItems(report.SupportTools)
	report.Result = inferBinaryResult(report.Metrics)
	return report
}

func applyBattleMeta(report *ParsedReport, bls map[string]interface{}) {
	if report == nil || bls == nil {
		return
	}
	if lid, ok := int64FromValue(bls["LID"]); ok && lid > 0 {
		report.LID = lid
		report.ID = captureID(report.MID, report.LID)
		report.ReportID = report.ID
	}
	if ai, ok := bls["AI"].(map[string]interface{}); ok {
		if targetName := cleanBattleLocation(stringFromValue(ai["N"])); targetName != "" {
			report.TargetName = targetName
			report.CastleName = targetName
		}
		if kingdomID, ok := int64FromValue(ai["K"]); ok {
			report.KingdomID = int(kingdomID)
		}
	}
	players := playerInfoByOID(bls)
	roles := pbiRoles(bls)
	for oid, role := range roles {
		info := players[oid]
		combatant := combatantFromPlayerInfo(oid, role, info)
		if combatant.Name == "" {
			continue
		}
		if role == "attacker" {
			report.Attacker = &combatant
		} else if role == "defender" {
			report.Defender = &combatant
		}
	}
	report.Metrics = metricsFromPBI(bls)
	combatMode := reportBattleEffectCombatMode(report)
	report.CommanderEffects = effectsFromLeader(bls["AL"], "commander", combatMode)
	report.CastellanEffects = effectsFromLeader(bls["DB"], "castellan", combatMode)
	report.Effects = append(report.Effects, report.CommanderEffects...)
	report.Effects = append(report.Effects, report.CastellanEffects...)
}

func reportBattleEffectCombatMode(report *ParsedReport) battleEffectCombatMode {
	if report != nil &&
		report.Attacker != nil &&
		report.Defender != nil &&
		report.Attacker.PlayerID > 0 &&
		report.Defender.PlayerID > 0 {
		return battleEffectCombatPVP
	}
	return battleEffectCombatPVE
}

func pbiRoles(bls map[string]interface{}) map[int64]string {
	out := make(map[int64]string)
	rows, _ := bls["PBI"].([]interface{})
	for _, raw := range rows {
		row, ok := raw.([]interface{})
		if !ok {
			continue
		}
		oid, ok := int64FromValue(rowValue(row, 0))
		if !ok {
			continue
		}
		side, _ := int64FromValue(rowValue(row, 1))
		if side == 0 {
			out[oid] = "attacker"
		} else if side == 1 {
			out[oid] = "defender"
		}
	}
	return out
}

func metricsFromPBI(bls map[string]interface{}) Metrics {
	var metrics Metrics
	rows, _ := bls["PBI"].([]interface{})
	for _, raw := range rows {
		row, ok := raw.([]interface{})
		if !ok {
			continue
		}
		side, _ := int64FromValue(rowValue(row, 1))
		started := int64Abs(rowValueInt64(row, 2))
		lost := int64Abs(rowValueInt64(row, 3))
		if side == 0 {
			metrics.AttackerSent += started
			metrics.AttackerLost += lost
			metrics.AttackersKilled += lost
		} else if side == 1 {
			metrics.DefenderStationed += started
			metrics.DefenderLost += lost
			metrics.DefendersKilled += lost
		}
	}
	if metrics.AttackerLost > 0 {
		metrics.AttackTradeRatio = roundFloat(float64(metrics.DefenderLost)/float64(metrics.AttackerLost), 2)
	}
	if metrics.DefenderLost > 0 {
		metrics.DefenseTradeRatio = roundFloat(float64(metrics.AttackerLost)/float64(metrics.DefenderLost), 2)
	}
	return metrics
}

func playerInfoByOID(bls map[string]interface{}) map[int64]map[string]interface{} {
	out := make(map[int64]map[string]interface{})
	rows, _ := bls["PI"].([]interface{})
	for _, raw := range rows {
		info, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		oid, ok := int64FromValue(info["OID"])
		if ok {
			out[oid] = info
		}
	}
	return out
}

func combatantFromPlayerInfo(oid int64, role string, info map[string]interface{}) Combatant {
	name := stringFromValue(info["N"])
	if name == "" {
		return Combatant{}
	}
	alliance := stringFromValue(info["AN"])
	allianceID, _ := int64FromValue(info["AID"])
	return Combatant{
		PlayerID:     oid,
		Name:         name,
		PlayerName:   name,
		AllianceID:   allianceID,
		Alliance:     alliance,
		AllianceName: alliance,
		Role:         role,
	}
}

func effectsFromLeader(value interface{}, side string, combatMode battleEffectCombatMode) []Effect {
	leader, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	rows := leaderEffectRows(leader, combatMode)
	if len(rows) == 0 {
		return nil
	}
	type groupedSource struct {
		value  float64
		cap    float64
		hasCap bool
	}
	type groupedEffect struct {
		effect  Effect
		meta    battleEffectMeta
		sources map[string]groupedSource
	}
	grouped := map[string]*groupedEffect{}
	for _, row := range rows {
		meta := battleEffectDisplay(row.id, side)
		if meta.skip || (meta.unknown && row.source != battleEffectSourceActive) {
			continue
		}
		value := battleEffectValue(row.id, row.values)
		if meta.scale != 0 {
			value *= meta.scale
		}
		if value == 0 {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%s", meta.label, meta.unit, meta.category, meta.template)
		capKey := fmt.Sprintf("%d:%s", row.id, row.capKey)
		if !row.hasCap {
			capKey = fmt.Sprintf("%d:%s:%d:%d", row.id, row.source, row.rawID, row.index)
		}
		entry := grouped[key]
		if entry == nil {
			entry = &groupedEffect{
				effect: Effect{
					Code:      fmt.Sprintf("%d", row.rawID),
					Label:     meta.label,
					Name:      meta.label,
					Category:  meta.category,
					SortOrder: meta.order,
					Side:      side,
				},
				meta:    meta,
				sources: map[string]groupedSource{},
			}
			grouped[key] = entry
		} else {
			entry.effect.Code += "," + fmt.Sprintf("%d", row.rawID)
			if meta.order < entry.effect.SortOrder {
				entry.effect.SortOrder = meta.order
			}
		}
		source := entry.sources[capKey]
		source.value += value
		if row.hasCap {
			source.cap = row.cap
			source.hasCap = true
		}
		entry.sources[capKey] = source
	}
	effects := make([]Effect, 0, len(grouped))
	for _, entry := range grouped {
		var value float64
		for _, source := range entry.sources {
			if source.hasCap {
				capped := math.Min(math.Abs(source.value), source.cap)
				if source.value < 0 {
					capped = -capped
				}
				value += capped
			} else {
				value += source.value
			}
		}
		entry.effect.Value = roundFloat(value, 1)
		entry.effect.FormattedValue = formatBattleEffectValue(entry.effect.Value, entry.meta.unit, entry.meta.negative)
		entry.effect.DisplayText = formatBattleEffectText(entry.effect.Value, entry.meta)
		effects = append(effects, entry.effect)
	}
	sort.SliceStable(effects, func(i, j int) bool {
		if effects[i].SortOrder != effects[j].SortOrder {
			return effects[i].SortOrder < effects[j].SortOrder
		}
		return effects[i].Label < effects[j].Label
	})
	return effects
}

type battleEffectCombatMode string
type battleEffectSource string

const (
	battleEffectCombatPVP battleEffectCombatMode = "pvp"
	battleEffectCombatPVE battleEffectCombatMode = "pve"

	battleEffectSourceActive    battleEffectSource = "active"
	battleEffectSourceEquipment battleEffectSource = "equipment"
	battleEffectSourceGem       battleEffectSource = "gem"
	battleEffectSourceSet       battleEffectSource = "set"
)

type leaderEffectRow struct {
	id     int64
	rawID  int64
	values []float64
	source battleEffectSource
	index  int
	cap    float64
	hasCap bool
	capKey string
}

type battleEffectDefinition struct {
	effectID int64
	capID    int64
	cap      float64
	hasCap   bool
	isPVP    bool
	isPVE    bool
}

type battleCatalogEffect struct {
	id     int64
	values []float64
}

type battleEquipmentSetEffect struct {
	needed  int64
	effects []battleCatalogEffect
}

type battleEffectDataCache struct {
	equipmentEffectIDs  map[int64]int64
	effectDefinitions   map[int64]battleEffectDefinition
	relicEffectIDs      map[int64]int64
	equipmentEffects    map[int64][]battleCatalogEffect
	gemEffects          map[int64][]battleCatalogEffect
	equipmentSetEffects map[int64][]battleEquipmentSetEffect
}

var (
	battleEffectDataOnce sync.Once
	battleEffectData     battleEffectDataCache
)

func leaderEffectRows(leader map[string]interface{}, combatMode battleEffectCombatMode) []leaderEffectRow {
	rows := []leaderEffectRow{}
	index := 0
	for _, raw := range arrayFromValue(leader["AE"]) {
		row := arrayFromValue(raw)
		if parsed, ok := effectRowFromArray(row, battleEffectSourceActive, combatMode, index); ok {
			rows = append(rows, parsed)
		}
		index++
	}

	setCounts := map[int64]int64{}
	data := loadBattleEffectData()
	for _, raw := range arrayFromValue(leader["EQ"]) {
		item := arrayFromValue(raw)
		setID := int64FromValueDefault(rowValue(item, 7))
		if setID > 0 {
			setCounts[setID]++
		}

		equipmentID := int64FromValueDefault(rowValue(item, 6))
		catalogEffects := data.equipmentEffects[equipmentID]
		for effectIndex, effectRaw := range arrayFromValue(rowValue(item, 5)) {
			effectRow := arrayFromValue(effectRaw)
			rawID := int64FromValueDefault(rowValue(effectRow, 0))
			values := battleEffectValuesFromRow(effectRow)
			if effectIndex < len(catalogEffects) {
				rawID = catalogEffects[effectIndex].id
				if len(values) == 0 {
					values = catalogEffects[effectIndex].values
				}
			}
			if parsed, ok := normalizedLeaderEffectRow(rawID, values, battleEffectSourceEquipment, combatMode, index); ok {
				rows = append(rows, parsed)
			}
			index++
		}

		catalogGemID := int64FromValueDefault(rowValue(item, 10))
		for _, effect := range data.gemEffects[catalogGemID] {
			if parsed, ok := normalizedLeaderEffectRow(effect.id, effect.values, battleEffectSourceGem, combatMode, index); ok {
				rows = append(rows, parsed)
			}
			index++
		}

		gemRow := arrayFromValue(rowValue(arrayFromValue(rowValue(arrayFromValue(rowValue(item, 12)), 3)), 4))
		for _, effectRaw := range gemRow {
			effectRow := arrayFromValue(effectRaw)
			if parsed, ok := normalizedLeaderEffectRow(
				int64FromValueDefault(rowValue(effectRow, 0)),
				battleEffectValuesFromRow(effectRow),
				battleEffectSourceGem,
				combatMode,
				index,
			); ok {
				rows = append(rows, parsed)
			}
			index++
		}
	}

	for setID, count := range setCounts {
		for _, effect := range battleEffectSetEffects(setID, count) {
			if parsed, ok := normalizedLeaderEffectRow(effect.id, effect.values, battleEffectSourceSet, combatMode, index); ok {
				rows = append(rows, parsed)
			}
			index++
		}
	}

	return rows
}

func effectRowFromArray(row []interface{}, source battleEffectSource, combatMode battleEffectCombatMode, index int) (leaderEffectRow, bool) {
	return normalizedLeaderEffectRow(int64FromValueDefault(rowValue(row, 0)), battleEffectValuesFromRow(row), source, combatMode, index)
}

func normalizedLeaderEffectRow(rawID int64, values []float64, source battleEffectSource, combatMode battleEffectCombatMode, index int) (leaderEffectRow, bool) {
	if rawID == 0 || len(values) == 0 {
		return leaderEffectRow{}, false
	}
	data := loadBattleEffectData()
	id := rawID
	if source != battleEffectSourceActive {
		if mapped := data.relicEffectIDs[rawID]; mapped > 0 {
			id = mapped
		} else if mapped := data.equipmentEffectIDs[rawID]; mapped > 0 {
			id = mapped
		}
	}
	definition, hasDefinition := data.effectDefinitions[id]
	if hasDefinition {
		if definition.isPVP && combatMode != battleEffectCombatPVP {
			return leaderEffectRow{}, false
		}
		if definition.isPVE && combatMode != battleEffectCombatPVE {
			return leaderEffectRow{}, false
		}
	}
	capValue, hasCap := battleEffectCap(id, definition, hasDefinition, source)
	capKey := fmt.Sprintf("%d", id)
	if hasDefinition {
		capKey = fmt.Sprintf("%d:%d", definition.effectID, definition.capID)
	}
	return leaderEffectRow{
		id:     id,
		rawID:  rawID,
		values: values,
		source: source,
		index:  index,
		cap:    capValue,
		hasCap: hasCap,
		capKey: capKey,
	}, true
}

func battleEffectValuesFromRow(row []interface{}) []float64 {
	if len(row) > 2 {
		if values := floatSliceFromValue(row[2]); len(values) > 0 {
			return values
		}
	}
	return battleEffectValuesFromValue(rowValue(row, 1))
}

func battleEffectCap(id int64, definition battleEffectDefinition, hasDefinition bool, source battleEffectSource) (float64, bool) {
	if source == battleEffectSourceActive && !battleEffectActiveCaps[id] {
		return 0, false
	}
	if override, ok := battleEffectCapOverrides[id]; ok {
		return override, true
	}
	if hasDefinition && definition.hasCap {
		return definition.cap, true
	}
	return 0, false
}

func loadBattleEffectData() battleEffectDataCache {
	battleEffectDataOnce.Do(func() {
		battleEffectData = buildBattleEffectData()
	})
	return battleEffectData
}

func buildBattleEffectData() battleEffectDataCache {
	data := battleEffectDataCache{
		equipmentEffectIDs:  map[int64]int64{},
		effectDefinitions:   map[int64]battleEffectDefinition{},
		relicEffectIDs:      map[int64]int64{},
		equipmentEffects:    map[int64][]battleCatalogEffect{},
		gemEffects:          map[int64][]battleCatalogEffect{},
		equipmentSetEffects: map[int64][]battleEquipmentSetEffect{},
	}

	caps := map[int64]float64{}
	for _, entry := range readBattleDataArray(serverdata.ReadEffectCapsItemsJSON) {
		capID := int64FromValueDefault(entry["capID"])
		max := numberFromValue(entry["maxTotalBonus"])
		if capID > 0 && max > 0 {
			caps[capID] = max
		}
	}

	for _, entry := range readBattleDataArray(serverdata.ReadEffectsItemsJSON) {
		effectID := int64FromValueDefault(entry["effectID"])
		if effectID == 0 {
			continue
		}
		capID := int64FromValueDefault(entry["capID"])
		cap, hasCap := caps[capID]
		name := stringFromValue(entry["name"])
		data.effectDefinitions[effectID] = battleEffectDefinition{
			effectID: effectID,
			capID:    capID,
			cap:      cap,
			hasCap:   hasCap,
			isPVP:    stringFromValue(entry["isPvPFight"]) == "1" || strings.Contains(strings.ToLower(name), "pvp"),
			isPVE:    stringFromValue(entry["isPvEFight"]) == "1" || strings.Contains(strings.ToLower(name), "pve"),
		}
	}

	for _, entry := range readBattleDataArray(serverdata.ReadEquipmentEffectsItemsJSON) {
		equipmentEffectID := int64FromValueDefault(entry["equipmentEffectID"])
		effectID := int64FromValueDefault(entry["effectID"])
		if equipmentEffectID > 0 && effectID > 0 {
			data.equipmentEffectIDs[equipmentEffectID] = effectID
		}
	}

	for _, entry := range readBattleDataArray(serverdata.ReadRelicEffectsItemsJSON) {
		relicEffectID := int64FromValueDefault(entry["id"])
		effectID := int64FromValueDefault(entry["effectID"])
		if relicEffectID > 0 && effectID > 0 {
			data.relicEffectIDs[relicEffectID] = effectID
		}
	}

	for _, entry := range readBattleDataArray(serverdata.ReadEquipmentsItemsJSON) {
		equipmentID := int64FromValueDefault(entry["equipmentID"])
		effects := catalogEffectsFromString(stringFromValue(entry["effects"]))
		if equipmentID > 0 && len(effects) > 0 {
			data.equipmentEffects[equipmentID] = effects
		}
	}

	for _, entry := range readBattleDataArray(serverdata.ReadGemsItemsJSON) {
		gemID := int64FromValueDefault(entry["gemID"])
		effects := catalogEffectsFromString(stringFromValue(entry["effects"]))
		if gemID > 0 && len(effects) > 0 {
			data.gemEffects[gemID] = effects
		}
	}

	for _, entry := range readBattleDataArray(serverdata.ReadEquipmentSetsItemsJSON) {
		setID := int64FromValueDefault(entry["setID"])
		needed := int64FromValueDefault(entry["neededItems"])
		effects := catalogEffectsFromString(stringFromValue(entry["effects"]))
		if setID > 0 && needed > 0 && len(effects) > 0 {
			data.equipmentSetEffects[setID] = append(data.equipmentSetEffects[setID], battleEquipmentSetEffect{needed: needed, effects: effects})
		}
	}

	return data
}

func battleEffectSetEffects(setID, equippedCount int64) []battleCatalogEffect {
	if setID <= 0 || equippedCount <= 0 {
		return nil
	}
	var out []battleCatalogEffect
	for _, entry := range loadBattleEffectData().equipmentSetEffects[setID] {
		if entry.needed <= equippedCount {
			out = append(out, entry.effects...)
		}
	}
	return out
}

func readBattleDataArray(read func() ([]byte, error)) []map[string]interface{} {
	raw, err := read()
	if err != nil {
		return nil
	}
	var rows []map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&rows); err != nil {
		return nil
	}
	return rows
}

func catalogEffectsFromString(value string) []battleCatalogEffect {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]battleCatalogEffect, 0, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(part, "&", 2)
		id := int64FromString(strings.TrimSpace(pair[0]))
		if id <= 0 || len(pair) < 2 {
			continue
		}
		var values []float64
		for _, valuePart := range strings.FieldsFunc(pair[1], func(r rune) bool { return r == '#' || r == '+' }) {
			value := numberFromString(strings.TrimSpace(valuePart))
			if value != 0 {
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			out = append(out, battleCatalogEffect{id: id, values: values})
		}
	}
	return out
}

func applyBattleWaves(report *ParsedReport, data map[string]interface{}) {
	if report == nil || data == nil || len(report.Waves) > 0 {
		return
	}
	rawWaves, _ := data["W"].([]interface{})
	for waveIndex, rawWave := range rawWaves {
		waveRows, ok := rawWave.([]interface{})
		if !ok {
			continue
		}
		wave := Wave{Index: waveIndex, Wave: waveIndex + 1}
		for laneIndex := 0; laneIndex < 3; laneIndex++ {
			laneNameValue := laneName(laneIndex)
			lane := WaveLane{Lane: laneNameValue}
			for _, sideRaw := range waveRows {
				sideRow, ok := sideRaw.([]interface{})
				if !ok || len(sideRow) <= laneIndex+1 {
					continue
				}
				side, _ := int64FromValue(rowValue(sideRow, 0))
				sideRole := sideRoleFromWaves(report, side)
				if sideRole == "" {
					continue
				}
				laneSummary := rowValue(sideRow, laneIndex+1)
				started, lost := laneTotals(laneSummary)
				units, tools := laneItemDetails(laneSummary, sideRole, laneNameValue)
				if len(units) > 0 {
					started, lost = battleItemTotals(units)
					report.TopUnits = append(report.TopUnits, units...)
				}
				if len(tools) > 0 {
					report.SupportTools = append(report.SupportTools, tools...)
				}
				if sideRole == "attacker" {
					lane.AttackerStart += started
					lane.AttackerLost += lost
					lane.AttackerUnitDetails = append(lane.AttackerUnitDetails, units...)
					lane.AttackerToolDetails = append(lane.AttackerToolDetails, tools...)
				} else if sideRole == "defender" {
					lane.DefenderStart += started
					lane.DefenderLost += lost
					lane.DefenderUnitDetails = append(lane.DefenderUnitDetails, units...)
					lane.DefenderToolDetails = append(lane.DefenderToolDetails, tools...)
				}
			}
			lane.Result = inferLaneResult(lane)
			wave.Lanes = append(wave.Lanes, lane)
		}
		report.Waves = append(report.Waves, wave)
	}
}

func applyBattleItemSummaries(report *ParsedReport, data map[string]interface{}) {
	if report == nil || data == nil {
		return
	}
	for _, raw := range arrayFromValue(data["Y"]) {
		row, ok := raw.([]interface{})
		if !ok || len(row) < 2 {
			continue
		}
		oid, _ := int64FromValue(rowValue(row, 0))
		side := sideRoleFromWaves(report, oid)
		if side == "" {
			continue
		}
		report.TopUnits = append(report.TopUnits, battleItemRows(row[1:], side, "courtyard", "", "unit")...)
	}
	for _, raw := range arrayFromValue(data["S"]) {
		row, ok := raw.([]interface{})
		if !ok || len(row) < 2 {
			continue
		}
		oid, _ := int64FromValue(rowValue(row, 0))
		side := sideRoleFromWaves(report, oid)
		if side == "" {
			continue
		}
		report.SupportTools = append(report.SupportTools, battleItemRows(row[1:], side, "support", "", "tool")...)
	}
}

func laneTotals(value interface{}) (int64, int64) {
	row, ok := value.([]interface{})
	if !ok || len(row) < 3 || !isNumber(row[0]) {
		return 0, 0
	}
	started := int64Abs(int64(numberFromValue(row[0])))
	lostValue := numberFromValue(row[2])
	lost := int64(0)
	if lostValue < 0 {
		lost = int64(-lostValue)
	}
	return started, lost
}

func laneItemDetails(value interface{}, side, lane string) ([]BattleItemDetail, []BattleItemDetail) {
	groups, ok := value.([]interface{})
	if !ok || len(groups) == 0 || isNumber(groups[0]) {
		return nil, nil
	}
	units := battleItemRowsFromGroup(groups[0], side, "wall", lane, "unit")
	var tools []BattleItemDetail
	for _, rawGroup := range groups[1:] {
		tools = append(tools, battleItemRowsFromGroup(rawGroup, side, "wall", lane, "tool")...)
	}
	return units, tools
}

func battleItemRowsFromGroup(group interface{}, side, phase, lane, kind string) []BattleItemDetail {
	return battleItemRows(arrayFromValue(group), side, phase, lane, kind)
}

func battleItemRows(rows []interface{}, side, phase, lane, kind string) []BattleItemDetail {
	items := make([]BattleItemDetail, 0, len(rows))
	for _, raw := range rows {
		row, ok := raw.([]interface{})
		if !ok || len(row) < 3 {
			continue
		}
		wodID, ok := int64FromValue(rowValue(row, 0))
		if !ok || wodID <= 0 {
			continue
		}
		amount := int64Abs(int64(numberFromValue(rowValue(row, 1))))
		delta := numberFromValue(rowValue(row, 2))
		usedOrLost := int64(0)
		if delta < 0 {
			usedOrLost = int64(-delta)
		}
		item := BattleItemDetail{Side: side, Phase: phase, Lane: lane, WodID: wodID, Amount: amount}
		if kind == "tool" {
			item.Used = usedOrLost
			if item.Amount == 0 && item.Used == 0 {
				continue
			}
		} else {
			item.Lost = usedOrLost
			if item.Amount == 0 && item.Lost == 0 {
				continue
			}
		}
		items = append(items, item)
	}
	return items
}

func battleItemTotals(items []BattleItemDetail) (int64, int64) {
	var started int64
	var lost int64
	for _, item := range items {
		started += item.Amount
		lost += item.Lost
	}
	return started, lost
}

func aggregateBattleItems(items []BattleItemDetail) []BattleItemDetail {
	if len(items) == 0 {
		return nil
	}
	byKey := map[string]*BattleItemDetail{}
	order := make([]string, 0, len(items))
	for _, item := range items {
		if item.WodID <= 0 {
			continue
		}
		key := fmt.Sprintf("%s|%s|%d", item.Side, item.Phase, item.WodID)
		entry := byKey[key]
		if entry == nil {
			copy := item
			copy.Lane = ""
			byKey[key] = &copy
			order = append(order, key)
			continue
		}
		entry.Amount += item.Amount
		entry.Lost += item.Lost
		entry.Used += item.Used
	}
	out := make([]BattleItemDetail, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].Lost + out[i].Used
		right := out[j].Lost + out[j].Used
		if left != right {
			return left > right
		}
		return out[i].Amount > out[j].Amount
	})
	return out
}

func inferLaneResult(lane WaveLane) string {
	if lane.DefenderStart > 0 && lane.DefenderLost < lane.DefenderStart {
		return "HELD"
	}
	if lane.DefenderStart > 0 && lane.DefenderLost >= lane.DefenderStart {
		return "BREACHED"
	}
	if lane.AttackerStart > 0 && lane.AttackerLost < lane.AttackerStart {
		return "BREACHED"
	}
	return "HELD"
}

func arrayFromValue(value interface{}) []interface{} {
	rows, _ := value.([]interface{})
	return rows
}

func isNumber(value interface{}) bool {
	switch value.(type) {
	case int, int64, float64, json.Number:
		return true
	default:
		return false
	}
}

func sideRoleFromWaves(report *ParsedReport, oid int64) string {
	if report.Attacker != nil && report.Attacker.PlayerID == oid {
		return "attacker"
	}
	if report.Defender != nil && report.Defender.PlayerID == oid {
		return "defender"
	}
	if oid < 0 {
		return "defender"
	}
	return ""
}

func ReportHasBothPlayers(report ParsedReport) bool {
	return combatantHasPlayer(report.Attacker) && combatantHasPlayer(report.Defender)
}

func combatantHasPlayer(combatant *Combatant) bool {
	return combatant != nil && (combatant.PlayerID > 0 || strings.TrimSpace(combatant.Name) != "" || strings.TrimSpace(combatant.PlayerName) != "")
}

func inferBinaryResult(metrics Metrics) string {
	if metrics.AttackerSent > 0 && metrics.AttackerLost >= metrics.AttackerSent {
		return "Defeat"
	}
	return "Victory"
}

func handleListReports(w http.ResponseWriter, r *http.Request) {
	reports, err := ReadParsedReports()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"reports": reports})
}

func handleListCloudReports(w http.ResponseWriter, r *http.Request) {
	reports, err := FetchCloudParsedReports(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"reports": reports})
}

func handlePostReport(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var capture Capture
	if err := json.NewDecoder(r.Body).Decode(&capture); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	parsed := ParseCapture(&capture)
	if capture.BLS == nil || parsed.ID == "" || !ReportHasBothPlayers(parsed) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "discarded": true, "id": capture.ID})
		return
	}
	if err := UpsertCapture(capture); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "id": capture.ID})
}

func parsedReportsFromCloudPayload(value interface{}) []ParsedReport {
	switch v := value.(type) {
	case []interface{}:
		var reports []ParsedReport
		for _, item := range v {
			reports = append(reports, parsedReportsFromCloudPayload(item)...)
		}
		return reports
	case string:
		return parsedReportsFromCloudString(v)
	case map[string]interface{}:
		for _, key := range []string{"reports", "data", "items"} {
			if nested, ok := v[key]; ok {
				return parsedReportsFromCloudPayload(nested)
			}
		}
		if rawPayload, ok := v["payload"].(string); ok {
			return parsedReportsFromCloudString(rawPayload)
		}
		for _, key := range []string{"parsed", "parsedReport", "report"} {
			if nested, ok := v[key]; ok {
				return parsedReportsFromCloudPayload(nested)
			}
		}
		if report, ok := parsedReportFromCloudMap(v); ok {
			return []ParsedReport{report}
		}
		if report, ok := parsedCaptureFromCloudMap(v); ok {
			return []ParsedReport{report}
		}
	}
	return nil
}

func parsedReportsFromCloudString(value string) []ParsedReport {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var payload interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	return parsedReportsFromCloudPayload(payload)
}

func parsedReportFromCloudMap(value map[string]interface{}) (ParsedReport, bool) {
	raw, err := json.Marshal(value)
	if err != nil {
		return ParsedReport{}, false
	}
	var report ParsedReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return ParsedReport{}, false
	}
	if report.ID == "" {
		report.ID = report.ReportID
	}
	if report.ReportID == "" {
		report.ReportID = report.ID
	}
	return report, report.ID != "" && ReportHasBothPlayers(report)
}

func parsedCaptureFromCloudMap(value map[string]interface{}) (ParsedReport, bool) {
	if _, ok := value["bls"]; !ok {
		if _, ok := value["BLS"]; !ok {
			return ParsedReport{}, false
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ParsedReport{}, false
	}
	var capture Capture
	if err := json.Unmarshal(raw, &capture); err != nil {
		return ParsedReport{}, false
	}
	report := ParseCapture(&capture)
	return report, report.ID != "" && ReportHasBothPlayers(report)
}

func battleEffectDisplay(id int64, side string) battleEffectMeta {
	if meta, ok := battleEffectDisplayByID[id]; ok {
		return meta
	}
	label := fmt.Sprintf("Effect %d", id)
	return battleEffectMeta{label: label, template: label, unit: "percent", category: "Other effects", order: 900}
}

type battleEffectMeta struct {
	label    string
	template string
	unit     string
	category string
	order    int
	negative bool
	scale    float64
	skip     bool
	unknown  bool
}

var battleEffectActiveCaps = map[int64]bool{
	5: true, 61: true, 62: true, 368: true, 369: true, 386: true, 410: true, 411: true, 412: true, 423: true, 424: true,
}

var battleEffectCapOverrides = map[int64]float64{
	473: 150,
}

var battleEffectDisplayByID = map[int64]battleEffectMeta{
	61:  battleEffect("Combat strength for melee units", "combat strength for melee units", "percent", "Unit effects", 10),
	411: battleEffect("Combat strength for melee units", "combat strength for melee units", "percent", "Unit effects", 10),
	613: battleEffect("Combat strength for melee units", "combat strength for melee units", "percent", "Unit effects", 10),
	467: battleEffect("Combat strength for melee units", "combat strength for melee units", "percent", "Unit effects", 10),
	62:  battleEffect("Combat strength for ranged units", "combat strength for ranged units", "percent", "Unit effects", 11),
	412: battleEffect("Combat strength for ranged units", "combat strength for ranged units", "percent", "Unit effects", 11),
	614: battleEffect("Combat strength for ranged units", "combat strength for ranged units", "percent", "Unit effects", 11),
	468: battleEffect("Combat strength for ranged units", "combat strength for ranged units", "percent", "Unit effects", 11),
	410: battleEffect("Combat strength when attacking enemy courtyard", "combat strength when attacking enemy courtyard", "percent", "Unit effects", 12),
	504: battleEffect("Combat strength when attacking enemy courtyard", "combat strength when attacking enemy courtyard", "percent", "Unit effects", 12),
	386: battleEffect("Combat strength when attacking enemy courtyard", "combat strength when attacking enemy courtyard", "percent", "Unit effects", 12),
	475: battleEffect("Combat strength when attacking enemy courtyard", "combat strength when attacking enemy courtyard", "percent", "Unit effects", 12),
	423: battleEffect("Combat strength for the front", "combat strength for the front when attacking", "percent", "Unit effects", 13),
	478: battleEffect("Combat strength for the front", "combat strength for the front when attacking", "percent", "Unit effects", 13),
	424: battleEffect("Combat strength for the flanks", "combat strength for the flanks when attacking", "percent", "Unit effects", 14),
	479: battleEffect("Combat strength for the flanks", "combat strength for the flanks when attacking", "percent", "Unit effects", 14),
	477: battleEffect("Unit combat strength when attacking", "unit combat strength when attacking", "percent", "Attack effects", 15),
	469: battleEffectNegative("Wall protection", "wall protection of Castle Lords", "percent", "Defense structure effects", 16),
	470: battleEffectNegative("Gate protection", "gate protection of Castle Lords", "percent", "Defense structure effects", 17),
	471: battleEffectNegative("Moat protection", "moat protection of Castle Lords", "percent", "Defense structure effects", 18),
	503: battleEffect("Front unit limit", "unit limit on the front", "percent", "Attack effects", 20),
	369: battleEffect("Front unit limit", "unit limit on the front", "percent", "Attack effects", 20),
	476: battleEffect("Front unit limit", "unit limit on the front", "percent", "Attack effects", 20),
	66:  battleEffect("Flank unit limit", "unit limit on the flanks", "percent", "Attack effects", 21),
	368: battleEffect("Flank unit limit", "flank unit limit when attacking", "percent", "Attack effects", 21),
	474: battleEffect("Flank unit limit", "unit limit on the flanks", "percent", "Attack effects", 21),
	700: battleEffect("Final assault capacity", "to troop capacity for final assault", "number", "Courtyard effects", 22),
	701: battleEffect("Final assault capacity", "to troop capacity for final assault", "percent", "Courtyard effects", 23),
	512: battleEffect("Courtyard support strength", "courtyard support unit strength", "percent", "Courtyard effects", 24),
	29:  battleEffect("Additional waves", "additional wave(s)", "number", "Pre-battle effects", 29),
	484: battleEffect("Additional waves", "additional wave(s)", "number", "Pre-battle effects", 29),
	426: battleEffect("Army travel speed", "army travel speed", "percent", "Pre-battle effects", 30),
	53:  battleEffect("Army travel speed", "army travel speed", "percent", "Pre-battle effects", 30),
	472: battleEffect("Army travel speed", "army travel speed", "percent", "Pre-battle effects", 30),
	97:  battleEffect("Travel speed", "Military, espionage and trade travel speed", "percent", "Pre-battle effects", 30),
	19:  battleEffect("Attack travel speed", "Attack travel speed", "percent", "Pre-battle effects", 31),
	55:  battleEffect("Later army detection", "later army detection", "percent", "Pre-battle effects", 32),
	481: battleEffect("Later army detection", "later army detection", "percent", "Pre-battle effects", 32),
	482: battleEffect("Army return travel speed", "army return travel speed against Castle Lords", "percent", "Post-battle effects", 33),
	111: battleEffect("Loot capacity", "loot capacity", "percent", "Post-battle effects", 40),
	431: battleEffect("Resources plundered", "resources plundered when looting", "percent", "Post-battle effects", 41),
	54:  battleEffect("Resources plundered", "resources plundered when looting", "percent", "Post-battle effects", 41),
	51:  battleEffect("Glory earned", "glory points earned when attacking", "percent", "Post-battle effects", 42),
	100: battleEffect("Glory bonus", "Glory bonus", "percent", "Post-battle effects", 42),
	45:  battleEffect("Glory earned", "glory points earned when attacking", "percent", "Post-battle effects", 42),
	22:  battleEffect("Glory earned", "glory points earned when attacking", "percent", "Post-battle effects", 42),
	52:  battleEffect("Honor earned", "honor points earned in battle", "percent", "Post-battle effects", 43),
	82:  battleEffect("XP earned", "XP earned in battle", "percent", "Post-battle effects", 44),
	112: battleEffect("XP earned", "XP earned in battle", "percent", "Post-battle effects", 44),
	43:  battleEffect("Coin loot", "Coins looted from NPC targets", "percent", "Post-battle effects", 45),
	60:  battleEffect("Equipment find", "chance of finding better equipment", "percent", "Post-battle effects", 46),
	48:  battleEffect("Attack strength bonus", "combat strength when attacking", "percent", "Attack effects", 49),
	20:  battleEffect("Alliance attack strength", "Combat strength bonus for attacks", "percent", "Attack effects", 50),
	25:  battleEffect("Event target attack strength", "Combat strength bonus against Foreign and Bloodcrow castles", "percent", "Attack effects", 51),

	473: battleEffect("Resources plundered", "resources plundered when looting", "percent", "Post-battle effects", 41),

	339:   battleEffect("Combat strength for defensive melee units", "combat strength bonus for defensive melee units", "percent", "Unit effects", 110),
	10:    battleEffect("Combat strength for defensive melee units", "combat strength bonus for defensive melee units", "percent", "Unit effects", 110),
	12105: battleEffect("Combat strength for defensive melee units", "combat strength bonus for defensive melee units", "percent", "Unit effects", 110),
	12203: battleEffect("Combat strength for defensive melee units", "combat strength bonus for defensive melee units", "percent", "Unit effects", 110),
	12303: battleEffect("Combat strength for defensive melee units", "combat strength bonus for defensive melee units", "percent", "Unit effects", 110),
	12507: battleEffect("Combat strength for defensive melee units", "combat strength bonus for defensive melee units", "percent", "Unit effects", 110),
	340:   battleEffect("Combat strength for defensive ranged units", "combat strength bonus for defensive ranged units", "percent", "Unit effects", 111),
	11:    battleEffect("Combat strength for defensive ranged units", "combat strength bonus for defensive ranged units", "percent", "Unit effects", 111),
	12106: battleEffect("Combat strength for defensive ranged units", "combat strength bonus for defensive ranged units", "percent", "Unit effects", 111),
	12204: battleEffect("Combat strength for defensive ranged units", "combat strength bonus for defensive ranged units", "percent", "Unit effects", 111),
	12304: battleEffect("Combat strength for defensive ranged units", "combat strength bonus for defensive ranged units", "percent", "Unit effects", 111),
	12508: battleEffect("Combat strength for defensive ranged units", "combat strength bonus for defensive ranged units", "percent", "Unit effects", 111),
	370:   battleEffect("Combat strength when defending the courtyard", "combat strength when defending the courtyard", "percent", "Unit effects", 112),
	501:   battleEffect("Combat strength when defending the courtyard", "combat strength when defending the courtyard", "percent", "Unit effects", 112),
	12108: battleEffect("Combat strength when defending the courtyard", "combat strength when defending the courtyard", "percent", "Unit effects", 112),
	12206: battleEffect("Combat strength when defending the courtyard", "combat strength when defending the courtyard", "percent", "Unit effects", 112),
	12306: battleEffect("Combat strength when defending the courtyard", "combat strength when defending the courtyard", "percent", "Unit effects", 112),
	12510: battleEffect("Combat strength when defending the courtyard", "combat strength when defending the courtyard", "percent", "Unit effects", 112),
	12109: battleEffect("Combat strength for defense units", "combat strength for defense units", "percent", "Unit effects", 113),
	12501: battleEffect("Combat strength for defense units", "combat strength for defense units", "percent", "Unit effects", 113),
	509:   battleEffect("Front defense", "combat strength on the front when defending", "percent", "Defense unit effects", 113),
	510:   battleEffect("Flank defense", "combat strength on the flanks when defending", "percent", "Defense unit effects", 114),
	12111: battleEffect("Flank defense", "combat strength for defense units of the flanks", "percent", "Defense unit effects", 114),
	12102: battleEffect("Wall protection", "wall protection", "percent", "Defense structure effects", 115),
	12103: battleEffect("Gate protection", "gate protection", "percent", "Defense structure effects", 116),
	12104: battleEffect("Moat protection", "moat protection", "percent", "Defense structure effects", 117),
	420:   battleEffect("Wall unit limit", "unit limit on the wall", "number", "Defense unit effects", 120),
	387:   battleEffect("Wall unit limit", "to troop capacity on wall defense", "percent", "Defense unit effects", 120),
	12107: battleEffect("Wall unit limit", "wall unit limit when defending", "percent", "Defense unit effects", 120),
	702:   battleEffect("Courtyard defense capacity", "to troop capacity in courtyard defense", "number", "Courtyard effects", 121),
	371:   battleEffect("Courtyard defense capacity", "to troop capacity in courtyard defense", "number", "Courtyard effects", 121),
	705:   battleEffect("Courtyard defense capacity", "to troop capacity in courtyard defense", "percent", "Courtyard effects", 121),
	706:   battleEffect("Alliance support capacity", "to alliance support troop capacity", "number", "Courtyard effects", 122),
	385:   battleEffect("Alliance support capacity", "to alliance support troop capacity", "number", "Courtyard effects", 122),
	12112: battleEffect("Protector support", "Level 10 Protector of the north in courtyard defense", "number", "Defense unit effects", 122),
	427:   battleEffect("Surviving soldiers", "more surviving soldiers after defense", "percent", "Post-battle effects", 123),
	428:   battleEffect("Sight radius", "Sight Radius", "percent", "Pre-battle effects", 124),
	4:     battleEffect("Earlier attack warning", "earlier attack warning", "percent", "Pre-battle effects", 125),
	12503: battleEffect("Earlier attack warning", "earlier attack warning when defending against Castle Lords", "percent", "Pre-battle effects", 125),
	5:     battleEffectNegative("Fire damage suffered", "fire damage suffered when defending", "percent", "Defense structure effects", 126),
	429:   battleEffectNegative("Fire damage suffered", "fire damage suffered when defending", "percent", "Defense structure effects", 126),
	12309: battleEffectNegative("Fire damage suffered", "fire damage suffered when defending", "percent", "Defense structure effects", 126),
	12511: battleEffectNegative("Fire damage suffered", "fire damage suffered when defending", "percent", "Defense structure effects", 126),
	12101: battleEffectNegative("Resources lost", "resources lost after being looted", "percent", "Post-battle effects", 126),
	94:    battleEffect("Militia in the castle", "militia in the castle", "number", "Pre-battle effects", 127),
	115:   battleEffectFlag("Militia replacement", "Replaces the armed citizen with Militia", "Pre-battle effects", 128),
	1:     battleEffect("Glory defense bonus", "glory points earned when defending", "percent", "Post-battle effects", 129),
	21:    battleEffect("Alliance defense strength", "Combat strength bonus for defense", "percent", "Defense unit effects", 130),
	27:    battleEffect("Khan defense strength", "Combat strength bonus against Khan attacks", "percent", "Defense unit effects", 131),
}

func battleEffect(label, template, unit, category string, order int) battleEffectMeta {
	return battleEffectMeta{label: label, template: template, unit: unit, category: category, order: order}
}

func battleEffectNegative(label, template, unit, category string, order int) battleEffectMeta {
	return battleEffectMeta{label: label, template: template, unit: unit, category: category, order: order, negative: true}
}

func battleEffectFlag(label, template, category string, order int) battleEffectMeta {
	return battleEffectMeta{label: label, template: template, unit: "flag", category: category, order: order}
}

func formatBattleEffectValue(value float64, unit string, negative bool) string {
	if unit == "flag" {
		return ""
	}
	if negative {
		value = -math.Abs(value)
	}
	prefix := ""
	if value > 0 {
		prefix = "+"
	}
	if unit == "number" {
		return fmt.Sprintf("%s%.0f", prefix, value)
	}
	return fmt.Sprintf("%s%.1f%%", prefix, value)
}

func formatBattleEffectText(value float64, meta battleEffectMeta) string {
	if meta.unit == "flag" {
		return meta.template
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s", formatBattleEffectValue(value, meta.unit, meta.negative), meta.template))
}

func battleEffectValue(id int64, values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if len(values) > 1 && len(values)%2 == 0 && values[0] > 100 {
		var sum float64
		for i := 1; i < len(values); i += 2 {
			sum += values[i]
		}
		if sum != 0 {
			return sum
		}
	}
	return values[0]
}

func battleEffectValuesFromValue(value interface{}) []float64 {
	if m, ok := value.(map[string]interface{}); ok {
		return floatSliceFromValue(m["value"])
	}
	return floatSliceFromValue(value)
}

func floatSliceFromValue(value interface{}) []float64 {
	rows, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(rows))
	for _, raw := range rows {
		out = append(out, numberFromValue(raw))
	}
	return out
}

func normalizeCapture(capture *Capture) error {
	if capture.MID <= 0 {
		return errors.New("battle report capture missing MID")
	}
	if capture.LID == 0 {
		capture.LID = capture.MID
	}
	capture.ID = captureID(capture.MID, capture.LID)
	if capture.Version == 0 {
		capture.Version = 1
	}
	if capture.Source == "" {
		capture.Source = "local"
	}
	if capture.CapturedAtUnixMillis == 0 {
		capture.CapturedAtUnixMillis = time.Now().UnixMilli()
	}
	return nil
}

func sameReport(a, b Capture) bool {
	if a.MID > 0 && b.MID > 0 {
		aLID := a.LID
		if aLID == 0 {
			aLID = a.MID
		}
		bLID := b.LID
		if bLID == 0 {
			bLID = b.MID
		}
		if a.MID == b.MID && aLID == bLID {
			return true
		}
		return a.MID == b.MID && (a.LID == 0 || b.LID == 0 || a.LID == a.MID || b.LID == b.MID)
	}
	return a.ID != "" && a.ID == b.ID
}

func mergeCapture(existing, incoming Capture) Capture {
	merged := existing
	if incoming.Version != 0 {
		merged.Version = incoming.Version
	}
	if incoming.ID != "" {
		merged.ID = incoming.ID
	}
	if incoming.ClientID != "" {
		merged.ClientID = incoming.ClientID
	}
	if incoming.Source != "" {
		merged.Source = incoming.Source
	}
	if incoming.CapturedAtUnixMillis != 0 {
		merged.CapturedAtUnixMillis = incoming.CapturedAtUnixMillis
	}
	if incoming.MID != 0 {
		merged.MID = incoming.MID
	}
	if incoming.LID != 0 {
		merged.LID = incoming.LID
	}
	if incoming.NoticeType != 0 {
		merged.NoticeType = incoming.NoticeType
	}
	if incoming.BattleKey != "" {
		merged.BattleKey = incoming.BattleKey
	}
	if len(incoming.SNERow) > 0 {
		merged.SNERow = incoming.SNERow
	}
	if incoming.SNE != nil {
		merged.SNE = incoming.SNE
	}
	if incoming.BLS != nil {
		merged.BLS = incoming.BLS
	}
	if incoming.BLM != nil {
		merged.BLM = incoming.BLM
	}
	if incoming.BLD != nil {
		merged.BLD = incoming.BLD
	}
	if incoming.Wire != nil {
		if merged.Wire == nil {
			merged.Wire = map[string]string{}
		}
		for key, value := range incoming.Wire {
			merged.Wire[key] = value
		}
	}
	_ = normalizeCapture(&merged)
	return merged
}

func battleReportUploadURL() string {
	if url := strings.TrimSpace(os.Getenv("BATTLE_REPORTS_UPLOAD_URL")); url != "" {
		return url
	}
	base := strings.TrimSpace(os.Getenv("CLOUD_BACKEND_URL"))
	if base == "" {
		base = defaultCloudBackendURL
	}
	return strings.TrimRight(base, "/") + "/reports/battle"
}

func battleReportFetchURL() string {
	if url := strings.TrimSpace(os.Getenv("BATTLE_REPORTS_FETCH_URL")); url != "" {
		return url
	}
	if url := strings.TrimSpace(os.Getenv("BATTLE_REPORTS_UPLOAD_URL")); url != "" {
		return url
	}
	base := strings.TrimSpace(os.Getenv("CLOUD_BACKEND_URL"))
	if base == "" {
		base = defaultCloudBackendURL
	}
	return strings.TrimRight(base, "/") + "/reports/battle"
}

func applyReportKeyHeader(req *http.Request) {
	if req == nil {
		return
	}
	if key := strings.TrimSpace(os.Getenv("REPORT_UPLOAD_KEY")); key != "" {
		req.Header.Set("X-Citadel-Report-Key", key)
	} else if key := strings.TrimSpace(os.Getenv("CITADEL_REPORT_UPLOAD_KEY")); key != "" {
		req.Header.Set("X-Citadel-Report-Key", key)
	}
}

func reportUploadClientID() (string, error) {
	if id := strings.TrimSpace(os.Getenv("CITADEL_CLIENT_ID")); id != "" {
		return id, nil
	}
	path := filepath.Join(Paths.DataDir(), clientIDFileName)
	if raw, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(raw)); id != "" {
			return id, nil
		}
	}
	id, err := newReportUploadClientID()
	if err != nil {
		id = fmt.Sprintf("desktop-%d", time.Now().UnixNano())
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0644); err != nil {
		return id, nil
	}
	return id, nil
}

func newReportUploadClientID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "desktop-" + hex.EncodeToString(buf), nil
}

func archivePath() string {
	return filepath.Join(Paths.DataDir(), archiveFileName)
}

func captureID(mid, lid int64) string {
	if lid == 0 {
		lid = mid
	}
	return fmt.Sprintf("%d-%d", mid, lid)
}

func cleanBattleLocation(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "+")
	if len(parts) > 1 {
		last := strings.TrimSpace(parts[len(parts)-1])
		if last != "" {
			return last
		}
	}
	return value
}

func timeFromUnixMs(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).Format(time.RFC3339)
}

func rowValue(row []interface{}, index int) interface{} {
	if index < 0 || index >= len(row) {
		return nil
	}
	return row[index]
}

func rowValueInt64(row []interface{}, index int) int64 {
	v, _ := int64FromValue(rowValue(row, index))
	return v
}

func stringFromValue(value interface{}) string {
	s, _ := value.(string)
	return strings.TrimSpace(s)
}

func int64FromValue(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func int64FromValueDefault(value interface{}) int64 {
	n, _ := int64FromValue(value)
	return n
}

func int64FromString(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func numberFromValue(value interface{}) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		return numberFromString(v)
	default:
		return 0
	}
}

func numberFromString(value string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return n
}

func int64Abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func roundFloat(value float64, places int) float64 {
	pow := math.Pow(10, float64(places))
	return math.Round(value*pow) / pow
}

func laneName(index int) string {
	switch index {
	case 0:
		return "left"
	case 1:
		return "middle"
	case 2:
		return "right"
	default:
		return fmt.Sprintf("lane-%d", index+1)
	}
}

func sumNestedPositive(value interface{}) int64 {
	var total int64
	walkNumbers(value, func(v float64) {
		if v > 0 {
			total += int64(v)
		}
	})
	return total
}

func sumNestedNegativeAbs(value interface{}) int64 {
	var total int64
	walkNumbers(value, func(v float64) {
		if v < 0 {
			total += int64(-v)
		}
	})
	return total
}

func walkNumbers(value interface{}, fn func(float64)) {
	switch v := value.(type) {
	case []interface{}:
		for _, child := range v {
			walkNumbers(child, fn)
		}
	case float64:
		fn(v)
	case int:
		fn(float64(v))
	case int64:
		fn(float64(v))
	}
}
