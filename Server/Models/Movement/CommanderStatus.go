package movement

import "sort"

const CommanderSnapshotFreshnessSeconds int64 = 45

type CommanderState string

const (
	CommanderStateSyncing   CommanderState = "syncing"
	CommanderStateUnknown   CommanderState = "unknown"
	CommanderStateFree      CommanderState = "free"
	CommanderStateOutbound  CommanderState = "outbound"
	CommanderStateBusy      CommanderState = "busy"
	CommanderStatePosted    CommanderState = "posted"
	CommanderStateReturning CommanderState = "returning"
)

// CommanderRosterEntry is the stable identity copied from the equipment commander list.
// CommanderID is the wire LID used by GAM and attack commands.
type CommanderRosterEntry struct {
	CommanderID     int    `json:"commanderID"`
	Name            string `json:"name"`
	VisiblePosition int    `json:"visiblePosition"`
}

// CommanderStatusRow is the scheduler/UI projection for one owned commander.
type CommanderStatusRow struct {
	CommanderID     int            `json:"commanderID"`
	Name            string         `json:"name"`
	VisiblePosition int            `json:"visiblePosition"`
	Status          CommanderState `json:"status"`
	Busy            bool           `json:"busy"`
	Movement        *GAMMovement   `json:"movement,omitempty"`
}

// CommanderStatusSnapshot is one internally consistent read of movement and roster state.
type CommanderStatusSnapshot struct {
	ActiveMovements   []GAMMovement
	CommanderStatuses []CommanderStatusRow
	SnapshotReady     bool
	SnapshotFresh     bool
	LastSnapshotUnix  int64
}

func cloneGAMMovement(m GAMMovement) GAMMovement {
	m.TroopArray = cloneTroopArray(m.TroopArray)
	m.MarketGoods = append([]GAMMarketGood(nil), m.MarketGoods...)
	return m
}

func cloneGAMMovements(src []GAMMovement) []GAMMovement {
	if src == nil {
		return nil
	}
	out := make([]GAMMovement, len(src))
	for i := range src {
		out[i] = cloneGAMMovement(src[i])
	}
	return out
}

func cloneTroopArray(src [][]int) [][]int {
	if src == nil {
		return nil
	}
	out := make([][]int, len(src))
	for i := range src {
		out[i] = append([]int(nil), src[i]...)
	}
	return out
}

func snapshotIsFresh(ready bool, lastSnapshotUnix, nowUnix int64) bool {
	if !ready || lastSnapshotUnix <= 0 || nowUnix < lastSnapshotUnix {
		return false
	}
	return nowUnix-lastSnapshotUnix <= CommanderSnapshotFreshnessSeconds
}

