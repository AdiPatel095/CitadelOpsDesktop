package ReconfigureLoadout

import (
	"CitadelDesktop/Server/Models"
	"math"
	"sort"
	"sync"
)

// Scoring constants
const (
	Tier1BaseScore = 10000.0
	Tier1Decay     = 0.10 // 10% decay per position
	Tier2BaseScore = 100.0
	Tier2Decay     = 0.05 // 5% decay per position
)

// CommStatGetter maps stat names to functions that retrieve the value from CommStatModel
var CommStatGetter = map[string]func(*Models.CommStatModel) float64{
	// Base Stats
	"meleeCbtStr": func(c *Models.CommStatModel) float64 { return c.MeleeCbtStr },
	"rangeCbtStr": func(c *Models.CommStatModel) float64 { return c.RangeCbtStr },
	"frontCbtStr": func(c *Models.CommStatModel) float64 { return c.FrontCbtStr },
	"flankCbtStr": func(c *Models.CommStatModel) float64 { return c.FlankCbtStr },
	"allCbtStr":   func(c *Models.CommStatModel) float64 { return c.AllCbtStr },
	"cyCbtStr":    func(c *Models.CommStatModel) float64 { return c.CyCbtStr },
	"wallStr":     func(c *Models.CommStatModel) float64 { return c.WallStr },
	"gateStr":     func(c *Models.CommStatModel) float64 { return c.GateStr },
	"moatStr":     func(c *Models.CommStatModel) float64 { return c.MoatStr },
	"flankLimit":  func(c *Models.CommStatModel) float64 { return c.FlankLimit },
	"frontLimit":  func(c *Models.CommStatModel) float64 { return c.FrontLimit },
	"meadStr":     func(c *Models.CommStatModel) float64 { return c.MeadStr },
	"horrorStr":   func(c *Models.CommStatModel) float64 { return c.HorrorStr },
	"eliteStr":    func(c *Models.CommStatModel) float64 { return c.EliteStr },
	"wave":        func(c *Models.CommStatModel) float64 { return c.Wave },
	"cooldown":    func(c *Models.CommStatModel) float64 { return c.Cooldown },
	"relicStr":    func(c *Models.CommStatModel) float64 { return c.RelicStr },
	"beserkerStr": func(c *Models.CommStatModel) float64 { return c.BeserkerStr },
	"maidenSupp":  func(c *Models.CommStatModel) float64 { return c.MaidenSupp },
	"travel":      func(c *Models.CommStatModel) float64 { return c.Travel },
	"loot":        func(c *Models.CommStatModel) float64 { return c.Loot },

	// NPC Stats
	"NPCMelee": func(c *Models.CommStatModel) float64 { return c.NPCMelee },
	"NPCRange": func(c *Models.CommStatModel) float64 { return c.NPCRange },
	"NPCFront": func(c *Models.CommStatModel) float64 { return c.NPCFront },
	"NPCFlank": func(c *Models.CommStatModel) float64 { return c.NPCFlank },
	"NPCCy":    func(c *Models.CommStatModel) float64 { return c.NPCCy },
	"NPCWall":  func(c *Models.CommStatModel) float64 { return c.NPCWall },
	"NPCGate":  func(c *Models.CommStatModel) float64 { return c.NPCGate },
	"NPCMoat":  func(c *Models.CommStatModel) float64 { return c.NPCMoat },
	"NPCGlory": func(c *Models.CommStatModel) float64 { return c.NPCGlory },

	// CL/PvP Stats
	"CLMelee": func(c *Models.CommStatModel) float64 { return c.CLMelee },
	"CLRange": func(c *Models.CommStatModel) float64 { return c.CLRange },
	"CLFront": func(c *Models.CommStatModel) float64 { return c.CLFront },
	"CLFlank": func(c *Models.CommStatModel) float64 { return c.CLFlank },
	"CLCy":    func(c *Models.CommStatModel) float64 { return c.CLCy },
	"CLWall":  func(c *Models.CommStatModel) float64 { return c.CLWall },
	"CLGate":  func(c *Models.CommStatModel) float64 { return c.CLGate },
	"CLMoat":  func(c *Models.CommStatModel) float64 { return c.CLMoat },
	"CLLater": func(c *Models.CommStatModel) float64 { return c.CLLater },
	"CLFire":  func(c *Models.CommStatModel) float64 { return c.CLFire },
	"CLGlory": func(c *Models.CommStatModel) float64 { return c.CLGlory },

	// Combined stats (frontend uses these)
	"glory": func(c *Models.CommStatModel) float64 { return c.CLGlory + c.NPCGlory },
	"later": func(c *Models.CommStatModel) float64 { return c.CLLater },
	"fire":  func(c *Models.CommStatModel) float64 { return c.CLFire },
}

