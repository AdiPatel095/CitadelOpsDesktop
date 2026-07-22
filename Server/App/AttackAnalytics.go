package App

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type attackFeatureCaptureRequest struct {
	FeatureID      State.AttackFeatureID `json:"featureId"`
	SourceCastleID State.CastleID        `json:"sourceCastleId"`
	CommanderID    State.CommanderID     `json:"commanderId"`
	KingdomID      State.KingdomID       `json:"kingdomId"`
	TargetTypeID   int                   `json:"targetTypeId,omitempty"`
	TargetX        int                   `json:"targetX"`
	TargetY        int                   `json:"targetY"`
}

func attackFeatureCaptureStep(request attackFeatureCaptureRequest) Intent.Step {
	arguments, _ := json.Marshal(request)
	return Intent.Step{
		Name: "Attribute confirmed attack movement", Action: "attack.analytics.capture", ActionArguments: arguments,
	}
}

func (application *Application) captureAttackFeatureLaunch(_ context.Context, arguments json.RawMessage) error {
	var request attackFeatureCaptureRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if !State.IsReportAnalyticsFeature(request.FeatureID) {
		return fmt.Errorf("attack feature %q is not eligible for report analytics", request.FeatureID)
	}
	if request.SourceCastleID <= 0 || request.CommanderID < 0 {
		return fmt.Errorf("attack analytics capture requires a source castle and commander")
	}
	_, err := application.State.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		var selected State.MovementState
		for _, movement := range gameState.Movements {
			if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID ||
				movement.KingdomID != request.KingdomID || movement.TargetX != request.TargetX || movement.TargetY != request.TargetY ||
				movement.CommanderID == nil || *movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil {
				continue
			}
			if selected.ID == 0 || movement.ObservedAt.After(selected.ObservedAt) ||
				movement.ObservedAt.Equal(selected.ObservedAt) && movement.ID > selected.ID {
				selected = movement
			}
		}
		if selected.ID == 0 {
			return nil, false, fmt.Errorf(
				"CRA response did not return commander %d's %s movement to %d:%d",
				request.CommanderID, request.FeatureID, request.TargetX, request.TargetY,
			)
		}
		launchedAt := selected.ObservedAt
		if launchedAt.IsZero() {
			launchedAt = time.Now().UTC()
		}
		changed := State.RecordAttackFeatureLaunch(gameState, State.AttackFeatureLaunch{
			MovementID: selected.ID, FeatureID: request.FeatureID, KingdomID: request.KingdomID,
			TargetTypeID: request.TargetTypeID, TargetX: request.TargetX, TargetY: request.TargetY,
			LaunchedAt: launchedAt.UTC(), ArrivesAt: selected.ArrivesAt.UTC(),
		})
		return []string{"attack-analytics", "movements"}, changed, nil
	})
	return err
}
