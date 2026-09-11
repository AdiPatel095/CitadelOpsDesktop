package Ingest

import (
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/State"
)

const attackAnalyticsReportMatchWindow = 45 * time.Minute

func reconcileAttackFeatureBattleReport(gameState *State.GameState, capture *State.BattleReportCapture) (bool, error) {
	return reconcileAttackFeatureBattleReportForMovement(gameState, capture, 0)
}

func reconcileAttackFeatureBattleReportForMovement(
	gameState *State.GameState,
	capture *State.BattleReportCapture,
	movementID State.MovementID,
) (bool, error) {
	if gameState == nil || capture == nil || capture.AutomationFeature != "" || gameState.Player.ID <= 0 ||
		len(capture.Summary) == 0 || len(capture.Details) == 0 {
		return false, nil
	}
	var summary eventBattleSummary
	if err := json.Unmarshal(capture.Summary, &summary); err != nil {
		return false, fmt.Errorf("decode attack analytics battle summary: %w", err)
	}
	if !battleSummaryHasOwnAttacker(summary.Participants, gameState.Player.ID) {
		return false, nil
	}
	kingdomID, targetTypeID, targetX, targetY, targetKnown := eventBattleTargetIdentity(summary)
	if !targetKnown {
		return false, nil
	}
	observedAt := battleCaptureOccurredAt(gameState, *capture)
	bestIndex := -1
	bestDistance := attackAnalyticsReportMatchWindow + time.Second
	for index, record := range gameState.AttackAnalytics.PendingAttacks {
		if movementID != 0 && record.MovementID != movementID ||
			record.KingdomID != kingdomID || record.TargetX != targetX || record.TargetY != targetY {
			continue
		}
		if record.TargetTypeID > 0 && record.TargetTypeID != targetTypeID {
			continue
		}
		impactAt := record.ArrivesAt
		if impactAt.IsZero() {
			impactAt = record.LaunchedAt
		}
		delta := observedAt.Sub(impactAt)
		if delta < -eventReportPreImpactSkew || delta > attackAnalyticsReportMatchWindow {
			continue
		}
		distance := delta
		if distance < 0 {
			distance = -distance
		}
		if distance >= bestDistance {
			continue
		}
		bestIndex, bestDistance = index, distance
	}
	if bestIndex < 0 {
		return false, nil
	}
	pending := gameState.MutablePendingAttackAnalytics()
	record := pending[bestIndex]
	gameState.SetPendingAttackAnalytics(append(
		pending[:bestIndex],
		pending[bestIndex+1:]...,
	))
	capture.AutomationFeature = record.FeatureID
	capture.MovementID = record.MovementID
	if capture.OccurredAt.IsZero() {
		capture.OccurredAt = observedAt
	}
	return true, nil
}

func battleCaptureOccurredAt(gameState *State.GameState, capture State.BattleReportCapture) time.Time {
	if !capture.OccurredAt.IsZero() {
		return capture.OccurredAt.UTC()
	}
	if gameState != nil {
		if notice, found := gameState.LookupReportNotice(capture.MessageID); found && !notice.ObservedAt.IsZero() {
			occurredAt := notice.ObservedAt
			if notice.AgeSec > 0 {
				occurredAt = occurredAt.Add(-time.Duration(notice.AgeSec) * time.Second)
			}
			return occurredAt.UTC()
		}
	}
	if !capture.CapturedAt.IsZero() {
		return capture.CapturedAt.UTC()
	}
	return time.Now().UTC()
}
