package Reports

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/State"
)

func ParseBattleCapture(capture State.BattleReportCapture, ownPlayerID State.PlayerID) (BattleReport, error) {
	var summary struct {
		MID            int64               `json:"MID"`
		LID            int64               `json:"LID"`
		TypeID         int                 `json:"MT"`
		AttackerHealth int                 `json:"AHP"`
		DefenderHealth int                 `json:"DHP"`
		Players        []battlePlayerWire  `json:"PI"`
		Participants   [][]json.RawMessage `json:"PBI"`
		Target         struct {
			Name       string `json:"N"`
			DefenderID int64  `json:"DP"`
			KingdomID  int    `json:"K"`
			X          int    `json:"X"`
			Y          int    `json:"Y"`
		} `json:"AI"`
	}
	if err := json.Unmarshal(capture.Summary, &summary); err != nil {
		return BattleReport{}, fmt.Errorf("decode battle summary %d: %w", capture.MessageID, err)
	}
	if summary.MID <= 0 {
		summary.MID = capture.MessageID
	}
	if summary.LID <= 0 {
		summary.LID = capture.ReportID
	}
	players := make(map[int64]battlePlayerWire, len(summary.Players))
	for _, player := range summary.Players {
		players[player.ID] = player
	}
	var attackerID int64
	var defenderID int64
	metrics := BattleMetrics{}
	for _, row := range summary.Participants {
		if len(row) < 4 {
			continue
		}
		playerID, playerOK := rawInteger(row[0])
		side, sideOK := rawInteger(row[1])
		started, startedOK := rawInteger(row[2])
		lost, lostOK := rawInteger(row[3])
		if !playerOK || !sideOK || !startedOK || !lostOK {
			continue
		}
		lost = absolute(lost)
		if side == 0 {
			attackerID = playerID
			metrics.AttackerSent = started
			metrics.AttackerLost = lost
		} else if side == 1 {
			defenderID = playerID
			metrics.DefenderStationed = started
			metrics.DefenderLost = lost
		}
	}
	if defenderID == 0 {
		defenderID = summary.Target.DefenderID
	}
	metrics.AttackersKilled = metrics.DefenderLost
	metrics.DefendersKilled = metrics.AttackerLost
	if metrics.AttackerLost > 0 {
		metrics.AttackTradeRatio = float64(metrics.DefenderLost) / float64(metrics.AttackerLost)
	}
	if metrics.DefenderLost > 0 {
		metrics.DefenseTradeRatio = float64(metrics.AttackerLost) / float64(metrics.DefenderLost)
	}
	attacker := combatant(attackerID, "attacker", players, summary.Target.Name)
	defender := combatant(defenderID, "defender", players, summary.Target.Name)
	result := "Unknown"
	if summary.AttackerHealth > 0 && summary.DefenderHealth <= 0 {
		result = "Victory"
	} else if summary.AttackerHealth <= 0 && summary.DefenderHealth > 0 {
		result = "Defeat"
	}
	role := "unknown"
	if int64(ownPlayerID) == attackerID {
		role = "attacker"
	} else if int64(ownPlayerID) == defenderID {
		role = "defender"
	}
	capturedAt := capture.CapturedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	id := fmt.Sprintf("%d-%d", summary.MID, summary.LID)
	report := BattleReport{
		ID: id, ReportID: id, BattleKey: capture.BattleKey, MID: summary.MID, LID: summary.LID,
		KingdomID: summary.Target.KingdomID, TargetX: summary.Target.X, TargetY: summary.Target.Y,
		TargetName: summary.Target.Name, CastleName: summary.Target.Name,
		BattleType: fmt.Sprintf("Type %d", summary.TypeID), OccurredAt: capturedAt.Format(time.RFC3339),
		DateMs: capturedAt.UnixMilli(), Result: result, Role: role,
		Attacker: &attacker, Defender: &defender, Metrics: metrics,
	}
	report.TopUnits = parseBattleUnits(capture.Details, attackerID, defenderID)
	return report, nil
}

type battlePlayerWire struct {
	ID       int64  `json:"OID"`
	Name     string `json:"N"`
	Alliance string `json:"AN"`
}

func combatant(id int64, role string, players map[int64]battlePlayerWire, targetName string) BattleCombatant {
	player := players[id]
	name := player.Name
	if name == "" {
		if role == "defender" && targetName != "" {
			name = targetName
		} else if id != 0 {
			name = fmt.Sprintf("Player %d", id)
		}
	}
	return BattleCombatant{
		PlayerID: id, Name: name, Alliance: player.Alliance, CastleName: targetName, Role: role,
	}
}

func parseBattleUnits(raw json.RawMessage, attackerID int64, defenderID int64) []BattleItemDetail {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Units [][]json.RawMessage `json:"Y"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return nil
	}
	items := make([]BattleItemDetail, 0)
	for _, row := range payload.Units {
		if len(row) < 2 {
			continue
		}
		playerID, ok := rawInteger(row[0])
		if !ok {
			continue
		}
		side := "unknown"
		if playerID == attackerID {
			side = "attacker"
		} else if playerID == defenderID {
			side = "defender"
		}
		for _, rawUnit := range row[1:] {
			var unit []json.RawMessage
			if json.Unmarshal(rawUnit, &unit) != nil {
				continue
			}
			if len(unit) < 2 {
				continue
			}
			unitID, unitOK := rawInteger(unit[0])
			amount, amountOK := rawInteger(unit[1])
			if !unitOK || !amountOK || unitID <= 0 || amount <= 0 {
				continue
			}
			lost := int64(0)
			if len(unit) > 2 {
				lost, _ = rawInteger(unit[2])
				lost = absolute(lost)
			}
			items = append(items, BattleItemDetail{
				Side: side, Phase: "total", WodID: unitID, Amount: amount, Lost: lost,
			})
		}
	}
	sort.Slice(items, func(left, right int) bool { return items[left].Amount > items[right].Amount })
	if len(items) > 40 {
		items = items[:40]
	}
	return items
}

func absolute(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func reportIDString(messageID int64, reportID int64) string {
	return strconv.FormatInt(messageID, 10) + "-" + strconv.FormatInt(reportID, 10)
}
