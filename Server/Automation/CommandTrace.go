package Automation

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CommandTraceSourceCitadel    = "citadel"
	CommandTraceSourceGameClient = "game_client"

	CommandTraceQueued          = "queued"
	CommandTraceDispatching     = "dispatching"
	CommandTraceSent            = "sent"
	CommandTraceRetrying        = "retrying"
	CommandTraceResponded       = "responded"
	CommandTraceResponseError   = "response_error"
	CommandTraceTransportFailed = "transport_failed"
	CommandTraceCancelled       = "cancelled"
)

const (
	defaultCommandTraceCapacity = 1024
	maxCommandTraceQueryLimit   = 500
	nativeSendCorrelationWindow = 2 * time.Second
	responseFollowupWindow      = 2 * time.Second
)

// CommandTrace is a value-redacted account of one outbound game command. Shapes retain
// JSON keys and value types, but never the payload values themselves.
type CommandTrace struct {
	TraceID        uint64                `json:"traceId"`
	CommandID      uint64                `json:"commandId,omitempty"`
	BrokerID       uint64                `json:"brokerId,omitempty"`
	HarnessID      uint64                `json:"harnessId,omitempty"`
	SubmissionID   uint64                `json:"submissionId,omitempty"`
	FrameIndex     int                   `json:"frameIndex,omitempty"`
	WorkID         string                `json:"workId,omitempty"`
	Owner          string                `json:"owner"`
	Builder        string                `json:"builder,omitempty"`
	Intent         string                `json:"intent,omitempty"`
	Surface        string                `json:"surface,omitempty"`
	Effect         string                `json:"effect,omitempty"`
	Priority       Priority              `json:"priority,omitempty"`
	Lane           Lane                  `json:"lane,omitempty"`
	Source         string                `json:"source"`
	Opcode         string                `json:"opcode"`
	RequestShape   string                `json:"requestShape"`
	RequestFields  []string              `json:"requestFields,omitempty"`
	Status         string                `json:"status"`
	Attempts       int                   `json:"attempts,omitempty"`
	Coalesced      int                   `json:"coalesced,omitempty"`
	QueuedAt       time.Time             `json:"queuedAt,omitempty"`
	DispatchedAt   *time.Time            `json:"dispatchedAt,omitempty"`
	SentAt         *time.Time            `json:"sentAt,omitempty"`
	WireObservedAt *time.Time            `json:"wireObservedAt,omitempty"`
	ResponseAt     *time.Time            `json:"responseAt,omitempty"`
	CompletedAt    *time.Time            `json:"completedAt,omitempty"`
	LatencyMS      int64                 `json:"latencyMs,omitempty"`
	ResponseOpcode string                `json:"responseOpcode,omitempty"`
	ResponseCode   *int                  `json:"responseCode,omitempty"`
	ResponseShape  string                `json:"responseShape,omitempty"`
	ResponseFields []string              `json:"responseFields,omitempty"`
	StateBefore    map[string]StateStamp `json:"stateBefore,omitempty"`
	StateAfter     map[string]StateStamp `json:"stateAfter,omitempty"`
	StateChanges   []string              `json:"stateChanges,omitempty"`
	Error          string                `json:"error,omitempty"`
}

