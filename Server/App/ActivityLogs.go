package App

import (
	"fmt"
	"strings"
	"unicode"

	"CitadelDesktop/Server/Intent"
)

type featureActivity struct {
	severity string
	event    string
	detail   string
}

func featureActivities(receipt Intent.Receipt) []featureActivity {
	if supportingFeatureIntent(receipt.Intent) {
		return nil
	}
	switch receipt.Status {
	case Intent.StatusSucceeded:
		return completedFeatureActivities(receipt)
	case Intent.StatusFailed, Intent.StatusPartiallySucceeded, Intent.StatusIndeterminate:
		if receipt.Plan != nil && (receipt.Plan.Effect == Intent.EffectRead || !planHasGameCommand(receipt.Plan)) {
			return nil
		}
		summary := receiptSummary(receipt)
		detail := "Could not " + attemptedActivityDetail(summary)
		reason := userFacingFailureReason(receipt.Error)
		detail += ": " + reason
		severity := "ERROR"
		if availabilityGateFailure(receipt.Error) {
			severity = "WARN"
		}
		return []featureActivity{{severity: severity, event: featureActivityEvent(receipt.Intent), detail: detail}}
	default:
		return nil
	}
}

func availabilityGateFailure(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(lower, "not enough troops") || strings.Contains(lower, "insufficient troops") {
		return true
	}
	return strings.Contains(lower, " of item ") &&
		(strings.Contains(lower, " commander(s) require ") ||
			strings.Contains(lower, " attack formation requires "))
}

func userFacingFailureReason(value string) string {
	reason := strings.TrimSpace(value)
	if reason == "" {
		return "the action did not complete"
	}
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "timed out waiting for"):
		return "the game did not confirm the action in time"
	case strings.Contains(lower, "game session changed while waiting for"),
		strings.Contains(lower, "game session changed while committing"),
		strings.Contains(lower, "game websocket connection changed"):
		return "the game connection changed before the action could be confirmed"
	case strings.Contains(lower, "game websocket"):
		return "the game connection was unavailable"
	case strings.Contains(lower, "response did not include a result code"):
		return "the game returned an invalid confirmation"
	case strings.Contains(lower, "response state reduction failed"):
		return "the game confirmation could not be applied to the current state"
	case strings.Contains(lower, "outbound effect outcome is indeterminate"):
		return "the game did not confirm whether the action completed"
	case strings.Contains(lower, "intent plan became stale"):
		return "the game state changed before the action finished"
	case strings.Contains(lower, "persist "):
		return "the app could not save the action state"
	case strings.Contains(lower, "response observer is unavailable"),
		strings.Contains(lower, "step resolver"), strings.Contains(lower, " is not registered"):
		return "an internal app error prevented the action"
	}
	const unsuccessfulMarker = " was not successful: "
	if marker := strings.Index(lower, unsuccessfulMarker); marker >= 0 {
		reason = strings.TrimSpace(reason[marker+len(unsuccessfulMarker):])
		for _, source := range []string{" (official game text)", " (inferred from captures)", " (undocumented)"} {
			reason = strings.TrimSuffix(reason, source)
		}
		if reason != "" {
			return reason
		}
	}
	return reason
}

func completedFeatureActivities(receipt Intent.Receipt) []featureActivity {
	if receipt.Plan == nil || receipt.Plan.Effect == Intent.EffectRead || !planHasGameCommand(receipt.Plan) {
		return nil
	}
	event := featureActivityEvent(receipt.Intent)
	if event == "ATTACK" {
		attackSteps := attackLaunchSteps(receipt.Plan.Steps)
		if len(attackSteps) > 1 {
			activities := make([]featureActivity, 0, len(attackSteps))
			for index, step := range attackSteps {
				detail := completedActivityDetail(step.Name)
				if strings.TrimSpace(step.Name) == "" {
					detail = completedActivityDetail(receiptSummary(receipt))
				}
				activities = append(activities, featureActivity{
					severity: "INFO", event: event,
					detail: fmt.Sprintf("%s (%d of %d)", detail, index+1, len(attackSteps)),
				})
			}
			return activities
		}
	}
	return []featureActivity{{
		severity: "INFO", event: event, detail: completedActivityDetail(receiptSummary(receipt)),
	}}
}

