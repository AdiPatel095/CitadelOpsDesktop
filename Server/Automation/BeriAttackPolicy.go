package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	beriKingdomRefreshAge   = 5 * time.Minute
	beriCampOpenRetryWindow = 5 * time.Minute
	beriLaunchRetryInterval = 2 * time.Second
)

type BeriAttackPolicy struct{}

func NewBeriAttackPolicy() *BeriAttackPolicy { return &BeriAttackPolicy{} }

func (*BeriAttackPolicy) ID() string         { return "autoBeriWorldAttack" }
func (*BeriAttackPolicy) EnabledKey() string { return "auto_beri_world" }
func (*BeriAttackPolicy) ActorID() string    { return "autoBeriWorld" }
func (*BeriAttackPolicy) ScheduleKey() string {
	return "autoBeriWorld"
}

func (*BeriAttackPolicy) WakeDomains() []string {
	return []string{"beri", "boosters", "castles", "commanders", "events", "event-scores", "kingdom-transport", "map-berimond", "movements", "units"}
}

// A returned commander and its troops reopen one launch slot independently of
// every other Berimond movement. Skip shared state-event coalescing so that
// slot is refilled as a rolling window rather than appearing batch-paced.
func (*BeriAttackPolicy) UrgentWakeDomains() []string {
	return []string{"commanders", "movements", "units"}
}

func (*BeriAttackPolicy) WakeSections() []string {
	return []string{autoBeriWorldSection, AttackPresets.ConfigurationSection, commanderFeatureSection}
}

