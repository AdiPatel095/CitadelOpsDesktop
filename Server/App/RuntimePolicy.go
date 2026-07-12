package App

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
)

const (
	defaultAttackDelayMinSec = 4.0
	defaultAttackDelayMaxSec = 6.0
	defaultManualFocusSec    = 30
	minimumManualFocusSec    = 5
	maximumManualFocusSec    = 300
)

type runtimeSchedulerSettings struct {
	MinAttackDelay  float64 `json:"minAttackDelay"`
	MaxAttackDelay  float64 `json:"maxAttackDelay"`
	ManualFocusIdle int     `json:"manualFocusIdleSec"`
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

func (application *Application) manualFocusHold() time.Duration {
	seconds := application.runtimeSchedulerSettings().ManualFocusIdle
	switch {
	case seconds <= 0:
		seconds = defaultManualFocusSec
	case seconds < minimumManualFocusSec:
		seconds = minimumManualFocusSec
	case seconds > maximumManualFocusSec:
		seconds = maximumManualFocusSec
	}
	return time.Duration(seconds) * time.Second
}

func (application *Application) runtimeSchedulerSettings() runtimeSchedulerSettings {
	settings := runtimeSchedulerSettings{
		MinAttackDelay:  defaultAttackDelayMinSec,
		MaxAttackDelay:  defaultAttackDelayMaxSec,
		ManualFocusIdle: defaultManualFocusSec,
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

func (application *Application) executionGate(ctx context.Context, request Intent.Request, plan Intent.Plan) error {
	if application == nil || application.Session == nil || !strings.HasPrefix(request.Actor, "automation:") {
		return nil
	}
	for _, claim := range plan.Claims {
		if claim == "castle-focus" {
			return application.Session.WaitForManualFocusIdle(ctx)
		}
	}
	return nil
}
