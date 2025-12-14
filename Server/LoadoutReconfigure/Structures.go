package LoadoutReconfigure

import (
	"CitadelDesktop/Server/Models"
	"math"
)

// OptimizationProfile defines the user's weighted priorities
type OptimizationProfile struct {
	Tier0Stats          []string
	Tier1Stats          []string
	Tier2Stats          []string
	InterTierMultiplier float64
	IntraTierMultiplier float64
	RelevanceMask       map[string]bool
	StatCeilings        map[string]float64
	HeroCeilings        map[string]float64
}

// StatMaxValues contains the universal max possible values for each stat type
// Used for ceiling-aware efficiency scoring
var StatMaxValues = map[string]float64{
	// Base combat stats (equipment)
	"MeleeCbtStr": 40, "RangeCbtStr": 40, "FrontCbtStr": 25, "FlankCbtStr": 25,
	"AllCbtStr": 25, "CyCbtStr": 50, "WallStr": 50, "GateStr": 50, "MoatStr": 35,
	"FlankLimit": 15, "FrontLimit": 15, "MaidenSupp": 50, "Travel": 30, "Loot": 30,
	// Hero CL stats
	"CLMelee": 50, "CLRange": 50, "CLFront": 40, "CLFlank": 40, "CLCy": 60,
	"CLWall": 50, "CLGate": 50, "CLMoat": 30, "CLLater": 100, "CLFire": 40, "CLGlory": 35,
	// Hero NPC stats
	"NPCMelee": 50, "NPCRange": 50, "NPCFront": 40, "NPCFlank": 40, "NPCCy": 60,
	"NPCWall": 50, "NPCGate": 50, "NPCMoat": 30, "NPCGlory": 35,
	// Hero bonus stats
	"EliteStr": 60, "HorrorStr": 60, "BeserkerStr": 60, "RelicStr": 60, "MeadStr": 20,
	"Wave": 1, "Cooldown": 15,
}

// CandidateItem is a simplified representation of an equipment item (Base + Gem)
type CandidateItem struct {
	OriginalID float64
	GemID      float64 // If this is a base item with a gem, this is the gem's ID
	Slot       float64
	IsGem      bool
	HasGemSlot bool // True if this item CAN hold a gem (Rarity 5/15)
	// Stats stores the aggregated values keyed by the stat name (e.g., "MeleeCbtStr")
	Stats map[string]float64
	// StatEfficiency stores the efficiency/percent (0-100) for each stat
	StatEfficiency map[string]float64
}

// StatIDToNameMap maps the raw Game ID to the internal Field Name
// This mirrors Server/Models/CommStatModel.go logic
// Stat Mappings
var BaseStatMap = map[float64]string{
	1: "MeleeCbtStr", 2: "RangeCbtStr", 3: "WallStr", 4: "GateStr", 5: "MoatStr", 6: "Travel", 7: "Loot",
	101: "MeleeCbtStr", 102: "RangeCbtStr", 103: "WallStr", 104: "GateStr", 105: "MoatStr", 106: "Travel", 107: "Loot",
	108: "MeleeCbtStr", 109: "RangeCbtStr", 110: "WallStr", 111: "GateStr", 112: "MoatStr", 113: "Travel", 114: "Loot",
	115: "FlankLimit", 116: "CyCbtStr", 117: "FrontLimit", 118: "AllCbtStr", 119: "FrontCbtStr", 120: "FlankCbtStr", 121: "MaidenSupp",
	// Hero bonus stats
	20012: "EliteStr", 20013: "HorrorStr", 20015: "BeserkerStr", 20016: "RelicStr", 20017: "MeadStr",
	20018: "Wave", 20019: "Cooldown", 20020: "Travel",
}