type CommandTraceFilter struct {
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

// CommandShapeVariant aggregates commands with the same opcode and value-redacted request shape.
type CommandShapeVariant struct {
	Opcode         string         `json:"opcode"`
	RequestShape   string         `json:"requestShape"`
	RequestFields  []string       `json:"requestFields,omitempty"`
	Count          int            `json:"count"`
	Sources        []string       `json:"sources,omitempty"`
	Owners         []string       `json:"owners,omitempty"`
	Builders       []string       `json:"builders,omitempty"`
	Intents        []string       `json:"intents,omitempty"`
	Surfaces       []string       `json:"surfaces,omitempty"`
	Effects        []string       `json:"effects,omitempty"`
	StatusCounts   map[string]int `json:"statusCounts"`
	ResponseCodes  map[string]int `json:"responseCodes,omitempty"`
	ResponseShapes map[string]int `json:"responseShapes,omitempty"`
	StateChanges   map[string]int `json:"stateChanges,omitempty"`
	FirstSeenAt    time.Time      `json:"firstSeenAt"`
	LastSeenAt     time.Time      `json:"lastSeenAt"`
}

type CommandTraceOverview struct {
	Total         int            `json:"total"`
	Capacity      int            `json:"capacity"`
	Pending       int            `json:"pending"`
	UniqueOpcodes int            `json:"uniqueOpcodes"`
	BySource      map[string]int `json:"bySource"`
	ByStatus      map[string]int `json:"byStatus"`
	FirstSeenAt   *time.Time     `json:"firstSeenAt,omitempty"`
	LastSeenAt    *time.Time     `json:"lastSeenAt,omitempty"`
}

type commandTraceRecord struct {
	trace        CommandTrace
	payloadHash  [sha256.Size]byte
	pending      bool
	wireObserved bool
}

type commandTraceCommandKey struct {
	BrokerID  uint64
	CommandID uint64
}

type CommandTraceTracker struct {
	mu        sync.RWMutex
	capacity  int
	nextID    uint64
	order     []uint64
	records   map[uint64]*commandTraceRecord
	byCommand map[commandTraceCommandKey]uint64
	pending   map[string][]uint64
}

func NewCommandTraceTracker(capacity int) *CommandTraceTracker {
	if capacity <= 0 {
		capacity = defaultCommandTraceCapacity
	}
	return &CommandTraceTracker{
		capacity:  capacity,
		records:   make(map[uint64]*commandTraceRecord, capacity),
		byCommand: make(map[commandTraceCommandKey]uint64, capacity),
		pending:   make(map[string][]uint64),
	}
}

var CommandTraces = NewCommandTraceTracker(defaultCommandTraceCapacity)

func ObserveCommandQueued(command Command) {
	CommandTraces.RecordQueued(command)
}

func ObserveCommandDispatch(command Command) {
	CommandTraces.RecordDispatch(command)
}

func ObserveCommandSent(command Command) {
	CommandTraces.RecordSent(command)
}

func ObserveNativeCommandSent(payload []byte) {
	CommandTraces.RecordNativeSent(payload)
}

func ObserveCommandRetry(command Command) {
	CommandTraces.RecordRetry(command)
}

func ObserveCommandFailed(command Command, reason string) {
	CommandTraces.RecordTerminal(command, CommandTraceTransportFailed, reason)
}

func ObserveCommandCancelled(command Command, reason string) {
	CommandTraces.RecordTerminal(command, CommandTraceCancelled, reason)
}

func ObserveCommandResponse(opcode string, body []byte, responseCode int, hasResponseCode bool) {
	CommandTraces.RecordResponse(opcode, body, responseCode, hasResponseCode)
}

func RecentCommandTraces(filter CommandTraceFilter) []CommandTrace {
	return CommandTraces.Recent(filter)
}

func CommandTraceVariants(opcode string) []CommandShapeVariant {
	return CommandTraces.Variants(opcode)
}

func CommandTraceOverviewSnapshot() CommandTraceOverview {
	return CommandTraces.Overview()
}

func (t *CommandTraceTracker) RecordQueued(command Command) {
	opcode, shape, fields, hash := describeCommandWire(command)
	if opcode == "" || isSessionOpcode(opcode) {
		return
	}
	now := time.Now()
	queuedAt := command.QueuedAt
	if queuedAt.IsZero() {
		queuedAt = now
	}

	t.mu.Lock()
	commandKey := traceCommandKey(command)
	if traceID := t.byCommand[commandKey]; traceID != 0 {
		if record := t.records[traceID]; record != nil {
			record.trace.BrokerID = command.BrokerID
			record.trace.HarnessID = command.HarnessID
			record.trace.SubmissionID = command.SubmissionID
			record.trace.FrameIndex = command.FrameIndex
			record.trace.WorkID = command.WorkID
			if builder := strings.TrimSpace(command.Builder); builder != "" {
				record.trace.Builder = builder
			}
			if intent := strings.TrimSpace(command.Intent); intent != "" {
				record.trace.Intent = intent
			}
			if surface := strings.TrimSpace(command.Surface); surface != "" {
				record.trace.Surface = surface
			}
			if effect := strings.TrimSpace(command.Effect); effect != "" {
				record.trace.Effect = effect
			}
			if command.Priority > record.trace.Priority {
				record.trace.Owner = normalizedTraceOwner(command.Owner, CommandTraceSourceCitadel)
				record.trace.Priority = command.Priority
			}
			record.trace.Lane = command.Lane
			record.trace.Opcode = opcode
			record.trace.RequestShape = shape
			record.trace.RequestFields = fields
			record.trace.Status = CommandTraceQueued
			record.trace.Coalesced++
			record.payloadHash = hash
			t.mu.Unlock()
			return
		}
	}

	record := t.newRecordLocked(CommandTrace{
		CommandID:     command.ID,
		BrokerID:      command.BrokerID,
		HarnessID:     command.HarnessID,
		SubmissionID:  command.SubmissionID,
		FrameIndex:    command.FrameIndex,
		WorkID:        command.WorkID,
		Owner:         normalizedTraceOwner(command.Owner, CommandTraceSourceCitadel),
		Builder:       strings.TrimSpace(command.Builder),
		Intent:        strings.TrimSpace(command.Intent),
		Surface:       strings.TrimSpace(command.Surface),
		Effect:        strings.TrimSpace(command.Effect),
		Priority:      command.Priority,
		Lane:          command.Lane,
		Source:        CommandTraceSourceCitadel,
		Opcode:        opcode,
		RequestShape:  shape,
		RequestFields: fields,
		Status:        CommandTraceQueued,
		QueuedAt:      queuedAt,
	}, hash)
	if command.ID != 0 {
		t.byCommand[commandKey] = record.trace.TraceID
	}
	t.mu.Unlock()
}

func (t *CommandTraceTracker) RecordDispatch(command Command) {
	now := time.Now()
	t.mu.Lock()
	record := t.ensureCommandRecordLocked(command, now)
	if record != nil {
		record.trace.Status = CommandTraceDispatching
		record.trace.DispatchedAt = traceTime(now)
		record.trace.Attempts = command.Attempts + 1
		record.trace.Error = ""
	}
	t.mu.Unlock()
}

func (t *CommandTraceTracker) RecordSent(command Command) {
	now := time.Now()
	state := commandStateSnapshot(commandOpcode(command))
	t.mu.Lock()
	record := t.ensureCommandRecordLocked(command, now)
	if record != nil {
		t.markSentLocked(record, now, state, false)
	}
	t.mu.Unlock()
}

func (t *CommandTraceTracker) RecordNativeSent(payload []byte) {
	opcode, shape, fields, hash := describeWirePayload(payload)
	if opcode == "" || isSessionOpcode(opcode) {
		return
	}
	now := time.Now()
	state := commandStateSnapshot(opcode)

	t.mu.Lock()
	for i := len(t.order) - 1; i >= 0; i-- {
		record := t.records[t.order[i]]
		if record == nil || record.trace.Source != CommandTraceSourceCitadel || record.payloadHash != hash || record.wireObserved {
			continue
		}
		correlationAt := record.trace.DispatchedAt
		if correlationAt == nil {
			correlationAt = record.trace.SentAt
		}
		if correlationAt == nil || now.Sub(*correlationAt) < 0 || now.Sub(*correlationAt) > nativeSendCorrelationWindow {
			continue
		}
		record.wireObserved = true
		record.trace.WireObservedAt = traceTime(now)
		t.markSentLocked(record, now, state, true)
		t.mu.Unlock()
		return
	}

	record := t.newRecordLocked(CommandTrace{
		Owner:          CommandTraceSourceGameClient,
		Builder:        opcode,
		Intent:         "native_game_client",
		Surface:        CommandTraceSourceGameClient,
		Source:         CommandTraceSourceGameClient,
		Opcode:         opcode,
		RequestShape:   shape,
		RequestFields:  fields,
		Status:         CommandTraceSent,
		Attempts:       1,
		QueuedAt:       now,
		DispatchedAt:   traceTime(now),
		SentAt:         traceTime(now),
		WireObservedAt: traceTime(now),
		StateBefore:    state,
	}, hash)
	record.wireObserved = true
	t.addPendingLocked(record)
	t.mu.Unlock()
}

func (t *CommandTraceTracker) RecordRetry(command Command) {
	t.mu.Lock()
	traceID := t.byCommand[traceCommandKey(command)]
	if record := t.records[traceID]; record != nil {
		t.removePendingLocked(record.trace.TraceID)
		record.trace.Status = CommandTraceRetrying
		record.trace.Attempts = command.Attempts
		record.trace.Error = "transport send will be retried"
		record.wireObserved = false
	}
	t.mu.Unlock()
}

func (t *CommandTraceTracker) RecordTerminal(command Command, status, reason string) {
	if status != CommandTraceTransportFailed && status != CommandTraceCancelled {
		return
	}
	t.mu.Lock()
	traceID := t.byCommand[traceCommandKey(command)]
	if record := t.records[traceID]; record != nil {
		t.removePendingLocked(record.trace.TraceID)
		record.trace.Status = status
		record.trace.Error = strings.TrimSpace(reason)
		record.trace.CompletedAt = traceTime(time.Now())
	}
	t.mu.Unlock()
}

func (t *CommandTraceTracker) RecordResponse(opcode string, body []byte, responseCode int, hasResponseCode bool) {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" || isSessionOpcode(opcode) {
		return
	}
	shape, fields := describeJSONShape(body)
	now := time.Now()
	state := commandStateSnapshot(opcode)

	t.mu.Lock()
	queue := t.pending[opcode]
	var record *commandTraceRecord
	for len(queue) > 0 {
		traceID := queue[0]
		queue = queue[1:]
		candidate := t.records[traceID]
		if candidate != nil && candidate.pending && candidate.trace.Status == CommandTraceSent {
			record = candidate
			break
		}
	}
	if len(queue) == 0 {
		delete(t.pending, opcode)
	} else {
		t.pending[opcode] = queue
	}
	if record == nil {
		record = t.followupResponseRecordLocked(opcode, now)
		if record == nil {
			t.mu.Unlock()
			return
		}
	}

	t.removePendingLocked(record.trace.TraceID)
	if hasResponseCode && responseCode != 0 {
		record.trace.Status = CommandTraceResponseError
		record.trace.Error = "game response returned a non-zero code"
	} else {
		record.trace.Status = CommandTraceResponded
		record.trace.Error = ""
	}
	record.trace.ResponseAt = traceTime(now)
	record.trace.CompletedAt = traceTime(now)
	record.trace.ResponseOpcode = opcode
	record.trace.ResponseCode = nil
	if hasResponseCode {
		code := responseCode
		record.trace.ResponseCode = &code
	}
	record.trace.ResponseShape = shape
	record.trace.ResponseFields = fields
	record.trace.StateAfter = state
	record.trace.StateChanges = changedStateKeys(record.trace.StateBefore, state)
	if record.trace.SentAt != nil {
		record.trace.LatencyMS = now.Sub(*record.trace.SentAt).Milliseconds()
	}
	t.mu.Unlock()
}

