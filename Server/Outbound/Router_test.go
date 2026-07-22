package Outbound

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

func TestRouterPrioritizesQueuedCommandsWithinEachLane(t *testing.T) {
	tests := []struct {
		name   string
		opcode string
		lane   Lane
	}{
		{name: "command", opcode: "ain", lane: LaneCommand},
		{name: "attack", opcode: "cra", lane: LaneAttackLaunch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testRouterLanePriority(t, test.opcode, test.lane)
		})
	}
}

func testRouterLanePriority(t *testing.T, opcode string, lane Lane) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var order []string
	router := NewRouter(root, Config{
		Ready: func() bool { return true },
		Send: func(_ context.Context, payload []byte) error {
			label := outboundTestLabel(t, payload)
			mu.Lock()
			order = append(order, label)
			mu.Unlock()
			if label == "block" {
				close(started)
				<-release
			}
			return nil
		},
	})
	defer router.Close()
	results := make(chan error, 3)
	go func() {
		results <- router.Send(WithMetadata(context.Background(), Metadata{Priority: PriorityInteractive}), outboundTestPayload(t, opcode, "block"))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first command did not enter the transport")
	}
	go func() {
		results <- router.Send(WithMetadata(context.Background(), Metadata{Priority: PriorityBackground}), outboundTestPayload(t, opcode, "low"))
	}()
	waitForOutboundQueue(t, router, lane, 1)
	go func() {
		results <- router.Send(WithMetadata(context.Background(), Metadata{Priority: PriorityInteractive}), outboundTestPayload(t, opcode, "high"))
	}()
	waitForOutboundQueue(t, router, lane, 2)
	close(release)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(order, []string{"block", "high", "low"}) {
		t.Fatalf("send order = %#v", order)
	}
}

