package Intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"CitadelDesktop/Server/GameData"
)

func TestExpectedAutomationGameRejectionStaysOnLaneStatus(t *testing.T) {
	engine := &Engine{}
	receipt := Receipt{
		ID: "busy-commander", Actor: "automation:autoStorm", Status: StatusFailed,
		Plan: &Plan{Summary: "Attack Storm fort at 500:500"},
	}
	err := &ResponseCodeError{Opcode: "cra", Meaning: GameData.ResolveResponseCode(nil, "cra", 256)}
	receipt = engine.withFailure(receipt, err)

	if receipt.Failure == nil {
		t.Fatal("expected a public failure projection")
	}
	if receipt.Failure.Toast || receipt.Failure.Kind != FailureAvailability ||
		receipt.Failure.Severity != FailureSeverityWarning || receipt.Failure.GameCode == nil || *receipt.Failure.GameCode != 256 {
		t.Fatalf("automation failure = %#v", receipt.Failure)
	}
	if strings.Contains(receipt.Failure.Message, "CRA") || strings.Contains(receipt.Failure.Explanation, "response code") {
		t.Fatalf("public failure leaked protocol language: %#v", receipt.Failure)
	}
	if !strings.Contains(receipt.Failure.Recovery, "Automated combat pauses") {
		t.Fatalf("commander recovery omitted the safety consequence: %#v", receipt.Failure)
	}
}

func TestExpectedInteractiveGameRejectionStillExplainsItself(t *testing.T) {
	engine := &Engine{}
	receipt := Receipt{Actor: "ui", Status: StatusFailed, Plan: &Plan{Summary: "Launch an attack"}}
	err := &ResponseCodeError{Opcode: "cra", Meaning: GameData.ResolveResponseCode(nil, "cra", 256)}
	receipt = engine.withFailure(receipt, err)

	if receipt.Failure == nil || !receipt.Failure.Toast || receipt.Failure.Knowledge != FailureKnowledgeObserved {
		t.Fatalf("interactive failure = %#v", receipt.Failure)
	}
}

func TestOfficialClientEnchantFailureProjectsAsOfficialExpectedWarning(t *testing.T) {
	engine := &Engine{}
	receipt := Receipt{Actor: "ui", Status: StatusFailed, Plan: &Plan{Summary: "Upgrade relic equipment"}}
	receipt = engine.withFailure(receipt, NewResponseCodeError(nil, "ere", 227))

	if receipt.Failure == nil || !receipt.Failure.Toast || receipt.Failure.Kind != FailureGameRejected ||
		receipt.Failure.Severity != FailureSeverityWarning || receipt.Failure.Knowledge != FailureKnowledgeOfficial ||
		receipt.Failure.GameCode == nil || *receipt.Failure.GameCode != 227 ||
		!strings.Contains(receipt.Failure.Explanation, "did not gain a level") ||
		!strings.Contains(receipt.Failure.Recovery, "Retry the same level") {
		t.Fatalf("official-client enchant failure = %#v", receipt.Failure)
	}
}

func TestCRA91ExplainsIncompatiblePresetToolsAcrossAttackLanes(t *testing.T) {
	engine := &Engine{}
	for _, actor := range []string{"automation:autoNomad", "automation:autoStorm", "ui"} {
		receipt := Receipt{
			ID: actor, Actor: actor, Status: StatusFailed,
			Plan: &Plan{Summary: "Launch an attack"},
		}
		err := NewResponseCodeError(nil, "cra", 91)
		receipt = engine.withFailure(receipt, err)

		if receipt.Failure == nil || !receipt.Failure.Toast || receipt.Failure.Kind != FailureAvailability ||
			receipt.Failure.Severity != FailureSeverityError || receipt.Failure.Knowledge != FailureKnowledgeObserved ||
			receipt.Failure.GameCode == nil || *receipt.Failure.GameCode != 91 ||
			!strings.Contains(receipt.Failure.Explanation, "incompatible tools") ||
			!strings.Contains(receipt.Failure.Recovery, "attack preset") {
			t.Errorf("CRA 91 failure for %q = %#v", actor, receipt.Failure)
		}
	}
}