func (t *CommandTraceTracker) Recent(filter CommandTraceFilter) []CommandTrace {
	filter.Opcode = strings.ToLower(strings.TrimSpace(filter.Opcode))
	filter.WorkID = strings.TrimSpace(filter.WorkID)
	filter.Owner = strings.TrimSpace(filter.Owner)
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Surface = strings.TrimSpace(filter.Surface)
	filter.Effect = strings.TrimSpace(filter.Effect)
	filter.Status = strings.TrimSpace(filter.Status)
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxCommandTraceQueryLimit {
		limit = maxCommandTraceQueryLimit
	}

	t.mu.RLock()
	out := make([]CommandTrace, 0, min(limit, len(t.order)))
	for i := len(t.order) - 1; i >= 0 && len(out) < limit; i-- {
		record := t.records[t.order[i]]
		if record == nil ||
			(filter.BrokerID != 0 && record.trace.BrokerID != filter.BrokerID) ||
			(filter.HarnessID != 0 && record.trace.HarnessID != filter.HarnessID) ||
			(filter.SubmissionID != 0 && record.trace.SubmissionID != filter.SubmissionID) ||
			(filter.WorkID != "" && record.trace.WorkID != filter.WorkID) ||
			(filter.Opcode != "" && record.trace.Opcode != filter.Opcode) ||
			(filter.Owner != "" && record.trace.Owner != filter.Owner) ||
			(filter.Source != "" && record.trace.Source != filter.Source) ||
			(filter.Surface != "" && record.trace.Surface != filter.Surface) ||
			(filter.Effect != "" && record.trace.Effect != filter.Effect) ||
			(filter.Status != "" && record.trace.Status != filter.Status) {
			continue
		}
		out = append(out, cloneCommandTrace(record.trace))
	}
	t.mu.RUnlock()
	return out
}

