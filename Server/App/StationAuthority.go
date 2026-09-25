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

// The existing JAA/JCA own-owner snapshot supplies AID and RPT. AIN is
// resolved only after that snapshot commits, never from a cached alliance ID.
func stationAllianceRefreshStep(after time.Time) Intent.Step {
	args, _ := json.Marshal(after)
	return Intent.RebuildOnResume(Intent.Step{Name: "Refresh current alliance before stationing", Resolver: "station.alliance.refresh", ResolverArguments: args, AwaitOpcode: "ain", TimeoutMillis: 10000, SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted})
}
func resolveStationAllianceRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var after time.Time
	if err := json.Unmarshal(arguments, &after); err != nil {
		return Intent.Step{}, err
	}
	s := input.State
	if after.IsZero() || s.Player.AllianceObservedAt.Before(after) || s.Player.AllianceObservedAt.After(time.Now()) || s.Player.AllianceID <= 0 {
		return Intent.Step{}, fmt.Errorf("%w: fresh own-player alliance membership is unavailable", Intent.ErrPlanStale)
	}
	payload, _ := json.Marshal(map[string]any{"AID": s.Player.AllianceID})
	step := contextCommandStep("Refresh current alliance", "ain", payload, "ain")
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step, nil
}

func validateStationAuthority(s State.GameState, sourceID, targetID State.CastleID, after time.Time, minDays int, now time.Time) error {
	stale := func(detail string) error { return fmt.Errorf("%w: %s", Intent.ErrPlanStale, detail) }
	if after.IsZero() || now.Sub(after) > 30*time.Second || after.After(now) || s.Player.AllianceObservedAt.Before(after) || s.Player.AllianceObservedAt.After(now) || s.Player.AllianceID <= 0 || s.Alliance.ID != s.Player.AllianceID || s.Alliance.ObservedAt.Before(s.Player.AllianceObservedAt) || s.Alliance.ObservedAt.After(now) {
		return stale("current alliance was not refreshed for this dispatch")
	}
	p := s.Player.ProtectionMode
	if p.ObservedAt.Before(after) || p.ObservedAt.After(now) || p.PreparingOrActive(now) {
		return stale("fresh Protection Mode does not permit stationing")
	}
	source, ok := s.Castles[sourceID]
	if !ok {
		return stale("source castle disappeared")
	}
	target, ok := allianceHolding(s.Alliance, targetID)
	if !ok || !stationHoldingType(target.SlotType) || target.KingdomID != source.KingdomID || target.CastleID == source.ID || target.PlayerID == s.Player.ID {
		return stale("station target is no longer eligible")
	}
	self, protected := false, false
	for _, member := range s.Alliance.Members {
		if member.PlayerID == s.Player.ID {
			self = true
		}
		if member.PlayerID == target.PlayerID {
			protected = time.Duration(member.ReturnProtectionSec)*time.Second-now.Sub(s.Alliance.ObservedAt) > time.Duration(max(minDays, 0))*24*time.Hour
		}
	}
	if !self || !protected {
		return stale("current roster no longer confirms self membership and target protection")
	}
	return nil
}

type stationDispatchGuard struct {
	TargetOwner State.PlayerID  `json:"targetOwner"`
	Request     stationRequest  `json:"request"`
	Payload     json.RawMessage `json:"payload"`
}

func (a *Application) guardStationDispatch(ctx context.Context, args json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var g stationDispatchGuard
	if err := json.Unmarshal(args, &g); err != nil {
		return err
	}
	now := time.Now().UTC()
	key := "auto_station"
	if g.Request.Purpose == "autoBird" {
		key = "auto_bird"
	}
	if a.Configuration == nil || !Automation.FeatureEnabledAt(a.Configuration.Snapshot(), key, now) {
		return fmt.Errorf("%w: Auto Station is disabled", Intent.ErrPlanStale)
	}
	s := a.State.Snapshot()
	if err := validateStationSession(s, g.Request.Purpose, g.Request.ConnectionGeneration, now); err != nil {
		return err
	}
	target, _ := allianceHolding(s.Alliance, g.Request.TargetCastleID)
	if g.TargetOwner <= 0 || target.PlayerID != g.TargetOwner {
		return fmt.Errorf("%w: station target owner changed", Intent.ErrPlanStale)
	}
	if err := validateStationAuthority(s, g.Request.SourceCastleID, g.Request.TargetCastleID, g.Request.DispatchStartedAt, g.Request.MinimumRPTDays, now); err != nil {
		return err
	}
	if g.Request.Purpose == "autoStation" {
		var config struct {
			LeadTimeSec int `json:"leadTimeSec"`
		}
		config.LeadTimeSec = 60
		_ = json.Unmarshal(a.Configuration.Snapshot().Sections["automation.autoStation"], &config)
		if !freshStationThreat(s, g.Request.SourceCastleID, g.Request.DispatchStartedAt, config.LeadTimeSec, now) {
			return fmt.Errorf("%w: no fresh incoming attack in this castle's evacuation window", Intent.ErrPlanStale)
		}
	}
	if err := validateStationPayload(s, g.Request.TargetCastleID, g.Payload); err != nil {
		return err
	}
	return a.guardSupportBatch(ctx, g.Payload)
}
func validateStationPayload(s State.GameState, targetID State.CastleID, payload json.RawMessage) error {
	var wire struct{ TX, TY int }
	if err := json.Unmarshal(payload, &wire); err != nil {
		return err
	}
	target, ok := allianceHolding(s.Alliance, targetID)
	if !ok || wire.TX != target.X || wire.TY != target.Y {
		return fmt.Errorf("%w: station target coordinates changed", Intent.ErrPlanStale)
	}
	return nil
}

func freshStationThreat(s State.GameState, id State.CastleID, after time.Time, lead int, now time.Time) bool {
	if s.MovementSnapshot.ObservedAt.Before(after) || s.MovementSnapshot.ObservedAt.After(now) || s.MovementSnapshot.ConnectionGeneration != s.Session.ConnectionGeneration {
		return false
	}
	found := false
	s.RangeMovements(func(_ State.MovementID, m State.MovementState) bool {
		if m.TargetCastleID == id && State.IsIncomingPlayerAttack(s, m, now) && m.ArrivesAt.Sub(now) <= time.Duration(min(max(lead, 60), 3600))*time.Second {
			found = true
		}
		return true
	})
	return found
}

func validateStationSession(s State.GameState, lane string, generation uint64, now time.Time) error {
	if !s.Session.LoggedIn || !s.Session.SocketReady || s.Session.ConnectionGeneration != generation || s.Automations[lane].SafetyLock.Active(now) {
		return fmt.Errorf("%w: stationing session or safety authority changed", Intent.ErrPlanStale)
	}
	return nil
}
