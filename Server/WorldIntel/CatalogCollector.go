package WorldIntel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
)

type catalogDatasetDefinition struct {
	Key      string
	Label    string
	Category string
}

// officialCatalogDatasets is intentionally explicit. These are the public
// official collections that describe rankings, point thresholds, event
// scoring, gacha pulls, and the rewards attached to those systems.
var officialCatalogDatasets = []catalogDatasetDefinition{
	{Key: "islandrewardranks", Label: "Storm alliance rank rewards", Category: "storm"},
	{Key: "islandPlayerRewards", Label: "Storm player point rewards", Category: "storm"},
	{Key: "leaguetypes", Label: "League definitions", Category: "rankings"},
	{Key: "leaguetypeevents", Label: "League event scoring and rewards", Category: "rankings"},
	{Key: "highscoreboni", Label: "Highscore rank bonuses", Category: "rankings"},
	{Key: "leaderboardRewards", Label: "Leaderboard reward brackets", Category: "rankings"},
	{Key: "mightranks", Label: "Might rank thresholds", Category: "rankings"},
	{Key: "alliancefameranks", Label: "Alliance fame rank thresholds", Category: "rankings"},
	{Key: "allianceranks", Label: "Alliance rank definitions", Category: "rankings"},
	{Key: "alliancerankrights", Label: "Alliance rank rights", Category: "rankings"},
	{Key: "seasonRanks", Label: "Season rank definitions", Category: "rankings"},
	{Key: "tempServerRankPoints", Label: "Temporary server rank points", Category: "rankings"},
	{Key: "tempServerRankRewards", Label: "Temporary server rank rewards", Category: "rankings"},
	{Key: "allianceBattleGroundRankRewards", Label: "Alliance battleground rank rewards", Category: "rankings"},
	{Key: "artifactsLeagues", Label: "Artifact league definitions", Category: "rankings"},
	{Key: "gachaEvents", Label: "Gacha events, pull limits, and spin costs", Category: "gacha"},
	{Key: "luckywheelrewardsets", Label: "Lucky wheel reward sets", Category: "gacha"},
	{Key: "saleDaysLuckyWheelRewardSets", Label: "Sale-day lucky wheel reward sets", Category: "gacha"},
	{Key: "events", Label: "Event definitions", Category: "events"},
	{Key: "pointeventtypes", Label: "Point event types", Category: "events"},
	{Key: "pointeventquests", Label: "Point event scoring quests", Category: "events"},
	{Key: "longtermpointeventquests", Label: "Long-term point event quests", Category: "events"},
	{Key: "pointeventrewardsets", Label: "Point event reward sets", Category: "events"},
	{Key: "collectorEventOptions", Label: "Collector event scoring options", Category: "events"},
	{Key: "collectorEventRewards", Label: "Collector event rewards", Category: "events"},
	{Key: "seasonEventRewards", Label: "Season event rewards", Category: "rewards"},
	{Key: "seasonEndRewards", Label: "Season end rewards", Category: "rewards"},
	{Key: "seasonPromotionRewards", Label: "Season promotion rewards", Category: "rewards"},
	{Key: "daimyoEndRewards", Label: "Daimyo end rewards", Category: "rewards"},
	{Key: "donationRewards", Label: "Donation reward thresholds", Category: "rewards"},
	{Key: "activityrewards", Label: "Activity rewards", Category: "rewards"},
	{Key: "rewardBags", Label: "Reward bag definitions", Category: "rewards"},
}

