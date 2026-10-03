package App

import (
	"context"
	"crypto/sha256"
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
	for key, raw := range fields {
		switch {
		case step.Opcode == "hru" && (key == "U" || key == "A"),
			step.Opcode == "ahr" && key == "T", step.Opcode == "fco" && key == "FS":
			var number json.Number
			if json.Unmarshal(raw, &number) == nil {
				payload[key] = number
			} else {
				payload[key] = "<redacted>"
			}
		case key == "OID" || (step.Opcode == "ahr" && key == "ID"):
			payload[key] = dispatchIdentity(string(raw))
		default:
			payload[key] = "<redacted>"
		}
	}
	encoded, _ := json.Marshal(payload)
	command := step.Command
	command.Namespace = "EmpireEx_<namespace>"
	command.Payload = encoded
	wire, _ := Protocol.Encode(command)
	return string(wire)
}

func sanitizedHospitalJob(item State.QueueItem) map[string]any {
	return map[string]any{"productionIdentity": dispatchIdentity(item.ProductionID),
		"definition": item.Definition.ID, "amount": item.Amount, "startedAt": item.StartedAt,
		"completesAt": item.CompletesAt, "helpAvailable": item.AllianceHelpAvailable,
		"helpRequested": item.AllianceHelpRequested}
}
func sanitizedHospitalQueue(queue State.ProductionQueue) map[string]any {
	queued := make([]map[string]any, 0, len(queue.Queued))
	for _, item := range queue.Queued {
		queued = append(queued, sanitizedHospitalJob(item))
	}
	var active any
	if queue.Active != nil {
		active = sanitizedHospitalJob(*queue.Active)
	}
	return map[string]any{"lineId": queue.LineID, "active": active, "queued": queued,
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
		"epoch":                 protocol.FocusEpoch, "subcontext": protocol.FocusSubcontext, "observedAt": protocol.ObservedAt}
	snapshot := map[string]any{"opcode": opcode, "command": sanitizedDispatchCommand(step),
		"emitter":    map[string]string{"revision": BuildRevision, "version": Version},
		"dispatchAt": time.Now().UTC(), "correlationIdentity": dispatchIdentity(metadata.ResponseToken), "stateRevision": input.State.Revision,
		"castleObservedAt": castle.ContextSnapshotObservedAt, "focus": focus,
		"session": map[string]any{"generation": input.State.Session.Generation, "connectionGeneration": input.State.Session.ConnectionGeneration}}
	switch opcode {
	case "hru":
		snapshot["hospitalQueue"] = sanitizedHospitalQueue(castle.Production[2])
		snapshot["capacity"] = castle.Production[2].Capacity
		snapshot["occupancy"] = hospitalOccupiedSlots(castle.Production[2])
		result := "not_configured"
		if step.FinalDispatchAction != "" {
			result = "passed"
		}
		snapshot["finalGuard"] = map[string]string{"action": step.FinalDispatchAction, "result": result}
		source := "static_plan"
		if resolver == "hospital.heal.build" {
			source = resolver
		}
		snapshot["source"] = source
	case "ahr":
		var jobID int64
		_ = json.Unmarshal(payload["ID"], &jobID)
		queue := castle.Production[2]
		items := append([]State.QueueItem(nil), queue.Queued...)
		if queue.Active != nil {
			items = append(items, *queue.Active)
		}
		var job any
		for _, item := range items {
			if item.ProductionID == jobID {
				job = sanitizedHospitalJob(item)
				break
			}
		}
		snapshot["hospitalJob"] = job
		snapshot["jobIdentity"] = dispatchIdentity(jobID)
		ids := append([]int64(nil), input.State.AllianceHelpRequests.HospitalProductionIDs...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		requests := make([]string, 0, len(ids))
		for _, id := range ids {
			requests = append(requests, dispatchIdentity(id))
		}
		snapshot["listIdentity"] = dispatchIdentity(ids)
		snapshot["helpRequests"] = requests
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
		snapshot["completionEvents"] = append([]State.BuildingCompletionEvent{}, building.CompletionEvents...)
		snapshot["constructionIdentity"] = dispatchIdentity(metadata.OperationID)
	}
	return snapshot
}
