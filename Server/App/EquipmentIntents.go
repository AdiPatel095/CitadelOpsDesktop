package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	maxEquipmentUpgradeLevel  = 50
	defaultEquipmentStepDelay = 50
	equipmentRubyFreshness    = 2 * time.Minute
)

var baseEquipmentSlots = []int{1, 2, 3, 4, 6}

type resolvedLeader struct {
	kind      string
	id        int64
	available bool
	equipment map[string]State.EquipmentInstanceID
	gems      map[string]State.GemInstanceID
}

type equipmentReconfigureGemVerification struct {
	InstanceID   State.GemInstanceID `json:"instanceId"`
	DefinitionID State.GemID         `json:"definitionId"`
	Normal       bool                `json:"normal,omitempty"`
}

type equipmentReconfigureVerification struct {
	LeaderKind string                                         `json:"leaderKind"`
	LeaderID   int64                                          `json:"leaderId"`
	Equipment  map[string]State.EquipmentInstanceID           `json:"equipment"`
	Gems       map[string]equipmentReconfigureGemVerification `json:"gems"`
}

type equipmentReconfigureRequest struct {
	LeaderKind          string                               `json:"leaderKind"`
	LeaderID            int64                                `json:"leaderId"`
	CombatMode          string                               `json:"combatMode,omitempty"`
	SnapshotFingerprint string                               `json:"snapshotFingerprint,omitempty"`
	QuoteFingerprint    string                               `json:"quoteFingerprint,omitempty"`
	MaximumRubySpend    int64                                `json:"maximumRubySpend,omitempty"`
	Equipment           map[string]State.EquipmentInstanceID `json:"equipment"`
	Gems                map[string]State.GemInstanceID       `json:"gems"`
}

type equipmentExtractionDispatch struct {
	Request                      equipmentReconfigureRequest     `json:"request"`
	InitialQuote                 EquipmentDomain.ExtractionQuote `json:"initialQuote"`
	GemID                        State.GemInstanceID             `json:"gemId"`
	CarrierID                    State.EquipmentInstanceID       `json:"carrierId"`
	RubyCost                     int64                           `json:"rubyCost"`
	ExpectedRemainingRubySpend   int64                           `json:"expectedRemainingRubySpend"`
	ExpectedConnectionGeneration uint64                          `json:"expectedConnectionGeneration"`
	ExpectedCatalogDigest        string                          `json:"expectedCatalogDigest"`
	RubyResourceID               State.ResourceID                `json:"rubyResourceId"`
	PlanningRubyObservedAt       time.Time                       `json:"planningRubyObservedAt"`
	RequireNewRubyObservation    bool                            `json:"requireNewRubyObservation,omitempty"`
	RemainingPaidExtractions     []equipmentPaidExtraction       `json:"remainingPaidExtractions"`
}

type equipmentPaidExtraction struct {
	GemID        State.GemInstanceID       `json:"gemId"`
	CarrierID    State.EquipmentInstanceID `json:"carrierId"`
	DefinitionID State.GemID               `json:"definitionId"`
	Level        int                       `json:"level"`
	RubyCost     int64                     `json:"rubyCost"`
}

type equipmentFreeExtractionDispatch struct {
	LeaderKind            string                    `json:"leaderKind"`
	LeaderID              int64                     `json:"leaderId"`
	GemID                 State.GemInstanceID       `json:"gemId"`
	CarrierID             State.EquipmentInstanceID `json:"carrierId"`
	DefinitionID          State.GemID               `json:"definitionId"`
	Level                 int                       `json:"level"`
	ExpectedCatalogDigest string                    `json:"expectedCatalogDigest"`
}

func planEquipmentRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims: []string{"game:equipment"}, Summary: "Refresh all equipment state", SummaryDescriptor: Localization.New("server.app.refresh_all_equipment_state.7ef5e20b", "Refresh all equipment state", nil),
		Steps: equipmentRefreshSteps(),
	}, nil
}

func planEquipmentEquip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LeaderKind  string                    `json:"leaderKind"`
		LeaderID    int64                     `json:"leaderId"`
		EquipmentID State.EquipmentInstanceID `json:"equipmentId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	leader, err := resolveLeader(input.State, request.LeaderKind, request.LeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is busy", leader.id), Localization.New("server.app.commander_p_is_busy.94a46299", "commander {p0} is busy", Localization.Params{"p0": fmt.Sprintf("%d", leader.id)}))
	}
	item, ok := input.State.Inventory.Equipment[request.EquipmentID]
	if !ok || request.EquipmentID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not in current storage", request.EquipmentID), Localization.New("server.app.equipment_p_is_not.1a3f3f4a", "equipment {p0} is not in current storage", Localization.Params{"p0": fmt.Sprintf("%d", request.EquipmentID)}))
	}
	if item.WearerKind != "" {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is already worn by %s %d", item.ID, item.WearerKind, item.WearerID), Localization.New("server.app.equipment_p_is_already.08136b11", "equipment {p0} is already worn by {p1} {p2}", Localization.Params{"p0": fmt.Sprintf("%d", item.ID), "p1": fmt.Sprintf("%s", item.WearerKind), "p2": fmt.Sprintf("%d", item.WearerID)}))
	}
	if !validBaseSlot(item.Slot) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d uses unsupported slot %d", item.ID, item.Slot), Localization.New("server.app.equipment_p_uses_unsupported.6300a0fa", "equipment {p0} uses unsupported slot {p1}", Localization.Params{"p0": fmt.Sprintf("%d", item.ID), "p1": item.Slot}))
	}
	if expectedEquipmentType(leader.kind) != item.TypeID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not compatible with a %s", item.ID, leader.kind), Localization.New("server.app.equipment_p_is_not.1190cbd2", "equipment {p0} is not compatible with a {p1}", Localization.Params{"p0": fmt.Sprintf("%d", item.ID), "p1": fmt.Sprintf("%s", leader.kind)}))
	}
	payload, _ := json.Marshal(struct {
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		LeaderID    int64                     `json:"LID"`
		Equip       int                       `json:"E"`
	}{item.ID, leader.id, 1})
	steps := []Intent.Step{commandStep("Equip equipment", "eeq", payload, "eeq", Localization.New("server.app.equip_equipment.3f0d313f", "Equip equipment", nil))}
	steps = append(steps, equipmentMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Equip item %d on %s %d", item.ID, leader.kind, leader.id), SummaryDescriptor: Localization.New("server.app.equip_item_p_on.ee744e8c", "Equip item {p0} on {p1} {p2}", Localization.Params{"p0": fmt.Sprintf("%d", item.ID), "p1": fmt.Sprintf("%s", leader.kind), "p2": fmt.Sprintf("%d", leader.id)}), Steps: steps,
	}, nil
}

func planEquipmentUnequip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LeaderKind   string                      `json:"leaderKind"`
		LeaderID     int64                       `json:"leaderId"`
		EquipmentIDs []State.EquipmentInstanceID `json:"equipmentIds"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	leader, err := resolveLeader(input.State, request.LeaderKind, request.LeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is busy", leader.id), Localization.New("server.app.commander_p_is_busy.94a46299", "commander {p0} is busy", Localization.Params{"p0": fmt.Sprintf("%d", leader.id)}))
	}
	ids := uniqueEquipmentIDs(request.EquipmentIDs)
	if len(ids) == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("at least one equipmentId is required"), Localization.New("server.app.at_least_one_equipmentid.04ca4db6", "at least one equipmentId is required", nil))
	}
	steps := make([]Intent.Step, 0, len(ids)+2)
	for _, id := range ids {
		item, ok := input.State.Inventory.Equipment[id]
		if !ok || item.WearerKind != leader.kind || item.WearerID != leader.id {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not worn by %s %d", id, leader.kind, leader.id), Localization.New("server.app.equipment_p_is_not.a5f6d4bc", "equipment {p0} is not worn by {p1} {p2}", Localization.Params{"p0": fmt.Sprintf("%d", id), "p1": fmt.Sprintf("%s", leader.kind), "p2": fmt.Sprintf("%d", leader.id)}))
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{id, leader.id, 0})
		step := commandStep(fmt.Sprintf("Unequip equipment %d", id), "eeq", payload, "eeq", Localization.New("server.app.unequip_equipment_p.db1f9e15", "Unequip equipment {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		steps = append(steps, step)
	}
	steps = append(steps, equipmentMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Unequip %d item(s) from %s %d", len(ids), leader.kind, leader.id), SummaryDescriptor: Localization.New("server.app.unequip_p_item_s.fa77cd88", "Unequip {p0} item(s) from {p1} {p2}", Localization.Params{"p0": fmt.Sprintf("%d", len(ids)), "p1": fmt.Sprintf("%s", leader.kind), "p2": fmt.Sprintf("%d", leader.id)}), Steps: steps,
	}, nil
}

func planGemEquip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LeaderKind  string                    `json:"leaderKind"`
		LeaderID    int64                     `json:"leaderId"`
		EquipmentID State.EquipmentInstanceID `json:"equipmentId"`
		GemID       State.GemInstanceID       `json:"gemId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	leader, err := resolveLeader(input.State, request.LeaderKind, request.LeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is busy", leader.id), Localization.New("server.app.commander_p_is_busy.94a46299", "commander {p0} is busy", Localization.Params{"p0": fmt.Sprintf("%d", leader.id)}))
	}
	item, ok := input.State.Inventory.Equipment[request.EquipmentID]
	if !ok || item.WearerKind != leader.kind || item.WearerID != leader.id {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not worn by %s %d", request.EquipmentID, leader.kind, leader.id), Localization.New("server.app.equipment_p_is_not.a5f6d4bc", "equipment {p0} is not worn by {p1} {p2}", Localization.Params{"p0": fmt.Sprintf("%d", request.EquipmentID), "p1": fmt.Sprintf("%s", leader.kind), "p2": fmt.Sprintf("%d", leader.id)}))
	}
	if leader.gems[strconv.Itoa(item.Slot)] != 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment slot %d already has a gem", item.Slot), Localization.New("server.app.equipment_slot_p_already.bc00b10c", "equipment slot {p0} already has a gem", Localization.Params{"p0": item.Slot}))
	}
	gem, ok := input.State.Inventory.Gems[request.GemID]
	if !ok || request.GemID <= 0 || gem.WearerKind != "" {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("relic gem %d is not in current storage", request.GemID), Localization.New("server.app.relic_gem_p_is.91b51e57", "relic gem {p0} is not in current storage", Localization.Params{"p0": fmt.Sprintf("%d", request.GemID)}))
	}
	payload, _ := json.Marshal(struct {
		GemID       State.GemInstanceID       `json:"GID"`
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		LeaderID    int64                     `json:"LID"`
		Mode        int                       `json:"M"`
		RelicGem    int                       `json:"RGEM"`
	}{gem.ID, item.ID, leader.id, 0, 1})
	steps := []Intent.Step{commandStep("Equip relic gem", "bge", payload, "bge", Localization.New("server.app.equip_relic_gem.353e7fe5", "Equip relic gem", nil))}
	steps = append(steps, gemMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Socket gem %d into equipment %d", gem.ID, item.ID), SummaryDescriptor: Localization.New("server.app.socket_gem_p_into.3eb6fbbd", "Socket gem {p0} into equipment {p1}", Localization.Params{"p0": fmt.Sprintf("%d", gem.ID), "p1": fmt.Sprintf("%d", item.ID)}), Steps: steps,
	}, nil
}

func planGemUnequip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LeaderKind  string                    `json:"leaderKind"`
		LeaderID    int64                     `json:"leaderId"`
		EquipmentID State.EquipmentInstanceID `json:"equipmentId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	leader, err := resolveLeader(input.State, request.LeaderKind, request.LeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is busy", leader.id), Localization.New("server.app.commander_p_is_busy.94a46299", "commander {p0} is busy", Localization.Params{"p0": fmt.Sprintf("%d", leader.id)}))
	}
	item, ok := input.State.Inventory.Equipment[request.EquipmentID]
	if !ok || item.WearerKind != leader.kind || item.WearerID != leader.id {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not worn by %s %d", request.EquipmentID, leader.kind, leader.id), Localization.New("server.app.equipment_p_is_not.a5f6d4bc", "equipment {p0} is not worn by {p1} {p2}", Localization.Params{"p0": fmt.Sprintf("%d", request.EquipmentID), "p1": fmt.Sprintf("%s", leader.kind), "p2": fmt.Sprintf("%d", leader.id)}))
	}
	if leader.gems[strconv.Itoa(item.Slot)] == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d has no observed gem", item.ID), Localization.New("server.app.equipment_p_has_no.11dd344e", "equipment {p0} has no observed gem", Localization.Params{"p0": fmt.Sprintf("%d", item.ID)}))
	}
	payload, _ := json.Marshal(struct {
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		LeaderID    int64                     `json:"LID"`
	}{item.ID, leader.id})
	steps := []Intent.Step{commandStep("Unequip gem", "ege", payload, "ege", Localization.New("server.app.unequip_gem.ceb0c9e0", "Unequip gem", nil))}
	steps = append(steps, gemMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Remove the gem from equipment %d", item.ID), SummaryDescriptor: Localization.New("server.app.remove_the_gem_from.8bdadfc4", "Remove the gem from equipment {p0}", Localization.Params{"p0": fmt.Sprintf("%d", item.ID)}), Steps: steps,
	}, nil
}

