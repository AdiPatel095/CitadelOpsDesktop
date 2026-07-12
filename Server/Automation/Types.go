package Automation

import (
	"context"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type Snapshot struct {
	State         State.GameState
	Configuration Configuration.Snapshot
	GameData      *GameData.Store
	Now           time.Time
}

type Decision struct {
	Status      string
	Detail      string
	NextCheckAt time.Time
	Metrics     map[string]float64
	Request     *Intent.Request
	FollowUp    *Intent.Request
}

type Policy interface {
	ID() string
	EnabledKey() string
	Evaluate(context.Context, Snapshot) (Decision, error)
}

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type IntentSubmitter interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}
