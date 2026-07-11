package Automation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	CommandContractVersion      = 1
	maxCommandSubmissionSize    = 256
	maxCommandFrameBytes        = 4 << 20
	maxCommandSubmissionBytes   = 16 << 20
	maxCommandMetadataTextBytes = 256
)

// CommandFrame is one already-built game wire frame admitted through CommandHarness.
// Payload is deliberately excluded from serialized contracts; callers expose structured
// command arguments through Toolkit rather than transporting raw authenticated wire data.
type CommandFrame struct {
	Opcode  string `json:"opcode,omitempty"`
	Payload []byte `json:"-"`
}

// CommandSubmission is the shared in-process boundary used by the app and Toolkit adapters.
// Command names the reusable structured builder; Intent names the higher-level operation.
type CommandSubmission struct {
	ContractVersion int            `json:"contractVersion,omitempty"`
	Command         string         `json:"command,omitempty"`
	Intent          string         `json:"intent,omitempty"`
	Surface         string         `json:"surface,omitempty"`
	Effect          string         `json:"effect,omitempty"`
	Frames          []CommandFrame `json:"frames"`
	Options         CommandOptions `json:"-"`
}

type CommandShape struct {
	Signature string   `json:"signature"`
	Fields    []string `json:"fields,omitempty"`
}

type CommandStateCursor struct {
	StateKey     string `json:"stateKey"`
	AfterVersion uint64 `json:"afterVersion"`
}

type CommandFrameReceipt struct {
	Index       int                `json:"index"`
	CommandID   uint64             `json:"commandId,omitempty"`
	Opcode      string             `json:"opcode"`
	Lane        Lane               `json:"lane"`
	Shape       CommandShape       `json:"shape"`
	Observation CommandStateCursor `json:"observation"`
	QueuedAt    *time.Time         `json:"queuedAt,omitempty"`
}

// CommandReceipt is safe to return through an LLM tool, IPC, HTTP, or an internal caller.
// It preserves command identity and wire shape without returning the wire payload or values.
type CommandReceipt struct {
	ContractVersion int                   `json:"contractVersion"`
	BrokerID        uint64                `json:"brokerId"`
	HarnessID       uint64                `json:"harnessId"`
	SubmissionID    uint64                `json:"submissionId"`
	Command         string                `json:"command,omitempty"`
	Intent          string                `json:"intent,omitempty"`
	Surface         string                `json:"surface,omitempty"`
	Effect          string                `json:"effect,omitempty"`
	Owner           string                `json:"owner,omitempty"`
	WorkID          string                `json:"workId,omitempty"`
	Accepted        bool                  `json:"accepted"`
	Code            string                `json:"code,omitempty"`
	Message         string                `json:"message,omitempty"`
	Frames          []CommandFrameReceipt `json:"frames,omitempty"`
	Observations    []CommandStateCursor  `json:"observations,omitempty"`
}

// CommandDispatcher is the portable seam implemented by the local CommandHarness. A future
// companion or platform adapter can preserve the same submission/receipt contract.
type CommandDispatcher interface {
	Dispatch(context.Context, CommandSubmission) CommandReceipt
}

// CommandHarness validates command envelopes, derives redacted shapes, assigns lanes, captures
// state cursors, and atomically admits frames to a broker.
type CommandHarness struct {
	broker           *CommandBroker
	id               uint64
	nextSubmissionID atomic.Uint64
}

var nextCommandHarnessID atomic.Uint64

func NewCommandHarness(broker *CommandBroker) *CommandHarness {
	return &CommandHarness{broker: broker, id: nextCommandHarnessID.Add(1)}
}

// OutboundCommandHarness is the process-wide admission boundary. A nil broker makes it follow
// the replaceable Commands global used by the running app and focused tests.
var OutboundCommandHarness = NewCommandHarness(nil)

func DispatchCommands(ctx context.Context, submission CommandSubmission) CommandReceipt {
	return OutboundCommandHarness.Dispatch(ctx, submission)
}

