package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type HospitalPolicy struct{}

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
	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		wounded := orderedWounded(castle.Units.Hospital)
		if len(wounded) == 0 {
			continue
		}
		woundedCastles++
		queue, exists := castle.Production[2]
		if !exists || queue.ObservedAt.IsZero() {
			continue
		}
		observedQueues++
		occupied := len(queue.Queued)
		if queue.Active != nil {
			occupied++
		}
		if queue.Capacity <= 0 || occupied >= queue.Capacity {
			continue
		}
		for _, stack := range wounded {
			rubyCost, known := recordNumber(snapshot.GameData, "units", int64(stack.unitID), "healingCostC2")
			if !known {
				continue
			}
			intentName := "hospital.heal"
			amount := observedHospitalStack(queue)
			detail := fmt.Sprintf("Heal unit %d at %s", stack.unitID, castleName(castle))
			if rubyCost > 0 {
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
				Status: "ready", Detail: detail, NextCheckAt: snapshot.Now.Add(interval),
				Request: &Intent.Request{Name: intentName, Arguments: arguments},
			}, nil
		}
	}
	detail := "No wounded units need automatic healing"
	if woundedCastles > 0 && observedQueues == 0 {
		detail = "Waiting for hospital queues to be observed"
	} else if woundedCastles > 0 {
		detail = "Hospital queues are full or their safe healing stack size is not yet known"
	}
	return Decision{Status: "idle", Detail: detail, NextCheckAt: snapshot.Now.Add(interval)}, nil
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

func observedHospitalStack(queue State.ProductionQueue) int64 {
	var amount int64
	if queue.Active != nil && queue.Active.Amount > amount {
		amount = queue.Active.Amount
	}
	for _, item := range queue.Queued {
		if item.Amount > amount {
			amount = item.Amount
		}
	}
	return amount
}
