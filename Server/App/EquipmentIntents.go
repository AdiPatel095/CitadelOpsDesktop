package App

import (
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

func planEquipmentRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims: []string{"game:equipment"}, Summary: "Refresh all equipment state",
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
		return Intent.Plan{}, fmt.Errorf("commander %d is busy", leader.id)
	}
	item, ok := input.State.Inventory.Equipment[request.EquipmentID]
	if !ok || request.EquipmentID <= 0 {
		return Intent.Plan{}, fmt.Errorf("equipment %d is not in current storage", request.EquipmentID)
	}
	if item.WearerKind != "" {
		return Intent.Plan{}, fmt.Errorf("equipment %d is already worn by %s %d", item.ID, item.WearerKind, item.WearerID)
	}
	if !validBaseSlot(item.Slot) {
		return Intent.Plan{}, fmt.Errorf("equipment %d uses unsupported slot %d", item.ID, item.Slot)
	}
	if expectedEquipmentType(leader.kind) != item.TypeID {
		return Intent.Plan{}, fmt.Errorf("equipment %d is not compatible with a %s", item.ID, leader.kind)
	}
	payload, _ := json.Marshal(struct {
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		LeaderID    int64                     `json:"LID"`
		Equip       int                       `json:"E"`
	}{item.ID, leader.id, 1})
	steps := []Intent.Step{commandStep("Equip equipment", "eeq", payload, "eeq")}
	steps = append(steps, equipmentMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Equip item %d on %s %d", item.ID, leader.kind, leader.id), Steps: steps,
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
		return Intent.Plan{}, fmt.Errorf("commander %d is busy", leader.id)
	}
	ids := uniqueEquipmentIDs(request.EquipmentIDs)
	if len(ids) == 0 {
		return Intent.Plan{}, fmt.Errorf("at least one equipmentId is required")
	}
	steps := make([]Intent.Step, 0, len(ids)+2)
	for _, id := range ids {
		item, ok := input.State.Inventory.Equipment[id]
		if !ok || item.WearerKind != leader.kind || item.WearerID != leader.id {
			return Intent.Plan{}, fmt.Errorf("equipment %d is not worn by %s %d", id, leader.kind, leader.id)
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{id, leader.id, 0})
		step := commandStep(fmt.Sprintf("Unequip equipment %d", id), "eeq", payload, "eeq")
		steps = append(steps, step)
	}
	steps = append(steps, equipmentMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Unequip %d item(s) from %s %d", len(ids), leader.kind, leader.id), Steps: steps,
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
		return Intent.Plan{}, fmt.Errorf("commander %d is busy", leader.id)
	}
	item, ok := input.State.Inventory.Equipment[request.EquipmentID]
	if !ok || item.WearerKind != leader.kind || item.WearerID != leader.id {
		return Intent.Plan{}, fmt.Errorf("equipment %d is not worn by %s %d", request.EquipmentID, leader.kind, leader.id)
	}
	if leader.gems[strconv.Itoa(item.Slot)] != 0 {
		return Intent.Plan{}, fmt.Errorf("equipment slot %d already has a gem", item.Slot)
	}
	gem, ok := input.State.Inventory.Gems[request.GemID]
	if !ok || request.GemID <= 0 || gem.WearerKind != "" {
		return Intent.Plan{}, fmt.Errorf("relic gem %d is not in current storage", request.GemID)
	}
	payload, _ := json.Marshal(struct {
		GemID       State.GemInstanceID       `json:"GID"`
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		LeaderID    int64                     `json:"LID"`
		Mode        int                       `json:"M"`
		RelicGem    int                       `json:"RGEM"`
	}{gem.ID, item.ID, leader.id, 0, 1})
	steps := []Intent.Step{commandStep("Equip relic gem", "bge", payload, "bge")}
	steps = append(steps, gemMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Socket gem %d into equipment %d", gem.ID, item.ID), Steps: steps,
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
		return Intent.Plan{}, fmt.Errorf("commander %d is busy", leader.id)
	}
	item, ok := input.State.Inventory.Equipment[request.EquipmentID]
	if !ok || item.WearerKind != leader.kind || item.WearerID != leader.id {
		return Intent.Plan{}, fmt.Errorf("equipment %d is not worn by %s %d", request.EquipmentID, leader.kind, leader.id)
	}
	if leader.gems[strconv.Itoa(item.Slot)] == 0 {
		return Intent.Plan{}, fmt.Errorf("equipment %d has no observed gem", item.ID)
	}
	payload, _ := json.Marshal(struct {
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		LeaderID    int64                     `json:"LID"`
	}{item.ID, leader.id})
	steps := []Intent.Step{commandStep("Unequip gem", "ege", payload, "ege")}
	steps = append(steps, gemMutationRefreshSteps()...)
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Remove the gem from equipment %d", item.ID), Steps: steps,
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
		return Intent.Plan{}, fmt.Errorf("select two different %ss", first.kind)
	}
	if !first.available || !second.available {
		return Intent.Plan{}, fmt.Errorf("both commanders must be available")
	}
	firstItems := leaderBaseEquipment(first)
	secondItems := leaderBaseEquipment(second)
	if len(firstItems)+len(secondItems) == 0 {
		return Intent.Plan{}, fmt.Errorf("the selected leaders have no base equipment to swap")
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
			step := commandStep(fmt.Sprintf("Unequip equipment %d", id), "eeq", payload, "eeq")
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
			step := commandStep(fmt.Sprintf("Equip equipment %d", id), "eeq", payload, "eeq")
			steps = append(steps, step)
		}
	}
	steps = append(steps, equipmentMutationRefreshSteps()...)
	claims := append(equipmentLeaderClaims(first), "leader:"+first.kind+":"+strconv.FormatInt(second.id, 10))
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Swap %d base equipment item(s) between %s %d and %s %d", len(firstItems)+len(secondItems), first.kind, first.id, second.kind, second.id),
		Steps:   steps,
	}, nil
}

