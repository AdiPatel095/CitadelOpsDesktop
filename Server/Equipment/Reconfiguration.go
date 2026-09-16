package Equipment

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

// ReconfigurationTransition is the single transition model used for both the
// optimizer's cost quote and the authoritative command planner.
type ReconfigurationTransition struct {
	AlreadyAttached    map[int]bool
	GemsToDetach       map[State.GemInstanceID]State.GemInstance
	GemsToInsert       map[int]State.GemInstanceID
	ClearSlots         map[int]bool
	DetachCarrierCount map[int]int
}

type ExtractionQuote struct {
	RubyExtractionCount  int    `json:"rubyExtractionCount"`
	MaximumRubySpend     int64  `json:"maximumRubySpend"`
	RelicExtractionCount int    `json:"relicExtractionCount"`
	SocketInsertionCount int    `json:"socketInsertionCount"`
	Fingerprint          string `json:"fingerprint,omitempty"`
}

func BuildReconfigurationTransition(
	gameState State.GameState,
	currentEquipment map[string]State.EquipmentInstanceID,
	targetEquipment map[string]State.EquipmentInstanceID,
	targetGems map[string]State.GemInstanceID,
) (ReconfigurationTransition, error) {
	transition := ReconfigurationTransition{
		AlreadyAttached:    map[int]bool{},
		GemsToDetach:       map[State.GemInstanceID]State.GemInstance{},
		GemsToInsert:       map[int]State.GemInstanceID{},
		ClearSlots:         map[int]bool{},
		DetachCarrierCount: map[int]int{},
	}
	gemsByEquipment := make(map[State.EquipmentInstanceID]State.GemInstance, len(gameState.Inventory.Gems))
	for _, gem := range gameState.Inventory.Gems {
		if gem.EquipmentInstanceID <= 0 {
			continue
		}
		if existing, duplicate := gemsByEquipment[gem.EquipmentInstanceID]; duplicate && existing.ID != gem.ID {
			return ReconfigurationTransition{}, fmt.Errorf("equipment %d has more than one socketed gem", gem.EquipmentInstanceID)
		}
		gemsByEquipment[gem.EquipmentInstanceID] = gem
	}
	for slot := 1; slot <= 4; slot++ {
		key := strconv.Itoa(slot)
		gemID := targetGems[key]
		destinationEquipmentID := targetEquipment[key]
		if attached, found := gemsByEquipment[destinationEquipmentID]; found && attached.ID != gemID {
			transition.GemsToDetach[attached.ID] = attached
		}
		if gemID == 0 {
			continue
		}
		gem, found := gameState.Inventory.Gems[gemID]
		if !found {
			return ReconfigurationTransition{}, fmt.Errorf("gem %d is not in current state", gemID)
		}
		if gem.EquipmentInstanceID == destinationEquipmentID {
			transition.AlreadyAttached[slot] = true
			continue
		}
		transition.GemsToInsert[slot] = gemID
		if gem.EquipmentInstanceID > 0 {
			transition.GemsToDetach[gem.ID] = gem
		}
	}
	for _, slot := range optimizerSlots {
		key := strconv.Itoa(slot)
		if currentEquipment[key] != targetEquipment[key] {
			transition.ClearSlots[slot] = true
		}
	}
	for _, gem := range transition.GemsToDetach {
		parent, found := gameState.Inventory.Equipment[gem.EquipmentInstanceID]
		if !found {
			return ReconfigurationTransition{}, fmt.Errorf("cannot detach gem %d from missing equipment %d", gem.ID, gem.EquipmentInstanceID)
		}
		parentSlot := strconv.Itoa(parent.Slot)
		transition.DetachCarrierCount[parent.Slot]++
		if currentEquipment[parentSlot] == targetEquipment[parentSlot] && targetEquipment[parentSlot] != parent.ID {
			transition.ClearSlots[parent.Slot] = true
		}
	}
	return transition, nil
}

