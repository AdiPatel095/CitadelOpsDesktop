package GameParser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	riftattack "CitadelDesktop/Server/Models/RiftAttack"
	"CitadelDesktop/Server/ResponseRegistry"
)

// NotifyRiftCRALaunchChanged is wired by FrontendWebsocket after a Rift **cra** template is saved or availability changes.
var NotifyRiftCRALaunchChanged func()

var (
	riftCRABusyMu      sync.Mutex
	lastRiftCRABusyKey string
)

// craWireBody extracts the JSON body from an EmpireEx **cra** outbound frame.
func craWireBody(payload string) (string, bool) {
	parts := strings.Split(payload, "%")
	if len(parts) < 6 {
		return "", false
	}
	if parts[2] != ResponseRegistry.EmpireExToken && !strings.HasPrefix(parts[2], "EmpireEx_") {
		return "", false
	}
	if strings.ToLower(parts[3]) != "cra" {
		return "", false
	}
	body := parts[5]
	if body == "" {
		return "", false
	}
	return body, true
}

// craLaunchTargetsRift reports whether TX/TY/KID match the known world Rift tile.
func craLaunchTargetsRift(tx, ty, kid int) bool {
	rift, riftKid, ok := Models.GetMapState().FindRift()
	if !ok {
		return false
	}
	return tx == rift.X && ty == rift.Y && kid == riftKid
}

func craBodyHasTroopLayout(body GameCommands.CRALaunchBody) bool {
	for _, wave := range body.A {
		for _, flank := range []GameCommands.CRAFlank{wave.L, wave.R, wave.M} {
			for _, pair := range flank.U {
				if pair[0] > 0 && pair[1] > 0 {
					return true
				}
			}
			for _, pair := range flank.T {
				if pair[0] > 0 && pair[1] > 0 {
					return true
				}
			}
		}
	}
	for _, id := range body.AST {
		if id > 0 {
			return true
		}
	}
	for _, pair := range body.RW {
		if pair[0] > 0 && pair[1] > 0 {
			return true
		}
	}
	return false
}

func launchBodyFromSaved(launch riftattack.SavedLaunch) (GameCommands.CRALaunchBody, error) {
	bodyBytes, err := json.Marshal(launch.Body)
	if err != nil {
		return GameCommands.CRALaunchBody{}, err
	}
	var body GameCommands.CRALaunchBody
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return GameCommands.CRALaunchBody{}, err
	}
	return body, nil
}

// RoundUpToUnixMinute snaps a unix timestamp up to the next whole-minute boundary (seconds=0).
func RoundUpToUnixMinute(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	return ((ts + 59) / 60) * 60
}

func minArriveAtUnix(oneWayTT int) int64 {
	if oneWayTT <= 0 {
		return 0
	}
	return RoundUpToUnixMinute(time.Now().Unix() + int64(oneWayTT))
}

// NormalizeScheduledArriveAt snaps a scheduled arrival to min + whole-minute offsets (unix second 0).
func NormalizeScheduledArriveAt(arriveAtUnix int64, oneWayTT int) int64 {
	min := minArriveAtUnix(oneWayTT)
	if arriveAtUnix <= min {
		return min
	}
	snapped := RoundUpToUnixMinute(arriveAtUnix)
	offsetMin := (snapped - min) / 60
	if offsetMin <= 0 {
		return min
	}
	return min + offsetMin*60
}

func launchWireItem(launch riftattack.SavedLaunch, gs *Models.GameState, scheduled map[string]int64) map[string]interface{} {
	body, err := launchBodyFromSaved(launch)
	if err != nil {
		return map[string]interface{}{
			"id":          launch.ID,
			"savedAtUnix": launch.SavedAtUnix,
		}
	}
	commanderStatus, commanderBusy := riftCommanderStatus(gs, body.LID)
	useFeather := body.HBW == -1 && body.PTT == 1
	item := map[string]interface{}{
		"id":               launch.ID,
		"savedAtUnix":      launch.SavedAtUnix,
		"displayName":      launch.DisplayName,
		"commanderID":      body.LID,
		"sourceX":          body.SX,
		"sourceY":          body.SY,
		"targetX":          body.TX,
		"targetY":          body.TY,
		"kingdomID":        body.KID,
		"attackValid":      body.AV,
		"waveCount":        len(body.A),
		"useTravelFeather": useFeather,
		"commanderStatus":  commanderStatus,
		"commanderBusy":    commanderBusy,
		"canResend":        !commanderBusy,
	}
	if launch.OneWayTTSeconds > 0 {
		item["oneWayTTSeconds"] = launch.OneWayTTSeconds
		item["minArriveAtUnix"] = minArriveAtUnix(launch.OneWayTTSeconds)
		if launch.LastSuccessAtUnix > 0 {
			item["lastSuccessAtUnix"] = launch.LastSuccessAtUnix
		}
	}
	if scheduled != nil {
		if arriveAt, ok := scheduled[launch.ID]; ok && arriveAt > 0 {
			item["scheduledArriveAtUnix"] = arriveAt
		}
	}
	return item
}

