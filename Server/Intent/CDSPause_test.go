package Intent

import (
	"fmt"
	"reflect"
	"testing"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

func TestCDSPauseCancelsWithoutFailureFallbackAfterPreparation(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(fmt.Sprintf("prepared-%t", prepared), func(t *testing.T) {
			engine := NewEngine(nil, State.NewStore(State.NewGameState()), nil, nil, nil)
			receipt := Receipt{
				ID: "cds-pause", Intent: "troops.station", Actor: "automation:autoStation",
				Plan: &Plan{Effect: EffectLaunch},
			}
			completed := map[string]int{}
			if prepared {
				receipt.CompletedStepIndexes = []int{0}
				completed["prepare"] = 1
			}
			result := engine.failAfterProgress(receipt, fmt.Errorf("Station troops: %w", Outbound.ErrCDSPaused), completed)
			if result.Status != StatusCancelled || result.Phase != EffectPhaseCompleted {
				t.Fatalf("operator pause became a failure eligible for fallback: %+v", result)
			}
			if result.CompletedAt == nil || !reflect.DeepEqual(result.CompletedStepIndexes, receipt.CompletedStepIndexes) {
				t.Fatal("cancellation lost completion or prior preparation evidence")
			}
			if result.Failure != nil && result.Failure.GameCode != nil {
				t.Fatal("operator pause was presented as a game rejection")
			}
		})
	}
}
