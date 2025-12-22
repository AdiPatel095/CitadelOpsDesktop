package ReconfigureLoadout

import (
	"fmt"
	"sort"
	"sync"

	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
)

// PreparePriority converts the frontend payload stats into optimized data structures
// for the optimizer to use efficiently.
// - Tier 1 stats: 10000 base, 10% cumulative decay - sorted array for weighted scoring
// - Tier 2 stats: 100 base, 5% cumulative decay - sorted array for weighted scoring + bitmask for pruning
func PreparePriority(payload ReconfigurePayload) PreparedPriority {
	prepared := PreparedPriority{
		Tier1Stats:   make([]PriorityStat, 0),
		Tier2Stats:   make([]PriorityStat, 0),
		Tier2Bitmask: make(map[float64]bool),
	}

	// Convert payload stats to PriorityStat with IDs and separate by tier
	for _, s := range payload.Stats {
		statIDs := GetStatIDsForName(s.Stat, payload.EquipmentMode)

		priorityStat := PriorityStat{
			StatName: s.Stat,
			Tier:     s.Tier,
			Position: s.Position,
			StatIDs:  statIDs,
		}

		switch s.Tier {
		case 1:
			prepared.Tier1Stats = append(prepared.Tier1Stats, priorityStat)
		case 2:
			prepared.Tier2Stats = append(prepared.Tier2Stats, priorityStat)
			// Also add to bitmask for pruning
			for _, id := range statIDs {
				prepared.Tier2Bitmask[id] = true
			}
		}
	}

	// Sort tier 1 stats by position (highest priority first, position 0 = highest)
	sort.Slice(prepared.Tier1Stats, func(i, j int) bool {
		return prepared.Tier1Stats[i].Position < prepared.Tier1Stats[j].Position
	})

	// Sort tier 2 stats by position
	sort.Slice(prepared.Tier2Stats, func(i, j int) bool {
		return prepared.Tier2Stats[i].Position < prepared.Tier2Stats[j].Position
	})

	return prepared
}

// SeparateEquipmentBySlot groups equipment into 4 separate arrays by EquipSlotNumber
// Returns: slot1, slot2, slot3, slot4 arrays and slotOrder (indices sorted by size, smallest first)
func SeparateEquipmentBySlot(equipment []Models.EquipmentModel) (
	slot1, slot2, slot3, slot4 []Models.EquipmentModel,
	slotOrder [4]int,
) {
	// Initialize slot arrays
	slot1 = make([]Models.EquipmentModel, 0)
	slot2 = make([]Models.EquipmentModel, 0)
	slot3 = make([]Models.EquipmentModel, 0)
	slot4 = make([]Models.EquipmentModel, 0)

	// Separate equipment by slot number
	for _, equip := range equipment {
		switch int(equip.EquipSlotNumber) {
		case 1:
			slot1 = append(slot1, equip)
		case 2:
			slot2 = append(slot2, equip)
		case 3:
			slot3 = append(slot3, equip)
		case 4:
			slot4 = append(slot4, equip)
		}
	}

	// Create slot order sorted by size (smallest first for early pruning)
	// Each element is the slot number (1-4)
	type slotSize struct {
		slot int
		size int
	}
	sizes := []slotSize{
		{1, len(slot1)},
		{2, len(slot2)},
		{3, len(slot3)},
		{4, len(slot4)},
	}
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i].size < sizes[j].size
	})
	for i, s := range sizes {
		slotOrder[i] = s.slot
	}

	return slot1, slot2, slot3, slot4, slotOrder
}

