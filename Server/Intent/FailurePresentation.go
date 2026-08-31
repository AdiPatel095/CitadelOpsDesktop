package Intent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Outbound"
)

const maximumFailureActionRunes = 140

var (
	unresolvedGameTextPlaceholder = regexp.MustCompile(`\{[0-9]+\}`)
	userFacingTechnicalFailure    = regexp.MustCompile(
		`(?i)(?:\b(?:opcode|payload|resolver|dependency|intent|operation|revision|generation|cursor|json field|observer)\b|` +
			`\b(?:AID|CID|KID|LID|OID|PID|SID|TID|WID|WOD|AMT|CRA|GAA|SBP)\s*[:=])`,
	)
)

func (engine *Engine) withFailure(receipt Receipt, err error) Receipt {
	if err == nil {
		err = errors.New("the action did not complete")
	}
	receipt.RawError = err.Error()
	receipt.Error = engine.humanizeText(receipt.RawError)
	receipt.Failure = engine.failurePresentation(receipt, err)
	return receipt
}

func (engine *Engine) failurePresentation(receipt Receipt, err error) *FailurePresentation {
	presentation := &FailurePresentation{
		Kind:        FailureUnknown,
		Message:     failureHeadline(receipt),
		Explanation: "The action did not complete.",
		Severity:    FailureSeverityError,
		Toast:       true,
	}

	var responseError *ResponseCodeError
	if errors.As(err, &responseError) && responseError != nil {
		meaning := responseError.Meaning
		code := meaning.Code
		presentation.Kind = failureKindForResponseCode(meaning.Kind)
		presentation.GameCode = &code
		presentation.Knowledge = failureKnowledgeForResponseCode(meaning.Source)
		presentation.Explanation = responseCodeExplanation(meaning)
		presentation.Recovery = cleanFailureText(meaning.Recovery)
		if presentation.Recovery == "" && meaning.Source == GameData.ResponseCodeUnknown {
			presentation.Recovery = fmt.Sprintf(
				"Refresh the feature once before retrying. If it repeats, include game error %d when reporting it.",
				meaning.Code,
			)
		}
		if meaning.ExpectedState {
			presentation.Severity = FailureSeverityWarning
			if automationActor(receipt.Actor) && receipt.Status == StatusFailed {
				presentation.Toast = false
			}
		}
		if responseCodeSafetyFailure(err) {
			presentation.Kind = FailureInternal
			presentation.Explanation = "The game rejected this action, and an earlier game confirmation could not be applied safely."
			presentation.Recovery = "Refresh the feature and verify the current game state before retrying."
			presentation.Severity = FailureSeverityError
			presentation.Knowledge = ""
			presentation.Toast = true
		}
		return presentation
	}

	visible := engine.humanizeText(err.Error())
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case commanderAvailabilityFailure(lower):
		presentation.Kind = FailureAvailability
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation, presentation.Recovery = commanderAvailabilityExplanation(lower)
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case troopAvailabilityFailure(lower):
		presentation.Kind = FailureAvailability
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "There are not enough eligible troops available for this action."
		presentation.Recovery = "The feature lane will reevaluate after troop availability changes."
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case strings.Contains(lower, "timed out waiting for") ||
		(errors.Is(err, context.DeadlineExceeded) && !Outbound.IsIndeterminate(err) && receipt.Status != StatusIndeterminate):
		presentation.Kind = FailureTimeout
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game did not confirm the action in time."
		presentation.Recovery = "Check the game or feature status before retrying so a completed action is not repeated."
	case connectionChangedFailure(lower):
		presentation.Kind = FailureConnection
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game connection changed before the action could be confirmed."
		presentation.Recovery = "Wait for the current game state to finish refreshing before trying again."
	case strings.Contains(lower, "game websocket"):
		presentation.Kind = FailureConnection
		presentation.Explanation = "The game connection was unavailable."
		presentation.Recovery = "Reconnect to the game before trying again."
	case strings.Contains(lower, "response did not include a result code"):
		presentation.Kind = FailureInternal
		presentation.Explanation = "The game returned a confirmation the app could not validate."
		presentation.Recovery = "Refresh the feature before retrying. If it repeats, report the failed action."
	case errors.Is(err, ErrPlanStale) || strings.Contains(lower, "intent plan became stale"):
		presentation.Kind = FailureStaleState
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game state changed before the action finished."
		presentation.Recovery = "Review the refreshed feature status before trying again."
	case Outbound.IsIndeterminate(err) || receipt.Status == StatusIndeterminate:
		presentation.Kind = FailureIndeterminate
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game did not confirm whether the action completed."
		presentation.Recovery = "Check the game before retrying so a completed action is not duplicated."
	case strings.Contains(lower, "response state reduction failed"):
		presentation.Kind = FailureInternal
		presentation.Explanation = "The game confirmation could not be applied to the current feature state."
		presentation.Recovery = "Refresh the feature before trying again."
	case strings.Contains(lower, "persist "):
		presentation.Kind = FailureInternal
		presentation.Explanation = "The app could not save the action state safely."
		presentation.Recovery = "Do not repeat the action until storage is available and the feature status is current."
	case strings.Contains(lower, "response observer is unavailable"),
		strings.Contains(lower, "committed wire response observer is unavailable"),
		strings.Contains(lower, "action \"") && strings.Contains(lower, "is not registered"):
		presentation.Kind = FailureInternal
		presentation.Explanation = "An internal app error prevented the action."
		presentation.Recovery = "If this repeats, report the action and time it occurred."
	case userFacingTechnicalFailure.MatchString(visible):
		presentation.Kind = FailureInternal
		presentation.Explanation = "An internal app error prevented the action."
		presentation.Recovery = "If this repeats, report the action and time it occurred."
	default:
		presentation.Explanation = cleanFailureText(visible)
		if presentation.Explanation == "" {
			presentation.Explanation = "The action did not complete."
		}
	}

	if receipt.Status == StatusIndeterminate && presentation.Kind != FailureIndeterminate {
		presentation.Recovery = "Check the game before retrying so a completed action is not duplicated."
	}
	return presentation
}

