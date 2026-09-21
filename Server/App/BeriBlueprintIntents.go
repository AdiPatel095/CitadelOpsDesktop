package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type beriBlueprintSaveRequest struct {
	Target Buildings.TargetCaptureResult `json:"target"`
	Policy Buildings.TargetDiffPolicy    `json:"policy"`
}

type beriBlueprintSaveAction struct {
	Blueprint Buildings.BerimondBlueprint `json:"blueprint"`
}

type beriBlueprintActivateRequest struct {
	ID string `json:"id"`
}

func (application *Application) registerBeriBlueprintIntents() error {
	for name, action := range map[string]Intent.Action{
		"beri.blueprint.save":     application.saveBeriBlueprint,
		"beri.blueprint.activate": application.activateBeriBlueprint,
	} {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}
	for _, definition := range []Intent.Definition{
		{
			Name: "beri.blueprint.save", Description: "Preflight and save one durable Berimond camp blueprint without replacing other capture modes", DescriptionDescriptor: Localization.New("server.intent.description.d47ee3f3", "Preflight and save one durable Berimond camp blueprint without replacing other capture modes", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"target":{"version":1,"castleId":901,"kingdomId":10,"mode":"functional","ground":[],"buildings":[],"fixed":[],"summary":{}},"policy":{"allowPremium":false,"resourceReserves":{}}}`),
			Planner:          planBeriBlueprintSave,
		},
		{
			Name: "beri.blueprint.activate", Description: "Activate a saved Berimond blueprint or pause blueprint reconciliation without deleting it", DescriptionDescriptor: Localization.New("server.intent.description.7e1b773a", "Activate a saved Berimond blueprint or pause blueprint reconciliation without deleting it", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"id":"beri-functional"}`), Planner: planBeriBlueprintActivate,
		},
	} {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planBeriBlueprintSave(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request beriBlueprintSaveRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.Target.KingdomID != State.KingdomID(GameData.BerimondKingdomID) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Berimond blueprint must target kingdom %d", GameData.BerimondKingdomID), Localization.New("server.app.berimond_blueprint_must_target.7ed1f194", "Berimond blueprint must target kingdom {p0}", Localization.Params{"p0": fmt.Sprintf("%d", GameData.BerimondKingdomID)}))
	}
	diff, err := Buildings.CompileBlueprintDiff(input.State, input.GameData, Buildings.BlueprintDiffRequest{
		Target: request.Target, Policy: request.Policy,
	})
	if err != nil {
		return Intent.Plan{}, err
	}
	if !diff.Compilable {
		message := "Berimond blueprint preflight found an unsupported target"
		for _, issue := range append(diff.Normal.Issues, diff.Fixed.Issues...) {
			if issue.Severity == Buildings.TargetIssueError {
				message = issue.Message
				break
			}
		}
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%s", message), Localization.New("server.app.p.8af35f19", "{p0}", Localization.Params{"p0": fmt.Sprintf("%s", message)}))
	}
	now := time.Now().UTC()
	blueprint := Buildings.BerimondBlueprint{
		ID: Buildings.BerimondBlueprintID(diff.Target.Mode), Name: Buildings.BerimondBlueprintName(diff.Target.Mode),
		CreatedAt: now, UpdatedAt: now, Target: diff.Target,
	}
	canonical, _ := json.Marshal(beriBlueprintSaveAction{Blueprint: blueprint})
	return Intent.Plan{
		Claims: []string{"configuration:" + Buildings.BerimondBlueprintConfigurationSection},
		Summary: fmt.Sprintf(
			"Save and activate %s for Berimond camp %d (%d targets, %d planned actions)",
			blueprint.Name, blueprint.Target.CastleID, diff.TargetCount, diff.ActionCount,
		), SummaryDescriptor: Localization.New("server.app.save_and_activate_p.a0f583c2", "Save and activate {p0} for Berimond camp {p1} ({p2} targets, {p3} planned actions)", Localization.Params{"p0": fmt.Sprintf("%s", blueprint.Name), "p1": fmt.Sprintf("%d", blueprint.Target.CastleID), "p2": diff.TargetCount, "p3": diff.ActionCount}),
		Steps: []Intent.Step{{
			Name: "Save Berimond blueprint", Action: "beri.blueprint.save", ActionArguments: canonical,
		}},
	}, nil
}

