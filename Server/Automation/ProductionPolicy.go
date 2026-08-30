package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type ProductionPolicy struct {
	id            string
	enabledKey    string
	section       string
	lineID        int
	definitionKey string
	lastCastleID  State.CastleID
}

type productionSettings struct {
	Mode                      string                      `json:"mode"`
	CheckIntervalSec          int                         `json:"checkIntervalSec"`
	RecruitLevel10OnTitleLoss bool                        `json:"recruitLevel10OnTitleLoss"`
	GlobalItems               []productionTarget          `json:"globalItems"`
	Castles                   map[string]productionCastle `json:"castles"`
}

type productionTarget struct {
	ID     int64 `json:"id"`
	MinID  int64 `json:"minId,omitempty"`
	MaxID  int64 `json:"maxId,omitempty"`
	Amount int64 `json:"amount,omitempty"`
}

type productionCastle struct {
	Enabled bool               `json:"enabled"`
	Items   []productionTarget `json:"items"`
	Cursor  int                `json:"cursor,omitempty"`
}

const recruitmentStackCapacityEffectID = 189
const productionBaseQueueCapacity = 2

type productionTargetAvailability uint8

const (
	productionTargetAvailable productionTargetAvailability = iota
	productionTargetUnavailable
	productionTargetGloryTitleUnknown
	productionTargetGloryTitlePaused
)

type productionGloryTitleGuard struct {
	TitleGatedDefinitionID int64
	RequiredGloryTitleID   int64
	TitleLossFallback      bool
}

func NewRecruitPolicy() *ProductionPolicy {
	return &ProductionPolicy{
		id: "autoRecruit", enabledKey: "recruit_troops", section: "automation.recruitTroops",
		lineID: 0, definitionKey: "unit",
	}
}

func NewToolPolicy() *ProductionPolicy {
	return &ProductionPolicy{
		id: "autoTool", enabledKey: "auto_tool", section: "automation.autoTool",
		lineID: 1, definitionKey: "tool",
	}
}

func (policy *ProductionPolicy) ID() string { return policy.id }

func (policy *ProductionPolicy) EnabledKey() string { return policy.enabledKey }

func (policy *ProductionPolicy) WakeDomains() []string {
	if policy.id == "autoRecruit" {
		return []string{"production", "subscriptions", "glory-title", "kingdom-transport", "alliance-help"}
	}
	return []string{"production", "subscriptions", "kingdom-transport"}
}

func (policy *ProductionPolicy) WakeSections() []string { return []string{policy.section} }

