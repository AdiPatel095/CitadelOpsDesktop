package State

import (
	"sort"
	"strings"
	"time"
)

const (
	// PendingCommandRequestLimit bounds retained outbound request identities
	// per opcode. Older entries are discarded first.
	PendingCommandRequestLimit = 64
	// PendingCommandCorrelationWindow is the longest delay between an
	// outbound command and an inbound reply that may still be correlated by
	// FIFO order when the reply carries no causation operation ID.
	PendingCommandCorrelationWindow = 30 * time.Second
)

// RecordPendingCommandRequest appends one outbound request identity. Entries
// from another session generation are dropped first, and each opcode keeps at
// most PendingCommandRequestLimit entries. The slice is always rebuilt so a
// store generation that shares the previous backing array is never mutated.
func (state *GameState) RecordPendingCommandRequest(request PendingCommandRequest) bool {
	if state == nil {
		return false
	}
	request.Opcode = strings.ToLower(strings.TrimSpace(request.Opcode))
	if request.Opcode == "" || request.SentAt.IsZero() {
		return false
	}
	request.SessionGeneration = state.Session.Generation
	current := state.CommandContext.PendingRequests
	next := make([]PendingCommandRequest, 0, len(current)+1)
	sameOpcode := 0
	for _, existing := range current {
		if existing.SessionGeneration != request.SessionGeneration {
			continue
		}
		next = append(next, existing)
		if existing.Opcode == request.Opcode {
			sameOpcode++
		}
	}
	next = append(next, request)
	sameOpcode++
	for sameOpcode > PendingCommandRequestLimit {
		for index, existing := range next {
			if existing.Opcode == request.Opcode {
				next = append(next[:index:index], next[index+1:]...)
				sameOpcode--
				break
			}
		}
	}
	state.CommandContext.PendingRequests = normalizePendingCommandRequests(next)
	return true
}

// TakePendingCommandRequest removes and returns the request answered by an
// inbound reply. A reply carrying a causation operation ID is matched only to
// the oldest request of that operation. Otherwise the oldest request of the
// current session generation is matched when it was sent inside the FIFO
// correlation window. If any request of the opcode has outlived that window,
// FIFO order can no longer be trusted: every expired request is discarded and
// no request is returned, so callers reconcile nothing (fail closed).
func (state *GameState) TakePendingCommandRequest(
	opcode string,
	causationOperationID string,
	receivedAt time.Time,
) (PendingCommandRequest, bool) {
	if state == nil {
		return PendingCommandRequest{}, false
	}
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	causationOperationID = strings.TrimSpace(causationOperationID)
	generation := state.Session.Generation
	current := state.CommandContext.PendingRequests
	match := -1
	expired := false
	for index, existing := range current {
		if existing.Opcode != opcode || existing.SessionGeneration != generation {
			continue
		}
		if causationOperationID != "" {
			if existing.OperationID == causationOperationID {
				match = index
				break
			}
			continue
		}
		if !receivedAt.IsZero() && receivedAt.Sub(existing.SentAt) > PendingCommandCorrelationWindow {
			expired = true
			continue
		}
		if match < 0 {
			match = index
		}
	}
	if causationOperationID == "" && expired {
		match = -1
	}
	next := make([]PendingCommandRequest, 0, len(current))
	var taken PendingCommandRequest
	for index, existing := range current {
		if existing.SessionGeneration != generation {
			continue
		}
		if index == match {
			taken = existing
			continue
		}
		if expired && existing.Opcode == opcode &&
			receivedAt.Sub(existing.SentAt) > PendingCommandCorrelationWindow {
			continue
		}
		next = append(next, existing)
	}
	state.CommandContext.PendingRequests = normalizePendingCommandRequests(next)
	return taken, match >= 0
}

// DropPendingCommandRequestsBefore discards unresolved requests of one opcode
// sent before cutoff. A storage snapshot requested at cutoff already reflects
// every earlier sale, so those identities can no longer be correlated safely.
func (state *GameState) DropPendingCommandRequestsBefore(opcode string, cutoff time.Time) bool {
	if state == nil || cutoff.IsZero() {
		return false
	}
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	current := state.CommandContext.PendingRequests
	next := make([]PendingCommandRequest, 0, len(current))
	for _, existing := range current {
		if existing.SessionGeneration != state.Session.Generation ||
			existing.Opcode == opcode && existing.SentAt.Before(cutoff) {
			continue
		}
		next = append(next, existing)
	}
	if len(next) == len(current) {
		return false
	}
	state.CommandContext.PendingRequests = normalizePendingCommandRequests(next)
	return true
}