// Optimize is the shared optimizer method that works with pre-filtered equipment, heroes, and gems
// It takes OptimizerInput and returns an OptimizationResult with the best loadout
func Optimize(input OptimizerInput) OptimizationResult {

	// Step 1: Prepare priority data structures
	prepared := PreparePriority(input.Payload)

	// Step 1.5: Select Best Hero Independently
	// We pick the best hero purely based on score. Hero stats do not interact with
	// equipment ceilings or bitmasks in this model.
	var bestHeroID float64
	var bestHeroScore float64 = -1.0

	if len(input.Heroes) > 0 {
		for _, hero := range input.Heroes {
			score := ScoreEquipment(hero, prepared, true)
			if score > bestHeroScore {
				bestHeroScore = score
				bestHeroID = hero.ID
			}
		}
	}

	// Step 2: Separate equipment by slot
	slot1, slot2, slot3, slot4, slotOrder := SeparateEquipmentBySlot(input.Equipment)

	// Step 3: Score and select top 10 candidates from the first slot (smallest)
	// Only process the first slot in slotOrder - it has the fewest items
	firstSlotNum := slotOrder[0]
	var firstSlotEquipment []Models.EquipmentModel
	switch firstSlotNum {
	case 1:
		firstSlotEquipment = slot1
	case 2:
		firstSlotEquipment = slot2
	case 3:
		firstSlotEquipment = slot3
	case 4:
		firstSlotEquipment = slot4
	}

	topCandidates := SelectTopEquipment(firstSlotEquipment, prepared)

	// Step 4: For each candidate, spawn a goroutine to continue optimization
	// Each goroutine gets a copy of the bitmask updated with its candidate's stats
	var wg sync.WaitGroup
	wg.Add(len(topCandidates))

	// Store slot arrays for easy access by slot number
	slots := map[int]*[]Models.EquipmentModel{
		1: &slot1,
		2: &slot2,
		3: &slot3,
		4: &slot4,
	}

	// Result storage: one OptimizationResult per branch
	branchResults := make([]OptimizationResult, len(topCandidates)) // Array to store best result from each branch.

	for i, candidate := range topCandidates {
		go func(branchIndex int, selectedEquip Models.EquipmentModel) {
			defer wg.Done()

			// Create bitmask with ONLY Tier2 stat IDs initialized to false
			// Tier1 stats are NOT in bitmask - items with Tier1 stats pass through pruning
			branchBitmask := make(map[float64]bool)

			// Add ONLY Tier2 stat IDs as switches (false = not covered yet)
			for statID := range prepared.Tier2Bitmask {
				branchBitmask[statID] = false
			}

			// Flip to true for stat IDs present on the selected equipment
			for _, stat := range selectedEquip.EquipStats {
				if _, exists := branchBitmask[stat.ID]; exists {
					branchBitmask[stat.ID] = true
				}
			}

			// Step 5: Get slot 2 inventory (slotOrder[1]) and prune based on coverage
			secondSlotNum := slotOrder[1]
			secondSlotInventory := *slots[secondSlotNum]

			// Prune: Strict exclusion - remove items with any redundant stats
			prunedSlot2 := pruneInventory(secondSlotInventory, branchBitmask)

			// Branch tracking: best result found so far, running candidate, and capped stat IDs
			// Branch tracking: running candidate, and capped stat IDs
			currentCandidate := OptimizationResult{}
			cappedStatIDs := make(map[float64]bool) // Tracks stat IDs that have exceeded ceiling
			cappedBySlot := make(map[float64][]int) // Maps stat ID -> list of slot numbers that contribute to the cap
			_ = cappedBySlot                        // Will be used for cap-aware pruning

			// Initialize with slot 1 equipment
			switch firstSlotNum {
			case 1:
				branchResults[branchIndex].Equip1 = selectedEquip.ID
				currentCandidate.Equip1 = selectedEquip.ID
			case 2:
				branchResults[branchIndex].Equip2 = selectedEquip.ID
				currentCandidate.Equip2 = selectedEquip.ID
			case 3:
				branchResults[branchIndex].Equip3 = selectedEquip.ID
				currentCandidate.Equip3 = selectedEquip.ID
			case 4:
				branchResults[branchIndex].Equip4 = selectedEquip.ID
				currentCandidate.Equip4 = selectedEquip.ID
			}

			// Helper to set equipment ID in result by slot number
			setEquipBySlot := func(result *OptimizationResult, slotNum int, equipID float64) {
				switch slotNum {
				case 1:
					result.Equip1 = equipID
				case 2:
					result.Equip2 = equipID
				case 3:
					result.Equip3 = equipID
				case 4:
					result.Equip4 = equipID
				}
			}

			// Get slot numbers for iteration
			thirdSlotNum := slotOrder[2]
			fourthSlotNum := slotOrder[3]

			// Pre-calculate Max Potential Stats for future slots to optimize pruning
			maxSlot3Values := getMaxStatValues(*slots[thirdSlotNum])
			maxSlot4Values := getMaxStatValues(*slots[fourthSlotNum])

			// Initialize Base Stats (Slot 1)
			var commStats1 Models.CommStatModel
			var castStats1 Models.CastStatModel
			isCastellan := input.Payload.EquipmentMode == "Castellan"

			if isCastellan {
				GameParser.ProcessEquipStatCast(selectedEquip, &castStats1, &Models.CastEquipCeiling)
			} else {
				GameParser.ProcessEquipStatComm(selectedEquip, &commStats1, &Models.CommEquipCeiling)
			}

			// Deduplicate Slot 2
			prunedSlot2 = deduplicateInventory(prunedSlot2, prepared)

			// Fallback: If pruning removed all items, use unpruned inventory (dedup only)
			if len(prunedSlot2) == 0 && len(secondSlotInventory) > 0 {
				prunedSlot2 = deduplicateInventory(secondSlotInventory, prepared)
			}

			// === ITERATE SLOT 2 ===
			for _, slot2Equip := range prunedSlot2 {
				// Incremental Pruning (Loop 2)
				if isCastellan {
					stats12 := castStats1
					GameParser.ProcessEquipStatCast(slot2Equip, &stats12, &Models.CastEquipCeiling)

					currentScore := ScoreCastellan(&stats12, prepared, input.Payload.CombatMode)
					potential := calculatePotentialBonusCast(&stats12, []map[float64]float64{maxSlot3Values, maxSlot4Values}, prepared, &Models.CastEquipCeiling, input.Payload.CombatMode)

					if currentScore+potential <= branchResults[branchIndex].Score {
						continue
					}
				} else {
					stats12 := commStats1
					GameParser.ProcessEquipStatComm(slot2Equip, &stats12, &Models.CommEquipCeiling)

					currentScore := ScoreCommander(&stats12, prepared, input.Payload.CombatMode)
					potential := calculatePotentialBonusComm(&stats12, []map[float64]float64{maxSlot3Values, maxSlot4Values}, prepared, &Models.CommEquipCeiling, input.Payload.CombatMode)

					if currentScore+potential <= branchResults[branchIndex].Score {
						continue
					}
				}

				// Copy bitmask for this slot2 iteration
				bitmask2 := make(map[float64]bool)
				for k, v := range branchBitmask {
					bitmask2[k] = v
				}
				updateBitmask(bitmask2, slot2Equip)

				// Set slot 2 in current candidate
				setEquipBySlot(&currentCandidate, secondSlotNum, slot2Equip.ID)

				// Prune slot 3 based on bitmask2 using Strict Exclusion
				prunedSlot3 := deduplicateInventory(pruneInventory(*slots[thirdSlotNum], bitmask2), prepared)

				// Fallback: If pruning removed all items, use unpruned inventory (dedup only)
				if len(prunedSlot3) == 0 && len(*slots[thirdSlotNum]) > 0 {
					prunedSlot3 = deduplicateInventory(*slots[thirdSlotNum], prepared)
				}

				// === ITERATE SLOT 3 ===
				for _, slot3Equip := range prunedSlot3 {
					// Incremental Pruning (Loop 3)
					if isCastellan {
						stats12 := castStats1
						GameParser.ProcessEquipStatCast(slot2Equip, &stats12, &Models.CastEquipCeiling)

						stats123 := stats12
						GameParser.ProcessEquipStatCast(slot3Equip, &stats123, &Models.CastEquipCeiling)

						currentScore := ScoreCastellan(&stats123, prepared, input.Payload.CombatMode)
						potential := calculatePotentialBonusCast(&stats123, []map[float64]float64{maxSlot4Values}, prepared, &Models.CastEquipCeiling, input.Payload.CombatMode)

						if currentScore+potential <= branchResults[branchIndex].Score {
							continue
						}
					} else {
						stats12 := commStats1
						GameParser.ProcessEquipStatComm(slot2Equip, &stats12, &Models.CommEquipCeiling)

						stats123 := stats12
						GameParser.ProcessEquipStatComm(slot3Equip, &stats123, &Models.CommEquipCeiling)

						currentScore := ScoreCommander(&stats123, prepared, input.Payload.CombatMode)
						potential := calculatePotentialBonusComm(&stats123, []map[float64]float64{maxSlot4Values}, prepared, &Models.CommEquipCeiling, input.Payload.CombatMode)

						if currentScore+potential <= branchResults[branchIndex].Score {
							continue
						}
					}

					// Copy bitmask for this slot3 iteration
					bitmask3 := make(map[float64]bool)
					for k, v := range bitmask2 {
						bitmask3[k] = v
					}
					updateBitmask(bitmask3, slot3Equip)

					// Set slot 3 in current candidate
					setEquipBySlot(&currentCandidate, thirdSlotNum, slot3Equip.ID)

					// Prune slot 4 based on bitmask3 using Strict Exclusion
					prunedSlot4 := deduplicateInventory(pruneInventory(*slots[fourthSlotNum], bitmask3), prepared)

					// Fallback: If pruning removed all items, use unpruned inventory (dedup only)
					if len(prunedSlot4) == 0 && len(*slots[fourthSlotNum]) > 0 {
						prunedSlot4 = deduplicateInventory(*slots[fourthSlotNum], prepared)
					}

					// === ITERATE SLOT 4 ===
					for _, slot4Equip := range prunedSlot4 {
						// Set slot 4 in current candidate
						setEquipBySlot(&currentCandidate, fourthSlotNum, slot4Equip.ID)

						// === Build Full Stats ===
						var candidateScore float64
						if input.Payload.EquipmentMode == "Castellan" {
							aggregatedStats := Models.CastStatModel{}
							GameParser.ProcessEquipStatCast(selectedEquip, &aggregatedStats, &Models.CastEquipCeiling)
							GameParser.ProcessEquipStatCast(slot2Equip, &aggregatedStats, &Models.CastEquipCeiling)
							GameParser.ProcessEquipStatCast(slot3Equip, &aggregatedStats, &Models.CastEquipCeiling)
							GameParser.ProcessEquipStatCast(slot4Equip, &aggregatedStats, &Models.CastEquipCeiling)

							// Check Caps
							caps := CheckCastCeilings(&aggregatedStats, &Models.CastEquipCeiling)
							for id := range caps {
								if !cappedStatIDs[id] {
									cappedStatIDs[id] = true
									// Identify contributing slots
									var contributingSlots []int
									pieces := []Models.EquipmentModel{selectedEquip, slot2Equip, slot3Equip, slot4Equip}
									slotNums := []int{firstSlotNum, secondSlotNum, thirdSlotNum, fourthSlotNum}

									for i, piece := range pieces {
										for _, stat := range piece.EquipStats {
											if stat.ID == id {
												contributingSlots = append(contributingSlots, slotNums[i])
												break
											}
										}
									}
									cappedBySlot[id] = contributingSlots
								}
							}

							candidateScore = ScoreCastellan(&aggregatedStats, prepared, input.Payload.CombatMode)
						} else {
							aggregatedStats := Models.CommStatModel{}
							GameParser.ProcessEquipStatComm(selectedEquip, &aggregatedStats, &Models.CommEquipCeiling)
							GameParser.ProcessEquipStatComm(slot2Equip, &aggregatedStats, &Models.CommEquipCeiling)
							GameParser.ProcessEquipStatComm(slot3Equip, &aggregatedStats, &Models.CommEquipCeiling)
							GameParser.ProcessEquipStatComm(slot4Equip, &aggregatedStats, &Models.CommEquipCeiling)

							// Check Caps
							caps := CheckCommCeilings(&aggregatedStats, &Models.CommEquipCeiling)
							for id := range caps {
								if !cappedStatIDs[id] {
									cappedStatIDs[id] = true
									// Identify contributing slots
									var contributingSlots []int
									pieces := []Models.EquipmentModel{selectedEquip, slot2Equip, slot3Equip, slot4Equip}
									slotNums := []int{firstSlotNum, secondSlotNum, thirdSlotNum, fourthSlotNum}

									for i, piece := range pieces {
										for _, stat := range piece.EquipStats {
											if stat.ID == id {
												contributingSlots = append(contributingSlots, slotNums[i])
												break
											}
										}
									}
									cappedBySlot[id] = contributingSlots
								}
							}

							candidateScore = ScoreCommander(&aggregatedStats, prepared, input.Payload.CombatMode)
						}

						// Assign score to current candidate
						currentCandidate.Score = candidateScore

						// Cap Aware Optimization
						// Pass fully formed candidate (score included), context inventories (slots), cappedBySlot map, and the branch's selected piece
						CapAwareOptimization(&currentCandidate, slots, cappedBySlot, selectedEquip, prepared, input.Payload.EquipmentMode, input.Payload.CombatMode)

						if currentCandidate.Score > branchResults[branchIndex].Score {
							branchResults[branchIndex] = currentCandidate
						}
					}
				}
			}
		}(i, candidate)
	}

	wg.Wait()

	// Select best result from branchResults
	var finalBest OptimizationResult
	var maxScore float64 = -1

	for _, res := range branchResults {
		if res.Score > maxScore {
			maxScore = res.Score
			finalBest = res
		}
	}

	// Assign selected hero to result
	finalBest.Hero = bestHeroID

	// Step 6: Optimize Gems
	OptimizeGems(&finalBest, input, prepared)

	return finalBest
}

