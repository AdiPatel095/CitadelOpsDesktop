package LoadoutReconfigure

import (
	"CitadelDesktop/Server/Models"
	"math"
	"sync"
)

// RunOptimization is the main entry point
func RunOptimization(payload ReconfigurePayload, equipment []Models.EquipmentModel, gemsList []Models.Gem) Models.CommStatModel {
	// 1. Parse Profile
	profile := ParseProfile(payload)

	// 2. Prepare Inventory (Unsocket and Split)
	baseItems, gems := PrepareInventory(equipment, gemsList)

	// 3. Prune Inventory (Pareto)
	// Important: We prune Base and Gems separately BEFORE combination
	baseItems = PruneInventory(baseItems, profile)
	gems = PruneInventory(gems, profile)

	if len(baseItems) == 0 {
		return Models.CommStatModel{}
	}

	// 4. Calculate "Max Gem Potential" per Slot (for Heuristic)
	// We need this to value empty slots in Stage 1
	maxGemVal := GetMaxGemPotential(gems, profile)

	// 5. Stage 1: Solve Equipment and Hero in PARALLEL (they are independent)
	var wg sync.WaitGroup
	var bestBaseSet []CandidateItem
	var bestHero CandidateItem

	// 5a. Goroutine for Equipment (4 slots)
	wg.Add(1)
	go func() {
		defer wg.Done()
		bestBaseSet, _ = SolveBaseEquipment(baseItems, profile, maxGemVal)
	}()

	// 5b. Goroutine for Hero (independent)
	wg.Add(1)
	go func() {
		defer wg.Done()
		heroes := GetItemsBySlot(baseItems, 6)
		bestHero = SolveHero(heroes, profile)
	}()

	// Wait for both to complete
	wg.Wait()

	if len(bestBaseSet) == 0 {
		return Models.CommStatModel{}
	}

	// Combine equipment + hero
	if bestHero.OriginalID != 0 {
		bestBaseSet = append(bestBaseSet, bestHero)
	}

	// 6. Stage 2: Solve Gems (Greedy/Knapsack to fill gaps) - AFTER equipment is determined
	finalSet := SolveGems(bestBaseSet, gems, profile)

	// 7. Convert Result
	return ConstructResult(finalSet, equipment, gemsList)
}

// SolveBaseEquipment runs Branch & Bound on Base Items
// SolveBaseEquipment runs Branch & Bound on Base Items
func SolveBaseEquipment(baseItems []CandidateItem, profile OptimizationProfile, maxGemVal float64) ([]CandidateItem, float64) {
	// Helper to ensure we have at least one "Empty" item if list is empty
	// This prevents the loop from doing 0 iterations
	ensureNonEmpty := func(items []CandidateItem, slot float64) []CandidateItem {
		if len(items) > 0 {
			return items
		}
		// Return a single empty item placeholder
		return []CandidateItem{{
			OriginalID: 0,
			Slot:       slot,
			Stats:      make(map[string]float64),
			HasGemSlot: false, // Placeholder has no gem slot
		}}
	}

	// Equipment slots only (no heroes - they are solved separately)
	armors := ensureNonEmpty(GetItemsBySlot(baseItems, 1), 1)
	weapons := ensureNonEmpty(GetItemsBySlot(baseItems, 2), 2)
	helmets := ensureNonEmpty(GetItemsBySlot(baseItems, 3), 3)
	artifacts := ensureNonEmpty(GetItemsBySlot(baseItems, 4), 4)

	// Pre-calc max potentials (including gem slots!)
	// For potentials, we assume EVERY future item works perfectly
	calcMaxWithGems := func(items []CandidateItem, profile OptimizationProfile, maxGemVal float64) float64 {
		var max float64
		for _, i := range items {
			s := 0.0
			for stat, val := range i.Stats {
				mult := 1.0
				for _, t0 := range profile.Tier0Stats {
					if t0 == stat {
						mult = 1000000
						break
					}
				}
				if mult == 1.0 {
					for _, t1 := range profile.Tier1Stats {
						if t1 == stat {
							mult = profile.InterTierMultiplier
							break
						}
					}
				}
				s += val * mult
			}
			if i.HasGemSlot {
				s += maxGemVal
			}
			if s > max {
				max = s
			}
		}
		return max
	}

	maxWeapon := calcMaxWithGems(weapons, profile, maxGemVal)
	maxHelmet := calcMaxWithGems(helmets, profile, maxGemVal)
	maxArtifact := calcMaxWithGems(artifacts, profile, maxGemVal)

	var bestSet []CandidateItem
	var bestScore float64 = -1

	// Internal Heuristic Score for Equipment Only using individual scorers
	// Accumulates totals as items are added to properly calculate ceiling gaps
	scoreFn := func(items ...CandidateItem) float64 {
		var totalScore float64
		currentTotals := make(map[string]float64)

		for _, item := range items {
			// Score this item based on current totals (ceiling-aware)
			itemScore := ScoreEquipmentItem(item, profile, currentTotals)
			totalScore += itemScore

			// Update current totals with this item's stats
			for statName, value := range item.Stats {
				currentTotals[statName] += value
			}
		}

		// Add "Potential" for gem slots
		potential := 0.0
		for _, i := range items {
			if i.HasGemSlot {
				potential += maxGemVal
			}
		}
		return totalScore + potential
	}

	canBeatBest := func(currentScore float64, remainingMax float64) bool {
		return (currentScore + remainingMax) > bestScore
	}

	// 4-Nested Loop for Equipment Only (heroes solved separately)
	for _, armor := range armors {
		if !canBeatBest(scoreFn(armor), maxWeapon+maxHelmet+maxArtifact) {
			continue
		}
		for _, weapon := range weapons {
			if !canBeatBest(scoreFn(armor, weapon), maxHelmet+maxArtifact) {
				continue
			}
			for _, helmet := range helmets {
				if !canBeatBest(scoreFn(armor, weapon, helmet), maxArtifact) {
					continue
				}
				for _, artifact := range artifacts {
					finalS := scoreFn(armor, weapon, helmet, artifact)
					if finalS > bestScore {
						bestScore = finalS
						bestSet = []CandidateItem{armor, weapon, helmet, artifact}
					}
				}
			}
		}
	}

	return bestSet, bestScore
}

