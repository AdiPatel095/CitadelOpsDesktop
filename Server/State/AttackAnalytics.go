package State

import "time"

type AttackFeatureID string

const (
	AttackFeatureAutoTowers   AttackFeatureID = "autoTowers"
	AttackFeatureAutoStorm    AttackFeatureID = "autoStorm"
	AttackFeatureAutoInvasion AttackFeatureID = "autoInvasion"
	AttackFeatureAutoNomad    AttackFeatureID = "autoNomad"
	AttackFeatureAutoAdvisor  AttackFeatureID = "autoAdvisor"
	AttackFeatureAutoKhan     AttackFeatureID = "autoKhan"
	AttackFeatureRiftMaiden   AttackFeatureID = "riftMaiden"
	AttackFeatureRiftReplay   AttackFeatureID = "riftReplay"

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
	LaunchIDs      []MovementID          `json:"launchIds,omitempty"`
	PendingAttacks []AttackFeatureLaunch `json:"pendingAttacks,omitempty"`
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
		AttackFeatureRiftMaiden, AttackFeatureRiftReplay:
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
	return true
}
