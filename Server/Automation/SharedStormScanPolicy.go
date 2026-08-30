package Automation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"CitadelDesktop/Server/State"
)

type SharedStormScanCoordinator interface {
	AcquireStormScan(
		accountKey string,
		worldID string,
		kingdomID State.KingdomID,
		now time.Time,
	) State.StormScanAssignment
}

// SharedStormScanPolicy is a process-cooperative, read-only sensor lane. It is
// deliberately independent from Auto Storm settings: capable connected
// accounts contribute a fair portion of public map coverage, while each
// account's private automation settings alone decide whether to act on it.
type SharedStormScanPolicy struct {
	accountKey  string
	coordinator SharedStormScanCoordinator
}

func NewSharedStormScanPolicy(accountKey string, coordinator SharedStormScanCoordinator) *SharedStormScanPolicy {
	if strings.TrimSpace(accountKey) == "" || coordinator == nil {
		return nil
	}
	return &SharedStormScanPolicy{accountKey: strings.TrimSpace(accountKey), coordinator: coordinator}
}

func (*SharedStormScanPolicy) ID() string         { return "sharedStormScan" }
func (*SharedStormScanPolicy) EnabledKey() string { return "" }
func (*SharedStormScanPolicy) CorePolicy()        {}

func (*SharedStormScanPolicy) WakeDomains() []string {
	return []string{"account", "castles", "kingdom-transport", "session", "storm-scan"}
}

func (policy *SharedStormScanPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	castle, found := autoStormCastle(snapshot.State, nil)
	if !found {
		return Decision{
			Status: "waiting", Detail: "Shared Storm scanning is waiting for this account to unlock the Storm kingdom",
			NextCheckAt: snapshot.Now.Add(time.Minute),
		}, nil
	}
	worldID := strings.TrimSpace(snapshot.State.Account.WorldID)
	if worldID == "" {
		worldID = strings.TrimSpace(snapshot.State.Session.ServerURL)
	}
	if worldID == "" {
		return Decision{
			Status: "waiting", Detail: "Shared Storm scanning is waiting for a bound game world",
			NextCheckAt: snapshot.Now.Add(time.Minute),
		}, nil
	}
	assignment := policy.coordinator.AcquireStormScan(
		policy.accountKey, worldID, autoStormKingdomID, snapshot.Now,
	)
	metrics := map[string]float64{
		"sharedStormParticipants": float64(assignment.ParticipantCount),
		"sharedStormSlot":         float64(assignment.Slot),
		"sharedStormWindows":      float64(assignment.Coverage.WindowCount),
		"sharedStormFreshWindows": float64(assignment.Coverage.FreshWindowCount),
	}
	if len(assignment.Windows) == 0 {
		next := assignment.NextCheckAt
		if next.IsZero() || !next.After(snapshot.Now) {
			next = snapshot.Now.Add(time.Minute)
		}
		detail := fmt.Sprintf(
			"Shared Storm coverage is current for %d/%d windows across %d capable account(s)",
			assignment.Coverage.FreshWindowCount, assignment.Coverage.WindowCount, assignment.ParticipantCount,
		)
		if assignment.Coverage.WindowCount == 0 {
			detail = "Shared Storm scan participants are settling before work is partitioned"
		}
		return Decision{Status: "waiting", Detail: detail, NextCheckAt: next, Metrics: metrics}, nil
	}
	bounds := unionStormBounds(assignment.Windows)
	request := map[string]any{
		"sourceCastleId": castle.ID,
		"fullMap":        true,
		"cooperative":    true,
		"leaseId":        assignment.LeaseID,
		"windows":        assignment.Windows,
		"bounds":         bounds,
		"scanStartedAt":  snapshot.Now,
	}
	decision := autoStormIntentDecision(
		snapshot.Now,
		metrics,
		fmt.Sprintf(
			"Scan %d/%d leased Storm map windows for shared world coverage using participant slot %d/%d",
			len(assignment.Windows), assignment.Coverage.WindowCount, assignment.Slot+1, assignment.ParticipantCount,
		),
		"storm.map.scan",
		request,
	)
	decision.NextCheckAt = assignment.NextCheckAt
	decision.ReevaluateOnSuccess = true
	return *decision, nil
}

func unionStormBounds(windows []State.StormMapBounds) State.StormMapBounds {
	if len(windows) == 0 {
		return State.StormMapBounds{}
	}
	result := windows[0]
	for _, bounds := range windows[1:] {
		result.X1 = min(result.X1, bounds.X1)
		result.Y1 = min(result.Y1, bounds.Y1)
		result.X2 = max(result.X2, bounds.X2)
		result.Y2 = max(result.Y2, bounds.Y2)
	}
	return result
}

var _ Policy = (*SharedStormScanPolicy)(nil)
var _ CorePolicy = (*SharedStormScanPolicy)(nil)
var _ StateWakePolicy = (*SharedStormScanPolicy)(nil)
