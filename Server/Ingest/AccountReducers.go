package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceInitialState(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if len(frame.Payload) == 0 || json.Unmarshal(frame.Payload, &root) != nil {
		return nil, false, fmt.Errorf("initial state payload is not a JSON object")
	}
	changed := false
	if raw := root["gpi"]; len(raw) > 0 {
		updated, err := applyPlayerInfo(raw, gameState)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["gcl"]; len(raw) > 0 {
		updated, err := applyCastleList(raw, gameState)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["dcl"]; len(raw) > 0 {
		updated, err := applyCastleDetails(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["gcu"]; len(raw) > 0 {
		updated, err := applyPlayerResources(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["sce"]; len(raw) > 0 {
		updated, err := applyPlayerCurrencies(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["gal"]; len(raw) > 0 {
		updated, err := applyAllianceSummary(raw, gameState)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["gli"]; len(raw) > 0 {
		updated, err := applyLeaders(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["sne"]; len(raw) > 0 {
		updated, err := applyReportNotices(raw, frame.ReceivedAt, gameState)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	for section, field := range map[string]string{"gmu": "MP", "ufa": "CF", "ufp": "CFP"} {
		if raw := root[section]; len(raw) > 0 {
			updated, err := applyPlayerMetric(raw, field, section, gameState)
			if err != nil {
				return nil, false, err
			}
			changed = changed || updated
		}
	}
	return []string{"player", "castles", "resources", "alliance", "commanders", "castellans", "equipment", "reports"}, changed, nil
}

func reducePlayerInfo(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyPlayerInfo(frame.Payload, gameState)
	return []string{"player"}, changed, err
}

func reduceGlobalResources(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyPlayerResources(frame.Payload, gameState, gameData)
	return []string{"resources"}, changed, err
}

func reduceAllianceInfo(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		Alliance struct {
			ID      wireInt64         `json:"AID"`
			Name    string            `json:"N"`
			Members []json.RawMessage `json:"M"`
		} `json:"A"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode alliance: %w", err)
	}
	members := make([]State.AllianceMember, 0, len(payload.Alliance.Members))
	holdings := make([]State.AllianceHolding, 0, len(payload.Alliance.Members)*4)
	playerChanged := false
	for _, raw := range payload.Alliance.Members {
		var member struct {
			ID                  wireInt64           `json:"OID"`
			Name                string              `json:"N"`
			RankID              int                 `json:"AR"`
			Level               int                 `json:"L"`
			LegendLevel         int                 `json:"LL"`
			Might               wireFloat64         `json:"MP"`
			Glory               wireFloat64         `json:"CF"`
			AllianceID          wireInt64           `json:"AID"`
			Alliance            string              `json:"AN"`
			ReturnProtectionSec int                 `json:"RPT"`
			Holdings            [][]json.RawMessage `json:"AP"`
		}
		if json.Unmarshal(raw, &member) != nil || member.ID <= 0 {
			continue
		}
		members = append(members, State.AllianceMember{
			PlayerID: State.PlayerID(member.ID), Name: member.Name, RankID: member.RankID,
			Level: member.Level, LegendLevel: member.LegendLevel, Might: float64(member.Might),
			ReturnProtectionSec: member.ReturnProtectionSec,
		})
		for _, row := range member.Holdings {
			if len(row) < 5 {
				continue
			}
			holding := State.AllianceHolding{
				KingdomID: State.KingdomID(rowInt(row, 0)), CastleID: State.CastleID(rowInt(row, 1)),
				X: int(rowInt(row, 2)), Y: int(rowInt(row, 3)), SlotType: int(rowInt(row, 4)),
				PlayerID: State.PlayerID(member.ID),
			}
			if holding.CastleID > 0 && holding.X >= 0 && holding.Y >= 0 {
				holdings = append(holdings, holding)
			}
		}
		if State.PlayerID(member.ID) == gameState.Player.ID {
			if gameState.Player.Level != member.Level || gameState.Player.LegendLevel != member.LegendLevel ||
				gameState.Player.AllianceID != State.AllianceID(member.AllianceID) ||
				gameState.Player.Might != float64(member.Might) || gameState.Player.Glory != float64(member.Glory) {
				playerChanged = true
			}
			gameState.Player.Level = member.Level
			gameState.Player.LegendLevel = member.LegendLevel
			gameState.Player.AllianceID = State.AllianceID(member.AllianceID)
			gameState.Player.Might = float64(member.Might)
			gameState.Player.Glory = float64(member.Glory)
			if payload.Alliance.Name == "" {
				payload.Alliance.Name = member.Alliance
			}
		}
	}
	next := State.AllianceState{
		ID: State.AllianceID(payload.Alliance.ID), Name: payload.Alliance.Name, Members: members, Holdings: holdings,
	}
	if reflect.DeepEqual(gameState.Alliance, next) && !playerChanged {
		return nil, false, nil
	}
	gameState.Alliance = next
	return []string{"alliance", "player"}, true, nil
}

func applyPlayerInfo(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	var player struct {
		ID   wireInt64 `json:"PID"`
		Name string    `json:"PN"`
	}
	if err := json.Unmarshal(raw, &player); err != nil {
		return false, fmt.Errorf("decode player info: %w", err)
	}
	changed := false
	if player.ID > 0 && gameState.Player.ID != State.PlayerID(player.ID) {
		gameState.Player.ID = State.PlayerID(player.ID)
		changed = true
	}
	if player.Name != "" && gameState.Player.Name != player.Name {
		gameState.Player.Name = player.Name
		changed = true
	}
	return changed, nil
}

func applyPlayerResources(raw json.RawMessage, gameState *State.GameState, gameData *GameData.Store) (bool, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return false, fmt.Errorf("decode player resources: %w", err)
	}
	if nested := values["gcu"]; len(nested) > 0 {
		return applyPlayerResources(nested, gameState, gameData)
	}
	if gameState.Player.Resources == nil {
		gameState.Player.Resources = map[State.ResourceID]float64{}
	}
	changed := false
	for jsonKey, rawValue := range values {
		definitionID, ok := officialDefinitionID(gameData, "resources", "resourceID", jsonKey)
		if !ok {
			continue
		}
		amount, ok := rawFloat64(rawValue)
		if !ok {
			continue
		}
		id := State.ResourceID(definitionID)
		if current, exists := gameState.Player.Resources[id]; !exists || current != amount {
			gameState.Player.Resources[id] = amount
			changed = true
		}
	}
	return changed, nil
}

func applyPlayerCurrencies(raw json.RawMessage, gameState *State.GameState, gameData *GameData.Store) (bool, error) {
	rows, ok := decodeRows(raw)
	if !ok {
		return false, fmt.Errorf("decode player currencies: expected row array")
	}
	if gameState.Player.Currencies == nil {
		gameState.Player.Currencies = map[State.CurrencyID]float64{}
	}
	changed := false
	for _, row := range rows {
		jsonKey := rowString(row, 0)
		definitionID, exists := officialDefinitionID(gameData, "currencies", "currencyID", jsonKey)
		if !exists || len(row) < 2 {
			continue
		}
		amount, exists := rawFloat64(row[1])
		if !exists {
			continue
		}
		id := State.CurrencyID(definitionID)
		if current, exists := gameState.Player.Currencies[id]; !exists || current != amount {
			gameState.Player.Currencies[id] = amount
			changed = true
		}
	}
	return changed, nil
}

func applyAllianceSummary(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	var alliance struct {
		ID   wireInt64 `json:"AID"`
		Name string    `json:"N"`
	}
	if err := json.Unmarshal(raw, &alliance); err != nil {
		return false, fmt.Errorf("decode alliance summary: %w", err)
	}
	changed := false
	if alliance.ID > 0 && gameState.Alliance.ID != State.AllianceID(alliance.ID) {
		gameState.Alliance.ID = State.AllianceID(alliance.ID)
		changed = true
	}
	if alliance.ID > 0 && gameState.Player.AllianceID != State.AllianceID(alliance.ID) {
		gameState.Player.AllianceID = State.AllianceID(alliance.ID)
		changed = true
	}
	if alliance.Name != "" && gameState.Alliance.Name != alliance.Name {
		gameState.Alliance.Name = alliance.Name
		changed = true
	}
	return changed, nil
}

func applyPlayerMetric(raw json.RawMessage, field string, metric string, gameState *State.GameState) (bool, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return false, fmt.Errorf("decode player metric %s: %w", metric, err)
	}
	value, ok := rawFloat64(values[field])
	if !ok {
		return false, nil
	}
	switch metric {
	case "gmu":
		if gameState.Player.Might == value {
			return false, nil
		}
		gameState.Player.Might = value
	case "ufa":
		if gameState.Player.Glory == value {
			return false, nil
		}
		gameState.Player.Glory = value
	case "ufp":
		if gameState.Player.Gallantry == value {
			return false, nil
		}
		gameState.Player.Gallantry = value
	default:
		return false, nil
	}
	return true, nil
}

func officialDefinitionID(
	gameData *GameData.Store,
	collection string,
	idField string,
	jsonKey string,
) (int64, bool) {
	if gameData == nil || jsonKey == "" {
		return 0, false
	}
	catalog, err := gameData.Catalog(collection)
	if err != nil {
		return 0, false
	}
	raw, ok := catalog.FindByField("JSONKey", jsonKey)
	if !ok {
		return 0, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, false
	}
	return record.Int64(idField)
}

func officialLevel(gameData *GameData.Store, collection string, id int64) int {
	if gameData == nil || id <= 0 {
		return 0
	}
	catalog, err := gameData.Catalog(collection)
	if err != nil {
		return 0
	}
	raw, ok := catalog.Find(strconv.FormatInt(id, 10))
	if !ok {
		return 0
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0
	}
	level, _ := record.Int64("level")
	return int(level)
}

func frameSucceeded(frame Protocol.Frame) bool {
	return frame.ResponseCode == nil || *frame.ResponseCode == 0
}
