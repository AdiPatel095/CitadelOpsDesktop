package Automation

import (
	"encoding/json"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
)

const automationEnabledSection = "automation.enabled"

type automationEnabledControl struct {
	Enabled   bool      `json:"enabled"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	Timed     bool      `json:"-"`
}

func automationEnabledControls(snapshot Configuration.Snapshot) map[string]automationEnabledControl {
	result := map[string]automationEnabledControl{}
	var document map[string]json.RawMessage
	if json.Unmarshal(snapshot.Sections[automationEnabledSection], &document) != nil {
		return result
	}
	for feature, raw := range document {
		feature = strings.TrimSpace(feature)
		if feature == "" {
			continue
		}
		var enabled bool
		if json.Unmarshal(raw, &enabled) == nil {
			result[feature] = automationEnabledControl{Enabled: enabled}
			continue
		}
		var value struct {
			Enabled   bool       `json:"enabled"`
			ExpiresAt *time.Time `json:"expiresAt"`
		}
		if json.Unmarshal(raw, &value) != nil || value.ExpiresAt == nil || value.ExpiresAt.IsZero() {
			continue
		}
		result[feature] = automationEnabledControl{
			Enabled: value.Enabled, ExpiresAt: value.ExpiresAt.UTC(), Timed: true,
		}
	}
	return result
}

func (control automationEnabledControl) enabledAt(now time.Time) bool {
	return control.Enabled && (!control.Timed || now.Before(control.ExpiresAt))
}

func enabledFeatures(snapshot Configuration.Snapshot, now time.Time) map[string]bool {
	result := map[string]bool{}
	for feature, control := range automationEnabledControls(snapshot) {
		result[feature] = control.enabledAt(now)
	}
	return result
}

func configuredEnabledFeatures(snapshot Configuration.Snapshot) map[string]bool {
	result := map[string]bool{}
	for feature, control := range automationEnabledControls(snapshot) {
		result[feature] = control.Enabled
	}
	return result
}

func nextAutomationExpiration(snapshot Configuration.Snapshot, now time.Time) time.Time {
	var next time.Time
	for _, control := range automationEnabledControls(snapshot) {
		if !control.Enabled || !control.Timed {
			continue
		}
		if !control.ExpiresAt.After(now) {
			return now
		}
		if next.IsZero() || control.ExpiresAt.Before(next) {
			next = control.ExpiresAt
		}
	}
	return next
}

// Hosted settings remain account-owned after their effective expiration. The
// coordinator still wakes for future deadlines, but ignores already-expired
// values instead of repeatedly scheduling normalization writes.
func nextFutureAutomationExpiration(snapshot Configuration.Snapshot, now time.Time) time.Time {
	var next time.Time
	for _, control := range automationEnabledControls(snapshot) {
		if !control.Enabled || !control.Timed || !control.ExpiresAt.After(now) {
			continue
		}
		if next.IsZero() || control.ExpiresAt.Before(next) {
			next = control.ExpiresAt
		}
	}
	return next
}

func expiredAutomationEnabledDocument(snapshot Configuration.Snapshot, now time.Time) (json.RawMessage, bool) {
	var document map[string]json.RawMessage
	if json.Unmarshal(snapshot.Sections[automationEnabledSection], &document) != nil || document == nil {
		return nil, false
	}
	changed := false
	for feature, control := range automationEnabledControls(snapshot) {
		if control.Enabled && control.Timed && !now.Before(control.ExpiresAt) {
			document[feature] = json.RawMessage(`false`)
			changed = true
		}
	}
	if !changed {
		return nil, false
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func (coordinator *Coordinator) expireTimedAutomations(now time.Time) bool {
	if coordinator == nil || coordinator.configuration == nil {
		return false
	}
	if coordinator.externalConfigurationAuthority.Load() {
		return false
	}
	for attempt := 0; attempt < 3; attempt++ {
		snapshot := coordinator.configuration.Snapshot()
		value, changed := expiredAutomationEnabledDocument(snapshot, now)
		if !changed {
			return false
		}
		if _, err := coordinator.configuration.UpdateExpected(automationEnabledSection, value, &snapshot.Revision); err == nil {
			return true
		}
	}
	return false
}