func (policy *ProductionPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := productionSettings{Mode: "global", CheckIntervalSec: 300, Castles: map[string]productionCastle{}}
	if !decodeSection(snapshot.Configuration, policy.section, &settings) {
		return Decision{
			Status: "waiting", Detail: fmt.Sprintf("No %s production plan is configured", policy.definitionKey),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 300)),
		}, nil
	}
	interval := policyInterval(settings.CheckIntervalSec, 300)
	configured := 0
	observed := 0
	full := 0
	unknownStackCapacity := 0
	missingScheduledDefinition := 0
	unavailableDefinition := 0
	unavailableScheduledDefinition := 0
	gloryTitleUnknown := 0
	gloryTitlePaused := 0
	focusUnavailable := 0
	helpListPending := false
	var nextCastleSchedule time.Time
	for _, castleKey := range policy.orderedCastleKeys(settings.Castles, snapshot.State.Castles) {
		castlePlan := settings.Castles[castleKey]
		if !castlePlan.Enabled {
			continue
		}
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castleID := State.CastleID(castleIDValue)
		castle, exists := snapshot.State.Castles[castleID]
		if !exists {
			continue
		}
		if State.CastleFocusKnownUnavailable(snapshot.State, castle) {
			focusUnavailable++
			continue
		}
		effectiveScheduleKey := policy.id
		scheduleKey := ""
		if settings.Mode == "perCastle" {
			effectiveScheduleKey = policy.id + ":" + castleKey
			scheduleKey = effectiveScheduleKey
		}
		schedule := resolveWeeklySchedule(snapshot.Configuration, effectiveScheduleKey, snapshot.Now)
		if !schedule.Allowed {
			if !schedule.Next.IsZero() && (nextCastleSchedule.IsZero() || schedule.Next.Before(nextCastleSchedule)) {
				nextCastleSchedule = schedule.Next
			}
			continue
		}

		targets := castlePlan.Items
		if settings.Mode != "perCastle" {
			targets = settings.GlobalItems
		}
		scheduled := schedule.SlotOptionsEnabled
		rotating := !scheduled && policy.id == "autoRecruit" && settings.Mode == "perCastle" && len(targets) > 1
		cursor := 0
		var target productionTarget
		if scheduled {
			definitionID, valid := productionScheduleDefinitionID(schedule.Options, policy.definitionKey+"ID")
			if !valid {
				missingScheduledDefinition++
				continue
			}
			target = productionTarget{ID: definitionID}
			if policy.lineID == 0 {
				target.MinID, _ = productionScheduleDefinitionID(schedule.Options, "unitIDMin")
				target.MaxID, _ = productionScheduleDefinitionID(schedule.Options, "unitIDMax")
			}
		} else {
			if len(targets) == 0 {
				continue
			}
			if rotating {
				configuredCursor := castlePlan.Cursor
				if snapshot.ConfigurationExternallyOwned {
					if runtimeCursor, found := operationalCursor(
						snapshot.State, policy.id, productionOperationalCursorKey(castleKey),
					); found {
						configuredCursor = runtimeCursor
					}
				}
				cursor = productionRotationCursor(configuredCursor, len(targets))
			}
			target = targets[cursor]
			if target.ID <= 0 {
				continue
			}
		}
		configured++
		queue, queueExists := castle.Production[policy.lineID]
		queuePredatesCastle := !queueExists || State.ProductionQueuePredatesCastleSnapshot(castle, queue)
		if queuePredatesCastle && castleSnapshotCurrent(snapshot.State, castle) {
			// A current JAA/JCA already committed without this production line.
			// Wait for authoritative line data instead of refocusing in a loop.
			continue
		}
		if !queueExists || queuePredatesCastle || State.ProductionQueueNeedsRefresh(snapshot.State, queue, snapshot.Now) {
			arguments, _ := json.Marshal(map[string]any{"castleId": castleID, "refresh": true})
			policy.lastCastleID = castleID
			return Decision{
				Status:              "ready",
				Detail:              fmt.Sprintf("Refresh the %s production queue at %s", policy.definitionKey, castleName(castle)),
				NextCheckAt:         snapshot.Now.Add(coordinatorTick),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ScheduleKey:         scheduleKey,
				ReevaluateOnSuccess: true,
				ReevaluateOnStale:   true,
			}, nil
		}
		observed++
		// QS contains the queue slots after the active production stack. The active
		// stack must not consume one of those slots.
		occupied := len(queue.Queued)
		queueCapacity := policy.queueCapacity(snapshot.State, queue, snapshot.GameData)
		if queueCapacity <= 0 || occupied >= queueCapacity {
			full++
			if policy.lineID == 0 && occupied >= queueCapacity {
				if !State.OwnAllianceHelpListCurrent(snapshot.State) {
					helpListPending = true
					continue
				}
				if State.HasOutstandingRecruitmentAllianceHelpRequest(snapshot.State, castleID) {
					continue
				}
				if productionID := eligibleAllianceHelpProductionID(queue); productionID > 0 {
					arguments, _ := json.Marshal(map[string]any{"productionId": productionID})
					policy.lastCastleID = castleID
					return Decision{
						Status: "ready", Detail: fmt.Sprintf("Request alliance help for recruitment queue at %s", castleName(castle)),
						NextCheckAt:         snapshot.Now.Add(coordinatorTick),
						Request:             &Intent.Request{Name: "alliance.help.request", Arguments: arguments},
						ScheduleKey:         scheduleKey,
						ReevaluateOnSuccess: true,
						ReevaluateOnStale:   true,
					}, nil
				}
			}
			continue
		}
		resolvedTarget := target
		availability := productionTargetUnavailable
		var titleGuard *productionGloryTitleGuard
		candidateCount := 1
		if rotating {
			candidateCount = len(targets)
		}
		for offset := 0; offset < candidateCount; offset++ {
			candidateCursor := cursor
			candidate := target
			if rotating {
				candidateCursor = (cursor + offset) % len(targets)
				candidate = targets[candidateCursor]
				if candidate.ID <= 0 {
					continue
				}
			}
			attemptTarget, attemptAvailability, attemptGuard := policy.resolveQueueableTarget(
				candidate,
				castle,
				snapshot.State.Player,
				snapshot.State.Session.ConnectionGeneration,
				snapshot.GameData,
				settings.RecruitLevel10OnTitleLoss,
			)
			if attemptAvailability == productionTargetAvailable {
				resolvedTarget = attemptTarget
				availability = attemptAvailability
				titleGuard = attemptGuard
				cursor = candidateCursor
				break
			}
			if attemptAvailability == productionTargetGloryTitleUnknown ||
				attemptAvailability == productionTargetGloryTitlePaused && availability != productionTargetGloryTitleUnknown {
				availability = attemptAvailability
			}
		}
		if availability != productionTargetAvailable {
			switch availability {
			case productionTargetGloryTitleUnknown:
				gloryTitleUnknown++
			case productionTargetGloryTitlePaused:
				gloryTitlePaused++
			default:
				unavailableDefinition++
			}
			if scheduled {
				unavailableScheduledDefinition++
			}
			continue
		}
		target = resolvedTarget
		amount := policy.targetAmount(snapshot.State, castle, target, snapshot.GameData)
		if amount <= 0 {
			unknownStackCapacity++
			continue
		}
		fillAvailable := !rotating && !scheduled
		intentArguments := map[string]any{
			"castleId": castleID, "lineId": policy.lineID,
			"definitionId": target.ID, "amount": amount, "fillAvailable": fillAvailable,
		}
		if titleGuard != nil {
			intentArguments["titleGatedDefinitionId"] = titleGuard.TitleGatedDefinitionID
			intentArguments["requiredGloryTitleId"] = titleGuard.RequiredGloryTitleID
			intentArguments["titleLossFallback"] = titleGuard.TitleLossFallback
		}
		if scheduled {
			intentArguments["scheduledDefinitionId"] = target.ID
			intentArguments["scheduleValidUntil"] = schedule.ValidUntil.UTC()
		}
		arguments, _ := json.Marshal(intentArguments)
		var followUp *Intent.Request
		var operationalCursorUpdate *OperationalCursorUpdate
		detail := fmt.Sprintf("Queue the configured %s at %s", policy.definitionKey, castleName(castle))
		if titleGuard != nil && titleGuard.TitleLossFallback {
			detail = fmt.Sprintf("Queue the level 10 glory-title fallback at %s", castleName(castle))
		}
		if scheduled {
			detail = fmt.Sprintf("Queue scheduled %s %d at %s", policy.definitionKey, target.ID, castleName(castle))
		} else if rotating {
			nextCursor := (cursor + 1) % len(targets)
			if snapshot.ConfigurationExternallyOwned {
				operationalCursorUpdate = &OperationalCursorUpdate{
					Key: productionOperationalCursorKey(castleKey), Value: nextCursor,
				}
			} else {
				raw := snapshot.Configuration.Sections[policy.section]
				updated, updateErr := advanceProductionCursor(raw, castleKey, nextCursor)
				if updateErr != nil {
					return Decision{}, updateErr
				}
				followUpArguments, _ := json.Marshal(map[string]any{
					"section": policy.section, "value": updated, "expectedValue": json.RawMessage(raw),
				})
				followUp = &Intent.Request{Name: "config.update", Arguments: followUpArguments}
			}
			detail = fmt.Sprintf(
				"Queue Auto Recruit rotation unit %d of %d at %s",
				cursor+1, len(targets), castleName(castle),
			)
		}
		policy.lastCastleID = castleID
		return Decision{
			Status:              "ready",
			Detail:              detail,
			NextCheckAt:         snapshot.Now.Add(coordinatorTick),
			Request:             &Intent.Request{Name: "production.enqueue", Arguments: arguments},
			FollowUp:            followUp,
			OperationalCursor:   operationalCursorUpdate,
			ScheduleKey:         scheduleKey,
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	status := "idle"
	detail := fmt.Sprintf("No enabled castle has a configured %s", policy.definitionKey)
	if missingScheduledDefinition > 0 {
		detail = fmt.Sprintf("The active schedule slot has no valid %s", policy.definitionKey)
	} else if configured > 0 && observed == 0 {
		detail = "Waiting for production queues to be observed in the game session"
	} else if helpListPending {
		status = "waiting"
		detail = "Waiting for the current recruitment alliance-help request list"
	} else if configured > 0 && observed == full {
		detail = "All observed production queues are full"
	} else if focusUnavailable > 0 && configured == 0 {
		detail = "Configured production castles are not focusable in the current kingdom session"
	} else if gloryTitleUnknown > 0 {
		detail = "Waiting for the current player glory title before recruiting a title-gated level 11 unit"
	} else if gloryTitlePaused > 0 {
		detail = "Glory-title level 11 recruit slots are paused while the required title is lost"
	} else if unavailableDefinition > 0 {
		if unavailableScheduledDefinition > 0 {
			detail = fmt.Sprintf("The scheduled %s family is not currently available at any enabled castle", policy.definitionKey)
		} else {
			detail = fmt.Sprintf("No enabled castle can currently produce the configured %s family", policy.definitionKey)
		}
	} else if unknownStackCapacity > 0 {
		detail = "Waiting for the official building stack capacity"
	}
	nextCheck := snapshot.Now.Add(interval)
	if !nextCastleSchedule.IsZero() && nextCastleSchedule.Before(nextCheck) {
		nextCheck = nextCastleSchedule
	}
	return Decision{Status: status, Detail: detail, NextCheckAt: nextCheck}, nil
}

func castleSnapshotCurrent(state State.GameState, castle State.CastleState) bool {
	return !castle.ContextSnapshotObservedAt.IsZero() &&
		(state.Session.Generation == 0 || state.Session.ChangedAt.IsZero() ||
			!castle.ContextSnapshotObservedAt.Before(state.Session.ChangedAt))
}

// resolveQueueableTarget treats a configured recruitment definition as an
// official upgrade-family anchor. When the game replaces a researched unit ID
// with its next tier, Auto Recruit selects the highest member of that same
// family that the target castle currently reports as recruitable.
func (policy *ProductionPolicy) resolveQueueableTarget(
	target productionTarget,
	castle State.CastleState,
	player State.PlayerState,
	connectionGeneration uint64,
	gameData *GameData.Store,
	recruitLevel10OnTitleLoss bool,
) (productionTarget, productionTargetAvailability, *productionGloryTitleGuard) {
	var titleGuard *productionGloryTitleGuard
	if policy.lineID == 0 && gameData != nil {
		if unlock, titleGated := gameData.GloryTitleUnlockForUnit(target.ID); titleGated {
			currentTitleID, titleKnown := player.CurrentGloryTitle(connectionGeneration)
			if !titleKnown {
				return target, productionTargetGloryTitleUnknown, nil
			}
			titleEligible := gameData.GloryTitleIncludes(currentTitleID, unlock.RequiredTitleID)
			if !titleEligible && !recruitLevel10OnTitleLoss {
				return target, productionTargetGloryTitlePaused, nil
			}
			titleGuard = &productionGloryTitleGuard{
				TitleGatedDefinitionID: unlock.UnitID,
				RequiredGloryTitleID:   unlock.RequiredTitleID,
				TitleLossFallback:      !titleEligible,
			}
			if !titleEligible {
				if unlock.Level10UnitID <= 0 {
					return target, productionTargetUnavailable, nil
				}
				target.ID = unlock.Level10UnitID
			}
		}
	}
	if policy.lineID != 0 || castle.QueueableObservedAt.IsZero() {
		return target, productionTargetAvailable, titleGuard
	}
	available := make(map[int64]struct{})
	for _, definition := range castle.QueueableProduction[policy.lineID] {
		if definition.ID <= 0 || definition.Collection != "" && definition.Collection != "units" {
			continue
		}
		available[definition.ID] = struct{}{}
	}
	if len(available) == 0 {
		return target, productionTargetUnavailable, nil
	}
	if titleGuard != nil {
		if _, found := available[target.ID]; !found {
			return target, productionTargetUnavailable, nil
		}
		return target, productionTargetAvailable, titleGuard
	}

	family := productionUnitFamilyIDs(target, gameData)
	for index := len(family) - 1; index >= 0; index-- {
		if _, found := available[family[index]]; !found {
			continue
		}
		target.ID = family[index]
		return target, productionTargetAvailable, nil
	}
	return target, productionTargetUnavailable, nil
}

func productionUnitFamilyIDs(target productionTarget, gameData *GameData.Store) []int64 {
	if target.ID <= 0 {
		return nil
	}
	if gameData == nil {
		return []int64{target.ID}
	}
	units, err := gameData.Catalog("units")
	if err != nil {
		return []int64{target.ID}
	}

	seed := target.ID
	if !units.Contains(strconv.FormatInt(seed, 10)) {
		for _, candidate := range []int64{target.MinID, target.MaxID} {
			if units.Contains(strconv.FormatInt(candidate, 10)) {
				seed = candidate
				break
			}
		}
	}
	if !units.Contains(strconv.FormatInt(seed, 10)) {
		return []int64{target.ID}
	}

	const maximumFamilySize = 128
	seen := map[int64]struct{}{seed: {}}
	lower := []int64{seed}
	for current := seed; len(seen) < maximumFamilySize; {
		next := productionUnitLink(units, current, "downgradeWodID")
		if next <= 0 {
			break
		}
		if _, duplicate := seen[next]; duplicate {
			break
		}
		seen[next] = struct{}{}
		lower = append(lower, next)
		current = next
	}
	for left, right := 0, len(lower)-1; left < right; left, right = left+1, right-1 {
		lower[left], lower[right] = lower[right], lower[left]
	}

	family := append([]int64(nil), lower...)
	for current := seed; len(seen) < maximumFamilySize; {
		next := productionUnitLink(units, current, "upgradeWodID")
		if next <= 0 {
			break
		}
		if _, duplicate := seen[next]; duplicate {
			break
		}
		seen[next] = struct{}{}
		family = append(family, next)
		current = next
	}
	return family
}

func productionUnitLink(catalog *GameData.Catalog, definitionID int64, field string) int64 {
	if catalog == nil || definitionID <= 0 {
		return 0
	}
	linkedID, exists := catalog.Int64(strconv.FormatInt(definitionID, 10), field)
	if !exists || linkedID <= 0 {
		return 0
	}
	return linkedID
}

func productionScheduleDefinitionID(options map[string]any, key string) (int64, bool) {
	value, exists := options[key]
	if !exists {
		return 0, false
	}
	var id int64
	switch typed := value.(type) {
	case float64:
		if typed <= 0 || typed != math.Trunc(typed) {
			return 0, false
		}
		id = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		id = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		id = parsed
	case int:
		id = int64(typed)
	case int64:
		id = typed
	case int32:
		id = int64(typed)
	default:
		return 0, false
	}
	return id, id > 0
}

func (policy *ProductionPolicy) orderedCastleKeys(
	plans map[string]productionCastle,
	castles map[State.CastleID]State.CastleState,
) []string {
	keys := make([]string, 0, len(plans))
	for key, plan := range plans {
		castleIDValue, err := strconv.ParseInt(key, 10, 64)
		castleID := State.CastleID(castleIDValue)
		if err != nil || castleID <= 0 || !plan.Enabled {
			continue
		}
		if _, exists := castles[castleID]; exists {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		leftIDValue, _ := strconv.ParseInt(keys[left], 10, 64)
		rightIDValue, _ := strconv.ParseInt(keys[right], 10, 64)
		leftCastle := castles[State.CastleID(leftIDValue)]
		rightCastle := castles[State.CastleID(rightIDValue)]
		if leftCastle.KingdomID != rightCastle.KingdomID {
			return leftCastle.KingdomID < rightCastle.KingdomID
		}
		leftName := strings.ToLower(strings.TrimSpace(leftCastle.Name))
		rightName := strings.ToLower(strings.TrimSpace(rightCastle.Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return leftCastle.ID < rightCastle.ID
	})
	if policy.lastCastleID <= 0 || len(keys) < 2 {
		return keys
	}
	for index, key := range keys {
		castleIDValue, _ := strconv.ParseInt(key, 10, 64)
		if State.CastleID(castleIDValue) != policy.lastCastleID {
			continue
		}
		next := (index + 1) % len(keys)
		return append(append(make([]string, 0, len(keys)), keys[next:]...), keys[:next]...)
	}
	return keys
}

func productionRotationCursor(cursor int, count int) int {
	if cursor < 0 || count <= 0 {
		return 0
	}
	return cursor % count
}

func advanceProductionCursor(raw json.RawMessage, castleKey string, cursor int) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode production configuration: %w", err)
	}
	castles, ok := document["castles"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("production configuration has no castles object")
	}
	castle, ok := castles[castleKey].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("production configuration has no castle %s", castleKey)
	}
	castle["cursor"] = cursor
	return document, nil
}

