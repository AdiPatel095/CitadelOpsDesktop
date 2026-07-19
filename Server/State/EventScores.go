package State

import "time"

type ScalableEventScore struct {
	EventID            int64     `json:"eventId"`
	EventType          string    `json:"eventType,omitempty"`
	Name               string    `json:"name,omitempty"`
	LocalizationKey    string    `json:"localizationKey,omitempty"`
	DifficultyID       int64     `json:"difficultyId,omitempty"`
	DifficultyTypeID   int64     `json:"difficultyTypeId,omitempty"`
	DifficultyTypeName string    `json:"difficultyTypeName,omitempty"`
	PlayerScore        int64     `json:"playerScore"`
	AllianceScore      int64     `json:"allianceScore"`
	PlayerRank         int64     `json:"playerRank,omitempty"`
	AllianceRank       int64     `json:"allianceRank,omitempty"`
	RemainingSec       int64     `json:"remainingSec,omitempty"`
	ObservedAt         time.Time `json:"observedAt"`
}

type EventScoreState struct {
	ActiveEventID int64                        `json:"activeEventId,omitempty"`
	ByEvent       map[int64]ScalableEventScore `json:"byEvent"`
	ShopByPackage map[PackageID]EventShopRoute `json:"shopByPackage"`
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
	score, found := state.EventScores.ByEvent[state.EventScores.ActiveEventID]
	return score, found
}

func (state GameState) ScalableEventScoreReached(eventID int64, threshold int64) bool {
	score, found := state.EventScores.ByEvent[eventID]
	return threshold > 0 && found && score.PlayerScore >= threshold
}

func (state GameState) ActiveScalableEventScoreReached(threshold int64) bool {
	score, found := state.ActiveScalableEventScore()
	return threshold > 0 && found && score.PlayerScore >= threshold
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