func TestRouterSerializesPhysicalSendsAcrossLanes(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	active := 0
	maximumActive := 0
	router := NewRouter(root, Config{
		Ready: func() bool { return true },
		Send: func(context.Context, []byte) error {
			mu.Lock()
			active++
			if active > maximumActive {
				maximumActive = active
			}
			mu.Unlock()
			time.Sleep(15 * time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			return nil
		},
	})
	defer router.Close()
	results := make(chan error, 2)
	go func() { results <- router.Send(context.Background(), outboundTestPayload(t, "ain", "command")) }()
	go func() { results <- router.Send(context.Background(), outboundTestPayload(t, "cra", "attack")) }()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if maximumActive != 1 {
		t.Fatalf("maximum concurrent physical sends = %d", maximumActive)
	}
}

func TestRouterComparesPriorityAcrossReadyLanes(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var order []string
	router := NewRouter(root, Config{
		Ready: func() bool { return true },
		Send: func(_ context.Context, payload []byte) error {
			label := outboundTestLabel(t, payload)
			mu.Lock()
			order = append(order, label)
			mu.Unlock()
			if label == "block" {
				close(started)
				<-release
			}
			return nil
		},
	})
	defer router.Close()
	results := make(chan error, 3)
	go func() { results <- router.Send(context.Background(), outboundTestPayload(t, "ain", "block")) }()
	<-started
	go func() {
		results <- router.Send(WithMetadata(context.Background(), Metadata{Priority: PriorityBackground}), outboundTestPayload(t, "cra", "low-attack"))
	}()
	waitForOutboundQueue(t, router, LaneAttackLaunch, 1)
	go func() {
		results <- router.Send(WithMetadata(context.Background(), Metadata{Priority: PriorityInteractive}), outboundTestPayload(t, "ain", "high-command"))
	}()
	waitForOutboundQueue(t, router, LaneCommand, 1)
	close(release)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(order, []string{"block", "high-command", "low-attack"}) {
		t.Fatalf("send order = %#v", order)
	}
}

func TestQueuedCommandOrderingUsesDeadlineThenFIFO(t *testing.T) {
	now := time.Now()
	early := &queuedCommand{id: 3, enqueuedAt: now, metadata: Metadata{Priority: PriorityService, Deadline: now.Add(time.Minute)}}
	late := &queuedCommand{id: 1, enqueuedAt: now, metadata: Metadata{Priority: PriorityService, Deadline: now.Add(2 * time.Minute)}}
	if !queuedCommandBefore(early, late) {
		t.Fatal("earlier deadline did not win equal priority")
	}
	first := &queuedCommand{id: 1, enqueuedAt: now, metadata: Metadata{Priority: PriorityService}}
	second := &queuedCommand{id: 2, enqueuedAt: now, metadata: Metadata{Priority: PriorityService}}
	if !queuedCommandBefore(first, second) {
		t.Fatal("FIFO did not break equal priority and deadline")
	}
}

func TestQueuedCommandOrderingAgesOldWorkToPreventStarvation(t *testing.T) {
	now := time.Now()
	oldBackground := &queuedCommand{
		id: 1, enqueuedAt: now.Add(-2 * time.Minute), metadata: Metadata{Priority: PriorityBackground},
	}
	newInteractive := &queuedCommand{
		id: 2, enqueuedAt: now, metadata: Metadata{Priority: PriorityInteractive},
	}
	if !queuedCommandBeforeAt(oldBackground, newInteractive, now) {
		t.Fatal("aged background command did not eventually win the FIFO tie")
	}
}

func TestRouterKeepsAttackPacingIndependentFromCommandLane(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var order []string
	var sentAt = map[string]time.Time{}
	router := NewRouter(root, Config{
		Ready:       func() bool { return true },
		AttackDelay: func() time.Duration { return 70 * time.Millisecond },
		Send: func(_ context.Context, payload []byte) error {
			label := outboundTestLabel(t, payload)
			mu.Lock()
			order = append(order, label)
			sentAt[label] = time.Now()
			mu.Unlock()
			return nil
		},
	})
	defer router.Close()
	if err := router.Send(context.Background(), outboundTestPayload(t, "cra", "attack-1")); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() {
		results <- router.Send(context.Background(), outboundTestPayload(t, "cra", "attack-2"))
	}()
	waitForOutboundQueue(t, router, LaneAttackLaunch, 1)
	go func() {
		results <- router.Send(context.Background(), outboundTestPayload(t, "ain", "command"))
	}()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(order, []string{"attack-1", "command", "attack-2"}) {
		t.Fatalf("send order = %#v", order)
	}
	if delay := sentAt["attack-2"].Sub(sentAt["attack-1"]); delay < 60*time.Millisecond {
		t.Fatalf("attack delay = %s", delay)
	}
}

func TestRouterExposesActualNextAttackAvailability(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	router := NewRouter(root, Config{
		Ready:       func() bool { return true },
		AttackDelay: func() time.Duration { return 80 * time.Millisecond },
		Send:        func(context.Context, []byte) error { return nil },
	})
	defer router.Close()
	if err := router.Send(t.Context(), outboundTestPayload(t, "cra", "attack")); err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(router.NextAllowed(LaneAttackLaunch))
	if remaining < 60*time.Millisecond || remaining > 100*time.Millisecond {
		t.Fatalf("next attack availability is %s away", remaining)
	}
	if next := router.NextAllowed(LaneCommand); !next.IsZero() {
		t.Fatalf("command lane unexpectedly paced until %s", next)
	}
}

func TestRouterAppliesAttackDelayToEveryCRA(t *testing.T) {
	root, cancel := context.WithCancel(context.Background())
	defer cancel()
	router := NewRouter(root, Config{
		Ready:       func() bool { return true },
		AttackDelay: func() time.Duration { return time.Hour },
		Send:        func(context.Context, []byte) error { return nil },
	})
	defer router.Close()
	if err := router.Send(t.Context(), outboundTestPayload(t, "cra", "chain-first")); err != nil {
		t.Fatal(err)
	}
	nextContext, cancelNext := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancelNext()
	if err := router.Send(nextContext, outboundTestPayload(t, "cra", "chain-next")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("later CRA bypassed global attack pacing: %v", err)
	}
}

func TestRouterRemovesCancelledQueuedCommand(t *testing.T) {
	root, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	var order []string
	router := NewRouter(root, Config{
		Ready: func() bool { return true },
		Send: func(_ context.Context, payload []byte) error {
			label := outboundTestLabel(t, payload)
			mu.Lock()
			order = append(order, label)
			mu.Unlock()
			if label == "block" {
				close(started)
				<-release
			}
			return nil
		},
	})
	defer router.Close()
	first := make(chan error, 1)
	go func() { first <- router.Send(context.Background(), outboundTestPayload(t, "ain", "block")) }()
	<-started
	queuedContext, cancelQueued := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() { second <- router.Send(queuedContext, outboundTestPayload(t, "ain", "cancelled")) }()
	waitForOutboundQueue(t, router, LaneCommand, 1)
	cancelQueued()
	if err := <-second; err != context.Canceled {
		t.Fatalf("cancelled send error = %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(order, []string{"block"}) {
		t.Fatalf("send order = %#v", order)
	}
}

func TestRouterReportsPhysicalResultAfterInFlightCallerCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	router := NewRouter(t.Context(), Config{
		Ready: func() bool { return true },
		Send: func(context.Context, []byte) error {
			close(started)
			<-release
			return nil
		},
	})
	defer router.Close()
	sendContext, cancelSend := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- router.Send(sendContext, outboundTestPayload(t, "ain", "in-flight"))
	}()
	<-started
	cancelSend()
	select {
	case err := <-result:
		t.Fatalf("in-flight send returned before its physical outcome: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatalf("physical send result = %v", err)
	}
}

func TestRouterMeasuresFastPathIntentLatency(t *testing.T) {
	router := NewRouter(t.Context(), Config{
		Ready: func() bool { return true },
		Send: func(context.Context, []byte) error {
			time.Sleep(time.Millisecond)
			return nil
		},
	})
	defer router.Close()
	submittedAt := time.Now().Add(-time.Millisecond)
	ctx := WithMetadata(t.Context(), Metadata{
		OperationID: "latency-test", SubmittedAt: submittedAt, Priority: PriorityInteractive,
	})
	if err := router.Send(ctx, outboundTestPayload(t, "ain", "latency")); err != nil {
		t.Fatal(err)
	}

	stats := router.DispatchLatency()
	if stats.TargetMilliseconds != 25 {
		t.Fatalf("latency target = %.3fms, want 25ms", stats.TargetMilliseconds)
	}
	if stats.All.Count != 1 || stats.FastPath.Count != 1 {
		t.Fatalf("latency sample counts = all %d, fast path %d", stats.All.Count, stats.FastPath.Count)
	}
	if stats.FastPath.IntentToTransport.Count != 1 || stats.FastPath.IntentToDispatch.Count != 1 {
		t.Fatalf("intent latency distributions = %#v", stats.FastPath)
	}
	if stats.FastPath.IntentToTransport.LastMilliseconds < stats.FastPath.IntentToDispatch.LastMilliseconds {
		t.Fatalf("transport completion preceded dispatch: %#v", stats.FastPath)
	}
	if stats.FastPath.Transport.LastMilliseconds <= 0 {
		t.Fatalf("transport duration was not measured: %#v", stats.FastPath.Transport)
	}
}

func TestRouterMeasuresOnlyFirstSuccessfulCommandForIntentLatency(t *testing.T) {
	router := NewRouter(t.Context(), Config{
		Ready: func() bool { return true },
		Send:  func(context.Context, []byte) error { return nil },
	})
	defer router.Close()
	metadata := Metadata{
		OperationID: "multi-step", SubmittedAt: time.Now().Add(-time.Millisecond), Priority: PriorityInteractive,
	}
	for _, label := range []string{"first", "second"} {
		if err := router.Send(WithMetadata(t.Context(), metadata), outboundTestPayload(t, "ain", label)); err != nil {
			t.Fatal(err)
		}
	}
	stats := router.DispatchLatency()
	if stats.All.Count != 2 || stats.All.IntentToTransport.Count != 1 {
		t.Fatalf("multi-step latency counts = %#v", stats.All)
	}
}

func TestDefaultPriorityUsesActorPolicy(t *testing.T) {
	tests := map[string]Priority{
		"ui":                       PriorityInteractive,
		"cli":                      PriorityInteractive,
		"scheduler:rift":           PriorityScheduled,
		"automation:autoStation":   PriorityAutoStation,
		"automation:autoTCI":       PriorityAutoTCI,
		"automation:autoBird":      PriorityAutoBird,
		"automation:autoHospital":  PriorityHospital,
		"automation:autoSceatRes":  PriorityAutoSceat,
		"automation:autoRecruit":   PriorityRecruit,
		"automation:autoTool":      PriorityAutoTool,
		"automation:autoBeriWorld": PriorityAutoBeri,
		"report-manager":           PriorityService,
	}
	for actor, wanted := range tests {
		if actual := DefaultPriority(actor); actual != wanted {
			t.Errorf("DefaultPriority(%q) = %d, want %d", actor, actual, wanted)
		}
	}
}

func TestReportManagerDoesNotYieldToAutomationLock(t *testing.T) {
	if YieldsToAutomationLock("report-manager") {
		t.Fatal("passive report ingestion was paused by the automation lock")
	}
	if !YieldsToAutomationLock("automation:autoInvasion") {
		t.Fatal("Auto Invasion did not yield to the automation lock")
	}
}

func TestAutomationSafetyPriorityOrder(t *testing.T) {
	if PriorityAutoStation <= PriorityAutoBird {
		t.Fatalf("Auto Station priority %d must remain above Auto Bird %d", PriorityAutoStation, PriorityAutoBird)
	}
	if PriorityAutoBird <= PriorityAutoTCI {
		t.Fatalf("Auto Bird priority %d must remain above Auto TCI %d", PriorityAutoBird, PriorityAutoTCI)
	}
	if PriorityAutoBird <= PriorityHospital || PriorityAutoBird <= PriorityBackground {
		t.Fatalf("Auto Bird priority %d must remain above routine automation", PriorityAutoBird)
	}
}

func outboundTestPayload(t *testing.T, opcode string, label string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"label": label})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Protocol.Encode(Protocol.Command{Namespace: "EmpireEx_21", Opcode: opcode, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func outboundTestLabel(t *testing.T, payload []byte) string {
	t.Helper()
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Label string `json:"label"`
	}
	if err := json.Unmarshal(frame.Payload, &body); err != nil {
		t.Fatal(err)
	}
	return body.Label
}

func waitForOutboundQueue(t *testing.T, router *Router, lane Lane, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		router.mu.Lock()
		actual := len(router.queues[lane])
		router.mu.Unlock()
		if actual >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s queue did not reach %d", lane, count)
}
