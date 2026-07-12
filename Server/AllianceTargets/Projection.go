package AllianceTargets

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/State"
)

func buildTrackerTargets(gameState State.GameState, detail trackerAllianceDetail, cartography []trackerCartographyPlayer) []Target {
	castlesByName := make(map[string][]Castle, len(cartography))
	for _, player := range cartography {
		if castles := attackableTargetCastles(player.Castles); len(castles) > 0 {
			castlesByName[strings.ToLower(strings.TrimSpace(player.Name))] = castles
		}
	}
	now := time.Now().UTC()
	rows := make([]Target, 0, len(detail.Players)*4)
	for _, player := range detail.Players {
		castles := castlesByName[strings.ToLower(strings.TrimSpace(player.Name))]
		birdUntil, _ := time.Parse(time.RFC3339Nano, player.BirdUntil)
		rpt := 0
		if birdUntil.After(now) {
			rpt = int(birdUntil.Sub(now).Seconds())
		}
		for _, targetCastle := range castles {
			closest, distance, found := closestOwnedCastle(gameState, targetCastle.X, targetCastle.Y)
			if !found {
				continue
			}
			rows = append(rows, Target{
				PlayerID: trackerGameID(player.PlayerID), Name: player.Name, Might: int64(player.Might),
				UnderBird: rpt > 0, RPTSeconds: rpt, BirdUntil: player.BirdUntil, UpdatedAt: player.UpdatedAt,
				TargetCastle: targetCastle, ClosestOwnCastle: closest, Distance: roundDistance(distance),
			})
		}
	}
	sortTargets(rows)
	return rows
}

func buildLiveTargets(gameState State.GameState, alliance State.AllianceState, detail trackerAllianceDetail) []Target {
	metadata := make(map[int64]struct {
		might     int64
		updatedAt string
	}, len(detail.Players))
	for _, player := range detail.Players {
		metadata[trackerGameID(player.PlayerID)] = struct {
			might     int64
			updatedAt string
		}{might: int64(player.Might), updatedAt: player.UpdatedAt}
	}
	holdings := make(map[State.PlayerID][]State.AllianceHolding, len(alliance.Members))
	for _, holding := range alliance.Holdings {
		if holding.KingdomID == 0 && attackableCastleType(holding.SlotType) {
			holdings[holding.PlayerID] = append(holdings[holding.PlayerID], holding)
		}
	}
	elapsed := 0
	if !alliance.ObservedAt.IsZero() {
		elapsed = max(0, int(time.Since(alliance.ObservedAt).Seconds()))
	}
	rows := make([]Target, 0, len(alliance.Holdings))
	for _, member := range alliance.Members {
		playerMetadata := metadata[int64(member.PlayerID)]
		might := int64(member.Might)
		if playerMetadata.might > 0 {
			might = playerMetadata.might
		}
		rpt := max(0, member.ReturnProtectionSec-elapsed)
		birdUntil := ""
		if rpt > 0 {
			birdUntil = time.Now().UTC().Add(time.Duration(rpt) * time.Second).Format(time.RFC3339)
		}
		for _, holding := range holdings[member.PlayerID] {
			targetCastle := Castle{
				CastleID: int64(holding.CastleID), TypeName: castleTypeName(holding.SlotType),
				X: holding.X, Y: holding.Y, TypeID: holding.SlotType,
			}
			closest, distance, found := closestOwnedCastle(gameState, holding.X, holding.Y)
			if !found {
				continue
			}
			rows = append(rows, Target{
				PlayerID: int64(member.PlayerID), Name: member.Name, Might: might,
				UnderBird: rpt > 0, RPTSeconds: rpt, BirdUntil: birdUntil, UpdatedAt: playerMetadata.updatedAt,
				TargetCastle: targetCastle, ClosestOwnCastle: closest, Distance: roundDistance(distance),
			})
		}
	}
	sortTargets(rows)
	return rows
}

