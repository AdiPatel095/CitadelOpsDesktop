package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	RunID          string                `json:"runId,omitempty"`
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
	// At process start the live player profile has not arrived yet
	// (Player.ID is 0 until the first authenticated baseline), but the
	// persisted account binding already knows who this profile belongs to.
	// Without the fallback the report query silently returns nothing and
	// the rolling attack history starts empty after every restart.
	playerID := int64(snapshot.Player.ID)
	if playerID <= 0 {
		playerID = int64(snapshot.Account.PlayerID)
	}
	worldID := snapshot.Account.WorldID
	if worldID == "" {
		worldID = snapshot.Session.ServerURL
	}
	reports, err := reportStore.Recent(ctx, Reports.BattleReportQuery{
		AccountUID: snapshot.Account.UID,
		WorldID:    worldID,
		PlayerID:   playerID,
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
			TroopCount:   report.TroopsSent,
			TargetTypeID: report.TargetTypeID,
			TargetX:      report.TargetX,
			TargetY:      report.TargetY,
			LaunchedAt:   time.UnixMilli(report.DateMs).UTC(),
		})
	}
	if len(records) == 0 {
		return nil
	}
	_, err = state.ApplyComponents(State.Components(State.ComponentAttackAnalytics), func(gameState *State.GameState) ([]string, bool, error) {
		changed := State.MergeAutoStormLaunchHistory(gameState, records, now)
		return []string{"attack-analytics"}, changed, nil
	})
	return err
}

// refreshAttackHistoryOnBaseline re-runs the Auto Storm launch-history
// restore every time a new authenticated baseline commits (gbd). The login
// path can drop or reset state assembled before the baseline — after a
// process restart the startup restore may even have run before identity was
// known — so each baseline re-merges the rolling history from the local
// battle-report store. MergeAutoStormLaunchHistory is idempotent (keyed by
// movement, max troop count wins), so repeated merges only ever heal.
func (application *Application) refreshAttackHistoryOnBaseline(ctx context.Context) {
	if application.State == nil || application.ReportStore == nil {
		return
	}
	events, unsubscribe := application.State.Subscribe(32)
	defer unsubscribe()
	var restoredBaseline uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-events:
			snapshot := application.State.ReadOnlyView()
			baseline := snapshot.Session.BaselineGeneration
			if baseline == 0 || baseline == restoredBaseline {
				continue
			}
			if err := restoreRecentAutoStormLaunchHistory(ctx, application.State, application.ReportStore); err != nil {
				continue
			}
			restoredBaseline = baseline
		}
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
	var gameData *GameData.Store
	if application.GameData != nil {
		gameData, _ = application.GameData.Current()
	}
	_, err := application.State.ApplyComponents(State.Components(
		State.ComponentAttackAnalytics, State.ComponentRift, State.ComponentTowerQueue,
	), func(gameState *State.GameState) ([]string, bool, error) {
		var selected State.MovementState
		gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
			if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID ||
				movement.KingdomID != request.KingdomID || movement.TargetX != request.TargetX || movement.TargetY != request.TargetY ||
				movement.CommanderID == nil || *movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil {
				return true
			}
			if selected.ID == 0 || movement.ObservedAt.After(selected.ObservedAt) ||
				movement.ObservedAt.Equal(selected.ObservedAt) && movement.ID > selected.ID {
				selected = movement
			}
			return true
		})
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
			TroopCount:   attackMovementTroopCount(gameData, selected.Units),
			TargetTypeID: request.TargetTypeID, TargetX: request.TargetX, TargetY: request.TargetY,
			LaunchedAt: launchedAt.UTC(), ArrivesAt: selected.ArrivesAt.UTC(),
		})
		domains := []string{"attack-analytics", "movements"}
		if request.RunID != "" && request.FeatureID == State.AttackFeatureRiftMaiden {
			run := gameState.Rift.MaidenRun
			if run != nil && run.ID == request.RunID && run.Status == "running" && !movementIDPresent(run.LaunchIDs, selected.ID) {
				run.LaunchIDs = append(run.LaunchIDs, selected.ID)
				run.AttacksLaunched = len(run.LaunchIDs)
				if run.AttacksLaunched >= run.RequestedAttacks {
					run.AttacksLaunched = run.RequestedAttacks
					run.Status = "completed"
				}
				run.UpdatedAt = time.Now().UTC()
				changed = true
				domains = append(domains, "rift")
			}
		}
		if changed && request.FeatureID == State.AttackFeatureAutoTowers {
			gameState.IncrementTowerQueueConfirmedLaunches(request.SourceCastleID)
			domains = append(domains, "tower-queue")
		}
		return domains, changed, nil
	})
	return err
}

func movementIDPresent(values []State.MovementID, wanted State.MovementID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func attackMovementTroopCount(gameData *GameData.Store, units map[State.UnitID]int64) int64 {
	if gameData == nil || len(units) == 0 {
		return 0
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return 0
	}
	total := int64(0)
	for unitID, amount := range units {
		if unitID <= 0 || amount <= 0 {
			continue
		}
		raw, found := catalog.Find(strconv.FormatInt(int64(unitID), 10))
		if !found {
			return 0
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return 0
		}
		if GameData.IsToolRecord(record) {
			continue
		}
		total = saturatingTroopAdd(total, amount)
	}
	return total
}