// CastStatGetter maps stat names to functions that retrieve the value from CastStatModel
var CastStatGetter = map[string]func(*Models.CastStatModel) float64{
	// Base Stats
	"meleeCbtStr":   func(c *Models.CastStatModel) float64 { return c.MeleeCbtStr },
	"rangeCbtStr":   func(c *Models.CastStatModel) float64 { return c.RangeCbtStr },
	"opCbtStr":      func(c *Models.CastStatModel) float64 { return c.OpCbtStr },
	"mainCbtStr":    func(c *Models.CastStatModel) float64 { return c.MainCbtStr },
	"cyCbtStr":      func(c *Models.CastStatModel) float64 { return c.CyCbtStr },
	"allCbtStr":     func(c *Models.CastStatModel) float64 { return c.AllCbtStr },
	"frontCbtStr":   func(c *Models.CastStatModel) float64 { return c.FrontCbtStr },
	"flankCbtStr":   func(c *Models.CastStatModel) float64 { return c.FlankCbtStr },
	"wallStr":       func(c *Models.CastStatModel) float64 { return c.WallStr },
	"gateStr":       func(c *Models.CastStatModel) float64 { return c.GateStr },
	"moatStr":       func(c *Models.CastStatModel) float64 { return c.MoatStr },
	"wallLimit":     func(c *Models.CastStatModel) float64 { return c.WallLimit },
	"protectorSupp": func(c *Models.CastStatModel) float64 { return c.ProtectorSupp },
	"lootStr":       func(c *Models.CastStatModel) float64 { return c.Loot },

	// Economy Stats
	"recruit":      func(c *Models.CastStatModel) float64 { return c.Recruit },
	"meadProd":     func(c *Models.CastStatModel) float64 { return c.MeadProd },
	"research":     func(c *Models.CastStatModel) float64 { return c.Research },
	"hospital":     func(c *Models.CastStatModel) float64 { return c.Hospital },
	"construction": func(c *Models.CastStatModel) float64 { return c.Construction },
	"baseRes":      func(c *Models.CastStatModel) float64 { return c.BaseRes },
	"kingRes":      func(c *Models.CastStatModel) float64 { return c.KingRes },
	"po":           func(c *Models.CastStatModel) float64 { return c.PO },
	"resTransport": func(c *Models.CastStatModel) float64 { return c.ResTransport },
	"honeyProd":    func(c *Models.CastStatModel) float64 { return c.HoneyProd },
	"meadStorage":  func(c *Models.CastStatModel) float64 { return c.MeadStorage },
	"honeyStorage": func(c *Models.CastStatModel) float64 { return c.HoneyStorage },

	// NPC Stats
	"NPCMelee":     func(c *Models.CastStatModel) float64 { return c.NPCMelee },
	"NPCRange":     func(c *Models.CastStatModel) float64 { return c.NPCRange },
	"NPCFront":     func(c *Models.CastStatModel) float64 { return c.NPCFront },
	"NPCFlank":     func(c *Models.CastStatModel) float64 { return c.NPCFlank },
	"NPCCy":        func(c *Models.CastStatModel) float64 { return c.NPCCy },
	"NPCWall":      func(c *Models.CastStatModel) float64 { return c.NPCWall },
	"NPCGate":      func(c *Models.CastStatModel) float64 { return c.NPCGate },
	"NPCMoat":      func(c *Models.CastStatModel) float64 { return c.NPCMoat },
	"NPCWallLimit": func(c *Models.CastStatModel) float64 { return c.NPCWallLimit },

	// CL/PvP Stats
	"CLMelee":     func(c *Models.CastStatModel) float64 { return c.CLMelee },
	"CLRange":     func(c *Models.CastStatModel) float64 { return c.CLRange },
	"CLCy":        func(c *Models.CastStatModel) float64 { return c.CLCy },
	"CLWall":      func(c *Models.CastStatModel) float64 { return c.CLWall },
	"CLGate":      func(c *Models.CastStatModel) float64 { return c.CLGate },
	"CLMoat":      func(c *Models.CastStatModel) float64 { return c.CLMoat },
	"CLWallLimit": func(c *Models.CastStatModel) float64 { return c.CLWallLimit },
	"CLFire":      func(c *Models.CastStatModel) float64 { return c.CLFire },
	"CLGlory":     func(c *Models.CastStatModel) float64 { return c.CLGlory },
	"CLEarly":     func(c *Models.CastStatModel) float64 { return c.CLEarly },

	// Combined stats (frontend uses these)
	"glory": func(c *Models.CastStatModel) float64 { return c.CLGlory },
	"fire":  func(c *Models.CastStatModel) float64 { return c.CLFire },
	"early": func(c *Models.CastStatModel) float64 { return c.CLEarly },
}