func failureHeadline(receipt Receipt) string {
	action := ""
	if receipt.Plan != nil {
		action = strings.TrimSpace(receipt.Plan.Summary)
	}
	action = strings.TrimRight(action, " .!?\t\r\n")
	if action != "" {
		runes := []rune(action)
		if len(runes) > maximumFailureActionRunes {
			action = strings.TrimSpace(string(runes[:maximumFailureActionRunes-1])) + "…"
		}
		switch receipt.Status {
		case StatusPartiallySucceeded:
			return fmt.Sprintf("“%s” completed only in part.", action)
		case StatusIndeterminate:
			return fmt.Sprintf("We could not confirm whether “%s” completed.", action)
		default:
			return fmt.Sprintf("Could not complete “%s”.", action)
		}
	}
	switch receipt.Status {
	case StatusPartiallySucceeded:
		return "This action completed only in part."
	case StatusIndeterminate:
		return "We could not confirm whether this action completed."
	default:
		return "This action could not be completed."
	}
}

func responseCodeExplanation(meaning GameData.ResponseCodeMeaning) string {
	message := cleanFailureText(meaning.Message)
	if meaning.Source == GameData.ResponseCodeUnknown || message == "" {
		return "The game declined this action but does not provide a known explanation."
	}
	if unresolvedGameTextPlaceholder.MatchString(message) {
		return "The game declined this action, but its published explanation was incomplete."
	}
	return message
}

func failureKindForResponseCode(kind GameData.ResponseCodeKind) FailureKind {
	switch kind {
	case GameData.ResponseCodeAvailability, GameData.ResponseCodeCooldown, GameData.ResponseCodeContext:
		return FailureAvailability
	case GameData.ResponseCodeStaleState:
		return FailureStaleState
	default:
		return FailureGameRejected
	}
}

func failureKnowledgeForResponseCode(source GameData.ResponseCodeSource) FailureKnowledge {
	switch source {
	case GameData.ResponseCodeOfficial:
		return FailureKnowledgeOfficial
	case GameData.ResponseCodeObserved:
		return FailureKnowledgeObserved
	default:
		return FailureKnowledgeUnknown
	}
}

func automationActor(actor string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(actor)), "automation:")
}

func troopAvailabilityFailure(lower string) bool {
	if strings.Contains(lower, "not enough troops") || strings.Contains(lower, "insufficient troops") {
		return true
	}
	return strings.Contains(lower, " of item ") &&
		(strings.Contains(lower, " commander(s) require ") || strings.Contains(lower, " attack formation requires "))
}

func commanderAvailabilityFailure(lower string) bool {
	return strings.Contains(lower, "no commander") ||
		strings.Contains(lower, "no commanders") ||
		strings.Contains(lower, "commander availability changed") ||
		(strings.Contains(lower, "no available") && strings.Contains(lower, "commander")) ||
		(strings.Contains(lower, "commander") &&
			(strings.Contains(lower, " is no longer available") || strings.Contains(lower, " is not available"))) ||
		(strings.Contains(lower, "no assigned") && strings.Contains(lower, "commander"))
}

func commanderAvailabilityExplanation(lower string) (string, string) {
	switch {
	case strings.Contains(lower, "no commanders are assigned"):
		return "No commander is assigned to this feature.", "Assign at least one eligible commander in the feature settings."
	case strings.Contains(lower, "supports the required"), strings.Contains(lower, "current roster"):
		return "No assigned commander currently meets this feature's requirements.", "Assign a commander that meets the feature requirements."
	default:
		return "No eligible commander is available right now.", "Wait for a commander to return; the feature lane will reevaluate automatically."
	}
}

func connectionChangedFailure(lower string) bool {
	return strings.Contains(lower, "game session changed while waiting for") ||
		strings.Contains(lower, "game session changed while committing") ||
		strings.Contains(lower, "game websocket connection changed")
}

func responseCodeSafetyFailure(err error) bool {
	lower := strings.ToLower(err.Error())
	return Outbound.IsIndeterminate(err) ||
		strings.Contains(lower, "commit earlier acknowledged response") ||
		strings.Contains(lower, "commit acknowledged response") ||
		strings.Contains(lower, "response state reduction failed")
}

func cleanFailureText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