func planHasGameCommand(plan *Intent.Plan) bool {
	if plan == nil {
		return false
	}
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.Opcode) != "" || strings.TrimSpace(step.Command.Opcode) != "" ||
			strings.TrimSpace(step.AwaitOpcode) != "" || len(step.AwaitOpcodes) > 0 || step.CommandDependencies != nil {
			return true
		}
	}
	return false
}

func attackLaunchSteps(steps []Intent.Step) []Intent.Step {
	result := make([]Intent.Step, 0, len(steps))
	for _, step := range steps {
		opcode := strings.ToLower(strings.TrimSpace(step.Opcode))
		if opcode == "" {
			opcode = strings.ToLower(strings.TrimSpace(step.Command.Opcode))
		}
		dependencyOpcode := ""
		if step.CommandDependencies != nil {
			dependencyOpcode = strings.ToLower(strings.TrimSpace(step.CommandDependencies.Opcode))
		}
		if opcode == "cra" || dependencyOpcode == "cra" {
			result = append(result, step)
		}
	}
	return result
}

func supportingFeatureIntent(intent string) bool {
	intent = strings.ToLower(strings.TrimSpace(intent))
	return intent == "config.update" || intent == "game.focus_castle" || intent == "map.query" ||
		intent == "construction.shop" || intent == "shop.package.history" || intent == "alliance.inspect" ||
		strings.Contains(intent, ".refresh") || strings.HasSuffix(intent, ".scan")
}

func featureActivityEvent(intent string) string {
	intent = strings.ToLower(strings.TrimSpace(intent))
	switch {
	case strings.Contains(intent, "attack"), intent == "advisor.run.launch",
		intent == "rift.maiden_wave.launch", intent == "rift.launch.replay", intent == "tower.launch",
		intent == "khan.taunt":
		return "ATTACK"
	case intent == "spy.launch":
		return "ESPIONAGE"
	case intent == "production.enqueue", intent == "crafting.start":
		return "QUEUE"
	case intent == "resource.ship", strings.HasPrefix(intent, "resource.market."),
		strings.HasPrefix(intent, "resource.kingdom."), strings.HasPrefix(intent, "troops."),
		intent == "beri.transfer", intent == "movement.recall", intent == "storm.island.return":
		return "TRANSPORT"
	case strings.Contains(intent, "purchase"), intent == "khan.defense_tools.replenish":
		return "PURCHASE"
	case strings.HasPrefix(intent, "construction."):
		return "CONSTRUCTION"
	case strings.HasPrefix(intent, "building."), strings.HasPrefix(intent, "decoration."):
		return "BUILDING"
	case strings.HasPrefix(intent, "hospital."):
		return "HOSPITAL"
	case strings.HasPrefix(intent, "equipment."):
		return "EQUIPMENT"
	case strings.HasPrefix(intent, "crafting."):
		return "CRAFTING"
	case intent == "alliance.help.request":
		return "ALLIANCE HELP"
	case strings.Contains(intent, "cooldown"), strings.HasSuffix(intent, ".skip"):
		return "TIME SKIP"
	case strings.Contains(intent, "difficulty"), intent == "advisor.activate":
		return "EVENT"
	case strings.HasPrefix(intent, "defense."), strings.HasPrefix(intent, "khan.protection"), intent == "khan.open_gate":
		return "DEFENSE"
	default:
		return "ACTION"
	}
}

func receiptSummary(receipt Intent.Receipt) string {
	if receipt.Plan != nil {
		if summary := strings.TrimSpace(receipt.Plan.Summary); summary != "" {
			return summary
		}
	}
	name := strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(strings.TrimSpace(receipt.Intent))
	if name == "" {
		return "feature action"
	}
	return name
}

