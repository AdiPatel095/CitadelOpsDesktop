package gamestate

import "sync"

// AutoRecruitSession holds the resolved runtime plan for Auto Recruit.
type AutoRecruitSession struct {
	ActiveUnitByCastle map[int]int `json:"activeUnitByCastle,omitempty"`
	UpdatedAtUnixMs    int64       `json:"updatedAtUnixMs,omitempty"`
}

var autoRecruitSessionMu sync.Mutex

// SetAutoRecruitActiveUnits stores the resolved castle -> unit plan.
func (gs *GameState) SetAutoRecruitActiveUnits(units map[int]int, updatedAtUnixMs int64) {
	if gs == nil {
		return
	}
	next := make(map[int]int, len(units))
	for castleID, unitID := range units {
		if castleID > 0 && unitID > 0 {
			next[castleID] = unitID
		}
	}
	autoRecruitSessionMu.Lock()
	defer autoRecruitSessionMu.Unlock()
	gs.AutoRecruit.ActiveUnitByCastle = next
	gs.AutoRecruit.UpdatedAtUnixMs = updatedAtUnixMs
}

// AutoRecruitActiveUnits returns a copy of the resolved castle -> unit plan.
func (gs *GameState) AutoRecruitActiveUnits() map[int]int {
	if gs == nil {
		return nil
	}
	autoRecruitSessionMu.Lock()
	defer autoRecruitSessionMu.Unlock()
	out := make(map[int]int, len(gs.AutoRecruit.ActiveUnitByCastle))
	for castleID, unitID := range gs.AutoRecruit.ActiveUnitByCastle {
		out[castleID] = unitID
	}
	return out
}

// ClearAutoRecruitActiveUnits clears the resolved runtime plan.
func (gs *GameState) ClearAutoRecruitActiveUnits() {
	if gs == nil {
		return
	}
	autoRecruitSessionMu.Lock()
	defer autoRecruitSessionMu.Unlock()
	gs.AutoRecruit.ActiveUnitByCastle = nil
	gs.AutoRecruit.UpdatedAtUnixMs = 0
}
