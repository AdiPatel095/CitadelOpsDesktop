package App

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Reports"
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

func restoreRecentAutoStormLaunchHistory(
	ctx context.Context,
	state *State.Store,
	reportStore *Reports.SQLiteStore,
) error {
	if state == nil || reportStore == nil {
		return nil
	}
	now := time.Now().UTC()
	snapshot := state.ReadOnlyView()
	reports, err := reportStore.Recent(ctx, Reports.BattleReportQuery{
		AccountUID: snapshot.Account.UID,
		WorldID:    snapshot.Account.WorldID,
		PlayerID:   int64(snapshot.Player.ID),
		FeatureID:  string(State.AttackFeatureAutoStorm),
		Since:      now.Add(-72 * time.Hour),
		Limit:      100_000,
	})
	if err != nil {
		return err
	}
	records := make([]State.AttackFeatureLaunch, 0, len(reports))
	for _, report := range reports {
		if report.MovementID <= 0 || report.DateMs <= 0 {
			continue
		}
		records = append(records, State.AttackFeatureLaunch{
			MovementID:   State.MovementID(report.MovementID),
			FeatureID:    State.AttackFeatureAutoStorm,
			KingdomID:    State.KingdomID(GameData.StormKingdomID),
			TargetTypeID: report.TargetTypeID,
			TargetX:      report.TargetX,
			TargetY:      report.TargetY,
			LaunchedAt:   time.UnixMilli(report.DateMs).UTC(),
		})
	}
	if len(records) == 0 {
		return nil
	}
	_, err = state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		changed := State.MergeAutoStormLaunchHistory(gameState, records, now)
		return []string{"attack-analytics"}, changed, nil
	})
	return err
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
		domains := []string{"attack-analytics", "movements"}
		if changed && request.FeatureID == State.AttackFeatureAutoTowers {
			if gameState.TowerQueue.ConfirmedLaunchesByCastle == nil {
				gameState.TowerQueue.ConfirmedLaunchesByCastle = map[State.CastleID]int64{}
			}
			gameState.TowerQueue.ConfirmedLaunchesByCastle[request.SourceCastleID]++
			domains = append(domains, "tower-queue")
		}
		return domains, changed, nil
	})
	return err
}
