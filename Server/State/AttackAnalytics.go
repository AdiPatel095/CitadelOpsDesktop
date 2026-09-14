package State

import (
	"math"
	"reflect"
	"sort"
	"time"
)

type AttackFeatureID string

const (
	AttackFeatureAutoTowers    AttackFeatureID = "autoTowers"
	AttackFeatureAutoFortress  AttackFeatureID = "autoFortress"
	AttackFeatureAutoStorm     AttackFeatureID = "autoStorm"
	AttackFeatureAutoInvasion  AttackFeatureID = "autoInvasion"
	AttackFeatureAutoNomad     AttackFeatureID = "autoNomad"
	AttackFeatureAutoAdvisor   AttackFeatureID = "autoAdvisor"
	AttackFeatureAutoKhan      AttackFeatureID = "autoKhan"
	AttackFeatureAutoBeriWorld AttackFeatureID = "autoBeriWorld"
	AttackFeatureRiftMaiden    AttackFeatureID = "riftMaiden"
	AttackFeatureRiftReplay    AttackFeatureID = "riftReplay"

	AttackFeatureTargetSettlementGrace = 30 * time.Second
	TowerAdvisorTimeSkipUsageRetention = 72 * time.Hour
)

type AttackFeatureLaunch struct {
	MovementID   MovementID      `json:"movementId"`
	FeatureID    AttackFeatureID `json:"featureId"`
	KingdomID    KingdomID       `json:"kingdomId"`
	TroopCount   int64           `json:"troopCount,omitempty"`
	TargetTypeID int             `json:"targetTypeId,omitempty"`
	TargetX      int             `json:"targetX"`
	TargetY      int             `json:"targetY"`
	LaunchedAt   time.Time       `json:"launchedAt"`
	ArrivesAt    time.Time       `json:"arrivesAt,omitempty"`
}

// TowerAdvisorTimeSkipUsage records only confirmed Auto Towers Advisor CRA
// launches. The movement id makes retries idempotent, while UsedAt lets the
// policy apply the user's cap to the authoritative server reset session.
type TowerAdvisorTimeSkipUsage struct {
	MovementID MovementID `json:"movementId"`
	TimeSkips  int64      `json:"timeSkips"`
	UsedAt     time.Time  `json:"usedAt"`
}

type AttackAnalyticsState struct {
	LaunchIDs                   []MovementID                `json:"launchIds,omitempty"`
	PendingAttacks              []AttackFeatureLaunch       `json:"pendingAttacks,omitempty"`
	RecentAutoStormLaunches     []AttackFeatureLaunch       `json:"recentAutoStormLaunches,omitempty"`
	RecentTowerAdvisorTimeSkips []TowerAdvisorTimeSkipUsage `json:"recentTowerAdvisorTimeSkips,omitempty"`
}

func IsAttackAnalyticsFeature(featureID AttackFeatureID) bool {
	switch featureID {
	case AttackFeatureAutoTowers, AttackFeatureAutoFortress, AttackFeatureAutoStorm:
		return true
	default:
		return false
	}
}

func IsReportAnalyticsFeature(featureID AttackFeatureID) bool {
	switch featureID {
	case AttackFeatureAutoTowers, AttackFeatureAutoFortress, AttackFeatureAutoStorm,
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
	launchIDs := append(gameState.MutableAttackAnalyticsLaunchIDs(), record.MovementID)
	if len(launchIDs) > 12_000 {
		launchIDs = append([]MovementID(nil), launchIDs[len(launchIDs)-12_000:]...)
	}
	gameState.SetAttackAnalyticsLaunchIDs(launchIDs)
	pending := append(gameState.MutablePendingAttackAnalytics(), record)
	if len(pending) > 512 {
		pending = append([]AttackFeatureLaunch(nil), pending[len(pending)-512:]...)
	}
	gameState.SetPendingAttackAnalytics(pending)
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
		if exists {
			current.TroopCount = max(record.TroopCount, current.TroopCount)
			if record.LaunchedAt.Before(current.LaunchedAt) {
				current.LaunchedAt = record.LaunchedAt.UTC()
			}
			if current.ArrivesAt.IsZero() && !record.ArrivesAt.IsZero() {
				current.ArrivesAt = record.ArrivesAt.UTC()
			}
			merged[record.MovementID] = current
			continue
		}
		record.LaunchedAt = record.LaunchedAt.UTC()
		merged[record.MovementID] = record
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
	gameState.SetRecentAutoStormLaunches(next)
	return true
}

func RecordTowerAdvisorTimeSkipUsage(
	gameState *GameState,
	record TowerAdvisorTimeSkipUsage,
	now time.Time,
) bool {
	if gameState == nil || record.MovementID <= 0 || record.TimeSkips <= 0 || record.UsedAt.IsZero() {
		return false
	}
	if now.IsZero() {
		now = record.UsedAt
	}
	now = now.UTC()
	cutoff := now.Add(-TowerAdvisorTimeSkipUsageRetention)
	merged := make(map[MovementID]TowerAdvisorTimeSkipUsage, len(gameState.AttackAnalytics.RecentTowerAdvisorTimeSkips)+1)
	for _, candidate := range append(
		append([]TowerAdvisorTimeSkipUsage(nil), gameState.AttackAnalytics.RecentTowerAdvisorTimeSkips...),
		record,
	) {
		if candidate.MovementID <= 0 || candidate.TimeSkips <= 0 || candidate.UsedAt.IsZero() || candidate.UsedAt.Before(cutoff) {
			continue
		}
		candidate.UsedAt = candidate.UsedAt.UTC()
		if current, exists := merged[candidate.MovementID]; exists {
			current.TimeSkips = max(current.TimeSkips, candidate.TimeSkips)
			if candidate.UsedAt.Before(current.UsedAt) {
				current.UsedAt = candidate.UsedAt
			}
			merged[candidate.MovementID] = current
			continue
		}
		merged[candidate.MovementID] = candidate
	}
	next := make([]TowerAdvisorTimeSkipUsage, 0, len(merged))
	for _, candidate := range merged {
		next = append(next, candidate)
	}
	sort.Slice(next, func(left, right int) bool {
		if next[left].UsedAt.Equal(next[right].UsedAt) {
			return next[left].MovementID < next[right].MovementID
		}
		return next[left].UsedAt.Before(next[right].UsedAt)
	})
	if len(next) > 12_000 {
		next = append([]TowerAdvisorTimeSkipUsage(nil), next[len(next)-12_000:]...)
	}
	if reflect.DeepEqual(gameState.AttackAnalytics.RecentTowerAdvisorTimeSkips, next) {
		return false
	}
	gameState.SetRecentTowerAdvisorTimeSkips(next)
	return true
}

func TowerAdvisorTimeSkipsUsedSince(
	gameState GameState,
	since time.Time,
	now time.Time,
) (int64, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	since = since.UTC()
	if since.IsZero() || since.After(now) || since.Before(now.Add(-TowerAdvisorTimeSkipUsageRetention)) {
		return 0, false
	}
	var used int64
	for _, record := range gameState.AttackAnalytics.RecentTowerAdvisorTimeSkips {
		if record.MovementID <= 0 || record.TimeSkips <= 0 || record.UsedAt.IsZero() {
			return 0, false
		}
		usedAt := record.UsedAt.UTC()
		if usedAt.After(now) {
			return 0, false
		}
		if usedAt.Before(since) {
			continue
		}
		if used > math.MaxInt64-record.TimeSkips {
			return math.MaxInt64, true
		}
		used += record.TimeSkips
	}
	return used, true
}
