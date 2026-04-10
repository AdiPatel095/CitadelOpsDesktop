package ReconfigureLoadout

import "CitadelDesktop/Server/Models"

// ReconfigurePayload is the payload structure received from the frontend
// Tier 1 = Priority Stats (scored and weighted by position)
// Tier 2 = Optimize Stats (just needs to be present, checked via bitmask)
type ReconfigurePayload struct {
	EquipmentMode string `json:"equipmentMode"` // "Commander" or "Castellan"
	CombatMode    string `json:"combatMode"`    // "PVP" or "NPC"
	TargetIndex   int    `json:"targetIndex"`
	Stats         []struct {
		Stat     string `json:"stat"`
		Tier     int    `json:"tier"`     // 1 = Priority, 2 = Optimize
		Position int    `json:"position"` // Position within the tier (0-indexed)
	} `json:"stats"`
}

// PreparedPriority holds preprocessed stat priority data for efficient optimizer use.
// Tier 1 stats: High priority, 10000 base score, 10% cumulative decay
// Tier 2 stats: Lower priority, 100 base score, 5% cumulative decay
type PreparedPriority struct {
	// Tier1Stats contains priority stats for weighted scoring (sorted by position)
	Tier1Stats []PriorityStat

	// Tier2Stats contains optimize stats for weighted scoring (sorted by position)
	// Also used for pruning bitmask generation
	Tier2Stats []PriorityStat

	// Tier2Bitmask is a set of stat IDs for O(1) presence lookup (derived from Tier2Stats)
	// Used for pruning equipment combinations
	Tier2Bitmask map[float64]bool
}

// OptimizationResult holds the result of the optimization
// This is a generic result that can be converted to CommStatModel or CastStatModel
type OptimizationResult struct {
	Equip1 float64 // Equipment ID for slot 1
	Equip2 float64 // Equipment ID for slot 2
	Equip3 float64 // Equipment ID for slot 3
	Equip4 float64 // Equipment ID for slot 4
	Hero   float64 // Hero ID for slot 6
	Gem1   float64 // Gem ID for slot 1
	Gem2   float64 // Gem ID for slot 2
	Gem3   float64 // Gem ID for slot 3
	Gem4   float64 // Gem ID for slot 4
	Score  float64 // Score of the loadout
}

// OptimizerInput contains the pre-filtered equipment, heroes, and gems
// for the optimizer to work with
type OptimizerInput struct {
	Equipment []Models.EquipmentModel // Pre-filtered equipment (slots 1-4)
	Heroes    []Models.EquipmentModel // Pre-filtered heroes (slot 6)
	Gems      []Models.Gem            // Pre-filtered gems
	Payload   ReconfigurePayload      // Original payload with stat priorities
}
