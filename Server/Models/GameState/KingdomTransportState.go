package gamestate

import "sync"

// KingdomTransportGood is one resource amount travelling in a kingdom transfer.
type KingdomTransportGood struct {
	Resource string  `json:"resource"`
	Amount   float64 `json:"amount"`
}

// KingdomResourceTransport is one pending **kpi.RT** resource shipment.
type KingdomResourceTransport struct {
	KingdomID    int                    `json:"kingdomId"`
	Goods        []KingdomTransportGood `json:"goods"`
	RemainingSec int                    `json:"remainingSec"`
}

// KingdomTransportUnlock is one **kpi.UL** kingdom transport capability row.
type KingdomTransportUnlock struct {
	KingdomID int `json:"kingdomId"`
	Unlocked  int `json:"unlocked"`
	Created   int `json:"created"`
	Stage     int `json:"stage,omitempty"`
}

// KingdomTransportState is the latest complete **kpi** snapshot.
type KingdomTransportState struct {
	Unlocks []KingdomTransportUnlock   `json:"unlocks,omitempty"`
	Pending []KingdomResourceTransport `json:"pending,omitempty"`
}

var kingdomTransportStateMu sync.Mutex

// SetKingdomTransportState replaces the current kpi snapshot.
func (gs *GameState) SetKingdomTransportState(state KingdomTransportState) {
	if gs == nil {
		return
	}
	kingdomTransportStateMu.Lock()
	defer kingdomTransportStateMu.Unlock()
	gs.KingdomTransport = state
}

// KingdomTransportSnapshot returns a detached copy of the latest kpi snapshot.
func (gs *GameState) KingdomTransportSnapshot() KingdomTransportState {
	if gs == nil {
		return KingdomTransportState{}
	}
	kingdomTransportStateMu.Lock()
	defer kingdomTransportStateMu.Unlock()
	state := KingdomTransportState{
		Unlocks: append([]KingdomTransportUnlock(nil), gs.KingdomTransport.Unlocks...),
		Pending: make([]KingdomResourceTransport, len(gs.KingdomTransport.Pending)),
	}
	for i, pending := range gs.KingdomTransport.Pending {
		state.Pending[i] = pending
		state.Pending[i].Goods = append([]KingdomTransportGood(nil), pending.Goods...)
	}
	return state
}