func (policy *ProductionPolicy) queueCapacity(state State.GameState, queue State.ProductionQueue, gameData *GameData.Store) int {
	expected, known := productionVIPQueueCapacity(state, policy.lineID, gameData)
	if queue.Capacity <= 0 {
		return expected
	}
	if !known || queue.Capacity < expected {
		return queue.Capacity
	}
	return expected
}

func productionVIPQueueCapacity(state State.GameState, lineID int, gameData *GameData.Store) (int, bool) {
	if gameData == nil || state.Player.VIP.Level <= 0 {
		return productionBaseQueueCapacity, false
	}
	catalog, err := gameData.Catalog("viplevels")
	if err != nil {
		return productionBaseQueueCapacity, false
	}
	field := "recruitmentBonusSlots"
	if lineID == 1 {
		field = "productionBonusSlots"
	}
	bonus, exists := catalog.Int64(strconv.Itoa(state.Player.VIP.Level), field)
	if !exists || bonus < 0 {
		return productionBaseQueueCapacity, false
	}
	return productionBaseQueueCapacity + int(bonus), true
}

func (policy *ProductionPolicy) targetAmount(
	state State.GameState,
	castle State.CastleState,
	target productionTarget,
	gameData *GameData.Store,
) int64 {
	if target.Amount > 0 {
		return target.Amount
	}
	if policy.lineID == 0 {
		return recruitmentStackAmount(state, castle, gameData)
	}
	return toolProductionStackAmount(castle, target.ID, gameData)
}

