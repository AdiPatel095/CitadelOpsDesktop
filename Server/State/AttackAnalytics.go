package State

import (
	"reflect"
	"sort"
	"time"
)

type AttackFeatureID string

const (
	AttackFeatureAutoTowers    AttackFeatureID = "autoTowers"
	AttackFeatureAutoStorm     AttackFeatureID = "autoStorm"
	AttackFeatureAutoInvasion  AttackFeatureID = "autoInvasion"
	AttackFeatureAutoNomad     AttackFeatureID = "autoNomad"
	AttackFeatureAutoAdvisor   AttackFeatureID = "autoAdvisor"
	AttackFeatureAutoKhan      AttackFeatureID = "autoKhan"
	AttackFeatureAutoBeriWorld AttackFeatureID = "autoBeriWorld"
	AttackFeatureRiftMaiden    AttackFeatureID = "riftMaiden"
	AttackFeatureRiftReplay    AttackFeatureID = "riftReplay"

	AttackFeatureTargetSettlementGrace = 30 * time.Second
)

type AttackFeatureLaunch struct {
	MovementID   MovementID      `json:"movementId"`
	FeatureID    AttackFeatureID `json:"featureId"`
	KingdomID    KingdomID       `json:"kingdomId"`
	TargetTypeID int             `json:"targetTypeId,omitempty"`
	TargetX      int             `json:"targetX"`
	TargetY      int             `json:"targetY"`
	LaunchedAt   time.Time       `json:"launchedAt"`
	ArrivesAt    time.Time       `json:"arrivesAt,omitempty"`
}

type AttackAnalyticsState struct {
	LaunchIDs               []MovementID          `json:"launchIds,omitempty"`
	PendingAttacks          []AttackFeatureLaunch `json:"pendingAttacks,omitempty"`
	RecentAutoStormLaunches []AttackFeatureLaunch `json:"recentAutoStormLaunches,omitempty"`
}

func IsAttackAnalyticsFeature(featureID AttackFeatureID) bool {
	switch featureID {
	case AttackFeatureAutoTowers, AttackFeatureAutoStorm:
		return true
	default:
		return false
	}
}

func IsReportAnalyticsFeature(featureID AttackFeatureID) bool {
	switch featureID {
	case AttackFeatureAutoTowers, AttackFeatureAutoStorm,
		AttackFeatureAutoInvasion, AttackFeatureAutoNomad, AttackFeatureAutoAdvisor, AttackFeatureAutoKhan,
		AttackFeatureAutoBeriWorld, AttackFeatureRiftMaiden, AttackFeatureRiftReplay:
		return true
	default:
		return false
	}
}

func EventActivityFeature(kind EventActivityKind) AttackFeatureID {
	switch kind {
	case EventActivityInvasion:
		return AttackFeatureAutoInvasion
	case EventActivityCamp:
		return AttackFeatureAutoNomad
	case EventActivityAdvisor:
		return AttackFeatureAutoAdvisor
	case EventActivityKhan, EventActivityKhanDefense:
		return AttackFeatureAutoKhan
	default:
		return ""
	}
}

func AttackFeatureTargetPendingAt(
	gameState GameState,
	featureID AttackFeatureID,
	kingdomID KingdomID,
	targetTypeID int,
	targetX int,
	targetY int,
	now time.Time,
) bool {
	for _, pending := range gameState.AttackAnalytics.PendingAttacks {
		if pending.FeatureID != featureID || pending.KingdomID != kingdomID ||
			pending.TargetX != targetX || pending.TargetY != targetY ||
			targetTypeID > 0 && pending.TargetTypeID > 0 && pending.TargetTypeID != targetTypeID {
			continue
		}
		settlesAt := pending.ArrivesAt
		if settlesAt.IsZero() {
			settlesAt = pending.LaunchedAt
		}
		if settlesAt.IsZero() {
			continue
		}
		if now.IsZero() || settlesAt.Add(AttackFeatureTargetSettlementGrace).After(now) {
			return true
		}
	}
	return false
}

func RecordAttackFeatureLaunch(gameState *GameState, record AttackFeatureLaunch) bool {
	if gameState == nil || record.MovementID <= 0 || !IsReportAnalyticsFeature(record.FeatureID) {
		return false
	}
	for _, movementID := range gameState.AttackAnalytics.LaunchIDs {
		if movementID == record.MovementID {
			return false
		}
	}
	gameState.AttackAnalytics.LaunchIDs = append(gameState.AttackAnalytics.LaunchIDs, record.MovementID)
	if len(gameState.AttackAnalytics.LaunchIDs) > 12_000 {
		gameState.AttackAnalytics.LaunchIDs = append(
			[]MovementID(nil), gameState.AttackAnalytics.LaunchIDs[len(gameState.AttackAnalytics.LaunchIDs)-12_000:]...,
		)
	}
	gameState.AttackAnalytics.PendingAttacks = append(gameState.AttackAnalytics.PendingAttacks, record)
	if len(gameState.AttackAnalytics.PendingAttacks) > 512 {
		gameState.AttackAnalytics.PendingAttacks = append(
			[]AttackFeatureLaunch(nil), gameState.AttackAnalytics.PendingAttacks[len(gameState.AttackAnalytics.PendingAttacks)-512:]...,
		)
	}
	if record.FeatureID == AttackFeatureAutoStorm {
		MergeAutoStormLaunchHistory(gameState, []AttackFeatureLaunch{record}, record.LaunchedAt)
	}
	return true
}

func MergeAutoStormLaunchHistory(gameState *GameState, records []AttackFeatureLaunch, now time.Time) bool {
	if gameState == nil || len(records) == 0 {
		return false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.UTC().Add(-72 * time.Hour)
	merged := make(map[MovementID]AttackFeatureLaunch, len(gameState.AttackAnalytics.RecentAutoStormLaunches)+len(records))
	for _, record := range append(append([]AttackFeatureLaunch(nil), gameState.AttackAnalytics.RecentAutoStormLaunches...), records...) {
		if record.MovementID <= 0 || record.LaunchedAt.IsZero() || record.LaunchedAt.Before(cutoff) ||
			record.FeatureID != AttackFeatureAutoStorm {
			continue
		}
		current, exists := merged[record.MovementID]
		if !exists || record.LaunchedAt.After(current.LaunchedAt) {
			record.LaunchedAt = record.LaunchedAt.UTC()
			merged[record.MovementID] = record
		}
	}
	next := make([]AttackFeatureLaunch, 0, len(merged))
	for _, record := range merged {
		next = append(next, record)
	}
	sort.Slice(next, func(left, right int) bool {
		if next[left].LaunchedAt.Equal(next[right].LaunchedAt) {
			return next[left].MovementID < next[right].MovementID
		}
		return next[left].LaunchedAt.Before(next[right].LaunchedAt)
	})
	if len(next) > 12_000 {
		next = append([]AttackFeatureLaunch(nil), next[len(next)-12_000:]...)
	}
	if reflect.DeepEqual(gameState.AttackAnalytics.RecentAutoStormLaunches, next) {
		return false
	}
	gameState.AttackAnalytics.RecentAutoStormLaunches = next
	return true
}
