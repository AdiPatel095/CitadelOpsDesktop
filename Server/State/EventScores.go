package State

import "time"

type ScalableEventScore struct {
	EventID            int64      `json:"eventId"`
	EventType          string     `json:"eventType,omitempty"`
	Name               string     `json:"name,omitempty"`
	LocalizationKey    string     `json:"localizationKey,omitempty"`
	DifficultyID       int64      `json:"difficultyId,omitempty"`
	DifficultyTypeID   int64      `json:"difficultyTypeId,omitempty"`
	DifficultyTypeName string     `json:"difficultyTypeName,omitempty"`
	PlayerScore        int64      `json:"playerScore"`
	AllianceScore      int64      `json:"allianceScore"`
	PlayerRank         int64      `json:"playerRank,omitempty"`
	AllianceRank       int64      `json:"allianceRank,omitempty"`
	RemainingSec       int64      `json:"remainingSec,omitempty"`
	LeagueID           int64      `json:"leagueId,omitempty"`
	AllianceLeagueID   int64      `json:"allianceLeagueId,omitempty"`
	RewardSetID        int64      `json:"rewardSetId,omitempty"`
	RewardPagesReached int        `json:"rewardPagesReached,omitempty"`
	RewardPagesTotal   int        `json:"rewardPagesTotal,omitempty"`
	NextRewardScore    int64      `json:"nextRewardScore,omitempty"`
	AdvisorActive      bool       `json:"advisorActive,omitempty"`
	AdvisorCurrencyID  CurrencyID `json:"advisorCurrencyId,omitempty"`
	AdvisorFree        bool       `json:"advisorFree,omitempty"`
	ObservedAt         time.Time  `json:"observedAt"`
}

type EventScoreState struct {
	ActiveEventID   int64                        `json:"activeEventId,omitempty"`
	ByEvent         map[int64]ScalableEventScore `json:"byEvent"`
	ShopByPackage   map[PackageID]EventShopRoute `json:"shopByPackage"`
	ActivityByEvent map[int64]EventActivityState `json:"activityByEvent"`
	RankingByEvent  map[int64]EventRankingState  `json:"rankingByEvent"`
	// Inventory is the last authoritative `sei` inventory for this account.
	// It deliberately remains account-private: event and shop eligibility can
	// vary by server, level, rollout, or account even when the public calendar
	// is shared.
	Inventory EventInventoryState `json:"inventory"`
}

type EventInventoryState struct {
	ObservedAt    time.Time                   `json:"observedAt,omitempty"`
	ActiveByEvent map[int64]EventAvailability `json:"activeByEvent"`
}

type EventAvailability struct {
	EventID int64     `json:"eventId"`
	EndsAt  time.Time `json:"endsAt"`
}

func (availability EventAvailability) ActiveAt(now time.Time) bool {
	return availability.EventID > 0 && !availability.EndsAt.IsZero() && now.Before(availability.EndsAt)
}

func (state GameState) EventAvailable(eventID int64, now time.Time) (EventAvailability, bool) {
	availability, found := state.EventScores.Inventory.ActiveByEvent[eventID]
	return availability, found && availability.ActiveAt(now)
}

func (state GameState) AnyEventAvailable(eventIDs []int64, now time.Time) (EventAvailability, bool) {
	for _, eventID := range eventIDs {
		if availability, found := state.EventAvailable(eventID, now); found {
			return availability, true
		}
	}
	return EventAvailability{}, false
}

type EventActivityKind string

const (
	EventActivityInvasion    EventActivityKind = "invasion"
	EventActivityCamp        EventActivityKind = "camp"
	EventActivityAdvisor     EventActivityKind = "advisor"
	EventActivityKhan        EventActivityKind = "khan"
	EventActivityKhanDefense EventActivityKind = "khanDefense"
)

type EventCombatTotals struct {
	Launches    int64 `json:"launches"`
	Battles     int64 `json:"battles"`
	Victories   int64 `json:"victories"`
	Defeats     int64 `json:"defeats"`
	TroopLosses int64 `json:"troopLosses"`
	ToolsUsed   int64 `json:"toolsUsed"`
	Loot        int64 `json:"loot"`
}