func riftCommanderStatus(gs *Models.GameState, commanderID int) (string, bool) {
	if gs == nil {
		return "unknown", true
	}
	status, known := gs.Movement.CommanderStatus(commanderID, time.Now().Unix())
	if !known {
		return "unknown", true
	}
	return string(status.Status), status.Busy
}

// RecordRiftCRASuccess stores feather **TT** from a successful inbound **cra** ack for the matching captured launch.
func RecordRiftCRASuccess(payload string) {
	commanderID, tt, ok := CommanderAndTTFromGAMLikeJSON(payload)
	if !ok {
		return
	}
	if !riftattack.UpdateLaunchTravelTime(commanderID, tt, time.Now().Unix()) {
		return
	}
	Logging.RiftLogf("travel_tt", "feather TT=%ds commander LID=%d", tt, commanderID)
	if NotifyRiftCRALaunchChanged != nil {
		NotifyRiftCRALaunchChanged()
	}
}

// TryCaptureOutboundRiftCRA saves outbound **cra** frames that target the Rift (browser UI only).
// Citadel-queued replays/schedules are skipped — they already exist as saved templates.
func TryCaptureOutboundRiftCRA(payload string) {
	if ResponseRegistry.TryConsumeAppOutboundCRACaptureSkip() {
		return
	}
	bodyJSON, ok := craWireBody(payload)
	if !ok {
		return
	}
	var body GameCommands.CRALaunchBody
	if err := json.Unmarshal([]byte(bodyJSON), &body); err != nil {
		return
	}
	if !craLaunchTargetsRift(body.TX, body.TY, body.KID) {
		return
	}
	if !craBodyHasTroopLayout(body) {
		return
	}

	var bodyMap riftattack.CRALaunchBodyJSON
	if err := json.Unmarshal([]byte(bodyJSON), &bodyMap); err != nil {
		return
	}

	gs := Models.GetGameState()
	savedAt := time.Now().Unix()
	launch := riftattack.SavedLaunch{
		SavedAtUnix: savedAt,
		WirePayload: payload,
		Body:        bodyMap,
	}
	if !riftattack.AppendLaunch(gs.PlayerID, launch) {
		Logging.RiftLogf("capture_skip", "duplicate wave layout already in Rift list (LID=%d waves=%d)",
			body.LID, len(body.A))
		return
	}
	Logging.RiftLogf("capture", "saved CRA → Rift (%d,%d) K%d LID=%d AV=%d waves=%d",
		body.TX, body.TY, body.KID, body.LID, body.AV, len(body.A))
	resetRiftCRABusySnapshot()
	if NotifyRiftCRALaunchChanged != nil {
		NotifyRiftCRALaunchChanged()
	}
}

// MaybeNotifyRiftCRALaunchBusyChanged pushes an update when commander status changes for saved launches.
func MaybeNotifyRiftCRALaunchBusyChanged() {
	if NotifyRiftCRALaunchChanged == nil {
		return
	}
	f := riftattack.Snapshot()
	if len(f.Launches) == 0 {
		return
	}
	gs := Models.GetGameState()
	parts := make([]string, 0, len(f.Launches))
	for _, launch := range f.Launches {
		body, err := launchBodyFromSaved(launch)
		if err != nil {
			continue
		}
		status, busy := riftCommanderStatus(gs, body.LID)
		parts = append(parts, fmt.Sprintf("%d:%s:%t", body.LID, status, busy))
	}
	key := strings.Join(parts, ",")

	riftCRABusyMu.Lock()
	if key == lastRiftCRABusyKey {
		riftCRABusyMu.Unlock()
		return
	}
	lastRiftCRABusyKey = key
	riftCRABusyMu.Unlock()

	// Never call Notify while holding riftCRABusyMu — payload build may load schedules/launches.
	if NotifyRiftCRALaunchChanged != nil {
		NotifyRiftCRALaunchChanged()
	}
}