func spyAvailability(gameState State.GameState, gameData *GameData.Store) SpyAvailability {
	result := SpyAvailability{Taverns: []Tavern{}}
	var source State.CastleState
	for _, castle := range gameState.Castles {
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			source = castle
			break
		}
	}
	if source.ID <= 0 {
		return result
	}
	result.SourceCastle = Castle{CastleID: int64(source.ID), Name: source.Name, X: source.X, Y: source.Y, TypeID: source.SlotType}
	result.BuildingRowsLoaded = len(source.Buildings) > 0
	if gameData != nil {
		if buildings, err := gameData.Catalog("buildings"); err == nil {
			for _, building := range source.Buildings {
				raw, found := buildings.Find(strconv.FormatInt(int64(building.DefinitionID), 10))
				if !found {
					continue
				}
				record, decodeErr := GameData.DecodeRecord(raw)
				if decodeErr != nil {
					continue
				}
				capacity, capacityOK := record.Int64("spySize")
				if !capacityOK || capacity <= 0 {
					continue
				}
				level, _ := record.Int64("level")
				result.Capacity += int(capacity)
				result.Taverns = append(result.Taverns, Tavern{Level: int(level), Capacity: int(capacity)})
			}
		}
	}
	sort.Slice(result.Taverns, func(left, right int) bool {
		if result.Taverns[left].Level != result.Taverns[right].Level {
			return result.Taverns[left].Level < result.Taverns[right].Level
		}
		return result.Taverns[left].Capacity < result.Taverns[right].Capacity
	})
	for _, movement := range gameState.Movements {
		if movement.TypeID == 3 && movement.SourceCastleID == source.ID {
			result.Active += movement.SpyCount
		}
	}
	result.Available = max(0, result.Capacity-result.Active)
	return result
}

func attackableTargetCastles(rows [][]int) []Castle {
	castles := make([]Castle, 0, 4)
	for _, row := range rows {
		if len(row) < 3 || !attackableCastleType(row[2]) {
			continue
		}
		castles = append(castles, Castle{TypeName: castleTypeName(row[2]), X: row[0], Y: row[1], TypeID: row[2]})
	}
	return castles
}

func attackableCastleType(typeID int) bool {
	switch typeID {
	case 1, 3, 4, 5, 6, 22:
		return true
	default:
		return false
	}
}

func castleTypeName(typeID int) string {
	switch typeID {
	case 1:
		return "Main castle"
	case 3, 6:
		return "Capital"
	case 4:
		return "Outpost"
	case 5, 22:
		return "Metropolis"
	default:
		return "Player castle"
	}
}

func closestOwnedCastle(gameState State.GameState, x int, y int) (Castle, float64, bool) {
	best := Castle{}
	bestDistance := math.MaxFloat64
	for _, castle := range gameState.Castles {
		if castle.ID <= 0 || castle.KingdomID != 0 || castle.X == 0 && castle.Y == 0 {
			continue
		}
		distance := math.Hypot(float64(x-castle.X), float64(y-castle.Y))
		if distance < bestDistance {
			best = Castle{CastleID: int64(castle.ID), Name: castle.Name, TypeName: castleTypeName(castle.SlotType), X: castle.X, Y: castle.Y, TypeID: castle.SlotType}
			bestDistance = distance
		}
	}
	return best, bestDistance, bestDistance != math.MaxFloat64
}

func enrichTargetNames(gameState State.GameState, targets []Target) {
	byID := map[int64]string{}
	byCoordinate := map[string]string{}
	for _, kingdom := range gameState.Map {
		for _, observation := range kingdom {
			name := strings.TrimSpace(observation.Name)
			if name == "" {
				continue
			}
			byCoordinate[coordinateKey(observation.X, observation.Y)] = name
			if observation.ObjectID > 0 {
				byID[observation.ObjectID] = name
			}
		}
	}
	for _, capture := range gameState.Reports.SpyCaptures {
		report, err := Reports.ParseSpyCapture(capture)
		if err != nil || strings.TrimSpace(report.Castle.Name) == "" {
			continue
		}
		byCoordinate[coordinateKey(report.Castle.X, report.Castle.Y)] = report.Castle.Name
		if report.Castle.ID > 0 {
			byID[report.Castle.ID] = report.Castle.Name
		}
	}
	for index := range targets {
		castle := &targets[index].TargetCastle
		if castle.TypeName == "" {
			castle.TypeName = castleTypeName(castle.TypeID)
		}
		if castle.CastleID > 0 {
			castle.Name = byID[castle.CastleID]
		}
		if castle.Name == "" {
			castle.Name = byCoordinate[coordinateKey(castle.X, castle.Y)]
		}
	}
}

func sortTargets(rows []Target) {
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].Distance != rows[right].Distance {
			return rows[left].Distance < rows[right].Distance
		}
		if !strings.EqualFold(rows[left].Name, rows[right].Name) {
			return strings.ToLower(rows[left].Name) < strings.ToLower(rows[right].Name)
		}
		return rows[left].TargetCastle.TypeID < rows[right].TargetCastle.TypeID
	})
}

func coordinateKey(x int, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}

func roundDistance(value float64) float64 {
	return math.Round(value*10) / 10
}