func planEquipmentReconfigure(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request equipmentReconfigureRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.MaximumRubySpend < 0 {
		return Intent.Plan{}, fmt.Errorf("maximumRubySpend cannot be negative")
	}
	leader, err := resolveLeader(input.State, request.LeaderKind, request.LeaderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, fmt.Errorf("commander %d is busy", leader.id)
	}
	if leader.kind == "commander" {
		commanderID := State.CommanderID(leader.id)
		now := time.Now().UTC()
		if State.CommanderHasActiveMovementAt(input.State, commanderID, now) ||
			input.CommanderHolds != nil && input.CommanderHolds.CommanderHeldAt(commanderID, now) {
			return Intent.Plan{}, fmt.Errorf("commander %d is travelling or reserved for a launch", leader.id)
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
			return Intent.Plan{}, fmt.Errorf("%w: equipment changed after this preview was generated", Intent.ErrPlanStale)
		}
	}
	selectedEquipment := map[State.EquipmentInstanceID]struct{}{}
	selectedItems := map[int]State.EquipmentInstance{}
	selectedFamily := EquipmentDomain.LoadoutFamilyUnknown
	for _, slot := range []int{1, 2, 3, 4} {
		id := request.Equipment[strconv.Itoa(slot)]
		if id <= 0 {
			return Intent.Plan{}, fmt.Errorf("optimized loadout is missing equipment slot %d", slot)
		}
	}
	for _, slot := range baseEquipmentSlots {
		rawSlot := strconv.Itoa(slot)
		id := request.Equipment[rawSlot]
		if id == 0 {
			if slot == 6 {
				continue
			}
			return Intent.Plan{}, fmt.Errorf("optimized loadout is missing equipment slot %d", slot)
		}
		item, ok := input.State.Inventory.Equipment[id]
		if !ok || item.Slot != slot || item.TypeID != expectedEquipmentType(leader.kind) {
			return Intent.Plan{}, fmt.Errorf("equipment %d is not valid for %s slot %d", id, leader.kind, slot)
		}
		if item.WearerKind != "" && (item.WearerKind != leader.kind || item.WearerID != leader.id) {
			return Intent.Plan{}, fmt.Errorf("equipment %d is worn by another leader", id)
		}
		family := EquipmentDomain.EquipmentFamily(item)
		if family == EquipmentDomain.LoadoutFamilyUnknown {
			return Intent.Plan{}, fmt.Errorf("equipment %d has no verified ordinary or relic classification", id)
		}
		if selectedFamily != EquipmentDomain.LoadoutFamilyUnknown && family != selectedFamily {
			return Intent.Plan{}, fmt.Errorf("optimized loadout mixes ordinary and relic equipment")
		}
		selectedFamily = family
		if _, duplicate := selectedEquipment[id]; duplicate {
			return Intent.Plan{}, fmt.Errorf("equipment %d appears in more than one slot", id)
		}
		selectedEquipment[id] = struct{}{}
		selectedItems[slot] = item
	}
	appearanceFamily, appearanceRestrictsFamily, err := EquipmentDomain.RetainedAppearanceFamily(input.State, leader.equipment)
	if err != nil {
		return Intent.Plan{}, err
	}
	if appearanceRestrictsFamily && appearanceFamily != selectedFamily {
		return Intent.Plan{}, fmt.Errorf("gemmed appearance item prevents switching between ordinary and relic equipment")
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
			return Intent.Plan{}, fmt.Errorf("gem %d is not in current state", id)
		}
		if request.SnapshotFingerprint != "" && !EquipmentDomain.GemMatchesLeaderAndMode(
			input.State, gem, leader.kind, leader.id, strings.ToLower(strings.TrimSpace(request.CombatMode)),
		) {
			return Intent.Plan{}, fmt.Errorf("gem %d is not compatible with this %s %s loadout", id, leader.kind, request.CombatMode)
		}
		if !EquipmentDomain.GemMatchesEquipmentFamily(gem, selectedItems[slot]) {
			return Intent.Plan{}, fmt.Errorf("gem %d does not match equipment %d's ordinary or relic family", id, selectedItems[slot].ID)
		}
		if gem.WearerKind != "" && (gem.WearerKind != leader.kind || gem.WearerID != leader.id) {
			return Intent.Plan{}, fmt.Errorf("gem %d is worn by another leader", id)
		}
		if _, duplicate := selectedGems[id]; duplicate {
			return Intent.Plan{}, fmt.Errorf("gem %d appears in more than one slot", id)
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
			return Intent.Plan{}, fmt.Errorf("cannot detach gem %d from unavailable equipment %d", gem.ID, gem.EquipmentInstanceID)
		}
		if parent.Extraction != nil && parent.Extraction.GemID == gem.ID {
			return Intent.Plan{}, fmt.Errorf("%w: gem %d has an unresolved prior ruby extraction attempt", Intent.ErrPlanStale, gem.ID)
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
		return Intent.Plan{}, fmt.Errorf("%w: the selected alternative or extraction quote changed", Intent.ErrPlanStale)
	}
	if quote.MaximumRubySpend > request.MaximumRubySpend {
		return Intent.Plan{}, fmt.Errorf("ruby extraction quote %d exceeds approved maximumRubySpend %d", quote.MaximumRubySpend, request.MaximumRubySpend)
	}
	var rubyResourceID State.ResourceID
	var planningRubyObservedAt time.Time
	if quote.MaximumRubySpend > 0 {
		if request.SnapshotFingerprint == "" || request.QuoteFingerprint == "" {
			return Intent.Plan{}, fmt.Errorf("paid gem extraction requires the exact preview fingerprint and quote")
		}
		resourceID, found := input.GameData.ResourceIDForJSONKey("C2")
		if !found || resourceID <= 0 {
			return Intent.Plan{}, fmt.Errorf("official ruby resource is unavailable")
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
		step := commandStep(fmt.Sprintf("Clear equipment slot %d", slot), "eeq", payload, "eeq")
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
				return Intent.Plan{}, fmt.Errorf("cannot mount gem carrier %d while slot %d is occupied", parent.ID, parent.Slot)
			}
			equipPayload, _ := json.Marshal(struct {
				EquipmentID State.EquipmentInstanceID `json:"EID"`
				LeaderID    int64                     `json:"LID"`
				Equip       int                       `json:"E"`
			}{parent.ID, leader.id, 1})
			equipStep := commandStep(fmt.Sprintf("Mount gem carrier %d", parent.ID), "eeq", equipPayload, "eeq")
			steps = append(steps, equipStep)
			mountedBySlot[parent.Slot] = parent.ID
		}
		detachPayload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
		}{parent.ID, leader.id})
		detachStep := commandStep(fmt.Sprintf("Detach gem %d", gem.ID), "ege", detachPayload, "ege")
		if EquipmentDomain.GemFamily(gem) == EquipmentDomain.LoadoutFamilyOrdinary {
			rubyCost, costErr := EquipmentDomain.NormalGemRemovalCost(input.GameData, gem)
			if costErr != nil {
				return Intent.Plan{}, costErr
			}
			if rubyCost > 0 {
				if paidExtractionIndex >= len(paidExtractions) || paidExtractions[paidExtractionIndex].GemID != gem.ID {
					return Intent.Plan{}, fmt.Errorf("paid extraction schedule changed while planning gem %d", gem.ID)
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
					return Intent.Plan{}, fmt.Errorf("encode ruby extraction guard: %w", encodeErr)
				}
				detachStep.PreDispatchAction, detachStep.PreDispatchArguments = "equipment.reconfigure.extraction.arm", dispatchArguments
				detachStep.FinalDispatchAction, detachStep.FinalDispatchArguments = "equipment.reconfigure.extraction.dispatch", dispatchArguments
				detachStep.DefinitiveSendFailureAction, detachStep.DefinitiveSendFailureArguments = "equipment.reconfigure.extraction.disarm", dispatchArguments
				detachStep.DefinitiveResponseFailureAction, detachStep.DefinitiveResponseFailureArguments = "equipment.reconfigure.extraction.reject", dispatchArguments
				detachStep.ResponseProjectionFailureIndeterminate = true
				steps = append(steps, detachStep, Intent.Step{
					Name: "Confirm paid gem extraction", Action: "equipment.reconfigure.extraction.confirm", ActionArguments: dispatchArguments,
				})
				remainingRubySpend -= rubyCost
				paidExtractionIndex++
			} else {
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
		unequipStep := commandStep(fmt.Sprintf("Return gem carrier %d", parent.ID), "eeq", unequipPayload, "eeq")
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
			return Intent.Plan{}, fmt.Errorf("equipment slot %d is unexpectedly occupied", slot)
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{id, leader.id, 1})
		step := commandStep(fmt.Sprintf("Equip optimized slot %d", slot), "eeq", payload, "eeq")
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
		step := commandStep(fmt.Sprintf("Socket optimized gem in slot %d", slot), "bge", payload, "bge")
		steps = append(steps, step)
	}
	steps = append(steps, equipmentRefreshSteps()...)
	verificationArguments, err := json.Marshal(verification)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("encode optimized loadout verification: %w", err)
	}
	steps = append(steps, Intent.Step{
		Name: "Verify optimized loadout", Action: "equipment.reconfigure.verify", ActionArguments: verificationArguments,
	})
	claims := equipmentLeaderClaims(leader)
	if quote.MaximumRubySpend > 0 {
		claims = append(claims, "currency:"+strconv.FormatInt(int64(rubyResourceID), 10))
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Apply optimized loadout to %s %d", leader.kind, leader.id), Steps: steps,
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
			return fmt.Errorf("%s %d equipment slot %d did not match the selected loadout", leader.kind, leader.id, slot)
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
			return fmt.Errorf("%s %d gem slot %d did not match the selected loadout", leader.kind, leader.id, slot)
		}
		if !expected.Normal && actualID == expected.InstanceID {
			continue
		}
		actual, found := application.State.ReadOnlyView().Inventory.Gems[actualID]
		if !expected.Normal || !found || actualID >= 0 || actual.DefinitionID != expected.DefinitionID ||
			actual.EquipmentInstanceID != request.Equipment[key] ||
			actual.WearerKind != leader.kind || actual.WearerID != leader.id {
			return fmt.Errorf("%s %d gem slot %d did not match the selected loadout", leader.kind, leader.id, slot)
		}
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
		return time.Time{}, fmt.Errorf("ruby extraction authority is invalid")
	}
	if !gameState.Session.LoggedIn || !gameState.Session.SocketReady || gameState.Session.ConnectionGeneration == 0 {
		return time.Time{}, fmt.Errorf("%w: game session is unavailable for ruby extraction", Intent.ErrPlanStale)
	}
	observation, found := gameState.Player.ResourceObservations[resourceID]
	if !found || observation.ObservedAt.IsZero() || observation.ConnectionGeneration != gameState.Session.ConnectionGeneration ||
		!gameState.Session.ChangedAt.IsZero() && observation.ObservedAt.Before(gameState.Session.ChangedAt) ||
		observation.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(observation.ObservedAt) > equipmentRubyFreshness ||
		!requireAfter.IsZero() && !observation.ObservedAt.After(requireAfter) {
		return time.Time{}, fmt.Errorf("%w: a fresh current-session ruby balance is unavailable", Intent.ErrPlanStale)
	}
	value, found := gameState.Player.Resources[resourceID]
	if !found || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return time.Time{}, fmt.Errorf("ruby balance is unavailable")
	}
	if int64(math.Floor(value)) < required {
		return time.Time{}, fmt.Errorf("%w: ruby balance cannot cover the remaining %d-ruby extraction ceiling", Intent.ErrPlanStale, required)
	}
	return observation.ObservedAt, nil
}