type EventAttackRecord struct {
	MovementID   MovementID        `json:"movementId"`
	Kind         EventActivityKind `json:"kind"`
	KingdomID    KingdomID         `json:"kingdomId"`
	TargetTypeID int               `json:"targetTypeId,omitempty"`
	TargetX      int               `json:"targetX"`
	TargetY      int               `json:"targetY"`
	LaunchedAt   time.Time         `json:"launchedAt"`
	ArrivesAt    time.Time         `json:"arrivesAt,omitempty"`
}

type EventActivityState struct {
	EventID                     int64               `json:"eventId"`
	OccurrenceEndsAt            time.Time           `json:"occurrenceEndsAt,omitempty"`
	ObservedFrom                time.Time           `json:"observedFrom"`
	Invasion                    EventCombatTotals   `json:"invasion"`
	Camp                        EventCombatTotals   `json:"camp"`
	Advisor                     EventCombatTotals   `json:"advisor"`
	Khan                        EventCombatTotals   `json:"khan"`
	KhanDefense                 EventCombatTotals   `json:"khanDefense"`
	AdvisorObservedAt           time.Time           `json:"advisorObservedAt,omitempty"`
	FortifyCurrencies           []string            `json:"fortifyCurrencies,omitempty"`
	FortifyCurrenciesObservedAt time.Time           `json:"fortifyCurrenciesObservedAt,omitempty"`
	LaunchIDs                   []MovementID        `json:"launchIds,omitempty"`
	PendingAttacks              []EventAttackRecord `json:"pendingAttacks,omitempty"`
	ProcessedReportIDs          []int64             `json:"processedReportIds,omitempty"`
}

type EventOccurrence struct {
	EndsAt       time.Time
	ObservedFrom time.Time
}

type EventRankingEntry struct {
	Alliance    string     `json:"alliance,omitempty"`
	AllianceID  AllianceID `json:"allianceId,omitempty"`
	MemberCount int64      `json:"memberCount,omitempty"`
	FamePoints  int64      `json:"famePoints,omitempty"`
	Rank        int64      `json:"rank"`
	Score       int64      `json:"score"`
}

type EventRankingState struct {
	EventID        int64               `json:"eventId"`
	Scope          string              `json:"scope,omitempty"`
	LeagueID       int64               `json:"leagueId"`
	ListType       int64               `json:"listType"`
	TotalAlliances int64               `json:"totalAlliances"`
	OwnAllianceID  AllianceID          `json:"ownAllianceId,omitempty"`
	SearchValue    string              `json:"searchValue,omitempty"`
	FirstRank      int64               `json:"firstRank,omitempty"`
	GlobalFlag     int64               `json:"globalFlag,omitempty"`
	Entries        []EventRankingEntry `json:"entries"`
	Pending        bool                `json:"pending,omitempty"`
	ObservedAt     time.Time           `json:"observedAt,omitempty"`
}

type EventShopRoute struct {
	EventID      int64     `json:"eventId"`
	RemainingSec int64     `json:"remainingSec"`
	ObservedAt   time.Time `json:"observedAt"`
}

func (state GameState) ActiveScalableEventScore() (ScalableEventScore, bool) {
	if state.EventScores.ActiveEventID <= 0 {
		return ScalableEventScore{}, false
	}
	score, found := state.LookupScalableEventScore(state.EventScores.ActiveEventID)
	return score, found
}

func (state GameState) ScalableEventScoreReached(eventID int64, threshold int64) bool {
	score, found := state.LookupScalableEventScore(eventID)
	return threshold > 0 && found && score.PlayerScore >= threshold
}

func (state GameState) ActiveScalableEventScoreReached(threshold int64) bool {
	score, found := state.ActiveScalableEventScore()
	return threshold > 0 && found && score.PlayerScore >= threshold
}

func ScalableEventEndsAt(score ScalableEventScore) time.Time {
	if score.ObservedAt.IsZero() || score.RemainingSec <= 0 {
		return time.Time{}
	}
	return score.ObservedAt.Add(time.Duration(score.RemainingSec) * time.Second).UTC()
}