// SolveHero picks the best hero based on independent scoring (no synergy with equipment)
func SolveHero(heroes []CandidateItem, profile OptimizationProfile) CandidateItem {
	var bestHero CandidateItem
	bestScore := -1.0

	for _, hero := range heroes {
		// Skip empty placeholder heroes
		if hero.OriginalID == 0 {
			continue
		}

		// Score hero independently using hero-only ceilings
		score := ScoreHeroOnly(hero, profile)
		if score > bestScore {
			bestScore = score
			bestHero = hero
		}
	}

	return bestHero
}

// ScoreHeroOnly calculates score for a hero based purely on StatEfficiency * Multiplier
func ScoreHeroOnly(hero CandidateItem, profile OptimizationProfile) float64 {
	var score float64

	// Apply positional decay
	decayBase := 1.0 - (profile.InterTierMultiplier * 0.02)
	if decayBase < 0.01 {
		decayBase = 0.01
	}
	if decayBase > 0.99 {
		decayBase = 0.99
	}

	calcTier := func(stats []string, baseMultiplier float64, tierName string) float64 {
		var tierScore float64
		for i, stat := range stats {
			decay := math.Pow(decayBase, float64(i))

			// User requirement: "heroes score should be based purely with statEff * base score * multiplier"
			// We use the raw efficiency from the game data (0-100)
			rawEfficiency := hero.StatEfficiency[stat]
			if rawEfficiency == 0 {
				// Fallback if efficiency missing: calculate from Universal Max
				val := hero.Stats[stat]
				max := StatMaxValues[stat]
				if max > 0 {
					rawEfficiency = (val / max) * 100.0
				} else {
					rawEfficiency = 50.0 // Penalty default
				}
			}

			// Score = Efficiency * Multiplier * Decay
			// We treat Efficiency as the primary scorer (e.g. 100% = 100 points * multiplier)
			contrib := rawEfficiency * baseMultiplier * decay

			tierScore += contrib
		}
		return tierScore
	}

	score += calcTier(profile.Tier0Stats, 1000000, "T0")
	score += calcTier(profile.Tier1Stats, profile.IntraTierMultiplier, "T1")
	score += calcTier(profile.Tier2Stats, 1.0, "T2")

	return score
}

