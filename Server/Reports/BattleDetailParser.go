package Reports

import (
	"encoding/json"
	"sort"
)

var battleLaneNames = [...]string{"Left flank", "Middle front", "Right flank"}

func parseBattleWaves(raw json.RawMessage, attackerID int64, defenderID int64) []BattleWave {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Waves []json.RawMessage `json:"W"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}

	waves := make([]BattleWave, 0, len(payload.Waves))
	for waveIndex, rawWave := range payload.Waves {
		var participants []json.RawMessage
		if json.Unmarshal(rawWave, &participants) != nil {
			continue
		}
		lanes := [len(battleLaneNames)]BattleWaveLane{}
		for laneIndex, laneName := range battleLaneNames {
			lanes[laneIndex].Lane = laneName
		}

		for _, rawParticipant := range participants {
			var participant []json.RawMessage
			if json.Unmarshal(rawParticipant, &participant) != nil || len(participant) < 2 {
				continue
			}
			playerID, ok := rawInteger(participant[0])
			if !ok {
				continue
			}
			side := battleSide(playerID, attackerID, defenderID)
			if side == "" {
				continue
			}

			for laneIndex := 0; laneIndex < len(battleLaneNames) && laneIndex+1 < len(participant); laneIndex++ {
				var lane []json.RawMessage
				if json.Unmarshal(participant[laneIndex+1], &lane) != nil || len(lane) < 2 {
					continue
				}
				units := parseBattleItemRows(lane[0], side, "wall", battleLaneNames[laneIndex], false)
				tools := parseBattleItemRows(lane[1], side, "wall", battleLaneNames[laneIndex], true)
				resolved := &lanes[laneIndex]
				if side == "attacker" {
					resolved.AttackerUnitDetails = append(resolved.AttackerUnitDetails, units...)
					resolved.AttackerToolDetails = append(resolved.AttackerToolDetails, tools...)
				} else {
					resolved.DefenderUnitDetails = append(resolved.DefenderUnitDetails, units...)
					resolved.DefenderToolDetails = append(resolved.DefenderToolDetails, tools...)
				}
			}
		}

		resolvedLanes := make([]BattleWaveLane, 0, len(lanes))
		for laneIndex := range lanes {
			lane := lanes[laneIndex]
			lane.AttackerStart, lane.AttackerLost = battleUnitTotals(lane.AttackerUnitDetails)
			lane.DefenderStart, lane.DefenderLost = battleUnitTotals(lane.DefenderUnitDetails)
			if lane.AttackerStart == 0 && lane.DefenderStart == 0 &&
				len(lane.AttackerToolDetails) == 0 && len(lane.DefenderToolDetails) == 0 {
				continue
			}
			lane.Result = resolvedBattleLaneResult(lane)
			resolvedLanes = append(resolvedLanes, lane)
		}
		if len(resolvedLanes) == 0 {
			continue
		}
		waves = append(waves, BattleWave{
			Index: waveIndex + 1,
			Wave:  waveIndex + 1,
			Lanes: resolvedLanes,
		})
	}
	return waves
}

func parseBattleTools(raw json.RawMessage, waves []BattleWave, attackerID int64, defenderID int64) []BattleItemDetail {
	items := make([]BattleItemDetail, 0)
	if len(raw) > 0 {
		var payload struct {
			Support [][]json.RawMessage `json:"S"`
		}
		if json.Unmarshal(raw, &payload) == nil {
			for _, row := range payload.Support {
				if len(row) < 2 {
					continue
				}
				playerID, ok := rawInteger(row[0])
				if !ok {
					continue
				}
				side := battleSide(playerID, attackerID, defenderID)
				if side == "" {
					continue
				}
				for _, rawTool := range row[1:] {
					if item, parsed := parseBattleItemRow(rawTool, side, "support", "", true); parsed {
						items = append(items, item)
					}
				}
			}
		}
	}
	for _, wave := range waves {
		for _, lane := range wave.Lanes {
			items = append(items, lane.AttackerToolDetails...)
			items = append(items, lane.DefenderToolDetails...)
		}
	}
	return aggregateBattleTools(items)
}

func parseBattleEffects(raw json.RawMessage) ([]BattleEffect, []BattleEffect) {
	if len(raw) == 0 {
		return nil, nil
	}
	var payload struct {
		Attacker json.RawMessage `json:"AL"`
		Defender json.RawMessage `json:"DB"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil, nil
	}
	return parseBattleLeaderEffects(payload.Attacker, "commander"), parseBattleLeaderEffects(payload.Defender, "castellan")
}

