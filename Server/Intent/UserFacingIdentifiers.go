package Intent

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
)

func (engine *Engine) identifierLabels() GameData.IdentifierLabels {
	if engine == nil {
		return GameData.IdentifierLabels{}
	}
	engine.labelsMu.RLock()
	labels := engine.labels
	ready := engine.labelsReady
	engine.labelsMu.RUnlock()
	if !ready {
		input := engine.planningContext()
		return GameData.NewIdentifierLabels(input.State, input.GameData, input.Language)
	}
	return labels
}

func (engine *Engine) humanizeText(text string) string {
	return engine.identifierLabels().Humanize(text)
}

func humanizePlanIdentifiers(input PlanningContext, plan Plan) Plan {
	labels := GameData.NewIdentifierLabels(input.State, input.GameData, input.Language)
	beforeSummary := plan.Summary
	plan.Summary = labels.Humanize(plan.Summary)
	if beforeSummary != plan.Summary {
		plan.SummaryDescriptor = nil
	}
	plan.SummaryDescriptor = Localization.Bind(plan.SummaryDescriptor, plan.Summary)
	for index := range plan.Steps {
		beforeName := plan.Steps[index].Name
		plan.Steps[index].Name = labels.Humanize(plan.Steps[index].Name)
		if beforeName != plan.Steps[index].Name {
			plan.Steps[index].NameDescriptor = nil
		}
		plan.Steps[index].NameDescriptor = Localization.Bind(plan.Steps[index].NameDescriptor, plan.Steps[index].Name)
	}
	return plan
}

func (engine *Engine) humanizeReceiptIdentifiers(receipt Receipt) Receipt {
	labels := engine.identifierLabels()
	if receipt.Error == "" {
		receipt.RawError = ""
		receipt.Failure = nil
	} else if receipt.RawError == "" {
		receipt.RawError = receipt.Error
	}
	receipt.Error = labels.Humanize(receipt.Error)
	if receipt.Failure != nil {
		failure := *receipt.Failure
		beforeMessage := failure.Message
		failure.Message = labels.Humanize(failure.Message)
		if beforeMessage != failure.Message {
			failure.MessageDescriptor = nil
		}
		beforeExplanation := failure.Explanation
		failure.Explanation = labels.Humanize(failure.Explanation)
		if beforeExplanation != failure.Explanation {
			failure.ExplanationDescriptor = nil
		}
		beforeRecovery := failure.Recovery
		failure.Recovery = labels.Humanize(failure.Recovery)
		if beforeRecovery != failure.Recovery {
			failure.RecoveryDescriptor = nil
		}
		failure.MessageDescriptor = Localization.Bind(failure.MessageDescriptor, failure.Message)
		failure.ExplanationDescriptor = Localization.Bind(failure.ExplanationDescriptor, failure.Explanation)
		failure.RecoveryDescriptor = Localization.Bind(failure.RecoveryDescriptor, failure.Recovery)
		receipt.Failure = &failure
	}
	if receipt.Plan != nil {
		plan := *receipt.Plan
		beforeSummary := plan.Summary
		plan.Summary = labels.Humanize(plan.Summary)
		if beforeSummary != plan.Summary {
			plan.SummaryDescriptor = nil
		}
		plan.SummaryDescriptor = Localization.Bind(plan.SummaryDescriptor, plan.Summary)
		plan.Steps = append([]Step(nil), plan.Steps...)
		for index := range plan.Steps {
			beforeName := plan.Steps[index].Name
			plan.Steps[index].Name = labels.Humanize(plan.Steps[index].Name)
			if beforeName != plan.Steps[index].Name {
				plan.Steps[index].NameDescriptor = nil
			}
			plan.Steps[index].NameDescriptor = Localization.Bind(plan.Steps[index].NameDescriptor, plan.Steps[index].Name)
		}
		receipt.Plan = &plan
	}
	return receipt
}