func planEquipmentSwap(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LeaderKind     string `json:"leaderKind"`
		FirstLeaderID  int64  `json:"firstLeaderId"`
		SecondLeaderID int64  `json:"secondLeaderId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	first, err := resolveLeader(input.State, request.LeaderKind, request.FirstLeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	second, err := resolveLeader(input.State, request.LeaderKind, request.SecondLeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if first.id == second.id {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("select two different %ss", first.kind), Localization.New("server.app.select_two_different_p.11549cfd", "select two different {p0}s", Localization.Params{"p0": fmt.Sprintf("%s", first.kind)}))
	}
	if !first.available || !second.available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("both commanders must be available"), Localization.New("server.app.both_commanders_must_be.02a3a243", "both commanders must be available", nil))
	}
	firstItems := leaderBaseEquipment(first)
	secondItems := leaderBaseEquipment(second)
	if len(firstItems)+len(secondItems) == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the selected leaders have no base equipment to swap"), Localization.New("server.app.the_selected_leaders_have.dcac1c96", "the selected leaders have no base equipment to swap", nil))
	}
	steps := make([]Intent.Step, 0, (len(firstItems)+len(secondItems))*2+2)
	for _, move := range []struct {
		leader resolvedLeader
		items  []State.EquipmentInstanceID
	}{{first, firstItems}, {second, secondItems}} {
		for _, id := range move.items {
			payload, _ := json.Marshal(struct {
				EquipmentID State.EquipmentInstanceID `json:"EID"`
				LeaderID    int64                     `json:"LID"`
				Equip       int                       `json:"E"`
			}{id, move.leader.id, 0})
			step := commandStep(fmt.Sprintf("Unequip equipment %d", id), "eeq", payload, "eeq", Localization.New("server.app.unequip_equipment_p.db1f9e15", "Unequip equipment {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
			steps = append(steps, step)
		}
	}
	for _, move := range []struct {
		leader resolvedLeader
		items  []State.EquipmentInstanceID
	}{{first, secondItems}, {second, firstItems}} {
		for _, id := range move.items {
			payload, _ := json.Marshal(struct {
				EquipmentID State.EquipmentInstanceID `json:"EID"`
				LeaderID    int64                     `json:"LID"`
				Equip       int                       `json:"E"`
			}{id, move.leader.id, 1})
			step := commandStep(fmt.Sprintf("Equip equipment %d", id), "eeq", payload, "eeq", Localization.New("server.app.equip_equipment_p.cd4a719e", "Equip equipment {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
			steps = append(steps, step)
		}
	}
	steps = append(steps, equipmentMutationRefreshSteps()...)
	claims := append(equipmentLeaderClaims(first), "leader:"+first.kind+":"+strconv.FormatInt(second.id, 10))
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Swap %d base equipment item(s) between %s %d and %s %d", len(firstItems)+len(secondItems), first.kind, first.id, second.kind, second.id), SummaryDescriptor: Localization.New("server.app.swap_p_base_equipment.d0f1afaf", "Swap {p0} base equipment item(s) between {p1} {p2} and {p3} {p4}", Localization.Params{"p0": len(firstItems) + len(secondItems), "p1": fmt.Sprintf("%s", first.kind), "p2": fmt.Sprintf("%d", first.id), "p3": fmt.Sprintf("%s", second.kind), "p4": fmt.Sprintf("%d", second.id)}),
		Steps: steps,
	}, nil
}

func planEquipmentReconfigure(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request equipmentReconfigureRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.MaximumRubySpend < 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("maximumRubySpend cannot be negative"), Localization.New("server.app.maximumrubyspend_cannot_be_negative.e9e10826", "maximumRubySpend cannot be negative", nil))
	}
	leader, err := resolveLeader(input.State, request.LeaderKind, request.LeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is busy", leader.id), Localization.New("server.app.commander_p_is_busy.94a46299", "commander {p0} is busy", Localization.Params{"p0": fmt.Sprintf("%d", leader.id)}))
	}
	if leader.kind == "commander" {
		commanderID := State.CommanderID(leader.id)
		now := time.Now().UTC()
		if State.CommanderHasActiveMovementAt(input.State, commanderID, now) ||
			input.CommanderHolds != nil && input.CommanderHolds.CommanderHeldAt(commanderID, now) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is travelling or reserved for a launch", leader.id), Localization.New("server.app.commander_p_is_travelling.ccd4f924", "commander {p0} is travelling or reserved for a launch", Localization.Params{"p0": fmt.Sprintf("%d", leader.id)}))
		}
	}
	if request.SnapshotFingerprint != "" {
		fingerprint, fingerprintErr := EquipmentDomain.SnapshotFingerprint(
			input.State, input.GameData, leader.kind, leader.id, request.CombatMode,
		)
		if fingerprintErr != nil {
			return Intent.Plan{}, fingerprintErr
		}
		if fingerprint != request.SnapshotFingerprint {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: equipment changed after this preview was generated", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a370dae3", "intent plan became stale before dispatch: equipment changed after this preview was generated", nil))
		}
	}
	selectedEquipment := map[State.EquipmentInstanceID]struct{}{}
	selectedItems := map[int]State.EquipmentInstance{}
	selectedFamily := EquipmentDomain.LoadoutFamilyUnknown
	for _, slot := range []int{1, 2, 3, 4} {
		id := request.Equipment[strconv.Itoa(slot)]
		if id <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("optimized loadout is missing equipment slot %d", slot), Localization.New("server.app.optimized_loadout_is_missing.696f256d", "optimized loadout is missing equipment slot {p0}", Localization.Params{"p0": slot}))
		}
	}
	for _, slot := range baseEquipmentSlots {
		rawSlot := strconv.Itoa(slot)
		id := request.Equipment[rawSlot]
		if id == 0 {
			if slot == 6 {
				continue
			}
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("optimized loadout is missing equipment slot %d", slot), Localization.New("server.app.optimized_loadout_is_missing.696f256d", "optimized loadout is missing equipment slot {p0}", Localization.Params{"p0": slot}))
		}
		item, ok := input.State.Inventory.Equipment[id]
		if !ok || item.Slot != slot || item.TypeID != expectedEquipmentType(leader.kind) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not valid for %s slot %d", id, leader.kind, slot), Localization.New("server.app.equipment_p_is_not.22a9f8ed", "equipment {p0} is not valid for {p1} slot {p2}", Localization.Params{"p0": fmt.Sprintf("%d", id), "p1": fmt.Sprintf("%s", leader.kind), "p2": slot}))
		}
		if item.WearerKind != "" && (item.WearerKind != leader.kind || item.WearerID != leader.id) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is worn by another leader", id), Localization.New("server.app.equipment_p_is_worn.904a7b6e", "equipment {p0} is worn by another leader", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		family := EquipmentDomain.EquipmentFamily(item)
		if family == EquipmentDomain.LoadoutFamilyUnknown {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d has no verified ordinary or relic classification", id), Localization.New("server.app.equipment_p_has_no.71752bfc", "equipment {p0} has no verified ordinary or relic classification", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		if selectedFamily != EquipmentDomain.LoadoutFamilyUnknown && family != selectedFamily {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("optimized loadout mixes ordinary and relic equipment"), Localization.New("server.app.optimized_loadout_mixes_ordinary.fa5ef964", "optimized loadout mixes ordinary and relic equipment", nil))
		}
		selectedFamily = family
		if _, duplicate := selectedEquipment[id]; duplicate {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d appears in more than one slot", id), Localization.New("server.app.equipment_p_appears_in.d6369ad6", "equipment {p0} appears in more than one slot", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		selectedEquipment[id] = struct{}{}
		selectedItems[slot] = item
	}
	appearanceFamily, appearanceRestrictsFamily, err := EquipmentDomain.RetainedAppearanceFamily(input.State, leader.equipment)
	if err != nil {
		return Intent.Plan{}, err
	}
	if appearanceRestrictsFamily && appearanceFamily != selectedFamily {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("gemmed appearance item prevents switching between ordinary and relic equipment"), Localization.New("server.app.gemmed_appearance_item_prevents.ca61d727", "gemmed appearance item prevents switching between ordinary and relic equipment", nil))
	}
	selectedGems := map[State.GemInstanceID]struct{}{}
	verification := equipmentReconfigureVerification{
		LeaderKind: leader.kind, LeaderID: leader.id, Equipment: cloneEquipmentSelection(request.Equipment),
		Gems: map[string]equipmentReconfigureGemVerification{},
	}
	for slot := 1; slot <= 4; slot++ {
		key := strconv.Itoa(slot)
		id := request.Gems[key]
		if id == 0 {
			continue
		}
		gem, ok := input.State.Inventory.Gems[id]
		if !ok {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("gem %d is not in current state", id), Localization.New("server.app.gem_p_is_not.8a3a97b2", "gem {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		if request.SnapshotFingerprint != "" && !EquipmentDomain.GemMatchesLeaderAndMode(
			input.State, gem, leader.kind, leader.id, strings.ToLower(strings.TrimSpace(request.CombatMode)),
		) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("gem %d is not compatible with this %s %s loadout", id, leader.kind, request.CombatMode), Localization.New("server.app.gem_p_is_not.f7267477", "gem {p0} is not compatible with this {p1} {p2} loadout", Localization.Params{"p0": fmt.Sprintf("%d", id), "p1": fmt.Sprintf("%s", leader.kind), "p2": fmt.Sprintf("%s", request.CombatMode)}))
		}
		if !EquipmentDomain.GemMatchesEquipmentFamily(gem, selectedItems[slot]) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("gem %d does not match equipment %d's ordinary or relic family", id, selectedItems[slot].ID), Localization.New("server.app.gem_p_does_not.36e0a081", "gem {p0} does not match equipment {p1}'s ordinary or relic family", Localization.Params{"p0": fmt.Sprintf("%d", id), "p1": fmt.Sprintf("%d", selectedItems[slot].ID)}))
		}
		if gem.WearerKind != "" && (gem.WearerKind != leader.kind || gem.WearerID != leader.id) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("gem %d is worn by another leader", id), Localization.New("server.app.gem_p_is_worn.15ca833c", "gem {p0} is worn by another leader", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		if _, duplicate := selectedGems[id]; duplicate {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("gem %d appears in more than one slot", id), Localization.New("server.app.gem_p_appears_in.88a0ce41", "gem {p0} appears in more than one slot", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		selectedGems[id] = struct{}{}
		verification.Gems[key] = equipmentReconfigureGemVerification{
			InstanceID: id, DefinitionID: gem.DefinitionID, Normal: id < 0,
		}
	}

	transition, err := EquipmentDomain.BuildReconfigurationTransition(
		input.State, leader.equipment, request.Equipment, request.Gems,
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	for _, gem := range transition.GemsToDetach {
		parent, found := input.State.Inventory.Equipment[gem.EquipmentInstanceID]
		if !found || parent.WearerKind != "" && (parent.WearerKind != leader.kind || parent.WearerID != leader.id) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("cannot detach gem %d from unavailable equipment %d", gem.ID, gem.EquipmentInstanceID), Localization.New("server.app.cannot_detach_gem_p.830edc7b", "cannot detach gem {p0} from unavailable equipment {p1}", Localization.Params{"p0": fmt.Sprintf("%d", gem.ID), "p1": fmt.Sprintf("%d", gem.EquipmentInstanceID)}))
		}
		if parent.Extraction != nil && parent.Extraction.GemID == gem.ID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: gem %d has an unresolved prior ruby extraction attempt", Intent.ErrPlanStale, gem.ID), Localization.New("server.app.intent_plan_became_stale.ac6f7858", "intent plan became stale before dispatch: gem {p1} has an unresolved prior ruby extraction attempt", Localization.Params{"p1": fmt.Sprintf("%d", gem.ID)}))
		}
	}
	quote, err := EquipmentDomain.QuoteReconfiguration(input.GameData, transition)
	if err != nil {
		return Intent.Plan{}, err
	}
	expectedQuoteFingerprint := EquipmentDomain.ReconfigurationQuoteFingerprint(
		request.SnapshotFingerprint, request.Equipment, request.Gems, quote,
	)
	if request.QuoteFingerprint != "" && request.QuoteFingerprint != expectedQuoteFingerprint {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the selected alternative or extraction quote changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.2244c1fa", "intent plan became stale before dispatch: the selected alternative or extraction quote changed", nil))
	}
	if quote.MaximumRubySpend > request.MaximumRubySpend {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("ruby extraction quote %d exceeds approved maximumRubySpend %d", quote.MaximumRubySpend, request.MaximumRubySpend), Localization.New("server.app.ruby_extraction_quote_p.9c0fc5a0", "ruby extraction quote {p0} exceeds approved maximumRubySpend {p1}", Localization.Params{"p0": quote.MaximumRubySpend, "p1": request.MaximumRubySpend}))
	}
	var rubyResourceID State.ResourceID
	var planningRubyObservedAt time.Time
	if quote.MaximumRubySpend > 0 {
		if request.SnapshotFingerprint == "" || request.QuoteFingerprint == "" {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("paid gem extraction requires the exact preview fingerprint and quote"), Localization.New("server.app.paid_gem_extraction_requires.d950bb27", "paid gem extraction requires the exact preview fingerprint and quote", nil))
		}
		resourceID, found := input.GameData.ResourceIDForJSONKey("C2")
		if !found || resourceID <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("official ruby resource is unavailable"), Localization.New("server.app.official_ruby_resource_is.d1b848a8", "official ruby resource is unavailable", nil))
		}
		rubyResourceID = State.ResourceID(resourceID)
		planningRubyObservedAt, err = validateEquipmentRubyAuthority(
			input.State, rubyResourceID, quote.MaximumRubySpend, time.Now().UTC(), time.Time{},
		)
		if err != nil {
			return Intent.Plan{}, err
		}
	}

	steps := make([]Intent.Step, 0, 32)
	mountedBySlot := map[int]State.EquipmentInstanceID{}
	for _, slot := range baseEquipmentSlots {
		id := leader.equipment[strconv.Itoa(slot)]
		if id <= 0 {
			continue
		}
		if !transition.ClearSlots[slot] {
			mountedBySlot[slot] = id
			continue
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{id, leader.id, 0})
		step := commandStep(fmt.Sprintf("Clear equipment slot %d", slot), "eeq", payload, "eeq", Localization.New("server.app.clear_equipment_slot_p.e6f28390", "Clear equipment slot {p0, number}", Localization.Params{"p0": slot}))
		steps = append(steps, step)
	}
	detachIDs := make([]State.GemInstanceID, 0, len(transition.GemsToDetach))
	for id := range transition.GemsToDetach {
		detachIDs = append(detachIDs, id)
	}
	sort.Slice(detachIDs, func(left, right int) bool { return detachIDs[left] < detachIDs[right] })
	paidExtractions := make([]equipmentPaidExtraction, 0, quote.RubyExtractionCount)
	for _, gemID := range detachIDs {
		gem := transition.GemsToDetach[gemID]
		if EquipmentDomain.GemFamily(gem) != EquipmentDomain.LoadoutFamilyOrdinary {
			continue
		}
		rubyCost, costErr := EquipmentDomain.NormalGemRemovalCost(input.GameData, gem)
		if costErr != nil {
			return Intent.Plan{}, costErr
		}
		if rubyCost > 0 {
			paidExtractions = append(paidExtractions, equipmentPaidExtraction{
				GemID: gem.ID, CarrierID: gem.EquipmentInstanceID, DefinitionID: gem.DefinitionID,
				Level: gem.Level, RubyCost: rubyCost,
			})
		}
	}
	remainingRubySpend := quote.MaximumRubySpend
	paidExtractionIndex := 0
	catalogDigest := ""
	if input.GameData != nil {
		catalogDigest = input.GameData.Metadata().DigestSHA256
	}
	for _, gemID := range detachIDs {
		gem := transition.GemsToDetach[gemID]
		parent := input.State.Inventory.Equipment[gem.EquipmentInstanceID]
		if mountedBySlot[parent.Slot] != parent.ID {
			if mountedBySlot[parent.Slot] != 0 {
				return Intent.Plan{}, Localization.WithError(fmt.Errorf("cannot mount gem carrier %d while slot %d is occupied", parent.ID, parent.Slot), Localization.New("server.app.cannot_mount_gem_carrier.2a426f26", "cannot mount gem carrier {p0} while slot {p1} is occupied", Localization.Params{"p0": fmt.Sprintf("%d", parent.ID), "p1": parent.Slot}))
			}
			equipPayload, _ := json.Marshal(struct {
				EquipmentID State.EquipmentInstanceID `json:"EID"`
				LeaderID    int64                     `json:"LID"`
				Equip       int                       `json:"E"`
			}{parent.ID, leader.id, 1})
			equipStep := commandStep(fmt.Sprintf("Mount gem carrier %d", parent.ID), "eeq", equipPayload, "eeq", Localization.New("server.app.mount_gem_carrier_p.fd01b831", "Mount gem carrier {p0}", Localization.Params{"p0": fmt.Sprintf("%d", parent.ID)}))
			steps = append(steps, equipStep)
			mountedBySlot[parent.Slot] = parent.ID
		}
		detachPayload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
		}{parent.ID, leader.id})
		detachStep := commandStep(fmt.Sprintf("Detach gem %d", gem.ID), "ege", detachPayload, "ege", Localization.New("server.app.detach_gem_p.3af31498", "Detach gem {p0}", Localization.Params{"p0": fmt.Sprintf("%d", gem.ID)}))
		if EquipmentDomain.GemFamily(gem) == EquipmentDomain.LoadoutFamilyOrdinary {
			rubyCost, costErr := EquipmentDomain.NormalGemRemovalCost(input.GameData, gem)
			if costErr != nil {
				return Intent.Plan{}, costErr
			}
			if rubyCost > 0 {
				if paidExtractionIndex >= len(paidExtractions) || paidExtractions[paidExtractionIndex].GemID != gem.ID {
					return Intent.Plan{}, Localization.WithError(fmt.Errorf("paid extraction schedule changed while planning gem %d", gem.ID), Localization.New("server.app.paid_extraction_schedule_changed.1abc0eba", "paid extraction schedule changed while planning gem {p0}", Localization.Params{"p0": fmt.Sprintf("%d", gem.ID)}))
				}
				dispatch := equipmentExtractionDispatch{
					Request: request, InitialQuote: quote, GemID: gem.ID, CarrierID: parent.ID,
					RubyCost: rubyCost, ExpectedRemainingRubySpend: remainingRubySpend,
					ExpectedConnectionGeneration: input.State.Session.ConnectionGeneration,
					ExpectedCatalogDigest:        catalogDigest, RubyResourceID: rubyResourceID,
					PlanningRubyObservedAt:    planningRubyObservedAt,
					RequireNewRubyObservation: paidExtractionIndex > 0,
					RemainingPaidExtractions:  append([]equipmentPaidExtraction(nil), paidExtractions[paidExtractionIndex:]...),
				}
				dispatchArguments, encodeErr := json.Marshal(dispatch)
				if encodeErr != nil {
					return Intent.Plan{}, Localization.WithError(fmt.Errorf("encode ruby extraction guard: %w", encodeErr), Localization.ErrorContext(Localization.New("server.app.encode_ruby_extraction_guard.4b62df81", "encode ruby extraction guard", nil), encodeErr))
				}
				detachStep.PreDispatchAction, detachStep.PreDispatchArguments = "equipment.reconfigure.extraction.arm", dispatchArguments
				detachStep.FinalDispatchAction, detachStep.FinalDispatchArguments = "equipment.reconfigure.extraction.dispatch", dispatchArguments
				detachStep.DefinitiveSendFailureAction, detachStep.DefinitiveSendFailureArguments = "equipment.reconfigure.extraction.disarm", dispatchArguments
				detachStep.DefinitiveResponseFailureAction, detachStep.DefinitiveResponseFailureArguments = "equipment.reconfigure.extraction.reject", dispatchArguments
				detachStep.ResponseProjectionFailureIndeterminate = true
				steps = append(steps, detachStep, Intent.Step{
					Name: "Confirm paid gem extraction", NameDescriptor: Localization.New("server.app.confirm_paid_gem_extraction.6978298e", "Confirm paid gem extraction", nil), Action: "equipment.reconfigure.extraction.confirm", ActionArguments: dispatchArguments,
				})
				remainingRubySpend -= rubyCost
				paidExtractionIndex++
			} else {
				dispatchArguments, encodeErr := json.Marshal(equipmentFreeExtractionDispatch{
					LeaderKind: request.LeaderKind, LeaderID: request.LeaderID,
					GemID: gem.ID, CarrierID: parent.ID, DefinitionID: gem.DefinitionID, Level: gem.Level,
					ExpectedCatalogDigest: catalogDigest,
				})
				if encodeErr != nil {
					return Intent.Plan{}, Localization.WithError(fmt.Errorf("encode free extraction guard: %w", encodeErr), Localization.ErrorContext(Localization.New("server.app.encode_free_extraction_guard.104eafc7", "encode free extraction guard", nil), encodeErr))
				}
				detachStep.FinalDispatchAction = "equipment.reconfigure.extraction.free.dispatch"
				detachStep.FinalDispatchArguments = dispatchArguments
				steps = append(steps, detachStep)
			}
		} else {
			steps = append(steps, detachStep)
		}
		if request.Equipment[strconv.Itoa(parent.Slot)] == parent.ID && transition.DetachCarrierCount[parent.Slot] == 1 {
			continue
		}
		unequipPayload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{parent.ID, leader.id, 0})
		unequipStep := commandStep(fmt.Sprintf("Return gem carrier %d", parent.ID), "eeq", unequipPayload, "eeq", Localization.New("server.app.return_gem_carrier_p.fb53bfd5", "Return gem carrier {p0}", Localization.Params{"p0": fmt.Sprintf("%d", parent.ID)}))
		steps = append(steps, unequipStep)
		delete(mountedBySlot, parent.Slot)
	}

	for _, slot := range baseEquipmentSlots {
		id := request.Equipment[strconv.Itoa(slot)]
		if id <= 0 {
			continue
		}
		if mountedBySlot[slot] == id {
			continue
		}
		if mountedBySlot[slot] != 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment slot %d is unexpectedly occupied", slot), Localization.New("server.app.equipment_slot_p_is.abd7f412", "equipment slot {p0} is unexpectedly occupied", Localization.Params{"p0": slot}))
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{id, leader.id, 1})
		step := commandStep(fmt.Sprintf("Equip optimized slot %d", slot), "eeq", payload, "eeq", Localization.New("server.app.equip_optimized_slot_p.f4430f75", "Equip optimized slot {p0, number}", Localization.Params{"p0": slot}))
		steps = append(steps, step)
		mountedBySlot[slot] = id
	}
	for slot := 1; slot <= 4; slot++ {
		gemID := request.Gems[strconv.Itoa(slot)]
		if gemID == 0 || transition.AlreadyAttached[slot] {
			continue
		}
		gem := input.State.Inventory.Gems[gemID]
		commandGemID := int64(gem.ID)
		relicGem := 1
		if commandGemID < 0 {
			commandGemID = int64(gem.DefinitionID)
			relicGem = 0
		}
		payload, _ := json.Marshal(struct {
			GemID       int64                     `json:"GID"`
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Mode        int                       `json:"M"`
			RelicGem    int                       `json:"RGEM"`
		}{commandGemID, request.Equipment[strconv.Itoa(slot)], leader.id, 0, relicGem})
		step := commandStep(fmt.Sprintf("Socket optimized gem in slot %d", slot), "bge", payload, "bge", Localization.New("server.app.socket_optimized_gem_in.2ed4d955", "Socket optimized gem in slot {p0, number}", Localization.Params{"p0": slot}))
		steps = append(steps, step)
	}
	steps = append(steps, equipmentRefreshSteps()...)
	verificationArguments, err := json.Marshal(verification)
	if err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("encode optimized loadout verification: %w", err), Localization.ErrorContext(Localization.New("server.app.encode_optimized_loadout_verification.294974c7", "encode optimized loadout verification", nil), err))
	}
	steps = append(steps, Intent.Step{
		Name: "Verify optimized loadout", NameDescriptor: Localization.New("server.app.verify_optimized_loadout.cd2c0935", "Verify optimized loadout", nil), Action: "equipment.reconfigure.verify", ActionArguments: verificationArguments,
	})
	claims := equipmentLeaderClaims(leader)
	if quote.MaximumRubySpend > 0 {
		claims = append(claims, "currency:"+strconv.FormatInt(int64(rubyResourceID), 10))
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Apply optimized loadout to %s %d", leader.kind, leader.id), SummaryDescriptor: Localization.New("server.app.apply_optimized_loadout_to.2ee0e3e8", "Apply optimized loadout to {p0} {p1}", Localization.Params{"p0": fmt.Sprintf("%s", leader.kind), "p1": fmt.Sprintf("%d", leader.id)}), Steps: steps,
	}, nil
}

func (application *Application) verifyEquipmentReconfigure(_ context.Context, arguments json.RawMessage) error {
	var request equipmentReconfigureVerification
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	leader, err := resolveLeader(application.State.ReadOnlyView(), request.LeaderKind, request.LeaderID)
	if err != nil {
		return err
	}
	for _, slot := range baseEquipmentSlots {
		key := strconv.Itoa(slot)
		if leader.equipment[key] != request.Equipment[key] {
			return Localization.WithError(fmt.Errorf("%s %d equipment slot %d did not match the selected loadout", leader.kind, leader.id, slot), Localization.New("server.app.p_p_equipment_slot.5aea94a3", "{p0} {p1} equipment slot {p2} did not match the selected loadout", Localization.Params{"p0": fmt.Sprintf("%s", leader.kind), "p1": fmt.Sprintf("%d", leader.id), "p2": slot}))
		}
	}
	for slot := 1; slot <= 4; slot++ {
		key := strconv.Itoa(slot)
		expected, hasExpected := request.Gems[key]
		actualID := leader.gems[key]
		if !hasExpected && actualID == 0 {
			continue
		}
		if !hasExpected || actualID == 0 {
			return Localization.WithError(fmt.Errorf("%s %d gem slot %d did not match the selected loadout", leader.kind, leader.id, slot), Localization.New("server.app.p_p_gem_slot.46b5d4e8", "{p0} {p1} gem slot {p2} did not match the selected loadout", Localization.Params{"p0": fmt.Sprintf("%s", leader.kind), "p1": fmt.Sprintf("%d", leader.id), "p2": slot}))
		}
		if !expected.Normal && actualID == expected.InstanceID {
			continue
		}
		actual, found := application.State.ReadOnlyView().Inventory.Gems[actualID]
		if !expected.Normal || !found || actualID >= 0 || actual.DefinitionID != expected.DefinitionID ||
			actual.EquipmentInstanceID != request.Equipment[key] ||
			actual.WearerKind != leader.kind || actual.WearerID != leader.id {
			return Localization.WithError(fmt.Errorf("%s %d gem slot %d did not match the selected loadout", leader.kind, leader.id, slot), Localization.New("server.app.p_p_gem_slot.46b5d4e8", "{p0} {p1} gem slot {p2} did not match the selected loadout", Localization.Params{"p0": fmt.Sprintf("%s", leader.kind), "p1": fmt.Sprintf("%d", leader.id), "p2": slot}))
		}
	}
	return nil
}

func (application *Application) validateFreeEquipmentExtractionDispatch(_ context.Context, raw json.RawMessage) error {
	var arguments equipmentFreeExtractionDispatch
	if err := decodeIntentArguments(raw, &arguments); err != nil {
		return err
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("equipment extraction state is unavailable"), Localization.New("server.app.equipment_extraction_state_is.4cf9ca3d", "equipment extraction state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if arguments.ExpectedCatalogDigest == "" || gameData.Metadata().DigestSHA256 != arguments.ExpectedCatalogDigest {
		return Localization.WithError(fmt.Errorf("%w: official gem removal prices changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d7d59275", "intent plan became stale before dispatch: official gem removal prices changed", nil))
	}
	gameState := application.State.ReadOnlyView()
	leader, err := resolveLeader(gameState, arguments.LeaderKind, arguments.LeaderID)
	if err != nil {
		return err
	}
	if !leader.available {
		return Localization.WithError(fmt.Errorf("%w: commander became unavailable before gem extraction", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.f2f8e02a", "intent plan became stale before dispatch: commander became unavailable before gem extraction", nil))
	}
	gem, found := gameState.Inventory.Gems[arguments.GemID]
	if !found || gem.EquipmentInstanceID != arguments.CarrierID || gem.DefinitionID != arguments.DefinitionID ||
		gem.Level != arguments.Level || EquipmentDomain.GemFamily(gem) != EquipmentDomain.LoadoutFamilyOrdinary {
		return Localization.WithError(fmt.Errorf("%w: normal gem %d is no longer on carrier %d", Intent.ErrPlanStale, arguments.GemID, arguments.CarrierID), Localization.New("server.app.intent_plan_became_stale.5aff6d7d", "intent plan became stale before dispatch: normal gem {p1} is no longer on carrier {p2}", Localization.Params{"p1": fmt.Sprintf("%d", arguments.GemID), "p2": fmt.Sprintf("%d", arguments.CarrierID)}))
	}
	carrier, found := gameState.Inventory.Equipment[arguments.CarrierID]
	if !found || carrier.WearerKind != leader.kind || carrier.WearerID != leader.id ||
		leader.equipment[strconv.Itoa(carrier.Slot)] != carrier.ID {
		return Localization.WithError(fmt.Errorf("%w: gem carrier %d is not mounted on the selected leader", Intent.ErrPlanStale, arguments.CarrierID), Localization.New("server.app.intent_plan_became_stale.8648c909", "intent plan became stale before dispatch: gem carrier {p1} is not mounted on the selected leader", Localization.Params{"p1": fmt.Sprintf("%d", arguments.CarrierID)}))
	}
	cost, costErr := EquipmentDomain.NormalGemRemovalCost(gameData, gem)
	if costErr != nil || cost != 0 {
		return Localization.WithError(fmt.Errorf("%w: official removal price changed for gem %d", Intent.ErrPlanStale, gem.ID), Localization.New("server.app.intent_plan_became_stale.4cb8c375", "intent plan became stale before dispatch: official removal price changed for gem {p1}", Localization.Params{"p1": fmt.Sprintf("%d", gem.ID)}))
	}
	return nil
}

func validateEquipmentRubyAuthority(
	gameState State.GameState,
	resourceID State.ResourceID,
	required int64,
	now time.Time,
	requireAfter time.Time,
) (time.Time, error) {
	if resourceID <= 0 || required < 0 {
		return time.Time{}, Localization.WithError(fmt.Errorf("ruby extraction authority is invalid"), Localization.New("server.app.ruby_extraction_authority_is.15b19d4a", "ruby extraction authority is invalid", nil))
	}
	if !gameState.Session.LoggedIn || !gameState.Session.SocketReady || gameState.Session.ConnectionGeneration == 0 {
		return time.Time{}, Localization.WithError(fmt.Errorf("%w: game session is unavailable for ruby extraction", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.3a7eb5ec", "intent plan became stale before dispatch: game session is unavailable for ruby extraction", nil))
	}
	observation, found := gameState.Player.ResourceObservations[resourceID]
	if !found || observation.ObservedAt.IsZero() || observation.ConnectionGeneration != gameState.Session.ConnectionGeneration ||
		!gameState.Session.ChangedAt.IsZero() && observation.ObservedAt.Before(gameState.Session.ChangedAt) ||
		observation.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(observation.ObservedAt) > equipmentRubyFreshness ||
		!requireAfter.IsZero() && !observation.ObservedAt.After(requireAfter) {
		return time.Time{}, Localization.WithError(fmt.Errorf("%w: a fresh current-session ruby balance is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.8eb2bf6c", "intent plan became stale before dispatch: a fresh current-session ruby balance is unavailable", nil))
	}
	value, found := gameState.Player.Resources[resourceID]
	if !found || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return time.Time{}, Localization.WithError(fmt.Errorf("ruby balance is unavailable"), Localization.New("server.app.ruby_balance_is_unavailable.f542080d", "ruby balance is unavailable", nil))
	}
	if int64(math.Floor(value)) < required {
		return time.Time{}, Localization.WithError(fmt.Errorf("%w: ruby balance cannot cover the remaining %d-ruby extraction ceiling", Intent.ErrPlanStale, required), Localization.New("server.app.intent_plan_became_stale.3cdc102a", "intent plan became stale before dispatch: ruby balance cannot cover the remaining {p1, number}-ruby extraction ceiling", Localization.Params{"p1": required}))
	}
	return observation.ObservedAt, nil
}

func (application *Application) validateEquipmentExtractionDispatch(
	arguments equipmentExtractionDispatch,
	metadata Outbound.Metadata,
	requireMarker bool,
) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("equipment extraction state is unavailable"), Localization.New("server.app.equipment_extraction_state_is.4cf9ca3d", "equipment extraction state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if arguments.ExpectedCatalogDigest == "" || gameData.Metadata().DigestSHA256 != arguments.ExpectedCatalogDigest {
		return Localization.WithError(fmt.Errorf("%w: official gem removal prices changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d7d59275", "intent plan became stale before dispatch: official gem removal prices changed", nil))
	}
	quote := arguments.InitialQuote
	quote.Fingerprint = ""
	expectedQuoteFingerprint := EquipmentDomain.ReconfigurationQuoteFingerprint(
		arguments.Request.SnapshotFingerprint, arguments.Request.Equipment, arguments.Request.Gems, quote,
	)
	if arguments.Request.QuoteFingerprint == "" || arguments.Request.QuoteFingerprint != expectedQuoteFingerprint ||
		quote.MaximumRubySpend <= 0 || quote.MaximumRubySpend > arguments.Request.MaximumRubySpend {
		return Localization.WithError(fmt.Errorf("%w: selected extraction quote is no longer authorized", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.7d890214", "intent plan became stale before dispatch: selected extraction quote is no longer authorized", nil))
	}
	if len(arguments.RemainingPaidExtractions) == 0 ||
		arguments.RemainingPaidExtractions[0].GemID != arguments.GemID ||
		arguments.RemainingPaidExtractions[0].CarrierID != arguments.CarrierID ||
		arguments.RemainingPaidExtractions[0].RubyCost != arguments.RubyCost {
		return Localization.WithError(fmt.Errorf("%w: paid extraction schedule changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.76999187", "intent plan became stale before dispatch: paid extraction schedule changed", nil))
	}
	gameState := application.State.ReadOnlyView()
	if arguments.ExpectedConnectionGeneration == 0 || gameState.Session.ConnectionGeneration != arguments.ExpectedConnectionGeneration {
		return Localization.WithError(fmt.Errorf("%w: game session changed before ruby extraction", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a9a68905", "intent plan became stale before dispatch: game session changed before ruby extraction", nil))
	}
	leader, err := resolveLeader(gameState, arguments.Request.LeaderKind, arguments.Request.LeaderID)
	if err != nil {
		return err
	}
	if !leader.available {
		return Localization.WithError(fmt.Errorf("%w: commander became unavailable before ruby extraction", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4a14fe85", "intent plan became stale before dispatch: commander became unavailable before ruby extraction", nil))
	}
	remaining := int64(0)
	for index, expected := range arguments.RemainingPaidExtractions {
		gem, found := gameState.Inventory.Gems[expected.GemID]
		if !found || gem.EquipmentInstanceID != expected.CarrierID || gem.DefinitionID != expected.DefinitionID ||
			gem.Level != expected.Level || EquipmentDomain.GemFamily(gem) != EquipmentDomain.LoadoutFamilyOrdinary {
			return Localization.WithError(fmt.Errorf("%w: normal gem %d is no longer on carrier %d", Intent.ErrPlanStale, expected.GemID, expected.CarrierID), Localization.New("server.app.intent_plan_became_stale.5aff6d7d", "intent plan became stale before dispatch: normal gem {p1} is no longer on carrier {p2}", Localization.Params{"p1": fmt.Sprintf("%d", expected.GemID), "p2": fmt.Sprintf("%d", expected.CarrierID)}))
		}
		carrier, found := gameState.Inventory.Equipment[expected.CarrierID]
		if !found {
			return Localization.WithError(fmt.Errorf("%w: gem carrier %d is unavailable", Intent.ErrPlanStale, expected.CarrierID), Localization.New("server.app.intent_plan_became_stale.7ec3ac07", "intent plan became stale before dispatch: gem carrier {p1} is unavailable", Localization.Params{"p1": fmt.Sprintf("%d", expected.CarrierID)}))
		}
		if index == 0 {
			if carrier.WearerKind != leader.kind || carrier.WearerID != leader.id ||
				leader.equipment[strconv.Itoa(carrier.Slot)] != carrier.ID {
				return Localization.WithError(fmt.Errorf("%w: current gem carrier %d is not mounted on the selected leader", Intent.ErrPlanStale, expected.CarrierID), Localization.New("server.app.intent_plan_became_stale.545c9a4c", "intent plan became stale before dispatch: current gem carrier {p1} is not mounted on the selected leader", Localization.Params{"p1": fmt.Sprintf("%d", expected.CarrierID)}))
			}
		} else if carrier.WearerKind != "" && (carrier.WearerKind != leader.kind || carrier.WearerID != leader.id) {
			return Localization.WithError(fmt.Errorf("%w: future gem carrier %d is unavailable", Intent.ErrPlanStale, expected.CarrierID), Localization.New("server.app.intent_plan_became_stale.b512c7c7", "intent plan became stale before dispatch: future gem carrier {p1} is unavailable", Localization.Params{"p1": fmt.Sprintf("%d", expected.CarrierID)}))
		}
		cost, costErr := EquipmentDomain.NormalGemRemovalCost(gameData, gem)
		if costErr != nil || cost != expected.RubyCost || remaining > math.MaxInt64-cost {
			return Localization.WithError(fmt.Errorf("%w: official removal price changed for gem %d", Intent.ErrPlanStale, gem.ID), Localization.New("server.app.intent_plan_became_stale.4cb8c375", "intent plan became stale before dispatch: official removal price changed for gem {p1}", Localization.Params{"p1": fmt.Sprintf("%d", gem.ID)}))
		}
		remaining += cost
	}
	if remaining != arguments.ExpectedRemainingRubySpend || remaining <= 0 {
		return Localization.WithError(fmt.Errorf("%w: remaining ruby extraction quote changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.46638e5f", "intent plan became stale before dispatch: remaining ruby extraction quote changed", nil))
	}
	currentCarrier := gameState.Inventory.Equipment[arguments.CarrierID]
	operationID := strings.TrimSpace(metadata.OperationID)
	responseToken := strings.TrimSpace(metadata.ResponseToken)
	if operationID == "" {
		return Localization.WithError(fmt.Errorf("ruby extraction operation identity is unavailable"), Localization.New("server.app.ruby_extraction_operation_identity.5224336a", "ruby extraction operation identity is unavailable", nil))
	}
	if requireMarker {
		expected := arguments.RemainingPaidExtractions[0]
		marker := currentCarrier.Extraction
		if marker == nil || marker.GemID != arguments.GemID || marker.RubyCost != arguments.RubyCost ||
			marker.DefinitionID != expected.DefinitionID || marker.Level != expected.Level ||
			marker.OperationID != operationID || marker.ConnectionGeneration != arguments.ExpectedConnectionGeneration ||
			marker.ResponseToken != responseToken {
			return Localization.WithError(fmt.Errorf("%w: ruby extraction dispatch marker changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.013ea596", "intent plan became stale before dispatch: ruby extraction dispatch marker changed", nil))
		}
	} else if currentCarrier.Extraction != nil {
		return Localization.WithError(fmt.Errorf("%w: gem %d has an unresolved prior ruby extraction attempt", Intent.ErrPlanStale, arguments.GemID), Localization.New("server.app.intent_plan_became_stale.ac6f7858", "intent plan became stale before dispatch: gem {p1} has an unresolved prior ruby extraction attempt", Localization.Params{"p1": fmt.Sprintf("%d", arguments.GemID)}))
	}
	requireAfter := time.Time{}
	if arguments.RequireNewRubyObservation {
		requireAfter = arguments.PlanningRubyObservedAt
	}
	_, err = validateEquipmentRubyAuthority(gameState, arguments.RubyResourceID, remaining, time.Now().UTC(), requireAfter)
	return err
}

func (application *Application) armEquipmentExtraction(ctx context.Context, raw json.RawMessage) error {
	var arguments equipmentExtractionDispatch
	if err := decodeIntentArguments(raw, &arguments); err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	if err := application.validateEquipmentExtractionDispatch(arguments, metadata, false); err != nil {
		return err
	}
	operationID, responseToken := strings.TrimSpace(metadata.OperationID), strings.TrimSpace(metadata.ResponseToken)
	now := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentInventory), func(gameState *State.GameState) ([]string, bool, error) {
		carrier, found := gameState.Inventory.Equipment[arguments.CarrierID]
		gem, gemFound := gameState.Inventory.Gems[arguments.GemID]
		if !found || !gemFound || gem.EquipmentInstanceID != carrier.ID || carrier.Extraction != nil {
			return nil, false, Localization.WithError(fmt.Errorf("%w: gem carrier changed before ruby extraction", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.2fe09a07", "intent plan became stale before dispatch: gem carrier changed before ruby extraction", nil))
		}
		carrier.Extraction = &State.GemExtractionAttempt{
			GemID: arguments.GemID, DefinitionID: arguments.RemainingPaidExtractions[0].DefinitionID,
			Level: arguments.RemainingPaidExtractions[0].Level, RubyCost: arguments.RubyCost, OperationID: operationID,
			ResponseToken: responseToken, ConnectionGeneration: arguments.ExpectedConnectionGeneration, ArmedAt: now,
		}
		gameState.SetInventoryEquipment(carrier.ID, carrier)
		return []string{"inventory", "equipment"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) finalizeEquipmentExtractionDispatch(ctx context.Context, raw json.RawMessage) error {
	var arguments equipmentExtractionDispatch
	if err := decodeIntentArguments(raw, &arguments); err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	if err := application.validateEquipmentExtractionDispatch(arguments, metadata, true); err != nil {
		return err
	}
	operationID := strings.TrimSpace(metadata.OperationID)
	now := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentInventory), func(gameState *State.GameState) ([]string, bool, error) {
		carrier := gameState.Inventory.Equipment[arguments.CarrierID]
		if carrier.Extraction == nil || carrier.Extraction.OperationID != operationID {
			return nil, false, Localization.WithError(fmt.Errorf("%w: ruby extraction dispatch marker changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.013ea596", "intent plan became stale before dispatch: ruby extraction dispatch marker changed", nil))
		}
		if !carrier.Extraction.DispatchedAt.IsZero() {
			return nil, false, nil
		}
		marker := *carrier.Extraction
		marker.DispatchedAt = now
		carrier.Extraction = &marker
		gameState.SetInventoryEquipment(carrier.ID, carrier)
		return []string{"inventory", "equipment"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) disarmEquipmentExtraction(ctx context.Context, raw json.RawMessage) error {
	return application.clearEquipmentExtractionMarker(ctx, raw, false)
}

func (application *Application) rejectEquipmentExtraction(ctx context.Context, raw json.RawMessage) error {
	return application.clearEquipmentExtractionMarker(ctx, raw, false)
}

func (application *Application) confirmEquipmentExtraction(ctx context.Context, raw json.RawMessage) error {
	return application.clearEquipmentExtractionMarker(ctx, raw, true)
}

func (application *Application) clearEquipmentExtractionMarker(ctx context.Context, raw json.RawMessage, requireDetached bool) error {
	var arguments equipmentExtractionDispatch
	if err := decodeIntentArguments(raw, &arguments); err != nil {
		return err
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	event, err := application.State.ApplyComponents(State.Components(State.ComponentInventory), func(gameState *State.GameState) ([]string, bool, error) {
		carrier, found := gameState.Inventory.Equipment[arguments.CarrierID]
		if !found || carrier.Extraction == nil || carrier.Extraction.OperationID != operationID {
			return nil, false, nil
		}
		if requireDetached {
			if gem, gemFound := gameState.Inventory.Gems[arguments.GemID]; gemFound && gem.EquipmentInstanceID == carrier.ID {
				return nil, false, Localization.WithError(fmt.Errorf("%w: paid gem extraction was not authoritatively observed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4e3a954a", "intent plan became stale before dispatch: paid gem extraction was not authoritatively observed", nil))
			}
		}
		carrier.Extraction = nil
		gameState.SetInventoryEquipment(carrier.ID, carrier)
		return []string{"inventory", "equipment"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func cloneEquipmentSelection(source map[string]State.EquipmentInstanceID) map[string]State.EquipmentInstanceID {
	result := make(map[string]State.EquipmentInstanceID, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (application *Application) planEquipmentUpgrade(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		ItemKind    string `json:"itemKind"`
		ItemID      int64  `json:"itemId"`
		TargetLevel int    `json:"targetLevel"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.ItemKind = strings.ToLower(strings.TrimSpace(request.ItemKind))
	if request.TargetLevel < 1 || request.TargetLevel > maxEquipmentUpgradeLevel {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("targetLevel must be between 1 and %d", maxEquipmentUpgradeLevel), Localization.New("server.app.targetlevel_must_be_between.88afe620", "targetLevel must be between 1 and {p0}", Localization.Params{"p0": maxEquipmentUpgradeLevel}))
	}
	currentLevel := 0
	maximumLevel := maxEquipmentUpgradeLevel
	upgradeOpcode := "ere"
	relicUpgrade := true
	var payload json.RawMessage
	var wearerKind string
	var wearerID int64
	switch request.ItemKind {
	case "equipment":
		item, ok := input.State.Inventory.Equipment[State.EquipmentInstanceID(request.ItemID)]
		if !ok || request.ItemID <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d is not in current state", request.ItemID), Localization.New("server.app.equipment_p_is_not.eacf0b7b", "equipment {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID)}))
		}
		var supported bool
		maximumLevel, relicUpgrade, supported = equipmentUpgradeLevelCap(item)
		if !supported {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("equipment %d has an unsupported or unverified enchantment type", request.ItemID), Localization.New("server.app.equipment_p_has_an.776d0317", "equipment {p0} has an unsupported or unverified enchantment type", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID)}))
		}
		currentLevel = item.Level
		wearerKind, wearerID = item.WearerKind, item.WearerID
		if relicUpgrade {
			payload, _ = json.Marshal(struct {
				CostMode  int   `json:"C2"`
				ItemID    int64 `json:"RIID"`
				Equipment int   `json:"EQ"`
			}{0, request.ItemID, 1})
		} else {
			upgradeOpcode = "eqe"
			payload, _ = json.Marshal(struct {
				CostMode int   `json:"C2"`
				ItemID   int64 `json:"EID"`
			}{0, request.ItemID})
		}
	case "gem":
		gem, ok := input.State.Inventory.Gems[State.GemInstanceID(request.ItemID)]
		if !ok || request.ItemID <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("relic gem %d is not in current state", request.ItemID), Localization.New("server.app.relic_gem_p_is.2c5b94be", "relic gem {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID)}))
		}
		currentLevel = gem.Level
		wearerKind, wearerID = gem.WearerKind, gem.WearerID
		if wearerKind == "" && gem.EquipmentInstanceID != 0 {
			carrier, found := input.State.Inventory.Equipment[gem.EquipmentInstanceID]
			if !found {
				return Intent.Plan{}, Localization.WithError(fmt.Errorf("relic gem %d references missing equipment %d", request.ItemID, gem.EquipmentInstanceID), Localization.New("server.app.relic_gem_p_references.4b542572", "relic gem {p0} references missing equipment {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.ItemID), "p1": fmt.Sprintf("%d", gem.EquipmentInstanceID)}))
			}
			wearerKind, wearerID = carrier.WearerKind, carrier.WearerID
		}
		payload, _ = json.Marshal(struct {
			CostMode  int   `json:"C2"`
			ItemID    int64 `json:"RIID"`
			Equipment int   `json:"EQ"`
		}{0, request.ItemID, 0})
	default:
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("itemKind must be equipment or gem"), Localization.New("server.app.itemkind_must_be_equipment.f68f1395", "itemKind must be equipment or gem", nil))
	}
	if request.TargetLevel > maximumLevel {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("targetLevel cannot exceed %d for this %s", maximumLevel, request.ItemKind), Localization.New("server.app.targetlevel_cannot_exceed_p.c4bd25e9", "targetLevel cannot exceed {p0} for this {p1}", Localization.Params{"p0": maximumLevel, "p1": fmt.Sprintf("%s", request.ItemKind)}))
	}
	if request.TargetLevel <= currentLevel {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("targetLevel must be above current level %d", currentLevel), Localization.New("server.app.targetlevel_must_be_above.b6cddad0", "targetLevel must be above current level {p0}", Localization.Params{"p0": currentLevel}))
	}
	claims, err := equipmentUpgradeClaims(input.State, request.ItemKind, request.ItemID, wearerKind, wearerID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if err := application.verifyEquipmentCoinReserve(context.Background(), nil); err != nil {
		return Intent.Plan{}, err
	}
	delay := application.equipmentUpgradeDelay()
	coinReserve, _ := application.equipmentUpgradeSettings()
	steps := make([]Intent.Step, 0, (request.TargetLevel-currentLevel)*2+4)
	if relicUpgrade {
		steps = append(steps, equipmentUpgradeContextStep())
	}
	for level := currentLevel + 1; level <= request.TargetLevel; level++ {
		guard := Intent.Step{Name: "Verify coin reserve", NameDescriptor: Localization.New("server.app.verify_coin_reserve.47ce334b", "Verify coin reserve", nil), Action: "equipment.verify_coin_reserve", DelayMillis: delay}
		steps = append(steps, Intent.RebuildOnResume(guard))
		upgradeStep := Intent.Step{
			Name: fmt.Sprintf("Upgrade %s to level %d", request.ItemKind, level), NameDescriptor: Localization.New("server.app.upgrade_p_to_level.b3b197bd", "Upgrade {p0} to level {p1}", Localization.Params{"p0": fmt.Sprintf("%s", request.ItemKind), "p1": level}), Opcode: upgradeOpcode, Payload: payload,
			AwaitOpcode: upgradeOpcode, TimeoutMillis: 8_000, SuccessCodes: []int{0},
			// The game commits its separate coin/currency updates before returning
			// 227 for a consumed failed roll. Recheck the reserve, then repeat this
			// exact level; never count the roll as successful or stale progress.
			ResponseRetry: &Intent.ResponseRetryPolicy{
				Codes: []int{227}, GuardAction: "equipment.verify_coin_reserve", DelayMillis: delay,
			},
			Command: Protocol.Command{Opcode: upgradeOpcode, Payload: payload},
		}
		if coinReserve > 0 {
			upgradeStep.CoinCost = &Intent.CoinCostRequirement{Reserve: int64(math.Ceil(coinReserve)), Source: "configured equipment upgrade coin reserve"}
		}
		steps = append(steps, upgradeStep)
	}
	steps = append(steps, equipmentRefreshSteps()...)
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Upgrade %s %d from level %d to %d", request.ItemKind, request.ItemID, currentLevel, request.TargetLevel), SummaryDescriptor: Localization.New("server.app.upgrade_p_p_from.255b5cef", "Upgrade {p0} {p1} from level {p2} to {p3}", Localization.Params{"p0": fmt.Sprintf("%s", request.ItemKind), "p1": fmt.Sprintf("%d", request.ItemID), "p2": currentLevel, "p3": request.TargetLevel}),
		Steps: steps,
	}, nil
}

