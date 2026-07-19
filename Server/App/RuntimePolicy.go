package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
)

const (
	defaultAttackDelayMinSec = 4.0
	defaultAttackDelayMaxSec = 6.0
)

type runtimeSchedulerSettings struct {
	MinAttackDelay   float64        `json:"minAttackDelay"`
	MaxAttackDelay   float64        `json:"maxAttackDelay"`
	BotLocked        bool           `json:"botLocked"`
	AttackPriorities map[string]int `json:"attackPriorities"`
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
	if application.Session == nil || !Outbound.YieldsToAutomationLock(request.Actor) {
		return nil
	}
	switch point {
	case Intent.ExecutionBeforeClaims:
		return application.Session.WaitForAutomationUnlocked(ctx)
	case Intent.ExecutionBeforeStep:
		if application.Session.AutomationLocked() {
			return Outbound.ErrAutomationLocked
		}
	}
	return nil
}
