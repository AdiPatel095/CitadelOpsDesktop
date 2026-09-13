package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

func TestAutoBuyerFeastIsolatedFromInvalidAndStaleOtherGoals(t *testing.T) {
	for _, tc := range []struct {
		name, packages, specialists string
		invalid                     bool
	}{
		{"removed package", `[{"enabled":true,"shopId":"master-blacksmith","packageId":999,"targetPurchasesPerReset":1}]`, `[]`, true},
		{"lowered stock", `[{"enabled":true,"shopId":"master-blacksmith","packageId":100,"targetPurchasesPerReset":999}]`, `[]`, true},
		{"invalid specialist", `[]`, `[{"enabled":true,"id":999,"minimumDays":14}]`, true},
		{"valid specialist needs refresh", `[]`, `[{"enabled":true,"id":0,"minimumDays":14,"maximumRubyCostPerPurchase":100000}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			state := autoBuyerPolicyTestState(now)
			castle := state.Castles[10]
			castle.Resources[5] = State.ResourceBalance{Amount: 200000}
			state.Castles[10] = castle
			state.Market.BoostersObservedAt = time.Time{}
			settings := json.RawMessage(`{"version":1,"sourceCastleId":10,"packages":` + tc.packages + `,"specialists":` + tc.specialists + `,"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000}}`)
			before := string(settings)
			d, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{State: state, GameData: autoBuyerPolicyTestStore(t), Now: now, Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}}})
			if err != nil || d.Request == nil || d.Request.Name != "autoBuyer.feast.purchase" {
				t.Fatalf("feast blocked: %+v %v", d, err)
			}
			if tc.invalid && (d.Metrics["invalidGoals"] == 0 || !strings.Contains(d.Detail, "skipped invalid goal")) {
				t.Fatalf("invalid goal hidden: %+v", d)
			}
			if string(settings) != before {
				t.Fatal("saved goals mutated")
			}
		})
	}
}

func TestAutoBuyerIsolationRejectsEveryDuplicateWithoutMutatingSettings(t *testing.T) {
	rule := autoBuyerPackageRule{Enabled: true, ShopID: "master-blacksmith", PackageID: 100, TargetPurchasesPerReset: 1}
	settings := autoBuyerSettings{Packages: []autoBuyerPackageRule{rule, rule}}
	metrics := map[string]float64{}
	filtered, detail := isolateAutoBuyerRules(autoBuyerPolicyTestStore(t), settings, metrics)
	if detail == "" || metrics["invalidGoals"] != 2 || filtered.Packages[0].Enabled || filtered.Packages[1].Enabled {
		t.Fatalf("duplicate goals admitted: %+v", filtered)
	}
	if !settings.Packages[0].Enabled || !settings.Packages[1].Enabled {
		t.Fatal("saved goals were disabled")
	}
}

func TestAutoBuyerPendingFeastPollsWithoutBuyingOrTightLoop(t *testing.T) {
	now := time.Now().UTC()
	state := autoBuyerPolicyTestState(now)
	state.Market.FeastPurchasePending = true
	state.Market.FeastPurchasePendingSince = now.Add(-time.Minute)
	config := Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: json.RawMessage(`{"version":1,"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10}}`)}}
	evaluate := func() Decision {
		d, e := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{State: state, GameData: autoBuyerPolicyTestStore(t), Now: now, Configuration: config})
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	d := evaluate()
	if d.Request == nil || d.Request.Name != "autoBuyer.boosters.refresh" || d.ReevaluateOnSuccess || !d.NextCheckAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("unsafe pending decision: %+v", d)
	}
	last := now.Add(-time.Second)
	state.Automations["autoBuyer"] = State.AutomationState{LastRunAt: &last}
	d = evaluate()
	if d.Request != nil || !d.NextCheckAt.Equal(last.Add(30*time.Second)) {
		t.Fatalf("pending polling loop: %+v", d)
	}
}