func (t *CommandTraceTracker) Variants(opcode string) []CommandShapeVariant {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	t.mu.RLock()
	variants := make(map[string]*CommandShapeVariant)
	sets := make(map[string]map[string]map[string]struct{})
	for _, traceID := range t.order {
		record := t.records[traceID]
		if record == nil || (opcode != "" && record.trace.Opcode != opcode) {
			continue
		}
		trace := record.trace
		key := trace.Opcode + "\x00" + trace.RequestShape
		variant := variants[key]
		if variant == nil {
			variant = &CommandShapeVariant{
				Opcode:         trace.Opcode,
				RequestShape:   trace.RequestShape,
				RequestFields:  append([]string(nil), trace.RequestFields...),
				StatusCounts:   make(map[string]int),
				ResponseCodes:  make(map[string]int),
				ResponseShapes: make(map[string]int),
				StateChanges:   make(map[string]int),
				FirstSeenAt:    trace.QueuedAt,
				LastSeenAt:     trace.QueuedAt,
			}
			variants[key] = variant
			sets[key] = map[string]map[string]struct{}{
				"sources": {}, "owners": {}, "builders": {}, "intents": {}, "surfaces": {}, "effects": {},
			}
		}
		variant.Count++
		variant.StatusCounts[trace.Status]++
		seenAt := trace.QueuedAt
		if variant.FirstSeenAt.IsZero() || (!seenAt.IsZero() && seenAt.Before(variant.FirstSeenAt)) {
			variant.FirstSeenAt = seenAt
		}
		if seenAt.After(variant.LastSeenAt) {
			variant.LastSeenAt = seenAt
		}
		addVariantSet(sets[key]["sources"], trace.Source)
		addVariantSet(sets[key]["owners"], trace.Owner)
		addVariantSet(sets[key]["builders"], trace.Builder)
		addVariantSet(sets[key]["intents"], trace.Intent)
		addVariantSet(sets[key]["surfaces"], trace.Surface)
		addVariantSet(sets[key]["effects"], trace.Effect)
		if trace.ResponseCode != nil {
			variant.ResponseCodes[strconv.Itoa(*trace.ResponseCode)]++
		}
		if trace.ResponseShape != "" {
			variant.ResponseShapes[trace.ResponseShape]++
		}
		for _, stateKey := range trace.StateChanges {
			variant.StateChanges[stateKey]++
		}
	}
	t.mu.RUnlock()

	out := make([]CommandShapeVariant, 0, len(variants))
	for key, variant := range variants {
		variant.Sources = sortedVariantSet(sets[key]["sources"])
		variant.Owners = sortedVariantSet(sets[key]["owners"])
		variant.Builders = sortedVariantSet(sets[key]["builders"])
		variant.Intents = sortedVariantSet(sets[key]["intents"])
		variant.Surfaces = sortedVariantSet(sets[key]["surfaces"])
		variant.Effects = sortedVariantSet(sets[key]["effects"])
		if len(variant.ResponseCodes) == 0 {
			variant.ResponseCodes = nil
		}
		if len(variant.ResponseShapes) == 0 {
			variant.ResponseShapes = nil
		}
		if len(variant.StateChanges) == 0 {
			variant.StateChanges = nil
		}
		out = append(out, *variant)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Opcode != out[j].Opcode {
			return out[i].Opcode < out[j].Opcode
		}
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].RequestShape < out[j].RequestShape
	})
	return out
}

