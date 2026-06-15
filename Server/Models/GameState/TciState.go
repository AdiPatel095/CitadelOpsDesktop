package gamestate

import "time"

// GbcCIPLRow is one **gbc** product line (PID, AMT) from EmpireEx_21 **gbc** payload field PL.
type GbcCIPLRow struct {
	PID int `json:"pid"`
	AMT int `json:"amt"`
}

// TciSession holds ephemeral AutoTCI purchase/equip scratch data (SIN, last gbc). Cleared on [GameState.Reset].
// Not the same as gca.CI (equipped); this is account stash + shop list.
type TciSession struct {
	// SINItemCounts is merged from **sin** opcode and embedded **sin** on envelopes (e.g. **jaa**): all RD rows.
	// When checking construction CIDs, only treat ids that you know are constructionItemIDs (catalog), not building wodIDs.
	SINItemCounts map[int]int `json:"sinItemCounts,omitempty"`
	// CIInventoryCounts is the account construction-item bag from **gii** root CI [[wireCID,count],…] (not gca.CI slots).
	CIInventoryCounts map[int]int `json:"ciInventoryCounts,omitempty"`
	// GbcTrivialCIPL is the last **gbc** PL array (replaced on each gbc).
	GbcTrivialCIPL  []GbcCIPLRow `json:"gbcTrivialCIPL,omitempty"`
	GbcCIPLAtMillis int64        `json:"gbcCIPLAtMillis,omitempty"`
	// SINAtMillis throttles explicit SendSIN requests from AutoTCI.
	SINAtMillis int64 `json:"sinAtMillis,omitempty"`
}

// ReplaceGbcTrivialCIPL sets the last gbc product list; PL JSON is parsed in GameParser and applied from MessageRouter.
func (gs *GameState) ReplaceGbcTrivialCIPL(rows []GbcCIPLRow, atMillis int64) {
	if gs == nil {
		return
	}
	if len(rows) == 0 {
		gs.Tci.GbcTrivialCIPL = nil
	} else {
		gs.Tci.GbcTrivialCIPL = append([]GbcCIPLRow(nil), rows...)
	}
	gs.Tci.GbcCIPLAtMillis = atMillis
}

// ReplaceSINItemCountsFromMap replaces the SIN map entirely (tests or explicit reset).
func (gs *GameState) ReplaceSINItemCountsFromMap(m map[int]int) {
	if gs == nil {
		return
	}
	gs.Tci.SINItemCounts = m
	gs.Tci.SINAtMillis = time.Now().UnixMilli()
}

// MergeSINItemCountsFromMap updates counts for keys present in m and leaves other keys unchanged.
func (gs *GameState) MergeSINItemCountsFromMap(m map[int]int) {
	if gs == nil || len(m) == 0 {
		return
	}
	if gs.Tci.SINItemCounts == nil {
		gs.Tci.SINItemCounts = make(map[int]int, len(m))
	}
	for k, v := range m {
		gs.Tci.SINItemCounts[k] = v
	}
	gs.Tci.SINAtMillis = time.Now().UnixMilli()
}

// ReplaceCIInventoryCountsFromMap replaces the **gii** construction inventory bag (authoritative for that opcode).
func (gs *GameState) ReplaceCIInventoryCountsFromMap(m map[int]int) {
	if gs == nil {
		return
	}
	if len(m) == 0 {
		gs.Tci.CIInventoryCounts = nil
		return
	}
	cp := make(map[int]int, len(m))
	for k, v := range m {
		cp[k] = v
	}
	gs.Tci.CIInventoryCounts = cp
}

// MergeCIInventoryDelta adjusts the in-memory **gii** bag (e.g. after **sbp** before the next server **gii**).
func (gs *GameState) MergeCIInventoryDelta(cid int, delta int) {
	if gs == nil || cid <= 0 || delta == 0 {
		return
	}
	if gs.Tci.CIInventoryCounts == nil {
		gs.Tci.CIInventoryCounts = make(map[int]int)
	}
	n := gs.Tci.CIInventoryCounts[cid] + delta
	if n <= 0 {
		delete(gs.Tci.CIInventoryCounts, cid)
	} else {
		gs.Tci.CIInventoryCounts[cid] = n
	}
	if len(gs.Tci.CIInventoryCounts) == 0 {
		gs.Tci.CIInventoryCounts = nil
	}
}

// CIInventoryCountForCID returns the count from the last inbound **gii** construction inventory (root CI pairs).
// Temporary construction items are not tracked via **sin**; use this for AutoTCI “already own” checks.
func (gs *GameState) CIInventoryCountForCID(cid int) int {
	if gs == nil || gs.Tci.CIInventoryCounts == nil {
		return 0
	}
	return gs.Tci.CIInventoryCounts[cid]
}

// GbcCIPLFindAMT returns AMT for a product id in the last gbc list (0, false) if not found.
func (gs *GameState) GbcCIPLFindAMT(productID int) (int, bool) {
	if gs == nil {
		return 0, false
	}
	for i := range gs.Tci.GbcTrivialCIPL {
		if gs.Tci.GbcTrivialCIPL[i].PID == productID {
			amt := gs.Tci.GbcTrivialCIPL[i].AMT
			if amt <= 0 {
				amt = 1
			}
			return amt, true
		}
	}
	return 0, false
}
