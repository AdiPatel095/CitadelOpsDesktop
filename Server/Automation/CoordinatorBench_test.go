package Automation

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func allBenchmarkPolicies() []Policy {
	return []Policy{
		NewRecruitPolicy(), NewToolPolicy(), NewHospitalPolicy(), NewAllianceHelpPolicy(), NewAutoEquipmentCleanupPolicy(),
		NewDailyAttackRefreshPolicy(), NewConstructionPolicy(), NewCraftingPolicy(), NewCraftingLogisticsPolicy(),
		NewAutoBirdPolicy(), NewAutoStationPolicy(), NewBeriPolicy(), NewBeriToolPolicy(), NewBeriBuildPolicy(),
		NewBeriAttackPolicy(), NewFoodBalancePolicy(), NewAutoTowerPolicy(), NewInvasionRecoveryPolicy(),
		NewAutoFortressPolicy(), NewAutoInvasionPolicy(), NewAutoNomadPolicy(), NewAutoAdvisorPolicy(),
		NewAutoBoosterPolicy(), NewAutoBuyerPolicy(), NewRiftMaidenRunPolicy(), NewAutoKhanPolicy(),
		NewAutoKhanCooldownPolicy(), NewAutoKhanRagePolicy(), NewAutoKhanDefensePolicy(), NewAutoStormPolicy(),
		NewAutoStormShopPolicy(), NewAutoStormBuildPolicy(),
	}
}

// BenchmarkEvaluateAllPolicies is one coordinator evaluation pass over every
// policy (mostly disabled, as on a typical hosted account) with a realistic
// configuration: enabled controls for the whole catalogue and a weekly schedule.
func BenchmarkEvaluateAllPolicies(b *testing.B) {
	controls := map[string]bool{}
	for _, policy := range allBenchmarkPolicies() {
		controls[policy.EnabledKey()] = false
	}
	for _, key := range []string{"auto_tower", "auto_khan", "auto_nomad", "auto_bird"} {
		controls[key] = true
	}
	enabled, _ := json.Marshal(controls)
	schedule := json.RawMessage(`{"featureSchedules":{"autoTowers":{"enabled":true,"timeZone":"UTC","slots":[{"day":0,"startMinute":0,"endMinute":1440},{"day":1,"startMinute":0,"endMinute":1440},{"day":2,"startMinute":0,"endMinute":1440},{"day":3,"startMinute":0,"endMinute":1440},{"day":4,"startMinute":0,"endMinute":1440},{"day":5,"startMinute":0,"endMinute":1440},{"day":6,"startMinute":0,"endMinute":1440}]}}}`)
	configuration, err := Configuration.Open(b.TempDir(), map[string]json.RawMessage{"automation.enabled": enabled, "scheduler": schedule})
	if err != nil {
		b.Fatal(err)
	}
	state := State.NewStore(coordinatorReadyState())
	policies := allBenchmarkPolicies()
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}, policies...)
	runtime := make(map[string]*policyRuntime, len(policies))
	for _, policy := range policies {
		runtime[policy.ID()] = &policyRuntime{}
	}
	results := make(chan operationResult, len(policies)*2+1)
	coordinator.evaluate(b.Context(), runtime, results)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for _, current := range runtime {
			current.evaluationPending = true
		}
		coordinator.evaluate(b.Context(), runtime, results)
	}
}
