package Automation

import (
	"CitadelDesktop/Server/Localization"
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
			Status: "ready", Detail: "Refresh equipment and gem storage before automatic cleanup", DetailDescriptor: Localization.New("server.automation.refresh_equipment_and_gem.1c84e3c0", "Refresh equipment and gem storage before automatic cleanup", nil),
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
			Status: "ready", Detail: fmt.Sprintf("Sell %d eligible non-relic equipment item(s)", counts.Equipment), DetailDescriptor: Localization.New("server.automation.sell_p_eligible_non.3d6653dd", "Sell {p0} eligible non-relic equipment item(s)", Localization.Params{"p0": counts.Equipment}),
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
			Status: "ready", Detail: fmt.Sprintf("Sell %d eligible non-relic gem(s)", counts.Gems), DetailDescriptor: Localization.New("server.automation.sell_p_eligible_non.99deaea8", "Sell {p0} eligible non-relic gem(s)", Localization.Params{"p0": counts.Gems}),
			NextCheckAt: nextCheck, Metrics: metrics,
			Request:             &Intent.Request{Name: "equipment.sell", Arguments: arguments},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	return Decision{
		Status: "idle", Detail: "Equipment and gem storage are clean", DetailDescriptor: Localization.New("server.automation.equipment_and_gem_storage.866351ca", "Equipment and gem storage are clean", nil),
		NextCheckAt: nextCheck, Metrics: metrics,
	}, nil
}

func autoEquipmentCleanupInterval(seconds int) time.Duration {
	if seconds < minAutoEquipmentCleanupInterval {
		seconds = minAutoEquipmentCleanupInterval
	}
	return policyInterval(seconds, autoEquipmentCleanupDefaultInterval)
}