// CombatModeStatMap maps base stat names to their CL (PvP) and NPC (PvE) counterparts
// Some stats don't have combat-specific versions (empty string means no counterpart)
var CombatModeStatMap = map[string]struct{ CL, NPC string }{
	"meleeCbtStr": {"CLMelee", "NPCMelee"},
	"rangeCbtStr": {"CLRange", "NPCRange"},
	"frontLimit":  {"CLFront", "NPCFront"},
	"flankLimit":  {"CLFlank", "NPCFlank"},
	"cyCbtStr":    {"CLCy", "NPCCy"},
	"wallStr":     {"CLWall", "NPCWall"},
	"gateStr":     {"CLGate", "NPCGate"},
	"moatStr":     {"CLMoat", "NPCMoat"},
	"wallLimit":   {"CLWallLimit", "NPCWallLimit"},
}

// getCombatStatName returns the combat-mode specific stat name for a base stat
// combatMode: "PVP" or "NPC"
func getCombatStatName(baseStat string, combatMode string) string {
	mapping, exists := CombatModeStatMap[baseStat]
	if !exists {
		return "" // No combat-specific version
	}
	if combatMode == "PVP" {
		return mapping.CL
	}
	return mapping.NPC
}

// calculateWeight returns the weight for a given position using cumulative decay
// weight = baseScore * (1 - decay)^position
func calculateWeight(baseScore float64, decay float64, position int) float64 {
	return baseScore * math.Pow(1-decay, float64(position))
}

// ScoreCommander calculates a heuristic score for a CommStatModel based on priorities.
// Tier1: 10000 base, 10% cumulative decay per position
// Tier2: 100 base, 5% cumulative decay per position
// Combat mode determines whether CL or NPC stats are scored alongside base stats
func ScoreCommander(stats *Models.CommStatModel, priorities PreparedPriority, combatMode string) float64 {
	var score float64

	// Score Tier1 stats (priority stats)
	for i, priorityStat := range priorities.Tier1Stats {
		weight := calculateWeight(Tier1BaseScore, Tier1Decay, i)

		// Score the base stat
		if getter, exists := CommStatGetter[priorityStat.StatName]; exists {
			score += getter(stats) * weight
		}

		// Also score the combat-mode specific stat at same priority
		combatStatName := getCombatStatName(priorityStat.StatName, combatMode)
		if combatStatName != "" {
			if getter, exists := CommStatGetter[combatStatName]; exists {
				score += getter(stats) * weight
			}
		}
	}

	// Score Tier2 stats (optimize stats) with position-based decay
	for i, priorityStat := range priorities.Tier2Stats {
		weight := calculateWeight(Tier2BaseScore, Tier2Decay, i)

		// Score the base stat
		if getter, exists := CommStatGetter[priorityStat.StatName]; exists {
			score += getter(stats) * weight
		}

		// Also score the combat-mode specific stat at same priority
		combatStatName := getCombatStatName(priorityStat.StatName, combatMode)
		if combatStatName != "" {
			if getter, exists := CommStatGetter[combatStatName]; exists {
				score += getter(stats) * weight
			}
		}
	}

	return score
}