// ScoreEquipmentItem calculates score using "Gap-Aware" Efficiency
// 1. Reverse calc MaxPossible = Value / (RawEff/100)
// 2. EffectiveContrib = min(Value, Gap)
// 3. EffectiveEff = (EffectiveContrib / MaxPossible) * 100
func ScoreEquipmentItem(item CandidateItem, profile OptimizationProfile, currentTotals map[string]float64) float64 {
	var score float64

	decayBase := 1.0 - (profile.InterTierMultiplier * 0.02)
	if decayBase < 0.01 {
		decayBase = 0.01
	}
	if decayBase > 0.99 {
		decayBase = 0.99
	}

	calcTier := func(stats []string, baseMultiplier float64) float64 {
		var tierScore float64
		for i, stat := range stats {
			decay := math.Pow(decayBase, float64(i))

			statValue := item.Stats[stat]
			if statValue == 0 {
				continue
			}

			// 1. Get/Calculate Raw Efficiency
			rawEfficiency := item.StatEfficiency[stat]
			if rawEfficiency == 0 {
				max := StatMaxValues[stat]
				if max > 0 {
					rawEfficiency = (statValue / max) * 100.0
				} else {
					rawEfficiency = 50.0
				}
			}

			// 2. Reverse Calculate Max Possible for this specific item's roll
			// e.g. Val=15, Eff=75% -> MaxPossible=20
			maxPossible := 0.0
			if rawEfficiency > 0 {
				maxPossible = statValue / (rawEfficiency / 100.0)
			}

			// 3. Calculate Ceiling Gap
			currentVal := currentTotals[stat]
			ceiling := float64(999999)
			if cap, ok := profile.StatCeilings[stat]; ok && cap > 0 {
				ceiling = cap
			}
			ceilingGap := ceiling - currentVal
			if ceilingGap < 0 {
				ceilingGap = 0
			}

			// 4. Calculate Effective Contribution (what we can actually add)
			effectiveContrib := math.Min(statValue, ceilingGap)

			// 5. Calculate Effective Efficiency Score
			// e.g. Contrib=10, MaxPossible=20 -> Eff=50%
			effectiveEfficiency := 0.0
			if maxPossible > 0 {
				effectiveEfficiency = (effectiveContrib / maxPossible) * 100.0
			}

			// Score
			tierScore += effectiveEfficiency * baseMultiplier * decay
		}
		return tierScore
	}

	score += calcTier(profile.Tier0Stats, 1000000)
	score += calcTier(profile.Tier1Stats, profile.IntraTierMultiplier)
	score += calcTier(profile.Tier2Stats, 1.0)

	return score
}

// ScoreGemItem calculates score using "Gap-Aware" Efficiency (Same logic as Equipment)
func ScoreGemItem(gem CandidateItem, profile OptimizationProfile, currentTotals map[string]float64) float64 {
	var score float64

	// Apply positional decay
	decayBase := 1.0 - (profile.InterTierMultiplier * 0.02)
	if decayBase < 0.01 {
		decayBase = 0.01
	}
	if decayBase > 0.99 {
		decayBase = 0.99
	}

	calcTier := func(stats []string, baseMultiplier float64) float64 {
		var tierScore float64
		for i, stat := range stats {
			decay := math.Pow(decayBase, float64(i))

			statValue := gem.Stats[stat]
			if statValue == 0 {
				continue
			}

			// 1. Get/Calculate Raw Efficiency
			rawEfficiency := gem.StatEfficiency[stat]
			if rawEfficiency == 0 {
				max := StatMaxValues[stat]
				if max > 0 {
					rawEfficiency = (statValue / max) * 100.0
				} else {
					rawEfficiency = 50.0
				}
			}

			// 2. Reverse Calculate Max Possible
			maxPossible := 0.0
			if rawEfficiency > 0 {
				maxPossible = statValue / (rawEfficiency / 100.0)
			}

			// 3. Calculate Ceiling Gap
			currentVal := currentTotals[stat]
			ceiling := float64(999999)
			if cap, ok := profile.StatCeilings[stat]; ok && cap > 0 {
				ceiling = cap
			}
			ceilingGap := ceiling - currentVal
			if ceilingGap < 0 {
				ceilingGap = 0
			}

			// 4. Calculate Effective Contribution
			effectiveContrib := math.Min(statValue, ceilingGap)

			// 5. Calculate Effective Efficiency Score
			effectiveEfficiency := 0.0
			if maxPossible > 0 {
				effectiveEfficiency = (effectiveContrib / maxPossible) * 100.0
			}

			// Score
			tierScore += effectiveEfficiency * baseMultiplier * decay
		}
		return tierScore
	}

	score += calcTier(profile.Tier0Stats, 1000000)
	score += calcTier(profile.Tier1Stats, profile.IntraTierMultiplier)
	score += calcTier(profile.Tier2Stats, 1.0)

	return score
}

