package GameData

import (
	"strconv"
	"strings"
)

type SubscriptionEffect struct {
	ID        int
	Value     int64
	Decorated bool
}

// SubscriptionEffectsView returns immutable official subscription effects
// decoded once for the process-shared game-data generation.
func (store *Store) SubscriptionEffectsView(typeID int) []SubscriptionEffect {
	if store == nil || typeID <= 0 {
		return nil
	}
	store.subscriptionEffectsOnce.Do(func() {
		store.subscriptionEffects = map[int][]SubscriptionEffect{}
		catalog, err := store.Catalog("subscriptionsBuffs")
		if err != nil {
			store.subscriptionEffectsErr = err
			return
		}
		for _, raw := range catalog.rows {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			subscriptionTypeID, ok := record.Int64("subscriptionTypeID")
			effects, _ := record.String("effects")
			if !ok || subscriptionTypeID <= 0 {
				continue
			}
			for _, encoded := range strings.Split(effects, ",") {
				effectIDText, valueText, found := strings.Cut(strings.TrimSpace(encoded), "&")
				if !found {
					continue
				}
				effectID, effectErr := strconv.Atoi(strings.TrimSpace(effectIDText))
				value, valueErr := strconv.ParseInt(strings.TrimSpace(valueText), 10, 64)
				if effectErr != nil || valueErr != nil || effectID <= 0 {
					continue
				}
				store.subscriptionEffects[int(subscriptionTypeID)] = append(
					store.subscriptionEffects[int(subscriptionTypeID)],
					SubscriptionEffect{ID: effectID, Value: value, Decorated: strings.ContainsAny(valueText, "+#")},
				)
			}
		}
	})
	if store.subscriptionEffectsErr != nil {
		return nil
	}
	return store.subscriptionEffects[typeID]
}
