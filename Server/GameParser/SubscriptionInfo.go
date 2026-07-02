package GameParser

import (
	"encoding/json"

	"CitadelDesktop/Server/Models"
)

// UpdateSubscriptionInfo parses sie/upc subscription state. SP[].STID is the subscription type id.
func UpdateSubscriptionInfo(subscriptionMap map[string]interface{}) {
	if subscriptionMap == nil {
		return
	}
	rawSubscriptions, ok := subscriptionMap["SP"].([]interface{})
	if !ok {
		return
	}
	subscriptions := make([]Models.ActiveSubscription, 0, len(rawSubscriptions))
	for _, raw := range rawSubscriptions {
		row, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typeID := jsonIntFromMap(row, "STID")
		if typeID <= 0 {
			continue
		}
		subscriptions = append(subscriptions, Models.ActiveSubscription{
			TypeID:         typeID,
			RemainingSec:   jsonIntFromMap(row, "RS"),
			GracePeriodSec: jsonIntFromMap(row, "RSGP"),
		})
	}
	Models.GetGameState().ReplaceActiveSubscriptions(subscriptions)
}

func UpdateSubscriptionInfoFromPayload(payload string) {
	var subscriptionMap map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &subscriptionMap); err != nil {
		return
	}
	UpdateSubscriptionInfo(subscriptionMap)
}
