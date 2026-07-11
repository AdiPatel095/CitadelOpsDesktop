package spyreport

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	movement "CitadelDesktop/Server/Models/Movement"
	"CitadelDesktop/Server/Paths"
)

const (
	defaultCloudBackendURL     = "https://citadelops.app/api"
	cloudRequestTimeout        = 20 * time.Second
	allianceSessionRenewWindow = 5 * time.Minute
)

type allianceSessionState struct {
	Token      string
	PlayerID   int64
	AllianceID int64
	ExpiresAt  time.Time
}

type spyCloudEnvelope struct {
	ReportID            string `json:"reportID"`
	MID                 int64  `json:"mid"`
	ScannedAtUnixMillis int64  `json:"scannedAtUnixMillis"`
	AllianceID          int64  `json:"allianceID"`
	CastleID            int64  `json:"castleID"`
	TargetPlayerID      int64  `json:"targetPlayerID"`
	SourcePlayerID      int64  `json:"sourcePlayerID"`
	KingdomID           int    `json:"kingdomID"`
	TargetX             int    `json:"targetX"`
	TargetY             int    `json:"targetY"`
	Status              string `json:"status"`
	Payload             string `json:"payload"`
}

type movementCloudEnvelope struct {
	MovementID           int    `json:"movementID"`
	ObservedAtUnixMillis int64  `json:"observedAtUnixMillis"`
	AllianceID           int    `json:"allianceID"`
	PlayerID             int    `json:"playerID"`
	CastleID             int64  `json:"castleID,omitempty"`
	KingdomID            int    `json:"kingdomID"`
	TargetX              int    `json:"targetX"`
	TargetY              int    `json:"targetY"`
	MovementType         int    `json:"movementType"`
	Payload              string `json:"payload"`
}

var (
	allianceSessionMu  sync.Mutex
	allianceSession    allianceSessionState
	uploadedMovementMu sync.Mutex
	uploadedMovements  = map[int]time.Time{}
)

func RefreshAllianceSession(playerID, allianceID int, memberPlayerIDs []int) error {
	_, err := ensureAllianceSession(int64(playerID), int64(allianceID), memberPlayerIDs)
	return err
}

func UploadCaptureToCloud(capture Capture, memberPlayerIDs []int) error {
	parsed := ParseCapture(capture)
	if capture.BSD == nil || parsed.Castle.ID <= 0 || parsed.Source.ID <= 0 {
		return fmt.Errorf("spy report is missing castle or source identity")
	}
	allianceID := int64From(capture.BSDOwnerAllianceID())
	if allianceID <= 0 {
		return fmt.Errorf("spy report is missing source alliance")
	}
	session, err := ensureAllianceSession(parsed.Source.ID, allianceID, memberPlayerIDs)
	if err != nil {
		return err
	}
	parsed.RawCapture = &capture
	payload, err := json.Marshal(parsed)
	if err != nil {
		return err
	}
	envelope := spyCloudEnvelope{
		ReportID: parsed.ID, MID: parsed.MID, ScannedAtUnixMillis: parsed.CapturedAtUnixMillis,
		AllianceID: allianceID, CastleID: parsed.Castle.ID, TargetPlayerID: parsed.Target.ID,
		SourcePlayerID: parsed.Source.ID, KingdomID: parsed.Castle.KingdomID,
		TargetX: parsed.Castle.X, TargetY: parsed.Castle.Y, Status: parsed.Status, Payload: string(payload),
	}
	return sendAllianceJSON(http.MethodPost, "/reports/spy", envelope, session.Token, nil)
}

func FetchAllianceSpyReports() ([]ParsedReport, error) {
	allianceSessionMu.Lock()
	session := allianceSession
	allianceSessionMu.Unlock()
	if session.Token == "" || time.Now().After(session.ExpiresAt) {
		return nil, fmt.Errorf("alliance report session unavailable")
	}
	var response struct {
		Reports []json.RawMessage `json:"reports"`
	}
	if err := sendAllianceJSON(http.MethodGet, "/reports/spy", nil, session.Token, &response); err != nil {
		return nil, err
	}
	reports := make([]ParsedReport, 0, len(response.Reports))
	for _, raw := range response.Reports {
		var report ParsedReport
		if json.Unmarshal(raw, &report) == nil && report.ID != "" {
			report.RawCapture = nil
			reports = append(reports, report)
		}
	}
	return reports, nil
}

