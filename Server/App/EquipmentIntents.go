package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	equipmentRefreshFreshness = 60 * time.Second
	maxEquipmentUpgradeLevel  = 50
	defaultEquipmentStepDelay = 50
)

var baseEquipmentSlots = []int{1, 2, 3, 4, 6}

type resolvedLeader struct {
	kind      string
	id        int64
	available bool
	equipment map[string]State.EquipmentInstanceID
	gems      map[string]State.GemInstanceID
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
		Summary: fmt.Sprintf("Swap %d base equipment item(s) between %s %d and %d", len(firstItems)+len(secondItems), first.kind, first.id, second.id),
		Steps:   steps,
	}, nil
}

func planEquipmentReconfigure(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LeaderKind string                               `json:"leaderKind"`
		LeaderID   int64                                `json:"leaderId"`
		Equipment  map[string]State.EquipmentInstanceID `json:"equipment"`
		Gems       map[string]State.GemInstanceID       `json:"gems"`
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
	selectedEquipment := map[State.EquipmentInstanceID]struct{}{}
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
		if _, duplicate := selectedEquipment[id]; duplicate {
			return Intent.Plan{}, fmt.Errorf("equipment %d appears in more than one slot", id)
		}
		selectedEquipment[id] = struct{}{}
	}
	selectedGems := map[State.GemInstanceID]struct{}{}
	for slot := 1; slot <= 4; slot++ {
		id := request.Gems[strconv.Itoa(slot)]
		if id == 0 {
			continue
		}
		gem, ok := input.State.Inventory.Gems[id]
		if !ok {
			return Intent.Plan{}, fmt.Errorf("gem %d is not in current state", id)
		}
		if gem.WearerKind != "" && (gem.WearerKind != leader.kind || gem.WearerID != leader.id) {
			return Intent.Plan{}, fmt.Errorf("gem %d is worn by another leader", id)
		}
		if _, duplicate := selectedGems[id]; duplicate {
			return Intent.Plan{}, fmt.Errorf("gem %d appears in more than one slot", id)
		}
		selectedGems[id] = struct{}{}
	}

	// Index current sockets once. The previous planner scanned every stored gem
	// for each target slot, then rebuilt every equipment slot even when the
	// preview already matched it.
	gemsByEquipment := make(map[State.EquipmentInstanceID]State.GemInstance, len(input.State.Inventory.Gems))
	for _, gem := range input.State.Inventory.Gems {
		if gem.EquipmentInstanceID > 0 {
			gemsByEquipment[gem.EquipmentInstanceID] = gem
		}
	}
	alreadyAttached := map[int]bool{}
	gemsToDetach := map[State.GemInstanceID]State.GemInstance{}
	for slot := 1; slot <= 4; slot++ {
		gemID := request.Gems[strconv.Itoa(slot)]
		destinationEquipmentID := request.Equipment[strconv.Itoa(slot)]
		if attached, found := gemsByEquipment[destinationEquipmentID]; found && attached.ID != gemID {
			gemsToDetach[attached.ID] = attached
		}
		if gemID == 0 {
			continue
		}
		gem := input.State.Inventory.Gems[gemID]
		if gem.EquipmentInstanceID == destinationEquipmentID {
			alreadyAttached[slot] = true
			continue
		}
		if gem.EquipmentInstanceID > 0 {
			gemsToDetach[gem.ID] = gem
		}
	}

	// Clear only slots that change. A detached gem can require an otherwise
	// retained slot to be temporarily clear so its carrier can be mounted.
	clearSlots := map[int]bool{}
	detachCarrierCounts := map[int]int{}
	for _, slot := range baseEquipmentSlots {
		if leader.equipment[strconv.Itoa(slot)] != request.Equipment[strconv.Itoa(slot)] {
			clearSlots[slot] = true
		}
	}
	for _, gem := range gemsToDetach {
		parent, found := input.State.Inventory.Equipment[gem.EquipmentInstanceID]
		if !found || parent.WearerKind != "" && (parent.WearerKind != leader.kind || parent.WearerID != leader.id) {
			return Intent.Plan{}, fmt.Errorf("cannot detach gem %d from unavailable equipment %d", gem.ID, gem.EquipmentInstanceID)
		}
		parentSlot := strconv.Itoa(parent.Slot)
		detachCarrierCounts[parent.Slot]++
		if leader.equipment[parentSlot] == request.Equipment[parentSlot] && request.Equipment[parentSlot] != parent.ID {
			clearSlots[parent.Slot] = true
		}
	}

	steps := make([]Intent.Step, 0, 32)
	mountedBySlot := map[int]State.EquipmentInstanceID{}
	for _, slot := range baseEquipmentSlots {
		id := leader.equipment[strconv.Itoa(slot)]
		if id <= 0 {
			continue
		}
		if !clearSlots[slot] {
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
	detachIDs := make([]State.GemInstanceID, 0, len(gemsToDetach))
	for id := range gemsToDetach {
		detachIDs = append(detachIDs, id)
	}
	sort.Slice(detachIDs, func(left, right int) bool { return detachIDs[left] < detachIDs[right] })
	for _, gemID := range detachIDs {
		gem := gemsToDetach[gemID]
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
		steps = append(steps, detachStep)
		if request.Equipment[strconv.Itoa(parent.Slot)] == parent.ID && detachCarrierCounts[parent.Slot] == 1 {
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
		if gemID == 0 || alreadyAttached[slot] {
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
	return Intent.Plan{
		Claims:  equipmentLeaderClaims(leader),
		Summary: fmt.Sprintf("Apply optimized loadout to %s %d", leader.kind, leader.id), Steps: steps,
	}, nil
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
	eqFlag := 0
	awaitOpcodes := []string{"ere", "gsue", "guse"}
	switch request.ItemKind {
	case "equipment":
		item, ok := input.State.Inventory.Equipment[State.EquipmentInstanceID(request.ItemID)]
		if !ok || request.ItemID <= 0 {
			return Intent.Plan{}, fmt.Errorf("equipment %d is not in current state", request.ItemID)
		}
		currentLevel = item.Level
		eqFlag = 1
		awaitOpcodes = []string{"ere", "eqe"}
	case "gem":
		gem, ok := input.State.Inventory.Gems[State.GemInstanceID(request.ItemID)]
		if !ok || request.ItemID <= 0 {
			return Intent.Plan{}, fmt.Errorf("relic gem %d is not in current state", request.ItemID)
		}
		currentLevel = gem.Level
	default:
		return Intent.Plan{}, fmt.Errorf("itemKind must be equipment or gem")
	}
	if request.TargetLevel <= currentLevel {
		return Intent.Plan{}, fmt.Errorf("targetLevel must be above current level %d", currentLevel)
	}
	if err := application.verifyEquipmentCoinReserve(context.Background(), nil); err != nil {
		return Intent.Plan{}, err
	}
	delay := application.equipmentUpgradeDelay()
	steps := []Intent.Step{equipmentUpgradeContextStep()}
	for level := currentLevel + 1; level <= request.TargetLevel; level++ {
		guard := Intent.Step{Name: "Verify coin reserve", Action: "equipment.verify_coin_reserve", DelayMillis: delay}
		steps = append(steps, Intent.RebuildOnResume(guard))
		payload, _ := json.Marshal(struct {
			CostMode  int   `json:"C2"`
			ItemID    int64 `json:"RIID"`
			Equipment int   `json:"EQ"`
		}{0, request.ItemID, eqFlag})
		steps = append(steps, Intent.Step{
			Name: fmt.Sprintf("Upgrade %s to level %d", request.ItemKind, level), Opcode: "ere", Payload: payload,
			AwaitOpcodes: awaitOpcodes, TimeoutMillis: 8_000, SuccessCodes: []int{0},
			Command: Protocol.Command{Opcode: "ere", Payload: payload},
		})
	}
	steps = append(steps, equipmentRefreshSteps()...)
	return Intent.Plan{
		Claims:  []string{"game:equipment", request.ItemKind + ":" + strconv.FormatInt(request.ItemID, 10)},
		Summary: fmt.Sprintf("Upgrade %s %d from level %d to %d", request.ItemKind, request.ItemID, currentLevel, request.TargetLevel),
		Steps:   steps,
	}, nil
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
			if int64(id) < 450 || request.SellPost2026 {
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
	observation, ok := gameState.Observations[opcode]
	observedAt := observation.SuccessfulInboundAt()
	if !ok || observedAt.IsZero() || time.Since(observedAt) > equipmentRefreshFreshness {
		return fmt.Errorf("%s storage is stale; run equipment.refresh before selling", opcode)
	}
	return nil
}

func equipmentMatchesSale(item State.EquipmentInstance, category string, sellLookItems bool, sellPost2026 bool, keepStars int) bool {
	switch category {
	case "non_relic_equipment":
		if item.RarityID == 5 || item.RarityID == 15 || item.Slot == 5 && !sellLookItems {
			return false
		}
		// Some ordinary storage rows do not include a catalog definition. The
		// parser then retains the instance ID as the only stable identity; that
		// must not make an otherwise eligible item look like post-2026 equipment.
		definitionUnknown := item.ID > 0 && item.DefinitionID == State.EquipmentID(item.ID)
		return definitionUnknown || int64(item.DefinitionID) < 1366 || sellPost2026
	case "relic1_equipment":
		return item.RarityID == 5 && len(item.Effects) < 4
	case "relic2_equipment":
		standard := item.RarityID == 5 && len(item.Effects) == 4 && item.Slot != 6
		hero := item.RarityID == 15 && len(item.Effects) == 6 && item.Slot == 6
		return (standard || hero) && effectStars(item.Effects) < keepStars
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
