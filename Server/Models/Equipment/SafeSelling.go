package equipment

import (
	"encoding/json"
	"log"
	"sync"
)

var (
	oldRedGearIDs   map[int]struct{}
	oldRedGemIDs    map[int]struct{}
	post2026GearIDs map[int]struct{}
	post2026GemIDs  map[int]struct{}
	sellingInitOnce sync.Once
)

func initSellingBuckets() {
	sellingInitOnce.Do(func() {
		oldRedGearIDs = make(map[int]struct{})
		oldRedGemIDs = make(map[int]struct{})
		post2026GearIDs = make(map[int]struct{})
		post2026GemIDs = make(map[int]struct{})

		loadIDs := func(raw []byte, label string, target map[int]struct{}) {
			if len(raw) == 0 {
				log.Printf("[SellingBuckets] Warning: embedded %s is empty", label)
				return
			}
			var ids []int
			if err := json.Unmarshal(raw, &ids); err != nil {
				log.Printf("[SellingBuckets] Error parsing embedded %s: %v", label, err)
				return
			}
			for _, id := range ids {
				target[id] = struct{}{}
			}
		}

		loadIDs(embeddedOldRedGearJSON, "old_red_gear", oldRedGearIDs)
		loadIDs(embeddedOldRedGemsJSON, "old_red_gems", oldRedGemIDs)
		loadIDs(embeddedPost2026GearJSON, "post_2026_gear", post2026GearIDs)
		loadIDs(embeddedPost2026GemsJSON, "post_2026_gems", post2026GemIDs)

		log.Printf("[SellingBuckets] Loaded %d old red gear, %d old red gems", len(oldRedGearIDs), len(oldRedGemIDs))
		log.Printf("[SellingBuckets] Loaded %d special post-2026 gear, %d special post-2026 gems", len(post2026GearIDs), len(post2026GemIDs))
	})
}

// IsOldRedEquipment returns true if the equipment ID is in the pre-2026 bucket.
func IsOldRedEquipment(id int) bool {
	initSellingBuckets()
	_, ok := oldRedGearIDs[id]
	return ok
}

// IsOldRedGem returns true if the gem ID is in the pre-2026 bucket.
func IsOldRedGem(id int) bool {
	initSellingBuckets()
	_, ok := oldRedGemIDs[id]
	return ok
}

// IsSpecialPost2026Equipment returns true if the equipment belongs to Rift, Spore, or Victorious sets.
func IsSpecialPost2026Equipment(id int) bool {
	initSellingBuckets()
	_, ok := post2026GearIDs[id]
	return ok
}

// IsSpecialPost2026Gem returns true if the gem belongs to Rift, Spore, or Victorious sets.
func IsSpecialPost2026Gem(id int) bool {
	initSellingBuckets()
	_, ok := post2026GemIDs[id]
	return ok
}