func parseBattleLeaderEffects(raw json.RawMessage, side string) []BattleEffect {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var leader struct {
		Effects []json.RawMessage `json:"AE"`
	}
	if json.Unmarshal(raw, &leader) != nil {
		return nil
	}
	effects := make([]BattleEffect, 0, len(leader.Effects))
	for _, rawEffect := range leader.Effects {
		var row []json.RawMessage
		if json.Unmarshal(rawEffect, &row) != nil || len(row) < 2 {
			continue
		}
		definitionID, ok := rawInteger(row[0])
		if !ok || definitionID <= 0 {
			continue
		}
		var values []float64
		if json.Unmarshal(row[1], &values) != nil || len(values) == 0 {
			continue
		}
		source := ""
		if len(row) > 2 {
			_ = json.Unmarshal(row[2], &source)
		}
		effects = append(effects, BattleEffect{
			DefinitionID: definitionID,
			Values:       values,
			Source:       source,
			Side:         side,
		})
	}
	return effects
}

func parseBattleItemRows(
	raw json.RawMessage,
	side string,
	phase string,
	lane string,
	tools bool,
) []BattleItemDetail {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return nil
	}
	items := make([]BattleItemDetail, 0, len(rows))
	for _, rawItem := range rows {
		if item, parsed := parseBattleItemRow(rawItem, side, phase, lane, tools); parsed {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(left, right int) bool {
		if items[left].Amount != items[right].Amount {
			return items[left].Amount > items[right].Amount
		}
		return items[left].WodID < items[right].WodID
	})
	return items
}

func parseBattleItemRow(
	raw json.RawMessage,
	side string,
	phase string,
	lane string,
	tools bool,
) (BattleItemDetail, bool) {
	var row []json.RawMessage
	if json.Unmarshal(raw, &row) != nil || len(row) < 2 {
		return BattleItemDetail{}, false
	}
	itemID, idOK := rawInteger(row[0])
	amount, amountOK := rawInteger(row[1])
	if !idOK || !amountOK || itemID <= 0 {
		return BattleItemDetail{}, false
	}
	amount = absolute(amount)
	changed := int64(0)
	if len(row) > 2 {
		changed, _ = rawInteger(row[2])
		changed = absolute(changed)
	}
	if amount == 0 && changed == 0 {
		return BattleItemDetail{}, false
	}
	item := BattleItemDetail{
		Side: side, Phase: phase, Lane: lane, WodID: itemID, Amount: amount,
	}
	if tools {
		item.Used = changed
	} else {
		item.Lost = changed
	}
	return item, true
}

func aggregateBattleTools(items []BattleItemDetail) []BattleItemDetail {
	type toolKey struct {
		side string
		id   int64
	}
	aggregated := make(map[toolKey]BattleItemDetail)
	for _, item := range items {
		if item.WodID <= 0 || item.Side == "" {
			continue
		}
		key := toolKey{side: item.Side, id: item.WodID}
		current := aggregated[key]
		current.Side = item.Side
		current.Phase = "total"
		current.WodID = item.WodID
		current.Amount += item.Amount
		current.Used += item.Used
		aggregated[key] = current
	}
	result := make([]BattleItemDetail, 0, len(aggregated))
	for _, item := range aggregated {
		result = append(result, item)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Used != result[right].Used {
			return result[left].Used > result[right].Used
		}
		if result[left].Amount != result[right].Amount {
			return result[left].Amount > result[right].Amount
		}
		if result[left].Side != result[right].Side {
			return result[left].Side < result[right].Side
		}
		return result[left].WodID < result[right].WodID
	})
	return result
}

func battleSide(playerID int64, attackerID int64, defenderID int64) string {
	if playerID == attackerID {
		return "attacker"
	}
	if playerID == defenderID {
		return "defender"
	}
	return ""
}

func battleUnitTotals(items []BattleItemDetail) (int64, int64) {
	var started int64
	var lost int64
	for _, item := range items {
		started += item.Amount
		lost += item.Lost
	}
	return started, lost
}

func resolvedBattleLaneResult(lane BattleWaveLane) string {
	if lane.AttackerStart > 0 && lane.AttackerLost >= lane.AttackerStart {
		return "HELD"
	}
	if lane.DefenderStart > 0 && lane.DefenderLost < lane.DefenderStart {
		return "HELD"
	}
	if lane.DefenderStart > 0 && lane.DefenderLost >= lane.DefenderStart {
		return "BREACHED"
	}
	if lane.AttackerStart > 0 && lane.AttackerLost < lane.AttackerStart {
		return "BREACHED"
	}
	return "HELD"
}