func normalizeGAMMovements(src []GAMMovement) []GAMMovement {
	reversed := make([]GAMMovement, 0, len(src))
	seenCommanders := make(map[int]struct{})
	seenMIDs := make(map[int]struct{})
	for i := len(src) - 1; i >= 0; i-- {
		m := src[i]
		if m.CommanderID >= 0 {
			if _, seen := seenCommanders[m.CommanderID]; seen {
				continue
			}
		}
		if m.MID != 0 {
			if _, seen := seenMIDs[m.MID]; seen {
				continue
			}
		}
		if m.CommanderID >= 0 {
			seenCommanders[m.CommanderID] = struct{}{}
		}
		if m.MID != 0 {
			seenMIDs[m.MID] = struct{}{}
		}
		reversed = append(reversed, cloneGAMMovement(m))
	}
	out := make([]GAMMovement, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out
}

func normalizeIncomingGAMMovements(src []GAMMovement) []GAMMovement {
	reversed := make([]GAMMovement, 0, len(src))
	seenMIDs := make(map[int]struct{})
	for i := len(src) - 1; i >= 0; i-- {
		movement := src[i]
		if movement.MID != 0 {
			if _, seen := seenMIDs[movement.MID]; seen {
				continue
			}
			seenMIDs[movement.MID] = struct{}{}
		}
		reversed = append(reversed, cloneGAMMovement(movement))
	}
	out := make([]GAMMovement, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out
}

func (pm *PlayerMovement) rebuildCommanderByMIDLocked() {
	pm.CommanderByMID = make(map[int]int)
	for _, m := range pm.ActiveMovements {
		if m.MID != 0 && m.CommanderID >= 0 {
			pm.CommanderByMID[m.MID] = m.CommanderID
		}
	}
}

// ReplaceSnapshot atomically replaces all owned movements with one fully assembled GAM snapshot.
func (pm *PlayerMovement) ReplaceSnapshot(movements []GAMMovement, observedUnix int64) {
	pm.ReplaceSnapshotWithIncoming(movements, nil, observedUnix)
}

// ReplaceSnapshotWithIncoming atomically publishes one fully assembled GAM snapshot, including
// hostile outbound attacks which must not enter commander availability state.
func (pm *PlayerMovement) ReplaceSnapshotWithIncoming(movements, incoming []GAMMovement, observedUnix int64) {
	pm.mu.Lock()
	pm.ActiveMovements = normalizeGAMMovements(movements)
	pm.IncomingAttacks = normalizeIncomingGAMMovements(incoming)
	pm.SnapshotReady = true
	pm.LastSnapshotUnix = observedUnix
	pm.SnapshotVersion++
	pm.rebuildCommanderByMIDLocked()
	pm.mu.Unlock()
}

// InvalidateSnapshot preserves last-known movement detail but prevents it from proving availability.
func (pm *PlayerMovement) InvalidateSnapshot() {
	pm.mu.Lock()
	pm.SnapshotReady = false
	pm.mu.Unlock()
}

// ApplyDelta replaces a commander's previous leg with a CAT/CRA leg. CAT returns commonly use a new MID.
func (pm *PlayerMovement) ApplyDelta(incoming GAMMovement) bool {
	pm.mu.Lock()
	if incoming.CommanderID < 0 && incoming.MID != 0 {
		if commanderID, ok := pm.CommanderByMID[incoming.MID]; ok {
			incoming.CommanderID = commanderID
		}
	}
	kept := make([]GAMMovement, 0, len(pm.ActiveMovements)+1)
	for _, existing := range pm.ActiveMovements {
		if incoming.MID != 0 && existing.MID == incoming.MID {
			continue
		}
		if incoming.CommanderID >= 0 && existing.CommanderID == incoming.CommanderID {
			continue
		}
		kept = append(kept, existing)
	}
	pm.ActiveMovements = append(kept, cloneGAMMovement(incoming))
	pm.rebuildCommanderByMIDLocked()
	pm.mu.Unlock()
	return true
}

func (pm *PlayerMovement) RemoveMovement(mid int) bool {
	pm.mu.Lock()
	removed := false
	kept := make([]GAMMovement, 0, len(pm.ActiveMovements))
	for _, m := range pm.ActiveMovements {
		if m.MID == mid {
			removed = true
			continue
		}
		kept = append(kept, m)
	}
	if removed {
		pm.ActiveMovements = kept
		pm.rebuildCommanderByMIDLocked()
	}
	incoming := make([]GAMMovement, 0, len(pm.IncomingAttacks))
	for _, movement := range pm.IncomingAttacks {
		if movement.MID == mid {
			removed = true
			continue
		}
		incoming = append(incoming, movement)
	}
	pm.IncomingAttacks = incoming
	pm.mu.Unlock()
	return removed
}

func (pm *PlayerMovement) CommanderForMID(mid int) (int, bool) {
	pm.mu.RLock()
	commanderID, ok := pm.CommanderByMID[mid]
	pm.mu.RUnlock()
	return commanderID, ok
}

func (pm *PlayerMovement) SetCommanderRoster(entries []CommanderRosterEntry) {
	deduped := make(map[int]CommanderRosterEntry, len(entries))
	for _, entry := range entries {
		if entry.CommanderID < 0 {
			continue
		}
		deduped[entry.CommanderID] = entry
	}
	roster := make([]CommanderRosterEntry, 0, len(deduped))
	for _, entry := range deduped {
		roster = append(roster, entry)
	}
	sort.Slice(roster, func(i, j int) bool {
		left, right := roster[i], roster[j]
		if left.VisiblePosition != right.VisiblePosition {
			return left.VisiblePosition < right.VisiblePosition
		}
		return left.CommanderID < right.CommanderID
	})
	pm.mu.Lock()
	pm.CommanderRoster = roster
	pm.mu.Unlock()
}

func (pm *PlayerMovement) Snapshot() ([]GAMMovement, []CommanderRosterEntry, bool, int64) {
	pm.mu.RLock()
	movements := cloneGAMMovements(pm.ActiveMovements)
	roster := append([]CommanderRosterEntry(nil), pm.CommanderRoster...)
	ready := pm.SnapshotReady
	lastSnapshotUnix := pm.LastSnapshotUnix
	pm.mu.RUnlock()
	return movements, roster, ready, lastSnapshotUnix
}

// IncomingSnapshot returns the hostile incoming movements from the latest authoritative GAM frame.
func (pm *PlayerMovement) IncomingSnapshot() ([]GAMMovement, bool, int64, uint64) {
	pm.mu.RLock()
	movements := cloneGAMMovements(pm.IncomingAttacks)
	ready := pm.SnapshotReady
	lastSnapshotUnix := pm.LastSnapshotUnix
	version := pm.SnapshotVersion
	pm.mu.RUnlock()
	return movements, ready, lastSnapshotUnix, version
}

func classifyCommanderMovement(m GAMMovement, nowUnix int64) CommanderState {
	if m.D == 1 {
		return CommanderStateReturning
	}
	if m.TT > 0 && m.EffectivePT(nowUnix) >= m.TT {
		if m.TWD > 0 {
			return CommanderStatePosted
		}
		return CommanderStateBusy
	}
	return CommanderStateOutbound
}

func (pm *PlayerMovement) StatusSnapshot(nowUnix int64) CommanderStatusSnapshot {
	movements, roster, ready, lastSnapshotUnix := pm.Snapshot()
	fresh := snapshotIsFresh(ready, lastSnapshotUnix, nowUnix)
	byCommander := make(map[int]GAMMovement)
	for _, m := range movements {
		if m.CommanderID >= 0 {
			byCommander[m.CommanderID] = m
		}
	}
	rows := make([]CommanderStatusRow, 0, len(roster)+len(byCommander))
	seen := make(map[int]struct{}, len(roster))
	appendRow := func(entry CommanderRosterEntry) {
		row := CommanderStatusRow{
			CommanderID:     entry.CommanderID,
			Name:            entry.Name,
			VisiblePosition: entry.VisiblePosition,
			Status:          CommanderStateSyncing,
			Busy:            true,
		}
		movement, hasMovement := byCommander[entry.CommanderID]
		if hasMovement {
			movementCopy := cloneGAMMovement(movement)
			row.Movement = &movementCopy
		}
		if !ready {
			row.Status = CommanderStateSyncing
		} else if !fresh {
			row.Status = CommanderStateUnknown
		} else if hasMovement {
			row.Status = classifyCommanderMovement(movement, nowUnix)
		} else {
			row.Status = CommanderStateFree
			row.Busy = false
		}
		rows = append(rows, row)
		seen[entry.CommanderID] = struct{}{}
	}
	for _, entry := range roster {
		appendRow(entry)
	}
	for commanderID := range byCommander {
		if _, ok := seen[commanderID]; ok {
			continue
		}
		appendRow(CommanderRosterEntry{CommanderID: commanderID, VisiblePosition: 1 << 30})
	}
	return CommanderStatusSnapshot{
		ActiveMovements:   movements,
		CommanderStatuses: rows,
		SnapshotReady:     ready,
		SnapshotFresh:     fresh,
		LastSnapshotUnix:  lastSnapshotUnix,
	}
}

// CommanderStatus returns the scheduler/UI projection for one owned commander.
func (pm *PlayerMovement) CommanderStatus(commanderID int, nowUnix int64) (CommanderStatusRow, bool) {
	if commanderID < 0 {
		return CommanderStatusRow{}, false
	}
	status := pm.StatusSnapshot(nowUnix)
	for _, row := range status.CommanderStatuses {
		if row.CommanderID == commanderID {
			return row, true
		}
	}
	return CommanderStatusRow{}, false
}

// IsCommanderUnavailable fails closed until a recent complete GAM snapshot proves the commander free.
func (pm *PlayerMovement) IsCommanderUnavailable(commanderID int, nowUnix int64) bool {
	if commanderID < 0 {
		return true
	}
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if !snapshotIsFresh(pm.SnapshotReady, pm.LastSnapshotUnix, nowUnix) {
		return true
	}
	known := false
	for _, commander := range pm.CommanderRoster {
		if commander.CommanderID == commanderID {
			known = true
			break
		}
	}
	if !known {
		return true
	}
	for _, movement := range pm.ActiveMovements {
		if movement.CommanderID == commanderID {
			return true
		}
	}
	return false
}

func (pm *PlayerMovement) HasActiveCommanderMovement() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, m := range pm.ActiveMovements {
		if m.CommanderID >= 0 {
			return true
		}
	}
	return false
}
