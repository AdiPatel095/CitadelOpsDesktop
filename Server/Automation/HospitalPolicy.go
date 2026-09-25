package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type HospitalPolicy struct{}

const (
	hospitalLineID = 2
	// The intent resolves the exact fresh entitlement before dispatch.
	hospitalMaximumStackAmount int64 = 15
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
	return []string{"production", "subscriptions", "units", "kingdom-transport", "alliance-help"}
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
	helpListPending := false
	focusUnavailable := 0
	outstandingHelp := State.OutstandingHospitalAllianceHelpRequests(snapshot.State)
	for _, castleID := range castleIDs {
		castle := snapshot.State.Castles[castleID]
		wounded := orderedWounded(castle.Units.Hospital)
		queue, exists := castle.Production[hospitalLineID]
		queueObserved := exists && !queue.ObservedAt.IsZero()
		if State.CastleFocusKnownUnavailable(snapshot.State, castle) {
			if len(wounded) > 0 || queueObserved && eligibleAllianceHelpProductionID(queue) > 0 {
				focusUnavailable++
			}
			continue
		}
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
					if !State.OwnAllianceHelpListCurrent(snapshot.State) {
						helpListPending = true
						continue
					}
					if outstandingHelp >= State.MaximumOutstandingHospitalAllianceHelpRequests {
						helpCapacityReached = true
						continue
					}
					arguments, _ := json.Marshal(map[string]any{"productionId": productionID})
					return Decision{
						Status: "ready", Detail: fmt.Sprintf("Request alliance help for hospital queue at %s", castleName(castle)), DetailDescriptor: Localization.New("server.automation.request_alliance_help_for.629d7046", "Request alliance help for hospital queue at {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleName(castle))}),
						NextCheckAt:         snapshot.Now.Add(coordinatorTick),
						Request:             &Intent.Request{Name: "alliance.help.request", Arguments: arguments},
						ReevaluateOnSuccess: true,
						ReevaluateOnStale:   true,
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
			amount := hospitalMaximumStackAmount
			detail := fmt.Sprintf("Heal unit %d at %s", stack.unitID, castleName(castle))
			var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.heal_unit_p_at.f5708f3d", "Heal unit {p0} at {p1}", Localization.Params{"p0": fmt.Sprintf("%d", stack.unitID), "p1": fmt.Sprintf("%s", castleName(castle))})
			if known && rubyCost > 0 {
				intentName = "hospital.discard"
				amount = stack.amount
				detail = fmt.Sprintf("Discard ruby-only wounded unit %d at %s", stack.unitID, castleName(castle))
				detailLocalizationMessage = Localization.New("server.automation.discard_ruby_only_wounded.e23cc815", "Discard ruby-only wounded unit {p0} at {p1}", Localization.Params{"p0": fmt.Sprintf("%d", stack.unitID), "p1": fmt.Sprintf("%s", castleName(castle))})
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
				Status: "ready",
				Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
				NextCheckAt:         snapshot.Now.Add(coordinatorTick),
				Request:             &Intent.Request{Name: intentName, Arguments: arguments},
				ReevaluateOnSuccess: true,
				ReevaluateOnStale:   true,
			}, nil
		}
	}
	status := "idle"
	detail := "No wounded units need automatic healing"
	var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.no_wounded_units_need.a9bed334", "No wounded units need automatic healing", nil)
	if woundedCastles > 0 && observedQueues == 0 {
		detail = "Waiting for hospital queues to be observed"
		detailLocalizationMessage = Localization.New("server.automation.waiting_for_hospital_queues.68a3cfa8", "Waiting for hospital queues to be observed", nil)
	} else if helpListPending {
		status = "waiting"
		detail = "Waiting for the current hospital alliance-help request list"
		detailLocalizationMessage = Localization.New("server.automation.waiting_for_the_current.166fa1a9", "Waiting for the current hospital alliance-help request list", nil)
	} else if helpCapacityReached {
		status = "waiting"
		detail = "Waiting for the outstanding hospital alliance-help request to finish"
		detailLocalizationMessage = Localization.New("server.automation.waiting_for_the_outstanding.cf7744e8", "Waiting for the outstanding hospital alliance-help request to finish", nil)
	} else if woundedCastles > 0 {
		detail = "Hospital queues are full or their capacity is not yet known"
		detailLocalizationMessage = Localization.New("server.automation.hospital_queues_are_full.69fe66a9", "Hospital queues are full or their capacity is not yet known", nil)
	} else if focusUnavailable > 0 {
		detail = "Hospital castles retained from closed kingdoms are not focusable in the current session"
		detailLocalizationMessage = Localization.New("server.automation.hospital_castles_retained_from.b4724cd9", "Hospital castles retained from closed kingdoms are not focusable in the current session", nil)
	}
	return Decision{Status: status, Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), NextCheckAt: snapshot.Now.Add(interval)}, nil
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

func hospitalQueueCapacity(castle State.CastleState, gameData *GameData.Store) int {
	if gameData == nil {
		return 0
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return 0
	}
	var capacity int
	for _, building := range castle.Buildings {
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if !found {
			continue
		}
		slots := int64(definition.Values["hospitalSlots"])
		if int(slots) > capacity {
			capacity = int(slots)
		}
	}
	if capacity > 5 {
		return 5
	}
	return capacity
}