func (application *Application) saveBeriBlueprint(_ context.Context, arguments json.RawMessage) error {
	var input beriBlueprintSaveAction
	if err := decodeIntentArguments(arguments, &input); err != nil {
		return err
	}
	input.Blueprint.ID = strings.TrimSpace(input.Blueprint.ID)
	if input.Blueprint.ID == "" {
		return Localization.WithError(fmt.Errorf("Berimond blueprint id is required"), Localization.New("server.app.berimond_blueprint_id_is.e7de48f0", "Berimond blueprint id is required", nil))
	}
	raw, _ := application.Configuration.Section(Buildings.BerimondBlueprintConfigurationSection)
	document, err := Buildings.DecodeBerimondBlueprintDocument(raw, nil)
	if err != nil {
		return err
	}
	if existing, found := document.Blueprints[input.Blueprint.ID]; found && !existing.CreatedAt.IsZero() {
		input.Blueprint.CreatedAt = existing.CreatedAt
	}
	if input.Blueprint.CreatedAt.IsZero() {
		input.Blueprint.CreatedAt = time.Now().UTC()
	}
	input.Blueprint.UpdatedAt = time.Now().UTC()
	document.Blueprints[input.Blueprint.ID] = input.Blueprint
	document.ActiveID = input.Blueprint.ID
	canonical, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = application.Configuration.Update(Buildings.BerimondBlueprintConfigurationSection, canonical)
	return err
}

func planBeriBlueprintActivate(
	_ context.Context,
	_ Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request beriBlueprintActivateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.ID = strings.TrimSpace(request.ID)
	canonical, _ := json.Marshal(request)
	summary := "Pause Berimond blueprint reconciliation"
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.pause_berimond_blueprint_reconciliation.02fc8be0", "Pause Berimond blueprint reconciliation", nil)
	if request.ID != "" {
		summary = fmt.Sprintf("Activate Berimond blueprint %s", request.ID)
		summaryLocalizationMessage = Localization.New("server.app.activate_berimond_blueprint_p.498827f1", "Activate Berimond blueprint {p0}", Localization.Params{"p0": fmt.Sprintf("%s", request.ID)})
	}
	return Intent.Plan{
		Claims:  []string{"configuration:" + Buildings.BerimondBlueprintConfigurationSection},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: []Intent.Step{{
			Name: "Select Berimond blueprint", Action: "beri.blueprint.activate", ActionArguments: canonical,
		}},
	}, nil
}

func (application *Application) activateBeriBlueprint(_ context.Context, arguments json.RawMessage) error {
	var request beriBlueprintActivateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	request.ID = strings.TrimSpace(request.ID)
	raw, _ := application.Configuration.Section(Buildings.BerimondBlueprintConfigurationSection)
	document, err := Buildings.DecodeBerimondBlueprintDocument(raw, nil)
	if err != nil {
		return err
	}
	if request.ID != "" {
		if _, exists := document.Blueprints[request.ID]; !exists {
			return Localization.WithError(fmt.Errorf("Berimond blueprint %q does not exist", request.ID), Localization.New("server.app.berimond_blueprint_p_does.8cf4d4b9", "Berimond blueprint {p0} does not exist", Localization.Params{"p0": fmt.Sprintf("%q", request.ID)}))
		}
	}
	document.ActiveID = request.ID
	canonical, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = application.Configuration.Update(Buildings.BerimondBlueprintConfigurationSection, canonical)
	return err
}
