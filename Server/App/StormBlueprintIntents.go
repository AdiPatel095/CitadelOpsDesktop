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

type stormBlueprintSaveRequest struct {
	Target Buildings.TargetCaptureResult `json:"target"`
	Policy Buildings.TargetDiffPolicy    `json:"policy"`
}

type stormBlueprintSaveAction struct {
	Blueprint Buildings.StormBlueprint `json:"blueprint"`
}

type stormBlueprintActivateRequest struct {
	ID string `json:"id"`
}

func (application *Application) registerStormBlueprintIntents() error {
	for name, action := range map[string]Intent.Action{
		"storm.blueprint.save":     application.saveStormBlueprint,
		"storm.blueprint.activate": application.activateStormBlueprint,
	} {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}
	for _, definition := range []Intent.Definition{
		{
			Name: "storm.blueprint.save", Description: "Preflight and save one durable Storm castle blueprint without replacing other capture modes", DescriptionDescriptor: Localization.New("server.intent.description.047d42e6", "Preflight and save one durable Storm castle blueprint without replacing other capture modes", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"target":{"version":1,"castleId":5358,"kingdomId":4,"mode":"functional","ground":[],"buildings":[],"fixed":[],"summary":{}},"policy":{"allowPremium":false,"resourceReserves":{}}}`),
			Planner:          planStormBlueprintSave,
		},
		{
			Name: "storm.blueprint.activate", Description: "Activate a saved Storm blueprint or pause blueprint reconciliation without deleting it", DescriptionDescriptor: Localization.New("server.intent.description.d747fb0d", "Activate a saved Storm blueprint or pause blueprint reconciliation without deleting it", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"id":"storm-functional"}`), Planner: planStormBlueprintActivate,
		},
	} {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planStormBlueprintSave(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request stormBlueprintSaveRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.Target.KingdomID != State.KingdomID(GameData.StormKingdomID) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Storm blueprint must target kingdom %d", GameData.StormKingdomID), Localization.New("server.app.storm_blueprint_must_target.5a092177", "Storm blueprint must target kingdom {p0}", Localization.Params{"p0": fmt.Sprintf("%d", GameData.StormKingdomID)}))
	}
	diff, err := Buildings.CompileBlueprintDiff(input.State, input.GameData, Buildings.BlueprintDiffRequest{
		Target: request.Target, Policy: request.Policy,
	})
	if err != nil {
		return Intent.Plan{}, err
	}
	if !diff.Compilable {
		message := "Storm blueprint preflight found an unsupported target"
		for _, issue := range append(diff.Normal.Issues, diff.Fixed.Issues...) {
			if issue.Severity == Buildings.TargetIssueError {
				message = issue.Message
				break
			}
		}
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%s", message), Localization.New("server.app.p.8af35f19", "{p0}", Localization.Params{"p0": fmt.Sprintf("%s", message)}))
	}
	now := time.Now().UTC()
	blueprint := Buildings.StormBlueprint{
		ID: Buildings.StormBlueprintID(diff.Target.Mode), Name: Buildings.StormBlueprintName(diff.Target.Mode),
		CreatedAt: now, UpdatedAt: now, Target: diff.Target,
	}
	canonical, _ := json.Marshal(stormBlueprintSaveAction{Blueprint: blueprint})
	return Intent.Plan{
		Claims: []string{"configuration:" + Buildings.StormBlueprintConfigurationSection},
		Summary: fmt.Sprintf(
			"Save and activate %s for Storm castle %d (%d targets, %d planned actions)",
			blueprint.Name, blueprint.Target.CastleID, diff.TargetCount, diff.ActionCount,
		), SummaryDescriptor: Localization.New("server.app.save_and_activate_p.ea88e1ae", "Save and activate {p0} for Storm castle {p1} ({p2} targets, {p3} planned actions)", Localization.Params{"p0": fmt.Sprintf("%s", blueprint.Name), "p1": fmt.Sprintf("%d", blueprint.Target.CastleID), "p2": diff.TargetCount, "p3": diff.ActionCount}),
		Steps: []Intent.Step{{
			Name: "Save Storm blueprint", Action: "storm.blueprint.save", ActionArguments: canonical,
		}},
	}, nil
}

func (application *Application) saveStormBlueprint(_ context.Context, arguments json.RawMessage) error {
	var input stormBlueprintSaveAction
	if err := decodeIntentArguments(arguments, &input); err != nil {
		return err
	}
	input.Blueprint.ID = strings.TrimSpace(input.Blueprint.ID)
	if input.Blueprint.ID == "" {
		return Localization.WithError(fmt.Errorf("Storm blueprint id is required"), Localization.New("server.app.storm_blueprint_id_is.988208bb", "Storm blueprint id is required", nil))
	}
	raw, _ := application.Configuration.Section(Buildings.StormBlueprintConfigurationSection)
	document, err := Buildings.DecodeStormBlueprintDocument(raw, nil)
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
	_, err = application.Configuration.Update(Buildings.StormBlueprintConfigurationSection, canonical)
	return err
}

func planStormBlueprintActivate(
	_ context.Context,
	_ Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request stormBlueprintActivateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.ID = strings.TrimSpace(request.ID)
	canonical, _ := json.Marshal(request)
	summary := "Pause Storm blueprint reconciliation"
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.pause_storm_blueprint_reconciliation.8dcb6d0d", "Pause Storm blueprint reconciliation", nil)
	if request.ID != "" {
		summary = fmt.Sprintf("Activate Storm blueprint %s", request.ID)
		summaryLocalizationMessage = Localization.New("server.app.activate_storm_blueprint_p.73ea1671", "Activate Storm blueprint {p0}", Localization.Params{"p0": fmt.Sprintf("%s", request.ID)})
	}
	return Intent.Plan{
		Claims:  []string{"configuration:" + Buildings.StormBlueprintConfigurationSection},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: []Intent.Step{{
			Name: "Select Storm blueprint", Action: "storm.blueprint.activate", ActionArguments: canonical,
		}},
	}, nil
}

func (application *Application) activateStormBlueprint(_ context.Context, arguments json.RawMessage) error {
	var request stormBlueprintActivateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	request.ID = strings.TrimSpace(request.ID)
	raw, _ := application.Configuration.Section(Buildings.StormBlueprintConfigurationSection)
	document, err := Buildings.DecodeStormBlueprintDocument(raw, nil)
	if err != nil {
		return err
	}
	if request.ID != "" {
		if _, exists := document.Blueprints[request.ID]; !exists {
			return Localization.WithError(fmt.Errorf("Storm blueprint %q does not exist", request.ID), Localization.New("server.app.storm_blueprint_p_does.9a259c66", "Storm blueprint {p0} does not exist", Localization.Params{"p0": fmt.Sprintf("%q", request.ID)}))
		}
	}
	document.ActiveID = request.ID
	canonical, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = application.Configuration.Update(Buildings.StormBlueprintConfigurationSection, canonical)
	return err
}