func (t *CommandTraceTracker) Overview() CommandTraceOverview {
	t.mu.RLock()
	overview := CommandTraceOverview{
		Total:    len(t.order),
		Capacity: t.capacity,
		BySource: make(map[string]int),
		ByStatus: make(map[string]int),
	}
	opcodes := make(map[string]struct{})
	for _, traceID := range t.order {
		record := t.records[traceID]
		if record == nil {
			continue
		}
		trace := record.trace
		opcodes[trace.Opcode] = struct{}{}
		overview.BySource[trace.Source]++
		overview.ByStatus[trace.Status]++
		if record.pending {
			overview.Pending++
		}
		seenAt := trace.QueuedAt
		if overview.FirstSeenAt == nil || (!seenAt.IsZero() && seenAt.Before(*overview.FirstSeenAt)) {
			overview.FirstSeenAt = traceTime(seenAt)
		}
		if overview.LastSeenAt == nil || seenAt.After(*overview.LastSeenAt) {
			overview.LastSeenAt = traceTime(seenAt)
		}
	}
	overview.UniqueOpcodes = len(opcodes)
	t.mu.RUnlock()
	return overview
}

func (t *CommandTraceTracker) ensureCommandRecordLocked(command Command, now time.Time) *commandTraceRecord {
	commandKey := traceCommandKey(command)
	if traceID := t.byCommand[commandKey]; traceID != 0 {
		if record := t.records[traceID]; record != nil {
			return record
		}
	}
	opcode, shape, fields, hash := describeCommandWire(command)
	if opcode == "" || isSessionOpcode(opcode) {
		return nil
	}
	queuedAt := command.QueuedAt
	if queuedAt.IsZero() {
		queuedAt = now
	}
	record := t.newRecordLocked(CommandTrace{
		CommandID:     command.ID,
		BrokerID:      command.BrokerID,
		HarnessID:     command.HarnessID,
		SubmissionID:  command.SubmissionID,
		FrameIndex:    command.FrameIndex,
		WorkID:        command.WorkID,
		Owner:         normalizedTraceOwner(command.Owner, CommandTraceSourceCitadel),
		Builder:       strings.TrimSpace(command.Builder),
		Intent:        strings.TrimSpace(command.Intent),
		Surface:       strings.TrimSpace(command.Surface),
		Effect:        strings.TrimSpace(command.Effect),
		Priority:      command.Priority,
		Lane:          command.Lane,
		Source:        CommandTraceSourceCitadel,
		Opcode:        opcode,
		RequestShape:  shape,
		RequestFields: fields,
		Status:        CommandTraceQueued,
		QueuedAt:      queuedAt,
	}, hash)
	if command.ID != 0 {
		t.byCommand[commandKey] = record.trace.TraceID
	}
	return record
}

