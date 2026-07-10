package GameParser

import (
	"sort"
	"strconv"
	"sync"
	"time"

	"CitadelDesktop/Server/Models"
	gamestate "CitadelDesktop/Server/Models/GameState"
	riftattack "CitadelDesktop/Server/Models/RiftAttack"
	"CitadelDesktop/Server/ResponseRegistry"
)

const (
	gamActivePollIntervalSec = 10
	gamIdlePollIntervalSec   = 30
)

// lastGamAutoPollUnix is read/written only by the single poll-ticker goroutine.
var lastGamAutoPollUnix int64

var movementTickerOnce sync.Once

func init() {
	gamestate.EvaluateCommanderWireBusy = func(gs *gamestate.GameState, commanderWireID int) bool {
		return CommanderMarching(gs, commanderWireID, 0, 0, 0, 0, false)
	}
	startMovementPollTicker()
}

func coordsMatch(ax, ay, bx, by int) bool {
	return ax == bx && ay == by
}

// sourceIsRift reports a leg whose source tile is the world Rift.
func sourceIsRift(m Models.GAMMovement) bool {
	rift, _, ok := Models.GetMapState().FindRift()
	if !ok {
		return false
	}
	return coordsMatch(m.SourceX, m.SourceY, rift.X, rift.Y)
}

func movementLegsForCommander(gs *Models.GameState, commanderWireID int) []Models.GAMMovement {
	if gs == nil || commanderWireID < 0 {
		return nil
	}
	movements, _, _, _ := gs.Movement.Snapshot()
	legs := make([]Models.GAMMovement, 0, 1)
	for _, movement := range movements {
		if movement.CommanderID == commanderWireID {
			legs = append(legs, movement)
		}
	}
	return legs
}

func intFromBody(body map[string]interface{}, key string) int {
	if body == nil {
		return 0
	}
	v, ok := body[key].(float64)
	if !ok {
		return 0
	}
	return int(v)
}

// savedLaunchCommanderForRiftReturn is a fallback for an orphan Rift return without UM.L.ID.
func savedLaunchCommanderForRiftReturn(m Models.GAMMovement) (int, bool) {
	for _, launch := range riftattack.Load().Launches {
		lid := riftattack.CommanderIDFromLaunch(launch)
		if lid < 0 {
			continue
		}
		sx, sy := intFromBody(launch.Body, "SX"), intFromBody(launch.Body, "SY")
		if coordsMatch(m.TargetX, m.TargetY, sx, sy) {
			return lid, true
		}
	}
	return 0, false
}

// CommanderMarching is the compatibility busy gate used by existing attack features. Unknown or stale
// state is unavailable by design; only a recent complete GAM snapshot may prove a commander free.
func CommanderMarching(gs *Models.GameState, wireID, homeX, homeY, targetX, targetY int, useLaunchCoords bool) bool {
	if gs == nil || wireID < 0 {
		return true
	}
	return gs.Movement.IsCommanderUnavailable(wireID, time.Now().Unix())
}

func CommanderMarchingForLaunch(gs *Models.GameState, wireID, homeX, homeY, targetX, targetY int) bool {
	return CommanderMarching(gs, wireID, homeX, homeY, targetX, targetY, true)
}

func startMovementPollTicker() {
	movementTickerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				maybeAutoPollGAM()
			}
		}()
	})
}

// maybeAutoPollGAM keeps free state authoritative too: active or unsynchronized state polls every ten
// seconds, while an idle complete snapshot polls every thirty seconds.
func maybeAutoPollGAM() {
	if !ResponseRegistry.IsGameWebSocketReady() {
		return
	}
	gs := Models.GetGameState()
	_, _, snapshotReady, lastSnapshotUnix := gs.Movement.Snapshot()
	interval := int64(gamIdlePollIntervalSec)
	if !snapshotReady || gs.Movement.HasActiveCommanderMovement() {
		interval = gamActivePollIntervalSec
	}
	now := time.Now().Unix()
	if snapshotReady && lastSnapshotUnix > 0 && now-lastSnapshotUnix < interval {
		return
	}
	if (!snapshotReady || lastSnapshotUnix <= 0) && now-lastGamAutoPollUnix < interval {
		return
	}
	if RequestGAMSnapshot() {
		lastGamAutoPollUnix = now
	}
}

func empireExResponseCode(messageParts []string) (int, bool) {
	if len(messageParts) <= 4 {
		return 0, false
	}
	code, err := strconv.Atoi(messageParts[4])
	if err != nil {
		return 0, false
	}
	return code, true
}

func ParseCRAResponse(messageParts []string, payload string) {
	ParseGAMMessage(payload)
	code, ok := empireExResponseCode(messageParts)
	if !ok || code != 0 {
		return
	}
	RecordRiftCRASuccess(payload)
}

func IsCommanderWireIDBusy(gs *Models.GameState, commanderWireID int) bool {
	return CommanderMarching(gs, commanderWireID, 0, 0, 0, 0, false)
}

func BusyCommanderWireIDs(gs *Models.GameState) []int {
	if gs == nil {
		return nil
	}
	status := gs.Movement.StatusSnapshot(time.Now().Unix())
	out := make([]int, 0, len(status.CommanderStatuses))
	for _, commander := range status.CommanderStatuses {
		if commander.Busy {
			out = append(out, commander.CommanderID)
		}
	}
	sort.Ints(out)
	return out
}
