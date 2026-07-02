package featureview

import (
	"time"

	"CitadelDesktop/Server/Models"
)

func featureScheduleBlockedUntil(st *Models.SettingsState, featureID string, now time.Time, fallback, minSleep time.Duration) (time.Time, bool) {
	if st == nil || st.FeatureScheduleAllows(featureID, now) {
		return time.Time{}, false
	}
	if fallback <= 0 {
		fallback = 30 * time.Second
	}
	if minSleep <= 0 {
		minSleep = 5 * time.Second
	}
	sleepUntil := now.Add(fallback)
	if next, ok := st.NextFeatureScheduleStart(featureID, now); ok && next.Before(sleepUntil) {
		sleepUntil = next
	}
	if min := now.Add(minSleep); sleepUntil.Before(min) {
		sleepUntil = min
	}
	return sleepUntil, true
}