// SolveGems picks the best gems to fill the gaps in the base set
func SolveGems(baseSet []CandidateItem, gems []CandidateItem, profile OptimizationProfile) []CandidateItem {
	finalSet := make([]CandidateItem, len(baseSet))
	copy(finalSet, baseSet)

	// Count slots that can hold gems
	slotsWithGems := 0
	for _, i := range baseSet {
		if i.HasGemSlot {
			slotsWithGems++
		}
	}

	if slotsWithGems == 0 {
		return finalSet
	}

	// Calculate current totals from base equipment
	getCurrentTotals := func(items []CandidateItem) map[string]float64 {
		totals := make(map[string]float64)
		for _, item := range items {
			for stat, val := range item.Stats {
				totals[stat] += val
			}
		}
		return totals
	}

	currentSet := append([]CandidateItem{}, baseSet...)

	// Greedy: Pick gem that yields highest score gain based on ceiling gaps
	for i := 0; i < slotsWithGems; i++ {
		bestGemIdx := -1
		bestScore := -1.0

		// Calculate current totals for ceiling gap calculation
		currentTotals := getCurrentTotals(currentSet)

		for gIdx, gem := range gems {
			// Score this gem based on current totals (ceiling-aware)
			gemScore := ScoreGemItem(gem, profile, currentTotals)

			if gemScore > bestScore {
				bestScore = gemScore
				bestGemIdx = gIdx
			}
		}

		if bestGemIdx != -1 {
			// Add Gem to set
			picked := gems[bestGemIdx]
			currentSet = append(currentSet, picked)

			// Remove from available
			gems = append(gems[:bestGemIdx], gems[bestGemIdx+1:]...)
		}
	}

	return currentSet
}

func GetMaxGemPotential(gems []CandidateItem, profile OptimizationProfile) float64 {
	// Find the single highest scoring gem for the current profile
	// Used as a heuristic upper bound for an empty slot
	max := 0.0
	emptyTotals := make(map[string]float64) // Assume no stats capped yet for max potential

	for _, g := range gems {
		// Calculate score of just this gem
		s := ScoreGemItem(g, profile, emptyTotals)
		if s > max {
			max = s
		}
	}
	return max
}

// GetMaxPotential calculates the maximum possible score for a single item from a list.
// Used for Branch & Bound pruning.
func GetMaxPotential(items []CandidateItem, profile OptimizationProfile) float64 {
	max := 0.0
	emptyTotals := make(map[string]float64) // Assume no stats capped yet for max potential

	for _, item := range items {
		s := ScoreEquipmentItem(item, profile, emptyTotals)
		if s > max {
			max = s
		}
	}
	return max
}

