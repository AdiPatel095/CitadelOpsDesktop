package WorldIntel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
)

func TestOfficialCatalogCollectorBuildsStormGachaAndReferencedRewards(t *testing.T) {
	raw := []byte(`{
		"versionInfo":{},"buildings":[{}],"units":[{}],
		"islandrewardranks":[{"islandRewardRankID":"1","cargoPointRequirement":"500","rewardIDs":"100,101","topXValue":"10"}],
		"gachaEvents":[{"eventID":"8","gachaID":"2","minPulls":"1","maxPulls":"20","rewardSetID":"9"}],
		"rewards":[
			{"rewardID":"100","comment1":"Storm tools","units":"1,20"},
			{"rewardID":"101","comment1":"Storm currency","aquamarine":"500"},
			{"rewardID":"999","comment1":"Unreferenced"}
		]
	}`)
	store, err := GameData.DecodeStore(raw, GameData.SourceMetadata{
		ItemVersion:  "781.02",
		SourceURL:    "https://empire-html5.goodgamestudios.com/default/items/items_v781.02.json",
		DigestSHA256: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := buildOfficialCatalogSnapshots(
		store, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC), 17334928,
	)
	if err != nil {
		t.Fatal(err)
	}
	byKey := make(map[string]CatalogDatasetSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byKey[snapshot.DatasetKey] = snapshot
		if err := ValidateFinalizedCatalogSnapshot(snapshot); err != nil {
			t.Fatalf("%s validation: %v", snapshot.DatasetKey, err)
		}
	}
	for _, key := range []string{"islandrewardranks", "gachaEvents", "rankingReferencedRewards"} {
		if _, found := byKey[key]; !found {
			t.Errorf("missing %s snapshot", key)
		}
	}
	var rewards []map[string]any
	if err := json.Unmarshal(byKey["rankingReferencedRewards"].Rows, &rewards); err != nil {
		t.Fatal(err)
	}
	if len(rewards) != 2 {
		t.Fatalf("referenced rewards = %#v", rewards)
	}
}

func TestCatalogSnapshotIdentityIgnoresCollectorAndCaptureTime(t *testing.T) {
	base := CatalogDatasetSnapshot{
		Source: OfficialCatalogSource, SourceVersion: "781.02",
		SourceURL:    "https://empire-html5.goodgamestudios.com/default/items/items_v781.02.json",
		SourceDigest: strings.Repeat("a", 64), DatasetKey: "gachaEvents", DatasetLabel: "Gacha events",
		Category: "gacha", Fields: []string{"eventID"}, Rows: json.RawMessage(`[{"eventID":"8"}]`),
		CapturedAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC), CollectorPlayerID: 17334928,
	}
	first, err := FinalizeCatalogSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	base.CapturedAt = base.CapturedAt.Add(time.Hour)
	base.CollectorPlayerID = 17756610
	second, err := FinalizeCatalogSnapshot(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.SnapshotID != second.SnapshotID {
		t.Fatalf("snapshot IDs differ: %s != %s", first.SnapshotID, second.SnapshotID)
	}
}

func TestCatalogCollectorAssignmentsAndSlots(t *testing.T) {
	if !(Settings{CollectorPlayerID: 17334928, CollectorSlot: 1, CollectorSlots: 2}).collectsCatalog() {
		t.Fatal("Adolphus assignment was not accepted")
	}
	if !(Settings{CollectorPlayerID: 17756610, CollectorSlot: 0, CollectorSlots: 2}).collectsCatalog() {
		t.Fatal("James assignment was not accepted")
	}
	if (Settings{CollectorPlayerID: 0, CollectorSlot: 0, CollectorSlots: 0}).collectsCatalog() {
		t.Fatal("unassigned Amos settings were accepted")
	}
	first := time.Unix(0, 0).UTC()
	_, firstSlot := collectorBucket(first, 0, 2)
	_, secondSlot := collectorBucket(first, 1, 2)
	_, firstSlotNext := collectorBucket(first.Add(captureBucket), 0, 2)
	_, secondSlotNext := collectorBucket(first.Add(captureBucket), 1, 2)
	if !firstSlot || secondSlot || firstSlotNext || !secondSlotNext {
		t.Fatalf("slot schedule = %v %v %v %v", firstSlot, secondSlot, firstSlotNext, secondSlotNext)
	}
}
