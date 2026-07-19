package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

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
	accountChanged := false
	if raw := root["gpi"]; len(raw) > 0 {
		player, err := decodePlayerInfo(raw)
		if err != nil {
			return nil, false, err
		}
		incomingPlayerID := State.PlayerID(player.ID)
		incomingWorldID := strings.TrimSpace(gameState.Session.ServerURL)
		boundWorldID, boundPlayerID := State.BoundAccount(*gameState)
		if incomingPlayerID > 0 && ((boundPlayerID > 0 && incomingPlayerID != boundPlayerID) ||
			(incomingWorldID != "" && boundWorldID != "" && !strings.EqualFold(incomingWorldID, boundWorldID))) {
			resetInitialAccountState(gameState)
			// CommitFrame records the observation before invoking the reducer. The
			// account reset intentionally removed the prior account's protocol
			// history, so record this new account's baseline again.
			recordObservation(gameState, frame, "")
			accountChanged = true
			changed = true
		}
		updated := applyDecodedPlayerInfo(player, gameState)
		changed = changed || updated
		bindingChanged := incomingPlayerID > 0 && gameState.Account.PlayerID != incomingPlayerID
		if incomingPlayerID > 0 && incomingWorldID != "" &&
			!strings.EqualFold(strings.TrimSpace(gameState.Account.WorldID), incomingWorldID) {
			bindingChanged = true
		}
		if bindingChanged {
			gameState.Account.PlayerID = incomingPlayerID
			if incomingWorldID != "" {
				gameState.Account.WorldID = incomingWorldID
			}
			gameState.Account.BoundAt = frame.ReceivedAt.UTC()
			changed = true
		}
	}
	if raw := root["gcl"]; len(raw) > 0 {
		updated, err := applyCastleList(raw, gameState)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["dcl"]; len(raw) > 0 {
		updated, err := applyCastleDetails(raw, gameState, gameData, frame.ReceivedAt)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["gpc"]; len(raw) > 0 {
		updated, err := applyQueueableProduction(raw, gameState, gameData, frame.ReceivedAt)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["vip"]; len(raw) > 0 {
		updated, err := applyVIPInfo(raw, gameState)
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
	if raw := root["vli"]; len(raw) > 0 {
		updated, err := applyAchievementSnapshot(raw, frame.ReceivedAt, gameState)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["skl"]; len(raw) > 0 {
		updated, err := applyLegendSkills(raw, frame.ReceivedAt, gameState)
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
	if raw := root["gie"]; len(raw) > 0 {
		updated, err := applyGenerals(raw, frame.ReceivedAt, gameState)
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
	if raw := root["sei"]; len(raw) > 0 {
		updated, err := applyScalableEventSnapshot(raw, frame.ReceivedAt, gameState, gameData)
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
	for _, embedded := range []struct {
		opcode  string
		reducer Reducer
	}{
		{"sie", reduceSubscriptions}, {"upc", reduceSubscriptions},
		{"boi", reduceMarketBooster}, {"cmi", reduceMarketInfo}, {"kpi", reduceKingdomTransport},
	} {
		raw := root[embedded.opcode]
		if len(raw) == 0 {
			continue
		}
		nestedFrame := frame
		nestedFrame.Opcode = embedded.opcode
		nestedFrame.Payload = raw
		_, updated, err := embedded.reducer(context.Background(), nestedFrame, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	baselineChanged := false
	if gameState.Session.LoggedIn && gameState.Session.SocketReady &&
		gameState.Session.Generation > 0 && !frame.ReceivedAt.Before(gameState.Session.ChangedAt) &&
		gameState.Session.BaselineGeneration != gameState.Session.Generation {
		gameState.Session.BaselineGeneration = gameState.Session.Generation
		changed = true
		baselineChanged = true
	}
	domains := []string{
		"player", "castles", "resources", "currencies", "alliance", "commanders", "castellans",
		"equipment", "generals", "general-skills", "reports", "subscriptions", "market", "kingdom-transport", "production", "events", "event-scores", "achievements", "legend-skills",
	}
	if accountChanged {
		domains = []string{
			"session-context", "player", "castles", "resources", "currencies", "alliance", "alliances",
			"commanders", "castellans", "generals", "general-skills", "equipment", "gems", "inventory",
			"movements", "stationing", "scheduled", "automations", "reports", "subscriptions", "market",
			"kingdom-transport", "production", "crafting", "buildings", "building-layout", "building-queue",
			"building-construction", "construction-items", "construction-offers", "units", "defense", "map",
			"events", "event-scores", "tower-cooldowns", "tower-queue", "invasion", "storm", "nomad-camps",
			"khan", "rift", "attack-dialog", "achievements", "legend-skills", "command-context",
		}
	}
	if baselineChanged {
		domains = append(domains, "session")
	}
	return domains, changed, nil
}

func resetInitialAccountState(gameState *State.GameState) {
	if gameState == nil {
		return
	}
	revision := gameState.Revision
	updatedAt := gameState.UpdatedAt
	session := gameState.Session
	catalogVersion := gameState.CatalogVersion
	languageVersion := gameState.LanguageVersion
	next := State.NewGameState()
	next.Revision = revision
	next.UpdatedAt = updatedAt
	next.Session = session
	next.CatalogVersion = catalogVersion
	next.LanguageVersion = languageVersion
	*gameState = next
}

func reduceLegendSkills(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyLegendSkills(frame.Payload, frame.ReceivedAt, gameState)
	return []string{"player", "legend-skills"}, changed, err
}

func applyLegendSkills(raw json.RawMessage, observedAt time.Time, gameState *State.GameState) (bool, error) {
	var payload struct {
		ActiveIDs         []wireInt64 `json:"SID"`
		ResetRemainingSec wireInt64   `json:"RS"`
		SkillPoints       wireInt64   `json:"SP"`
		ResetCount        wireInt64   `json:"RC"`
		SceatSkillIDs     []wireInt64 `json:"SIDS"`
		SceatActivations  []struct {
			ID           wireInt64 `json:"ID"`
			RemainingSec wireInt64 `json:"RS"`
		} `json:"SSA"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode Hall of Legends skills: %w", err)
	}
	activationsByID := make(map[int64]int64, len(payload.SceatActivations))
	for _, activation := range payload.SceatActivations {
		if activation.ID > 0 {
			activationsByID[int64(activation.ID)] = max(int64(activation.RemainingSec), 0)
		}
	}
	activations := make([]State.SceatSkillActivation, 0, len(activationsByID))
	for id, remainingSec := range activationsByID {
		activations = append(activations, State.SceatSkillActivation{ID: id, RemainingSec: remainingSec})
	}
	sort.Slice(activations, func(left, right int) bool { return activations[left].ID > activations[right].ID })
	updated := State.LegendSkillState{
		ActiveIDs:         normalizedLegendSkillWireIDs(payload.ActiveIDs),
		SkillPoints:       max(int64(payload.SkillPoints), 0),
		ResetRemainingSec: max(int64(payload.ResetRemainingSec), 0),
		ResetCount:        max(int64(payload.ResetCount), 0),
		SceatSkillIDs:     normalizedLegendSkillWireIDs(payload.SceatSkillIDs),
		SceatActivations:  activations,
		ObservedAt:        observedAt,
	}
	changed := !reflect.DeepEqual(gameState.Player.LegendSkills, updated)
	gameState.Player.LegendSkills = updated
	return changed, nil
}

func reduceLegendSkillPurchase(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	if gameState.Player.LegendSkills.ObservedAt.IsZero() {
		return nil, false, fmt.Errorf("Hall of Legends state has not been observed before SKP")
	}
	if gameData == nil {
		return nil, false, fmt.Errorf("official Hall of Legends skill catalog is unavailable")
	}
	var payload struct {
		IDs []wireInt64 `json:"IDS"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Hall of Legends purchase: %w", err)
	}
	affectedIDs := normalizedLegendSkillWireIDs(payload.IDs)
	if len(affectedIDs) == 0 {
		return []string{"player", "legend-skills"}, false, nil
	}
	catalog, err := gameData.Catalog("legendskills")
	if err != nil {
		return nil, false, fmt.Errorf("load Hall of Legends skill catalog: %w", err)
	}
	replacedGroups := make(map[legendSkillGroup]struct{}, len(affectedIDs))
	for _, id := range affectedIDs {
		group, exists, groupErr := legendSkillGroupForID(catalog, id)
		if groupErr != nil {
			return nil, false, groupErr
		}
		if !exists {
			return nil, false, fmt.Errorf("SKP returned unknown Hall of Legends skill %d", id)
		}
		replacedGroups[group] = struct{}{}
	}
	activeIDs := make([]int64, 0, len(gameState.Player.LegendSkills.ActiveIDs)+len(affectedIDs))
	for _, id := range gameState.Player.LegendSkills.ActiveIDs {
		group, exists, groupErr := legendSkillGroupForID(catalog, id)
		if groupErr != nil {
			return nil, false, groupErr
		}
		if exists {
			if _, replaced := replacedGroups[group]; replaced {
				continue
			}
		}
		activeIDs = append(activeIDs, id)
	}
	activeIDs = append(activeIDs, affectedIDs...)
	updated := gameState.Player.LegendSkills
	updated.ActiveIDs = normalizedLegendSkillIDs(activeIDs)
	elapsed := frame.ReceivedAt.Sub(updated.ObservedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedSec := int64(elapsed / time.Second)
	updated.ResetRemainingSec = max(updated.ResetRemainingSec-elapsedSec, 0)
	updated.SceatActivations = append([]State.SceatSkillActivation(nil), updated.SceatActivations...)
	for index := range updated.SceatActivations {
		updated.SceatActivations[index].RemainingSec = max(updated.SceatActivations[index].RemainingSec-elapsedSec, 0)
	}
	updated.ObservedAt = frame.ReceivedAt
	changed := !reflect.DeepEqual(gameState.Player.LegendSkills, updated)
	gameState.Player.LegendSkills = updated
	return []string{"player", "legend-skills"}, changed, nil
}

func reduceLegendSkillReset(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) {
		return nil, false, nil
	}
	updated := gameState.Player.LegendSkills
	updated.ActiveIDs = []int64{}
	updated.ObservedAt = time.Time{}
	changed := !reflect.DeepEqual(gameState.Player.LegendSkills, updated)
	gameState.Player.LegendSkills = updated
	return []string{"player", "legend-skills"}, changed, nil
}

type legendSkillGroup struct {
	TreeID  int64
	GroupID int64
}

func legendSkillGroupForID(catalog *GameData.Catalog, id int64) (legendSkillGroup, bool, error) {
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return legendSkillGroup{}, false, nil
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return legendSkillGroup{}, false, fmt.Errorf("decode Hall of Legends skill %d: %w", id, err)
	}
	treeID, treeExists := record.Int64("skillTreeID")
	groupID, groupExists := record.Int64("skillGroupID")
	if !treeExists || !groupExists {
		return legendSkillGroup{}, false, fmt.Errorf("Hall of Legends skill %d has no tree/group identity", id)
	}
	return legendSkillGroup{TreeID: treeID, GroupID: groupID}, true, nil
}

func normalizedLegendSkillWireIDs(values []wireInt64) []int64 {
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		ids = append(ids, int64(value))
	}
	return normalizedLegendSkillIDs(ids)
}

func normalizedLegendSkillIDs(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	ids := make([]int64, 0, len(values))
	for _, id := range values {
		if id <= 0 {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] > ids[right] })
	return ids
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

func reducePlayerCurrencies(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyPlayerCurrencies(frame.Payload, gameState, gameData)
	return []string{"resources", "currencies"}, changed, err
}

func newPlayerMetricReducer(field string, metric string) Reducer {
	return func(
		_ context.Context,
		frame Protocol.Frame,
		gameState *State.GameState,
		_ *GameData.Store,
	) ([]string, bool, error) {
		if !frameSucceeded(frame) || len(frame.Payload) == 0 {
			return nil, false, nil
		}
		changed, err := applyPlayerMetric(frame.Payload, field, metric, gameState)
		return []string{"player", "resources"}, changed, err
	}
}

func reducePlayerSummary(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode player summary: %w", err)
	}
	beforePlayer := gameState.Player
	beforeAlliance := gameState.Alliance
	applyCastleOwner(root["O"], gameState)
	changed := !reflect.DeepEqual(beforePlayer, gameState.Player) || !reflect.DeepEqual(beforeAlliance, gameState.Alliance)
	return []string{"player", "alliance", "resources"}, changed, nil
}

func reduceCastleDetails(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyCastleDetails(frame.Payload, gameState, gameData, frame.ReceivedAt)
	return []string{"castles", "resources", "units"}, changed, err
}

func reduceVIPInfo(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyVIPInfo(frame.Payload, gameState)
	return []string{"player"}, changed, err
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
	containsCurrentPlayer := false
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
			containsCurrentPlayer = true
			playerAllianceID := State.AllianceID(member.AllianceID)
			if playerAllianceID <= 0 {
				playerAllianceID = State.AllianceID(payload.Alliance.ID)
			}
			nameChanged := member.Name != "" && gameState.Player.Name != member.Name
			if nameChanged || gameState.Player.Level != member.Level || gameState.Player.LegendLevel != member.LegendLevel ||
				gameState.Player.AllianceID != playerAllianceID ||
				gameState.Player.Might != float64(member.Might) || gameState.Player.Glory != float64(member.Glory) {
				playerChanged = true
			}
			if member.Name != "" {
				gameState.Player.Name = member.Name
			}
			gameState.Player.Level = member.Level
			gameState.Player.LegendLevel = member.LegendLevel
			gameState.Player.AllianceID = playerAllianceID
			gameState.Player.Might = float64(member.Might)
			gameState.Player.Glory = float64(member.Glory)
			if payload.Alliance.Name == "" {
				payload.Alliance.Name = member.Alliance
			}
		}
	}
	next := State.AllianceState{
		ID: State.AllianceID(payload.Alliance.ID), Name: payload.Alliance.Name, Members: members, Holdings: holdings,
		ObservedAt: frame.ReceivedAt,
	}
	if gameState.Alliances == nil {
		gameState.Alliances = map[State.AllianceID]State.AllianceState{}
	}
	directoryChanged := !reflect.DeepEqual(gameState.Alliances[next.ID], next)
	gameState.Alliances[next.ID] = next
	allianceChanged := false
	if containsCurrentPlayer {
		allianceChanged = !reflect.DeepEqual(gameState.Alliance, next)
		if allianceChanged {
			gameState.Alliance = next
		}
	} else if gameState.Player.ID > 0 && next.ID > 0 && next.ID == gameState.Player.AllianceID {
		cleared := State.AllianceState{Members: []State.AllianceMember{}, Holdings: []State.AllianceHolding{}}
		allianceChanged = gameState.Player.AllianceID != 0 || !reflect.DeepEqual(gameState.Alliance, cleared)
		gameState.Player.AllianceID = 0
		gameState.Alliance = cleared
	}
	if !directoryChanged && !allianceChanged && !playerChanged {
		return nil, false, nil
	}
	return []string{"alliance", "alliances", "player"}, true, nil
}

type wirePlayerInfo struct {
	ID   wireInt64 `json:"PID"`
	Name string    `json:"PN"`
}

func decodePlayerInfo(raw json.RawMessage) (wirePlayerInfo, error) {
	var player wirePlayerInfo
	if err := json.Unmarshal(raw, &player); err != nil {
		return wirePlayerInfo{}, fmt.Errorf("decode player info: %w", err)
	}
	return player, nil
}

func applyPlayerInfo(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	player, err := decodePlayerInfo(raw)
	if err != nil {
		return false, err
	}
	return applyDecodedPlayerInfo(player, gameState), nil
}

func applyDecodedPlayerInfo(player wirePlayerInfo, gameState *State.GameState) bool {
	changed := false
	if player.ID > 0 && gameState.Player.ID != State.PlayerID(player.ID) {
		gameState.Player.ID = State.PlayerID(player.ID)
		changed = true
	}
	if player.Name != "" && gameState.Player.Name != player.Name {
		gameState.Player.Name = player.Name
		changed = true
	}
	return changed
}

func applyVIPInfo(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return false, fmt.Errorf("decode VIP state: %w", err)
	}
	next := State.VIPState{
		Points: rawInteger(values["VP"]), Level: int(rawInteger(values["VRL"])),
		RemainingSec: int(rawInteger(values["VRS"])), Upgrade: int(rawInteger(values["UPG"])),
	}
	if reflect.DeepEqual(gameState.Player.VIP, next) {
		return false, nil
	}
	gameState.Player.VIP = next
	return true, nil
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
