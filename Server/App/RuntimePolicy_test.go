package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Session"
)

func TestRuntimeSchedulerSettingsDriveHarnessPolicies(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"scheduler":         json.RawMessage(`{"minAttackDelay":4.5,"maxAttackDelay":4.5,"botLocked":true,"attackPriorities":{"autoTowers":82}}`),
		"session.reconnect": json.RawMessage(`{"relogDelaySec":420}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Configuration: configuration}
	if delay := application.attackLaunchDelay(); delay != 4500*time.Millisecond {
		t.Fatalf("attack delay = %s", delay)
	}
	if !application.automationLocked() {
		t.Fatal("bot lock setting was not applied")
	}
	if delay := application.relogDelay(); delay != 7*time.Minute {
		t.Fatalf("relog delay = %s", delay)
	}
	if weight := application.attackAdmissionWeight(Intent.Request{}, Intent.Admission{Module: "autoTowers"}); weight != 82 {
		t.Fatalf("Auto Towers admission weight = %d", weight)
	}
	if weight := application.attackAdmissionWeight(Intent.Request{}, Intent.Admission{Module: "futureModule"}); weight != 50 {
		t.Fatalf("default admission weight = %d", weight)
	}
}

func TestRuntimeRelogDelayDefaultsAndClamps(t *testing.T) {
	if delay := (&Application{}).relogDelay(); delay != 5*time.Minute {
		t.Fatalf("default relog delay = %s", delay)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"session.reconnect": json.RawMessage(`{"relogDelaySec":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if delay := (&Application{Configuration: configuration}).relogDelay(); delay != time.Minute {
		t.Fatalf("minimum relog delay = %s", delay)
	}
}

func TestExecutionGateYieldsAutomatedWorkWhileBotIsLocked(t *testing.T) {
	controller := Session.NewController(context.Background(), Session.NewUnavailableTransport(), nil, nil)
	controller.SetAutomationLocked(true)
	application := &Application{Session: controller}
	plan := Intent.Plan{Claims: []string{"game:equipment"}}

	start := time.Now()
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("manual UI intent was delayed by %s", elapsed)
	}

	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoRecruit"}, plan, Intent.ExecutionBeforeStep,
	); !errors.Is(err, Outbound.ErrAutomationLocked) {
		t.Fatalf("active automation gate error = %v", err)
	}

	start = time.Now()
	go func() {
		time.Sleep(35 * time.Millisecond)
		controller.SetAutomationLocked(false)
	}()
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoRecruit"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("background automation ignored bot lock after %s", elapsed)
	}
}

func TestExecutionGateFailsWritesClosedWhenPersistenceIsUnavailable(t *testing.T) {
	application := &Application{statePersistenceErr: errors.New("disk full")}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, Intent.Plan{Effect: Intent.EffectWrite},
		Intent.ExecutionBeforeClaims,
	); err == nil || !strings.Contains(err.Error(), "durable storage is unavailable") {
		t.Fatalf("write gate error = %v", err)
	}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "ui"}, Intent.Plan{Effect: Intent.EffectRead},
		Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatalf("read was blocked by persistence health: %v", err)
	}
}

func TestExecutionGateRequiresExplicitCommanderAssignmentsForAttackModules(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		commanderFeatureSection: json.RawMessage(`{"version":1,"assignments":{"autoStorm":[16,17]}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Configuration: configuration}
	plan := Intent.Plan{
		Effect:    Intent.EffectLaunch,
		Claims:    []string{"commander:16", "leader:commander:16"},
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoStorm"},
	}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoStorm"}, plan, Intent.ExecutionBeforeClaims,
	); err != nil {
		t.Fatalf("assigned commander was blocked: %v", err)
	}

	plan.Claims = []string{"commander:0", "leader:commander:0"}
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoStorm"}, plan, Intent.ExecutionBeforeClaims,
	); err == nil || !strings.Contains(err.Error(), "commander 0 is not assigned to autoStorm") {
		t.Fatalf("unassigned commander gate error = %v", err)
	}

	plan.Admission.Module = "autoTowers"
	if err := application.executionGate(
		context.Background(), Intent.Request{Actor: "automation:autoTowers"}, plan, Intent.ExecutionBeforeClaims,
	); err == nil || !strings.Contains(err.Error(), "assign at least one commander to autoTowers") {
		t.Fatalf("missing feature assignment gate error = %v", err)
	}
}
