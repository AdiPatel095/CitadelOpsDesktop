package Outbound

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRouterFinalDispatchValidationRunsAfterQueueWait(t *testing.T) {
	blockedSendStarted := make(chan struct{})
	releaseBlockedSend := make(chan struct{})
	var stateValid atomic.Bool
	stateValid.Store(true)
	stale := errors.New("dispatch state changed while queued")
	var validationCalls atomic.Int32
	var mu sync.Mutex
	var sent []string
	router := NewRouter(t.Context(), Config{
		Ready: func() bool { return true },
		Send: func(_ context.Context, payload []byte) error {
			label := outboundTestLabel(t, payload)
			mu.Lock()
			sent = append(sent, label)
			mu.Unlock()
			if label == "block" {
				close(blockedSendStarted)
				<-releaseBlockedSend
			}
			return nil
		},
	})
	defer router.Close()

	firstResult := make(chan error, 1)
	go func() {
		firstResult <- router.Send(t.Context(), outboundTestPayload(t, "ain", "block"))
	}()
	select {
	case <-blockedSendStarted:
	case <-t.Context().Done():
		t.Fatal("blocking command did not enter transport")
	}

	validate := func(context.Context) error {
		validationCalls.Add(1)
		if !stateValid.Load() {
			return stale
		}
		return nil
	}
	queued, err := router.enqueue(
		t.Context(), LaneCommand, outboundTestPayload(t, "ain", "stale"),
		Metadata{FinalDispatchValidation: validate},
	)
	if err != nil {
		t.Fatal(err)
	}
	stateValid.Store(false)
	close(releaseBlockedSend)

	if err := <-firstResult; err != nil {
		t.Fatalf("blocking command error = %v", err)
	}
	if err := <-queued.result; !errors.Is(err, stale) {
		t.Fatalf("queued command error = %v, want %v", err, stale)
	} else if IsIndeterminate(err) {
		t.Fatalf("queued validation error became indeterminate: %v", err)
	}
	if validationCalls.Load() != 1 {
		t.Fatalf("queued validation calls = %d, want 1", validationCalls.Load())
	}

	stateValid.Store(true)
	allowedContext := WithFinalDispatchValidation(t.Context(), validate)
	if err := router.Send(allowedContext, outboundTestPayload(t, "ain", "allowed")); err != nil {
		t.Fatalf("allowed guarded command error = %v", err)
	}
	if err := router.Send(t.Context(), outboundTestPayload(t, "ain", "unguarded")); err != nil {
		t.Fatalf("unguarded command error = %v", err)
	}
	if validationCalls.Load() != 2 {
		t.Fatalf("validation calls = %d, want 2", validationCalls.Load())
	}

	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(sent, []string{"block", "allowed", "unguarded"}) {
		t.Fatalf("physical sends = %#v", sent)
	}
}