func (h *CommandHarness) Dispatch(ctx context.Context, submission CommandSubmission) CommandReceipt {
	if ctx == nil {
		ctx = context.Background()
	}
	broker := h.broker
	if broker == nil {
		broker = Commands
	}
	receipt := CommandReceipt{
		ContractVersion: CommandContractVersion,
		BrokerID:        broker.id,
		HarnessID:       h.id,
		SubmissionID:    h.nextSubmissionID.Add(1),
		Command:         strings.TrimSpace(submission.Command),
		Intent:          strings.TrimSpace(submission.Intent),
		Surface:         strings.TrimSpace(submission.Surface),
		Effect:          strings.TrimSpace(submission.Effect),
		WorkID:          strings.TrimSpace(submission.Options.WorkID),
	}
	if receipt.Surface == "" {
		receipt.Surface = strings.TrimSpace(submission.Options.Surface)
	}
	if receipt.Surface == "" {
		receipt.Surface = CommandSurfaceRuntime
	}
	if receipt.Effect == "" {
		receipt.Effect = strings.TrimSpace(submission.Options.Effect)
	}
	if submission.ContractVersion != 0 && submission.ContractVersion != CommandContractVersion {
		return rejectCommandReceipt(receipt, "unsupported_contract", "unsupported command contract version %d", submission.ContractVersion)
	}
	if len(submission.Frames) == 0 {
		return rejectCommandReceipt(receipt, "empty_submission", "command submission contains no frames")
	}
	if len(submission.Frames) > maxCommandSubmissionSize {
		return rejectCommandReceipt(receipt, "submission_too_large", "command submission contains %d frames; maximum is %d", len(submission.Frames), maxCommandSubmissionSize)
	}
	for field, value := range map[string]string{
		"command": receipt.Command,
		"intent":  receipt.Intent,
		"surface": receipt.Surface,
		"effect":  receipt.Effect,
		"workId":  receipt.WorkID,
	} {
		if len(value) > maxCommandMetadataTextBytes {
			return rejectCommandReceipt(receipt, "metadata_too_large", "%s exceeds %d bytes", field, maxCommandMetadataTextBytes)
		}
	}
	if err := ctx.Err(); err != nil {
		return rejectCommandReceipt(receipt, "cancelled", "%v", err)
	}

	type preparedFrame struct {
		receipt CommandFrameReceipt
	}
	prepared := make([]preparedFrame, len(submission.Frames))
	opcodes := make([]string, len(submission.Frames))
	totalPayloadBytes := 0
	for index, frame := range submission.Frames {
		if len(frame.Payload) > maxCommandFrameBytes {
			return rejectCommandReceipt(receipt, "frame_too_large", "frame %d exceeds %d bytes", index, maxCommandFrameBytes)
		}
		totalPayloadBytes += len(frame.Payload)
		if totalPayloadBytes > maxCommandSubmissionBytes {
			return rejectCommandReceipt(receipt, "submission_too_large", "command submission exceeds %d wire bytes", maxCommandSubmissionBytes)
		}
		opcode, shape, fields, _ := describeWirePayload(frame.Payload)
		if opcode == "" {
			return rejectCommandReceipt(receipt, "invalid_frame", "frame %d is not a recognized game command envelope", index)
		}
		if isSessionOpcode(opcode) {
			return rejectCommandReceipt(receipt, "session_opcode_forbidden", "frame %d uses session opcode %q, which cannot be queued through the command harness", index, opcode)
		}
		declaredOpcode := strings.ToLower(strings.TrimSpace(frame.Opcode))
		if declaredOpcode != "" && declaredOpcode != opcode {
			return rejectCommandReceipt(receipt, "opcode_mismatch", "frame %d declares opcode %q but contains %q", index, declaredOpcode, opcode)
		}
		opcodes[index] = opcode
		prepared[index].receipt = CommandFrameReceipt{
			Index:  index,
			Opcode: opcode,
			Lane:   commandLaneForOpcode(opcode),
			Shape: CommandShape{
				Signature: shape,
				Fields:    append([]string(nil), fields...),
			},
		}
	}

	logicalCommand := receipt.Command
	if logicalCommand == "" {
		logicalCommand = strings.TrimSpace(submission.Options.Builder)
	}
	if logicalCommand == "" {
		if len(opcodes) == 1 {
			logicalCommand = opcodes[0]
		} else {
			logicalCommand = "wire_batch"
		}
	}
	intent := receipt.Intent
	if intent == "" {
		intent = strings.TrimSpace(submission.Options.Intent)
	}
	if intent == "" {
		intent = logicalCommand
	}
	if len(logicalCommand) > maxCommandMetadataTextBytes || len(intent) > maxCommandMetadataTextBytes {
		return rejectCommandReceipt(receipt, "metadata_too_large", "resolved command or intent exceeds %d bytes", maxCommandMetadataTextBytes)
	}
	receipt.Command = logicalCommand
	receipt.Intent = intent

	seenState := make(map[string]bool, len(opcodes))
	commands := make([]Command, len(prepared))
	for index := range prepared {
		opcode := opcodes[index]
		stateKey := StateOpcode(opcode)
		cursor := CommandStateCursor{StateKey: stateKey, AfterVersion: StateSnapshot(stateKey).Version}
		prepared[index].receipt.Observation = cursor
		if !seenState[stateKey] {
			seenState[stateKey] = true
			receipt.Observations = append(receipt.Observations, cursor)
		}

		options := submission.Options
		options.Builder = logicalCommand
		options.Intent = intent
		options.Surface = receipt.Surface
		options.Effect = receipt.Effect
		options.Lane = commandLaneForOpcode(opcode)
		if len(prepared) > 1 && options.CoalesceKey != "" {
			options.CoalesceKey += ":frame:" + strconv.Itoa(index)
		}
		command := options.command(submission.Frames[index].Payload)
		command.BrokerID = receipt.BrokerID
		command.HarnessID = receipt.HarnessID
		command.SubmissionID = receipt.SubmissionID
		command.FrameIndex = index
		command.Opcode = opcode
		command.RequestShape = prepared[index].receipt.Shape.Signature
		command.RequestFields = append([]string(nil), prepared[index].receipt.Shape.Fields...)
		commands[index] = command
	}
	if len(commands) > 0 {
		receipt.Owner = commands[0].Owner
	}
	if err := ctx.Err(); err != nil {
		return rejectCommandReceipt(receipt, "cancelled", "%v", err)
	}

	accepted, ok := broker.submitBatch(commands)
	if !ok {
		receipt.Frames = make([]CommandFrameReceipt, len(prepared))
		for index := range prepared {
			receipt.Frames[index] = prepared[index].receipt
		}
		return rejectCommandReceipt(receipt, "queue_rejected", "command broker rejected the atomic %d-frame submission", len(commands))
	}

	receipt.Accepted = true
	receipt.Frames = make([]CommandFrameReceipt, len(accepted))
	for index, command := range accepted {
		ObserveCommandQueued(command)
		frameReceipt := prepared[index].receipt
		frameReceipt.CommandID = command.ID
		queuedAt := command.QueuedAt
		frameReceipt.QueuedAt = &queuedAt
		receipt.Frames[index] = frameReceipt
	}
	return receipt
}

func commandLaneForOpcode(opcode string) Lane {
	if strings.EqualFold(strings.TrimSpace(opcode), "cra") {
		return LaneAttackLaunch
	}
	return LaneCommand
}

func rejectCommandReceipt(receipt CommandReceipt, code, format string, args ...interface{}) CommandReceipt {
	receipt.Accepted = false
	receipt.Code = code
	receipt.Message = fmt.Sprintf(format, args...)
	return receipt
}
