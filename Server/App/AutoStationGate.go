package App

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"fmt"
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
	}
	config.LeadTimeSec = 60
	if err := json.Unmarshal(a.Configuration.Snapshot().Sections["automation.autoStation"], &config); err != nil {
		return err
	}
	state := a.State.Snapshot()
	if err := validateStationSession(state, "autoStation", request.ConnectionGeneration, now); err != nil {
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
	for _, op := range s.Stationing {
		if op.SourceCastleID == castle.ID && op.ActiveInState(s, now) {
			return stale("troops are already tracked outside the castle")
		}
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
