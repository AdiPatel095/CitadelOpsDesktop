package Automation

import (
	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/State"
)

// Only the premium policy permits phase fall-through. Queue, placement and
// resource prerequisites remain blocking, and skipped targets remain pending.
func rubyPolicyOnlyBlocked(diff Buildings.TargetDiffResult) bool {
	blocked := false
	for _, target := range diff.Targets {
		if target.Status == Buildings.TargetStatusSatisfied {
			continue
		}
		hasRuby := false
		for _, issue := range target.Issues {
			if Buildings.IsRubyUpgradeBlocker(issue.Code) {
				hasRuby = true
			} else if issue.Code != "resources_pending" {
				return false
			}
		}
		if !hasRuby {
			return false
		}
		blocked = true
	}
	return blocked
}

func rubyPolicyDetail(diff Buildings.TargetDiffResult) string {
	for _, issue := range diff.Issues {
		if Buildings.IsRubyUpgradeBlocker(issue.Code) {
			return issue.Message
		}
	}
	return "Premium upgrades remain pending while other eligible construction is complete."
}

func attachRubyUpgradeNotices(decision *Decision, snapshot Snapshot, castleID State.CastleID, target *Buildings.TargetCaptureResult, allowPremium bool) {
	if target == nil {
		return
	}
	diff, err := Buildings.CompileTargetDiff(snapshot.State, snapshot.GameData, Buildings.TargetDiffRequest{
		CastleID: castleID, Buildings: target.Buildings,
		Policy: Buildings.TargetDiffPolicy{AllowPremium: allowPremium},
	})
	if err != nil {
		return
	}
	attachRubyDiffNotices(decision, diff)
}

func attachRubyDiffNotices(decision *Decision, diff Buildings.TargetDiffResult) {
	for _, item := range diff.Targets {
		for _, issue := range item.Issues {
			if !Buildings.IsRubyUpgradeBlocker(issue.Code) {
				continue
			}
			if decision.Details == nil {
				decision.Details = map[string]string{}
			}
			decision.Details["rubyUpgradeNotice/"+item.TargetID] = item.Desired.DisplayName + ": " + issue.Message
			break
		}
	}
}

func attachStormRubyUpgradeNotices(decision *Decision, snapshot Snapshot, castle State.CastleState, settings autoStormSettings) {
	attachRubyUpgradeNotices(decision, snapshot, castle.ID, settings.Target, settings.Build.AllowPremium)
	catalog, err := snapshot.GameData.BuildingCatalog()
	if err != nil {
		return
	}
	fixed, err := autoStormFixedTargets(settings, catalog)
	if err != nil {
		return
	}
	diff, err := stormCompileFixedDiff(snapshot, settings, castle, fixed)
	if err == nil {
		attachRubyDiffNotices(decision, diff)
	}
}