func QuoteReconfiguration(gameData *GameData.Store, transition ReconfigurationTransition) (ExtractionQuote, error) {
	quote := ExtractionQuote{SocketInsertionCount: len(transition.GemsToInsert)}
	gemIDs := make([]int64, 0, len(transition.GemsToDetach))
	for id := range transition.GemsToDetach {
		gemIDs = append(gemIDs, int64(id))
	}
	sort.Slice(gemIDs, func(left, right int) bool { return gemIDs[left] < gemIDs[right] })
	for _, rawID := range gemIDs {
		gem := transition.GemsToDetach[State.GemInstanceID(rawID)]
		switch GemFamily(gem) {
		case LoadoutFamilyOrdinary:
			cost, err := NormalGemRemovalCost(gameData, gem)
			if err != nil {
				return ExtractionQuote{}, err
			}
			if quote.MaximumRubySpend > math.MaxInt64-cost {
				return ExtractionQuote{}, fmt.Errorf("normal gem removal cost exceeds supported ruby range")
			}
			quote.RubyExtractionCount++
			quote.MaximumRubySpend += cost
		case LoadoutFamilyRelic:
			quote.RelicExtractionCount++
		default:
			return ExtractionQuote{}, fmt.Errorf("gem %d has no verified ordinary or relic classification", gem.ID)
		}
	}
	return quote, nil
}

func NormalGemRemovalCost(gameData *GameData.Store, gem State.GemInstance) (int64, error) {
	if gameData == nil {
		return 0, fmt.Errorf("official game data is unavailable for normal gem %d removal cost", gem.ID)
	}
	if GemFamily(gem) != LoadoutFamilyOrdinary || gem.DefinitionID <= 0 {
		return 0, fmt.Errorf("normal gem %d has no valid catalog definition", gem.ID)
	}
	gems, err := gameData.Catalog("gems")
	if err != nil {
		return 0, fmt.Errorf("official gems catalog is unavailable: %w", err)
	}
	rawGem, found := gems.Find(strconv.FormatInt(int64(gem.DefinitionID), 10))
	if !found {
		return 0, fmt.Errorf("normal gem definition %d is missing from official game data", gem.DefinitionID)
	}
	gemRecord, err := GameData.DecodeRecord(rawGem)
	if err != nil {
		return 0, fmt.Errorf("decode normal gem definition %d: %w", gem.DefinitionID, err)
	}
	levelID, found := gemRecord.Int64("gemLevelID")
	if !found || levelID < 0 {
		return 0, fmt.Errorf("normal gem definition %d has no valid gemLevelID", gem.DefinitionID)
	}
	levels, err := gameData.Catalog("gemlevels")
	if err != nil {
		return 0, fmt.Errorf("official gemlevels catalog is unavailable: %w", err)
	}
	rawLevel, found := levels.Find(strconv.FormatInt(levelID, 10))
	if !found {
		return 0, fmt.Errorf("gem level %d is missing from official game data", levelID)
	}
	levelRecord, err := GameData.DecodeRecord(rawLevel)
	if err != nil {
		return 0, fmt.Errorf("decode gem level %d: %w", levelID, err)
	}
	cost, found := levelRecord.Int64("removalCostC2")
	if !found || cost < 0 {
		return 0, fmt.Errorf("gem level %d has no valid non-negative removalCostC2", levelID)
	}
	return cost, nil
}

func ReconfigurationQuoteFingerprint(
	snapshotFingerprint string,
	equipment map[string]State.EquipmentInstanceID,
	gems map[string]State.GemInstanceID,
	quote ExtractionQuote,
) string {
	digest := sha256.New()
	fingerprintWrite(digest, "equipment-extraction-quote-v1", snapshotFingerprint)
	for _, slot := range optimizerSlots {
		fingerprintWrite(digest, "equipment", slot, int64(equipment[strconv.Itoa(slot)]))
	}
	for slot := 1; slot <= gemSlotCount; slot++ {
		fingerprintWrite(digest, "gem", slot, int64(gems[strconv.Itoa(slot)]))
	}
	fingerprintWrite(digest, quote.RubyExtractionCount, quote.MaximumRubySpend, quote.RelicExtractionCount, quote.SocketInsertionCount)
	return hex.EncodeToString(digest.Sum(nil))
}
