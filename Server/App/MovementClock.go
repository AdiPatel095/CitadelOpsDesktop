package App

import (
	"context"
	"time"

	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/State"
)

func (application *Application) runMovementClock(ctx context.Context) {
	if application == nil || application.State == nil {
		return
	}
	events, unsubscribe := application.State.Subscribe(64)
	defer unsubscribe()
	var timer *time.Timer
	var timerChannel <-chan time.Time
	resetTimer := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timerChannel = nil
		}
		next := nextMovementCompletion(application.State.ReadOnlyView())
		if next.IsZero() {
			return
		}
		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			timer.Reset(delay)
		}
		timerChannel = timer.C
	}
	reconcile := func() {
		now := time.Now().UTC()
		_, _ = application.State.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
			if !Ingest.ReconcileExpiredMovements(gameState, now) {
				return nil, false, nil
			}
			return []string{"movements", "commanders", "khan"}, true, nil
		})
	}
	reconcile()
	resetTimer()
	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case event := <-events:
			if stateEventIncludes(event, "movements") {
				resetTimer()
			}
		case <-timerChannel:
			timerChannel = nil
			reconcile()
			resetTimer()
		}
	}
}

func nextMovementCompletion(gameState State.GameState) time.Time {
	var next time.Time
	for _, movement := range gameState.Movements {
		if movement.Direction == 0 && movement.WaitSeconds > 0 {
			continue
		}
		var completion *time.Time
		if movement.CommanderID != nil && State.MovementOwnedByCurrentPlayer(gameState, movement) {
			completion = State.CommanderMovementReleaseAt(movement)
		} else if movement.Direction == 0 {
			completion = movement.ArrivesAt
		} else if movement.Direction == 1 {
			completion = movement.ReturnsAt
		}
		if completion == nil || completion.IsZero() {
			continue
		}
		if next.IsZero() || completion.Before(next) {
			next = *completion
		}
	}
	return next
}

func stateEventIncludes(event State.Event, domain string) bool {
	for _, candidate := range event.Domains {
		if candidate == domain {
			return true
		}
	}
	return false
}