func resetRiftCRABusySnapshot() {
	riftCRABusyMu.Lock()
	lastRiftCRABusyKey = ""
	riftCRABusyMu.Unlock()
}

// ScheduledRiftCRAArrivals is wired by FeatureView to expose pending scheduled arrival times per launch id.
var ScheduledRiftCRAArrivals func() map[string]int64

// RiftCRALaunchWirePayload builds the websocket payload listing captured launches and commander availability.
func RiftCRALaunchWirePayload() map[string]interface{} {
	return RiftCRALaunchWirePayloadFromSnapshot(riftattack.Snapshot())
}

func RiftCRALaunchWirePayloadFromSnapshot(f riftattack.File) map[string]interface{} {
	gs := Models.GetGameState()
	var scheduled map[string]int64
	if ScheduledRiftCRAArrivals != nil {
		scheduled = ScheduledRiftCRAArrivals()
	}
	launches := make([]map[string]interface{}, 0, len(f.Launches))
	for _, launch := range f.Launches {
		launches = append(launches, launchWireItem(launch, gs, scheduled))
	}
	busyIDs := BusyCommanderWireIDs(gs)
	return map[string]interface{}{
		"launches":         launches,
		"busyCommanderIDs": busyIDs,
		"launchCount":      len(launches),
	}
}

// ReplaySavedRiftCRA re-sends one saved template by id, optionally overriding commander and source coords.
func ReplaySavedRiftCRA(launchID string, commanderID, sourceX, sourceY int) error {
	if !ResponseRegistry.LoginStatus {
		Logging.RiftLog("resend_blocked", "game websocket not logged in")
		return errRiftGameNotConnected
	}
	launch, ok := riftattack.FindLaunch(launchID)
	if !ok {
		Logging.RiftLogf("resend_failed", "launch %q not found", launchID)
		return errNoRiftCRATemplate
	}
	body, err := launchBodyFromSaved(launch)
	if err != nil {
		Logging.RiftLogf("resend_failed", "launch %q body parse: %v", launchID, err)
		return err
	}

	effectiveCommander := body.LID
	if commanderID >= 0 {
		effectiveCommander = commanderID
	}
	gs := Models.GetGameState()
	commanderStatus, commanderBusy := riftCommanderStatus(gs, effectiveCommander)
	if commanderBusy {
		Logging.RiftLogf("resend_blocked", "commander LID=%d unavailable status=%s", effectiveCommander, commanderStatus)
		return errRiftCRACommanderUnavailable
	}

	if commanderID >= 0 {
		body.LID = commanderID
	}
	if sourceX >= 0 && sourceY >= 0 {
		body.SX = sourceX
		body.SY = sourceY
	}
	if rift, riftKid, ok := Models.GetMapState().FindRift(); ok {
		body.TX = rift.X
		body.TY = rift.Y
		body.KID = riftKid
	}
	body.AV = 1

	payload, err := GameCommands.CRAPayloadFromBody(body)
	if err != nil {
		Logging.RiftLogf("resend_failed", "launch %q payload: %v", launchID, err)
		return err
	}
	receipt := GameCommands.DispatchPayload(context.Background(), "cra", "rift_saved_attack", payload, Automation.CommandOptions{
		Owner:    Automation.OwnerManual,
		Priority: Automation.PriorityManual,
	})
	if !receipt.Accepted {
		return fmt.Errorf("command harness rejected saved rift attack: %s", receipt.Message)
	}
	Logging.AppendRiftSendPayload(payload)
	Logging.RiftLogf("resend", "queued submission=%d cra launch=%s LID=%d (%d,%d)→(%d,%d) K%d waves=%d",
		receipt.SubmissionID, launchID, body.LID, body.SX, body.SY, body.TX, body.TY, body.KID, len(body.A))
	return nil
}

type riftCRAError string

func (e riftCRAError) Error() string { return string(e) }

const (
	errNoRiftCRATemplate           riftCRAError = "no saved Rift CRA launch template"
	errRiftCRACommanderUnavailable riftCRAError = "commander is unavailable; wait for an up-to-date free status"
	errRiftGameNotConnected        riftCRAError = "game not connected — log in before resending"
)