// equipmentUpgradeLevelCap mirrors the current official client eligibility
// and limits. The parser retains the client's exact RelicEquipmentVO wire
// discriminator separately from leader compatibility; no rarity-only guess is
// allowed to select ERE. Ordinary heroes and appearance items are not
// enchantable through EQE.
func equipmentUpgradeLevelCap(item State.EquipmentInstance) (maximumLevel int, relic bool, supported bool) {
	if !item.RelicKnown {
		return 0, false, false
	}
	if item.Relic {
		return maxEquipmentUpgradeLevel, true, true
	}
	if item.Slot < 1 || item.Slot > 4 {
		return 0, false, false
	}
	switch item.RarityID {
	case 0:
		return 20, false, true
	case 1:
		return 3, false, true
	case 2:
		return 8, false, true
	case 3:
		return 12, false, true
	case 4:
		return 16, false, true
	case 5:
		return 50, false, true
	default:
		return 0, false, false
	}
}

func equipmentUpgradeClaims(
	gameState State.GameState,
	itemKind string,
	itemID int64,
	wearerKind string,
	wearerID int64,
) ([]string, error) {
	claims := []string{
		"game:equipment",
		itemKind + ":" + strconv.FormatInt(itemID, 10),
		"account-resources",
	}
	wearerKind = strings.ToLower(strings.TrimSpace(wearerKind))
	switch wearerKind {
	case "":
		return claims, nil
	case "commander":
		commander, found := gameState.Commanders[State.CommanderID(wearerID)]
		if !found {
			return nil, Localization.WithError(fmt.Errorf("%s %d is worn by commander %d, which is missing from current state", itemKind, itemID, wearerID), Localization.New("server.app.p_p_is_worn.92345228", "{p0} {p1} is worn by commander {p2}, which is missing from current state", Localization.Params{"p0": fmt.Sprintf("%s", itemKind), "p1": fmt.Sprintf("%d", itemID), "p2": fmt.Sprintf("%d", wearerID)}))
		}
		if !commander.Available || State.CommanderHasActiveMovementAt(gameState, commander.ID, time.Now().UTC()) {
			return nil, Localization.WithError(fmt.Errorf("%s %d cannot be upgraded while commander %d is travelling", itemKind, itemID, wearerID), Localization.New("server.app.p_p_cannot_be.b88d1a3d", "{p0} {p1} cannot be upgraded while commander {p2} is travelling", Localization.Params{"p0": fmt.Sprintf("%s", itemKind), "p1": fmt.Sprintf("%d", itemID), "p2": fmt.Sprintf("%d", wearerID)}))
		}
	case "castellan":
		if _, found := gameState.Castellans[State.CastellanID(wearerID)]; !found {
			return nil, Localization.WithError(fmt.Errorf("%s %d is worn by castellan %d, which is missing from current state", itemKind, itemID, wearerID), Localization.New("server.app.p_p_is_worn.7e1d0e6a", "{p0} {p1} is worn by castellan {p2}, which is missing from current state", Localization.Params{"p0": fmt.Sprintf("%s", itemKind), "p1": fmt.Sprintf("%d", itemID), "p2": fmt.Sprintf("%d", wearerID)}))
		}
	default:
		return nil, Localization.WithError(fmt.Errorf("%s %d has unsupported wearer kind %q", itemKind, itemID, wearerKind), Localization.New("server.app.p_p_has_unsupported.f708acb2", "{p0} {p1} has unsupported wearer kind {p2}", Localization.Params{"p0": fmt.Sprintf("%s", itemKind), "p1": fmt.Sprintf("%d", itemID), "p2": fmt.Sprintf("%q", wearerKind)}))
	}
	claims = append(claims, "leader:"+wearerKind+":"+strconv.FormatInt(wearerID, 10))
	return claims, nil
}

