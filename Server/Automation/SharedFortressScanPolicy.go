package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"context"
	"fmt"
	"time"
)

// Fortress coverage is a read-only sensor lane, independent of attack and
// supply decisions, so an account busy buying troops cannot strand its tiles.
type SharedFortressScanPolicy struct {
	accountKey string
	scans      *State.WorldMapStore
}

func NewSharedFortressScanPolicy(account string, scans *State.WorldMapStore) *SharedFortressScanPolicy {
	if account == "" || scans == nil {
		return nil
	}
	return &SharedFortressScanPolicy{account, scans}
}
func (*SharedFortressScanPolicy) ID() string         { return "sharedFortressScan" }
func (*SharedFortressScanPolicy) EnabledKey() string { return "auto_fortress" }
func (*SharedFortressScanPolicy) WakeDomains() []string {
	return []string{"account", "castles", "kingdom-transport", "session"}
}
func (*SharedFortressScanPolicy) WakeSections() []string { return []string{autoFortressSection} }
func (p *SharedFortressScanPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	waiting := Decision{Status: "waiting", NextCheckAt: snapshot.Now.Add(time.Minute)}
	settings := defaultAutoFortressSettings()
	if !decodeSection(snapshot.Configuration, autoFortressSection, &settings) || validateAutoFortressSettings(settings) != "" || snapshot.GameData == nil {
		return waiting, nil
	}
	definitions, err := snapshot.GameData.KingdomFortressDefinitions()
	if err != nil {
		return waiting, nil
	}
	byKingdom := map[State.KingdomID]GameData.KingdomFortressDefinition{}
	for _, definition := range definitions {
		byKingdom[State.KingdomID(definition.KingdomID)] = definition
	}
	sources := autoFortressSources(&snapshot.State, settings, byKingdom)
	for _, source := range sources {
		p.scans.RegisterFortressScanner(p.accountKey, fortressScanScope(snapshot.State, source.KingdomID), source.X, source.Y, time.Duration(settings.MapRefreshIntervalSec)*time.Second, snapshot.Now)
	}
	for _, source := range sources {
		lease := p.scans.AcquireFortressScan(p.accountKey, fortressScanScope(snapshot.State, source.KingdomID), snapshot.Now)
		if lease.NextCheckAt.Before(waiting.NextCheckAt) {
			waiting.NextCheckAt = lease.NextCheckAt
		}
		if len(lease.Windows) == 0 {
			continue
		}
		decision := autoFortressRequest(snapshot, map[string]float64{"sharedFortressParticipants": float64(lease.ParticipantCount), "sharedFortressWindows": float64(len(lease.Windows))}, fmt.Sprintf("Discover every fortress across %s", castleName(source)), "fortress.map.scan", map[string]any{"sourceCastleId": source.ID, "kingdomId": source.KingdomID, "cooperative": true, "leaseId": lease.LeaseID, "windows": lease.Windows, "scanStartedAt": snapshot.Now}, castleDecisionDescriptor("fortress_discover", source, nil))
		decision.NextCheckAt = lease.NextCheckAt
		decision.ReevaluateOnSuccess = true
		return decision, nil
	}
	return waiting, nil
}

var _ Policy = (*SharedFortressScanPolicy)(nil)
var _ StateWakePolicy = (*SharedFortressScanPolicy)(nil)