// CapAwareOptimization optimizes the loadout based on capped stats.
func CapAwareOptimization(candidate *OptimizationResult, slots map[int]*[]Models.EquipmentModel, cappedBySlot map[float64][]int, branchPiece Models.EquipmentModel, prepared PreparedPriority, equipmentMode string, combatMode string) {
	// 1. Identify "forbidden stats" for each slot
	// Maps SlotNum -> StatID -> Forbidden (true)
	forbiddenStats := make(map[int]map[float64]bool)

	for statID, slotNums := range cappedBySlot {
		if len(slotNums) > 0 {
			// Prune only the first priority slot type (index 0)
			targetSlot := slotNums[0]
			if _, exists := forbiddenStats[targetSlot]; !exists {
				forbiddenStats[targetSlot] = make(map[float64]bool)
			}
			forbiddenStats[targetSlot][statID] = true
		}
	}

	// 2. Prune inventories for relevant slots (skipping the fixed branchPiece slot)
	// We need to identify which slots are variable (2, 3, 4 typically)
	// branchPiece.EquipSlotNumber tells us the fixed slot.
	fixedSlotNum := int(branchPiece.EquipSlotNumber)

	// Map to hold pruned lists for the variable slots
	prunedLists := make(map[int][]Models.EquipmentModel)
	var variableSlots []int

	for slotNum, inventoryPtr := range slots {
		if slotNum == fixedSlotNum {
			continue
		}
		variableSlots = append(variableSlots, slotNum)

		originalList := *inventoryPtr
		// Create pruned list
		pruned := make([]Models.EquipmentModel, 0, len(originalList))
		forbiddenForSlot := forbiddenStats[slotNum]

		for _, item := range originalList {
			isForbidden := false
			for _, stat := range item.EquipStats {
				if forbiddenForSlot[stat.ID] {
					isForbidden = true
					break
				}
			}
			if !isForbidden {
				pruned = append(pruned, item)
			}
		}
		prunedLists[slotNum] = pruned
	}

	// If we don't have exactly 3 variable slots + 1 fixed, something is unusual but we proceed with what we have.
	// Logic assumes 4 slots total.
	if len(variableSlots) != 3 {
		return // Should not happen in standard 4-slot optimization
	}

	// Sort variableSlots ensures consistent nesting order (e.g. 2, 3, 4)
	// (Map iteration order is random)
	// Since variableSlots is small, simple sort or just specific assignment:
	// We expect slots 1, 2, 3, 4. If fixed is 1, variable are 2, 3, 4.
	// Lets sort variableSlots to be deterministic.
	// ... omitting explicit sort import for brevity, assuming small input:
	// Bubble sort for 3 items
	for i := 0; i < len(variableSlots)-1; i++ {
		for j := 0; j < len(variableSlots)-i-1; j++ {
			if variableSlots[j] > variableSlots[j+1] {
				variableSlots[j], variableSlots[j+1] = variableSlots[j+1], variableSlots[j]
			}
		}
	}

	// 3. Nested Loop through pruned inventories with Bitmask Pruning
	slotA := variableSlots[0]
	slotB := variableSlots[1]
	slotC := variableSlots[2]

	listA := prunedLists[slotA]

	// Initialize Base Bitmask for this branch
	baseBitmask := make(map[float64]bool)
	for statID := range prepared.Tier2Bitmask {
		baseBitmask[statID] = false
	}
	// Update with branchPiece stats
	updateBitmask(baseBitmask, branchPiece)

	// Scorer Helpers
	var scoreFunc func(s1, s2, s3, s4 Models.EquipmentModel) float64

	if equipmentMode == "Castellan" {
		scoreFunc = func(s1, s2, s3, s4 Models.EquipmentModel) float64 {
			stats := Models.CastStatModel{}
			GameParser.ProcessEquipStatCast(s1, &stats, &Models.CastEquipCeiling)
			GameParser.ProcessEquipStatCast(s2, &stats, &Models.CastEquipCeiling)
			GameParser.ProcessEquipStatCast(s3, &stats, &Models.CastEquipCeiling)
			GameParser.ProcessEquipStatCast(s4, &stats, &Models.CastEquipCeiling)
			return ScoreCastellan(&stats, prepared, combatMode)
		}
	} else {
		scoreFunc = func(s1, s2, s3, s4 Models.EquipmentModel) float64 {
			stats := Models.CommStatModel{}
			GameParser.ProcessEquipStatComm(s1, &stats, &Models.CommEquipCeiling)
			GameParser.ProcessEquipStatComm(s2, &stats, &Models.CommEquipCeiling)
			GameParser.ProcessEquipStatComm(s3, &stats, &Models.CommEquipCeiling)
			GameParser.ProcessEquipStatComm(s4, &stats, &Models.CommEquipCeiling)
			return ScoreCommander(&stats, prepared, combatMode)
		}
	}

	// Helper to set result
	setResult := func(res *OptimizationResult, sNum int, id float64) {
		switch sNum {
		case 1:
			res.Equip1 = id
		case 2:
			res.Equip2 = id
		case 3:
			res.Equip3 = id
		case 4:
			res.Equip4 = id
		}
	}

	// Prune List A based on Base Bitmask
	// Use cap-pruned list as source
	listAPruned := pruneInventory(listA, baseBitmask)

	for _, itemA := range listAPruned {
		// Bitmask A
		bitmaskA := make(map[float64]bool)
		for k, v := range baseBitmask {
			bitmaskA[k] = v
		}
		updateBitmask(bitmaskA, itemA)

		// Prune List B based on Bitmask A
		// We use the CAP-PRUNED list for B as the source inventory
		listBPruned := pruneInventory(prunedLists[slotB], bitmaskA)

		for _, itemB := range listBPruned {
			// Bitmask B
			bitmaskB := make(map[float64]bool)
			for k, v := range bitmaskA {
				bitmaskB[k] = v
			}
			updateBitmask(bitmaskB, itemB)

			// Prune List C based on Bitmask B
			// We use the CAP-PRUNED list for C as the source inventory
			listCPruned := pruneInventory(prunedLists[slotC], bitmaskB)

			for _, itemC := range listCPruned {
				newScore := scoreFunc(branchPiece, itemA, itemB, itemC)

				if newScore > candidate.Score {
					candidate.Score = newScore

					// Update IDs
					setResult(candidate, fixedSlotNum, branchPiece.ID) // Should stay same
					setResult(candidate, slotA, itemA.ID)
					setResult(candidate, slotB, itemB.ID)
					setResult(candidate, slotC, itemC.ID)
				}
			}
		}
	}
}

