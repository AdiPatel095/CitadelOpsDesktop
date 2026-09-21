package Intent

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
	if receipt.Failure != nil {
		if receipt.Failure.MessageDescriptor == nil {
			receipt.Failure.MessageDescriptor = failureHeadlineDescriptor(receipt)
		}
		receipt.Failure.MessageDescriptor = Localization.Bind(receipt.Failure.MessageDescriptor, receipt.Failure.Message)
		receipt.Failure.ExplanationDescriptor = Localization.Bind(receipt.Failure.ExplanationDescriptor, receipt.Failure.Explanation)
		receipt.Failure.RecoveryDescriptor = Localization.Bind(receipt.Failure.RecoveryDescriptor, receipt.Failure.Recovery)
	}
	return receipt
}

func (engine *Engine) failurePresentation(receipt Receipt, err error) *FailurePresentation {
	var locked *LaneLockedError
	if errors.As(err, &locked) {
		lock := locked.Lock
		return &FailurePresentation{Kind: FailureUnknown, Message: "Automation lane safety lock", MessageDescriptor: Localization.New("server.intent.automation_lane_safety_lock.56256527", "Automation lane safety lock", nil), Explanation: lock.Detail(), ExplanationDescriptor: lock.DetailDescriptor(), Recovery: "The lane automatically becomes eligible again 30 minutes after this rejection; normal session and feature prerequisites still apply.", RecoveryDescriptor: Localization.New("server.intent.the_lane_automatically_becomes.544b1dee", "The lane automatically becomes eligible again 30 minutes after this rejection; normal session and feature prerequisites still apply.", nil), Severity: FailureSeverityError, Toast: locked.Cause != nil, GameCode: &lock.Code, GameOpcode: lock.Opcode, SafetyLock: &lock}
	}
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
		presentation.GameOpcode = responseError.Opcode
		presentation.Knowledge = failureKnowledgeForResponseCode(meaning.Source)
		presentation.Explanation = responseCodeExplanation(meaning)
		presentation.ExplanationDescriptor = nil
		if meaning.Source == GameData.ResponseCodeOfficial && presentation.Explanation == cleanFailureText(meaning.Message) && !unresolvedGameTextPlaceholder.MatchString(meaning.Message) {
			presentation.ExplanationDescriptor = Localization.Official("errorCode_"+strconv.Itoa(meaning.Code), meaning.Message)
		}
		presentation.Recovery = cleanFailureText(meaning.Recovery)
		if presentation.Recovery == "" && meaning.Source == GameData.ResponseCodeUnknown {
			presentation.Recovery = fmt.Sprintf(
				"Refresh the feature once before retrying. If it repeats, include game error %d when reporting it.",
				meaning.Code,
			)
			presentation.RecoveryDescriptor = Localization.New("server.intent.refresh_the_feature_once.caf697f1", "Refresh the feature once before retrying. If it repeats, include game error {p0} when reporting it.", Localization.Params{"p0": meaning.Code})
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
			presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_rejected_this.aced3f26", "The game rejected this action, and an earlier game confirmation could not be applied safely.", nil)
			presentation.Recovery = "Refresh the feature and verify the current game state before retrying."
			presentation.RecoveryDescriptor = Localization.New("server.intent.refresh_the_feature_and.73e3aea7", "Refresh the feature and verify the current game state before retrying.", nil)
			presentation.Severity = FailureSeverityError
			presentation.Knowledge = ""
			presentation.Toast = true
		}
		return presentation
	}

	visible := engine.humanizeText(err.Error())
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case Outbound.IsIndeterminate(err) || receipt.Status == StatusIndeterminate:
		presentation.Kind = FailureIndeterminate
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game did not confirm whether the action completed."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_did_not.cd415666", "The game did not confirm whether the action completed.", nil)
		presentation.Recovery = "Check the game before retrying so a completed action is not duplicated."
		presentation.RecoveryDescriptor = Localization.New("server.intent.check_the_game_before.df95f098", "Check the game before retrying so a completed action is not duplicated.", nil)
	case commanderAvailabilityFailure(lower):
		presentation.Kind = FailureAvailability
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation, presentation.Recovery = commanderAvailabilityExplanation(lower)
		presentation.ExplanationDescriptor, presentation.RecoveryDescriptor = commanderAvailabilityDescriptors(lower)
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case troopAvailabilityFailure(lower):
		presentation.Kind = FailureAvailability
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "There are not enough eligible troops available for this action."
		presentation.ExplanationDescriptor = Localization.New("server.intent.there_are_not_enough.ae9bd389", "There are not enough eligible troops available for this action.", nil)
		presentation.Recovery = "The feature lane will reevaluate after troop availability changes."
		presentation.RecoveryDescriptor = Localization.New("server.intent.the_feature_lane_will.482d1118", "The feature lane will reevaluate after troop availability changes.", nil)
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case errors.Is(err, ErrCoinUnavailable):
		presentation.Kind = FailureAvailability
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = cleanFailureText(err.Error())
		presentation.ExplanationDescriptor = nil
		presentation.Recovery = "The feature lane will reevaluate after the authoritative coin balance changes."
		presentation.RecoveryDescriptor = Localization.New("server.intent.the_feature_lane_will.fdf76403", "The feature lane will reevaluate after the authoritative coin balance changes.", nil)
		presentation.Toast = !automationActor(receipt.Actor)
	case strings.Contains(lower, "timed out waiting for") ||
		(errors.Is(err, context.DeadlineExceeded) && !Outbound.IsIndeterminate(err) && receipt.Status != StatusIndeterminate):
		presentation.Kind = FailureTimeout
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game did not confirm the action in time."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_did_not.28418e4c", "The game did not confirm the action in time.", nil)
		presentation.Recovery = "Check the game or feature status before retrying so a completed action is not repeated."
		presentation.RecoveryDescriptor = Localization.New("server.intent.check_the_game_or.e9e2262b", "Check the game or feature status before retrying so a completed action is not repeated.", nil)
	case connectionChangedFailure(lower):
		presentation.Kind = FailureConnection
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game connection changed before the action could be confirmed."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_connection_changed.66f326e8", "The game connection changed before the action could be confirmed.", nil)
		presentation.Recovery = "Wait for the current game state to finish refreshing before trying again."
		presentation.RecoveryDescriptor = Localization.New("server.intent.wait_for_the_current.5e0a4685", "Wait for the current game state to finish refreshing before trying again.", nil)
	case strings.Contains(lower, "game websocket"):
		presentation.Kind = FailureConnection
		presentation.Explanation = "The game connection was unavailable."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_connection_was.314c66b8", "The game connection was unavailable.", nil)
		presentation.Recovery = "Reconnect to the game before trying again."
		presentation.RecoveryDescriptor = Localization.New("server.intent.reconnect_to_the_game.b765cb6b", "Reconnect to the game before trying again.", nil)
	case strings.Contains(lower, "response did not include a result code"):
		presentation.Kind = FailureInternal
		presentation.Explanation = "The game returned a confirmation the app could not validate."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_returned_a.4372bf43", "The game returned a confirmation the app could not validate.", nil)
		presentation.Recovery = "Refresh the feature before retrying. If it repeats, report the failed action."
		presentation.RecoveryDescriptor = Localization.New("server.intent.refresh_the_feature_before.e44c9b62", "Refresh the feature before retrying. If it repeats, report the failed action.", nil)
	case strings.Contains(lower, "omitted feast status"), strings.Contains(lower, "omitted feast cost reduction"):
		presentation.Kind = FailureUnknown
		presentation.Knowledge = FailureKnowledgeObserved
		presentation.Explanation = "The game did not return the complete feast state, so Auto Buyer stopped before purchasing."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_did_not.b8f7e07b", "The game did not return the complete feast state, so Auto Buyer stopped before purchasing.", nil)
		presentation.Recovery = "Auto Buyer will retry the read-only refresh and will not purchase until the response is complete."
		presentation.RecoveryDescriptor = Localization.New("server.intent.auto_buyer_will_retry.7675c4dc", "Auto Buyer will retry the read-only refresh and will not purchase until the response is complete.", nil)
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case strings.Contains(lower, "pending feast purchase") && strings.Contains(lower, "authoritative feast snapshot"):
		presentation.Kind = FailureIndeterminate
		presentation.Knowledge = FailureKnowledgeObserved
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game has not yet confirmed whether the feast purchase completed."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_has_not.b47d0d28", "The game has not yet confirmed whether the feast purchase completed.", nil)
		presentation.Recovery = "Auto Buyer will keep another feast purchase blocked and retry read-only reconciliation."
		presentation.RecoveryDescriptor = Localization.New("server.intent.auto_buyer_will_keep.60086d0a", "Auto Buyer will keep another feast purchase blocked and retry read-only reconciliation.", nil)
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case errors.Is(err, ErrPlanStale) || strings.Contains(lower, "intent plan became stale"):
		presentation.Kind = FailureStaleState
		presentation.Severity = FailureSeverityWarning
		presentation.Explanation = "The game state changed before the action finished."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_state_changed.3a425c47", "The game state changed before the action finished.", nil)
		presentation.Recovery = "Review the refreshed feature status before trying again."
		presentation.RecoveryDescriptor = Localization.New("server.intent.review_the_refreshed_feature.f38e4c30", "Review the refreshed feature status before trying again.", nil)
		// A pre-mutation automation recheck is routine lane state, not an
		// interruptive user error. Interactive requests and operations that already
		// completed a write still need a visible warning.
		presentation.Toast = !automationActor(receipt.Actor) || receipt.Status != StatusFailed
	case strings.Contains(lower, "response state reduction failed"):
		presentation.Kind = FailureInternal
		presentation.Explanation = "The game confirmation could not be applied to the current feature state."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_game_confirmation_could.2de25ac1", "The game confirmation could not be applied to the current feature state.", nil)
		presentation.Recovery = "Refresh the feature before trying again."
		presentation.RecoveryDescriptor = Localization.New("server.intent.refresh_the_feature_before.5cb1fe51", "Refresh the feature before trying again.", nil)
	case strings.Contains(lower, "persist "):
		presentation.Kind = FailureInternal
		presentation.Explanation = "The app could not save the action state safely."
		presentation.ExplanationDescriptor = Localization.New("server.intent.the_app_could_not.079dfaaf", "The app could not save the action state safely.", nil)
		presentation.Recovery = "Do not repeat the action until storage is available and the feature status is current."
		presentation.RecoveryDescriptor = Localization.New("server.intent.do_not_repeat_the.6ed835f9", "Do not repeat the action until storage is available and the feature status is current.", nil)
	case strings.Contains(lower, "response observer is unavailable"),
		strings.Contains(lower, "committed wire response observer is unavailable"),
		strings.Contains(lower, "action \"") && strings.Contains(lower, "is not registered"):
		presentation.Kind = FailureInternal
		presentation.Explanation = "An internal app error prevented the action."
		presentation.ExplanationDescriptor = Localization.New("server.intent.an_internal_app_error.31622149", "An internal app error prevented the action.", nil)
		presentation.Recovery = "If this repeats, report the action and time it occurred."
		presentation.RecoveryDescriptor = Localization.New("server.intent.if_this_repeats_report.5626a29d", "If this repeats, report the action and time it occurred.", nil)
	case userFacingTechnicalFailure.MatchString(visible):
		presentation.Kind = FailureInternal
		presentation.Explanation = "An internal app error prevented the action."
		presentation.ExplanationDescriptor = Localization.New("server.intent.an_internal_app_error.31622149", "An internal app error prevented the action.", nil)
		presentation.Recovery = "If this repeats, report the action and time it occurred."
		presentation.RecoveryDescriptor = Localization.New("server.intent.if_this_repeats_report.5626a29d", "If this repeats, report the action and time it occurred.", nil)
	default:
		presentation.Explanation = cleanFailureText(visible)
		presentation.ExplanationDescriptor = Localization.FromError(err)
		if visible != err.Error() {
			presentation.ExplanationDescriptor = nil
		}
		if presentation.Explanation == "" {
			presentation.Explanation = "The action did not complete."
			presentation.ExplanationDescriptor = Localization.New("server.intent.the_action_did_not.1be9cb5f", "The action did not complete.", nil)
		}
	}

	if receipt.Status == StatusIndeterminate && presentation.Kind != FailureIndeterminate {
		presentation.Recovery = "Check the game before retrying so a completed action is not duplicated."
		presentation.RecoveryDescriptor = Localization.New("server.intent.check_the_game_before.df95f098", "Check the game before retrying so a completed action is not duplicated.", nil)
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
	case GameData.ResponseCodeOfficial, GameData.ResponseCodeOfficialClient:
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

func failureHeadlineDescriptor(receipt Receipt) *Localization.Message {
	result := Localization.New("server.intent.action_failed", "This action could not be completed.", nil)
	switch receipt.Status {
	case StatusPartiallySucceeded:
		result = Localization.New("server.intent.action_partial", "This action completed only in part.", nil)
	case StatusIndeterminate:
		result = Localization.New("server.intent.action_unconfirmed", "We could not confirm whether this action completed.", nil)
	}
	if receipt.Plan != nil && strings.TrimSpace(receipt.Plan.Summary) != "" {
		if receipt.Plan.SummaryDescriptor == nil {
			return nil
		}
		result.Context = []*Localization.Message{Localization.Clone(receipt.Plan.SummaryDescriptor)}
	}
	return result
}
func commanderAvailabilityDescriptors(lower string) (*Localization.Message, *Localization.Message) {
	switch {
	case strings.Contains(lower, "no commanders are assigned"):
		return Localization.New("server.intent.commander_unassigned", "No commander is assigned to this feature.", nil), Localization.New("server.intent.assign_commander", "Assign at least one eligible commander in the feature settings.", nil)
	case strings.Contains(lower, "supports the required"), strings.Contains(lower, "current roster"):
		return Localization.New("server.intent.commander_requirements", "No assigned commander currently meets this feature's requirements.", nil), Localization.New("server.intent.assign_eligible_commander", "Assign a commander that meets the feature requirements.", nil)
	default:
		return Localization.New("server.intent.commander_unavailable", "No eligible commander is available right now.", nil), Localization.New("server.intent.wait_for_commander", "Wait for a commander to return; the feature lane will reevaluate automatically.", nil)
	}
}
