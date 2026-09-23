package Automation

import (
	"CitadelDesktop/Server/Localization"
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
			Status: "waiting", Detail: "Waiting for official alliance-help request limits", DetailDescriptor: Localization.New("server.automation.waiting_for_official_alliance.afcb59f1", "Waiting for official alliance-help request limits", nil),
			NextCheckAt: nextCheck,
		}, nil
	}
	if snapshot.State.Session.Generation == 0 {
		return Decision{
			Status: "waiting", Detail: "Waiting for the current alliance-help request list", DetailDescriptor: Localization.New("server.automation.waiting_for_the_current.c6734bc1", "Waiting for the current alliance-help request list", nil),
			NextCheckAt: nextCheck,
		}, nil
	}
	currentObservation := observed.OthersObservedGeneration == snapshot.State.Session.Generation &&
		!observed.OthersObservedAt.IsZero()
	if !currentObservation {
		if observed.LastHelpAllGeneration == snapshot.State.Session.Generation {
			return Decision{
				Status: "waiting", Detail: "Listening for new alliance-help requests", DetailDescriptor: Localization.New("server.automation.listening_for_new_alliance.2ebc2fe3", "Listening for new alliance-help requests", nil),
				NextCheckAt: nextCheck,
			}, nil
		}
		return Decision{
			Status: "ready", Detail: "Check and help existing alliance requests now", DetailDescriptor: Localization.New("server.automation.check_and_help_existing.b305fa4d", "Check and help existing alliance requests now", nil),
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
			Status: "idle", Detail: "No alliance member currently needs help", DetailDescriptor: Localization.New("server.automation.no_alliance_member_currently.a95f4b58", "No alliance member currently needs help", nil),
			NextCheckAt: nextCheck, Metrics: metrics,
		}, nil
	}
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Help %d pending alliance request(s) now", len(pending)), DetailDescriptor: Localization.New("server.automation.help_p_pending_alliance.709c1b51", "Help {p0} pending alliance request(s) now", Localization.Params{"p0": len(pending)}),
		NextCheckAt: nextCheck, Metrics: metrics,
		Request:             &Intent.Request{Name: "alliance.help.answer_all"},
		ReevaluateOnSuccess: true,
		ReevaluateOnStale:   true,
	}, nil
}
