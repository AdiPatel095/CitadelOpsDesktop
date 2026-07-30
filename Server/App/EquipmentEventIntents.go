package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type equipmentEventTier struct {
	setID int64
	label string
	rank  int
}

type equipmentEventConfig struct {
	key   string
	label string
	tiers []equipmentEventTier
}

type officialEquipmentEventSet struct {
	equipmentBySlot map[int]map[State.EquipmentID]struct{}
	gemDefinitions  []State.GemID
}

type equipmentEventGemSource struct {
	definitionID State.GemID
	instanceID   State.GemInstanceID
	carrierID    State.EquipmentInstanceID
}

type equipmentEventGemAssignment struct {
	source          equipmentEventGemSource
	alreadyAttached bool
}

type equipmentEventCandidate struct {
	config      equipmentEventConfig
	tier        equipmentEventTier
	equipment   map[int]State.EquipmentInstance
	gems        map[int]equipmentEventGemAssignment
	gearCount   int
	gemCount    int
	completeSet bool
}

func planEquipmentEventApply(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CommanderID int64  `json:"commanderId"`
		Event       string `json:"event"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	leader, err := resolveLeader(input.State, "commander", request.CommanderID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if !leader.available {
		return Intent.Plan{}, fmt.Errorf("commander %d is busy", leader.id)
	}
	config, err := resolveEquipmentEventConfig(request.Event)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	setIDs := make([]int64, 0, len(config.tiers))
	for _, tier := range config.tiers {
		setIDs = append(setIDs, tier.setID)
	}
	officialSets, err := loadOfficialEquipmentEventSets(input.GameData, setIDs)
	if err != nil {
		return Intent.Plan{}, err
	}

	candidates := make([]equipmentEventCandidate, 0, len(config.tiers))
	for _, tier := range config.tiers {
		officialSet, found := officialSets[tier.setID]
		if !found || len(officialSet.equipmentBySlot) < len(baseEquipmentSlots) || len(officialSet.gemDefinitions) < 4 {
			return Intent.Plan{}, fmt.Errorf("%s set %d is incomplete in the current official game data", config.label, tier.setID)
		}
		candidates = append(candidates, resolveEquipmentEventCandidate(input.State, leader, config, tier, officialSet))
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		return betterEquipmentEventCandidate(candidates[left], candidates[right])
	})
	selected := candidates[0]
	if selected.gearCount == 0 {
		return Intent.Plan{}, fmt.Errorf("no available %s equipment is in storage or already on commander %d", config.label, leader.id)
	}
	return buildEquipmentEventPlan(input.State, leader, selected)
}

func resolveEquipmentEventConfig(raw string) (equipmentEventConfig, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.NewReplacer("-", "_", " ", "_").Replace(key)
	switch key {
	case "nomad", "nomad_invasion":
		return equipmentEventConfig{
			key: "nomad", label: "Nomad Invasion",
			tiers: []equipmentEventTier{{setID: 1087}},
		}, nil
	case "samurai", "samurai_invasion":
		return equipmentEventConfig{
			key: "samurai", label: "Samurai Invasion",
			tiers: []equipmentEventTier{{setID: 1088}},
		}, nil
	case "berimond", "battle_for_berimond":
		return equipmentEventConfig{
			key: "berimond", label: "Battle for Berimond",
			tiers: []equipmentEventTier{{setID: 1089}},
		}, nil
	case "foreign_lords", "foreign", "bloodcrow", "glory":
		return equipmentEventConfig{
			key: "foreign_lords", label: "Foreign Lords and Bloodcrows",
			tiers: []equipmentEventTier{{setID: 1090}},
		}, nil
	case "hollow_moon_pvp", "hollow_moon", "pvp":
		return equipmentEventConfig{
			key: "hollow_moon_pvp", label: "Hollow Moon PvP",
			tiers: []equipmentEventTier{
				{setID: 1096, label: "Gold", rank: 3},
				{setID: 1095, label: "Silver", rank: 2},
				{setID: 1094, label: "Bronze", rank: 1},
			},
		}, nil
	default:
		return equipmentEventConfig{}, fmt.Errorf("unknown commander equipment event %q", strings.TrimSpace(raw))
	}
}

func loadOfficialEquipmentEventSets(gameData *GameData.Store, setIDs []int64) (map[int64]officialEquipmentEventSet, error) {
	sets := make(map[int64]officialEquipmentEventSet, len(setIDs))
	wanted := make(map[int64]struct{}, len(setIDs))
	for _, setID := range setIDs {
		wanted[setID] = struct{}{}
		sets[setID] = officialEquipmentEventSet{equipmentBySlot: map[int]map[State.EquipmentID]struct{}{}}
	}
	equipmentCatalog, err := gameData.Catalog("equipments")
	if err != nil {
		return nil, err
	}
	for _, raw := range equipmentCatalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		setID, setFound := record.Int64("setID")
		if _, found := wanted[setID]; !setFound || !found {
			continue
		}
		wearerID, _ := record.Int64("wearerID")
		equipmentID, equipmentFound := record.Int64("equipmentID")
		slotID, slotFound := record.Int64("slotID")
		slot := int(slotID)
		if wearerID != 2 || !equipmentFound || equipmentID <= 0 || !slotFound || !validBaseSlot(slot) {
			continue
		}
		current := sets[setID]
		if current.equipmentBySlot[slot] == nil {
			current.equipmentBySlot[slot] = map[State.EquipmentID]struct{}{}
		}
		current.equipmentBySlot[slot][State.EquipmentID(equipmentID)] = struct{}{}
		sets[setID] = current
	}
	gemCatalog, err := gameData.Catalog("gems")
	if err != nil {
		return nil, err
	}
	for _, raw := range gemCatalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		setID, setFound := record.Int64("setID")
		if _, found := wanted[setID]; !setFound || !found {
			continue
		}
		wearerID, _ := record.Int64("wearerID")
		gemID, gemFound := record.Int64("gemID")
		if wearerID != 2 || !gemFound || gemID <= 0 {
			continue
		}
		current := sets[setID]
		current.gemDefinitions = append(current.gemDefinitions, State.GemID(gemID))
		sets[setID] = current
	}
	for setID, current := range sets {
		sort.Slice(current.gemDefinitions, func(left, right int) bool {
			return current.gemDefinitions[left] < current.gemDefinitions[right]
		})
		sets[setID] = current
	}
	return sets, nil
}

func resolveEquipmentEventCandidate(
	gameState State.GameState,
	leader resolvedLeader,
	config equipmentEventConfig,
	tier equipmentEventTier,
	officialSet officialEquipmentEventSet,
) equipmentEventCandidate {
	gemsByEquipment := make(map[State.EquipmentInstanceID]State.GemInstance, len(gameState.Inventory.Gems))
	for _, gem := range gameState.Inventory.Gems {
		if gem.EquipmentInstanceID > 0 {
			gemsByEquipment[gem.EquipmentInstanceID] = gem
		}
	}
	gemDefinitions := make(map[State.GemID]struct{}, len(officialSet.gemDefinitions))
	for _, definitionID := range officialSet.gemDefinitions {
		gemDefinitions[definitionID] = struct{}{}
	}
	equipment := map[int]State.EquipmentInstance{}
	for _, item := range gameState.Inventory.Equipment {
		if !equipmentEventItemAvailable(item, leader) {
			continue
		}
		definitions := officialSet.equipmentBySlot[item.Slot]
		if _, found := definitions[item.DefinitionID]; !found {
			continue
		}
		current, found := equipment[item.Slot]
		if !found || betterEquipmentEventItem(item, current, leader, gemsByEquipment, gemDefinitions) {
			equipment[item.Slot] = item
		}
	}
	assignments := resolveEquipmentEventGems(gameState, leader, equipment, officialSet.gemDefinitions, gemsByEquipment)
	return equipmentEventCandidate{
		config: config, tier: tier, equipment: equipment, gems: assignments,
		gearCount: len(equipment), gemCount: len(assignments),
		completeSet: len(equipment) == len(baseEquipmentSlots) && len(assignments) == 4,
	}
}

func equipmentEventItemAvailable(item State.EquipmentInstance, leader resolvedLeader) bool {
	if item.ID <= 0 || item.TypeID != 2 || !validBaseSlot(item.Slot) {
		return false
	}
	return item.WearerKind == "" || item.WearerKind == leader.kind && item.WearerID == leader.id
}

func betterEquipmentEventItem(
	candidate State.EquipmentInstance,
	current State.EquipmentInstance,
	leader resolvedLeader,
	gemsByEquipment map[State.EquipmentInstanceID]State.GemInstance,
	gemDefinitions map[State.GemID]struct{},
) bool {
	candidateGem, candidateHasGem := gemsByEquipment[candidate.ID]
	currentGem, currentHasGem := gemsByEquipment[current.ID]
	_, candidateHasSetGem := gemDefinitions[candidateGem.DefinitionID]
	_, currentHasSetGem := gemDefinitions[currentGem.DefinitionID]
	candidateHasSetGem = candidateHasGem && candidateHasSetGem
	currentHasSetGem = currentHasGem && currentHasSetGem
	if candidateHasSetGem != currentHasSetGem {
		return candidateHasSetGem
	}
	candidateWorn := candidate.WearerKind == leader.kind && candidate.WearerID == leader.id
	currentWorn := current.WearerKind == leader.kind && current.WearerID == leader.id
	if candidateWorn != currentWorn {
		return candidateWorn
	}
	if candidateHasGem != currentHasGem {
		return !candidateHasGem
	}
	if candidate.Level != current.Level {
		return candidate.Level > current.Level
	}
	return candidate.ID < current.ID
}

func resolveEquipmentEventGems(
	gameState State.GameState,
	leader resolvedLeader,
	equipment map[int]State.EquipmentInstance,
	definitions []State.GemID,
	gemsByEquipment map[State.EquipmentInstanceID]State.GemInstance,
) map[int]equipmentEventGemAssignment {
	assignments := map[int]equipmentEventGemAssignment{}
	usedDefinitions := map[State.GemID]struct{}{}
	for slot := 1; slot <= 4; slot++ {
		item, found := equipment[slot]
		if !found {
			continue
		}
		gem, found := gemsByEquipment[item.ID]
		if !found || !containsEventGemDefinition(definitions, gem.DefinitionID) {
			continue
		}
		if _, duplicate := usedDefinitions[gem.DefinitionID]; duplicate {
			continue
		}
		assignments[slot] = equipmentEventGemAssignment{
			source: equipmentEventGemSource{
				definitionID: gem.DefinitionID,
				instanceID:   gem.ID,
				carrierID:    item.ID,
			},
			alreadyAttached: true,
		}
		usedDefinitions[gem.DefinitionID] = struct{}{}
	}
	openSlots := make([]int, 0, 4-len(assignments))
	for slot := 1; slot <= 4; slot++ {
		if _, hasEquipment := equipment[slot]; !hasEquipment {
			continue
		}
		if _, assigned := assignments[slot]; !assigned {
			openSlots = append(openSlots, slot)
		}
	}
	for _, definitionID := range definitions {
		if len(openSlots) == 0 {
			break
		}
		if _, used := usedDefinitions[definitionID]; used {
			continue
		}
		source, found := resolveEquipmentEventGemSource(gameState, leader, definitionID, equipment)
		if !found {
			continue
		}
		slot := openSlots[0]
		openSlots = openSlots[1:]
		assignments[slot] = equipmentEventGemAssignment{source: source}
		usedDefinitions[definitionID] = struct{}{}
	}
	return assignments
}

func resolveEquipmentEventGemSource(
	gameState State.GameState,
	leader resolvedLeader,
	definitionID State.GemID,
	selectedEquipment map[int]State.EquipmentInstance,
) (equipmentEventGemSource, bool) {
	if gameState.Inventory.GemStacks[definitionID] > 0 {
		return equipmentEventGemSource{definitionID: definitionID}, true
	}
	var free *State.GemInstance
	var attached *State.GemInstance
	for _, gem := range gameState.Inventory.Gems {
		if gem.DefinitionID != definitionID {
			continue
		}
		candidate := gem
		if gem.EquipmentInstanceID <= 0 {
			if gem.WearerKind == "" && (free == nil || candidate.ID < free.ID) {
				free = &candidate
			}
			continue
		}
		parent, found := gameState.Inventory.Equipment[gem.EquipmentInstanceID]
		if !found || !equipmentEventItemAvailable(parent, leader) || parent.Slot < 1 || parent.Slot > 4 {
			continue
		}
		if selected, selectedSlot := selectedEquipment[parent.Slot]; selectedSlot && selected.ID == parent.ID {
			continue
		}
		if attached == nil || candidate.ID < attached.ID {
			attached = &candidate
		}
	}
	if free != nil {
		return equipmentEventGemSource{
			definitionID: definitionID,
			instanceID:   free.ID,
		}, true
	}
	if attached != nil {
		return equipmentEventGemSource{
			definitionID: definitionID,
			instanceID:   attached.ID,
			carrierID:    attached.EquipmentInstanceID,
		}, true
	}
	return equipmentEventGemSource{}, false
}

func containsEventGemDefinition(definitions []State.GemID, definitionID State.GemID) bool {
	index := sort.Search(len(definitions), func(index int) bool { return definitions[index] >= definitionID })
	return index < len(definitions) && definitions[index] == definitionID
}

func betterEquipmentEventCandidate(candidate equipmentEventCandidate, current equipmentEventCandidate) bool {
	if candidate.completeSet != current.completeSet {
		return candidate.completeSet
	}
	if candidate.gearCount != current.gearCount {
		return candidate.gearCount > current.gearCount
	}
	if candidate.gemCount != current.gemCount {
		return candidate.gemCount > current.gemCount
	}
	return candidate.tier.rank > current.tier.rank
}

func buildEquipmentEventPlan(
	gameState State.GameState,
	leader resolvedLeader,
	candidate equipmentEventCandidate,
) (Intent.Plan, error) {
	steps := make([]Intent.Step, 0, 36)
	for _, slot := range baseEquipmentSlots {
		id := leader.equipment[strconv.Itoa(slot)]
		if id <= 0 {
			continue
		}
		item, found := gameState.Inventory.Equipment[id]
		if !found || item.WearerKind != leader.kind || item.WearerID != leader.id {
			return Intent.Plan{}, fmt.Errorf("equipment slot %d is inconsistent with current commander state", slot)
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{item.ID, leader.id, 0})
		steps = append(steps, commandStep(fmt.Sprintf("Clear commander equipment slot %d", slot), "eeq", payload, "eeq"))
	}

	gemsByEquipment := make(map[State.EquipmentInstanceID]State.GemInstance, len(gameState.Inventory.Gems))
	for _, gem := range gameState.Inventory.Gems {
		if gem.EquipmentInstanceID > 0 {
			gemsByEquipment[gem.EquipmentInstanceID] = gem
		}
	}
	preservedGems := map[State.GemInstanceID]struct{}{}
	detachCarriers := map[State.EquipmentInstanceID]State.EquipmentInstance{}
	for _, assignment := range candidate.gems {
		if assignment.alreadyAttached {
			preservedGems[assignment.source.instanceID] = struct{}{}
			continue
		}
		if assignment.source.carrierID > 0 {
			parent, found := gameState.Inventory.Equipment[assignment.source.carrierID]
			if !found || !equipmentEventItemAvailable(parent, leader) || parent.Slot < 1 || parent.Slot > 4 {
				return Intent.Plan{}, fmt.Errorf("event gem carrier %d is unavailable", assignment.source.carrierID)
			}
			detachCarriers[parent.ID] = parent
		}
	}
	for slot := 1; slot <= 4; slot++ {
		item, found := candidate.equipment[slot]
		if !found {
			continue
		}
		gem, hasGem := gemsByEquipment[item.ID]
		if !hasGem {
			continue
		}
		if _, preserved := preservedGems[gem.ID]; !preserved {
			detachCarriers[item.ID] = item
		}
	}
	carriers := make([]State.EquipmentInstance, 0, len(detachCarriers))
	for _, carrier := range detachCarriers {
		carriers = append(carriers, carrier)
	}
	sort.Slice(carriers, func(left, right int) bool {
		if carriers[left].Slot != carriers[right].Slot {
			return carriers[left].Slot < carriers[right].Slot
		}
		return carriers[left].ID < carriers[right].ID
	})
	for _, carrier := range carriers {
		equipPayload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{carrier.ID, leader.id, 1})
		steps = append(steps, commandStep(fmt.Sprintf("Mount event gem carrier %d", carrier.ID), "eeq", equipPayload, "eeq"))
		detachPayload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
		}{carrier.ID, leader.id})
		steps = append(steps, commandStep(fmt.Sprintf("Detach gem from carrier %d", carrier.ID), "ege", detachPayload, "ege"))
		unequipPayload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{carrier.ID, leader.id, 0})
		steps = append(steps, commandStep(fmt.Sprintf("Return event gem carrier %d", carrier.ID), "eeq", unequipPayload, "eeq"))
	}

	for _, slot := range baseEquipmentSlots {
		item, found := candidate.equipment[slot]
		if !found {
			continue
		}
		payload, _ := json.Marshal(struct {
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Equip       int                       `json:"E"`
		}{item.ID, leader.id, 1})
		steps = append(steps, commandStep(fmt.Sprintf("Equip %s slot %d", candidate.config.label, slot), "eeq", payload, "eeq"))
	}
	for slot := 1; slot <= 4; slot++ {
		assignment, found := candidate.gems[slot]
		if !found || assignment.alreadyAttached {
			continue
		}
		commandGemID := int64(assignment.source.definitionID)
		relicGem := 0
		if assignment.source.instanceID > 0 {
			commandGemID = int64(assignment.source.instanceID)
			relicGem = 1
		}
		payload, _ := json.Marshal(struct {
			GemID       int64                     `json:"GID"`
			EquipmentID State.EquipmentInstanceID `json:"EID"`
			LeaderID    int64                     `json:"LID"`
			Mode        int                       `json:"M"`
			RelicGem    int                       `json:"RGEM"`
		}{commandGemID, candidate.equipment[slot].ID, leader.id, 0, relicGem})
		steps = append(steps, commandStep(fmt.Sprintf("Socket %s gem in slot %d", candidate.config.label, slot), "bge", payload, "bge"))
	}
	steps = append(steps, equipmentRefreshSteps()...)
	tierLabel := ""
	if candidate.tier.label != "" {
		tierLabel = " " + candidate.tier.label
	}
	return Intent.Plan{
		Claims: equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf(
			"Apply%s %s loadout to commander %d with %d equipment and %d gems",
			tierLabel, candidate.config.label, leader.id, candidate.gearCount, candidate.gemCount,
		),
		Steps: steps,
	}, nil
}