// PendingCommandRequests returns the current session's unresolved requests of
// one opcode in send order.
func PendingCommandRequests(state *GameState, opcode string) []PendingCommandRequest {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	result := []PendingCommandRequest{}
	for _, request := range state.CommandContext.PendingRequests {
		if request.Opcode == opcode && request.SessionGeneration == state.Session.Generation {
			result = append(result, request)
		}
	}
	return result
}

func normalizePendingCommandRequests(requests []PendingCommandRequest) []PendingCommandRequest {
	if len(requests) == 0 {
		return nil
	}
	return requests
}

// MarkInventoryEquipmentMutated records an outbound equipment or gem sale.
// The timestamp rides the small gem-stacks persistence part so recording it
// never republishes the complete equipment index.
func (state *GameState) MarkInventoryEquipmentMutated(at time.Time) bool {
	if state == nil || at.IsZero() || !at.After(state.Inventory.EquipmentMutatedAt) {
		return false
	}
	state.MutableInventoryGemStacks()
	state.Inventory.EquipmentMutatedAt = at
	return true
}

// RecruitmentHelpIneligibilityFallback bounds an AHR 269 record when the
// active recruitment job has no known completion time.
const RecruitmentHelpIneligibilityFallback = 30 * time.Minute

// MarkInventoryPackagePurchaseDispatched records an outbound package purchase.
// It rides the construction-offers persistence part with the counters it
// invalidates.
func (state *GameState) MarkInventoryPackagePurchaseDispatched(dispatch PackagePurchaseDispatch) bool {
	if state == nil || dispatch.SentAt.IsZero() || !dispatch.SentAt.After(state.Inventory.LastPackagePurchaseDispatch.SentAt) {
		return false
	}
	state.MutableInventoryConstructionOffers()
	state.Inventory.LastPackagePurchaseDispatch = dispatch
	return true
}

// PackageCountersAfterLastPurchase reports whether counters observed at
// observedAt were committed after the latest dispatched package purchase.
func PackageCountersAfterLastPurchase(gameState *GameState, observedAt time.Time) bool {
	return observedAt.After(gameState.Inventory.LastPackagePurchaseDispatch.SentAt)
}

// RecruitmentAllianceHelpMinimumUnits is the official client threshold for
// recruitment-list help: CastleRecruitDialogUnits treats the list as helpable
// only while some slot holds at least five units without RAH
// (dialog_allianceHelp_requestButton_inactive_minimumFailed: "No recruitment
// slot contains the minimum of 5 units."). Restored by CEO decision on CIT-13
// after its removal in 2.3.5.
const RecruitmentAllianceHelpMinimumUnits = 5

// RecruitmentAllianceHelpItemEligible is the single recruitment help rule used
// by both the automation policy and the request planner/resolver. The official
// client requests T=6 help for the whole recruitment list, so queued jobs stay
// eligible; a job qualifies only with at least five units and no inferred or
// observed RAH, before its known completion, and outside a live AHR 269 record.
// Selecting the first qualifying job therefore requests help exactly when at
// least one non-RAH slot holds five or more units.
func RecruitmentAllianceHelpItemEligible(state *GameState, castleID CastleID, item QueueItem, now time.Time) bool {
	if item.ProductionID <= 0 || item.AllianceHelpRequested ||
		item.Amount < RecruitmentAllianceHelpMinimumUnits {
		return false
	}
	if item.CompletesAt != nil && !now.IsZero() && !item.CompletesAt.After(now) {
		return false
	}
	return !RecruitmentAllianceHelpRejected(state, castleID, item.ProductionID, now)
}

