package State

import (
	"fmt"
	"time"
)

// StormPackageCapBlock is scoped to the owned Storm castle and Luna table.
// Session reconnects do not establish a new event occurrence.
type StormPackageCapBlock struct {
	CastleID  CastleID  `json:"castleId"`
	TableID   int64     `json:"tableId"`
	PackageID PackageID `json:"packageId"`
	Cap       int64     `json:"cap"`
	BlockedAt time.Time `json:"blockedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func StormShopOccurrenceKey(castleID CastleID, tableID int64) string {
	return fmt.Sprintf("%d:%d", castleID, tableID)
}

func stormPackageCapKey(castleID CastleID, tableID int64, packageID PackageID) string {
	return fmt.Sprintf("%s:%d", StormShopOccurrenceKey(castleID, tableID), packageID)
}

func (state *GameState) MutableStormPackageCapBlocks() map[string]StormPackageCapBlock {
	if state.stormMutationCOW && state.mutableStormParts&stormPackageCapBlocksMutable == 0 {
		state.Storm.PackageCapBlocks = cloneMap(state.Storm.PackageCapBlocks)
		state.mutableStormParts |= stormPackageCapBlocksMutable
	}
	if state.Storm.PackageCapBlocks == nil {
		state.Storm.PackageCapBlocks = map[string]StormPackageCapBlock{}
	}
	return state.Storm.PackageCapBlocks
}

func (state *GameState) BlockStormPackage(castleID CastleID, tableID int64, packageID PackageID, cap int64, observedAt time.Time) {
	blocks := state.MutableStormPackageCapBlocks()
	// The authoritative event end will be supplied by CIT-122. Until then the
	// bounded fallback is seven days; an omitted counter cannot shorten it.
	for key, block := range blocks {
		if !observedAt.Before(block.ExpiresAt) {
			delete(blocks, key)
		}
	}
	blocks[stormPackageCapKey(castleID, tableID, packageID)] = StormPackageCapBlock{
		CastleID: castleID, TableID: tableID, PackageID: packageID, Cap: cap,
		BlockedAt: observedAt, ExpiresAt: observedAt.Add(7 * 24 * time.Hour),
	}
}

func (state *GameState) StormPackageBlocked(castleID CastleID, tableID int64, packageID PackageID, now time.Time) bool {
	block, found := state.Storm.PackageCapBlocks[stormPackageCapKey(castleID, tableID, packageID)]
	return found && (now.IsZero() || now.Before(block.ExpiresAt))
}

// ReconcileStormPackageCaps accepts only an explicit below-cap observation
// newer than the rejection, in the same purchase-history scope.
func (state *GameState) ReconcileStormPackageCaps(castleID CastleID, tableID int64, offers map[PackageID]int64, observedAt time.Time) bool {
	changed := false
	for key, block := range state.Storm.PackageCapBlocks {
		count, listed := offers[block.PackageID]
		expired := !observedAt.IsZero() && !observedAt.Before(block.ExpiresAt)
		reset := block.CastleID == castleID && block.TableID == tableID &&
			observedAt.After(block.BlockedAt) && listed && count >= 0 && block.Cap > 0 && count < block.Cap
		if expired || reset {
			delete(state.MutableStormPackageCapBlocks(), key)
			changed = true
		}
	}
	return changed
}
