package Automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestAutomationEnabledControlsSupportBooleanAndTimedValues(t *testing.T) {
	now := time.Date(2026, 7, 20, 16, 0, 0, 0, time.UTC)
	snapshot := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		automationEnabledSection: json.RawMessage(`{
			"always":true,
			"off":false,
			"active":{"enabled":true,"expiresAt":"2026-07-20T17:00:00Z"},
			"expired":{"enabled":true,"expiresAt":"2026-07-20T15:00:00Z"}
		}`),
	}}
	enabled := enabledFeatures(snapshot, now)
	if !enabled["always"] || enabled["off"] || !enabled["active"] || enabled["expired"] {
		t.Fatalf("unexpected effective enabled controls: %#v", enabled)
	}
	if configured := configuredEnabledFeatures(snapshot); !configured["active"] || !configured["expired"] {
		t.Fatalf("timed controls lost their configured enabled state: %#v", configured)
	}
	if next := nextAutomationExpiration(snapshot, now); !next.Equal(now) {
		t.Fatalf("expired control should request immediate normalization, got %s", next)
	}
	value, changed := expiredAutomationEnabledDocument(snapshot, now)
	if !changed {
		t.Fatal("expired timed control was not normalized")
	}
	var normalized map[string]json.RawMessage
	if err := json.Unmarshal(value, &normalized); err != nil {
		t.Fatal(err)
	}
	if string(normalized["expired"]) != "false" || string(normalized["active"]) == "false" {
		t.Fatalf("unexpected normalized document: %s", value)
	}
	normalizedSnapshot := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		automationEnabledSection: value,
	}}
	if next := nextAutomationExpiration(normalizedSnapshot, now); !next.Equal(now.Add(time.Hour)) {
		t.Fatalf("next active automation expiration = %s", next)
	}
}

func TestCoordinatorTimedEnableExpiresAndCancelsRunningIntent(t *testing.T) {
	expiresAt := time.Now().UTC().Add(250 * time.Millisecond)
	enabled, err := json.Marshal(map[string]any{
		"timed": map[string]any{"enabled": true, "expiresAt": expiresAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		automationEnabledSection: enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewStore(coordinatorReadyState())
	policy := &coordinatorTestPolicy{id: "timed", decision: Decision{
		Status: "ready", Detail: "Run until the timed enable expires", NextCheckAt: time.Now().UTC().Add(time.Hour),
		Request: &Intent.Request{Name: "test.block"},
	}}
	submitter := &coordinatorTestBlockingSubmitter{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		t.Fatal("timed automation did not start")
	}
	select {
	case <-submitter.canceled:
	case <-time.After(time.Second):
		t.Fatal("timed automation was not cancelled at expiration")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current := enabledFeatures(configuration.Snapshot(), time.Now().UTC())
		if !current["timed"] && state.Snapshot().Automations["timed"].Status == "disabled" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf(
		"timed automation did not settle disabled: config=%s state=%+v",
		configuration.Snapshot().Sections[automationEnabledSection], state.Snapshot().Automations["timed"],
	)
}

func TestHostedCoordinatorTimedEnableExpiresWithoutPersistingAccountConfiguration(t *testing.T) {
	expiresAt := time.Now().UTC().Add(250 * time.Millisecond)
	enabled, err := json.Marshal(map[string]any{
		"timed": map[string]any{"enabled": true, "expiresAt": expiresAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		automationEnabledSection: enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := configuration.Snapshot()
	state := State.NewStore(coordinatorReadyState())
	policy := &coordinatorTestPolicy{id: "timed", decision: Decision{
		Status: "ready", Detail: "Run until the timed enable expires", NextCheckAt: time.Now().UTC().Add(time.Hour),
		Request: &Intent.Request{Name: "test.block"},
	}}
	submitter := &coordinatorTestBlockingSubmitter{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	coordinator.SetExternalConfigurationAuthority(true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		t.Fatal("hosted timed automation did not start")
	}
	select {
	case <-submitter.canceled:
	case <-time.After(time.Second):
		t.Fatal("hosted timed automation was not cancelled at expiration")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if state.Snapshot().Automations["timed"].Status == "disabled" {
			after := configuration.Snapshot()
			if after.Revision != before.Revision ||
				string(after.Sections[automationEnabledSection]) != string(before.Sections[automationEnabledSection]) {
				t.Fatalf("hosted timed expiry mutated account configuration: before=%s after=%s", before.Sections[automationEnabledSection], after.Sections[automationEnabledSection])
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("hosted timed automation did not settle disabled: %+v", state.Snapshot().Automations["timed"])
}