// getStatNames is a helper to extract stat names for logging
func getStatNames(stats []PriorityStat) []string {
	names := make([]string, len(stats))
	for i, s := range stats {
		names[i] = s.StatName
	}
	return names
}

// pruneInventory filters inventory keeping only items that do NOT contain any already covered stats
// Strict Exclusion: If an item has even one stat that is already covered (true in bitmask), it is pruned.
func pruneInventory(inventory []Models.EquipmentModel, bitmask map[float64]bool) []Models.EquipmentModel {
	pruned := make([]Models.EquipmentModel, 0)
	for _, equip := range inventory {
		isRedundant := false
		for _, stat := range equip.EquipStats {
			// If stat is in bitmask AND is true (already covered), the item is redundant
			if covered, exists := bitmask[stat.ID]; exists && covered {
				isRedundant = true
				break
			}
		}
		if !isRedundant {
			pruned = append(pruned, equip)
		}
	}
	return pruned
}

// updateBitmask updates the bitmask to mark stats present on the equipment as covered
func updateBitmask(bitmask map[float64]bool, equip Models.EquipmentModel) {
	for _, stat := range equip.EquipStats {
		if _, exists := bitmask[stat.ID]; exists {
			bitmask[stat.ID] = true
		}
	}
}

// getMaxStatValues returns a map of StatID -> MaxValue found in the inventory
func getMaxStatValues(inventory []Models.EquipmentModel) map[float64]float64 {
	maxValues := make(map[float64]float64)
	for _, item := range inventory {
		for _, stat := range item.EquipStats {
			if len(stat.Value) > 0 {
				val := stat.Value[0]
				if val > maxValues[stat.ID] {
					maxValues[stat.ID] = val
				}
			}
		}
	}
	return maxValues
}