func planEquipmentSell(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		Category      string `json:"category"`
		SellLookItems bool   `json:"sellLookItems,omitempty"`
		SellPost2026  bool   `json:"sellPost2026,omitempty"`
		KeepStars     int    `json:"keepStars,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.Category = strings.ToLower(strings.TrimSpace(request.Category))
	if request.KeepStars < 0 || request.KeepStars > 42 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("keepStars must be between 0 and 42"), Localization.New("server.app.keepstars_must_be_between.e4fc6134", "keepStars must be between 0 and 42", nil))
	}
	steps := []Intent.Step{}
	count := 0
	switch request.Category {
	case "non_relic_equipment", "relic1_equipment", "relic2_equipment":
		if err := requireRecentEquipmentSnapshot(input.State, "gei"); err != nil {
			return Intent.Plan{}, err
		}
		ids := make([]State.EquipmentInstanceID, 0)
		for id, item := range input.State.Inventory.Equipment {
			if item.WearerKind != "" || !equipmentMatchesSale(item, request.Category, request.SellLookItems, request.SellPost2026, request.KeepStars) {
				continue
			}
			ids = append(ids, id)
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		for _, id := range ids {
			payload, _ := json.Marshal(struct {
				EquipmentID State.EquipmentInstanceID `json:"EID"`
				LeaderID    int64                     `json:"LID"`
				Extra       int                       `json:"EX"`
				FilterID    int                       `json:"LFID"`
			}{id, -1, 0, -1})
			step := commandStep(fmt.Sprintf("Sell equipment %d", id), "seq", payload, "seq", Localization.New("server.app.sell_equipment_p.7101a2a7", "Sell equipment {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
			steps = append(steps, step)
		}
		count = len(ids)
		if count > 0 {
			steps = append(steps, commandStep("Refresh equipment storage", "gei", json.RawMessage(`{}`), "gei", Localization.New("server.app.refresh_equipment_storage.ac2d5167", "Refresh equipment storage", nil)))
		}
	case "non_relic_gems":
		if err := requireRecentEquipmentSnapshot(input.State, "ggm"); err != nil {
			return Intent.Plan{}, err
		}
		ids := make([]State.GemID, 0, len(input.State.Inventory.GemStacks))
		for id := range input.State.Inventory.GemStacks {
			if EquipmentDomain.MatchesNonRelicGemStackSale(id, request.SellPost2026) {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		for _, id := range ids {
			for index := int64(0); index < input.State.Inventory.GemStacks[id]; index++ {
				payload, _ := json.Marshal(struct {
					GemID    State.GemID `json:"GID"`
					RelicGem int         `json:"RGEM"`
					FilterID int         `json:"LFID"`
				}{id, 0, -1})
				step := commandStep(fmt.Sprintf("Sell gem %d", id), "sge", payload, "sge", Localization.New("server.app.sell_gem_p.a179cdcd", "Sell gem {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
				steps = append(steps, step)
				count++
			}
		}
		if count > 0 {
			steps = append(steps, commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm", Localization.New("server.app.refresh_gem_storage.10edb39b", "Refresh gem storage", nil)))
		}
	case "relic1_gems", "relic2_gems":
		if err := requireRecentEquipmentSnapshot(input.State, "ggm"); err != nil {
			return Intent.Plan{}, err
		}
		ids := make([]State.GemInstanceID, 0)
		for id, gem := range input.State.Inventory.Gems {
			if gem.WearerKind == "" && gemMatchesSale(gem, request.Category, request.KeepStars) {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		for _, id := range ids {
			payload, _ := json.Marshal(struct {
				GemID    State.GemInstanceID `json:"GID"`
				RelicGem int                 `json:"RGEM"`
				FilterID int                 `json:"LFID"`
			}{id, 1, -1})
			step := commandStep(fmt.Sprintf("Sell relic gem %d", id), "sge", payload, "sge", Localization.New("server.app.sell_relic_gem_p.30592411", "Sell relic gem {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
			steps = append(steps, step)
		}
		count = len(ids)
		if count > 0 {
			steps = append(steps, commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm", Localization.New("server.app.refresh_gem_storage.10edb39b", "Refresh gem storage", nil)))
		}
	default:
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("unknown equipment sale category %q", request.Category), Localization.New("server.app.unknown_equipment_sale_category.fdc283b8", "unknown equipment sale category {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Category)}))
	}
	return Intent.Plan{
		Claims: []string{"game:equipment"}, Summary: fmt.Sprintf("Sell %d item(s) from %s", count, request.Category), SummaryDescriptor: Localization.New("server.app.sell_p_item_s.cde0ecb8", "Sell {p0} item(s) from {p1}", Localization.Params{"p0": count, "p1": fmt.Sprintf("%s", request.Category)}), Steps: steps,
	}, nil
}

func (application *Application) verifyEquipmentCoinReserve(_ context.Context, _ json.RawMessage) error {
	threshold, _ := application.equipmentUpgradeSettings()
	if threshold <= 0 {
		return nil
	}
	coins := application.State.ReadOnlyView().Player.Resources[State.ResourceID(1)]
	if coins <= threshold {
		return Localization.WithError(fmt.Errorf("coins under upgrade reserve (%.0f <= %.0f)", coins, threshold), Localization.New("server.app.coins_under_upgrade_reserve.0eb8b000", "coins under upgrade reserve ({p0} <= {p1})", Localization.Params{"p0": coins, "p1": threshold}))
	}
	return nil
}

func (application *Application) equipmentUpgradeDelay() int {
	_, delay := application.equipmentUpgradeSettings()
	return delay
}

func (application *Application) equipmentUpgradeSettings() (float64, int) {
	threshold := float64(0)
	delay := defaultEquipmentStepDelay
	raw, ok := application.Configuration.Section("scheduler")
	if !ok {
		return threshold, delay
	}
	var settings struct {
		UpgradeCoinThreshold float64 `json:"upgradeCoinThreshold"`
		UpgradeEreDelayMS    int     `json:"upgradeEreDelayMs"`
	}
	if json.Unmarshal(raw, &settings) == nil {
		threshold = settings.UpgradeCoinThreshold
		if settings.UpgradeEreDelayMS >= 10 && settings.UpgradeEreDelayMS <= 5_000 {
			delay = settings.UpgradeEreDelayMS
		}
	}
	return threshold, delay
}

func equipmentRefreshSteps() []Intent.Step {
	return []Intent.Step{
		commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm", Localization.New("server.app.refresh_gem_storage.10edb39b", "Refresh gem storage", nil)),
		commandStep("Refresh equipment storage", "gei", json.RawMessage(`{}`), "gei", Localization.New("server.app.refresh_equipment_storage.ac2d5167", "Refresh equipment storage", nil)),
		commandStep("Refresh leader loadouts", "gli", json.RawMessage(`{}`), "gli", Localization.New("server.app.refresh_leader_loadouts.7ebc7385", "Refresh leader loadouts", nil)),
	}
}

func equipmentMutationRefreshSteps() []Intent.Step {
	return []Intent.Step{
		commandStep("Refresh leader loadouts", "gli", json.RawMessage(`{}`), "gli", Localization.New("server.app.refresh_leader_loadouts.7ebc7385", "Refresh leader loadouts", nil)),
		commandStep("Refresh equipment storage", "gei", json.RawMessage(`{}`), "gei", Localization.New("server.app.refresh_equipment_storage.ac2d5167", "Refresh equipment storage", nil)),
	}
}

func gemMutationRefreshSteps() []Intent.Step {
	return []Intent.Step{
		commandStep("Refresh leader loadouts", "gli", json.RawMessage(`{}`), "gli", Localization.New("server.app.refresh_leader_loadouts.7ebc7385", "Refresh leader loadouts", nil)),
		commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm", Localization.New("server.app.refresh_gem_storage.10edb39b", "Refresh gem storage", nil)),
	}
}

func resolveLeader(gameState State.GameState, kind string, id int64) (resolvedLeader, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "commander":
		leader, ok := gameState.Commanders[State.CommanderID(id)]
		if !ok {
			return resolvedLeader{}, Localization.WithError(fmt.Errorf("commander %d is not in current state", id), Localization.New("server.app.commander_p_is_not.7a3d451e", "commander {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		return resolvedLeader{kind: kind, id: id, available: leader.Available, equipment: leader.Equipment, gems: leader.Gems}, nil
	case "castellan":
		leader, ok := gameState.Castellans[State.CastellanID(id)]
		if !ok {
			return resolvedLeader{}, Localization.WithError(fmt.Errorf("castellan %d is not in current state", id), Localization.New("server.app.castellan_p_is_not.cce883a3", "castellan {p0} is not in current state", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		return resolvedLeader{kind: kind, id: id, available: true, equipment: leader.Equipment, gems: leader.Gems}, nil
	default:
		return resolvedLeader{}, Localization.WithError(fmt.Errorf("leaderKind must be commander or castellan"), Localization.New("server.app.leaderkind_must_be_commander.eb1dcd4c", "leaderKind must be commander or castellan", nil))
	}
}

func equipmentLeaderClaims(leader resolvedLeader) []string {
	return []string{"game:equipment", "leader:" + leader.kind + ":" + strconv.FormatInt(leader.id, 10)}
}

func expectedEquipmentType(kind string) int {
	if kind == "castellan" {
		return 1
	}
	return 2
}

func validBaseSlot(slot int) bool {
	for _, value := range baseEquipmentSlots {
		if value == slot {
			return true
		}
	}
	return false
}

func uniqueEquipmentIDs(values []State.EquipmentInstanceID) []State.EquipmentInstanceID {
	seen := map[State.EquipmentInstanceID]struct{}{}
	result := make([]State.EquipmentInstanceID, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func leaderBaseEquipment(leader resolvedLeader) []State.EquipmentInstanceID {
	result := make([]State.EquipmentInstanceID, 0, len(baseEquipmentSlots))
	for _, slot := range baseEquipmentSlots {
		if id := leader.equipment[strconv.Itoa(slot)]; id > 0 {
			result = append(result, id)
		}
	}
	return result
}

func requireRecentEquipmentSnapshot(gameState State.GameState, opcode string) error {
	if !EquipmentDomain.StorageSnapshotFresh(gameState, opcode, time.Now()) {
		return Localization.WithError(fmt.Errorf("%s storage is stale; run equipment.refresh before selling", opcode), Localization.New("server.app.p_storage_is_stale.6f231cc7", "{p0} storage is stale; run equipment.refresh before selling", Localization.Params{"p0": fmt.Sprintf("%s", opcode)}))
	}
	return nil
}

func equipmentMatchesSale(item State.EquipmentInstance, category string, sellLookItems bool, sellPost2026 bool, keepStars int) bool {
	switch category {
	case "non_relic_equipment":
		return EquipmentDomain.MatchesNonRelicEquipmentSale(item, sellLookItems, sellPost2026)
	case "relic1_equipment":
		return item.RarityID == 5 && len(item.Effects) < 4
	case "relic2_equipment":
		return EquipmentDomain.IsRelic2Equipment(item) && effectStars(item.Effects) < keepStars
	default:
		return false
	}
}

func gemMatchesSale(gem State.GemInstance, category string, keepStars int) bool {
	switch category {
	case "relic1_gems":
		return len(gem.Effects) == 3
	case "relic2_gems":
		return (gem.TypeID == 131 || gem.TypeID == 132) && len(gem.Effects) == 4 && effectStars(gem.Effects) < keepStars
	default:
		return false
	}
}

func effectStars(effects State.EquipmentEffects) int {
	total := 0
	for _, effect := range effects {
		if effect.RollPercent != nil {
			total += starsFromPercent(*effect.RollPercent)
		}
	}
	return total
}

func starsFromPercent(percent float64) int {
	switch {
	case percent >= 100:
		return 7
	case percent >= 90:
		return 6
	case percent >= 80:
		return 5
	case percent >= 70:
		return 4
	case percent >= 60:
		return 3
	case percent >= 40:
		return 2
	default:
		return 1
	}
}