func (t *CommandTraceTracker) followupResponseRecordLocked(opcode string, now time.Time) *commandTraceRecord {
	if opcode != "jca" {
		return nil
	}
	for i := len(t.order) - 1; i >= 0; i-- {
		record := t.records[t.order[i]]
		if record == nil || record.trace.Opcode != "jca" || record.trace.ResponseOpcode != "jaa" || record.trace.ResponseAt == nil {
			continue
		}
		elapsed := now.Sub(*record.trace.ResponseAt)
		if elapsed >= 0 && elapsed <= responseFollowupWindow {
			return record
		}
		if elapsed > responseFollowupWindow {
			return nil
		}
	}
	return nil
}

func (t *CommandTraceTracker) newRecordLocked(trace CommandTrace, hash [sha256.Size]byte) *commandTraceRecord {
	for len(t.order) >= t.capacity {
		t.evictOldestLocked()
	}
	t.nextID++
	trace.TraceID = t.nextID
	record := &commandTraceRecord{trace: trace, payloadHash: hash}
	t.records[trace.TraceID] = record
	t.order = append(t.order, trace.TraceID)
	return record
}

func (t *CommandTraceTracker) evictOldestLocked() {
	if len(t.order) == 0 {
		return
	}
	traceID := t.order[0]
	t.order = t.order[1:]
	record := t.records[traceID]
	if record != nil {
		commandKey := commandTraceCommandKey{BrokerID: record.trace.BrokerID, CommandID: record.trace.CommandID}
		if record.trace.CommandID != 0 && t.byCommand[commandKey] == traceID {
			delete(t.byCommand, commandKey)
		}
		t.removePendingLocked(traceID)
	}
	delete(t.records, traceID)
}