// RecruitmentAllianceHelpRejected reports whether a live AHR 269 record for
// the castle covers the production job.
func RecruitmentAllianceHelpRejected(state *GameState, castleID CastleID, productionID int64, now time.Time) bool {
	record, found := state.AllianceHelpRequests.IneligibleRecruitment[castleID]
	if !found || productionID <= 0 || (!now.IsZero() && !now.Before(record.Until)) {
		return false
	}
	for _, rejectedID := range record.ProductionIDs {
		if rejectedID == productionID {
			return true
		}
	}
	return false
}

// RecordRecruitmentHelpIneligibility stores an AHR 269 answer for the castle's
// current recruitment list. Until is the active job's known completion, or the
// bounded fallback when unknown.
func RecordRecruitmentHelpIneligibility(
	state *GameState,
	castleID CastleID,
	operationID string,
	observedAt time.Time,
) bool {
	if state == nil || castleID <= 0 || observedAt.IsZero() {
		return false
	}
	castle, found := state.Castles[castleID]
	if !found {
		return false
	}
	queue, found := castle.Production[0]
	if !found {
		return false
	}
	ids := []int64{}
	until := observedAt.Add(RecruitmentHelpIneligibilityFallback)
	if queue.Active != nil {
		if queue.Active.ProductionID > 0 {
			ids = append(ids, queue.Active.ProductionID)
		}
		if queue.Active.CompletesAt != nil && queue.Active.CompletesAt.After(observedAt) {
			until = *queue.Active.CompletesAt
		}
	}
	for _, item := range queue.Queued {
		if item.ProductionID > 0 {
			ids = append(ids, item.ProductionID)
		}
	}
	if len(ids) == 0 {
		return false
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	next := pruneRecruitmentHelpIneligibility(state.AllianceHelpRequests.IneligibleRecruitment, state, observedAt)
	if next == nil {
		next = map[CastleID]RecruitmentHelpIneligibility{}
	}
	next[castleID] = RecruitmentHelpIneligibility{
		ProductionIDs: ids, OperationID: operationID, ObservedAt: observedAt, Until: until,
	}
	state.AllianceHelpRequests.IneligibleRecruitment = next
	return true
}

// PruneRecruitmentHelpIneligibility drops expired records and records whose
// rejected jobs have all left the castle's recruitment queue.
func PruneRecruitmentHelpIneligibility(state *GameState, now time.Time) bool {
	if state == nil || len(state.AllianceHelpRequests.IneligibleRecruitment) == 0 {
		return false
	}
	next := pruneRecruitmentHelpIneligibility(state.AllianceHelpRequests.IneligibleRecruitment, state, now)
	if len(next) == len(state.AllianceHelpRequests.IneligibleRecruitment) {
		return false
	}
	state.AllianceHelpRequests.IneligibleRecruitment = next
	return true
}

func pruneRecruitmentHelpIneligibility(
	records map[CastleID]RecruitmentHelpIneligibility,
	state *GameState,
	now time.Time,
) map[CastleID]RecruitmentHelpIneligibility {
	var next map[CastleID]RecruitmentHelpIneligibility
	for castleID, record := range records {
		if !now.IsZero() && !now.Before(record.Until) {
			continue
		}
		if !recruitmentQueueContainsAny(state, castleID, record.ProductionIDs) {
			continue
		}
		if next == nil {
			next = map[CastleID]RecruitmentHelpIneligibility{}
		}
		record.ProductionIDs = append([]int64(nil), record.ProductionIDs...)
		next[castleID] = record
	}
	return next
}

func recruitmentQueueContainsAny(state *GameState, castleID CastleID, productionIDs []int64) bool {
	castle, found := state.Castles[castleID]
	if !found {
		return false
	}
	queue, found := castle.Production[0]
	if !found {
		return false
	}
	for _, productionID := range productionIDs {
		if queue.Active != nil && queue.Active.ProductionID == productionID {
			return true
		}
		for _, item := range queue.Queued {
			if item.ProductionID == productionID {
				return true
			}
		}
	}
	return false
}

func cloneRecruitmentHelpIneligibility(
	source map[CastleID]RecruitmentHelpIneligibility,
) map[CastleID]RecruitmentHelpIneligibility {
	if source == nil {
		return nil
	}
	clone := make(map[CastleID]RecruitmentHelpIneligibility, len(source))
	for castleID, record := range source {
		record.ProductionIDs = append([]int64(nil), record.ProductionIDs...)
		clone[castleID] = record
	}
	return clone
}