func (*BeriAttackPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := beriSettings{AttackCheckIntervalSec: 30, HorseTravelBoostID: -1}
	if !decodeSection(snapshot.Configuration, autoBeriWorldSection, &settings) {
		return beriAttackWaiting(snapshot.Now, "Auto Beri World settings have not been saved", 30), nil
	}
	if decision, locked := limitedEventGate(
		snapshot.State, snapshot.Now, []int64{GameData.BerimondEventID}, "Battle for Berimond",
	); locked {
		return decision, nil
	}
	interval := policyInterval(settings.AttackCheckIntervalSec, 30)
	if decision := beriGallantryBoosterGate(snapshot, settings); decision != nil {
		return *decision, nil
	}
	if settings.PresetID == "" {
		return beriAttackWaiting(snapshot.Now, "Choose a Berimond attack preset", settings.AttackCheckIntervalSec), nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return beriAttackWaiting(snapshot.Now, "Choose a supported horse travel boost", settings.AttackCheckIntervalSec), nil
	}
	if snapshot.GameData == nil {
		return beriAttackWaiting(snapshot.Now, "Official game data is unavailable", settings.AttackCheckIntervalSec), nil
	}
	preset, err := beriAttackPreset(snapshot, settings)
	if err != nil {
		return beriAttackWaiting(snapshot.Now, err.Error(), settings.AttackCheckIntervalSec), nil
	}
	commanderIDs, restricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoBeriWorld")
	if restricted && len(commanderIDs) == 0 {
		return beriAttackWaiting(snapshot.Now, "No commanders are assigned to Auto Beri World", settings.AttackCheckIntervalSec), nil
	}

	unlock, unlockObserved := snapshot.State.KingdomTransport.Unlocks[State.KingdomID(GameData.BerimondKingdomID)]
	if unlockObserved && !unlock.Unlocked {
		return Decision{
			Status: "complete", Detail: "The Battle for Berimond is not currently unlocked",
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	castle, castleFound := beriCastle(snapshot.State)
	if !castleFound {
		if snapshot.State.KingdomTransport.ObservedAt.IsZero() ||
			snapshot.Now.Sub(snapshot.State.KingdomTransport.ObservedAt) >= beriKingdomRefreshAge {
			return beriAttackIntentDecision(
				snapshot.Now, interval, "Refresh Berimond kingdom availability",
				"troops.kingdom.refresh", nil,
			), nil
		}
		if !unlockObserved || !unlock.Unlocked {
			return Decision{
				Status: "complete", Detail: "The Battle for Berimond is not currently available",
				NextCheckAt: snapshot.Now.Add(interval),
			}, nil
		}
		if unlock.Created {
			return beriAttackWaiting(snapshot.Now, "Waiting for the created Berimond camp to appear", settings.AttackCheckIntervalSec), nil
		}
		if !snapshot.State.Beri.CampOpenRequestedAt.IsZero() &&
			snapshot.Now.Sub(snapshot.State.Beri.CampOpenRequestedAt) < beriCampOpenRetryWindow {
			return beriAttackWaiting(snapshot.Now, "Waiting for the resource Berimond camp to open", settings.AttackCheckIntervalSec), nil
		}
		option, found := snapshot.GameData.CheapestNonPremiumBerimondCamp(snapshot.State.Player.Level)
		if !found {
			return beriAttackWaiting(snapshot.Now, "No unlocked non-premium Berimond camp is available", settings.AttackCheckIntervalSec), nil
		}
		return beriAttackIntentDecision(
			snapshot.Now, interval, fmt.Sprintf("Open non-premium Berimond camp %d", option.ID),
			"beri.camp.open", map[string]any{"campId": option.ID},
		), nil
	}

	commanderID, available := nextAvailableFeatureCommander(snapshot.State, commanderIDs, restricted, snapshot.Now)
	if !available {
		detail := "No commander is currently available"
		if restricted {
			detail = "No assigned Auto Beri World commander is currently available"
		}
		return beriAttackWaiting(snapshot.Now, detail, settings.AttackCheckIntervalSec), nil
	}
	target, targetFound := beriPendingTarget(snapshot.State, snapshot.Now)
	if !targetFound {
		return beriAttackIntentDecision(
			snapshot.Now, beriLaunchRetryInterval, "Find the next available Berimond tower",
			"beri.target.find", map[string]any{"sourceCastleId": castle.ID},
		), nil
	}
	baselineCapacity, err := (AttackCapacity.Resolver{}).ResolveContext(AttackCapacity.Context{
		Target: AttackCapacity.TargetContext{
			TargetType: AttackCapacity.TargetTypeBerimondTower,
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID,
				X: target.X, Y: target.Y, ObjectID: target.ObjectID, Level: target.Level,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return beriAttackWaiting(snapshot.Now, "Waiting for a complete Berimond tower target: "+err.Error(), settings.AttackCheckIntervalSec), nil
	}
	limitedPreset := AttackPresets.LimitToCapacity(preset, baselineCapacity)
	if itemID, required, availableCount, shortage := invasionPresetShortage(limitedPreset, castle); shortage {
		return Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Waiting for Berimond attack inventory: preset needs %d of item %d; camp has %d",
				required, itemID, availableCount,
			),
			NextCheckAt: snapshot.Now.Add(interval),
		}, nil
	}
	unreflectedLaunches := unreflectedBeriTowerLaunches(snapshot.State, castle, snapshot.Now)
	maximumCopies := len(snapshot.State.Commanders) + unreflectedLaunches + 1
	if availablePresetCopies(limitedPreset, castle, maximumCopies) <= unreflectedLaunches {
		if unreflectedLaunches > 0 {
			return Decision{
				Status: "waiting",
				Detail: fmt.Sprintf(
					"Waiting for Berimond inventory to reflect %d launched attack(s)", unreflectedLaunches,
				),
				NextCheckAt: snapshot.Now.Add(beriLaunchRetryInterval),
			}, nil
		}
		return beriAttackWaiting(snapshot.Now, "The selected Berimond preset has no launchable troops", settings.AttackCheckIntervalSec), nil
	}
	return beriAttackIntentDecision(
		snapshot.Now, beriLaunchRetryInterval, fmt.Sprintf("Attack Berimond tower at %d:%d", target.X, target.Y),
		"beri.tower.attack", map[string]any{
			"sourceCastleId": castle.ID, "targetX": target.X, "targetY": target.Y,
			"targetTypeId": target.TypeID, "targetObservedAt": snapshot.State.Beri.TargetObservedAt,
			"commanderId": commanderID, "preset": preset, "horseTravelBoostId": settings.HorseTravelBoostID,
		},
	), nil
}

func beriPendingTarget(gameState State.GameState, now time.Time) (State.MapObservation, bool) {
	state := gameState.Beri
	if state.TargetObservedAt.IsZero() || !state.TargetInvalidatedAt.Before(state.TargetObservedAt) {
		return State.MapObservation{}, false
	}
	target, found := gameState.LookupMapObservation(State.KingdomID(GameData.BerimondKingdomID), fmt.Sprintf("%d:%d", state.TargetX, state.TargetY))
	if !found || state.TargetTypeID != AttackCapacity.BerimondTowerMapTypeID ||
		target.TypeID != AttackCapacity.BerimondTowerMapTypeID || target.Level <= 0 ||
		target.ObservedAt.Before(state.TargetObservedAt) {
		return State.MapObservation{}, false
	}
	return target, true
}

func unreflectedBeriTowerLaunches(
	gameState State.GameState,
	source State.CastleState,
	now time.Time,
) int {
	count := 0
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.SourceCastleID != source.ID ||
			movement.KingdomID != State.KingdomID(GameData.BerimondKingdomID) ||
			!State.MovementOwnedByCurrentPlayer(gameState, movement) ||
			!State.CommanderMovementActiveAt(movement, now) {
			return true
		}
		isTower := movement.TargetTypeID == AttackCapacity.BerimondTowerMapTypeID
		if !isTower {
			beri := gameState.Beri
			isTower = beri.TargetTypeID == AttackCapacity.BerimondTowerMapTypeID &&
				movement.TargetX == beri.TargetX && movement.TargetY == beri.TargetY
		}
		if !isTower {
			return true
		}
		launchedAt := movement.StartedAt
		if launchedAt.IsZero() {
			launchedAt = movement.ObservedAt
		}
		if source.UnitsObservedAt.IsZero() || launchedAt.IsZero() || launchedAt.After(source.UnitsObservedAt) {
			count++
		}
		return true
	})
	return count
}

func beriAttackWaiting(now time.Time, detail string, seconds int) Decision {
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: now.Add(policyInterval(seconds, 30))}
}

func beriAttackIntentDecision(
	now time.Time,
	interval time.Duration,
	detail string,
	name string,
	arguments map[string]any,
) Decision {
	raw := json.RawMessage(`{}`)
	if arguments != nil {
		raw, _ = json.Marshal(arguments)
	}
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: now.Add(interval),
		Request:             &Intent.Request{Name: name, Arguments: raw},
		ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}
}
