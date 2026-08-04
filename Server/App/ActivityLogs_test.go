package App

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/Intent"
)

func TestFeatureActivitiesRecordsOneCompletedAction(t *testing.T) {
	receipt := Intent.Receipt{
		Intent: "production.enqueue", Status: Intent.StatusSucceeded,
		Plan: &Intent.Plan{
			Effect: Intent.EffectWrite, Summary: "Queue 5 stacks of 40 tools definition 614 at DesertTown",
			Steps: []Intent.Step{{Opcode: "bup"}, {Opcode: "bup"}, {Opcode: "bup"}},
		},
	}
	activities := featureActivities(receipt)
	if len(activities) != 1 {
		t.Fatalf("activities = %#v, want one queue entry", activities)
	}
	if activities[0].severity != "INFO" || activities[0].event != "QUEUE" ||
		activities[0].detail != "Queued 5 stacks of 40 tools definition 614 at DesertTown" {
		t.Fatalf("activity = %#v", activities[0])
	}
}

func TestFeatureActivitiesRecordsEachAttackInChain(t *testing.T) {
	receipt := Intent.Receipt{
		Intent: "nomad.camp.attack", Status: Intent.StatusSucceeded,
		Plan: &Intent.Plan{
			Effect: Intent.EffectLaunch, Summary: "Chain 2 attacks into locked camp 1166:1165",
			Steps: []Intent.Step{
				{Name: "Build and launch camp attack with commander 4", CommandDependencies: &Intent.CommandDependencyRequest{Opcode: "cra"}},
				{Name: "Capture first launch", Action: "nomad.attack.capture"},
				{Name: "Build and launch camp attack with commander 7", CommandDependencies: &Intent.CommandDependencyRequest{Opcode: "cra"}},
			},
		},
	}
	activities := featureActivities(receipt)
	if len(activities) != 2 {
		t.Fatalf("activities = %#v, want one entry per launched attack", activities)
	}
	if activities[0].detail != "Launched camp attack with commander 4 (1 of 2)" ||
		activities[1].detail != "Launched camp attack with commander 7 (2 of 2)" {
		t.Fatalf("attack activities = %#v", activities)
	}
}

func TestFeatureActivitiesSkipsLifecycleAndSupportWork(t *testing.T) {
	for _, receipt := range []Intent.Receipt{
		{Intent: "tower.attack", Status: Intent.StatusRunning},
		{
			Intent: "tower.queue.scan", Status: Intent.StatusSucceeded,
			Plan: &Intent.Plan{Effect: Intent.EffectRead, Summary: "Refresh complete tower map", Steps: []Intent.Step{{Opcode: "gaa"}}},
		},
		{
			Intent: "tower.attack", Status: Intent.StatusSucceeded,
			Plan: &Intent.Plan{Effect: Intent.EffectLaunch, Summary: "Skip tower attack: no commander is available"},
		},
		{
			Intent: "config.update", Status: Intent.StatusSucceeded,
			Plan: &Intent.Plan{Effect: Intent.EffectWrite, Summary: "Update crafting configuration", Steps: []Intent.Step{{Action: "config.update"}}},
		},
		{
			Intent: "nomad.target.lock", Status: Intent.StatusSucceeded,
			Plan: &Intent.Plan{Effect: Intent.EffectWrite, Summary: "Lock weakest camp", Steps: []Intent.Step{{Action: "nomad.target.lock"}}},
		},
	} {
		if activities := featureActivities(receipt); len(activities) != 0 {
			t.Errorf("featureActivities(%s, %s) = %#v, want no activity", receipt.Intent, receipt.Status, activities)
		}
	}
}

