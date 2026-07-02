package gamestate

import "sort"

// ActiveSubscription stores one active player/alliance subscription snapshot from sie/upc.
type ActiveSubscription struct {
	TypeID         int `json:"typeId,omitempty"`
	RemainingSec   int `json:"remainingSec,omitempty"`
	GracePeriodSec int `json:"gracePeriodSec,omitempty"`
}

// SubscriptionState stores active subscription packages keyed by subscription type id.
type SubscriptionState struct {
	ActiveByType map[int]ActiveSubscription `json:"activeByType,omitempty"`
}

func activeSubscriptionIsActive(sub ActiveSubscription) bool {
	return sub.TypeID > 0 && sub.RemainingSec > 0
}

// ReplaceActiveSubscriptions stores the latest active subscription snapshot.
func (gs *GameState) ReplaceActiveSubscriptions(subscriptions []ActiveSubscription) {
	next := make(map[int]ActiveSubscription, len(subscriptions))
	for _, sub := range subscriptions {
		if !activeSubscriptionIsActive(sub) {
			continue
		}
		next[sub.TypeID] = sub
	}
	gs.Subscriptions.ActiveByType = next
}

// ActiveSubscriptionTypeIDs returns active subscription type ids in stable order.
func (gs *GameState) ActiveSubscriptionTypeIDs() []int {
	if gs == nil || len(gs.Subscriptions.ActiveByType) == 0 {
		return nil
	}
	ids := make([]int, 0, len(gs.Subscriptions.ActiveByType))
	for typeID, sub := range gs.Subscriptions.ActiveByType {
		if activeSubscriptionIsActive(sub) {
			ids = append(ids, typeID)
		}
	}
	sort.Ints(ids)
	return ids
}

// HasActiveSubscriptionType reports whether the subscription type currently has active time.
func (gs *GameState) HasActiveSubscriptionType(typeID int) bool {
	if gs == nil || typeID <= 0 || len(gs.Subscriptions.ActiveByType) == 0 {
		return false
	}
	sub, ok := gs.Subscriptions.ActiveByType[typeID]
	return ok && activeSubscriptionIsActive(sub)
}
