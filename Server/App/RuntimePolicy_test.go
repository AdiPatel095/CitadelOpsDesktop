package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Session"
)

func TestRuntimeSchedulerSettingsDriveHarnessPolicies(t *testing.T) {
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{"minAttackDelay":4.5,"maxAttackDelay":4.5,"manualFocusIdleSec":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{Configuration: configuration}
	if delay := application.attackLaunchDelay(); delay != 4500*time.Millisecond {
		t.Fatalf("attack delay = %s", delay)
	}
	if hold := application.manualFocusHold(); hold != 5*time.Second {
		t.Fatalf("manual focus hold = %s", hold)
	}
}

func TestExecutionGateOnlyPausesFocusAutomation(t *testing.T) {
	controller := Session.NewController(context.Background(), Session.NewUnavailableTransport(), nil, nil)
	controller.SetManualFocusHoldProvider(func() time.Duration { return 35 * time.Millisecond })
	controller.RecordManualActivity(Session.Activity{Kind: "pointerdown", ObservedAt: time.Now()})
	application := &Application{Session: controller}
	plan := Intent.Plan{Claims: []string{"castle-focus"}}

	start := time.Now()
	if err := application.executionGate(context.Background(), Intent.Request{Actor: "ui"}, plan); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Millisecond {
		t.Fatalf("manual UI intent was delayed by %s", elapsed)
	}

	start = time.Now()
	if err := application.executionGate(context.Background(), Intent.Request{Actor: "automation:autoRecruit"}, plan); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("focus automation ignored manual hold after %s", elapsed)
	}
}