func TestPartialAutomationRejectionStillNotifies(t *testing.T) {
	engine := &Engine{}
	receipt := Receipt{Actor: "automation:autoStorm", Status: StatusPartiallySucceeded, Plan: &Plan{Summary: "Buy two Storm offers"}}
	err := &ResponseCodeError{Opcode: "sbp", Meaning: GameData.ResponseCodeMeaning{
		Code: 55, Message: "Not enough resources.", Source: GameData.ResponseCodeOfficial,
		Kind: GameData.ResponseCodeAvailability, Recovery: "Refresh the shop.", ExpectedState: true,
	}}
	receipt = engine.withFailure(receipt, err)

	if receipt.Failure == nil || !receipt.Failure.Toast || receipt.Failure.Severity != FailureSeverityWarning {
		t.Fatalf("partial automation rejection was hidden = %#v", receipt.Failure)
	}
}

func TestUndocumentedGameRejectionDoesNotInventACause(t *testing.T) {
	engine := &Engine{}
	receipt := Receipt{Actor: "automation:autoRecruit", Status: StatusFailed}
	err := &ResponseCodeError{Opcode: "ahr", Meaning: GameData.ResolveResponseCode(nil, "ahr", 269)}
	receipt = engine.withFailure(receipt, err)

	if receipt.Failure == nil || !receipt.Failure.Toast || receipt.Failure.Knowledge != FailureKnowledgeUnknown ||
		!strings.Contains(receipt.Failure.Explanation, "does not provide a known explanation") ||
		!strings.Contains(receipt.Failure.Recovery, "game error 269") {
		t.Fatalf("undocumented response projection = %#v", receipt.Failure)
	}
}

func TestAutomationCommanderAndTroopGatesDoNotToast(t *testing.T) {
	engine := &Engine{}
	for _, raw := range []string{
		"no commander is currently available",
		"no assigned commander supports the required maiden relic",
		"no assigned Auto Towers commander is in the current roster",
		"castle 12 has 20 of item 30; 1 commander(s) require 40",
	} {
		receipt := engine.withFailure(Receipt{Actor: "automation:autoTowers", Status: StatusFailed}, errorString(raw))
		if receipt.Failure == nil || receipt.Failure.Toast || receipt.Failure.Kind != FailureAvailability {
			t.Errorf("gate %q = %#v", raw, receipt.Failure)
		}
	}
}

func TestStaleCommanderAvailabilityStaysOnAutomationLane(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "automation:autoTowers", Status: StatusFailed},
		fmt.Errorf("%w: commander 42 is no longer available", ErrPlanStale),
	)
	if receipt.Failure == nil || receipt.Failure.Toast || receipt.Failure.Kind != FailureAvailability ||
		receipt.Failure.Severity != FailureSeverityWarning {
		t.Fatalf("stale commander gate = %#v", receipt.Failure)
	}
	if strings.Contains(receipt.Failure.Explanation, "42") {
		t.Fatalf("stale commander gate exposed an internal identifier = %#v", receipt.Failure)
	}
}

func TestExpectedRevisionConflictIsARecoverableStaleState(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "ui", Status: StatusFailed},
		fmt.Errorf("%w: state revision changed: expected 10, current 11", ErrPlanStale),
	)
	if receipt.Failure == nil || receipt.Failure.Kind != FailureStaleState ||
		receipt.Failure.Severity != FailureSeverityWarning || !receipt.Failure.Toast {
		t.Fatalf("expected revision projection = %#v", receipt.Failure)
	}
	if strings.Contains(receipt.Failure.Explanation, "revision") {
		t.Fatalf("expected revision projection exposed internal state = %#v", receipt.Failure)
	}
}

func TestAutomationPreflightStaleStateStaysOnLane(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "automation:autoBuyer", Status: StatusFailed},
		fmt.Errorf("%w: feast cost reduction is stale", ErrPlanStale),
	)
	if receipt.Failure == nil || receipt.Failure.Kind != FailureStaleState ||
		receipt.Failure.Severity != FailureSeverityWarning || receipt.Failure.Toast {
		t.Fatalf("automation preflight stale projection = %#v", receipt.Failure)
	}
}

func TestAutomationFeastRefreshProtocolGapStaysOnLaneAndLogs(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "automation:autoBuyer", Status: StatusFailed},
		errorString("Verify Auto Buyer feast refresh: the game omitted feast status from the committed Auto Buyer refresh"),
	)
	if receipt.Failure == nil || receipt.Failure.Toast || receipt.Failure.Severity != FailureSeverityError ||
		receipt.Failure.Knowledge != FailureKnowledgeObserved ||
		!strings.Contains(receipt.Failure.Explanation, "stopped before purchasing") ||
		!strings.Contains(receipt.Failure.Recovery, "read-only refresh") {
		t.Fatalf("Auto Buyer feast refresh failure = %#v", receipt.Failure)
	}
}

