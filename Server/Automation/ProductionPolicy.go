package Automation

import (
	"context"
	"encoding/json"
	"fmt"
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
	Mode             string                      `json:"mode"`
	CheckIntervalSec int                         `json:"checkIntervalSec"`
	GlobalItems      []productionTarget          `json:"globalItems"`
	Castles          map[string]productionCastle `json:"castles"`
}

type productionTarget struct {
	ID     int64 `json:"id"`
	Amount int64 `json:"amount,omitempty"`
}

type productionCastle struct {
	Enabled bool               `json:"enabled"`
	Items   []productionTarget `json:"items"`
}

const recruitmentStackCapacityEffectID = 189
const productionBaseQueueCapacity = 2

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
	return []string{"production", "subscriptions"}
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
		targets := castlePlan.Items
		if settings.Mode != "perCastle" {
			targets = settings.GlobalItems
		}
		if len(targets) == 0 || targets[0].ID <= 0 {
			continue
		}
		configured++
		scheduleKey := ""
		if settings.Mode == "perCastle" {
			scheduleKey = policy.id + ":" + castleKey
			if allowed, next := scheduleAllows(snapshot.Configuration, scheduleKey, snapshot.Now); !allowed {
				if !next.IsZero() && (nextCastleSchedule.IsZero() || next.Before(nextCastleSchedule)) {
					nextCastleSchedule = next
				}
				continue
			}
		}
		queue, queueExists := castle.Production[policy.lineID]
		if !queueExists || queue.ObservedAt.IsZero() {
			continue
		}
		observed++
		// QS contains the queue slots after the active production stack. The active
		// stack must not consume one of those slots.
		occupied := len(queue.Queued)
		queueCapacity := policy.queueCapacity(snapshot.State, queue, snapshot.GameData)
		if queueCapacity <= 0 || occupied >= queueCapacity {
			full++
			if policy.lineID == 0 && occupied >= queueCapacity {
				if productionID := eligibleAllianceHelpProductionID(queue); productionID > 0 {
					arguments, _ := json.Marshal(map[string]any{"productionId": productionID})
					policy.lastCastleID = castleID
					return Decision{
						Status: "ready", Detail: fmt.Sprintf("Request alliance help for recruitment queue at %s", castleName(castle)),
						NextCheckAt:         snapshot.Now.Add(coordinatorTick),
						Request:             &Intent.Request{Name: "alliance.help.request", Arguments: arguments},
						ScheduleKey:         scheduleKey,
						ReevaluateOnSuccess: true,
					}, nil
				}
			}
			continue
		}
		target := targets[0]
		amount := policy.targetAmount(snapshot.State, castle, target, snapshot.GameData)
		if amount <= 0 {
			unknownStackCapacity++
			continue
		}
		arguments, _ := json.Marshal(map[string]any{
			"castleId": castleID, "lineId": policy.lineID,
			"definitionId": target.ID, "amount": amount, "fillAvailable": true,
		})
		policy.lastCastleID = castleID
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Queue the configured %s at %s", policy.definitionKey, castleName(castle)),
			NextCheckAt:         snapshot.Now.Add(coordinatorTick),
			Request:             &Intent.Request{Name: "production.enqueue", Arguments: arguments},
			ScheduleKey:         scheduleKey,
			ReevaluateOnSuccess: true,
		}, nil
	}
	detail := fmt.Sprintf("No enabled castle has a configured %s", policy.definitionKey)
	if configured > 0 && observed == 0 {
		detail = "Waiting for production queues to be observed in the game session"
	} else if configured > 0 && observed == full {
		detail = "All observed production queues are full"
	} else if unknownStackCapacity > 0 {
		detail = "Waiting for the official building stack capacity"
	}
	nextCheck := snapshot.Now.Add(interval)
	if !nextCastleSchedule.IsZero() && nextCastleSchedule.Before(nextCheck) {
		nextCheck = nextCastleSchedule
	}
	return Decision{Status: "idle", Detail: detail, NextCheckAt: nextCheck}, nil
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
	record, found := productionRecord(catalog, int64(state.Player.VIP.Level))
	if !found {
		return productionBaseQueueCapacity, false
	}
	field := "recruitmentBonusSlots"
	if lineID == 1 {
		field = "productionBonusSlots"
	}
	bonus, exists := record.Int64(field)
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
	buildings, constructionItems, ok := productionCatalogs(gameData)
	if !ok {
		return 0
	}
	subscriptionBonus := recruitmentSubscriptionStackBonus(state, gameData)
	var best int64
	for instanceID, building := range castle.Buildings {
		record, found := productionRecord(buildings, int64(building.DefinitionID))
		if !found {
			continue
		}
		base, exists := record.Int64("stackSize")
		if !exists || base <= 0 {
			continue
		}
		groups := commaSeparatedIDs(recordString(record, "constructionItemGroupIDs"))
		amount := base + recruitmentConstructionBonus(castle.ConstructionSlots[instanceID], groups, constructionItems) + subscriptionBonus
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
	raw, exists := gameData.RawCollection("subscriptionsBuffs")
	if !exists {
		return 0
	}
	var buffs []struct {
		SubscriptionTypeID string `json:"subscriptionTypeID"`
		Effects            string `json:"effects"`
	}
	if err := json.Unmarshal(raw, &buffs); err != nil {
		return 0
	}
	bonuses := map[int]int64{}
	for _, buff := range buffs {
		typeID, err := strconv.Atoi(strings.TrimSpace(buff.SubscriptionTypeID))
		if err != nil || typeID <= 0 {
			continue
		}
		if _, active := activeTypeIDs[typeID]; !active {
			continue
		}
		for _, effect := range strings.Split(buff.Effects, ",") {
			effectIDText, valueText, found := strings.Cut(strings.TrimSpace(effect), "&")
			if !found || strings.ContainsAny(valueText, "+#") {
				continue
			}
			effectID, effectErr := strconv.Atoi(strings.TrimSpace(effectIDText))
			value, valueErr := strconv.ParseInt(strings.TrimSpace(valueText), 10, 64)
			if effectErr == nil && valueErr == nil && effectID == recruitmentStackCapacityEffectID && value > 0 {
				bonuses[typeID] = value
			}
		}
	}
	var total int64
	for _, bonus := range bonuses {
		total += bonus
	}
	return total
}

