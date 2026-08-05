package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const allianceHelpResponseCheckInterval = 30 * time.Second

type AllianceHelpPolicy struct{}

func NewAllianceHelpPolicy() *AllianceHelpPolicy { return &AllianceHelpPolicy{} }

func (*AllianceHelpPolicy) ID() string { return "autoAllianceHelp" }

func (*AllianceHelpPolicy) EnabledKey() string { return "" }

func (*AllianceHelpPolicy) CorePolicy() {}

func (*AllianceHelpPolicy) WakeDomains() []string { return []string{"alliance-help"} }

// Alliance-help requests are short-lived and can fill while other members are
// responding. Evaluate their domain on arrival instead of waiting for the
// coordinator's shared state-event coalescing window.
func (*AllianceHelpPolicy) UrgentWakeDomains() []string { return []string{"alliance-help"} }

func (*AllianceHelpPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	nextCheck := snapshot.Now.Add(allianceHelpResponseCheckInterval)
	observed := snapshot.State.AllianceHelpRequests
	if snapshot.GameData == nil {
		return Decision{
			Status: "waiting", Detail: "Waiting for official alliance-help request limits",
			NextCheckAt: nextCheck,
		}, nil
	}
	if snapshot.State.Session.Generation == 0 {
		return Decision{
			Status: "waiting", Detail: "Waiting for the current alliance-help request list",
			NextCheckAt: nextCheck,
		}, nil
	}
	currentObservation := observed.OthersObservedGeneration == snapshot.State.Session.Generation &&
		!observed.OthersObservedAt.IsZero()
	if !currentObservation {
		if observed.LastHelpAllGeneration == snapshot.State.Session.Generation {
			return Decision{
				Status: "waiting", Detail: "Listening for new alliance-help requests",
				NextCheckAt: nextCheck,
			}, nil
		}
		return Decision{
			Status: "ready", Detail: "Check and help existing alliance requests now",
			NextCheckAt: nextCheck, Metrics: map[string]float64{"pendingRequests": 0},
			Request: &Intent.Request{
				Name: "alliance.help.answer_all", Arguments: json.RawMessage(`{"allowUnobserved":true}`),
			},
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	pending := State.PendingOtherAllianceHelpListIDs(snapshot.State)
	metrics := map[string]float64{"pendingRequests": float64(len(pending))}
	if len(pending) == 0 {
		return Decision{
			Status: "idle", Detail: "No alliance member currently needs help",
			NextCheckAt: nextCheck, Metrics: metrics,
		}, nil
	}
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Help %d pending alliance request(s) now", len(pending)),
		NextCheckAt: nextCheck, Metrics: metrics,
		Request:             &Intent.Request{Name: "alliance.help.answer_all"},
		ReevaluateOnSuccess: true,
		ReevaluateOnStale:   true,
	}, nil
}