func completedActivityDetail(summary string) string {
	summary = strings.TrimSpace(summary)
	replacements := []struct {
		from string
		to   string
	}{
		{"Build and launch ", "Launched "},
		{"Attack ", "Launched an attack against "},
		{"Launch ", "Launched "},
		{"Chain ", "Launched "},
		{"Level camp ", "Launched a camp-leveling attack against "},
		{"Replay ", "Replayed "},
		{"Spy on ", "Sent spies to "},
		{"Queue ", "Queued "},
		{"Ship ", "Sent "},
		{"Transfer ", "Transferred "},
		{"Station ", "Stationed "},
		{"Recall ", "Recalled "},
		{"Buy ", "Purchased "},
		{"Purchase ", "Purchased "},
		{"Equip ", "Equipped "},
		{"Upgrade ", "Upgraded "},
		{"Heal wounded units: ", "Queued healing for "},
		{"Discard ", "Discarded "},
		{"Sell ", "Sold "},
		{"Apply ", "Applied "},
		{"Start ", "Started "},
		{"Stop ", "Stopped "},
		{"Activate ", "Activated "},
		{"Select ", "Selected "},
		{"Lock ", "Locked "},
		{"Open ", "Opened "},
		{"Clear ", "Cleared "},
		{"Collect ", "Collected "},
		{"Finish ", "Finished "},
		{"Replenish ", "Replenished "},
		{"Reset ", "Reset "},
		{"Return ", "Returned "},
		{"Rent ", "Rented "},
		{"Complete ", "Completed "},
		{"Request ", "Requested "},
		{"Update ", "Updated "},
		{"Construct ", "Started construction of "},
		{"Place ", "Placed "},
		{"Move ", "Moved "},
		{"Store ", "Stored "},
		{"Demolish ", "Demolished "},
		{"Rename ", "Renamed "},
		{"Delete ", "Deleted "},
	}
	for _, replacement := range replacements {
		if strings.HasPrefix(summary, replacement.from) {
			return replacement.to + strings.TrimPrefix(summary, replacement.from)
		}
	}
	if summary == "" {
		return "Completed feature action"
	}
	return "Completed " + lowerInitial(summary)
}

func attemptedActivityDetail(summary string) string {
	summary = strings.TrimSpace(summary)
	replacements := []struct {
		from string
		to   string
	}{
		{"Build and launch ", "launch "},
		{"Attack ", "launch an attack against "},
		{"Launch ", "launch "},
		{"Chain ", "launch "},
		{"Level camp ", "launch a camp-leveling attack against "},
		{"Replay ", "replay "},
		{"Spy on ", "send spies to "},
		{"Queue ", "queue "},
		{"Ship ", "send "},
		{"Transfer ", "transfer "},
		{"Station ", "station "},
		{"Recall ", "recall "},
		{"Buy ", "buy "},
		{"Purchase ", "purchase "},
		{"Equip ", "equip "},
		{"Upgrade ", "upgrade "},
		{"Heal wounded units: ", "queue healing for "},
		{"Discard ", "discard "},
		{"Sell ", "sell "},
		{"Apply ", "apply "},
		{"Start ", "start "},
		{"Stop ", "stop "},
		{"Activate ", "activate "},
		{"Select ", "select "},
		{"Lock ", "lock "},
		{"Open ", "open "},
		{"Clear ", "clear "},
		{"Collect ", "collect "},
		{"Finish ", "finish "},
		{"Replenish ", "replenish "},
		{"Reset ", "reset "},
		{"Return ", "return "},
		{"Rent ", "rent "},
		{"Complete ", "complete "},
		{"Request ", "request "},
		{"Update ", "update "},
		{"Construct ", "construct "},
		{"Place ", "place "},
		{"Move ", "move "},
		{"Store ", "store "},
		{"Demolish ", "demolish "},
		{"Rename ", "rename "},
		{"Delete ", "delete "},
	}
	for _, replacement := range replacements {
		if strings.HasPrefix(summary, replacement.from) {
			return replacement.to + strings.TrimPrefix(summary, replacement.from)
		}
	}
	return "complete " + lowerInitial(summary)
}

func lowerInitial(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "feature action"
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func userFacingGameName(value string) string {
	value = strings.NewReplacer("_", " ", "-", " ").Replace(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	runes := []rune(value)
	var result strings.Builder
	result.Grow(len(value) + 4)
	for index, current := range runes {
		if index > 0 && unicode.IsUpper(current) && (unicode.IsLower(runes[index-1]) || unicode.IsDigit(runes[index-1])) {
			result.WriteRune(' ')
		}
		result.WriteRune(current)
	}
	name := strings.Join(strings.Fields(result.String()), " ")
	nameRunes := []rune(name)
	if len(nameRunes) > 0 {
		nameRunes[0] = unicode.ToUpper(nameRunes[0])
	}
	return string(nameRunes)
}