func TestFeatureActivitiesRecordsOneUserFacingFailure(t *testing.T) {
	receipt := Intent.Receipt{
		Intent: "construction.purchase", Status: Intent.StatusFailed, Error: "the shop offer expired",
		Plan: &Intent.Plan{
			Effect: Intent.EffectWrite, Summary: "Buy construction-item package 2498 from Baltimore",
			Steps: []Intent.Step{{Opcode: "sbp"}},
		},
	}
	activities := featureActivities(receipt)
	if len(activities) != 1 || activities[0].severity != "ERROR" || activities[0].event != "PURCHASE" {
		t.Fatalf("activities = %#v", activities)
	}
	if strings.Contains(activities[0].detail, "intent=") || strings.Contains(activities[0].detail, "operation=") ||
		activities[0].detail != "Could not buy construction-item package 2498 from Baltimore: the shop offer expired" {
		t.Fatalf("failure detail = %q", activities[0].detail)
	}
}

func TestFeatureActivitiesMarksAttackInventoryGateAsWarning(t *testing.T) {
	receipt := Intent.Receipt{
		Intent: "storm.attack", Status: Intent.StatusFailed,
		Error: "Build and launch capacity-adjusted Storm attack: build Storm preset \"Trial\": castle 3849 has 277 of item 215; 1 commander(s) require 400",
		Plan: &Intent.Plan{
			Effect: Intent.EffectLaunch, Summary: "Attack Storm fort at 600:555 with Trial",
			Steps: []Intent.Step{{CommandDependencies: &Intent.CommandDependencyRequest{Opcode: "cra"}}},
		},
	}
	activities := featureActivities(receipt)
	if len(activities) != 1 || activities[0].severity != "WARN" || activities[0].event != "ATTACK" {
		t.Fatalf("inventory gate activities = %#v", activities)
	}
	if activities[0].detail != "Could not launch an attack against Storm fort at 600:555 with Trial: Build and launch capacity-adjusted Storm attack: build Storm preset \"Trial\": castle 3849 has 277 of item 215; 1 commander(s) require 400" {
		t.Fatalf("inventory gate detail = %q", activities[0].detail)
	}
}

func TestFeatureActivitiesHidesResponseDiagnosticsFromFailure(t *testing.T) {
	receipt := Intent.Receipt{
		Intent: "storm.shop.purchase", Status: Intent.StatusFailed,
		Error: "Purchase Luna trade-boat package: response code 53 for SBP was not successful: The shop offer expired. (official game text)",
		Plan: &Intent.Plan{
			Effect: Intent.EffectWrite, Summary: "Buy 2 x War horn from Luna for 5920 Aquamarine at Storm Castle",
			Steps: []Intent.Step{{Opcode: "sbp"}},
		},
	}
	activities := featureActivities(receipt)
	if len(activities) != 1 {
		t.Fatalf("activities = %#v", activities)
	}
	if strings.Contains(activities[0].detail, "SBP") || strings.Contains(activities[0].detail, "response code") ||
		activities[0].detail != "Could not buy 2 x War horn from Luna for 5920 Aquamarine at Storm Castle: The shop offer expired." {
		t.Fatalf("failure detail = %q", activities[0].detail)
	}
}

func TestUserFacingFailureReasonAlwaysExplainsFailure(t *testing.T) {
	if reason := userFacingFailureReason(""); reason != "the action did not complete" {
		t.Fatalf("empty failure reason = %q", reason)
	}
	if reason := userFacingFailureReason("Build and launch tower attack: timed out waiting for cra"); reason != "the game did not confirm the action in time" {
		t.Fatalf("timeout failure reason = %q", reason)
	}
}

func TestUserFacingGameNameHumanizesCatalogIdentifiers(t *testing.T) {
	tests := map[string]string{
		"elvenKeep":  "Elven Keep",
		"MeadMace":   "Mead Mace",
		"storm_tool": "Storm tool",
		"MEAD":       "MEAD",
	}
	for input, expected := range tests {
		if actual := userFacingGameName(input); actual != expected {
			t.Errorf("userFacingGameName(%q) = %q, want %q", input, actual, expected)
		}
	}
}