// ... (Rest of ParseProfile, CalculateScore, PrepareInventory, Helpers similar to before) ...
// (Re-pasting// ParseProfile converts the payload into our optimized profile structure
func ParseProfile(payload ReconfigurePayload) OptimizationProfile {
	profile := OptimizationProfile{
		RelevanceMask:       make(map[string]bool),
		StatCeilings:        GetBaseCeilings(),
		HeroCeilings:        GetHeroCeilings(),
		InterTierMultiplier: payload.InterTierMultiplier,
		IntraTierMultiplier: payload.IntraTierMultiplier, // CRITICAL: Was missing!
	}

	// Alias Map: Maps Base Stat -> [List of Related Stats]
	// We select the related stats based on CombatMode (PVP vs NPC)
	// Default to PVP (Castle Lord) if unspecified
	isPVP := (payload.CombatMode != "NPC") // Assume PVP unless explicitly NPC

	// Helper to Normalize Input Stat Name (camelCase -> TitleCase)
	// Frontend sends "meleeCbtStr", Backend uses "MeleeCbtStr"
	normalizeStat := func(input string) string {
		switch input {
		// Base Stats
		case "meleeCbtStr", "MeleeCbtStr":
			return "MeleeCbtStr"
		case "rangeCbtStr", "RangeCbtStr":
			return "RangeCbtStr"
		case "wallStr", "WallStr":
			return "WallStr"
		case "gateStr", "GateStr":
			return "GateStr"
		case "moatStr", "MoatStr":
			return "MoatStr"
		case "travel", "Travel":
			return "Travel"
		case "loot", "Loot":
			return "Loot"
		case "flankLimit", "FlankLimit":
			return "FlankLimit"
		case "frontLimit", "FrontLimit":
			return "FrontLimit"
		case "cyCbtStr", "CyCbtStr":
			return "CyCbtStr"
		case "allCbtStr", "AllCbtStr":
			return "AllCbtStr"
		case "frontCbtStr", "FrontCbtStr":
			return "FrontCbtStr"
		case "flankCbtStr", "FlankCbtStr":
			return "FlankCbtStr"
		case "maidenSupp", "MaidenSupp":
			return "MaidenSupp"

		// Hero Bonus Stats
		case "wave", "Wave":
			return "Wave"
		case "cooldown", "Cooldown":
			return "Cooldown"
		case "eliteStr", "EliteStr":
			return "EliteStr"
		case "horrorStr", "HorrorStr":
			return "HorrorStr"
		case "beserkerStr", "BeserkerStr":
			return "BeserkerStr"
		case "relicStr", "RelicStr":
			return "RelicStr"
		case "meadStr", "MeadStr":
			return "MeadStr"

		// CL Stats (Hero/Gem)
		case "clMelee", "CLMelee":
			return "CLMelee"
		case "clRange", "CLRange":
			return "CLRange"
		case "clFront", "CLFront":
			return "CLFront"
		case "clFlank", "CLFlank":
			return "CLFlank"
		case "clCy", "CLCy":
			return "CLCy"
		case "clWall", "CLWall":
			return "CLWall"
		case "clGate", "CLGate":
			return "CLGate"
		case "clMoat", "CLMoat":
			return "CLMoat"
		case "clLater", "CLLater":
			return "CLLater"
		case "clFire", "CLFire":
			return "CLFire"
		case "clGlory", "CLGlory":
			return "CLGlory"

		// NPC Stats (Hero/Gem)
		case "npcMelee", "NPCMelee":
			return "NPCMelee"
		case "npcRange", "NPCRange":
			return "NPCRange"
		case "npcFront", "NPCFront":
			return "NPCFront"
		case "npcFlank", "NPCFlank":
			return "NPCFlank"
		case "npcCy", "NPCCy":
			return "NPCCy"
		case "npcWall", "NPCWall":
			return "NPCWall"
		case "npcGate", "NPCGate":
			return "NPCGate"
		case "npcMoat", "NPCMoat":
			return "NPCMoat"
		case "npcGlory", "NPCGlory":
			return "NPCGlory"

		default:
			return input // Fallback
		}
	}

	expandStat := func(baseStat string) []string {
		aliases := []string{baseStat} // Always include the base stat itself

		switch baseStat {
		case "MeleeCbtStr":
			if isPVP {
				aliases = append(aliases, "CLMelee")
			} else {
				aliases = append(aliases, "NPCMelee")
			}
		case "RangeCbtStr":
			if isPVP {
				aliases = append(aliases, "CLRange")
			} else {
				aliases = append(aliases, "NPCRange")
			}
		case "WallStr":
			if isPVP {
				aliases = append(aliases, "CLWall")
			} else {
				aliases = append(aliases, "NPCWall")
			}
		case "GateStr":
			if isPVP {
				aliases = append(aliases, "CLGate")
			} else {
				aliases = append(aliases, "NPCGate")
			}
		case "MoatStr":
			if isPVP {
				aliases = append(aliases, "CLMoat")
			} else {
				aliases = append(aliases, "NPCMoat")
			}
		case "CyCbtStr":
			if isPVP {
				aliases = append(aliases, "CLCy")
			} else {
				aliases = append(aliases, "NPCCy")
			}
		case "FrontCbtStr":
			if isPVP {
				aliases = append(aliases, "CLFront")
			} else {
				aliases = append(aliases, "NPCFront")
			}
		case "FlankCbtStr":
			if isPVP {
				aliases = append(aliases, "CLFlank")
			} else {
				aliases = append(aliases, "NPCFlank")
			}
		case "Loot":
			if !isPVP {
				aliases = append(aliases, "NPCGlory")
			} else {
				aliases = append(aliases, "CLGlory")
			}
		}
		return aliases
	}

	for _, stat := range payload.Stats {
		// NORMALIZE FIRST
		normStat := normalizeStat(stat.Stat)

		expanded := expandStat(normStat)

		for _, s := range expanded {
			if stat.Tier == 0 {
				profile.Tier0Stats = append(profile.Tier0Stats, s)
			} else if stat.Tier == 1 {
				profile.Tier1Stats = append(profile.Tier1Stats, s)
			} else if stat.Tier == 2 {
				profile.Tier2Stats = append(profile.Tier2Stats, s)
			}
			profile.RelevanceMask[s] = true
		}
	}
	return profile
}

