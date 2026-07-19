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
		"scheduler": json.RawMessage(`{"minAttackDelay":4.5,"maxAttackDelay":4.5,"botLocked":true,"attackPriorities":{"autoTowers":82}}`),
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
	if weight := application.attackAdmissionWeight(Intent.Request{}, Intent.Admission{Module: "autoTowers"}); weight != 82 {
		t.Fatalf("Auto Towers admission weight = %d", weight)
	}
	if weight := application.attackAdmissionWeight(Intent.Request{}, Intent.Admission{Module: "futureModule"}); weight != 50 {
		t.Fatalf("default admission weight = %d", weight)
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