func recruitmentStackAmount(state State.GameState, castle State.CastleState, gameData *GameData.Store) int64 {
	if gameData == nil {
		return 0
	}
	buildings, buildingsErr := gameData.BuildingCatalog()
	constructionItems, constructionItemsErr := gameData.ConstructionItemCatalog()
	if buildingsErr != nil || constructionItemsErr != nil {
		return 0
	}
	subscriptionBonus := recruitmentSubscriptionStackBonus(state, gameData)
	var best int64
	for instanceID, building := range castle.Buildings {
		definition, found := buildings.DefinitionView(int64(building.DefinitionID))
		if !found {
			continue
		}
		base := int64(definition.Values["stackSize"])
		if base <= 0 {
			continue
		}
		amount := base + recruitmentConstructionBonus(
			castle.ConstructionSlots[instanceID], definition.ConstructionItemGroupIDs, constructionItems,
		) + subscriptionBonus
		if amount > best {
			best = amount
		}
	}
	return best
}

func recruitmentSubscriptionStackBonus(state State.GameState, gameData *GameData.Store) int64 {
	if gameData == nil || len(state.Subscriptions) == 0 {
		return 0
	}
	activeTypeIDs := map[int]struct{}{}
	for typeID, subscription := range state.Subscriptions {
		if subscription.TypeID > 0 {
			typeID = subscription.TypeID
		}
		if typeID > 0 && subscription.RemainingSec > 0 {
			activeTypeIDs[typeID] = struct{}{}
		}
	}
	if len(activeTypeIDs) == 0 {
		return 0
	}
	bonuses := map[int]int64{}
	for typeID := range activeTypeIDs {
		for _, effect := range gameData.SubscriptionEffectsView(typeID) {
			if !effect.Decorated && effect.ID == recruitmentStackCapacityEffectID && effect.Value > 0 {
				bonuses[typeID] = effect.Value
			}
		}
	}
	var total int64
	for _, bonus := range bonuses {
		total += bonus
	}
	return total
}