func buildOfficialCatalogSnapshots(
	store *GameData.Store,
	capturedAt time.Time,
	collectorPlayerID int64,
) ([]CatalogDatasetSnapshot, error) {
	if store == nil {
		return nil, fmt.Errorf("official game-data store is unavailable")
	}
	metadata := store.Metadata()
	base := CatalogDatasetSnapshot{
		Source: OfficialCatalogSource, SourceVersion: metadata.ItemVersion,
		SourceURL: metadata.SourceURL, SourceDigest: metadata.DigestSHA256,
		CapturedAt: capturedAt, CollectorPlayerID: collectorPlayerID,
	}
	snapshots := make([]CatalogDatasetSnapshot, 0, len(officialCatalogDatasets)+1)
	referencedRewardIDs := map[string]struct{}{}
	for _, definition := range officialCatalogDatasets {
		raw, found := store.RawCollection(definition.Key)
		if !found || countCatalogRows(raw) == 0 {
			continue
		}
		catalog, err := store.Catalog(definition.Key)
		if err != nil {
			return nil, fmt.Errorf("load official collection %s: %w", definition.Key, err)
		}
		summary := catalog.Summary()
		snapshot := base
		snapshot.DatasetKey = definition.Key
		snapshot.DatasetLabel = definition.Label
		snapshot.Category = definition.Category
		snapshot.Fields = summary.Fields
		snapshot.Rows = raw
		finalized, err := FinalizeCatalogSnapshot(snapshot)
		if err != nil {
			return nil, fmt.Errorf("finalize official collection %s: %w", definition.Key, err)
		}
		snapshots = append(snapshots, finalized)
		collectReferencedRewardIDs(raw, referencedRewardIDs)
	}
	if rewardSnapshot, found, err := buildReferencedRewardsSnapshot(store, base, referencedRewardIDs); err != nil {
		return nil, err
	} else if found {
		snapshots = append(snapshots, rewardSnapshot)
	}
	if len(snapshots) == 0 {
		return nil, fmt.Errorf("official item document contains no supported ranking or event collections")
	}
	sort.Slice(snapshots, func(left, right int) bool {
		if snapshots[left].Category != snapshots[right].Category {
			return snapshots[left].Category < snapshots[right].Category
		}
		return snapshots[left].DatasetLabel < snapshots[right].DatasetLabel
	})
	return snapshots, nil
}

func buildReferencedRewardsSnapshot(
	store *GameData.Store,
	base CatalogDatasetSnapshot,
	ids map[string]struct{},
) (CatalogDatasetSnapshot, bool, error) {
	if len(ids) == 0 {
		return CatalogDatasetSnapshot{}, false, nil
	}
	catalog, err := store.Catalog("rewards")
	if err != nil {
		return CatalogDatasetSnapshot{}, false, nil
	}
	rows := make([]json.RawMessage, 0, len(ids))
	for _, raw := range catalog.Rows() {
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			continue
		}
		id, found := scalarCatalogValue(record["rewardID"])
		if !found {
			continue
		}
		if _, referenced := ids[id]; referenced {
			rows = append(rows, raw)
		}
	}
	if len(rows) == 0 {
		return CatalogDatasetSnapshot{}, false, nil
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return CatalogDatasetSnapshot{}, false, fmt.Errorf("encode referenced official rewards: %w", err)
	}
	base.DatasetKey = "rankingReferencedRewards"
	base.DatasetLabel = "Referenced ranking and event reward definitions"
	base.Category = "rewards"
	base.Fields = catalog.Summary().Fields
	base.Rows = raw
	finalized, err := FinalizeCatalogSnapshot(base)
	if err != nil {
		return CatalogDatasetSnapshot{}, false, fmt.Errorf("finalize referenced official rewards: %w", err)
	}
	return finalized, true, nil
}

func collectReferencedRewardIDs(raw json.RawMessage, ids map[string]struct{}) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return
	}
	var walk func(any, bool)
	walk = func(current any, rewardField bool) {
		switch typed := current.(type) {
		case map[string]any:
			for key, nested := range typed {
				lower := strings.ToLower(key)
				isRewardID := strings.Contains(lower, "reward") && !strings.Contains(lower, "rewardset") && lower != "rewardbags"
				walk(nested, isRewardID)
			}
		case []any:
			for _, nested := range typed {
				walk(nested, rewardField)
			}
		case json.Number:
			if rewardField {
				ids[typed.String()] = struct{}{}
			}
		case string:
			if !rewardField {
				return
			}
			for _, candidate := range strings.FieldsFunc(typed, func(character rune) bool {
				return character == ',' || character == ';' || character == '|' || character == ' '
			}) {
				if _, err := strconv.ParseInt(candidate, 10, 64); err == nil {
					ids[candidate] = struct{}{}
				}
			}
		}
	}
	walk(value, false)
}

func countCatalogRows(raw json.RawMessage) int {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil {
		return 0
	}
	return len(rows)
}

func scalarCatalogValue(raw json.RawMessage) (string, bool) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), strings.TrimSpace(typed) != ""
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}
