package App

import (
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
			Name: "legend.skills.refresh", Description: "Refresh Hall of Legends allocations, reset state, and sovereign skill state", Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{}`), Planner: planLegendSkillsRefresh,
		},
		{
			Name: "legend.skill.purchase", Description: "Spend Hall of Legends skill points on the next official skill-group level", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"skillId":11}`), Planner: planLegendSkillPurchase,
		},
		{
			Name: "legend.skills.reset", Description: "Reset Hall of Legends allocations only when the live reset timer confirms it is free", Effect: Intent.EffectWrite,
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
		Summary: "Refresh Hall of Legends state",
		Steps:   []Intent.Step{commandStep("Refresh Hall of Legends", "skl", json.RawMessage(`{}`), "skl")},
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
		return Intent.Plan{}, fmt.Errorf("Hall of Legends skill %d is granted automatically and cannot be purchased directly", request.SkillID)
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
		Name: "Purchase Hall of Legends skill", Resolver: "legend.skill.purchase.build", ResolverArguments: resolverArguments,
		AwaitOpcode: "skp", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	return Intent.Plan{
		Claims:  []string{"hall-of-legends"},
		Summary: fmt.Sprintf("Purchase Hall of Legends skill %d", request.SkillID),
		Steps:   steps,
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
	return commandStep("Purchase Hall of Legends skill", "skp", payload, "skp"), nil
}

func validatedLegendSkillPurchase(input Intent.PlanningContext, request legendSkillPurchaseRequest) (legendSkillDefinition, error) {
	requested, err := legendSkillDefinitionForID(input.GameData, request.SkillID)
	if err != nil {
		return legendSkillDefinition{}, err
	}
	if requested.SpecialType != "" || requested.Cost <= 0 {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends skill %d is granted automatically and cannot be purchased directly", request.SkillID)
	}
	state := input.State.Player.LegendSkills
	if state.ObservedAt.IsZero() || state.SkillPoints <= 0 {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends state must be refreshed before purchasing a skill")
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
				return legendSkillDefinition{}, fmt.Errorf("Hall of Legends group %d has multiple active levels", requested.GroupID)
			}
			activeGroupID = active.ID
		}
	}
	if activeGroupID == requested.ID {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends skill %d is already active", requested.ID)
	}
	if requested.RequiredSkillID > 0 && activeGroupID != requested.RequiredSkillID {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends skill %d requires active skill %d", requested.ID, requested.RequiredSkillID)
	}
	if requested.RequiredSkillID == 0 && activeGroupID != 0 {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends group %d is already advanced to skill %d", requested.GroupID, activeGroupID)
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
			Name: "Reset Hall of Legends skills", Resolver: "legend.skills.reset.build", ResolverArguments: json.RawMessage(`{}`),
			AwaitOpcode: "skr", TimeoutMillis: 10_000, SuccessCodes: []int{0},
		},
		contextCommandStep("Refresh reset Hall of Legends", "skl", json.RawMessage(`{}`), "skl"),
	)
	return Intent.Plan{
		Claims:  []string{"hall-of-legends"},
		Summary: "Reset Hall of Legends skills during the free reset window",
		Steps:   steps,
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
	return commandStep("Reset Hall of Legends skills", "skr", json.RawMessage(`{}`), "skr"), nil
}

func validateFreeLegendSkillReset(state State.LegendSkillState, evaluatedAt time.Time) error {
	if state.ObservedAt.IsZero() || state.SkillPoints <= 0 {
		return fmt.Errorf("Hall of Legends state must be refreshed before resetting skills")
	}
	if len(state.ActiveIDs) == 0 {
		return fmt.Errorf("Hall of Legends has no allocated skills to reset")
	}
	if remainingSec := legendSkillResetRemainingSec(state, evaluatedAt); remainingSec > 0 {
		return fmt.Errorf("the next free Hall of Legends reset is available in %d seconds", remainingSec)
	}
	return nil
}

func legendSkillDefinitionForID(gameData *GameData.Store, skillID int64) (legendSkillDefinition, error) {
	if gameData == nil || skillID <= 0 {
		return legendSkillDefinition{}, fmt.Errorf("skillId must reference the loaded official Hall of Legends catalog")
	}
	catalog, err := gameData.Catalog("legendskills")
	if err != nil {
		return legendSkillDefinition{}, fmt.Errorf("load Hall of Legends skill catalog: %w", err)
	}
	raw, exists := catalog.Find(strconv.FormatInt(skillID, 10))
	if !exists {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends skill %d is not in the current official catalog", skillID)
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return legendSkillDefinition{}, fmt.Errorf("decode Hall of Legends skill %d: %w", skillID, err)
	}
	treeID, treeExists := record.Int64("skillTreeID")
	groupID, groupExists := record.Int64("skillGroupID")
	if !treeExists || !groupExists {
		return legendSkillDefinition{}, fmt.Errorf("Hall of Legends skill %d has no tree/group identity", skillID)
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