// ScoreCastellan calculates a heuristic score for a CastStatModel based on priorities.
// Tier1: 10000 base, 10% cumulative decay per position
// Tier2: 100 base, 5% cumulative decay per position
// Combat mode determines whether CL or NPC stats are scored alongside base stats
func ScoreCastellan(stats *Models.CastStatModel, priorities PreparedPriority, combatMode string) float64 {
	var score float64

	// Score Tier1 stats (priority stats)
	for i, priorityStat := range priorities.Tier1Stats {
		weight := calculateWeight(Tier1BaseScore, Tier1Decay, i)

		// Score the base stat
		if getter, exists := CastStatGetter[priorityStat.StatName]; exists {
			score += getter(stats) * weight
		}

		// Also score the combat-mode specific stat at same priority
		combatStatName := getCombatStatName(priorityStat.StatName, combatMode)
		if combatStatName != "" {
			if getter, exists := CastStatGetter[combatStatName]; exists {
				score += getter(stats) * weight
			}
		}
	}

	// Score Tier2 stats (optimize stats) with position-based decay
	for i, priorityStat := range priorities.Tier2Stats {
		weight := calculateWeight(Tier2BaseScore, Tier2Decay, i)

		// Score the base stat
		if getter, exists := CastStatGetter[priorityStat.StatName]; exists {
			score += getter(stats) * weight
		}

		// Also score the combat-mode specific stat at same priority
		combatStatName := getCombatStatName(priorityStat.StatName, combatMode)
		if combatStatName != "" {
			if getter, exists := CastStatGetter[combatStatName]; exists {
				score += getter(stats) * weight
			}
		}
	}

	return score
}

// ScoreEquipment scores an individual equipment piece based on its stat IDs and values.
// It maps the equipment's stat IDs to priority weights and sums weighted values.
// Tier1: 10000 base, 10% decay. Tier2: 100 base, 5% decay.
// If includeTier1 is false, only Tier2 stats are scored (for diversity in candidate selection).
func ScoreEquipment(equip Models.EquipmentModel, priorities PreparedPriority, includeTier1 bool) float64 {
	var score float64

	// Build a map from stat ID to weight for fast lookup
	statWeights := make(map[float64]float64)

	// Add Tier1 stat weights if included
	if includeTier1 {
		for i, priorityStat := range priorities.Tier1Stats {
			weight := calculateWeight(Tier1BaseScore, Tier1Decay, i)
			for _, statID := range priorityStat.StatIDs {
				statWeights[statID] = weight
			}
		}
	}

	// Add Tier2 stat weights with position-based decay
	for i, priorityStat := range priorities.Tier2Stats {
		weight := calculateWeight(Tier2BaseScore, Tier2Decay, i)
		for _, statID := range priorityStat.StatIDs {
			// Only add if not already covered by Tier1 (Tier1 takes precedence)
			if _, exists := statWeights[statID]; !exists {
				statWeights[statID] = weight
			}
		}
	}

	// Score each stat on the equipment
	for _, stat := range equip.EquipStats {
		if weight, exists := statWeights[stat.ID]; exists {
			// Use the first value in the Value array (typical case)
			if len(stat.Value) > 0 {
				score += stat.Value[0] * weight
			}
		}
	}

	return score
}

// ScoredEquipment pairs equipment with its score for sorting
type ScoredEquipment struct {
	Equipment Models.EquipmentModel
	Score     float64
}