// OptimizeGems optimizes the gem slots for the selected equipment/hero loadout
func OptimizeGems(result *OptimizationResult, input OptimizerInput, prepared PreparedPriority) {
	if len(input.Gems) == 0 {
		return
	}

	// 1. Get Top Candidates using the multi-pass filter
	candidateGems := filterTopStatGems(input.Gems, prepared)

	// 2. Reconstruct Base Loadout to track (for synergy context, though optimization focuses on Gem Bucket)
	finalEquip1 := findEquipByID(result.Equip1, input.Equipment)
	finalEquip2 := findEquipByID(result.Equip2, input.Equipment)
	finalEquip3 := findEquipByID(result.Equip3, input.Equipment)
	finalEquip4 := findEquipByID(result.Equip4, input.Equipment)
	var finalHero Models.EquipmentModel
	if result.Hero > 0 {
		finalHero = findEquipByID(result.Hero, input.Heroes)
	}

	// Initialize Stats using Gem Ceilings for EVERYTHING (Equip + Hero + Gems)
	// As requested: "process through a gemCeiling object and we use that for calculating gaps"
	isCastellan := input.Payload.EquipmentMode == "Castellan"
	var currentCommStats Models.CommStatModel
	var currentCastStats Models.CastStatModel

	// Prepare Gem Ceiling Models
	var castGemCeiling = Models.CastGemCeiling
	var commGemCeiling = Models.CommGemCeiling

	if isCastellan {
		// Process Equipment & Hero into currentStats using GEM CEILING
		GameParser.ProcessEquipStatCast(finalEquip1, &currentCastStats, &castGemCeiling)
		GameParser.ProcessEquipStatCast(finalEquip2, &currentCastStats, &castGemCeiling)
		GameParser.ProcessEquipStatCast(finalEquip3, &currentCastStats, &castGemCeiling)
		GameParser.ProcessEquipStatCast(finalEquip4, &currentCastStats, &castGemCeiling)
		Models.ApplyCastCeiling(&currentCastStats, &castGemCeiling)

		if result.Hero > 0 {
			GameParser.ProcessEquipStatCast(finalHero, &currentCastStats, &castGemCeiling)
			Models.ApplyCastCeiling(&currentCastStats, &castGemCeiling)
		}
	} else {
		// Process Equipment & Hero into currentStats using GEM CEILING
		GameParser.ProcessEquipStatComm(finalEquip1, &currentCommStats, &commGemCeiling)
		GameParser.ProcessEquipStatComm(finalEquip2, &currentCommStats, &commGemCeiling)
		GameParser.ProcessEquipStatComm(finalEquip3, &currentCommStats, &commGemCeiling)
		GameParser.ProcessEquipStatComm(finalEquip4, &currentCommStats, &commGemCeiling)
		Models.ApplyCommCeiling(&currentCommStats, &commGemCeiling)

		if result.Hero > 0 {
			GameParser.ProcessEquipStatComm(finalHero, &currentCommStats, &commGemCeiling)
			Models.ApplyCommCeiling(&currentCommStats, &commGemCeiling)
		}
	}

	// 3. Iteratively select 4 gems -> Filling the Gem Bucket ONLY (using current(Gem)Stats and GemCeiling)
	selectedGemIDs := make([]float64, 0, 4)

	// Copy candidates to a slice we can remove from
	availableGems := make([]Models.Gem, len(candidateGems))
	copy(availableGems, candidateGems)

	for i := 0; i < 4; i++ {
		if len(availableGems) == 0 {
			break
		}

		var bestGem Models.Gem
		var bestScore float64 = -1.0
		bestIndex := -1

		// Evaluate each candidate
		for idx, gem := range availableGems {
			var score float64
			if isCastellan {
				score = CalculateEffectiveGemScoreCast(gem, currentCastStats, prepared, castGemCeiling)
			} else {
				score = CalculateEffectiveGemScoreComm(gem, currentCommStats, prepared, commGemCeiling)
			}

			if score > bestScore {
				bestScore = score
				bestGem = gem
				bestIndex = idx
			}
		}

		if bestIndex != -1 {
			// Select this gem
			selectedGemIDs = append(selectedGemIDs, bestGem.ID)

			// Update current stats with this gem
			gemWrapper := Models.EquipmentModel{
				EquipStats: bestGem.GemStats,
			}

			if isCastellan {
				GameParser.ProcessEquipStatCast(gemWrapper, &currentCastStats, &castGemCeiling)
			} else {
				GameParser.ProcessEquipStatComm(gemWrapper, &currentCommStats, &commGemCeiling)
			}

			// Remove from available list
			availableGems = append(availableGems[:bestIndex], availableGems[bestIndex+1:]...)
		}
	}

	// Assign to result
	if len(selectedGemIDs) > 0 {
		result.Gem1 = selectedGemIDs[0]
	}
	if len(selectedGemIDs) > 1 {
		result.Gem2 = selectedGemIDs[1]
	}
	if len(selectedGemIDs) > 2 {
		result.Gem3 = selectedGemIDs[2]
	}
	if len(selectedGemIDs) > 3 {
		result.Gem4 = selectedGemIDs[3]
	}
}

