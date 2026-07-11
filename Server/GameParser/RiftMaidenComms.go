package GameParser

import (
	"context"
	"fmt"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFocus"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	equip "CitadelDesktop/Server/Models/Equipment"
	"CitadelDesktop/Server/ResponseRegistry"
)

// Maiden support stat id on relic artifacts (shield-maiden bonus).
const (
	MaidenSuppStatID          = 121
	MaidenSuppArtifactMin     = 300
	MaidenSuppArtifactMax     = 1050
	maidenSuppRelicRarity     = 5
	maidenSuppHeroRelicRarity = 15
)

// MaidenCommsWaveResult summarizes a maiden-comms wave dispatch.
type MaidenCommsWaveResult struct {
	Sent              int   `json:"sent"`
	SkippedBusy       int   `json:"skippedBusy"`
	SkippedNoArtifact int   `json:"skippedNoArtifact"`
	CommanderIDs      []int `json:"commanderIDs,omitempty"`
}

// maidenSuppStatValue reads the wire value for stat 121 (uses index 1 when present).
func maidenSuppStatValue(stat Models.Stat) float64 {
	if stat.ID != float64(MaidenSuppStatID) {
		return 0
	}
	if len(stat.Value) > 1 {
		return stat.Value[1]
	}
	if len(stat.Value) > 0 {
		return stat.Value[0]
	}
	return 0
}

func commanderHasRelicEquipped(comm Models.CommActualModel) bool {
	for _, eq := range comm.Equipment {
		rarity := int(eq.EquipRarity)
		if rarity == maidenSuppRelicRarity || rarity == maidenSuppHeroRelicRarity {
			return true
		}
	}
	return false
}

func maidenSuppInArtifactRange(v float64) bool {
	return v >= MaidenSuppArtifactMin && v <= MaidenSuppArtifactMax
}

// CommanderHasShieldMaidenArtifact reports whether a commander wears a relic with
// maiden-support stat in the shield-maiden artifact range (300–1050).
func CommanderHasShieldMaidenArtifact(comm Models.CommActualModel) bool {
	for _, eq := range comm.Equipment {
		rarity := int(eq.EquipRarity)
		if rarity != maidenSuppRelicRarity && rarity != maidenSuppHeroRelicRarity {
			continue
		}
		for _, stat := range eq.EquipStats {
			if maidenSuppInArtifactRange(maidenSuppStatValue(stat)) {
				return true
			}
		}
	}
	return false
}

// commanderQualifiesForMaidenComms checks relic stat rows and aggregated CommStat maidenSupp
// (covers gem-only maiden bonus on an otherwise blank relic row).
func commanderQualifiesForMaidenComms(comm Models.CommActualModel, maidenSupp float64) bool {
	if CommanderHasShieldMaidenArtifact(comm) {
		return true
	}
	return commanderHasRelicEquipped(comm) && maidenSuppInArtifactRange(maidenSupp)
}

// EligibleMaidenComms returns wire LIDs for commanders with shield-maiden artifacts that are not busy.
func EligibleMaidenComms(gs *Models.GameState, sourceX, sourceY, targetX, targetY int) []int {
	if gs == nil {
		return nil
	}
	out := make([]int, 0, len(gs.Equipment.CommActualArray))
	for i, comm := range gs.Equipment.CommActualArray {
		wireID := int(comm.ID)
		if wireID <= 0 {
			continue
		}
		maidenSupp := 0.0
		if i < len(equip.CommStatArray) {
			maidenSupp = equip.CommStatArray[i].MaidenSupp
		}
		if !commanderQualifiesForMaidenComms(comm, maidenSupp) {
			continue
		}
		if CommanderMarchingForLaunch(gs, wireID, sourceX, sourceY, targetX, targetY) {
			continue
		}
		out = append(out, wireID)
	}
	return out
}

// SendMaidenCommsWave queues dummy 1-wave rift attacks for eligible shield-maiden commanders.
// unitWodID selects the probe troop; when <= 0 the default from cra_launch.json empty shell is used.
// The dedicated attack websocket lane applies the configured random delay between launches.
func SendMaidenCommsWave(sourceX, sourceY, unitWodID int) (MaidenCommsWaveResult, error) {
	if !ResponseRegistry.LoginStatus {
		Logging.RiftLog("maiden_comms_blocked", "game websocket not logged in")
		return MaidenCommsWaveResult{}, errRiftGameNotConnected
	}

	gs := Models.GetGameState()
	rift, riftKid, ok := Models.GetMapState().FindRift()
	if !ok {
		Logging.RiftLog("maiden_comms_failed", "rift tile not known — refresh map coords first")
		return MaidenCommsWaveResult{}, fmt.Errorf("rift location unknown — refresh Rift coords first")
	}

	sx, sy, ok := resolveMaidenCommsSource(sourceX, sourceY)
	if !ok {
		Logging.RiftLog("maiden_comms_failed", "no source castle coords")
		return MaidenCommsWaveResult{}, fmt.Errorf("no source castle coordinates — focus a castle on the map")
	}

	effectiveUnit := unitWodID
	if effectiveUnit <= 0 {
		effectiveUnit = GameCommands.CRADummyMaidenProbeUnitWodID
	}
	if !mainCastleHasTroop(gs, effectiveUnit) {
		refreshMainCastleTroopsForMaidenComms(gs)
	}
	if !mainCastleHasTroop(gs, effectiveUnit) {
		Logging.RiftLogf("maiden_comms_failed", "unit %d not in main castle troopsI after main focus", effectiveUnit)
		return MaidenCommsWaveResult{}, fmt.Errorf("unit %d is not in main castle stock — focus main castle in-game, then retry", effectiveUnit)
	}

	result := MaidenCommsWaveResult{}
	wireIDs := make([]int, 0)

	for i, comm := range gs.Equipment.CommActualArray {
		wireID := int(comm.ID)
		if wireID <= 0 {
			continue
		}
		maidenSupp := 0.0
		if i < len(equip.CommStatArray) {
			maidenSupp = equip.CommStatArray[i].MaidenSupp
		}
		if !commanderQualifiesForMaidenComms(comm, maidenSupp) {
			result.SkippedNoArtifact++
			continue
		}
		if CommanderMarchingForLaunch(gs, wireID, sx, sy, rift.X, rift.Y) {
			result.SkippedBusy++
			continue
		}
		wireIDs = append(wireIDs, wireID)
	}

	if len(wireIDs) == 0 {
		Logging.RiftLogf("maiden_comms_skip", "no eligible comms (busy=%d noArtifact=%d)",
			result.SkippedBusy, result.SkippedNoArtifact)
		return result, nil
	}

	wave := GameCommands.CRADummyMaidenProbeWave(effectiveUnit, GameCommands.CRADummyMaidenProbeUnitCount)
	result.Sent = len(wireIDs)
	result.CommanderIDs = append(result.CommanderIDs, wireIDs...)

	go dispatchMaidenCommsQueued(wireIDs, sx, sy, rift.X, rift.Y, riftKid, effectiveUnit, wave)

	return result, nil
}

