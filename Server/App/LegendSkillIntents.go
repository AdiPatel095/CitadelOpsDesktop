package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const legendSkillFreshness = 5 * time.Minute

type legendSkillPurchaseRequest struct {
	SkillID int64 `json:"skillId"`
}

type legendSkillDefinition struct {
	ID              int64
	TreeID          int64
	GroupID         int64
	RequiredSkillID int64
	Cost            int64
	TotalCost       int64
	SpecialType     string
}

func (application *Application) registerLegendSkillIntents() error {
	if err := application.Intents.RegisterStepResolver("legend.skill.purchase.build", resolveLegendSkillPurchaseStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("legend.skills.reset.build", resolveLegendSkillsResetStep); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name: "legend.skills.refresh", Description: "Refresh Hall of Legends allocations, reset state, and sovereign skill state", DescriptionDescriptor: Localization.New("server.intent.description.84be481f", "Refresh Hall of Legends allocations, reset state, and sovereign skill state", nil), Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{}`), Planner: planLegendSkillsRefresh,
		},
		{
			Name: "general.skills.refresh", Description: "Refresh every owned general's active skills (the game never volunteers them; attack capacity needs them)", DescriptionDescriptor: Localization.New("server.intent.description.21804d05", "Refresh every owned general's active skills (the game never volunteers them; attack capacity needs them)", nil), Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{}`), Planner: planGeneralSkillsRefresh,
		},
		{
			Name: "legend.skill.purchase", Description: "Spend Hall of Legends skill points on the next official skill-group level", DescriptionDescriptor: Localization.New("server.intent.description.82958d03", "Spend Hall of Legends skill points on the next official skill-group level", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"skillId":11}`), Planner: planLegendSkillPurchase,
		},
		{
			Name: "legend.skills.reset", Description: "Reset Hall of Legends allocations only when the live reset timer confirms it is free", DescriptionDescriptor: Localization.New("server.intent.description.7b64eb88", "Reset Hall of Legends allocations only when the live reset timer confirms it is free", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{}`), Planner: planLegendSkillsReset,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planLegendSkillsRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims:  []string{"hall-of-legends"},
		Summary: "Refresh Hall of Legends state", SummaryDescriptor: Localization.New("server.app.refresh_hall_of_legends.5515786e", "Refresh Hall of Legends state", nil),
		Steps: []Intent.Step{commandStep("Refresh Hall of Legends", "skl", json.RawMessage(`{}`), "skl", Localization.New("server.app.refresh_hall_of_legends.8b3a8f86", "Refresh Hall of Legends", nil))},
	}, nil
}

// planGeneralSkillsRefresh pulls the general roster with active skills (C2S
// "gie"). Attack-capacity resolution needs a commander's general skills, but
// the game only sends them on request — the official client asks at login
// and the attack plans ask again when stale. A policy whose commander has an
// unobserved general cannot plan the attack that would ask, so it schedules
// this refresh instead of waiting forever.
func planGeneralSkillsRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims:  []string{"generals"},
		Summary: "Refresh general skills", SummaryDescriptor: Localization.New("server.app.refresh_general_skills.b215c64b", "Refresh general skills", nil),
		Steps: []Intent.Step{commandStep(
			"Refresh commander general attack limits", "gie", json.RawMessage(`{}`), "gie", Localization.New("server.app.refresh_commander_general_attack.1ffe2e87", "Refresh commander general attack limits", nil),
		)},
	}, nil
}

func planLegendSkillPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request legendSkillPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	definition, err := legendSkillDefinitionForID(input.GameData, request.SkillID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if definition.SpecialType != "" || definition.Cost <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Hall of Legends skill %d is granted automatically and cannot be purchased directly", request.SkillID), Localization.New("server.app.hall_of_legends_skill.076c65f3", "Hall of Legends skill {p0} is granted automatically and cannot be purchased directly", Localization.Params{"p0": fmt.Sprintf("%d", request.SkillID)}))
	}
	now := time.Now().UTC()
	needsRefresh := legendSkillStateNeedsRefresh(input.State.Player.LegendSkills, now)
	if !needsRefresh {
		if _, err := validatedLegendSkillPurchase(input, request); err != nil {
			return Intent.Plan{}, err
		}
	}
	resolverArguments, _ := json.Marshal(request)
	steps := make([]Intent.Step, 0, 2)
	if needsRefresh {
		steps = append(steps, contextCommandStep("Refresh Hall of Legends", "skl", json.RawMessage(`{}`), "skl"))
	}
	steps = append(steps, Intent.Step{
		Name: "Purchase Hall of Legends skill", NameDescriptor: Localization.New("server.app.purchase_hall_of_legends.98ed785e", "Purchase Hall of Legends skill", nil), Resolver: "legend.skill.purchase.build", ResolverArguments: resolverArguments,
		AwaitOpcode: "skp", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	return Intent.Plan{
		Claims:  []string{"hall-of-legends"},
		Summary: fmt.Sprintf("Purchase Hall of Legends skill %d", request.SkillID), SummaryDescriptor: Localization.New("server.app.purchase_hall_of_legends.ba758342", "Purchase Hall of Legends skill {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.SkillID)}),
		Steps: steps,
	}, nil
}

func resolveLegendSkillPurchaseStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request legendSkillPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, err := validatedLegendSkillPurchase(input, request); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		SkillID int64 `json:"ID"`
	}{request.SkillID})
	return commandStep("Purchase Hall of Legends skill", "skp", payload, "skp", Localization.New("server.app.purchase_hall_of_legends.98ed785e", "Purchase Hall of Legends skill", nil)), nil
}

