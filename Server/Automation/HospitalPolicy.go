package Automation

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

type HospitalPolicy struct{}

const (
	hospitalLineID                       = 2
	hospitalBaseStackAmount        int64 = 10
	hospitalSubscriptionStackBonus int64 = 5
	hospitalMaximumStackAmount     int64 = 15
	hospitalSubscriptionEffectID         = 189
	hospitalAllianceHelpLimit            = 3
)

type hospitalSettings struct {
	CheckIntervalSec int `json:"checkIntervalSec"`
}

type woundedStack struct {
	unitID State.UnitID
	amount int64
}

func NewHospitalPolicy() *HospitalPolicy { return &HospitalPolicy{} }

func (*HospitalPolicy) ID() string { return "autoHospital" }

func (*HospitalPolicy) EnabledKey() string { return "auto_hospital" }

func (*HospitalPolicy) WakeDomains() []string {
	return []string{"production", "subscriptions", "units"}
}

func (*HospitalPolicy) WakeSections() []string { return []string{"automation.autoHospital"} }

func (*HospitalPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := hospitalSettings{CheckIntervalSec: 300}
	decodeSection(snapshot.Configuration, "automation.autoHospital", &settings)
	interval := policyInterval(settings.CheckIntervalSec, 300)
	castleIDs := make([]State.CastleID, 0, len(snapshot.State.Castles))
	for castleID := range snapshot.State.Castles {
		castleIDs = append(castleIDs, castleID)
	}
	sort.Slice(castleIDs, func(left, right int) bool { return castleIDs[left] < castleIDs[right] })
	woundedCastles := 0
	observedQueues := 0
	helpCapacityReached := false
	outstandingHelp := State.OutstandingHospitalAllianceHelpRequests(snapshot.State)
	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		wounded := orderedWounded(castle.Units.Hospital)
		queue, exists := castle.Production[hospitalLineID]
		queueObserved := exists && !queue.ObservedAt.IsZero()
		if queueObserved {
			occupied := len(queue.Queued)
			if queue.Active != nil {
				occupied++
			}
			queueCapacity := hospitalQueueCapacity(castle, snapshot.GameData)
			if queueCapacity <= 0 {
				queueCapacity = queue.Capacity
			}
			if queueCapacity > 0 && occupied >= queueCapacity {
				if productionID := eligibleAllianceHelpProductionID(queue); productionID > 0 {
					if outstandingHelp >= hospitalAllianceHelpLimit {
						helpCapacityReached = true
						continue
					}
					arguments, _ := json.Marshal(map[string]any{"productionId": productionID})
					return Decision{
						Status: "ready", Detail: fmt.Sprintf("Request alliance help for hospital queue at %s", castleName(castle)),
						NextCheckAt:         snapshot.Now.Add(coordinatorTick),
						Request:             &Intent.Request{Name: "alliance.help.request", Arguments: arguments},
						ReevaluateOnSuccess: true,
					}, nil
				}
				continue
			}
			if len(wounded) > 0 && queueCapacity <= 0 {
				woundedCastles++
				observedQueues++
				continue
			}
		}
		if len(wounded) == 0 {
			continue
		}
		woundedCastles++
		if !queueObserved {
			continue
		}
		observedQueues++
		for _, stack := range wounded {
			rubyCost, known := recordNumber(snapshot.GameData, "units", int64(stack.unitID), "healingCostC2")
			intentName := "hospital.heal"
			amount := hospitalStackAmount(snapshot.State, snapshot.GameData)
			detail := fmt.Sprintf("Heal unit %d at %s", stack.unitID, castleName(castle))
			if known && rubyCost > 0 {
				intentName = "hospital.discard"
				amount = stack.amount
				detail = fmt.Sprintf("Discard ruby-only wounded unit %d at %s", stack.unitID, castleName(castle))
			}
			if amount <= 0 {
				continue
			}
			if amount > stack.amount {
				amount = stack.amount
			}
			arguments, _ := json.Marshal(map[string]any{
				"castleId": castleID, "unitId": stack.unitID, "amount": amount,
			})
			return Decision{
				Status:              "ready",
				Detail:              detail,
				NextCheckAt:         snapshot.Now.Add(coordinatorTick),
				Request:             &Intent.Request{Name: intentName, Arguments: arguments},
				ReevaluateOnSuccess: true,
			}, nil
		}
	}
	status := "idle"
	detail := "No wounded units need automatic healing"
	if woundedCastles > 0 && observedQueues == 0 {
		detail = "Waiting for hospital queues to be observed"
	} else if helpCapacityReached {
		status = "waiting"
		detail = fmt.Sprintf(
			"Waiting for one of %d outstanding hospital alliance-help requests to finish",
			outstandingHelp,
		)
	} else if woundedCastles > 0 {
		detail = "Hospital queues are full or their capacity is not yet known"
	}
	return Decision{Status: status, Detail: detail, NextCheckAt: snapshot.Now.Add(interval)}, nil
}

func orderedWounded(units map[State.UnitID]int64) []woundedStack {
	result := make([]woundedStack, 0, len(units))
	for unitID, amount := range units {
		if unitID > 0 && amount > 0 {
			result = append(result, woundedStack{unitID: unitID, amount: amount})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].amount != result[right].amount {
			return result[left].amount > result[right].amount
		}
		return result[left].unitID < result[right].unitID
	})
	return result
}

func hospitalStackAmount(state State.GameState, gameData *GameData.Store) int64 {
	amount := hospitalBaseStackAmount
	if hasHospitalSubscriptionStackBonus(state, gameData) {
		amount += hospitalSubscriptionStackBonus
	}
	if amount > hospitalMaximumStackAmount {
		return hospitalMaximumStackAmount
	}
	return amount
}

func hasHospitalSubscriptionStackBonus(state State.GameState, gameData *GameData.Store) bool {
	if gameData == nil || len(state.Subscriptions) == 0 {
		return false
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
		return false
	}
	raw, exists := gameData.RawCollection("subscriptionsBuffs")
	if !exists {
		return false
	}
	var buffs []struct {
		SubscriptionTypeID string `json:"subscriptionTypeID"`
		Effects            string `json:"effects"`
	}
	if err := json.Unmarshal(raw, &buffs); err != nil {
		return false
	}
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
			if !found {
				continue
			}
			effectID, effectErr := strconv.Atoi(strings.TrimSpace(effectIDText))
			value, valueErr := strconv.Atoi(strings.TrimSpace(valueText))
			if effectErr == nil && valueErr == nil && effectID == hospitalSubscriptionEffectID && value > 0 {
				return true
			}
		}
	}
	return false
}

func hospitalQueueCapacity(castle State.CastleState, gameData *GameData.Store) int {
	if gameData == nil {
		return 0
	}
	var capacity int
	for _, building := range castle.Buildings {
		slots := recordInteger(gameData, "buildings", int64(building.DefinitionID), "hospitalSlots")
		if int(slots) > capacity {
			capacity = int(slots)
		}
	}
	if capacity > 5 {
		return 5
	}
	return capacity
}
