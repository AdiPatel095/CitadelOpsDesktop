package WorldIntel

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeLeaderboardPageCapturesPublicPlayerAndHoldingFields(t *testing.T) {
	payload := json.RawMessage(`{
		"LT":"6","LID":"3","LR":"1",
		"L":[[1,"98765",{"OID":"42","N":"Public Player","AID":"9","AN":"Public Alliance","L":"70","LL":"950","MP":"98765","CF":"321","H":"456","AP":[[0,9001,12,34,1],[2,9002,45,67,12]]}]]
	}`)
	page, err := decodeLeaderboardPage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if page.ListType != 6 || page.LevelCategory != 3 || page.Total != 1 || len(page.Entries) != 1 {
		t.Fatalf("unexpected page: %#v", page)
	}
	entry := page.Entries[0]
	if entry.Rank != 1 || entry.Player.PlayerID != 42 || entry.Player.Might != 98765 ||
		entry.Player.Glory != 321 || entry.Player.Honor != 456 || len(entry.HoldingRows) != 2 {
		t.Fatalf("unexpected entry: %#v", entry)
	}
}

func TestLeaderboardScanMergesLootAndBuildsBoundedBatches(t *testing.T) {
	now := time.Date(2026, time.August, 6, 20, 15, 0, 0, time.UTC)
	scan := leaderboardScan{Players: map[int64]PlayerObservation{}, Holdings: map[string]HoldingObservation{}}
	might := leaderboardEntry{
		Rank: 1,
		Player: PlayerObservation{
			PlayerID: 42, Name: "Public Player", AllianceID: 9, AllianceName: "Public Alliance",
			Level: 70, LegendLevel: 950, Might: 98765, Glory: 321, Honor: 456, Source: "leaderboard",
		},
		HoldingRows: [][]json.RawMessage{{json.RawMessage(`0`), json.RawMessage(`9001`), json.RawMessage(`12`), json.RawMessage(`34`), json.RawMessage(`1`)}},
	}
	mergeLeaderboardEntry(&scan, "world.example", now, 6, might)
	loot := might
	loot.Points = -1
	loot.HoldingRows = nil
	mergeLeaderboardEntry(&scan, "world.example", now, 2, loot)

	batches, err := buildLeaderboardBatches("world.example", now, scan)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("batch count = %d, want one packed batch", len(batches))
	}
	player := batches[0].Players[0]
	if player.WeeklyLoot != 4_294_967_295 || player.Might != 98765 || player.Honor != 456 {
		t.Fatalf("merged player = %#v", player)
	}
	if batches[0].Alliances[0].MemberCount != 1 || batches[0].Alliances[0].TotalMight != 98765 {
		t.Fatalf("alliance aggregate = %#v", batches[0].Alliances[0])
	}
	if batches[0].Holdings[0].CastleID != 9001 {
		t.Fatalf("holding = %#v", batches[0].Holdings[0])
	}
	for _, batch := range batches {
		if err := ValidateFinalizedBatch(batch); err != nil {
			t.Fatalf("invalid batch: %v", err)
		}
	}
}

func TestCollectorBucketsAlternateEveryFifteenMinutes(t *testing.T) {
	first := time.Date(2026, time.August, 6, 20, 0, 0, 0, time.UTC)
	_, firstSlot := collectorBucket(first, 0, 2)
	_, secondSlot := collectorBucket(first, 1, 2)
	_, firstSlotNext := collectorBucket(first.Add(15*time.Minute), 0, 2)
	_, secondSlotNext := collectorBucket(first.Add(15*time.Minute), 1, 2)
	if firstSlot == secondSlot || firstSlotNext == secondSlotNext || firstSlot == firstSlotNext {
		t.Fatalf("slots did not alternate: %t %t then %t %t", firstSlot, secondSlot, firstSlotNext, secondSlotNext)
	}
	if next := nextCollectorSlot(first.Add(time.Minute), 0, 2); !next.Equal(first.Add(30 * time.Minute)) {
		t.Fatalf("next slot = %s, want %s", next, first.Add(30*time.Minute))
	}
}

func TestLeaderboardHoldingBatchesDoNotSplitAPlayerSnapshot(t *testing.T) {
	now := time.Date(2026, time.August, 6, 20, 15, 0, 0, time.UTC)
	scan := leaderboardScan{Players: map[int64]PlayerObservation{}, Holdings: map[string]HoldingObservation{}}
	for index := 0; index < MaximumHoldings-1; index++ {
		playerID := int64(index + 1)
		scan.Holdings[time.Unix(playerID, 0).String()] = HoldingObservation{
			WorldID: "world.example", AllianceID: 9, PlayerID: playerID, CastleID: playerID,
			ObservedAt: now,
		}
	}
	for castleID := int64(10_000); castleID < 10_003; castleID++ {
		scan.Holdings[time.Unix(castleID, 0).String()] = HoldingObservation{
			WorldID: "world.example", AllianceID: 9, PlayerID: 9_999, CastleID: castleID,
			ObservedAt: now,
		}
	}
	batches, err := buildLeaderboardBatches("world.example", now, scan)
	if err != nil {
		t.Fatal(err)
	}
	holdingBatches := make([]ObservationBatch, 0, 2)
	for _, batch := range batches {
		if len(batch.Holdings) > 0 {
			holdingBatches = append(holdingBatches, batch)
		}
	}
	if len(holdingBatches) != 2 {
		t.Fatalf("holding batch count = %d, want 2", len(holdingBatches))
	}
	for _, batch := range holdingBatches {
		count := 0
		for _, holding := range batch.Holdings {
			if holding.PlayerID == 9_999 {
				count++
			}
		}
		if count != 0 && count != 3 {
			t.Fatalf("player snapshot split with %d of 3 holdings in one batch", count)
		}
	}
}