func validatedLegendSkillPurchase(input Intent.PlanningContext, request legendSkillPurchaseRequest) (legendSkillDefinition, error) {
	requested, err := legendSkillDefinitionForID(input.GameData, request.SkillID)
	if err != nil {
		return legendSkillDefinition{}, err
	}
	if requested.SpecialType != "" || requested.Cost <= 0 {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends skill %d is granted automatically and cannot be purchased directly", request.SkillID), Localization.New("server.app.hall_of_legends_skill.076c65f3", "Hall of Legends skill {p0} is granted automatically and cannot be purchased directly", Localization.Params{"p0": fmt.Sprintf("%d", request.SkillID)}))
	}
	state := input.State.Player.LegendSkills
	if state.ObservedAt.IsZero() || state.SkillPoints <= 0 {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends state must be refreshed before purchasing a skill"), Localization.New("server.app.hall_of_legends_state.2e4fdd90", "Hall of Legends state must be refreshed before purchasing a skill", nil))
	}
	var activeGroupID int64
	spent := int64(0)
	for _, activeID := range state.ActiveIDs {
		active, activeErr := legendSkillDefinitionForID(input.GameData, activeID)
		if activeErr != nil {
			return legendSkillDefinition{}, activeErr
		}
		spent += max(active.TotalCost, 0)
		if active.TreeID == requested.TreeID && active.GroupID == requested.GroupID {
			if activeGroupID != 0 && activeGroupID != active.ID {
				return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends group %d has multiple active levels", requested.GroupID), Localization.New("server.app.hall_of_legends_group.023dd450", "Hall of Legends group {p0} has multiple active levels", Localization.Params{"p0": fmt.Sprintf("%d", requested.GroupID)}))
			}
			activeGroupID = active.ID
		}
	}
	if activeGroupID == requested.ID {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends skill %d is already active", requested.ID), Localization.New("server.app.hall_of_legends_skill.f7e34f13", "Hall of Legends skill {p0} is already active", Localization.Params{"p0": fmt.Sprintf("%d", requested.ID)}))
	}
	if requested.RequiredSkillID > 0 && activeGroupID != requested.RequiredSkillID {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends skill %d requires active skill %d", requested.ID, requested.RequiredSkillID), Localization.New("server.app.hall_of_legends_skill.7e87d67c", "Hall of Legends skill {p0} requires active skill {p1}", Localization.Params{"p0": fmt.Sprintf("%d", requested.ID), "p1": fmt.Sprintf("%d", requested.RequiredSkillID)}))
	}
	if requested.RequiredSkillID == 0 && activeGroupID != 0 {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends group %d is already advanced to skill %d", requested.GroupID, activeGroupID), Localization.New("server.app.hall_of_legends_group.17f099b3", "Hall of Legends group {p0} is already advanced to skill {p1}", Localization.Params{"p0": fmt.Sprintf("%d", requested.GroupID), "p1": fmt.Sprintf("%d", activeGroupID)}))
	}
	if spent+requested.Cost > state.SkillPoints {
		return legendSkillDefinition{}, fmt.Errorf(
			"Hall of Legends skill %d requires %d points but only %d remain",
			requested.ID, requested.Cost, max(state.SkillPoints-spent, 0),
		)
	}
	return requested, nil
}

func planLegendSkillsReset(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	now := time.Now().UTC()
	needsRefresh := legendSkillStateNeedsRefresh(input.State.Player.LegendSkills, now)
	if !needsRefresh {
		if err := validateFreeLegendSkillReset(input.State.Player.LegendSkills, now); err != nil {
			return Intent.Plan{}, err
		}
	}
	steps := make([]Intent.Step, 0, 3)
	if needsRefresh {
		steps = append(steps, contextCommandStep("Refresh Hall of Legends", "skl", json.RawMessage(`{}`), "skl"))
	}
	steps = append(steps,
		Intent.Step{
			Name: "Reset Hall of Legends skills", NameDescriptor: Localization.New("server.app.reset_hall_of_legends.f196d739", "Reset Hall of Legends skills", nil), Resolver: "legend.skills.reset.build", ResolverArguments: json.RawMessage(`{}`),
			AwaitOpcode: "skr", TimeoutMillis: 10_000, SuccessCodes: []int{0},
		},
		contextCommandStep("Refresh reset Hall of Legends", "skl", json.RawMessage(`{}`), "skl"),
	)
	return Intent.Plan{
		Claims:  []string{"hall-of-legends"},
		Summary: "Reset Hall of Legends skills during the free reset window", SummaryDescriptor: Localization.New("server.app.reset_hall_of_legends.720e9e1a", "Reset Hall of Legends skills during the free reset window", nil),
		Steps: steps,
	}, nil
}

func resolveLegendSkillsResetStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if err := validateFreeLegendSkillReset(input.State.Player.LegendSkills, time.Now().UTC()); err != nil {
		return Intent.Step{}, err
	}
	return commandStep("Reset Hall of Legends skills", "skr", json.RawMessage(`{}`), "skr", Localization.New("server.app.reset_hall_of_legends.f196d739", "Reset Hall of Legends skills", nil)), nil
}

func validateFreeLegendSkillReset(state State.LegendSkillState, evaluatedAt time.Time) error {
	if state.ObservedAt.IsZero() || state.SkillPoints <= 0 {
		return Localization.WithError(fmt.Errorf("Hall of Legends state must be refreshed before resetting skills"), Localization.New("server.app.hall_of_legends_state.bee45d04", "Hall of Legends state must be refreshed before resetting skills", nil))
	}
	if len(state.ActiveIDs) == 0 {
		return Localization.WithError(fmt.Errorf("Hall of Legends has no allocated skills to reset"), Localization.New("server.app.hall_of_legends_has.62616213", "Hall of Legends has no allocated skills to reset", nil))
	}
	if remainingSec := legendSkillResetRemainingSec(state, evaluatedAt); remainingSec > 0 {
		return Localization.WithError(fmt.Errorf("the next free Hall of Legends reset is available in %d seconds", remainingSec), Localization.New("server.app.the_next_free_hall.c10b7797", "the next free Hall of Legends reset is available in {p0} seconds", Localization.Params{"p0": remainingSec}))
	}
	return nil
}

func legendSkillDefinitionForID(gameData *GameData.Store, skillID int64) (legendSkillDefinition, error) {
	if gameData == nil || skillID <= 0 {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("skillId must reference the loaded official Hall of Legends catalog"), Localization.New("server.app.skillid_must_reference_the.848d9f06", "skillId must reference the loaded official Hall of Legends catalog", nil))
	}
	catalog, err := gameData.Catalog("legendskills")
	if err != nil {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("load Hall of Legends skill catalog: %w", err), Localization.ErrorContext(Localization.New("server.app.load_hall_of_legends.bcad1994", "load Hall of Legends skill catalog", nil), err))
	}
	raw, exists := catalog.Find(strconv.FormatInt(skillID, 10))
	if !exists {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends skill %d is not in the current official catalog", skillID), Localization.New("server.app.hall_of_legends_skill.50462e80", "Hall of Legends skill {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", skillID)}))
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("decode Hall of Legends skill %d: %w", skillID, err), Localization.ErrorContext(Localization.New("server.app.decode_hall_of_legends.77f1f064", "decode Hall of Legends skill {p0}", Localization.Params{"p0": fmt.Sprintf("%d", skillID)}), err))
	}
	treeID, treeExists := record.Int64("skillTreeID")
	groupID, groupExists := record.Int64("skillGroupID")
	if !treeExists || !groupExists {
		return legendSkillDefinition{}, Localization.WithError(fmt.Errorf("Hall of Legends skill %d has no tree/group identity", skillID), Localization.New("server.app.hall_of_legends_skill.19dd2520", "Hall of Legends skill {p0} has no tree/group identity", Localization.Params{"p0": fmt.Sprintf("%d", skillID)}))
	}
	definition := legendSkillDefinition{ID: skillID, TreeID: treeID, GroupID: groupID}
	definition.RequiredSkillID, _ = record.Int64("requiredSkillID")
	definition.Cost, _ = record.Int64("costSkillPoints")
	definition.TotalCost, _ = record.Int64("totalCostSkillPoints")
	definition.SpecialType, _ = record.String("specialType")
	return definition, nil
}

func legendSkillStateNeedsRefresh(state State.LegendSkillState, evaluatedAt time.Time) bool {
	if state.ObservedAt.IsZero() || state.SkillPoints <= 0 {
		return true
	}
	age := evaluatedAt.Sub(state.ObservedAt)
	return age >= legendSkillFreshness
}

func legendSkillResetRemainingSec(state State.LegendSkillState, evaluatedAt time.Time) int64 {
	elapsed := evaluatedAt.Sub(state.ObservedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	return max(state.ResetRemainingSec-int64(elapsed/time.Second), 0)
}
