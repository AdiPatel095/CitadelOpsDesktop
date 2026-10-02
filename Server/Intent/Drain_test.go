package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"CitadelDesktop/Server/State"
)

func drainTestEngine(t *testing.T, effect Effect) (*Engine, chan struct{}, chan struct{}) {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(Definition{Name: "test.drain", Effect: effect, Planner: func(context.Context, PlanningContext, json.RawMessage) (Plan, error) {
		return Plan{Steps: []Step{{Action: "test.drain"}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	engine := NewEngine(registry, State.NewStore(&state), nil, nil, nil)
	started, finish := make(chan struct{}, 1), make(chan struct{})
	if err := engine.RegisterAction("test.drain", func(ctx context.Context, _ json.RawMessage) error {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-finish:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}); err != nil {
		t.Fatal(err)
	}
	return engine, started, finish
}

func TestFinishOpenWaitsForMutationsAndGatesAdmission(t *testing.T) {
	engine, started, finish := drainTestEngine(t, EffectWrite)
	engine.SubmitDetached(Request{ID: "mutation", Name: "test.drain"})
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- engine.FinishOpen(ctx) }()
	for {
		engine.admissionMu.RLock()
		draining := engine.draining
		engine.admissionMu.RUnlock()
		if draining {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("gate never closed")
		case <-time.After(time.Millisecond):
		}
	}
	if receipt := engine.SubmitDetached(Request{ID: "new", Name: "test.drain"}); receipt.Status != StatusFailed || receipt.Error != "runtime is draining" {
		t.Fatalf("admitted new operation: %+v", receipt)
	}
	select {
	case err := <-done:
		t.Fatalf("drain finished with an open mutation: %v", err)
	default:
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	receipt, _ := engine.Operation("mutation")
	if receipt.Status != StatusSucceeded {
		t.Fatalf("mutation did not finish: %+v", receipt)
	}
	engine.ResumeAdmission()
	if receipt := engine.Submit(t.Context(), Request{ID: "after", Name: "test.drain"}); receipt.Status != StatusSucceeded {
		t.Fatalf("admission did not resume: %+v", receipt)
	}
}

func TestFinishOpenTimeoutLeavesMutationRunningAndReopensAdmission(t *testing.T) {
	engine, started, finish := drainTestEngine(t, EffectWrite)
	engine.SubmitDetached(Request{ID: "mutation", Name: "test.drain"})
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := engine.FinishOpen(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout = %v", err)
	}
	if receipt, _ := engine.Operation("mutation"); receipt.Terminal() {
		t.Fatalf("timeout cancelled mutation: %+v", receipt)
	}
	close(finish)
	if receipt := engine.Submit(t.Context(), Request{ID: "after", Name: "test.drain"}); receipt.Status != StatusSucceeded {
		t.Fatalf("admission stayed closed: %+v", receipt)
	}
	if err := engine.WaitIdle(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestFinishOpenCancelsReadAsFailed(t *testing.T) {
	engine, started, _ := drainTestEngine(t, EffectRead)
	engine.SubmitDetached(Request{ID: "scan", Name: "test.drain"})
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := engine.FinishOpen(ctx); err != nil {
		t.Fatal(err)
	}
	receipt, _ := engine.Operation("scan")
	if receipt.Status != StatusFailed || receipt.Phase != EffectPhaseCompleted {
		t.Fatalf("read drain receipt: %+v", receipt)
	}
}

func TestFinishOpenThreeMinuteBoundary(t *testing.T) {
	for _, delay := range []time.Duration{179 * time.Second, 181 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				engine, started, finish := drainTestEngine(t, EffectWrite)
				engine.SubmitDetached(Request{ID: "mutation", Name: "test.drain"})
				<-started
				go func() { time.Sleep(delay); close(finish) }()
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
				defer cancel()
				err := engine.FinishOpen(ctx)
				if delay < 3*time.Minute {
					if err != nil {
						t.Fatal("2:59 mutation did not finish", err)
					}
				} else {
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatal("3:00 did not time out", err)
					}
					if r, _ := engine.Operation("mutation"); r.Terminal() {
						t.Fatal("timeout terminated mutation")
					}
				}
				if err := engine.WaitIdle(t.Context()); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
