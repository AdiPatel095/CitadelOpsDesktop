package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
)

const recruitmentSlotCapacityEffectID = 189
const hospitalSlotCapacitySubscriptionBonus = 5

var (
	subscriptionBuffsOnce                          sync.Once
	subscriptionRecruitmentSlotCapacityBonusByType map[int]int
)

var fallbackSubscriptionRecruitmentSlotCapacityBonusByType = map[int]int{
	1: 40,
}

func buildSubscriptionRecruitmentSlotCapacityBonusByType() {
	subscriptionRecruitmentSlotCapacityBonusByType = make(map[int]int, len(fallbackSubscriptionRecruitmentSlotCapacityBonusByType))
	for typeID, bonus := range fallbackSubscriptionRecruitmentSlotCapacityBonusByType {
		subscriptionRecruitmentSlotCapacityBonusByType[typeID] = bonus
	}

	b, err := serverdata.ReadSubscriptionBuffsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		SubscriptionTypeID string `json:"subscriptionTypeID"`
		Effects            string `json:"effects"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		typeID, err := strconv.Atoi(strings.TrimSpace(r.SubscriptionTypeID))
		if err != nil || typeID <= 0 {
			continue
		}
		bonus := recruitmentSlotCapacityBonusFromEffects(r.Effects)
		if bonus <= 0 {
			continue
		}
		subscriptionRecruitmentSlotCapacityBonusByType[typeID] = bonus
	}
}

func recruitmentSlotCapacityBonusFromEffects(effects string) int {
	total := 0
	for _, effect := range strings.Split(effects, ",") {
		effect = strings.TrimSpace(effect)
		if effect == "" {
			continue
		}
		idText, valueText, ok := strings.Cut(effect, "&")
		if !ok {
			continue
		}
		effectID, err := strconv.Atoi(strings.TrimSpace(idText))
		if err != nil || effectID != recruitmentSlotCapacityEffectID {
			continue
		}
		if strings.ContainsAny(valueText, "+#") {
			continue
		}
		bonus, err := strconv.Atoi(strings.TrimSpace(valueText))
		if err != nil || bonus <= 0 {
			continue
		}
		total += bonus
	}
	return total
}

// SubscriptionRecruitmentSlotCapacityBonus returns the recruit stack capacity bonus for one subscription type.
func SubscriptionRecruitmentSlotCapacityBonus(subscriptionTypeID int) (int, bool) {
	if subscriptionTypeID <= 0 {
		return 0, false
	}
	subscriptionBuffsOnce.Do(buildSubscriptionRecruitmentSlotCapacityBonusByType)
	bonus, ok := subscriptionRecruitmentSlotCapacityBonusByType[subscriptionTypeID]
	return bonus, ok && bonus > 0
}

// ActiveSubscriptionRecruitmentSlotCapacityBonus sums recruit stack capacity bonuses for active subscription types.
func ActiveSubscriptionRecruitmentSlotCapacityBonus(activeSubscriptionTypeIDs []int) int {
	total := 0
	seen := make(map[int]struct{}, len(activeSubscriptionTypeIDs))
	for _, typeID := range activeSubscriptionTypeIDs {
		if typeID <= 0 {
			continue
		}
		if _, ok := seen[typeID]; ok {
			continue
		}
		seen[typeID] = struct{}{}
		if bonus, ok := SubscriptionRecruitmentSlotCapacityBonus(typeID); ok {
			total += bonus
		}
	}
	return total
}

// ActiveSubscriptionHospitalSlotCapacityBonus returns the hospital per-queue unit bonus for subscription
// types that carry the same recruit stack-capacity benefit.
func ActiveSubscriptionHospitalSlotCapacityBonus(activeSubscriptionTypeIDs []int) int {
	seen := make(map[int]struct{}, len(activeSubscriptionTypeIDs))
	for _, typeID := range activeSubscriptionTypeIDs {
		if typeID <= 0 {
			continue
		}
		if _, ok := seen[typeID]; ok {
			continue
		}
		seen[typeID] = struct{}{}
		if _, ok := SubscriptionRecruitmentSlotCapacityBonus(typeID); ok {
			return hospitalSlotCapacitySubscriptionBonus
		}
	}
	return 0
}
