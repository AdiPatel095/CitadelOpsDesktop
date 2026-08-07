package WorldIntel

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
)

const (
	leaderboardIntentName      = "world-intelligence.leaderboard.page"
	leaderboardPageSize        = int64(10)
	leaderboardInitialRank     = leaderboardPageSize / 2
	leaderboardPageDelay       = 15 * time.Millisecond
	maximumLeaderboardEntries  = int64(50_000)
	maximumLeaderboardPageSkew = int64(2)
)

type intentSubmitter interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}

type leaderboardPageRequest struct {
	ListType      int64 `json:"listType"`
	LevelCategory int64 `json:"levelCategory"`
	SearchValue   int64 `json:"searchValue"`
}

type leaderboardPage struct {
	ListType      int64
	LevelCategory int64
	Total         int64
	Entries       []leaderboardEntry
}

type leaderboardEntry struct {
	Rank        int64
	Points      float64
	Player      PlayerObservation
	HoldingRows [][]json.RawMessage
}

type leaderboardScan struct {
	Players  map[int64]PlayerObservation
	Holdings map[string]HoldingObservation
}

func (service *DesktopService) scanPublicLeaderboards(
	ctx context.Context,
	worldID string,
	collectorPlayerID int64,
	capturedAt time.Time,
) ([]ObservationBatch, int, error) {
	if service == nil || service.intents == nil {
		return nil, 0, fmt.Errorf("World Intelligence leaderboard intent service is unavailable")
	}
	scan := leaderboardScan{
		Players:  map[int64]PlayerObservation{},
		Holdings: map[string]HoldingObservation{},
	}
	for levelCategory := int64(1); levelCategory <= 6; levelCategory++ {
		if err := service.scanLeaderboardCategory(ctx, &scan, worldID, collectorPlayerID, 6, levelCategory, capturedAt); err != nil {
			return nil, len(scan.Players), err
		}
	}
	if err := service.scanLeaderboardCategory(ctx, &scan, worldID, collectorPlayerID, 2, 1, capturedAt); err != nil {
		return nil, len(scan.Players), err
	}
	if len(scan.Players) == 0 {
		return nil, 0, fmt.Errorf("GGE public leaderboards returned no players")
	}
	batches, err := buildLeaderboardBatches(worldID, capturedAt, scan)
	return batches, len(scan.Players), err
}