func PrepareInventory(equipStorage []Models.EquipmentModel, gemStorage []Models.Gem) ([]CandidateItem, []CandidateItem) {
	var base []CandidateItem
	var gems []CandidateItem

	// 1. Process Embedded Gems & Equipment
	for _, equip := range equipStorage {
		// A. Extract Gem first (don't lose it if we discard the item)
		if equip.GemSlot.Gem != nil {
			cGem := ParseOnlyGem(*equip.GemSlot.Gem)
			gems = append(gems, cGem)
		}

		// B. Filter Base Item - Only Rarity 5 (Relic) or 15 (Set/Heroic)
		if equip.EquipRarity == 5 || equip.EquipRarity == 15 {
			cleanEq := ParseOnlyBase(equip)
			cleanEq.HasGemSlot = true
			base = append(base, cleanEq)
		}
	}

	// 2. Process Loose Gems
	for _, gem := range gemStorage {
		cGem := ParseOnlyGem(gem)
		gems = append(gems, cGem)
	}

	return base, gems
}

func GetItemsBySlot(inv []CandidateItem, slot float64) []CandidateItem {
	var res []CandidateItem
	for _, i := range inv {
		if i.Slot == slot {
			res = append(res, i)
		}
	}
	return res
}

func ConstructResult(items []CandidateItem, equipment []Models.EquipmentModel, gems []Models.Gem) Models.CommStatModel {
	// Initialize separate accumulators for component ceilings
	tempEquipStat := Models.CommStatModel{}
	tempHeroStat := Models.CommStatModel{}
	tempGemStat := Models.CommStatModel{}

	finalResult := Models.CommStatModel{}

	// Helper to apply stats to a specific model with a specific ceiling
	applyToModel := func(dst *Models.CommStatModel, ceiling *Models.CommStatModel, stats []Models.Stat) {
		for _, s := range stats {
			updater, ok := Models.CommStatUpdaterMap[s.ID]
			if ok {
				val := s.Value[0]
				if len(s.Value) > 1 {
					if (s.ID >= 20012 && s.ID <= 20017) || s.ID == 121 {
						val = s.Value[1]
					}
				}
				updater(dst, val)
			}
			// Important: Apply Ceiling continuously (per EquipmentParser logic)
			Models.ApplyCommCeiling(dst, ceiling)
		}
	}

	// 1. Process Base Items (Equip/Hero)
	// We iterate through input items to find base equipment
	for _, item := range items {
		if item.IsGem {
			continue
		} // Handled later

		// Find Original Equipment
		var foundEquip *Models.EquipmentModel
		for _, e := range equipment {
			if e.ID == item.OriginalID {
				foundEquip = &e
				break
			}
		}

		if foundEquip != nil {
			// Apply IDs to Result
			if item.Slot == 6 {
				finalResult.Hero = foundEquip.ID
				applyToModel(&tempHeroStat, &Models.CommHeroCeiling, foundEquip.EquipStats)
			} else {
				switch item.Slot {
				case 1:
					finalResult.Equip1 = foundEquip.ID
				case 2:
					finalResult.Equip2 = foundEquip.ID
				case 3:
					finalResult.Equip3 = foundEquip.ID
				case 4:
					finalResult.Equip4 = foundEquip.ID
				}
				applyToModel(&tempEquipStat, &Models.CommEquipCeiling, foundEquip.EquipStats)
			}

			// Process Logic for Embedded Gems (if we kept them?)
			// Logic: Our Optimizer separates Gems.
			// If we kept an original gem, it appears as a separate CandidateItem in 'items' list (via Pruning/SolveGems logic?)
			// Wait, SolveGems returns a list of items.
			// If we picked a loose gem, it's there.
			// If we picked an embedded gem, it ALSO appears there (we extracted it).
			// So we should NOT process foundEquip.GemSlot here for stats, because the gem is treated as a separate entity in our 'items' list.
			// However, for ID mappping, we might need to know if the gem we picked is indeed the one inside this equip.
		}
	}

	// 2. Process Gems
	// Logic: We need to assign valid Gems to Slots 1-4.
	// We have a list of chosen Gems. We assign them greedily to available slots.
	// Since gems are mostly universal (except some types), we just fill 1->4.
	gemSlotsValues := []float64{0, 0, 0, 0} // Gem IDs for Slots 1, 2, 3, 4
	gemIndex := 0

	for _, item := range items {
		if item.IsGem {
			// Find Original Gem
			var foundGem *Models.Gem
			for _, g := range gems {
				if g.ID == item.OriginalID {
					foundGem = &g
					break
				}
			}
			// Fallback: Check if it's an embedded gem inside equipment list (if not in loose gems)
			if foundGem == nil {
				for _, e := range equipment {
					if e.GemSlot.Gem != nil && e.GemSlot.Gem.ID == item.OriginalID {
						foundGem = e.GemSlot.Gem
						break
					}
				}
			}

			if foundGem != nil {
				applyToModel(&tempGemStat, &Models.CommGemCeiling, foundGem.GemStats)

				// Assign ID to next available slot (Simple Logic for now)
				if gemIndex < 4 {
					gemSlotsValues[gemIndex] = foundGem.ID
					gemIndex++
				}
			}
		}
	}

	// Assign Gem IDs to Result
	finalResult.Gem1 = gemSlotsValues[0]
	finalResult.Gem2 = gemSlotsValues[1]
	finalResult.Gem3 = gemSlotsValues[2]
	finalResult.Gem4 = gemSlotsValues[3]

	// 3. Merge Logic (Sum Components)
	// Helpers from GameParser aren't importable (cycle?), so we implement manually
	merge := func(dst *Models.CommStatModel, eq, hero, gem *Models.CommStatModel) {
		dst.MeleeCbtStr = eq.MeleeCbtStr + hero.MeleeCbtStr + gem.MeleeCbtStr
		dst.RangeCbtStr = eq.RangeCbtStr + hero.RangeCbtStr + gem.RangeCbtStr
		dst.FrontCbtStr = eq.FrontCbtStr + hero.FrontCbtStr + gem.FrontCbtStr
		dst.FlankCbtStr = eq.FlankCbtStr + hero.FlankCbtStr + gem.FlankCbtStr
		dst.AllCbtStr = eq.AllCbtStr + hero.AllCbtStr + gem.AllCbtStr
		dst.CyCbtStr = eq.CyCbtStr + hero.CyCbtStr + gem.CyCbtStr
		dst.WallStr = eq.WallStr + hero.WallStr + gem.WallStr
		dst.GateStr = eq.GateStr + hero.GateStr + gem.GateStr
		dst.MoatStr = eq.MoatStr + hero.MoatStr + gem.MoatStr

		dst.FlankLimit = eq.FlankLimit + hero.FlankLimit + gem.FlankLimit
		dst.FrontLimit = eq.FrontLimit + hero.FrontLimit + gem.FrontLimit

		dst.MaidenSupp = eq.MaidenSupp + hero.MaidenSupp + gem.MaidenSupp
		dst.Travel = eq.Travel + hero.Travel + gem.Travel
		dst.Loot = eq.Loot + hero.Loot + gem.Loot
		dst.Wave = eq.Wave + hero.Wave + gem.Wave
		dst.Cooldown = eq.Cooldown + hero.Cooldown + gem.Cooldown

		// Hero Bonus Stats
		dst.EliteStr = eq.EliteStr + hero.EliteStr + gem.EliteStr
		dst.HorrorStr = eq.HorrorStr + hero.HorrorStr + gem.HorrorStr
		dst.BeserkerStr = eq.BeserkerStr + hero.BeserkerStr + gem.BeserkerStr
		dst.RelicStr = eq.RelicStr + hero.RelicStr + gem.RelicStr
		dst.MeadStr = eq.MeadStr + hero.MeadStr + gem.MeadStr

		// NPC
		dst.NPCMelee = eq.NPCMelee + hero.NPCMelee + gem.NPCMelee
		dst.NPCRange = eq.NPCRange + hero.NPCRange + gem.NPCRange
		dst.NPCFront = eq.NPCFront + hero.NPCFront + gem.NPCFront
		dst.NPCFlank = eq.NPCFlank + hero.NPCFlank + gem.NPCFlank
		dst.NPCCy = eq.NPCCy + hero.NPCCy + gem.NPCCy
		dst.NPCWall = eq.NPCWall + hero.NPCWall + gem.NPCWall
		dst.NPCGate = eq.NPCGate + hero.NPCGate + gem.NPCGate
		dst.NPCMoat = eq.NPCMoat + hero.NPCMoat + gem.NPCMoat
		dst.NPCGlory = eq.NPCGlory + hero.NPCGlory + gem.NPCGlory

		// PVP
		dst.CLMelee = eq.CLMelee + hero.CLMelee + gem.CLMelee
		dst.CLRange = eq.CLRange + hero.CLRange + gem.CLRange
		dst.CLFront = eq.CLFront + hero.CLFront + gem.CLFront
		dst.CLFlank = eq.CLFlank + hero.CLFlank + gem.CLFlank
		dst.CLCy = eq.CLCy + hero.CLCy + gem.CLCy
		dst.CLWall = eq.CLWall + hero.CLWall + gem.CLWall
		dst.CLGate = eq.CLGate + hero.CLGate + gem.CLGate
		dst.CLMoat = eq.CLMoat + hero.CLMoat + gem.CLMoat
		dst.CLLater = eq.CLLater + hero.CLLater + gem.CLLater
		dst.CLFire = eq.CLFire + hero.CLFire + gem.CLFire
		dst.CLGlory = eq.CLGlory + hero.CLGlory + gem.CLGlory
	}

	merge(&finalResult, &tempEquipStat, &tempHeroStat, &tempGemStat)

	// 4. Final Global Ceiling
	Models.ApplyCommCeiling(&finalResult, &Models.CommEquipCeiling)

	return finalResult
}

