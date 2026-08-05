package App

import (
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
			Name: "beri.blueprint.save", Description: "Preflight and save one durable Berimond camp blueprint without replacing other capture modes", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"target":{"version":1,"castleId":901,"kingdomId":10,"mode":"functional","ground":[],"buildings":[],"fixed":[],"summary":{}},"policy":{"allowPremium":false,"resourceReserves":{}}}`),
			Planner:          planBeriBlueprintSave,
		},
		{
			Name: "beri.blueprint.activate", Description: "Activate a saved Berimond blueprint or pause blueprint reconciliation without deleting it", Effect: Intent.EffectWrite,
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
		return Intent.Plan{}, fmt.Errorf("Berimond blueprint must target kingdom %d", GameData.BerimondKingdomID)
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
		return Intent.Plan{}, fmt.Errorf("%s", message)
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
		),
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
		return fmt.Errorf("Berimond blueprint id is required")
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
	if request.ID != "" {
		summary = fmt.Sprintf("Activate Berimond blueprint %s", request.ID)
	}
	return Intent.Plan{
		Claims:  []string{"configuration:" + Buildings.BerimondBlueprintConfigurationSection},
		Summary: summary,
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
			return fmt.Errorf("Berimond blueprint %q does not exist", request.ID)
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