// SelectTopEquipment selects top candidates from an equipment array using 2 parallel goroutines:
// Goroutine 1: Score full inventory, pick top 5
// Goroutine 2: Prune equipment containing ANY Tier1 stat IDs, score remainder, pick top 5
// Returns up to 10 items total for diverse optimization candidates.
func SelectTopEquipment(equipment []Models.EquipmentModel, priorities PreparedPriority) []Models.EquipmentModel {
	if len(equipment) <= 10 {
		return equipment // No need to filter if already small enough
	}

	// Build set of all Tier1 stat IDs for pruning check
	tier1StatIDs := make(map[float64]bool)
	for _, priorityStat := range priorities.Tier1Stats {
		for _, statID := range priorityStat.StatIDs {
			tier1StatIDs[statID] = true
		}
	}

	// Result collection with mutex for thread safety
	var mu sync.Mutex
	selected := make([]Models.EquipmentModel, 0, 10)
	selectedIDs := make(map[float64]bool)

	var wg sync.WaitGroup
	wg.Add(2)

	// === GOROUTINE 1: Score full inventory, pick top 5 ===
	go func() {
		defer wg.Done()

		scored := make([]ScoredEquipment, len(equipment))
		for i, equip := range equipment {
			scored[i] = ScoredEquipment{
				Equipment: equip,
				Score:     ScoreEquipment(equip, priorities, true),
			}
		}

		// Sort by score descending
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].Score > scored[j].Score
		})

		// Pick top 5, add to shared result
		mu.Lock()
		for i := 0; i < len(scored) && countSelected(selected) < 5; i++ {
			id := scored[i].Equipment.ID
			if !selectedIDs[id] {
				selectedIDs[id] = true
				selected = append(selected, scored[i].Equipment)
			}
		}
		mu.Unlock()
	}()

	// === GOROUTINE 2: Prune equipment with Tier1 stats, score remainder, pick top 5 ===
	go func() {
		defer wg.Done()

		// Prune: only keep equipment that has NO Tier1 stat IDs
		pruned := make([]Models.EquipmentModel, 0)
		for _, equip := range equipment {
			hasTier1Stat := false
			for _, stat := range equip.EquipStats {
				if tier1StatIDs[stat.ID] {
					hasTier1Stat = true
					break
				}
			}
			if !hasTier1Stat {
				pruned = append(pruned, equip)
			}
		}

		// Score pruned inventory
		scored := make([]ScoredEquipment, len(pruned))
		for i, equip := range pruned {
			scored[i] = ScoredEquipment{
				Equipment: equip,
				Score:     ScoreEquipment(equip, priorities, true),
			}
		}

		// Sort by score descending
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].Score > scored[j].Score
		})

		// Pick top 5 from pruned, add to shared result (dedup)
		mu.Lock()
		added := 0
		for i := 0; i < len(scored) && added < 5; i++ {
			id := scored[i].Equipment.ID
			if !selectedIDs[id] {
				selectedIDs[id] = true
				selected = append(selected, scored[i].Equipment)
				added++
			}
		}
		mu.Unlock()
	}()

	wg.Wait()
	return selected
}

// countSelected is a helper to count items in selected slice (needed because we check inside goroutine)
func countSelected(s []Models.EquipmentModel) int {
	return len(s)
}