// CalculateEffectiveGemScoreComm calculates the score a gem would ADD given current stats and ceilings
func CalculateEffectiveGemScoreComm(gem Models.Gem, currentStats Models.CommStatModel, prepared PreparedPriority, ceiling Models.CommStatModel) float64 {
	var totalScore float64

	// Create map of ID -> Weight (Tier1 + Tier2)
	weights := make(map[float64]float64)

	// Tier 1: 10000 base, 10% decay
	for i, pStat := range prepared.Tier1Stats {
		w := calculateWeight(Tier1BaseScore, Tier1Decay, i)
		for _, id := range pStat.StatIDs {
			weights[id] = w
		}
	}

	// Tier 2: 100 base, 5% decay
	for i, pStat := range prepared.Tier2Stats {
		w := calculateWeight(Tier2BaseScore, Tier2Decay, i)
		for _, id := range pStat.StatIDs {
			if _, exists := weights[id]; !exists {
				weights[id] = w
			}
		}
	}

	for _, stat := range gem.GemStats {
		if weight, ok := weights[stat.ID]; ok && len(stat.Value) > 0 {
			gemValue := stat.Value[0]

			// Find which stat name this ID belongs to to check ceiling
			statName := getStatNameFromID(stat.ID, false) // false = Commander
			if statName != "" {
				if getter, exists := CommStatGetter[statName]; exists {
					curr := getter(&currentStats)
					capVal := getter(&ceiling)

					effective := getEffectiveValue(gemValue, curr, capVal)
					totalScore += effective * weight
				}
			} else {
				// If we can't map ID to a stat name for checking caps, assume uncapped or not relevant for cap
				// But since it's in priority list (weight > 0), it SHOULD be mapped.
				// Fallback: full value
				totalScore += gemValue * weight
			}
		}
	}
	return totalScore
}