func (application *Application) validateEquipmentExtractionDispatch(
	arguments equipmentExtractionDispatch,
	metadata Outbound.Metadata,
	requireMarker bool,
) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("equipment extraction state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	if arguments.ExpectedCatalogDigest == "" || gameData.Metadata().DigestSHA256 != arguments.ExpectedCatalogDigest {
		return fmt.Errorf("%w: official gem removal prices changed", Intent.ErrPlanStale)
	}
	quote := arguments.InitialQuote
	quote.Fingerprint = ""
	expectedQuoteFingerprint := EquipmentDomain.ReconfigurationQuoteFingerprint(
		arguments.Request.SnapshotFingerprint, arguments.Request.Equipment, arguments.Request.Gems, quote,
	)
	if arguments.Request.QuoteFingerprint == "" || arguments.Request.QuoteFingerprint != expectedQuoteFingerprint ||
		quote.MaximumRubySpend <= 0 || quote.MaximumRubySpend > arguments.Request.MaximumRubySpend {
		return fmt.Errorf("%w: selected extraction quote is no longer authorized", Intent.ErrPlanStale)
	}
	if len(arguments.RemainingPaidExtractions) == 0 ||
		arguments.RemainingPaidExtractions[0].GemID != arguments.GemID ||
		arguments.RemainingPaidExtractions[0].CarrierID != arguments.CarrierID ||
		arguments.RemainingPaidExtractions[0].RubyCost != arguments.RubyCost {
		return fmt.Errorf("%w: paid extraction schedule changed", Intent.ErrPlanStale)
	}
	gameState := application.State.ReadOnlyView()
	if arguments.ExpectedConnectionGeneration == 0 || gameState.Session.ConnectionGeneration != arguments.ExpectedConnectionGeneration {
		return fmt.Errorf("%w: game session changed before ruby extraction", Intent.ErrPlanStale)
	}
	leader, err := resolveLeader(gameState, arguments.Request.LeaderKind, arguments.Request.LeaderID)
	if err != nil {
		return err
	}
	if !leader.available {
		return fmt.Errorf("%w: commander became unavailable before ruby extraction", Intent.ErrPlanStale)
	}
	remaining := int64(0)
	for index, expected := range arguments.RemainingPaidExtractions {
		gem, found := gameState.Inventory.Gems[expected.GemID]
		if !found || gem.EquipmentInstanceID != expected.CarrierID || gem.DefinitionID != expected.DefinitionID ||
			gem.Level != expected.Level || EquipmentDomain.GemFamily(gem) != EquipmentDomain.LoadoutFamilyOrdinary {
			return fmt.Errorf("%w: normal gem %d is no longer on carrier %d", Intent.ErrPlanStale, expected.GemID, expected.CarrierID)
		}
		carrier, found := gameState.Inventory.Equipment[expected.CarrierID]
		if !found {
			return fmt.Errorf("%w: gem carrier %d is unavailable", Intent.ErrPlanStale, expected.CarrierID)
		}
		if index == 0 {
			if carrier.WearerKind != leader.kind || carrier.WearerID != leader.id ||
				leader.equipment[strconv.Itoa(carrier.Slot)] != carrier.ID {
				return fmt.Errorf("%w: current gem carrier %d is not mounted on the selected leader", Intent.ErrPlanStale, expected.CarrierID)
			}
		} else if carrier.WearerKind != "" && (carrier.WearerKind != leader.kind || carrier.WearerID != leader.id) {
			return fmt.Errorf("%w: future gem carrier %d is unavailable", Intent.ErrPlanStale, expected.CarrierID)
		}
		cost, costErr := EquipmentDomain.NormalGemRemovalCost(gameData, gem)
		if costErr != nil || cost != expected.RubyCost || remaining > math.MaxInt64-cost {
			return fmt.Errorf("%w: official removal price changed for gem %d", Intent.ErrPlanStale, gem.ID)
		}
		remaining += cost
	}
	if remaining != arguments.ExpectedRemainingRubySpend || remaining <= 0 {
		return fmt.Errorf("%w: remaining ruby extraction quote changed", Intent.ErrPlanStale)
	}
	currentCarrier := gameState.Inventory.Equipment[arguments.CarrierID]
	operationID := strings.TrimSpace(metadata.OperationID)
	responseToken := strings.TrimSpace(metadata.ResponseToken)
	if operationID == "" {
		return fmt.Errorf("ruby extraction operation identity is unavailable")
	}
	if requireMarker {
		expected := arguments.RemainingPaidExtractions[0]
		marker := currentCarrier.Extraction
		if marker == nil || marker.GemID != arguments.GemID || marker.RubyCost != arguments.RubyCost ||
			marker.DefinitionID != expected.DefinitionID || marker.Level != expected.Level ||
			marker.OperationID != operationID || marker.ConnectionGeneration != arguments.ExpectedConnectionGeneration ||
			marker.ResponseToken != responseToken {
			return fmt.Errorf("%w: ruby extraction dispatch marker changed", Intent.ErrPlanStale)
		}
	} else if currentCarrier.Extraction != nil {
		return fmt.Errorf("%w: gem %d has an unresolved prior ruby extraction attempt", Intent.ErrPlanStale, arguments.GemID)
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
			return nil, false, fmt.Errorf("%w: gem carrier changed before ruby extraction", Intent.ErrPlanStale)
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
			return nil, false, fmt.Errorf("%w: ruby extraction dispatch marker changed", Intent.ErrPlanStale)
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
				return nil, false, fmt.Errorf("%w: paid gem extraction was not authoritatively observed", Intent.ErrPlanStale)
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
		return Intent.Plan{}, fmt.Errorf("targetLevel must be between 1 and %d", maxEquipmentUpgradeLevel)
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
			return Intent.Plan{}, fmt.Errorf("equipment %d is not in current state", request.ItemID)
		}
		var supported bool
		maximumLevel, relicUpgrade, supported = equipmentUpgradeLevelCap(item)
		if !supported {
			return Intent.Plan{}, fmt.Errorf("equipment %d has an unsupported or unverified enchantment type", request.ItemID)
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
			return Intent.Plan{}, fmt.Errorf("relic gem %d is not in current state", request.ItemID)
		}
		currentLevel = gem.Level
		wearerKind, wearerID = gem.WearerKind, gem.WearerID
		if wearerKind == "" && gem.EquipmentInstanceID != 0 {
			carrier, found := input.State.Inventory.Equipment[gem.EquipmentInstanceID]
			if !found {
				return Intent.Plan{}, fmt.Errorf("relic gem %d references missing equipment %d", request.ItemID, gem.EquipmentInstanceID)
			}
			wearerKind, wearerID = carrier.WearerKind, carrier.WearerID
		}
		payload, _ = json.Marshal(struct {
			CostMode  int   `json:"C2"`
			ItemID    int64 `json:"RIID"`
			Equipment int   `json:"EQ"`
		}{0, request.ItemID, 0})
	default:
		return Intent.Plan{}, fmt.Errorf("itemKind must be equipment or gem")
	}
	if request.TargetLevel > maximumLevel {
		return Intent.Plan{}, fmt.Errorf("targetLevel cannot exceed %d for this %s", maximumLevel, request.ItemKind)
	}
	if request.TargetLevel <= currentLevel {
		return Intent.Plan{}, fmt.Errorf("targetLevel must be above current level %d", currentLevel)
	}
	claims, err := equipmentUpgradeClaims(input.State, request.ItemKind, request.ItemID, wearerKind, wearerID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if err := application.verifyEquipmentCoinReserve(context.Background(), nil); err != nil {
		return Intent.Plan{}, err
	}
	delay := application.equipmentUpgradeDelay()
	steps := make([]Intent.Step, 0, (request.TargetLevel-currentLevel)*2+4)
	if relicUpgrade {
		steps = append(steps, equipmentUpgradeContextStep())
	}
	for level := currentLevel + 1; level <= request.TargetLevel; level++ {
		guard := Intent.Step{Name: "Verify coin reserve", Action: "equipment.verify_coin_reserve", DelayMillis: delay}
		steps = append(steps, Intent.RebuildOnResume(guard))
		steps = append(steps, Intent.Step{
			Name: fmt.Sprintf("Upgrade %s to level %d", request.ItemKind, level), Opcode: upgradeOpcode, Payload: payload,
			AwaitOpcode: upgradeOpcode, TimeoutMillis: 8_000, SuccessCodes: []int{0},
			// The game commits its separate coin/currency updates before returning
			// 227 for a consumed failed roll. Recheck the reserve, then repeat this
			// exact level; never count the roll as successful or stale progress.
			ResponseRetry: &Intent.ResponseRetryPolicy{
				Codes: []int{227}, GuardAction: "equipment.verify_coin_reserve", DelayMillis: delay,
			},
			Command: Protocol.Command{Opcode: upgradeOpcode, Payload: payload},
		})
	}
	steps = append(steps, equipmentRefreshSteps()...)
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Upgrade %s %d from level %d to %d", request.ItemKind, request.ItemID, currentLevel, request.TargetLevel),
		Steps:   steps,
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
			return nil, fmt.Errorf("%s %d is worn by commander %d, which is missing from current state", itemKind, itemID, wearerID)
		}
		if !commander.Available || State.CommanderHasActiveMovementAt(gameState, commander.ID, time.Now().UTC()) {
			return nil, fmt.Errorf("%s %d cannot be upgraded while commander %d is travelling", itemKind, itemID, wearerID)
		}
	case "castellan":
		if _, found := gameState.Castellans[State.CastellanID(wearerID)]; !found {
			return nil, fmt.Errorf("%s %d is worn by castellan %d, which is missing from current state", itemKind, itemID, wearerID)
		}
	default:
		return nil, fmt.Errorf("%s %d has unsupported wearer kind %q", itemKind, itemID, wearerKind)
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
		return Intent.Plan{}, fmt.Errorf("keepStars must be between 0 and 42")
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
			step := commandStep(fmt.Sprintf("Sell equipment %d", id), "seq", payload, "seq")
			steps = append(steps, step)
		}
		count = len(ids)
		if count > 0 {
			steps = append(steps, commandStep("Refresh equipment storage", "gei", json.RawMessage(`{}`), "gei"))
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
				step := commandStep(fmt.Sprintf("Sell gem %d", id), "sge", payload, "sge")
				steps = append(steps, step)
				count++
			}
		}
		if count > 0 {
			steps = append(steps, commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm"))
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
			step := commandStep(fmt.Sprintf("Sell relic gem %d", id), "sge", payload, "sge")
			steps = append(steps, step)
		}
		count = len(ids)
		if count > 0 {
			steps = append(steps, commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm"))
		}
	default:
		return Intent.Plan{}, fmt.Errorf("unknown equipment sale category %q", request.Category)
	}
	return Intent.Plan{
		Claims: []string{"game:equipment"}, Summary: fmt.Sprintf("Sell %d item(s) from %s", count, request.Category), Steps: steps,
	}, nil
}

