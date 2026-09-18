package State

import (
	"fmt"
	"time"
)

// NomadSequentialArrivalGuardHorizon covers the server's integer-second
// arrival clock plus subsecond uncertainty between local dispatches.
const NomadSequentialArrivalGuardHorizon = 2 * time.Second

type NomadSequentialArrivalBlock struct {
	MovementID MovementID
	LaunchedAt time.Time
	ArrivesAt  time.Time
	Live       bool
	Unknown    bool
}

// NomadSequentialArrivalBlockAt returns the first exact-target Auto Nomad or
// Samurai arrival which is too close to safely launch another attack. Live
// outgoing movements are authoritative; persisted analytics retain the guard
// across batches and process restarts. Source castle is intentionally ignored.
func NomadSequentialArrivalBlockAt(
	gameState GameState,
	eventID int64,
	kingdomID KingdomID,
	targetTypeID int,
	targetX int,
	targetY int,
	now time.Time,
) (NomadSequentialArrivalBlock, bool) {
	now = now.UTC()
	pending := make(map[MovementID]EventAttackRecord)
	launched := make(map[MovementID]struct{})
	activity, found := gameState.LookupEventActivity(eventID)
	occurrence, active := gameState.LookupEventOccurrence(eventID)
	if found && active && SameEventOccurrence(activity.OccurrenceEndsAt, occurrence.EndsAt) {
		for _, movementID := range activity.LaunchIDs {
			launched[movementID] = struct{}{}
		}
		for _, launch := range activity.PendingAttacks {
			if launch.Kind != EventActivityCamp || launch.KingdomID != kingdomID ||
				launch.TargetX != targetX || launch.TargetY != targetY ||
				targetTypeID > 0 && launch.TargetTypeID > 0 && launch.TargetTypeID != targetTypeID {
				continue
			}
			pending[launch.MovementID] = launch
		}
	}

	liveIDs := make(map[MovementID]struct{})
	var selected NomadSequentialArrivalBlock
	blocked := false
	consider := func(candidate NomadSequentialArrivalBlock) {
		if !candidate.Unknown && candidate.ArrivesAt.After(now.Add(NomadSequentialArrivalGuardHorizon)) {
			return
		}
		if !blocked || candidate.Unknown && !selected.Unknown ||
			candidate.Unknown == selected.Unknown && candidate.ArrivesAt.Before(selected.ArrivesAt) {
			selected, blocked = candidate, true
		}
	}

	gameState.RangeMovements(func(id MovementID, movement MovementState) bool {
		if movement.Direction != 0 || movement.KingdomID != kingdomID || movement.TargetX != targetX || movement.TargetY != targetY ||
			targetTypeID > 0 && movement.TargetTypeID > 0 && movement.TargetTypeID != targetTypeID {
			return true
		}
		if _, ownSource := gameState.Castles[movement.SourceCastleID]; !ownSource {
			return true
		}
		liveIDs[id] = struct{}{}
		if _, knownLaunch := launched[id]; knownLaunch {
			if _, stillPending := pending[id]; !stillPending {
				return true
			}
		}
		candidate := NomadSequentialArrivalBlock{MovementID: id, Live: true, Unknown: movement.ArrivesAt == nil || movement.ArrivesAt.IsZero()}
		if launch, found := pending[id]; found {
			candidate.LaunchedAt = launch.LaunchedAt.UTC()
		}
		if candidate.LaunchedAt.IsZero() {
			candidate.LaunchedAt = movement.ObservedAt.UTC()
		}
		if !candidate.Unknown {
			candidate.ArrivesAt = movement.ArrivesAt.UTC()
		}
		consider(candidate)
		return true
	})

	for movementID, launch := range pending {
		if _, live := liveIDs[movementID]; live {
			continue
		}
		candidate := NomadSequentialArrivalBlock{
			MovementID: movementID, LaunchedAt: launch.LaunchedAt.UTC(), ArrivesAt: launch.ArrivesAt.UTC(),
			Unknown: launch.ArrivesAt.IsZero(),
		}
		if nomadPendingLaunchSettled(gameState, kingdomID, targetTypeID, targetX, targetY, candidate) {
			continue
		}
		consider(candidate)
	}
	return selected, blocked
}

func nomadPendingLaunchSettled(
	gameState GameState,
	kingdomID KingdomID,
	targetTypeID int,
	targetX int,
	targetY int,
	launch NomadSequentialArrivalBlock,
) bool {
	threshold := gameState.MovementSnapshot.ObservedAt
	if !launch.ArrivesAt.IsZero() {
		threshold = launch.ArrivesAt.Add(NomadSequentialArrivalGuardHorizon)
	}
	if threshold.IsZero() || launch.ArrivesAt.IsZero() && threshold.Before(launch.LaunchedAt) {
		return false
	}
	key := fmt.Sprintf("%d:%d:%d", kingdomID, targetX, targetY)
	cooldown, hasCooldown := gameState.NomadCamps.Cooldowns[key]
	if hasCooldown && cooldown.PendingCooldownRefresh {
		return false
	}
	observation, hasObservation := gameState.LookupMapObservation(kingdomID, fmt.Sprintf("%d:%d", targetX, targetY))
	hasObservation = hasObservation && observation.TypeID == targetTypeID
	if hasCooldown && (!hasObservation || !cooldown.CooldownObservedAt.Before(observation.ObservedAt)) {
		return cooldown.CooldownRemaining <= 0 && !cooldown.CooldownObservedAt.Before(threshold) &&
			!cooldown.CooldownObservedAt.Before(cooldown.LastSuccessfulBattleAt)
	}
	return hasObservation && observation.EventCampCooldownRemaining <= 0 && !observation.ObservedAt.Before(threshold) &&
		(!hasCooldown || !observation.ObservedAt.Before(cooldown.LastSuccessfulBattleAt))
}