func GetBaseCeilings() map[string]float64 {
	// Map Models.CommBaseStatCeilings (Equipment Only)
	c := Models.CommBaseStatCeilings
	return map[string]float64{
		"MeleeCbtStr": c.MeleeCbtStr, "RangeCbtStr": c.RangeCbtStr,
		"WallStr": c.WallStr, "GateStr": c.GateStr, "MoatStr": c.MoatStr,
		// Actually CommBaseStatCeilings.CLMelee is 100.
		"CLMelee": c.CLMelee, "CLRange": c.CLRange,
		"CLWall": c.CLWall, "CLGate": c.CLGate, "CLMoat": c.CLMoat,
		// ... Map the rest shorthand ...
	}
}

func getKeys(m map[string]bool) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func GetHeroCeilings() map[string]float64 {
	c := Models.CommHeroEquipCeilings
	return map[string]float64{
		"CLMelee": c.CLMelee,
		"CLRange": c.CLRange,
		"CLWall":  c.CLWall,
		"CLGate":  c.CLGate,
		"CLMoat":  c.CLMoat,
		"CLLater": c.CLLater,
		"CLFront": c.CLFront,
		"CLFlank": c.CLFlank,
		"CLCy":    c.CLCy,
		// ... Add others as needed
	}
}

func ParseOnlyBase(eq Models.EquipmentModel) CandidateItem {
	candidate := CandidateItem{
		OriginalID:     eq.ID,
		Slot:           eq.EquipSlotNumber,
		HasGemSlot:     (eq.EquipRarity == 5 || eq.EquipRarity == 15),
		Stats:          make(map[string]float64),
		StatEfficiency: make(map[string]float64),
	}

	context := "Equipment"
	if eq.EquipSlotNumber == 6 {
		context = "Hero"
	}

	add := func(id, val, efficiency float64) {
		name := GetStatName(id, context)
		if name != "" {
			candidate.Stats[name] += val
			// Store efficiency (use max if stat appears multiple times)
			if efficiency > candidate.StatEfficiency[name] {
				candidate.StatEfficiency[name] = efficiency
			}
		}
	}

	for _, stat := range eq.EquipStats {
		val := stat.Value[0]
		// IDs 200xx are typically Hero Bonuses which store the real value in index 1
		// 121 is MaidenSupp
		if (stat.ID >= 20010 && stat.ID <= 20030) || stat.ID == 121 {
			if len(stat.Value) > 1 {
				val = stat.Value[1]
			}
		}
		add(stat.ID, val, stat.Percent)
	}
	return candidate
}

func ParseOnlyGem(gem Models.Gem) CandidateItem {
	c := CandidateItem{
		OriginalID:     gem.ID,
		IsGem:          true,
		Stats:          make(map[string]float64),
		StatEfficiency: make(map[string]float64),
	}
	// Helper to add stat with efficiency
	add := func(id, val, efficiency float64) {
		name := GetStatName(id, "Gem")
		if name != "" {
			c.Stats[name] += val
			if efficiency > c.StatEfficiency[name] {
				c.StatEfficiency[name] = efficiency
			}
		}
	}
	for _, s := range gem.GemStats {
		val := s.Value[0]
		add(s.ID, val, s.Percent)
	}
	return c
}