// CheckCommCeilings checks all Commander stats against their ceilings and returns capped stat IDs
func CheckCommCeilings(stats *Models.CommStatModel, ceiling *Models.CommStatModel) map[float64]bool {
	capped := make(map[float64]bool)

	markCapped := func(statName string) {
		for _, id := range CommStatNameToIDs[statName] {
			capped[id] = true
		}
	}

	// Base stats
	if ceiling.MeleeCbtStr > 0 && stats.MeleeCbtStr >= ceiling.MeleeCbtStr {
		markCapped("meleeCbtStr")
	}
	if ceiling.RangeCbtStr > 0 && stats.RangeCbtStr >= ceiling.RangeCbtStr {
		markCapped("rangeCbtStr")
	}
	if ceiling.FrontCbtStr > 0 && stats.FrontCbtStr >= ceiling.FrontCbtStr {
		markCapped("frontCbtStr")
	}
	if ceiling.FlankCbtStr > 0 && stats.FlankCbtStr >= ceiling.FlankCbtStr {
		markCapped("flankCbtStr")
	}
	if ceiling.AllCbtStr > 0 && stats.AllCbtStr >= ceiling.AllCbtStr {
		markCapped("allCbtStr")
	}
	if ceiling.CyCbtStr > 0 && stats.CyCbtStr >= ceiling.CyCbtStr {
		markCapped("cyCbtStr")
	}
	if ceiling.WallStr > 0 && stats.WallStr >= ceiling.WallStr {
		markCapped("wallStr")
	}
	if ceiling.GateStr > 0 && stats.GateStr >= ceiling.GateStr {
		markCapped("gateStr")
	}
	if ceiling.MoatStr > 0 && stats.MoatStr >= ceiling.MoatStr {
		markCapped("moatStr")
	}
	if ceiling.FlankLimit > 0 && stats.FlankLimit >= ceiling.FlankLimit {
		markCapped("flankLimit")
	}
	if ceiling.FrontLimit > 0 && stats.FrontLimit >= ceiling.FrontLimit {
		markCapped("frontLimit")
	}
	if ceiling.MaidenSupp > 0 && stats.MaidenSupp >= ceiling.MaidenSupp {
		markCapped("maidenSupp")
	}
	if ceiling.Travel > 0 && stats.Travel >= ceiling.Travel {
		markCapped("travel")
	}
	if ceiling.Loot > 0 && stats.Loot >= ceiling.Loot {
		markCapped("loot")
	}

	// NPC stats
	if ceiling.NPCMelee > 0 && stats.NPCMelee >= ceiling.NPCMelee {
		markCapped("NPCMelee")
	}
	if ceiling.NPCRange > 0 && stats.NPCRange >= ceiling.NPCRange {
		markCapped("NPCRange")
	}
	if ceiling.NPCFront > 0 && stats.NPCFront >= ceiling.NPCFront {
		markCapped("NPCFront")
	}
	if ceiling.NPCFlank > 0 && stats.NPCFlank >= ceiling.NPCFlank {
		markCapped("NPCFlank")
	}
	if ceiling.NPCCy > 0 && stats.NPCCy >= ceiling.NPCCy {
		markCapped("NPCCy")
	}
	if ceiling.NPCWall > 0 && stats.NPCWall >= ceiling.NPCWall {
		markCapped("NPCWall")
	}
	if ceiling.NPCGate > 0 && stats.NPCGate >= ceiling.NPCGate {
		markCapped("NPCGate")
	}
	if ceiling.NPCMoat > 0 && stats.NPCMoat >= ceiling.NPCMoat {
		markCapped("NPCMoat")
	}
	if ceiling.NPCGlory > 0 && stats.NPCGlory >= ceiling.NPCGlory {
		markCapped("NPCGlory")
	}

	// CL stats
	if ceiling.CLMelee > 0 && stats.CLMelee >= ceiling.CLMelee {
		markCapped("CLMelee")
	}
	if ceiling.CLRange > 0 && stats.CLRange >= ceiling.CLRange {
		markCapped("CLRange")
	}
	if ceiling.CLFront > 0 && stats.CLFront >= ceiling.CLFront {
		markCapped("CLFront")
	}
	if ceiling.CLFlank > 0 && stats.CLFlank >= ceiling.CLFlank {
		markCapped("CLFlank")
	}
	if ceiling.CLCy > 0 && stats.CLCy >= ceiling.CLCy {
		markCapped("CLCy")
	}
	if ceiling.CLWall > 0 && stats.CLWall >= ceiling.CLWall {
		markCapped("CLWall")
	}
	if ceiling.CLGate > 0 && stats.CLGate >= ceiling.CLGate {
		markCapped("CLGate")
	}
	if ceiling.CLMoat > 0 && stats.CLMoat >= ceiling.CLMoat {
		markCapped("CLMoat")
	}
	if ceiling.CLLater > 0 && stats.CLLater >= ceiling.CLLater {
		markCapped("CLLater")
	}
	if ceiling.CLFire > 0 && stats.CLFire >= ceiling.CLFire {
		markCapped("CLFire")
	}
	if ceiling.CLGlory > 0 && stats.CLGlory >= ceiling.CLGlory {
		markCapped("CLGlory")
	}

	return capped
}

