package App

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

func (a *Application) guardOpenGate(ctx context.Context, arguments json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var request defenseOpenGateRequest
	if err := json.Unmarshal(arguments, &request); err != nil {
		return err
	}
	now := time.Now().UTC()
	if a.Configuration == nil || !Automation.FeatureEnabledAt(a.Configuration.Snapshot(), "auto_station", now) {
		return fmt.Errorf("%w: Auto Station is disabled", Intent.ErrPlanStale)
	}
	var config struct {
		OpenGateFallback bool `json:"openGateFallback"`
		LeadTimeSec      int  `json:"leadTimeSec"`
		Settings         map[string][]struct {
			ID     State.UnitID `json:"id"`
			Amount int64        `json:"amount"`
		} `json:"settings"`
	}
	config.LeadTimeSec = 60
	// Match policy defaults when this optional section has never been saved.
	// A present malformed value remains a dispatch blocker.
	if raw := a.Configuration.Snapshot().Sections["automation.autoStation"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &config); err != nil {
			return err
		}
	}
	state := a.State.Snapshot()
	if err := validateStationSession(state, "autoStation", request.ConnectionGeneration, now); err != nil {
		return err
	}
	reserved := map[State.UnitID]int64{}
	if !state.Player.ProtectionMode.PreparingOrActive(now) {
		for _, item := range config.Settings[strconv.FormatInt(int64(request.CastleID), 10)] {
			reserved[item.ID] = item.Amount
		}
	}
	var data *GameData.Store
	if a.GameData != nil {
		data, _ = a.GameData.Current()
	}
	if err := validateTrackedGateRemainder(state, request, reserved, data, now); err != nil {
		return err
	}
	return validateAutoStationGate(state, request, config.OpenGateFallback, config.LeadTimeSec, now)
}
func validateAutoStationGate(s State.GameState, r defenseOpenGateRequest, fallback bool, lead int, now time.Time) error {
	stale := func(reason string) error { return fmt.Errorf("%w: %s", Intent.ErrPlanStale, reason) }
	if r.PlannedAt.IsZero() || now.Sub(r.PlannedAt) > 30*time.Second || r.PlannedAt.After(now) {
		return stale("gate request needs a fresh evaluation")
	}
	p := s.Player.ProtectionMode
	if p.ObservedAt.Before(r.PlannedAt) || p.ObservedAt.After(now) {
		return stale("fresh Protection Mode is unavailable")
	}
	if r.RequireProtectionMode && !p.PreparingOrActive(now) {
		return stale("Protection Mode ended before opening gates")
	}
	if !fallback && !p.PreparingOrActive(now) {
		return stale("ordinary gate fallback is disabled")
	}
	if s.MovementSnapshot.ObservedAt.Before(r.PlannedAt) || s.MovementSnapshot.ObservedAt.After(now) || s.MovementSnapshot.ConnectionGeneration != s.Session.ConnectionGeneration {
		return stale("incoming attacks were not refreshed for this session")
	}
	castle, ok := s.Castles[r.CastleID]
	if !ok || castle.KingdomID != 0 {
		return stale("gate protocol is confirmed only for owned primary-kingdom castles")
	}
	if castle.Defense.OpenGateUntil != nil && castle.Defense.OpenGateUntil.After(now) {
		return stale("gates are already open")
	}
	var first time.Time
	s.RangeMovements(func(_ State.MovementID, m State.MovementState) bool {
		if m.TargetCastleID == castle.ID && State.IsIncomingPlayerAttack(s, m, now) {
			if first.IsZero() || m.ArrivesAt.Before(first) {
				first = *m.ArrivesAt
			}

		}
		return true
	})
	if first.IsZero() || first.Sub(now) > time.Duration(min(max(lead, 60), 3600))*time.Second {
		return stale("no current attack is within this castle's gate window")
	}

	return nil
}

func validateTrackedGateRemainder(s State.GameState, r defenseOpenGateRequest, reserves map[State.UnitID]int64, data *GameData.Store, now time.Time) error {
	for _, op := range s.Stationing {
		if op.SourceCastleID == r.CastleID && op.ActiveInState(s, now) {
			castle := s.Castles[r.CastleID]
			if castle.UnitsObservedAt.Before(r.PlannedAt) || castle.UnitsObservedAt.Before(op.UpdatedAt) || castle.UnitsObservedAt.After(now) {
				return fmt.Errorf("%w: post-dispatch castle inventory is unavailable", Intent.ErrPlanStale)
			}
			remaining, known := Automation.EligibleStationRemainder(data, castle, reserves)
			if !known || remaining == 0 {
				return fmt.Errorf("%w: no confirmed eligible troops remain after evacuation", Intent.ErrPlanStale)
			}
		}
	}
	return nil
}
