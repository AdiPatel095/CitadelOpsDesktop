package Intent

import "CitadelDesktop/Server/GameData"

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
	plan.Summary = labels.Humanize(plan.Summary)
	for index := range plan.Steps {
		plan.Steps[index].Name = labels.Humanize(plan.Steps[index].Name)
	}
	return plan
}

func (engine *Engine) humanizeReceiptIdentifiers(receipt Receipt) Receipt {
	labels := engine.identifierLabels()
	if receipt.Error == "" {
		receipt.RawError = ""
	} else if receipt.RawError == "" {
		receipt.RawError = receipt.Error
	}
	receipt.Error = labels.Humanize(receipt.Error)
	if receipt.Plan != nil {
		plan := *receipt.Plan
		plan.Summary = labels.Humanize(plan.Summary)
		plan.Steps = append([]Step(nil), plan.Steps...)
		for index := range plan.Steps {
			plan.Steps[index].Name = labels.Humanize(plan.Steps[index].Name)
		}
		receipt.Plan = &plan
	}
	return receipt
}