var HeroStatMap = map[float64]string{
	// Specific Overrides for Heroes/Gems where ID 1 means CLMelee
	1: "CLMelee", 2: "CLRange", 3: "CLWall", 4: "CLGate", 5: "CLMoat",

	// Hero Castle Lord (8xx range)
	806: "CLLater", 807: "CLMoat", 808: "CLWall", 809: "CLGate", 810: "CLCy", 811: "CLFlank", 812: "CLFront",
	813: "CLMelee", 814: "CLRange", 815: "CLFire", 816: "CLGlory",

	// Gem CL stats (3xx range)
	309: "CLWall", 310: "CLGate", 311: "CLMelee", 312: "CLRange", 313: "CLFlank", 314: "CLCy", 315: "CLFront",
	316: "CLLater", 317: "CLMoat", 318: "CLFire", 319: "CLGlory",

	// Gem NPC stats (2xx range)
	209: "NPCWall", 210: "NPCGate", 211: "NPCMelee", 212: "NPCRange", 213: "NPCFlank", 214: "NPCCy", 215: "NPCFront",
	216: "NPCMoat", 320: "NPCGlory",
}

// ========================================
// UNMAPPED STAT IDs - TODO: Map these later
// ========================================
// 20014: Unknown hero bonus stat (between HorrorStr and BeserkerStr)
//
// 4xx range (NPC hero stats - matches 8xx pattern for NPC instead of CL):
//   406, 407, 408, 409, 410, 411, 412, 413, 414, 415, 416
//
// 6xx range (different hero type - unknown category):
//   609, 612, 616, 617
//
// 105xx range (extended NPC stats?):
//   10507, 10508, 10509, 10510, 10512, 10513, 10514, 10516, 10517, 10518
//
// 3xxxx range (production/special bonuses with very high values):
//   30009, 30012, 30017, 30020
// ========================================

func GetStatName(id float64, context string) string {
	if context == "Hero" || context == "Gem" {
		if val, ok := HeroStatMap[id]; ok {
			return val
		}
		// Fallback: Some gems might use Base IDs (e.g. Loot ID 7) which are universal
		if val, ok := BaseStatMap[id]; ok {
			return val
		}
	} else {
		// Default Equipment Context
		if val, ok := BaseStatMap[id]; ok {
			return val
		}
	}
	return ""
}

// ParseEquipmentToCandidate converts a raw EquipmentModel to a CandidateItem
func ParseEquipmentToCandidate(eq Models.EquipmentModel) CandidateItem {
	candidate := CandidateItem{
		OriginalID: eq.ID,
		Slot:       eq.EquipSlotNumber,
		HasGemSlot: (eq.EquipRarity == 5 || eq.EquipRarity == 15),
		Stats:      make(map[string]float64),
	}

	// Helper to add value
	addStat := func(id float64, val float64) {
		name := GetStatName(id, "Equipment")
		if name != "" {
			candidate.Stats[name] += val
		}
	}

	// Process Base Stats
	for _, stat := range eq.EquipStats {
		// Logic from EquipmentParser.go: Special handling for certain IDs
		val := stat.Value[0]
		if (stat.ID >= 20012 && stat.ID <= 20017) || stat.ID == 121 {
			if len(stat.Value) > 1 {
				val = stat.Value[1]
			}
		}
		addStat(stat.ID, val)
	}

	// Process Gem Stats (if embedded)
	if eq.EquipRarity == 5 || eq.EquipRarity == 15 {
		if eq.GemSlot.Gem != nil {
			for _, gemStat := range eq.GemSlot.Gem.GemStats {
				val := gemStat.Value[0]
				if gemStat.ID >= 20012 && gemStat.ID <= 20017 {
					if len(gemStat.Value) > 1 {
						val = gemStat.Value[1]
					}
				}
				addStat(gemStat.ID, val)
			}
		}
	}

	return candidate
}

// MergeCandidates combines two candidates (e.g. Item + Gem)
func MergeCandidates(base CandidateItem, gem CandidateItem) CandidateItem {
	newCandidate := CandidateItem{
		OriginalID: base.OriginalID,
		GemID:      gem.OriginalID, // Capture the gem's ID
		Slot:       base.Slot,
		Stats:      make(map[string]float64),
	}

	for k, v := range base.Stats {
		newCandidate.Stats[k] = v
	}
	for k, v := range gem.Stats {
		newCandidate.Stats[k] += v
	}

	// Clean up rounding errors
	for k, v := range newCandidate.Stats {
		newCandidate.Stats[k] = math.Round(v*10) / 10
	}

	return newCandidate
}
