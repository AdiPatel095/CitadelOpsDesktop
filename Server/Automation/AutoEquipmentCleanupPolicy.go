package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/Intent"
)

const (
	autoEquipmentCleanupSection         = "automation.autoEquipmentCleanup"
	autoEquipmentCleanupDefaultInterval = 60
	minAutoEquipmentCleanupInterval     = 60
)

type AutoEquipmentCleanupPolicy struct{}

type autoEquipmentCleanupSettings struct {
	Version          int `json:"version"`
	CheckIntervalSec int `json:"checkIntervalSec"`
}

func NewAutoEquipmentCleanupPolicy() *AutoEquipmentCleanupPolicy {
	return &AutoEquipmentCleanupPolicy{}
}

func (*AutoEquipmentCleanupPolicy) ID() string { return "autoEquipmentCleanup" }

func (*AutoEquipmentCleanupPolicy) EnabledKey() string { return "auto_equipment_cleanup" }

func (*AutoEquipmentCleanupPolicy) WakeSections() []string {
	return []string{autoEquipmentCleanupSection}
}

func (*AutoEquipmentCleanupPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := autoEquipmentCleanupSettings{
		Version: 1, CheckIntervalSec: autoEquipmentCleanupDefaultInterval,
	}
	decodeSection(snapshot.Configuration, autoEquipmentCleanupSection, &settings)
	interval := autoEquipmentCleanupInterval(settings.CheckIntervalSec)
	nextCheck := snapshot.Now.Add(interval)

	if !Equipment.CleanupStorageFresh(snapshot.State, snapshot.Now) {
		return Decision{
			Status: "ready", Detail: "Refresh equipment and gem storage before automatic cleanup",
			NextCheckAt:         nextCheck,
			Request:             &Intent.Request{Name: "equipment.refresh", Arguments: json.RawMessage(`{}`)},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}

	counts := Equipment.AutomaticCleanupCounts(snapshot.State)
	metrics := map[string]float64{
		"eligibleEquipment": float64(counts.Equipment),
		"eligibleGems":      float64(counts.Gems),
	}
	if counts.Equipment > 0 {
		arguments, _ := json.Marshal(map[string]any{
			"category": "non_relic_equipment", "sellLookItems": false, "sellPost2026": false,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Sell %d eligible non-relic equipment item(s)", counts.Equipment),
			NextCheckAt: nextCheck, Metrics: metrics,
			Request:             &Intent.Request{Name: "equipment.sell", Arguments: arguments},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	if counts.Gems > 0 {
		arguments, _ := json.Marshal(map[string]any{
			"category": "non_relic_gems", "sellPost2026": false,
		})
		return Decision{
			Status: "ready", Detail: fmt.Sprintf("Sell %d eligible non-relic gem(s)", counts.Gems),
			NextCheckAt: nextCheck, Metrics: metrics,
			Request:             &Intent.Request{Name: "equipment.sell", Arguments: arguments},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	return Decision{
		Status: "idle", Detail: "Equipment and gem storage are clean",
		NextCheckAt: nextCheck, Metrics: metrics,
	}, nil
}

func autoEquipmentCleanupInterval(seconds int) time.Duration {
	if seconds < minAutoEquipmentCleanupInterval {
		seconds = minAutoEquipmentCleanupInterval
	}
	return policyInterval(seconds, autoEquipmentCleanupDefaultInterval)
}
