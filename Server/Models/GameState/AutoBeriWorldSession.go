package gamestate

import "sync"

// AutoBeriWorldSession holds ephemeral state from the latest **fuc** troop-space check.
type AutoBeriWorldSession struct {
	LastFucTroopAmount int `json:"lastFucTroopAmount"`
	LastFucParsedSCID  int `json:"lastFucParsedSCID"`
}

var autoBeriWorldSessionMu sync.Mutex

// SetAutoBeriWorldFucResult stores the latest parsed **fuc** response for the transfer loop.
func (gs *GameState) SetAutoBeriWorldFucResult(troopAmount, parsedSCID int) {
	if gs == nil {
		return
	}
	autoBeriWorldSessionMu.Lock()
	defer autoBeriWorldSessionMu.Unlock()
	gs.AutoBeriWorld.LastFucTroopAmount = troopAmount
	gs.AutoBeriWorld.LastFucParsedSCID = parsedSCID
}

// ClearAutoBeriWorldFucResult resets ephemeral **fuc** parse state before a new troop-space check.
func (gs *GameState) ClearAutoBeriWorldFucResult() {
	if gs == nil {
		return
	}
	autoBeriWorldSessionMu.Lock()
	defer autoBeriWorldSessionMu.Unlock()
	gs.AutoBeriWorld.LastFucTroopAmount = 0
	gs.AutoBeriWorld.LastFucParsedSCID = 0
}

// AutoBeriWorldFucResult returns the last parsed **fuc** values (zeros if none yet).
func (gs *GameState) AutoBeriWorldFucResult() (troopAmount, parsedSCID int) {
	if gs == nil {
		return 0, 0
	}
	autoBeriWorldSessionMu.Lock()
	defer autoBeriWorldSessionMu.Unlock()
	return gs.AutoBeriWorld.LastFucTroopAmount, gs.AutoBeriWorld.LastFucParsedSCID
}
