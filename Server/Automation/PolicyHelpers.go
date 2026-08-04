package Automation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/CommanderFeatures"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const commanderFeatureSection = CommanderFeatures.Section

func decodeSection(snapshot Configuration.Snapshot, section string, destination any) bool {
	raw := snapshot.Sections[section]
	return len(raw) > 0 && json.Unmarshal(raw, destination) == nil
}

func commanderFeatureCandidates(
	gameState State.GameState,
	configuration Configuration.Snapshot,
	featureID string,
) ([]State.CommanderID, bool) {
	raw, configured := configuration.Sections[commanderFeatureSection]
	if !configured {
		return nil, false
	}
	if len(raw) == 0 {
		return nil, true
	}
	settings, err := CommanderFeatures.Decode(raw)
	if err != nil {
		return nil, true
	}
	return CommanderFeatures.Candidates(gameState, settings, featureID)
}

func hasAvailableFeatureCommander(
	gameState State.GameState,
	candidates []State.CommanderID,
	restricted bool,
	now time.Time,
) bool {
	_, found := nextAvailableFeatureCommander(gameState, candidates, restricted, now)
	return found
}

func nextAvailableFeatureCommander(
	gameState State.GameState,
	candidates []State.CommanderID,
	restricted bool,
	now time.Time,
) (State.CommanderID, bool) {
	if !restricted {
		candidates = make([]State.CommanderID, 0, len(gameState.Commanders))
		for commanderID := range gameState.Commanders {
			if commanderID >= 0 {
				candidates = append(candidates, commanderID)
			}
		}
		sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	}
	for _, commanderID := range candidates {
		if commander, exists := gameState.Commanders[commanderID]; exists && commander.Available &&
			!State.CommanderHasActiveMovementAt(gameState, commanderID, now) {
			return commanderID, true
		}
	}
	return 0, false
}

func validHorseTravelBoostID(value int) bool {
	if value == 0 || value == -1 {
		return true
	}
	_, valid := GameData.HorseTravelBoostTierForSelection(value)
	return valid
}

func resolvePolicyHorseTravelBoost(
	gameData *GameData.Store,
	castle State.CastleState,
	value int,
) error {
	if value == 0 || value == -1 {
		return nil
	}
	tier, selected := GameData.HorseTravelBoostTierForSelection(value)
	if !selected {
		return fmt.Errorf("horseTravelBoostId must be -1, 1007, 1008, or 1009")
	}
	if gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	_, err := gameData.ResolveHorseTravelBoost(castle, tier)
	return err
}

func horseTravelBoostLayoutNeedsRefresh(castle State.CastleState, now time.Time, maximumAge time.Duration) bool {
	observedAt := castle.Layout.ObservedAt
	if observedAt.IsZero() || observedAt.After(now) {
		return true
	}
	return maximumAge > 0 && now.Sub(observedAt) >= maximumAge
}

func policyInterval(seconds int, fallback int) time.Duration {
	if seconds < 30 || seconds > 86_400 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}

func pendingKingdomResourceTransport(
	gameState State.GameState,
	kingdomID State.KingdomID,
) (State.KingdomResourceTransport, bool) {
	for _, pending := range gameState.KingdomTransport.Pending {
		if pending.KingdomID == kingdomID {
			return pending, true
		}
	}
	return State.KingdomResourceTransport{}, false
}

func kingdomResourceTransportWorkflow(
	gameState State.GameState,
	kingdomID State.KingdomID,
) (State.KingdomResourceTransportWorkflow, bool) {
	workflow, exists := gameState.KingdomTransport.ResourceWorkflows[kingdomID]
	return workflow, exists
}

