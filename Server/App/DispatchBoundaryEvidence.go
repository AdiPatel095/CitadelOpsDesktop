package App

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// Diagnostic budgets do not affect the command or any dispatch decision.
const dispatchEvidenceMaxItems = 32
const dispatchEvidenceMaxBytes = 16 << 10
const dispatchEvidenceMaxCommandBytes = 1024

func dispatchText(value string) string {
	if len(value) > 128 {
		return "<redacted>"
	}
	return value
}

func dispatchListIdentity(ids []int64) string {
	digest := sha256.New()
	var data [8]byte
	for _, id := range ids {
		binary.LittleEndian.PutUint64(data[:], uint64(id))
		_, _ = digest.Write(data[:])
	}
	return hex.EncodeToString(digest.Sum(nil)[:8])
}

func dispatchIdentity(value any) string {
	digest := sha256.Sum256([]byte(fmt.Sprint(value)))
	return hex.EncodeToString(digest[:8])
}

// Only protocol-defined public values are retained. Unknown fields are redacted
// rather than relying on an ever-growing list of sensitive field names.
func sanitizedDispatchCommand(step Intent.Step) string {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(step.Command.Payload, &fields)
	payload := map[string]any{}
	redactedFields := 0
	for key, raw := range fields {
		switch {
		case step.Opcode == "hru" && (key == "U" || key == "A"),
			step.Opcode == "ahr" && key == "T", step.Opcode == "fco" && key == "FS":
			var number json.Number
			if len(raw) <= 32 && json.Unmarshal(raw, &number) == nil {
				payload[key] = number
			} else {
				payload[key] = "<redacted>"
			}
		case key == "OID" || (step.Opcode == "ahr" && key == "ID"):
			payload[key] = dispatchIdentity(string(raw))
		default:
			redactedFields++
		}
	}
	if redactedFields > 0 {
		payload["redactedFields"] = redactedFields
	}
	encoded, _ := json.Marshal(payload)
	command := step.Command
	command.Namespace = "EmpireEx_<namespace>"
	command.Payload = encoded
	wire, _ := Protocol.Encode(command)
	if len(wire) > dispatchEvidenceMaxCommandBytes {
		command.Payload = json.RawMessage(`{"truncated":true}`)
		wire, _ = Protocol.Encode(command)
	}
	return string(wire)
}

func sanitizedHospitalJob(item State.QueueItem) map[string]any {
	return map[string]any{"productionIdentity": dispatchIdentity(item.ProductionID),
		"definition": item.Definition.ID, "amount": item.Amount, "startedAt": item.StartedAt,
		"completesAt": item.CompletesAt, "helpAvailable": item.AllianceHelpAvailable,
		"helpRequested": item.AllianceHelpRequested}
}
func sanitizedHospitalQueue(queue State.ProductionQueue) map[string]any {
	queued := make([]map[string]any, 0, min(len(queue.Queued), dispatchEvidenceMaxItems))
	for _, item := range queue.Queued[:min(len(queue.Queued), dispatchEvidenceMaxItems)] {
		queued = append(queued, sanitizedHospitalJob(item))
	}
	var active any
	if queue.Active != nil {
		active = sanitizedHospitalJob(*queue.Active)
	}
	return map[string]any{"lineId": queue.LineID, "active": active, "queued": queued,
		"queuedCount": len(queue.Queued), "queuedTruncated": len(queue.Queued) > len(queued),
		"slots": queue.Slots, "capacity": queue.Capacity, "occupancy": hospitalOccupiedSlots(queue), "observedAt": queue.ObservedAt}
}