// CalculateEffectiveGemScoreCast calculates the score a gem would ADD given current stats and ceilings
func CalculateEffectiveGemScoreCast(gem Models.Gem, currentStats Models.CastStatModel, prepared PreparedPriority, ceiling Models.CastStatModel) float64 {
	var totalScore float64

	// Create map of ID -> Weight (Tier1 + Tier2)
	weights := make(map[float64]float64)

	// Tier 1: 10000 base, 10% decay
	for i, pStat := range prepared.Tier1Stats {
		w := calculateWeight(Tier1BaseScore, Tier1Decay, i)
		for _, id := range pStat.StatIDs {
			weights[id] = w
		}
	}

	// Tier 2: 100 base, 5% decay
	for i, pStat := range prepared.Tier2Stats {
		w := calculateWeight(Tier2BaseScore, Tier2Decay, i)
		for _, id := range pStat.StatIDs {
			if _, exists := weights[id]; !exists {
				weights[id] = w
			}
		}
	}

	for _, stat := range gem.GemStats {
		if weight, ok := weights[stat.ID]; ok && len(stat.Value) > 0 {
			gemValue := stat.Value[0]

			statName := getStatNameFromID(stat.ID, true) // true = Castellan
			if statName != "" {
				if getter, exists := CastStatGetter[statName]; exists {
					curr := getter(&currentStats)
					capVal := getter(&ceiling)

					effective := getEffectiveValue(gemValue, curr, capVal)
					totalScore += effective * weight
				}
			} else {
				totalScore += gemValue * weight
			}
		}
	}
	return totalScore
}

// getEffectiveValue returns how much of val contributes to the score without exceeding cap
func getEffectiveValue(addVal, current, capVal float64) float64 {
	if capVal <= 0 {
		return addVal // No ceiling
	}
	if current >= capVal {
		return 0 // Already capped
	}
	space := capVal - current
	if addVal <= space {
		return addVal // Fully effective
	}
	return space // Partially effective
}

// getStatNameFromID tries to find a single stat name that maps to this ID
// This is reverse lookup and simplistic (assumes 1-to-1 or just finds first match)
// Used for finding which ceiling to check.
func getStatNameFromID(id float64, isCastellan bool) string {
	var mapping map[string][]float64
	if isCastellan {
		mapping = CastStatNameToIDs
	} else {
		mapping = CommStatNameToIDs
	}

	for name, ids := range mapping {
		for _, mapID := range ids {
			if mapID == id {
				return name
			}
		}
	}
	return ""
}

// findEquipByID finds an equipment model by ID in a slice
func findEquipByID(id float64, list []Models.EquipmentModel) Models.EquipmentModel {
	for _, item := range list {
		if item.ID == id {
			return item
		}
	}
	return Models.EquipmentModel{}
}

// filterTopStatGems selects the best gems by iteratively scoring with reducing priorities
// Combines Tier1 and Tier2 stats for comprehensive gem selection
func filterTopStatGems(gems []Models.Gem, prepared PreparedPriority) []Models.Gem {
	uniqueGems := make(map[float64]Models.Gem)

	// Combine Tier1 and Tier2 priorities into a single list for iteration
	// Tier1 stats come first (higher priority), then Tier2 stats
	combinedPriorities := make([]PriorityStat, 0, len(prepared.Tier1Stats)+len(prepared.Tier2Stats))
	combinedPriorities = append(combinedPriorities, prepared.Tier1Stats...)
	combinedPriorities = append(combinedPriorities, prepared.Tier2Stats...)

	// Create a copy to manipulate
	currentPriorities := make([]PriorityStat, len(combinedPriorities))
	copy(currentPriorities, combinedPriorities)

	// Iterate as long as we have priorities
	// For each pass, we score gems based on CURRENT priorities, pick top 5
	// Then remove the top priority stat and repeat
	for len(currentPriorities) > 0 {

		// Score all gems with current priority list
		type scoredGem struct {
			Gem   Models.Gem
			Score float64
		}
		scoredGems := make([]scoredGem, len(gems))

		for i, gem := range gems {
			score := ScoreGem(gem, currentPriorities)
			scoredGems[i] = scoredGem{Gem: gem, Score: score}
		}

		// Sort by score descending
		sort.Slice(scoredGems, func(i, j int) bool {
			return scoredGems[i].Score > scoredGems[j].Score
		})

		// Pick top 5 from this pass
		count := 0
		for _, sg := range scoredGems {
			if count >= 5 {
				break
			}
			uniqueGems[sg.Gem.ID] = sg.Gem
			count++
		}

		// Remove the top priority stat (index 0) (Shift)
		currentPriorities = currentPriorities[1:]
	}

	// Convert map to slice
	result := make([]Models.Gem, 0, len(uniqueGems))
	for _, g := range uniqueGems {
		result = append(result, g)
	}

	// Optional: Sort final result by value or some metric?
	// The requirement is just to return this list for the optimizer to use.
	// We'll just return the gathered unique gems.
	return result
}

// ScoreGem calculates a score for a single gem based on a list of priority stats
func ScoreGem(gem Models.Gem, priorities []PriorityStat) float64 {
	var score float64

	// Build simple weight map for current priorities
	// Since we are iterating and dropping, the "first" in the list is always most important for THAT pass.
	// We can use the same decay logic or just simple weighting.
	// Using standard decay based on current position in the sliced list.

	statWeights := make(map[float64]float64)
	for i, pStat := range priorities {
		weight := calculateWeight(Tier1BaseScore, Tier1Decay, i)
		for _, statID := range pStat.StatIDs {
			statWeights[statID] = weight
		}
	}

	for _, stat := range gem.GemStats {
		if weight, exists := statWeights[stat.ID]; exists {
			// Gems have values in nested array too? Yes per GemModel.
			// However, GemStats use Stat struct which has Value []float64
			if len(stat.Value) > 0 {
				score += stat.Value[0] * weight
			}
		}
	}

	return score
}

