package gamestate

import "sync"

// AutoToolSession holds the resolved runtime plan for Auto Tool.
type AutoToolSession struct {
	ActiveToolByCastle map[int]int `json:"activeToolByCastle,omitempty"`
	UpdatedAtUnixMs    int64       `json:"updatedAtUnixMs,omitempty"`
}

var autoToolSessionMu sync.Mutex

// SetAutoToolActiveTools stores the resolved castle -> tool plan.
func (gs *GameState) SetAutoToolActiveTools(tools map[int]int, updatedAtUnixMs int64) {
	if gs == nil {
		return
	}
	next := make(map[int]int, len(tools))
	for castleID, toolID := range tools {
		if castleID > 0 && toolID > 0 {
			next[castleID] = toolID
		}
	}
	autoToolSessionMu.Lock()
	defer autoToolSessionMu.Unlock()
	gs.AutoTool.ActiveToolByCastle = next
	gs.AutoTool.UpdatedAtUnixMs = updatedAtUnixMs
}

// AutoToolActiveTools returns a copy of the resolved castle -> tool plan.
func (gs *GameState) AutoToolActiveTools() map[int]int {
	if gs == nil {
		return nil
	}
	autoToolSessionMu.Lock()
	defer autoToolSessionMu.Unlock()
	out := make(map[int]int, len(gs.AutoTool.ActiveToolByCastle))
	for castleID, toolID := range gs.AutoTool.ActiveToolByCastle {
		out[castleID] = toolID
	}
	return out
}

// ClearAutoToolActiveTools clears the resolved runtime plan.
func (gs *GameState) ClearAutoToolActiveTools() {
	if gs == nil {
		return
	}
	autoToolSessionMu.Lock()
	defer autoToolSessionMu.Unlock()
	gs.AutoTool.ActiveToolByCastle = nil
	gs.AutoTool.UpdatedAtUnixMs = 0
}