// dailyAttackLimitAllowance returns -1 when the feature's cap is disabled.
// A non-nil decision means no normal CRA may be queued until authoritative
// server state changes. Advisor attacks intentionally do not call this helper.
func dailyAttackLimitAllowance(
	snapshot Snapshot,
	limit int64,
	interval time.Duration,
	metrics map[string]float64,
) (int64, *Decision) {
	if limit == 0 {
		return -1, nil
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if metrics != nil {
		metrics["dailyAttackLimit"] = float64(limit)
	}
	if limit < 0 {
		return 0, &Decision{
			Status: "waiting", Detail: "Daily attack limit cannot be negative",
			NextCheckAt: snapshot.Now.Add(interval), Metrics: metrics,
		}
	}
	attacks := snapshot.State.DailyAttacks
	if attacks.ObservedAt.IsZero() {
		return 0, &Decision{
			Status: "waiting", Detail: "Waiting for the server daily attack count before queuing another attack",
			NextCheckAt: snapshot.Now.Add(interval), Metrics: metrics,
		}
	}
	if metrics != nil {
		metrics["dailyAttackCount"] = float64(attacks.Count)
		metrics["serverDailyAttackThreshold"] = float64(attacks.ServerThreshold)
	}
	remaining := max(int64(0), limit-attacks.Count)
	if remaining == 0 {
		return 0, &Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Daily attack limit reached: %d / %d; normal attacks resume when the server count resets",
				attacks.Count, limit,
			),
			NextCheckAt: snapshot.Now.Add(interval), Metrics: metrics,
		}
	}
	if metrics != nil {
		metrics["dailyAttacksRemaining"] = float64(remaining)
	}
	return remaining, nil
}

func capDailyAttackBatch(requested int, allowance int64) int {
	if requested <= 0 || allowance < 0 {
		return max(0, requested)
	}
	if allowance >= int64(requested) {
		return requested
	}
	return int(allowance)
}

func marketLogisticsRequired(snapshot Snapshot) (bool, error) {
	kingdomCastleCounts := map[State.KingdomID]int{}
	for _, castle := range snapshot.State.Castles {
		kingdomCastleCounts[castle.KingdomID]++
	}
	hasSameKingdomPair := false
	for _, count := range kingdomCastleCounts {
		if count > 1 {
			hasSameKingdomPair = true
			break
		}
	}
	if !hasSameKingdomPair || snapshot.GameData == nil {
		return false, nil
	}
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		if kingdomCastleCounts[castle.KingdomID] < 2 {
			continue
		}
		hasMarketplace, err := snapshot.GameData.CastleHasMarketplace(castle)
		if err != nil {
			return false, fmt.Errorf("check marketplace at %s: %w", castleName(castle), err)
		}
		if hasMarketplace {
			return true, nil
		}
	}
	return false, nil
}

func kingdomLogisticsRequired(gameState State.GameState) bool {
	kingdoms := map[State.KingdomID]struct{}{}
	for _, castle := range gameState.Castles {
		kingdoms[castle.KingdomID] = struct{}{}
		if len(kingdoms) > 1 {
			return true
		}
	}
	return false
}

func sortedNumericKeys[Value any](values map[string]Value) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if id, err := strconv.ParseInt(key, 10, 64); err == nil && id > 0 {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		leftID, _ := strconv.ParseInt(keys[left], 10, 64)
		rightID, _ := strconv.ParseInt(keys[right], 10, 64)
		return leftID < rightID
	})
	return keys
}

func recordNumber(store *GameData.Store, collection string, id int64, field string) (float64, bool) {
	if store == nil || id <= 0 {
		return 0, false
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return 0, false
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return 0, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, false
	}
	return record.Float64(field)
}

func recordInteger(store *GameData.Store, collection string, id int64, field string) int64 {
	value, ok := recordNumber(store, collection, id, field)
	if !ok {
		return 0
	}
	return int64(value)
}

func commaSeparatedIDs(value string) map[int64]struct{} {
	result := map[int64]struct{}{}
	for _, part := range strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == '#'
	}) {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			result[id] = struct{}{}
		}
	}
	return result
}

func eligibleAllianceHelpProductionID(queue State.ProductionQueue) int64 {
	if queue.LineID != 0 && queue.LineID != 2 {
		return 0
	}
	eligible := func(item State.QueueItem) bool {
		return item.ProductionID > 0 && !item.AllianceHelpRequested && (queue.LineID != 0 || item.Amount >= 5)
	}
	if queue.Active != nil && eligible(*queue.Active) {
		return queue.Active.ProductionID
	}
	for _, item := range queue.Queued {
		if eligible(item) {
			return item.ProductionID
		}
	}
	return 0
}