func TestPendingFeastReconciliationStaysVisibleOnAutomationLane(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "automation:autoBuyer", Status: StatusFailed},
		errorString("the game has not yet resolved the pending feast purchase from an authoritative feast snapshot"),
	)
	if receipt.Failure == nil || receipt.Failure.Toast ||
		receipt.Failure.Kind != FailureIndeterminate ||
		receipt.Failure.Severity != FailureSeverityWarning ||
		receipt.Failure.Knowledge != FailureKnowledgeObserved ||
		!strings.Contains(receipt.Failure.Explanation, "not yet confirmed") ||
		!strings.Contains(receipt.Failure.Recovery, "keep another feast purchase blocked") {
		t.Fatalf("pending feast reconciliation projection = %#v", receipt.Failure)
	}
}

func TestAutomationStaleStateAfterMutationStillWarnsUser(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "automation:autoBuyer", Status: StatusPartiallySucceeded},
		fmt.Errorf("%w: feast timer did not refresh", ErrPlanStale),
	)
	if receipt.Failure == nil || receipt.Failure.Kind != FailureStaleState ||
		receipt.Failure.Severity != FailureSeverityWarning || !receipt.Failure.Toast {
		t.Fatalf("post-mutation stale projection = %#v", receipt.Failure)
	}
}

func TestResponseRejectionDoesNotHideJoinedCommitFailure(t *testing.T) {
	engine := &Engine{}
	responseErr := NewResponseCodeError(nil, "cra", 256)
	err := errors.Join(responseErr, errorString("commit earlier acknowledged response: disk unavailable"))
	receipt := engine.withFailure(
		Receipt{Actor: "automation:autoStorm", Status: StatusFailed},
		err,
	)
	if receipt.Failure == nil || !receipt.Failure.Toast || receipt.Failure.Kind != FailureInternal ||
		receipt.Failure.Severity != FailureSeverityError || receipt.Failure.GameCode == nil ||
		*receipt.Failure.GameCode != 256 || receipt.Failure.Knowledge != "" {
		t.Fatalf("joined response and commit projection = %#v", receipt.Failure)
	}
}

func TestBareDeadlineExceededHasUserFacingTimeoutGuidance(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(
		Receipt{Actor: "ui", Status: StatusFailed},
		fmt.Errorf("request expired: %w", context.DeadlineExceeded),
	)
	if receipt.Failure == nil || receipt.Failure.Kind != FailureTimeout ||
		receipt.Failure.Severity != FailureSeverityWarning ||
		!strings.Contains(receipt.Failure.Explanation, "did not confirm") {
		t.Fatalf("deadline projection = %#v", receipt.Failure)
	}
}

func TestInternalExecutionPlumbingDoesNotReachUserWording(t *testing.T) {
	engine := &Engine{}
	for _, raw := range []string{
		"committed wire response observer is unavailable",
		`action "storm.scan.burst" is not registered`,
	} {
		receipt := engine.withFailure(Receipt{Actor: "ui", Status: StatusFailed}, errorString(raw))
		if receipt.Failure == nil || receipt.Failure.Kind != FailureInternal ||
			receipt.Failure.Explanation != "An internal app error prevented the action." ||
			strings.Contains(receipt.Failure.Explanation, "observer") ||
			strings.Contains(receipt.Failure.Explanation, "registered") {
			t.Errorf("internal failure %q = %#v", raw, receipt.Failure)
		}
	}
}

func TestFailureProjectionSerializesWithoutRawDiagnosticError(t *testing.T) {
	engine := &Engine{}
	receipt := engine.withFailure(Receipt{Actor: "ui", Status: StatusFailed}, errorString("timed out waiting for cra"))
	payload, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !strings.Contains(text, `"failure"`) || !strings.Contains(text, `"kind":"timeout"`) {
		t.Fatalf("serialized receipt omitted failure projection: %s", text)
	}
	if strings.Contains(text, "RawError") || strings.Contains(text, "rawError") {
		t.Fatalf("serialized receipt exposed diagnostic error field: %s", text)
	}
}

type errorString string

func (value errorString) Error() string { return string(value) }