func EnsureEventActivity(gameState *GameState, eventID int64, observedAt time.Time) (EventActivityState, bool) {
	if gameState == nil || eventID <= 0 {
		return EventActivityState{}, false
	}
	score, active := gameState.LookupScalableEventScore(eventID)
	if !active {
		return EventActivityState{}, false
	}
	endsAt := ScalableEventEndsAt(score)
	current, exists := gameState.MutableEventActivity(eventID)
	if !exists || !SameEventOccurrence(current.OccurrenceEndsAt, endsAt) {
		if observedAt.IsZero() {
			observedAt = score.ObservedAt
		}
		current = EventActivityState{
			EventID: eventID, OccurrenceEndsAt: endsAt, ObservedFrom: observedAt.UTC(),
			LaunchIDs: []MovementID{}, PendingAttacks: []EventAttackRecord{}, ProcessedReportIDs: []int64{},
		}
		gameState.SetEventActivity(eventID, current)
		return current, true
	}
	return current, true
}

func RecordEventAttackLaunch(gameState *GameState, eventID int64, record EventAttackRecord) bool {
	activity, ok := EnsureEventActivity(gameState, eventID, record.LaunchedAt)
	if !ok || record.MovementID <= 0 {
		return false
	}
	for _, movementID := range activity.LaunchIDs {
		if movementID == record.MovementID {
			return false
		}
	}
	activity.LaunchIDs = append(activity.LaunchIDs, record.MovementID)
	if len(activity.LaunchIDs) > 12_000 {
		activity.LaunchIDs = append([]MovementID(nil), activity.LaunchIDs[len(activity.LaunchIDs)-12_000:]...)
	}
	activity.PendingAttacks = append(activity.PendingAttacks, record)
	if len(activity.PendingAttacks) > 512 {
		activity.PendingAttacks = append([]EventAttackRecord(nil), activity.PendingAttacks[len(activity.PendingAttacks)-512:]...)
	}
	totals := EventCombatTotalsFor(&activity, record.Kind)
	if totals == nil {
		return false
	}
	totals.Launches++
	gameState.SetEventActivity(eventID, activity)
	return true
}

func UpdateAdvisorEventActivity(gameState *GameState, eventID int64, observedAt time.Time, currentAttack, wins, defeats int64, troopLosses, toolsUsed int64) bool {
	activity, ok := EnsureEventActivity(gameState, eventID, observedAt)
	if !ok || !activity.AdvisorObservedAt.IsZero() && observedAt.Before(activity.AdvisorObservedAt) {
		return false
	}
	previous := activity.Advisor
	previousObservedAt := activity.AdvisorObservedAt
	completed := max(int64(0), wins) + max(int64(0), defeats)
	activity.Advisor.Launches = max(activity.Advisor.Launches, currentAttack, completed)
	activity.Advisor.Battles = completed
	activity.Advisor.Victories = max(int64(0), wins)
	activity.Advisor.Defeats = max(int64(0), defeats)
	activity.Advisor.TroopLosses = max(int64(0), troopLosses)
	activity.Advisor.ToolsUsed = max(int64(0), toolsUsed)
	activity.AdvisorObservedAt = observedAt.UTC()
	gameState.SetEventActivity(eventID, activity)
	return previous != activity.Advisor || !previousObservedAt.Equal(activity.AdvisorObservedAt)
}

func EventCombatTotalsFor(activity *EventActivityState, kind EventActivityKind) *EventCombatTotals {
	if activity == nil {
		return nil
	}
	switch kind {
	case EventActivityInvasion:
		return &activity.Invasion
	case EventActivityCamp:
		return &activity.Camp
	case EventActivityAdvisor:
		return &activity.Advisor
	case EventActivityKhan:
		return &activity.Khan
	case EventActivityKhanDefense:
		return &activity.KhanDefense
	default:
		return nil
	}
}

func SameEventOccurrence(left, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return left.IsZero() && right.IsZero()
	}
	delta := left.Sub(right)
	return delta >= -10*time.Minute && delta <= 10*time.Minute
}

func (state GameState) ActiveShopForPackage(packageID PackageID, now time.Time) (EventShopRoute, bool) {
	route, found := state.EventScores.ShopByPackage[packageID]
	if !found || route.EventID <= 0 || route.RemainingSec <= 0 || route.ObservedAt.IsZero() {
		return EventShopRoute{}, false
	}
	if now.After(route.ObservedAt.Add(time.Duration(route.RemainingSec) * time.Second)) {
		return EventShopRoute{}, false
	}
	return route, true
}