// calculatePotentialBonusComm calculates the max possible score increase from future slots
func calculatePotentialBonusComm(currentStats *Models.CommStatModel, maxStatsPool []map[float64]float64, prepared PreparedPriority, ceiling *Models.CommStatModel, combatMode string) float64 {
	var totalPotential float64

	for i, pStat := range prepared.Tier1Stats {
		weight := calculateWeight(Tier1BaseScore, Tier1Decay, i)

		statFunc, exists := CommStatGetter[pStat.StatName]
		if !exists {
			continue
		}

		currentVal := statFunc(currentStats)
		ceilVal := statFunc(ceiling)

		var gap float64
		if ceilVal > 0 {
			gap = ceilVal - currentVal
			if gap < 0 {
				gap = 0
			}
		} else {
			gap = 999999
		}

		var maxAvailable float64
		for _, statID := range pStat.StatIDs {
			for _, maxMap := range maxStatsPool {
				maxAvailable += maxMap[statID]
			}
		}

		if maxAvailable > gap {
			maxAvailable = gap
		}

		totalPotential += maxAvailable * weight

		combatStatName := getCombatStatName(pStat.StatName, combatMode)
		if combatStatName != "" {
			if statFunc, exists := CommStatGetter[combatStatName]; exists {
				currentVal := statFunc(currentStats)
				ceilVal := statFunc(ceiling)

				var gap float64
				if ceilVal > 0 {
					gap = ceilVal - currentVal
					if gap < 0 {
						gap = 0
					}
				} else {
					gap = 999999
				}

				var maxAvailable float64
				for _, statID := range pStat.StatIDs {
					for _, maxMap := range maxStatsPool {
						maxAvailable += maxMap[statID]
					}
				}
				if maxAvailable > gap {
					maxAvailable = gap
				}
				totalPotential += maxAvailable * weight
			}
		}
	}
	return totalPotential
}

// calculatePotentialBonusCast calculates the max possible score increase from future slots for Castellan
func calculatePotentialBonusCast(currentStats *Models.CastStatModel, maxStatsPool []map[float64]float64, prepared PreparedPriority, ceiling *Models.CastStatModel, combatMode string) float64 {
	var totalPotential float64

	for i, pStat := range prepared.Tier1Stats {
		weight := calculateWeight(Tier1BaseScore, Tier1Decay, i)

		statFunc, exists := CastStatGetter[pStat.StatName]
		if !exists {
			continue
		}

		currentVal := statFunc(currentStats)
		ceilVal := statFunc(ceiling)

		var gap float64
		if ceilVal > 0 {
			gap = ceilVal - currentVal
			if gap < 0 {
				gap = 0
			}
		} else {
			gap = 999999
		}

		var maxAvailable float64
		for _, statID := range pStat.StatIDs {
			for _, maxMap := range maxStatsPool {
				maxAvailable += maxMap[statID]
			}
		}

		if maxAvailable > gap {
			maxAvailable = gap
		}

		totalPotential += maxAvailable * weight

		combatStatName := getCombatStatName(pStat.StatName, combatMode)
		if combatStatName != "" {
			if statFunc, exists := CastStatGetter[combatStatName]; exists {
				currentVal := statFunc(currentStats)
				ceilVal := statFunc(ceiling)

				var gap float64
				if ceilVal > 0 {
					gap = ceilVal - currentVal
					if gap < 0 {
						gap = 0
					}
				} else {
					gap = 999999
				}

				var maxAvailable float64
				for _, statID := range pStat.StatIDs {
					for _, maxMap := range maxStatsPool {
						maxAvailable += maxMap[statID]
					}
				}
				if maxAvailable > gap {
					maxAvailable = gap
				}
				totalPotential += maxAvailable * weight
			}
		}
	}
	return totalPotential
}

// deduplicateInventory prunes the inventory by keeping only the best item for each unique set of stat IDs.
// "Best" is determined by ScoreEquipment using the provided priorities.
func deduplicateInventory(items []Models.EquipmentModel, priorities PreparedPriority) []Models.EquipmentModel {
	bestBySig := make(map[string]ScoredEquipment)

	for _, item := range items {
		sig := getStatSignature(item)
		score := ScoreEquipment(item, priorities, true) // Score checking EVERYTHING (Tier1 + Tier2)

		if best, exists := bestBySig[sig]; exists {
			if score > best.Score {
				bestBySig[sig] = ScoredEquipment{Equipment: item, Score: score}
			}
		} else {
			bestBySig[sig] = ScoredEquipment{Equipment: item, Score: score}
		}
	}

	deduplicated := make([]Models.EquipmentModel, 0, len(bestBySig))
	for _, best := range bestBySig {
		deduplicated = append(deduplicated, best.Equipment)
	}
	return deduplicated
}

// getStatSignature generates a unique string key based on the sorted StatIDs of the equipment.
func getStatSignature(item Models.EquipmentModel) string {
	ids := make([]float64, 0, len(item.EquipStats))
	for _, stat := range item.EquipStats {
		ids = append(ids, stat.ID)
	}
	sort.Float64s(ids)

	return fmt.Sprint(ids)
}