func recruitmentConstructionBonus(
	slots []State.ConstructionSlot,
	buildingGroups []int64,
	catalog *GameData.ConstructionItemCatalog,
) int64 {
	var total int64
	for _, slot := range slots {
		definition, found := catalog.DefinitionView(int64(slot.DefinitionID))
		if !found || !recruitmentConstructionApplies(definition, buildingGroups) {
			continue
		}
		if definition.StackSize > 0 {
			total += definition.StackSize
		}
	}
	return total
}

func recruitmentConstructionApplies(definition GameData.ConstructionItemTier, buildingGroups []int64) bool {
	if definition.GroupID > 0 && containsInt64(buildingGroups, definition.GroupID) {
		return true
	}
	if strings.EqualFold(definition.LockRemoval, "SOLDIER_RECRUITMENT") {
		return true
	}
	name := strings.ToLower(definition.InternalName + " " + definition.Comment + " " + definition.DisplayNameKey)
	return strings.Contains(name, "barrack") && strings.Contains(name, "stack")
}

func toolProductionStackAmount(castle State.CastleState, toolID int64, gameData *GameData.Store) int64 {
	if gameData == nil {
		return 0
	}
	buildings, err := gameData.BuildingCatalog()
	if err != nil {
		return 0
	}
	tool, found := gameData.UnitRuntimeView(toolID)
	if !found {
		return 0
	}
	workshopName := strings.ToLower(strings.TrimSpace(tool.InternalName))
	if workshopName != "workshop" && workshopName != "dworkshop" {
		return 0
	}
	var best int64
	for _, building := range castle.Buildings {
		definition, found := buildings.DefinitionView(int64(building.DefinitionID))
		if !found || !strings.EqualFold(definition.InternalName, workshopName) {
			continue
		}
		amount := int64(definition.Values["stackSize"])
		if amount <= 0 {
			if definition.Level > 0 && definition.Level <= 4 {
				amount = definition.Level * 20
			}
		}
		if amount > best {
			best = amount
		}
	}
	return best
}

func castleName(castle State.CastleState) string {
	if castle.Name != "" {
		return castle.Name
	}
	return fmt.Sprintf("castle %d", castle.ID)
}
