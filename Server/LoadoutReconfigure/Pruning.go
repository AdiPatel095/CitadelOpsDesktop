package LoadoutReconfigure

// PruneInventory filters out items that are strictly dominated by others
func PruneInventory(inventory []CandidateItem, profile OptimizationProfile) []CandidateItem {
	// Group items by Slot (1=Armor, 2=Weapon, 3=Helm, 4=Artifact, 6=Hero)
	slots := make(map[float64][]CandidateItem)
	for _, item := range inventory {
		slots[item.Slot] = append(slots[item.Slot], item)
	}

	var prunedInv []CandidateItem
	for _, items := range slots {
		prunedInv = append(prunedInv, ParetoPrune(items, profile)...)
	}
	return prunedInv
}

// ParetoPrune removes items that are dominated by another item in the list
func ParetoPrune(items []CandidateItem, profile OptimizationProfile) []CandidateItem {
	var kept []CandidateItem

	// O(N^2) comparison - fine for N < 1000
	for i, candidate := range items {
		dominated := false
		for j, comparison := range items {
			if i == j {
				continue
			}
			if IsDominated(candidate, comparison, profile) {
				dominated = true
				break
			}
		}
		if !dominated {
			kept = append(kept, candidate)
		}
	}
	return kept
}

// IsDominated returns true if 'weak' is strictly worse than 'strong'
// 'strong' must be >= 'weak' in ALL relevant stats, and > in at least one.
// IsDominated returns true if 'weak' is strictly worse than 'strong'
// 'strong' must be >= 'weak' in ALL relevant stats, and > in at least one.
func IsDominated(weak CandidateItem, strong CandidateItem, profile OptimizationProfile) bool {
	// Critical: If weak has a Gem Slot and strong doesn't, weak has "Potential" that strong lacks.
	// Therefore, weak cannot be dominated.
	if weak.HasGemSlot && !strong.HasGemSlot {
		return false
	}
	// If strong has a slot and weak doesn't, strong has an extra advantage.
	// We proceed to check stats.

	strictlyBetterInOne := false

	// If strong has a slot and weak doesn't, strong is already "better" in potential.
	if strong.HasGemSlot && !weak.HasGemSlot {
		strictlyBetterInOne = true
	}

	// Iterate over all relevant stats defined in the profile
	for statName := range profile.RelevanceMask {
		valWeak := weak.Stats[statName]
		valStrong := strong.Stats[statName]

		// If weak has more of ANY relevant stat, it is NOT dominated
		if valWeak > valStrong {
			return false
		}

		if valStrong > valWeak {
			strictlyBetterInOne = true
		}
	}

	return strictlyBetterInOne
}

// ApplyRelevanceMask creates a copy of the item with only relevant stats
func ApplyRelevanceMask(item CandidateItem, profile OptimizationProfile) CandidateItem {
	masked := CandidateItem{
		OriginalID: item.OriginalID,
		Slot:       item.Slot,
		IsGem:      item.IsGem,
		Stats:      make(map[string]float64),
	}

	for k, v := range item.Stats {
		if profile.RelevanceMask[k] {
			masked.Stats[k] = v
		}
	}
	return masked
}
