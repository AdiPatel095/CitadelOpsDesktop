package movement

import (
	"encoding/json"
	"sync"
)

// PlayerMovement groups auto-bird tracking and GAM-derived active movements.
type PlayerMovement struct {
	mu              sync.RWMutex
	BirdMovements   map[int][]BirdMovement `json:"birdMovements"`   // CastleID -> active bird movements
	ActiveMovements []GAMMovement          `json:"activeMovements"` // Parsed from GAM message(s); single source of truth for attack busy state
	IncomingAttacks []GAMMovement          `json:"incomingAttacks"` // Hostile outbound attacks targeting an owned castle
	// CommanderByMID is a fallback for GAM-like deltas that omit UM.L.ID. Live CAT returns usually
	// carry a new MID and LID, so commander identity is primarily reconciled by LID, not MID.
	CommanderByMID   map[int]int            `json:"-"`
	CommanderRoster  []CommanderRosterEntry `json:"commanderRoster"`
	SnapshotReady    bool                   `json:"snapshotReady"`
	LastSnapshotUnix int64                  `json:"lastSnapshotUnix"`
	SnapshotVersion  uint64                 `json:"snapshotVersion"`
}

type playerMovementJSON struct {
	BirdMovements    map[int][]BirdMovement `json:"birdMovements"`
	ActiveMovements  []GAMMovement          `json:"activeMovements"`
	IncomingAttacks  []GAMMovement          `json:"incomingAttacks"`
	CommanderRoster  []CommanderRosterEntry `json:"commanderRoster"`
	SnapshotReady    bool                   `json:"snapshotReady"`
	LastSnapshotUnix int64                  `json:"lastSnapshotUnix"`
	SnapshotVersion  uint64                 `json:"snapshotVersion"`
}

// NewPlayerMovement returns initialized movement state.
func NewPlayerMovement() PlayerMovement {
	return PlayerMovement{
		BirdMovements:  make(map[int][]BirdMovement),
		CommanderByMID: make(map[int]int),
	}
}

// Reset clears all movement and commander availability state without replacing the lock.
func (pm *PlayerMovement) Reset() {
	pm.mu.Lock()
	pm.BirdMovements = make(map[int][]BirdMovement)
	pm.ActiveMovements = nil
	pm.IncomingAttacks = nil
	pm.CommanderByMID = make(map[int]int)
	pm.CommanderRoster = nil
	pm.SnapshotReady = false
	pm.LastSnapshotUnix = 0
	pm.SnapshotVersion = 0
	pm.mu.Unlock()
}

func (pm *PlayerMovement) MarshalJSON() ([]byte, error) {
	pm.mu.RLock()
	value := playerMovementJSON{
		BirdMovements:    cloneBirdMovements(pm.BirdMovements),
		ActiveMovements:  cloneGAMMovements(pm.ActiveMovements),
		IncomingAttacks:  cloneGAMMovements(pm.IncomingAttacks),
		CommanderRoster:  append([]CommanderRosterEntry(nil), pm.CommanderRoster...),
		SnapshotReady:    pm.SnapshotReady,
		LastSnapshotUnix: pm.LastSnapshotUnix,
		SnapshotVersion:  pm.SnapshotVersion,
	}
	pm.mu.RUnlock()
	return json.Marshal(value)
}

func (pm *PlayerMovement) UnmarshalJSON(data []byte) error {
	var value playerMovementJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	pm.mu.Lock()
	pm.BirdMovements = cloneBirdMovements(value.BirdMovements)
	if pm.BirdMovements == nil {
		pm.BirdMovements = make(map[int][]BirdMovement)
	}
	pm.ActiveMovements = cloneGAMMovements(value.ActiveMovements)
	pm.IncomingAttacks = cloneGAMMovements(value.IncomingAttacks)
	pm.CommanderRoster = append([]CommanderRosterEntry(nil), value.CommanderRoster...)
	pm.SnapshotReady = value.SnapshotReady
	pm.LastSnapshotUnix = value.LastSnapshotUnix
	pm.SnapshotVersion = value.SnapshotVersion
	pm.rebuildCommanderByMIDLocked()
	pm.mu.Unlock()
	return nil
}

func cloneBirdMovements(src map[int][]BirdMovement) map[int][]BirdMovement {
	if src == nil {
		return nil
	}
	out := make(map[int][]BirdMovement, len(src))
	for castleID, movements := range src {
		out[castleID] = append([]BirdMovement(nil), movements...)
	}
	return out
}