// CheckCastCeilings checks all Castellan stats against their ceilings and returns capped stat IDs
func CheckCastCeilings(stats *Models.CastStatModel, ceiling *Models.CastStatModel) map[float64]bool {
	capped := make(map[float64]bool)

	markCapped := func(statName string) {
		for _, id := range CastStatNameToIDs[statName] {
			capped[id] = true
		}
	}

	// Base stats
	if ceiling.MeleeCbtStr > 0 && stats.MeleeCbtStr >= ceiling.MeleeCbtStr {
		markCapped("meleeCbtStr")
	}
	if ceiling.RangeCbtStr > 0 && stats.RangeCbtStr >= ceiling.RangeCbtStr {
		markCapped("rangeCbtStr")
	}
	if ceiling.WallStr > 0 && stats.WallStr >= ceiling.WallStr {
		markCapped("wallStr")
	}
	if ceiling.GateStr > 0 && stats.GateStr >= ceiling.GateStr {
		markCapped("gateStr")
	}
	if ceiling.MoatStr > 0 && stats.MoatStr >= ceiling.MoatStr {
		markCapped("moatStr")
	}
	if ceiling.WallLimit > 0 && stats.WallLimit >= ceiling.WallLimit {
		markCapped("wallLimit")
	}
	if ceiling.CyCbtStr > 0 && stats.CyCbtStr >= ceiling.CyCbtStr {
		markCapped("cyCbtStr")
	}
	if ceiling.Loot > 0 && stats.Loot >= ceiling.Loot {
		markCapped("lootStr")
	}
	if ceiling.ProtectorSupp > 0 && stats.ProtectorSupp >= ceiling.ProtectorSupp {
		markCapped("protectorSupp")
	}

	// NPC stats
	if ceiling.NPCMelee > 0 && stats.NPCMelee >= ceiling.NPCMelee {
		markCapped("NPCMelee")
	}
	if ceiling.NPCRange > 0 && stats.NPCRange >= ceiling.NPCRange {
		markCapped("NPCRange")
	}
	if ceiling.NPCWall > 0 && stats.NPCWall >= ceiling.NPCWall {
		markCapped("NPCWall")
	}
	if ceiling.NPCGate > 0 && stats.NPCGate >= ceiling.NPCGate {
		markCapped("NPCGate")
	}
	if ceiling.NPCMoat > 0 && stats.NPCMoat >= ceiling.NPCMoat {
		markCapped("NPCMoat")
	}
	if ceiling.NPCCy > 0 && stats.NPCCy >= ceiling.NPCCy {
		markCapped("NPCCy")
	}
	if ceiling.NPCWallLimit > 0 && stats.NPCWallLimit >= ceiling.NPCWallLimit {
		markCapped("NPCWallLimit")
	}

	// CL stats
	if ceiling.CLMelee > 0 && stats.CLMelee >= ceiling.CLMelee {
		markCapped("CLMelee")
	}
	if ceiling.CLRange > 0 && stats.CLRange >= ceiling.CLRange {
		markCapped("CLRange")
	}
	if ceiling.CLWall > 0 && stats.CLWall >= ceiling.CLWall {
		markCapped("CLWall")
	}
	if ceiling.CLGate > 0 && stats.CLGate >= ceiling.CLGate {
		markCapped("CLGate")
	}
	if ceiling.CLMoat > 0 && stats.CLMoat >= ceiling.CLMoat {
		markCapped("CLMoat")
	}
	if ceiling.CLCy > 0 && stats.CLCy >= ceiling.CLCy {
		markCapped("CLCy")
	}
	if ceiling.CLWallLimit > 0 && stats.CLWallLimit >= ceiling.CLWallLimit {
		markCapped("CLWallLimit")
	}
	if ceiling.CLFire > 0 && stats.CLFire >= ceiling.CLFire {
		markCapped("CLFire")
	}
	if ceiling.CLGlory > 0 && stats.CLGlory >= ceiling.CLGlory {
		markCapped("CLGlory")
	}
	if ceiling.CLEarly > 0 && stats.CLEarly >= ceiling.CLEarly {
		markCapped("CLEarly")
	}

	return capped
}