func (t *CommandTraceTracker) markSentLocked(record *commandTraceRecord, now time.Time, state map[string]StateStamp, wireObserved bool) {
	if record.trace.SentAt == nil {
		record.trace.SentAt = traceTime(now)
	}
	if wireObserved && record.trace.WireObservedAt == nil {
		record.trace.WireObservedAt = traceTime(now)
	}
	record.trace.Status = CommandTraceSent
	if record.trace.Attempts == 0 {
		record.trace.Attempts = 1
	}
	if len(record.trace.StateBefore) == 0 {
		record.trace.StateBefore = cloneStateStamps(state)
	}
	record.trace.Error = ""
	t.addPendingLocked(record)
}

func (t *CommandTraceTracker) addPendingLocked(record *commandTraceRecord) {
	if record.pending {
		return
	}
	record.pending = true
	for _, opcode := range expectedResponseOpcodes(record.trace.Opcode) {
		t.pending[opcode] = append(t.pending[opcode], record.trace.TraceID)
	}
}

func (t *CommandTraceTracker) removePendingLocked(traceID uint64) {
	record := t.records[traceID]
	if record == nil || !record.pending {
		return
	}
	record.pending = false
	for opcode, queue := range t.pending {
		kept := queue[:0]
		for _, candidate := range queue {
			if candidate != traceID {
				kept = append(kept, candidate)
			}
		}
		if len(kept) == 0 {
			delete(t.pending, opcode)
		} else {
			t.pending[opcode] = kept
		}
	}
}

func describeWirePayload(payload []byte) (string, string, []string, [sha256.Size]byte) {
	hash := sha256.Sum256(payload)
	value := string(payload)
	parts := strings.SplitN(value, "%", 6)
	if len(parts) <= 2 {
		return "", "", nil, hash
	}
	opcode := strings.ToLower(strings.TrimSpace(parts[2]))
	body := ""
	if strings.HasPrefix(parts[2], "EmpireEx_") {
		if len(parts) <= 3 {
			return "", "", nil, hash
		}
		opcode = strings.ToLower(strings.TrimSpace(parts[3]))
		if len(parts) > 5 {
			body = strings.TrimSuffix(parts[5], "%")
		}
	} else if len(parts) > 4 {
		body = strings.TrimSuffix(parts[4], "%")
	}
	shape, fields := describeJSONShape([]byte(body))
	return opcode, shape, fields, hash
}

func describeJSONShape(body []byte) (string, []string) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "empty", nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return "raw", nil
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "raw", nil
	}
	fields := topLevelJSONFields(value)
	return jsonShape(value, 0), fields
}

func jsonShape(value interface{}, depth int) string {
	if depth >= 8 {
		return "..."
	}
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			return "number"
		}
		return "integer"
	case []interface{}:
		if len(typed) == 0 {
			return "array<empty>"
		}
		shapes := make(map[string]struct{})
		for _, item := range typed {
			shapes[jsonShape(item, depth+1)] = struct{}{}
		}
		items := sortedVariantSet(shapes)
		return "array<" + strings.Join(items, "|") + ">"
	case map[string]interface{}:
		if len(typed) == 0 {
			return "object{}"
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, key+":"+jsonShape(typed[key], depth+1))
		}
		return "object{" + strings.Join(parts, ",") + "}"
	default:
		return "unknown"
	}
}