func (service *DesktopService) scanLeaderboardCategory(
	ctx context.Context,
	scan *leaderboardScan,
	worldID string,
	collectorPlayerID int64,
	listType int64,
	levelCategory int64,
	capturedAt time.Time,
) error {
	seenRanks := map[int64]struct{}{}
	searchValue := leaderboardInitialRank
	maximumPages := maximumLeaderboardEntries/leaderboardPageSize + maximumLeaderboardPageSkew
	for pageNumber := int64(0); pageNumber < maximumPages; pageNumber++ {
		if err := service.requireCollectorSession(worldID, collectorPlayerID); err != nil {
			return err
		}
		page, err := service.requestLeaderboardPage(ctx, leaderboardPageRequest{
			ListType: listType, LevelCategory: levelCategory, SearchValue: searchValue,
		})
		if err != nil {
			return fmt.Errorf("read GGE leaderboard %d category %d around rank %d: %w", listType, levelCategory, searchValue, err)
		}
		if page.Total < 0 || page.Total > maximumLeaderboardEntries {
			return fmt.Errorf("GGE leaderboard %d category %d reported an unsupported total of %d", listType, levelCategory, page.Total)
		}
		if page.Total == 0 {
			return nil
		}
		if len(page.Entries) == 0 {
			return fmt.Errorf("GGE leaderboard %d category %d returned no rows for %d reported players", listType, levelCategory, page.Total)
		}
		lastRank := int64(0)
		for _, entry := range page.Entries {
			if entry.Rank <= 0 || entry.Rank > page.Total {
				return fmt.Errorf("GGE leaderboard returned rank %d outside 1..%d", entry.Rank, page.Total)
			}
			seenRanks[entry.Rank] = struct{}{}
			lastRank = max(lastRank, entry.Rank)
			mergeLeaderboardEntry(scan, worldID, capturedAt, listType, entry)
		}
		service.setScanProgress(len(scan.Players))
		if int64(len(seenRanks)) >= page.Total || lastRank >= page.Total {
			return nil
		}
		searchValue += leaderboardPageSize
		if searchValue > page.Total+leaderboardPageSize {
			return fmt.Errorf("GGE leaderboard pagination stopped before rank %d of %d", lastRank, page.Total)
		}
		timer := time.NewTimer(leaderboardPageDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("GGE leaderboard pagination exceeded the bounded page limit")
}

func (service *DesktopService) requestLeaderboardPage(ctx context.Context, request leaderboardPageRequest) (leaderboardPage, error) {
	arguments, _ := json.Marshal(request)
	receipt := service.intents.Submit(ctx, Intent.Request{
		Name: leaderboardIntentName, Actor: "world-intelligence", Priority: Outbound.PriorityBackground,
		Arguments: arguments,
	})
	if receipt.Status != Intent.StatusSucceeded {
		message := strings.TrimSpace(receipt.DiagnosticError())
		if message == "" {
			message = fmt.Sprintf("leaderboard intent ended with status %s", receipt.Status)
		}
		return leaderboardPage{}, fmt.Errorf("%s", message)
	}
	for _, exchange := range receipt.Exchanges {
		if exchange.Response == nil || exchange.Response.Opcode != "hgh" {
			continue
		}
		page, err := decodeLeaderboardPage(exchange.Response.Payload)
		if err != nil {
			return leaderboardPage{}, err
		}
		if page.ListType != request.ListType || page.LevelCategory != request.LevelCategory {
			return leaderboardPage{}, fmt.Errorf(
				"GGE returned leaderboard %d category %d for requested leaderboard %d category %d",
				page.ListType, page.LevelCategory, request.ListType, request.LevelCategory,
			)
		}
		return page, nil
	}
	return leaderboardPage{}, fmt.Errorf("leaderboard response was not captured")
}

func decodeLeaderboardPage(payload json.RawMessage) (leaderboardPage, error) {
	var wire struct {
		Entries       []json.RawMessage `json:"L"`
		ListType      json.RawMessage   `json:"LT"`
		LevelCategory json.RawMessage   `json:"LID"`
		Total         json.RawMessage   `json:"LR"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		return leaderboardPage{}, fmt.Errorf("decode GGE leaderboard page: %w", err)
	}
	listType, err := decodeWireInt(wire.ListType)
	if err != nil {
		return leaderboardPage{}, fmt.Errorf("decode leaderboard list type: %w", err)
	}
	levelCategory, err := decodeWireInt(wire.LevelCategory)
	if err != nil {
		return leaderboardPage{}, fmt.Errorf("decode leaderboard level category: %w", err)
	}
	total, err := decodeWireInt(wire.Total)
	if err != nil {
		return leaderboardPage{}, fmt.Errorf("decode leaderboard total: %w", err)
	}
	page := leaderboardPage{ListType: listType, LevelCategory: levelCategory, Total: total, Entries: make([]leaderboardEntry, 0, len(wire.Entries))}
	for index, raw := range wire.Entries {
		entry, err := decodeLeaderboardEntry(raw)
		if err != nil {
			return leaderboardPage{}, fmt.Errorf("decode leaderboard row %d: %w", index, err)
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
}

func decodeLeaderboardEntry(raw json.RawMessage) (leaderboardEntry, error) {
	var columns []json.RawMessage
	if err := json.Unmarshal(raw, &columns); err != nil || len(columns) < 3 {
		return leaderboardEntry{}, fmt.Errorf("expected rank, points, and player details")
	}
	rank, err := decodeWireInt(columns[0])
	if err != nil {
		return leaderboardEntry{}, fmt.Errorf("rank: %w", err)
	}
	points, err := decodeWireFloat(columns[1])
	if err != nil {
		return leaderboardEntry{}, fmt.Errorf("points: %w", err)
	}
	var details struct {
		PlayerID     json.RawMessage     `json:"OID"`
		Name         string              `json:"N"`
		AllianceID   json.RawMessage     `json:"AID"`
		AllianceName string              `json:"AN"`
		Level        json.RawMessage     `json:"L"`
		LegendLevel  json.RawMessage     `json:"LL"`
		Might        json.RawMessage     `json:"MP"`
		Glory        json.RawMessage     `json:"CF"`
		Honor        json.RawMessage     `json:"H"`
		Holdings     [][]json.RawMessage `json:"AP"`
	}
	if err := json.Unmarshal(columns[2], &details); err != nil {
		return leaderboardEntry{}, fmt.Errorf("player details: %w", err)
	}
	playerID, err := decodeWireInt(details.PlayerID)
	if err != nil || playerID <= 0 || strings.TrimSpace(details.Name) == "" {
		return leaderboardEntry{}, fmt.Errorf("player identity is invalid")
	}
	allianceID, _ := decodeOptionalWireInt(details.AllianceID)
	level, _ := decodeOptionalWireInt(details.Level)
	legendLevel, _ := decodeOptionalWireInt(details.LegendLevel)
	might, _ := decodeOptionalWireFloat(details.Might)
	glory, _ := decodeOptionalWireFloat(details.Glory)
	honor, _ := decodeOptionalWireFloat(details.Honor)
	return leaderboardEntry{
		Rank: rank, Points: points, HoldingRows: details.Holdings,
		Player: PlayerObservation{
			PlayerID: playerID, Name: details.Name, AllianceID: allianceID, AllianceName: details.AllianceName,
			Level: int(max(level, 0)), LegendLevel: int(max(legendLevel, 0)), Might: max(might, 0),
			Glory: max(glory, 0), Honor: max(honor, 0), Source: "leaderboard",
		},
	}, nil
}

func mergeLeaderboardEntry(scan *leaderboardScan, worldID string, observedAt time.Time, listType int64, entry leaderboardEntry) {
	incoming := entry.Player
	incoming.WorldID = worldID
	incoming.ObservedAt = observedAt
	if listType == 2 {
		incoming.WeeklyLoot = entry.Points
		if incoming.WeeklyLoot < 0 {
			incoming.WeeklyLoot += math.Pow(2, 32)
		}
	}
	current, found := scan.Players[incoming.PlayerID]
	if found {
		if incoming.Name != "" {
			current.Name = incoming.Name
		}
		if incoming.AllianceID > 0 || current.AllianceID == 0 {
			current.AllianceID = incoming.AllianceID
			current.AllianceName = incoming.AllianceName
		}
		if incoming.Level > 0 {
			current.Level = incoming.Level
		}
		if incoming.LegendLevel > 0 {
			current.LegendLevel = incoming.LegendLevel
		}
		if incoming.Might > 0 {
			current.Might = incoming.Might
		}
		if incoming.Glory > 0 {
			current.Glory = incoming.Glory
		}
		if incoming.Honor > 0 {
			current.Honor = incoming.Honor
		}
		if listType == 2 {
			current.WeeklyLoot = max(incoming.WeeklyLoot, 0)
		}
		incoming = current
	}
	scan.Players[incoming.PlayerID] = incoming
	if incoming.AllianceID <= 0 {
		return
	}
	for _, row := range entry.HoldingRows {
		if len(row) < 5 {
			continue
		}
		kingdomID, kingdomErr := decodeWireInt(row[0])
		castleID, castleErr := decodeWireInt(row[1])
		x, xErr := decodeWireInt(row[2])
		y, yErr := decodeWireInt(row[3])
		slotType, slotErr := decodeWireInt(row[4])
		if kingdomErr != nil || castleErr != nil || xErr != nil || yErr != nil || slotErr != nil ||
			kingdomID < 0 || castleID <= 0 || x < 0 || y < 0 || slotType < 0 {
			continue
		}
		key := strconv.FormatInt(castleID, 10)
		scan.Holdings[key] = HoldingObservation{
			WorldID: worldID, AllianceID: incoming.AllianceID, PlayerID: incoming.PlayerID,
			CastleID: castleID, KingdomID: kingdomID, X: int(x), Y: int(y), SlotType: int(slotType),
			ObservedAt: observedAt,
		}
	}
}

func buildLeaderboardBatches(worldID string, capturedAt time.Time, scan leaderboardScan) ([]ObservationBatch, error) {
	players := make([]PlayerObservation, 0, len(scan.Players))
	for _, player := range scan.Players {
		players = append(players, player)
	}
	sort.Slice(players, func(left, right int) bool { return players[left].PlayerID < players[right].PlayerID })

	type allianceAggregate struct {
		observation AllianceObservation
		members     map[int64]struct{}
	}
	aggregates := map[int64]*allianceAggregate{}
	for _, player := range players {
		if player.AllianceID <= 0 || strings.TrimSpace(player.AllianceName) == "" {
			continue
		}
		aggregate := aggregates[player.AllianceID]
		if aggregate == nil {
			aggregate = &allianceAggregate{
				observation: AllianceObservation{
					WorldID: worldID, AllianceID: player.AllianceID, Name: player.AllianceName,
					Source: "leaderboard", ObservedAt: capturedAt,
				},
				members: map[int64]struct{}{},
			}
			aggregates[player.AllianceID] = aggregate
		}
		aggregate.observation.Name = player.AllianceName
		aggregate.observation.TotalMight += player.Might
		aggregate.members[player.PlayerID] = struct{}{}
	}
	alliances := make([]AllianceObservation, 0, len(aggregates))
	for _, aggregate := range aggregates {
		aggregate.observation.MemberCount = len(aggregate.members)
		alliances = append(alliances, aggregate.observation)
	}
	sort.Slice(alliances, func(left, right int) bool { return alliances[left].AllianceID < alliances[right].AllianceID })

	holdings := make([]HoldingObservation, 0, len(scan.Holdings))
	for _, holding := range scan.Holdings {
		holdings = append(holdings, holding)
	}
	sort.Slice(holdings, func(left, right int) bool {
		if holdings[left].PlayerID != holdings[right].PlayerID {
			return holdings[left].PlayerID < holdings[right].PlayerID
		}
		if holdings[left].KingdomID != holdings[right].KingdomID {
			return holdings[left].KingdomID < holdings[right].KingdomID
		}
		return holdings[left].CastleID < holdings[right].CastleID
	})

	holdingChunks := make([][]HoldingObservation, 0, (len(holdings)+MaximumHoldings-1)/MaximumHoldings)
	for start := 0; start < len(holdings); {
		end := min(start+MaximumHoldings, len(holdings))
		if end < len(holdings) {
			for end > start && holdings[end-1].PlayerID == holdings[end].PlayerID {
				end--
			}
			if end == start {
				return nil, fmt.Errorf("player %d has more than %d public holdings", holdings[start].PlayerID, MaximumHoldings)
			}
		}
		holdingChunks = append(holdingChunks, holdings[start:end])
		start = end
	}
	batchCount := max(
		(len(players)+MaximumPlayers-1)/MaximumPlayers,
		(len(alliances)+MaximumAlliances-1)/MaximumAlliances,
		len(holdingChunks),
	)
	batches := make([]ObservationBatch, 0, batchCount)
	appendBatch := func(batch ObservationBatch) error {
		batch.WorldID = worldID
		batch.CapturedAt = capturedAt
		finalized, err := FinalizeBatch(batch)
		if err != nil {
			return err
		}
		batches = append(batches, finalized)
		return nil
	}
	for index := 0; index < batchCount; index++ {
		batch := ObservationBatch{}
		if start := index * MaximumPlayers; start < len(players) {
			batch.Players = append([]PlayerObservation(nil), players[start:min(start+MaximumPlayers, len(players))]...)
		}
		if start := index * MaximumAlliances; start < len(alliances) {
			batch.Alliances = append([]AllianceObservation(nil), alliances[start:min(start+MaximumAlliances, len(alliances))]...)
		}
		if index < len(holdingChunks) {
			batch.Holdings = append([]HoldingObservation(nil), holdingChunks[index]...)
		}
		if err := appendBatch(batch); err != nil {
			return nil, err
		}
	}
	return batches, nil
}

func decodeOptionalWireInt(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return 0, nil
	}
	return decodeWireInt(raw)
}

func decodeWireInt(raw json.RawMessage) (int64, error) {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" || value == "null" {
		return 0, fmt.Errorf("value is missing")
	}
	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		return integer, nil
	}
	floating, err := strconv.ParseFloat(value, 64)
	if err != nil || math.Trunc(floating) != floating || floating > math.MaxInt64 || floating < math.MinInt64 {
		return 0, fmt.Errorf("%q is not an integer", value)
	}
	return int64(floating), nil
}

func decodeOptionalWireFloat(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == `""` {
		return 0, nil
	}
	return decodeWireFloat(raw)
}

func decodeWireFloat(raw json.RawMessage) (float64, error) {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("%q is not a finite number", value)
	}
	return number, nil
}