func (application *Application) verifyEquipmentCoinReserve(_ context.Context, _ json.RawMessage) error {
	threshold, _ := application.equipmentUpgradeSettings()
	if threshold <= 0 {
		return nil
	}
	coins := application.State.ReadOnlyView().Player.Resources[State.ResourceID(1)]
	if coins <= threshold {
		return fmt.Errorf("coins under upgrade reserve (%.0f <= %.0f)", coins, threshold)
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
		commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm"),
		commandStep("Refresh equipment storage", "gei", json.RawMessage(`{}`), "gei"),
		commandStep("Refresh leader loadouts", "gli", json.RawMessage(`{}`), "gli"),
	}
}

func equipmentMutationRefreshSteps() []Intent.Step {
	return []Intent.Step{
		commandStep("Refresh leader loadouts", "gli", json.RawMessage(`{}`), "gli"),
		commandStep("Refresh equipment storage", "gei", json.RawMessage(`{}`), "gei"),
	}
}

func gemMutationRefreshSteps() []Intent.Step {
	return []Intent.Step{
		commandStep("Refresh leader loadouts", "gli", json.RawMessage(`{}`), "gli"),
		commandStep("Refresh gem storage", "ggm", json.RawMessage(`{}`), "ggm"),
	}
}

func resolveLeader(gameState State.GameState, kind string, id int64) (resolvedLeader, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "commander":
		leader, ok := gameState.Commanders[State.CommanderID(id)]
		if !ok {
			return resolvedLeader{}, fmt.Errorf("commander %d is not in current state", id)
		}
		return resolvedLeader{kind: kind, id: id, available: leader.Available, equipment: leader.Equipment, gems: leader.Gems}, nil
	case "castellan":
		leader, ok := gameState.Castellans[State.CastellanID(id)]
		if !ok {
			return resolvedLeader{}, fmt.Errorf("castellan %d is not in current state", id)
		}
		return resolvedLeader{kind: kind, id: id, available: true, equipment: leader.Equipment, gems: leader.Gems}, nil
	default:
		return resolvedLeader{}, fmt.Errorf("leaderKind must be commander or castellan")
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
		return fmt.Errorf("%s storage is stale; run equipment.refresh before selling", opcode)
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
