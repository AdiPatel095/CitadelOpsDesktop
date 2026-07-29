package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/CommanderFeatures"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

const (
	defaultAttackDelayMinSec = 4.0
	defaultAttackDelayMaxSec = 6.0
	defaultRelogDelaySec     = 5 * 60
	minimumRelogDelaySec     = 60
	maximumRelogDelaySec     = 24 * 60 * 60
	commanderFeatureSection  = CommanderFeatures.Section
)

type runtimeSchedulerSettings struct {
	MinAttackDelay   float64        `json:"minAttackDelay"`
	MaxAttackDelay   float64        `json:"maxAttackDelay"`
	BotLocked        bool           `json:"botLocked"`
	AttackPriorities map[string]int `json:"attackPriorities"`
}

type runtimeSessionReconnectSettings struct {
	RelogDelaySec int `json:"relogDelaySec"`
}

func (application *Application) attackLaunchDelay() time.Duration {
	settings := application.runtimeSchedulerSettings()
	minimum := settings.MinAttackDelay
	if minimum < defaultAttackDelayMinSec {
		minimum = defaultAttackDelayMinSec
	}
	maximum := settings.MaxAttackDelay
	if maximum < minimum {
		maximum = minimum
	}
	seconds := minimum
	if maximum > minimum {
		seconds += rand.Float64() * (maximum - minimum)
	}
	return time.Duration(seconds * float64(time.Second))
}

func (application *Application) runtimeSchedulerSettings() runtimeSchedulerSettings {
	settings := runtimeSchedulerSettings{
		MinAttackDelay:   defaultAttackDelayMinSec,
		MaxAttackDelay:   defaultAttackDelayMaxSec,
		AttackPriorities: map[string]int{},
	}
	if application == nil || application.Configuration == nil {
		return settings
	}
	raw, ok := application.Configuration.Section("scheduler")
	if ok {
		_ = json.Unmarshal(raw, &settings)
	}
	return settings
}

func (application *Application) relogDelay() time.Duration {
	settings := runtimeSessionReconnectSettings{RelogDelaySec: defaultRelogDelaySec}
	if application != nil && application.Configuration != nil {
		if raw, ok := application.Configuration.Section("session.reconnect"); ok {
			_ = json.Unmarshal(raw, &settings)
		}
	}
	seconds := settings.RelogDelaySec
	if seconds < minimumRelogDelaySec {
		seconds = minimumRelogDelaySec
	}
	if seconds > maximumRelogDelaySec {
		seconds = maximumRelogDelaySec
	}
	return time.Duration(seconds) * time.Second
}

func (application *Application) automationLocked() bool {
	return application.runtimeSchedulerSettings().BotLocked
}

func (application *Application) attackAdmissionWeight(_ Intent.Request, admission Intent.Admission) int {
	weight := application.runtimeSchedulerSettings().AttackPriorities[admission.Module]
	if weight <= 0 {
		weight = application.defaultAttackAdmissionWeight(admission.Module)
	}
	if weight > 100 {
		return 100
	}
	return weight
}

func (application *Application) defaultAttackAdmissionWeight(moduleID string) int {
	if application != nil && application.Intents != nil {
		for _, definition := range application.Intents.Registry().Definitions() {
			if definition.AttackModule != nil && definition.AttackModule.ID == moduleID {
				return definition.AttackModule.DefaultWeight
			}
		}
	}
	return 50
}

func (application *Application) executionGate(
	ctx context.Context,
	request Intent.Request,
	plan Intent.Plan,
	point Intent.ExecutionPoint,
) error {
	if application == nil {
		return nil
	}
	if plan.Effect != Intent.EffectRead {
		if err := application.PersistenceError(); err != nil {
			return fmt.Errorf("durable storage is unavailable: %w", err)
		}
	}
	if point == Intent.ExecutionBeforeClaims {
		if err := application.requireAssignedAttackCommanders(plan); err != nil {
			return err
		}
	}
	if application.Session == nil || !Outbound.YieldsToAutomationLock(request.Actor) {
		return nil
	}
	switch point {
	case Intent.ExecutionBeforeClaims:
		return application.Session.WaitForActorAutomationUnlocked(ctx, request.Actor)
	case Intent.ExecutionBeforeStep:
		if application.Session.AutomationLockedFor(request.Actor) {
			return Outbound.ErrAutomationLocked
		}
	}
	return nil
}

func (application *Application) requireAssignedAttackCommanders(plan Intent.Plan) error {
	if plan.Admission == nil || plan.Admission.Class != Intent.AdmissionAttackLaunch {
		return nil
	}
	moduleID := strings.TrimSpace(plan.Admission.Module)
	if moduleID == "" {
		return fmt.Errorf("attack launch does not identify its commander feature")
	}
	if moduleID == manualAllianceAttackModuleID {
		return nil
	}
	label := application.attackModuleLabel(moduleID)
	if application.Configuration == nil {
		return fmt.Errorf("%s commander assignments are unavailable", label)
	}
	raw, exists := application.Configuration.Section(commanderFeatureSection)
	if !exists {
		return nil
	}
	if len(raw) == 0 {
		return fmt.Errorf("%s commander assignments are invalid", label)
	}
	settings, err := CommanderFeatures.Decode(raw)
	if err != nil {
		return fmt.Errorf("%s commander assignments are invalid", label)
	}
	commanders, err := attackPlanCommanderIDs(plan)
	if err != nil {
		return fmt.Errorf("validate %s commander assignment: %w", label, err)
	}
	gameState := State.NewGameState()
	if application.State != nil {
		gameState = application.State.Snapshot()
	}
	for _, commanderID := range commanders {
		if !CommanderFeatures.AssignmentAllows(settings, moduleID, commanderID) {
			return fmt.Errorf("commander %d is not assigned to %s", commanderID, label)
		}
		if !CommanderFeatures.MeetsFeatureRequirements(gameState, settings, moduleID, commanderID) {
			return fmt.Errorf("commander %d does not meet the %s equipment requirement", commanderID, label)
		}
	}
	return nil
}

func (application *Application) attackModuleLabel(moduleID string) string {
	if application != nil && application.Intents != nil {
		for _, definition := range application.Intents.Registry().Definitions() {
			if definition.AttackModule != nil && definition.AttackModule.ID == moduleID {
				return definition.AttackModule.Label
			}
		}
	}
	return moduleID
}

func attackPlanCommanderIDs(plan Intent.Plan) ([]State.CommanderID, error) {
	seen := map[State.CommanderID]struct{}{}
	commanders := make([]State.CommanderID, 0)
	for _, claim := range plan.Claims {
		claim = strings.TrimSpace(claim)
		if !strings.HasPrefix(claim, "commander:") {
			continue
		}
		rawID := strings.TrimSpace(strings.TrimPrefix(claim, "commander:"))
		wireID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || wireID < 0 {
			return nil, fmt.Errorf("invalid commander claim %q", claim)
		}
		commanderID := State.CommanderID(wireID)
		if _, duplicate := seen[commanderID]; duplicate {
			continue
		}
		seen[commanderID] = struct{}{}
		commanders = append(commanders, commanderID)
	}
	if len(commanders) == 0 {
		return nil, fmt.Errorf("attack plan does not claim a commander")
	}
	return commanders, nil
}