func recruitmentConstructionBonus(slots []State.ConstructionSlot, buildingGroups map[int64]struct{}, catalog *GameData.Catalog) int64 {
	var total int64
	for _, slot := range slots {
		record, found := productionRecord(catalog, int64(slot.DefinitionID))
		if !found || !recruitmentConstructionApplies(record, buildingGroups) {
			continue
		}
		if stackSize, exists := record.Int64("stackSize"); exists && stackSize > 0 {
			total += stackSize
		}
	}
	return total
}

func recruitmentConstructionApplies(record GameData.Record, buildingGroups map[int64]struct{}) bool {
	if groupID, exists := record.Int64("constructionItemGroupID"); exists && groupID > 0 {
		if _, applies := buildingGroups[groupID]; applies {
			return true
		}
	}
	if strings.EqualFold(recordString(record, "lockRemoval"), "SOLDIER_RECRUITMENT") {
		return true
	}
	name := strings.ToLower(recordString(record, "name") + " " + recordString(record, "comment2") + " " + recordString(record, "displayNameKey"))
	return strings.Contains(name, "barrack") && strings.Contains(name, "stack")
}

func toolProductionStackAmount(castle State.CastleState, toolID int64, gameData *GameData.Store) int64 {
	buildings, _, ok := productionCatalogs(gameData)
	if !ok {
		return 0
	}
	units, err := gameData.Catalog("units")
	if err != nil {
		return 0
	}
	tool, found := productionRecord(units, toolID)
	if !found {
		return 0
	}
	workshopName := strings.ToLower(strings.TrimSpace(recordString(tool, "name")))
	if workshopName != "workshop" && workshopName != "dworkshop" {
		return 0
	}
	var best int64
	for _, building := range castle.Buildings {
		record, found := productionRecord(buildings, int64(building.DefinitionID))
		if !found || !strings.EqualFold(recordString(record, "name"), workshopName) {
			continue
		}
		amount, exists := record.Int64("stackSize")
		if !exists || amount <= 0 {
			if level, levelExists := record.Int64("level"); levelExists && level > 0 && level <= 4 {
				amount = level * 20
			}
		}
		if amount > best {
			best = amount
		}
	}
	return best
}

func productionCatalogs(gameData *GameData.Store) (*GameData.Catalog, *GameData.Catalog, bool) {
	if gameData == nil {
		return nil, nil, false
	}
	buildings, buildingsErr := gameData.Catalog("buildings")
	constructionItems, constructionItemsErr := gameData.Catalog("constructionItems")
	if buildingsErr != nil || constructionItemsErr != nil {
		return nil, nil, false
	}
	return buildings, constructionItems, true
}

func productionRecord(catalog *GameData.Catalog, id int64) (GameData.Record, bool) {
	if catalog == nil || id <= 0 {
		return nil, false
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return nil, false
	}
	record, err := GameData.DecodeRecord(raw)
	return record, err == nil
}

func recordString(record GameData.Record, field string) string {
	value, _ := record.String(field)
	return value
}

func castleName(castle State.CastleState) string {
	if castle.Name != "" {
		return castle.Name
	}
	return fmt.Sprintf("castle %d", castle.ID)
}