func UploadMatchingMovements(movements []movement.GAMMovement, playerID, allianceID int, memberPlayerIDs []int) {
	if playerID <= 0 || allianceID <= 0 {
		return
	}
	hasOutboundAttack := false
	for _, item := range movements {
		if item.MID > 0 && item.D == 0 && len(item.TroopArray) > 0 {
			hasOutboundAttack = true
			break
		}
	}
	if !hasOutboundAttack {
		return
	}
	session, sessionErr := ensureAllianceSession(int64(playerID), int64(allianceID), memberPlayerIDs)
	if sessionErr != nil {
		return
	}
	captures, err := ReadCaptures()
	if err != nil {
		return
	}
	reports := make([]ParsedReport, 0, len(captures))
	for index := range captures {
		reports = append(reports, ParseCapture(captures[index]))
	}
	if sharedReports, sharedErr := FetchAllianceSpyReports(); sharedErr == nil {
		reports = mergeParsedReports(reports, sharedReports)
	}
	now := time.Now()
	for _, item := range movements {
		if item.MID <= 0 || item.D != 0 || len(item.TroopArray) == 0 {
			continue
		}
		uploadedMovementMu.Lock()
		_, alreadyUploaded := uploadedMovements[item.MID]
		uploadedMovementMu.Unlock()
		if alreadyUploaded {
			continue
		}
		var matched *ParsedReport
		for index := range reports {
			report := reports[index]
			if report.Castle.KingdomID == item.KID && report.Castle.X == item.TargetX && report.Castle.Y == item.TargetY && now.Sub(time.UnixMilli(report.CapturedAtUnixMillis)) <= 6*time.Hour {
				matched = &report
				break
			}
		}
		if matched == nil {
			continue
		}
		raw, marshalErr := json.Marshal(item)
		if marshalErr != nil {
			continue
		}
		envelope := movementCloudEnvelope{MovementID: item.MID, ObservedAtUnixMillis: now.UnixMilli(), AllianceID: allianceID, PlayerID: playerID, CastleID: matched.Castle.ID, KingdomID: item.KID, TargetX: item.TargetX, TargetY: item.TargetY, MovementType: item.MovementType, Payload: string(raw)}
		if sendAllianceJSON(http.MethodPost, "/reports/spy/movement", envelope, session.Token, nil) == nil {
			uploadedMovementMu.Lock()
			uploadedMovements[item.MID] = now
			for mid, uploadedAt := range uploadedMovements {
				if now.Sub(uploadedAt) > 24*time.Hour {
					delete(uploadedMovements, mid)
				}
			}
			uploadedMovementMu.Unlock()
		}
	}
}

func (capture Capture) BSDOwnerAllianceID() interface{} {
	owner, _ := capture.BSD["SO"].(map[string]interface{})
	return owner["AID"]
}

func ensureAllianceSession(playerID, allianceID int64, memberPlayerIDs []int) (allianceSessionState, error) {
	if playerID <= 0 || allianceID <= 0 {
		return allianceSessionState{}, fmt.Errorf("player and alliance identity are required")
	}
	allianceSessionMu.Lock()
	defer allianceSessionMu.Unlock()
	if allianceSession.PlayerID == playerID && allianceSession.AllianceID == allianceID && time.Until(allianceSession.ExpiresAt) > allianceSessionRenewWindow {
		return allianceSession, nil
	}
	clientID, err := cloudClientID()
	if err != nil {
		return allianceSessionState{}, err
	}
	body := map[string]interface{}{"clientID": clientID, "playerID": playerID, "allianceID": allianceID, "memberPlayerIDs": memberPlayerIDs}
	var response struct {
		Token      string    `json:"token"`
		PlayerID   int64     `json:"playerID"`
		AllianceID int64     `json:"allianceID"`
		ExpiresAt  time.Time `json:"expiresAt"`
	}
	if err := sendAllianceJSON(http.MethodPost, "/reports/alliance/session", body, "", &response); err != nil {
		return allianceSessionState{}, err
	}
	allianceSession = allianceSessionState{Token: response.Token, PlayerID: response.PlayerID, AllianceID: response.AllianceID, ExpiresAt: response.ExpiresAt}
	return allianceSession, nil
}

func sendAllianceJSON(method, path string, body interface{}, token string, output interface{}) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, cloudBaseURL()+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("X-Citadel-Alliance-Token", token)
	}
	applyCloudReportKey(req)
	resp, err := (&http.Client{Timeout: cloudRequestTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("spy cloud request failed: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	return nil
}

func cloudBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("SPY_REPORTS_BACKEND_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(os.Getenv("CLOUD_BACKEND_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return defaultCloudBackendURL
}
func applyCloudReportKey(req *http.Request) {
	if key := strings.TrimSpace(os.Getenv("REPORT_UPLOAD_KEY")); key != "" {
		req.Header.Set("X-Citadel-Report-Key", key)
	} else if key := strings.TrimSpace(os.Getenv("CITADEL_REPORT_UPLOAD_KEY")); key != "" {
		req.Header.Set("X-Citadel-Report-Key", key)
	}
}
func cloudClientID() (string, error) {
	if id := strings.TrimSpace(os.Getenv("CITADEL_CLIENT_ID")); id != "" {
		return id, nil
	}
	path := filepath.Join(Paths.DataDir(), "SpyReportClientID.txt")
	if raw, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(raw)) != "" {
		return strings.TrimSpace(string(raw)), nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := fmt.Sprintf("desktop-%x", raw)
	if err := os.WriteFile(path, []byte(id+"\n"), 0644); err != nil {
		return "", err
	}
	return id, nil
}
