package App

import (
	"encoding/json"
	"fmt"
	"strings"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

const autoSupportCoinReserve int64 = 0

// Feature identity is runtime-owned. A manual station request cannot opt in.
func supportCoinHorseEligible(input Intent.PlanningContext) bool {
	return input.AutomationLane == "autoBird" || input.AutomationLane == "autoStation" || strings.HasPrefix(input.IntentName, "auto_bird.")
}

func supportTravelChoice(input Intent.PlanningContext, source State.CastleState, movements int64, fallback bool) (int, int, *Localization.Message, error) {
	observed, pending, known := Intent.TravelTicketBudget(input.State, input.CurrencyAvailability)
	available := max(int64(0), observed-pending)
	if !known || available < movements && !fallback {
		return 0, 0, nil, Intent.RequireTravelTickets(input, movements)
	}
	if available >= movements {
		return -1, 1, nil, nil
	}
	horse, err := resolveCastleHorseTravelBoostID(input.GameData, source, 1007)
	if err != nil {
		return 0, 0, nil, err
	}
	notice := coinHorseSupportTravelMessage(movements, available)
	return horse, 0, notice, nil
}

func coinHorseSupportTravelMessage(needed, available int64) *Localization.Message {
	message := Localization.New("server.support.coin_horse", "Sent with a coin horse: not enough travel feathers ({needed} needed, {available} available)", Localization.Params{"needed": needed, "available": available})
	return Localization.Bind(message, fmt.Sprintf("Sent with a coin horse: not enough travel feathers (%d needed, %d available)", needed, available))
}

// One notice per operation, including partially completed multi-batch support.
// Only acknowledged sends can claim that the coin horse was used.
func coinHorseSupportTravelActivity(receipt Intent.Receipt) (featureActivity, bool) {
	if receipt.Plan == nil {
		return featureActivity{}, false
	}
	for _, index := range receipt.CompletedStepIndexes {
		if index < 0 || index >= len(receipt.Plan.Steps) {
			continue
		}
		step := receipt.Plan.Steps[index]
		if step.Opcode != "cds" || step.CoinCost == nil || step.CoinCost.Source != Intent.SupportCoinHorseSource || step.NameDescriptor == nil {
			continue
		}
		payload := step.Payload
		if len(payload) == 0 {
			payload = step.Command.Payload
		}
		var fields struct {
			HBW int
			PTT *int
		}
		if json.Unmarshal(payload, &fields) != nil || fields.PTT == nil || *fields.PTT != 0 || fields.HBW <= 0 {
			continue
		}
		message := Localization.Clone(step.NameDescriptor)
		message = Localization.Bind(message, fmt.Sprintf("Sent with a coin horse: not enough travel feathers (%v needed, %v available)", message.Params["needed"], message.Params["available"]))
		return featureActivity{severity: "INFO", event: featureActivityEvent(receipt.Intent), detail: message.FallbackText, descriptor: message}, true
	}
	return featureActivity{}, false
}