func dispatchMaidenCommsQueued(
	wireIDs []int,
	sx, sy, targetX, targetY, kingdomID, unitWodID int,
	wave GameCommands.CRAWave,
) {
	for _, wireID := range wireIDs {
		params := GameCommands.CRALaunchParams{
			SourceX:          sx,
			SourceY:          sy,
			TargetX:          targetX,
			TargetY:          targetY,
			KingdomID:        kingdomID,
			CommanderID:      wireID,
			UseTravelFeather: true,
			AttackValid:      1,
			Waves:            []GameCommands.CRAWave{wave},
		}
		payload, err := GameCommands.CRAPayload(params)
		if err != nil {
			Logging.RiftLogf("maiden_comms_failed", "LID=%d payload: %v", wireID, err)
			continue
		}
		commanderID := wireID
		receipt := GameCommands.DispatchPayload(context.Background(), "cra", "rift_maiden_probe", payload, Automation.CommandOptions{
			Owner:    Automation.OwnerManual,
			Priority: Automation.PriorityManual,
			Guard: func() bool {
				return ResponseRegistry.LoginStatus && !CommanderMarchingForLaunch(
					Models.GetGameState(), commanderID, sx, sy, targetX, targetY,
				)
			},
		})
		if !receipt.Accepted {
			Logging.RiftLogf("maiden_comms_failed", "LID=%d queue: %s", wireID, receipt.Message)
			continue
		}
		Logging.AppendRiftSendPayload(payload)
		Logging.RiftLogf("maiden_comms", "queued submission=%d dummy wave LID=%d unit=%d (%d,%d)→(%d,%d) K%d",
			receipt.SubmissionID, wireID, unitWodID, sx, sy, targetX, targetY, kingdomID)
	}
	MaybeNotifyRiftCRALaunchBusyChanged()
}

func mainCastleHasTroop(gs *Models.GameState, unitWodID int) bool {
	if gs == nil || unitWodID <= 0 {
		return false
	}
	count, ok := gs.Castle.MainCastle.Troops.TroopsI[unitWodID]
	return ok && count > 0
}

// refreshMainCastleTroopsForMaidenComms focuses main via JAA so TroopsI reflects in-castle stock.
func refreshMainCastleTroopsForMaidenComms(gs *Models.GameState) {
	if gs == nil {
		return
	}
	main := &gs.Castle.MainCastle
	castleID := int(main.Aid)
	if castleID <= 0 {
		return
	}
	kid, x, y := main.MapKingdomID, main.MapX, main.MapY
	if x == 0 && y == 0 {
		t := main.Troops
		if t.X != 0 || t.Y != 0 {
			kid, x, y = t.KingdomID, t.X, t.Y
		}
	}
	if x == 0 && y == 0 {
		if px, py, ok := gs.ResolveCastleMapCoords(castleID, kid); ok {
			x, y = px, py
		}
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleID && (loc.X != 0 || loc.Y != 0) {
			if kid == 0 {
				kid = loc.KingdomID
			}
			if x == 0 && y == 0 {
				x, y = loc.X, loc.Y
			}
			break
		}
	}
	if x == 0 && y == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lease, ok := GameFocus.Acquire(ctx, GameFocus.Request{
		Owner:           GameFocus.OwnerManual,
		Priority:        GameFocus.PriorityManual,
		Reason:          "rift maiden comms troop refresh",
		MaxHold:         10 * time.Second,
		Claims:          []GameFocus.Claim{GameFocus.CastleClaim(castleID, "troop-refresh")},
		PreemptLower:    true,
		SupersedeManual: true,
	})
	if !ok {
		return
	}
	defer lease.Release()
	FocusPlayerCastleTroopsWithLease(lease, kid, castleID, x, y)
}

func resolveMaidenCommsSource(sourceX, sourceY int) (int, int, bool) {
	if sourceX >= 0 && sourceY >= 0 && (sourceX != 0 || sourceY != 0) {
		return sourceX, sourceY, true
	}
	gs := Models.GetGameState()
	f := gs.CastleFocus
	if f.MapPX != 0 || f.MapPY != 0 {
		return f.MapPX, f.MapPY, true
	}
	if x, y, ok := gs.ResolveCastleMapCoords(f.CastleAID, f.KingdomID); ok {
		return x, y, true
	}
	return 0, 0, false
}
