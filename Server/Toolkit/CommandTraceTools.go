package Toolkit

import (
	"context"
	"encoding/json"
	"strings"

	"CitadelDesktop/Server/Automation"
)

type commandTraceInput struct {
	Mode         string `json:"mode"`
	BrokerID     uint64 `json:"brokerId,omitempty"`
	HarnessID    uint64 `json:"harnessId,omitempty"`
	SubmissionID uint64 `json:"submissionId,omitempty"`
	WorkID       string `json:"workId,omitempty"`
	Opcode       string `json:"opcode,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Source       string `json:"source,omitempty"`
	Surface      string `json:"surface,omitempty"`
	Effect       string `json:"effect,omitempty"`
	Status       string `json:"status,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

func registerCommandTraceTools(harness *Harness) error {
	return harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.command.trace",
			Description: "Inspect bounded, value-redacted telemetry for commands sent by Citadel or the native game client. Request and response shapes contain field names and types only; stateChanges are observations during the send/response window, not proof of causation.",
			InputSchema: objectSchema(map[string]interface{}{
				"mode":         enumProperty("Choose an aggregate overview, recent command lifecycles, or observed wire-shape variants.", "summary", "recent", "variants"),
				"brokerId":     schemaProperty("integer", "Optional broker identity from a command receipt for mode=recent."),
				"harnessId":    schemaProperty("integer", "Optional harness identity from a command receipt for mode=recent."),
				"submissionId": schemaProperty("integer", "Optional submission identity from a command receipt for mode=recent."),
				"workId":       schemaProperty("string", "Optional automation work identity filter for mode=recent."),
				"opcode":       schemaProperty("string", "Optional exact opcode filter; variants commonly use this to inspect one command family."),
				"owner":        schemaProperty("string", "Optional exact owner filter for mode=recent."),
				"source":       enumProperty("Optional source filter for mode=recent.", Automation.CommandTraceSourceCitadel, Automation.CommandTraceSourceGameClient),
				"surface":      schemaProperty("string", "Optional command surface filter, such as internal_app, toolkit, runtime, or a companion adapter name."),
				"effect":       schemaProperty("string", "Optional exact command effect filter for mode=recent."),
				"status": enumProperty("Optional lifecycle status filter for mode=recent.",
					Automation.CommandTraceQueued,
					Automation.CommandTraceDispatching,
					Automation.CommandTraceSent,
					Automation.CommandTraceRetrying,
					Automation.CommandTraceResponded,
					Automation.CommandTraceResponseError,
					Automation.CommandTraceTransportFailed,
					Automation.CommandTraceCancelled,
				),
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum recent traces to return, from 1 to 500; default 100.",
					"minimum":     1,
					"maximum":     500,
				},
			}, "mode"),
			Effect: EffectRead,
			Tags:   []string{"command", "trace", "discovery"},
		},
		Handler: readCommandTrace,
	})
}

func readCommandTrace(_ context.Context, raw json.RawMessage) (interface{}, error) {
	input, err := decodeStrict[commandTraceInput](raw)
	if err != nil {
		return nil, err
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	input.Opcode = strings.ToLower(strings.TrimSpace(input.Opcode))
	input.WorkID = strings.TrimSpace(input.WorkID)
	input.Owner = strings.TrimSpace(input.Owner)
	input.Source = strings.TrimSpace(input.Source)
	input.Surface = strings.TrimSpace(input.Surface)
	input.Effect = strings.TrimSpace(input.Effect)
	input.Status = strings.TrimSpace(input.Status)
	if input.Limit < 0 || input.Limit > 500 {
		return nil, toolError("invalid_arguments", "limit must be between 1 and 500 when supplied")
	}
	if input.Source != "" && input.Source != Automation.CommandTraceSourceCitadel && input.Source != Automation.CommandTraceSourceGameClient {
		return nil, toolError("invalid_arguments", "source must be citadel or game_client")
	}
	if input.Status != "" && !validCommandTraceStatus(input.Status) {
		return nil, toolError("invalid_arguments", "status is not a recognized command lifecycle state")
	}

	overview := Automation.CommandTraceOverviewSnapshot()
	switch input.Mode {
	case "summary":
		if input.BrokerID != 0 || input.HarnessID != 0 || input.SubmissionID != 0 || input.WorkID != "" || input.Opcode != "" || input.Owner != "" || input.Source != "" || input.Surface != "" || input.Effect != "" || input.Status != "" || input.Limit != 0 {
			return nil, toolError("invalid_arguments", "mode=summary does not accept filters")
		}
		return overview, nil
	case "recent":
		return map[string]interface{}{
			"overview": overview,
			"traces": Automation.RecentCommandTraces(Automation.CommandTraceFilter{
				BrokerID:     input.BrokerID,
				HarnessID:    input.HarnessID,
				SubmissionID: input.SubmissionID,
				WorkID:       input.WorkID,
				Opcode:       input.Opcode,
				Owner:        input.Owner,
				Source:       input.Source,
				Surface:      input.Surface,
				Effect:       input.Effect,
				Status:       input.Status,
				Limit:        input.Limit,
			}),
		}, nil
	case "variants":
		if input.BrokerID != 0 || input.HarnessID != 0 || input.SubmissionID != 0 || input.WorkID != "" || input.Owner != "" || input.Source != "" || input.Surface != "" || input.Effect != "" || input.Status != "" || input.Limit != 0 {
			return nil, toolError("invalid_arguments", "mode=variants accepts only the optional opcode filter")
		}
		return map[string]interface{}{
			"overview": overview,
			"variants": Automation.CommandTraceVariants(input.Opcode),
		}, nil
	default:
		return nil, toolError("invalid_arguments", "mode must be summary, recent, or variants")
	}
}

func validCommandTraceStatus(status string) bool {
	switch status {
	case Automation.CommandTraceQueued,
		Automation.CommandTraceDispatching,
		Automation.CommandTraceSent,
		Automation.CommandTraceRetrying,
		Automation.CommandTraceResponded,
		Automation.CommandTraceResponseError,
		Automation.CommandTraceTransportFailed,
		Automation.CommandTraceCancelled:
		return true
	default:
		return false
	}
}