func topLevelJSONFields(value interface{}) []string {
	object, ok := value.(map[string]interface{})
	if !ok || len(object) == 0 {
		return nil
	}
	fields := make([]string, 0, len(object))
	for field := range object {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func commandOpcode(command Command) string {
	if opcode := strings.ToLower(strings.TrimSpace(command.Opcode)); opcode != "" {
		return opcode
	}
	opcode, _, _, _ := describeWirePayload(command.Payload)
	return opcode
}

func describeCommandWire(command Command) (string, string, []string, [sha256.Size]byte) {
	opcode, shape, fields, hash := describeWirePayload(command.Payload)
	if preservedOpcode := strings.ToLower(strings.TrimSpace(command.Opcode)); preservedOpcode != "" {
		opcode = preservedOpcode
	}
	if command.RequestShape != "" {
		shape = command.RequestShape
	}
	if command.RequestFields != nil {
		fields = append([]string(nil), command.RequestFields...)
	}
	return opcode, shape, fields, hash
}

func expectedResponseOpcodes(opcode string) []string {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	if opcode == "" {
		return nil
	}
	if opcode == "jca" {
		return []string{"jca", "jaa"}
	}
	return []string{opcode}
}

func isSessionOpcode(opcode string) bool {
	switch strings.ToLower(strings.TrimSpace(opcode)) {
	case "lli", "vck", "nch", "pin":
		return true
	default:
		return false
	}
}

func commandStateSnapshot(opcode string) map[string]StateStamp {
	keys := []string{
		StateAll,
		StateSession,
		StateFocus,
		StateCastles,
		StateResources,
		StateMovement,
		StateInventory,
		StateTransport,
		StateEquipment,
		StateAlliance,
		StateOpcode(opcode),
	}
	all := State.All()
	out := make(map[string]StateStamp, len(keys))
	for _, key := range keys {
		stamp := all[key]
		if stamp.Version != 0 || !stamp.UpdatedAt.IsZero() {
			out[key] = stamp
		}
	}
	return out
}

func changedStateKeys(before, after map[string]StateStamp) []string {
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	changed := make([]string, 0, len(keys))
	for key := range keys {
		if before[key].Version != after[key].Version {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	return changed
}

func normalizedTraceOwner(owner, source string) string {
	owner = strings.TrimSpace(owner)
	if owner != "" {
		return owner
	}
	if source == CommandTraceSourceGameClient {
		return CommandTraceSourceGameClient
	}
	return OwnerManual
}

func traceCommandKey(command Command) commandTraceCommandKey {
	return commandTraceCommandKey{BrokerID: command.BrokerID, CommandID: command.ID}
}

func addVariantSet(set map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		set[value] = struct{}{}
	}
}

func sortedVariantSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func cloneCommandTrace(trace CommandTrace) CommandTrace {
	trace.RequestFields = append([]string(nil), trace.RequestFields...)
	trace.ResponseFields = append([]string(nil), trace.ResponseFields...)
	trace.StateChanges = append([]string(nil), trace.StateChanges...)
	trace.StateBefore = cloneStateStamps(trace.StateBefore)
	trace.StateAfter = cloneStateStamps(trace.StateAfter)
	if trace.ResponseCode != nil {
		code := *trace.ResponseCode
		trace.ResponseCode = &code
	}
	trace.DispatchedAt = cloneTraceTime(trace.DispatchedAt)
	trace.SentAt = cloneTraceTime(trace.SentAt)
	trace.WireObservedAt = cloneTraceTime(trace.WireObservedAt)
	trace.ResponseAt = cloneTraceTime(trace.ResponseAt)
	trace.CompletedAt = cloneTraceTime(trace.CompletedAt)
	return trace
}

func traceTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

func cloneTraceTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return traceTime(*value)
}

func cloneStateStamps(stamps map[string]StateStamp) map[string]StateStamp {
	if len(stamps) == 0 {
		return nil
	}
	out := make(map[string]StateStamp, len(stamps))
	for key, stamp := range stamps {
		out[key] = stamp
	}
	return out
}