func captureDispatchBoundaryEvidence(ctx context.Context, input Intent.PlanningContext, step Intent.Step, resolver string, _ []byte) any {
	metadata := Outbound.MetadataFromContext(ctx)
	opcode := strings.ToLower(step.Opcode)
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(step.Command.Payload, &payload)
	// AHR/2 evidence is the hospital path only. Recruitment receipts stay CIT-13's.
	var helpType int
	_ = json.Unmarshal(payload["T"], &helpType)
	if opcode == "ahr" && helpType != allianceHelpHospitalType {
		return nil
	}
	castleID := input.ProtocolContext.FocusedCastleID
	var args struct {
		CastleID State.CastleID `json:"castleId"`
	}
	_ = json.Unmarshal(step.FinalDispatchArguments, &args)
	if args.CastleID > 0 {
		castleID = args.CastleID
	}
	castle := input.State.Castles[castleID]
	protocol := input.ProtocolContext
	focus := map[string]any{"identity": "<castle>", "focused": castle.Focused,
		"matchesCommittedFocus": castleID == protocol.FocusedCastleID,
		"epoch":                 protocol.FocusEpoch, "subcontext": dispatchText(string(protocol.FocusSubcontext)), "observedAt": protocol.ObservedAt}
	snapshot := map[string]any{"opcode": opcode, "command": sanitizedDispatchCommand(step),
		"emitter":    map[string]string{"revision": dispatchText(BuildRevision), "version": dispatchText(Version)},
		"dispatchAt": time.Now().UTC(), "correlationIdentity": dispatchIdentity(metadata.ResponseToken), "stateRevision": input.State.Revision,
		"castleObservedAt": castle.ContextSnapshotObservedAt, "focus": focus,
		"session": map[string]any{"generation": input.State.Session.Generation, "connectionGeneration": input.State.Session.ConnectionGeneration}}
	switch opcode {
	case "crm":
		var source State.CastleID
		_ = json.Unmarshal(payload["SID"], &source)
		var goods [][]json.RawMessage
		_ = json.Unmarshal(payload["G"], &goods)
		var amount int64
		if len(goods) == 1 && len(goods[0]) >= 2 {
			_ = json.Unmarshal(goods[0][1], &amount)
		}
		now := time.Now().UTC()
		row := input.State.Market.Castles[source]
		ids := make([]State.CastleID, 0, len(input.State.Castles))
		for id := range input.State.Castles {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		ordinal := 0
		for i, id := range ids {
			if id == source {
				ordinal = i + 1
				break
			}
		}
		required, err := marketBarrowsRequired(input, source, amount)
		age := float64(0)
		if !row.ObservedAt.IsZero() {
			age = now.Sub(row.ObservedAt).Seconds()
		}
		snapshot["market"] = map[string]any{"sourceOrdinal": ordinal, "availableBarrows": row.AvailableBarrows,
			"totalBarrows": row.TotalBarrows, "observedAt": row.ObservedAt, "ageSec": age,
			"lastRefreshAt": input.State.Market.ObservedAt, "requiredBarrows": required, "capacityKnown": err == nil,
			"computedAvailable": State.AvailableMarketBarrowsAt(&input.State, row, now),
			"lease":             State.MarketBarrowLeaseAt(&input.State, source, now),
			"ready":             State.MarketBarrowSourceStatusAt(&input.State, source, now).Ready}
	case "hru":
		snapshot["hospitalQueue"] = sanitizedHospitalQueue(castle.Production[2])
		snapshot["capacity"] = castle.Production[2].Capacity
		snapshot["occupancy"] = hospitalOccupiedSlots(castle.Production[2])
		result := "not_configured"
		if step.FinalDispatchAction != "" {
			result = "passed"
		}
		snapshot["finalGuard"] = map[string]string{"action": dispatchText(step.FinalDispatchAction), "result": result}
		source := "static_plan"
		if resolver == "hospital.heal.build" {
			source = resolver
		}
		snapshot["source"] = source
	case "ahr":
		var jobID int64
		_ = json.Unmarshal(payload["ID"], &jobID)
		queue := castle.Production[2]
		items := queue.Queued
		var job any
		for _, item := range items {
			if item.ProductionID == jobID {
				job = sanitizedHospitalJob(item)
				break
			}
		}
		if queue.Active != nil && queue.Active.ProductionID == jobID {
			job = sanitizedHospitalJob(*queue.Active)
		}
		snapshot["hospitalJob"] = job
		snapshot["jobIdentity"] = dispatchIdentity(jobID)
		allIDs := input.State.AllianceHelpRequests.HospitalProductionIDs
		ids := append([]int64(nil), allIDs[:min(len(allIDs), dispatchEvidenceMaxItems)]...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		requests := make([]string, 0, len(ids))
		for _, id := range ids {
			requests = append(requests, dispatchIdentity(id))
		}
		snapshot["listIdentity"] = dispatchListIdentity(allIDs)
		snapshot["helpRequests"] = requests
		snapshot["helpRequestCount"] = len(allIDs)
		snapshot["helpRequestsTruncated"] = len(allIDs) > len(requests)
		snapshot["helpObservedAt"] = input.State.AllianceHelpRequests.ObservedAt
		snapshot["helpGeneration"] = input.State.AllianceHelpRequests.OwnObservedGeneration
		snapshot["castle"] = map[string]any{"identity": "<castle>", "focused": castle.Focused, "observedAt": castle.ContextSnapshotObservedAt}
	case "fco":
		var objectID State.BuildingInstanceID
		_ = json.Unmarshal(payload["OID"], &objectID)
		building, found := castle.Buildings[objectID]
		snapshot["target"] = map[string]any{"identity": dispatchIdentity(objectID), "found": found,
			"definition": building.DefinitionID, "constructionState": building.ConstructionState,
			"progressSec": building.ProgressSec, "level": building.Level, "observedAt": castle.Layout.ObservedAt}
		snapshot["targetVersion"] = input.Partitions.Version(State.CastlePartition(&input.State, "building-layout", castleID))
		events := append([]State.BuildingCompletionEvent{}, building.CompletionEvents[:min(len(building.CompletionEvents), dispatchEvidenceMaxItems)]...)
		for i := range events {
			switch events[i].Opcode {
			case "fco", "eup", "bup", "jaa":
			default:
				events[i].Opcode = "<redacted>"
			}
		}
		snapshot["completionEvents"] = events
		snapshot["completionEventCount"] = len(building.CompletionEvents)
		snapshot["completionEventsTruncated"] = len(building.CompletionEvents) > len(events)
		snapshot["constructionIdentity"] = dispatchIdentity(metadata.OperationID)
	}
	return boundedDispatchSnapshot(snapshot)
}

func boundedDispatchSnapshot(snapshot map[string]any) any {
	snapshot["maxBytes"] = dispatchEvidenceMaxBytes
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return nil
	}
	if len(encoded) > dispatchEvidenceMaxBytes {
		// Preserve required scalar state and identities, summarize variable lists.
		snapshot["truncated"] = true
		if queue, ok := snapshot["hospitalQueue"].(map[string]any); ok {
			queue["queued"] = []any{}
			queue["queuedTruncated"] = true
		}
		if _, ok := snapshot["helpRequests"]; ok {
			snapshot["helpRequests"] = []any{}
			snapshot["helpRequestsTruncated"] = true
		}
		if _, ok := snapshot["completionEvents"]; ok {
			snapshot["completionEvents"] = []any{}
			snapshot["completionEventsTruncated"] = true
		}
		encoded, err = json.Marshal(snapshot)
	}
	if err != nil || len(encoded) > dispatchEvidenceMaxBytes {
		return nil
	}
	return json.RawMessage(encoded)
}
