package App

import (
	"context"
	"testing"

	"CitadelDesktop/Server/Session"
)

func TestLegacy138IntentAndAutomationParityManifest(t *testing.T) {
	application, err := New(context.Background(), Config{
		DataDir: t.TempDir(), Offline: true, Transport: Session.NewUnavailableTransport(),
	})
	if err != nil {
		t.Fatal(err)
	}
	capabilities := map[string][]string{
		"application": {"app.update.check", "app.update.install", "game_data.refresh"},
		"session":     {"session.start", "session.stop", "session.select_browser"},
		"castle":      {"game.focus_castle", "decoration.apply_preset"},
		"movement":    {"game.refresh_movements", "movement.recall", "troops.station", "spy.launch"},
		"alliance":    {"alliance.refresh", "alliance.inspect", "alliance.help.request"},
		"equipment": {
			"equipment.refresh", "equipment.equip", "equipment.unequip", "equipment.gem.equip",
			"equipment.gem.unequip", "equipment.swap", "equipment.reconfigure", "equipment.upgrade", "equipment.sell",
		},
		"production":   {"production.enqueue", "hospital.heal", "hospital.discard"},
		"construction": {"construction.equip", "construction.upgrade", "construction.shop", "construction.inventory.refresh", "construction.purchase"},
		"crafting":     {"crafting.refresh", "crafting.start", "crafting.rent_slot", "crafting.skip", "resource.logistics.refresh", "resource.ship", "resource.market.ship", "resource.kingdom.ship", "resource.kingdom.skip", "resource.kingdom.settle"},
		"rift":         {"rift.maiden_wave.launch", "rift.launch.replay", "rift.template.rename", "rift.template.delete"},
		"towers":       {"tower.queue.scan", "tower.context.refresh", "tower.attack", "tower.launch"},
		"reports":      {"report.spy.fetch", "report.spy.share", "report.battle.summary", "report.battle.details"},
		"beri":         {"beri.capacity.refresh", "beri.transfer", "beri.camp.open", "beri.target.find", "beri.tower.attack"},
	}
	for capability, names := range capabilities {
		for _, name := range names {
			if _, found := application.Intents.Registry().Definition(name); !found {
				t.Errorf("1.3.8 %s capability has no 2.0 intent %q", capability, name)
			}
		}
	}

	policyIDs := map[string]bool{}
	for _, id := range application.Automation.PolicyIDs() {
		policyIDs[id] = true
	}
	for _, id := range []string{
		"autoRecruit", "autoTool", "autoHospital", "autoTCI", "autoSceatRes", "autoSceatResLogistics",
		"autoBird", "autoStation", "autoBeriWorld", "autoFoodBalance", "autoTowers",
	} {
		if !policyIDs[id] {
			t.Errorf("1.3.8 automation %q has no 2.0 policy", id)
		}
	}
}
