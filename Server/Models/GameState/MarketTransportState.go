package gamestate

import (
	"sync"
	"time"
)

// MarketAreaEffect is one cmi AE row retained for game-data-driven capacity calculations.
type MarketAreaEffect struct {
	EffectID int       `json:"effectId"`
	Values   []float64 `json:"values,omitempty"`
	Source   string    `json:"source,omitempty"`
}

// MarketCastleState is one castle row from cmi.
type MarketCastleState struct {
	CastleID         int                `json:"castleId"`
	KingdomID        int                `json:"kingdomId"`
	TotalBarrows     int                `json:"totalBarrows"`
	AvailableBarrows int                `json:"availableBarrows"`
	Resources        map[string]float64 `json:"resources,omitempty"`
	AreaEffects      []MarketAreaEffect `json:"areaEffects,omitempty"`
}

// MarketPendingShipment records a same-kingdom market send started by Citadel this session.
type MarketPendingShipment struct {
	SourceCastleID int     `json:"sourceCastleId"`
	TargetCastleID int     `json:"targetCastleId"`
	Resource       string  `json:"resource"`
	Amount         float64 `json:"amount"`
	ArrivesAtUnix  int64   `json:"arrivesAtUnix"`
}

// MarketTransportState combines cmi state with the account-wide caravan-overloader level from boi.
type MarketTransportState struct {
	Castles            []MarketCastleState     `json:"castles,omitempty"`
	CaravanLevel       int                     `json:"caravanLevel"`
	CaravanLevelLoaded bool                    `json:"caravanLevelLoaded"`
	Pending            []MarketPendingShipment `json:"pending,omitempty"`
}

var marketTransportStateMu sync.Mutex

func cloneMarketCastles(castles []MarketCastleState) []MarketCastleState {
	out := make([]MarketCastleState, len(castles))
	for i := range castles {
		out[i] = castles[i]
		out[i].Resources = make(map[string]float64, len(castles[i].Resources))
		for key, value := range castles[i].Resources {
			out[i].Resources[key] = value
		}
		out[i].AreaEffects = make([]MarketAreaEffect, len(castles[i].AreaEffects))
		for effectIndex := range castles[i].AreaEffects {
			out[i].AreaEffects[effectIndex] = castles[i].AreaEffects[effectIndex]
			out[i].AreaEffects[effectIndex].Values = append([]float64(nil), castles[i].AreaEffects[effectIndex].Values...)
		}
	}
	return out
}

func pruneMarketPending(pending []MarketPendingShipment, nowUnix int64) []MarketPendingShipment {
	out := pending[:0]
	for _, shipment := range pending {
		if shipment.ArrivesAtUnix > nowUnix {
			out = append(out, shipment)
		}
	}
	return out
}

// SetMarketCastles replaces cmi castle rows while retaining booster and in-session pending state.
func (gs *GameState) SetMarketCastles(castles []MarketCastleState) {
	if gs == nil {
		return
	}
	marketTransportStateMu.Lock()
	defer marketTransportStateMu.Unlock()
	gs.MarketTransport.Castles = cloneMarketCastles(castles)
	gs.MarketTransport.Pending = pruneMarketPending(gs.MarketTransport.Pending, time.Now().Unix())
}

// SetMarketCaravanLevel applies the permanent ruby shop level parsed from boi.
func (gs *GameState) SetMarketCaravanLevel(level int) {
	if gs == nil {
		return
	}
	if level < 0 {
		level = 0
	}
	marketTransportStateMu.Lock()
	defer marketTransportStateMu.Unlock()
	gs.MarketTransport.CaravanLevel = level
	gs.MarketTransport.CaravanLevelLoaded = true
}

// RecordMarketShipment protects a known in-flight delivery from duplicate automation sends.
func (gs *GameState) RecordMarketShipment(shipment MarketPendingShipment) {
	if gs == nil || shipment.SourceCastleID <= 0 || shipment.TargetCastleID <= 0 || shipment.Resource == "" || shipment.Amount <= 0 {
		return
	}
	marketTransportStateMu.Lock()
	defer marketTransportStateMu.Unlock()
	gs.MarketTransport.Pending = pruneMarketPending(gs.MarketTransport.Pending, time.Now().Unix())
	gs.MarketTransport.Pending = append(gs.MarketTransport.Pending, shipment)
}

// MarketTransportSnapshot returns a detached, pruned copy of current market state.
func (gs *GameState) MarketTransportSnapshot() MarketTransportState {
	if gs == nil {
		return MarketTransportState{}
	}
	marketTransportStateMu.Lock()
	defer marketTransportStateMu.Unlock()
	gs.MarketTransport.Pending = pruneMarketPending(gs.MarketTransport.Pending, time.Now().Unix())
	return MarketTransportState{
		Castles:            cloneMarketCastles(gs.MarketTransport.Castles),
		CaravanLevel:       gs.MarketTransport.CaravanLevel,
		CaravanLevelLoaded: gs.MarketTransport.CaravanLevelLoaded,
		Pending:            append([]MarketPendingShipment(nil), gs.MarketTransport.Pending...),
	}
}
