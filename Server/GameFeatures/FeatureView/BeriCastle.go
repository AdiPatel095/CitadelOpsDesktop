package featureview

import (
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
)

// RegisterBeriCastleDiscovery persists Beri castle CID (fuc) and map tile from GCL KID 10.
func RegisterBeriCastleDiscovery(castleCID, mapX, mapY int) {
	if castleCID <= 0 {
		return
	}
	gs := Models.GetGameState()
	if gs != nil {
		gs.UpsertPlayerCastleLocation(castle.BeriWorldKingdomID, castleCID, mapX, mapY)
		gs.Castle.BeriWorldCastle.Aid = float64(castleCID)
		if mapX > 0 {
			gs.Castle.BeriWorldCastle.MapX = mapX
		}
		if mapY > 0 {
			gs.Castle.BeriWorldCastle.MapY = mapY
		}
		gs.Castle.BeriWorldCastle.MapKingdomID = castle.BeriWorldKingdomID
	}
	st := Models.GetSettingsState()
	if st == nil {
		return
	}
	cfg := st.AutoBeriWorld
	// Do not overwrite a Beri CID the user set in settings; GCL only fills when empty.
	if castleCID > 0 && cfg.BeriCastleCID == 0 {
		cfg.BeriCastleCID = castleCID
	}
	if mapX > 0 {
		cfg.BeriMapX = mapX
	}
	if mapY > 0 {
		cfg.BeriMapY = mapY
	}
	st.UpdateAutoBeriWorldConfig(cfg)
	SyncBeriCastleFromSettings()
	Logging.AutoBeriWorldLogf("discovered", "beri CID=%d map=(%d,%d)", castleCID, mapX, mapY)
}

// RegisterMainCastleKutSource persists kut SCID from the main castle (KID 0 GCL) when settings still empty.
func RegisterMainCastleKutSource(mainCastleAID int) {
	if mainCastleAID <= 0 {
		return
	}
	st := Models.GetSettingsState()
	if st == nil {
		return
	}
	cfg := st.AutoBeriWorld
	if cfg.KutSourceCastleSCID == mainCastleAID {
		return
	}
	if cfg.KutSourceCastleSCID != 0 {
		return
	}
	cfg.KutSourceCastleSCID = mainCastleAID
	st.UpdateAutoBeriWorldConfig(cfg)
	Logging.AutoBeriWorldLogf("discovered", "kut SCID=%d (main castle)", mainCastleAID)
}

// SyncKutSourceFromMainCastle copies MainCastle.Aid into settings when kut SCID is still unset.
func SyncKutSourceFromMainCastle() {
	gs := Models.GetGameState()
	if gs == nil {
		return
	}
	mainAID := int(gs.Castle.MainCastle.Aid)
	if mainAID > 0 {
		RegisterMainCastleKutSource(mainAID)
	}
}

// ResolveKutSourceSCID returns the main-world source castle for kut (main castle instance id).
func ResolveKutSourceSCID() (scid int, ok bool) {
	if gs := Models.GetGameState(); gs != nil {
		if aid := int(gs.Castle.MainCastle.Aid); aid > 0 {
			return aid, true
		}
	}
	if st := Models.GetSettingsState(); st != nil && st.AutoBeriWorld.KutSourceCastleSCID > 0 {
		return st.AutoBeriWorld.KutSourceCastleSCID, true
	}
	return 0, false
}

// SyncBeriCastleFromSettings copies BeriCastleCID from AutoBeriWorld.json into GameState for fuc/kut.
func SyncBeriCastleFromSettings() {
	st := Models.GetSettingsState()
	if st == nil || st.AutoBeriWorld.BeriCastleCID <= 0 {
		return
	}
	cfg := st.AutoBeriWorld
	gs := Models.GetGameState()
	if gs == nil {
		return
	}
	cid := cfg.BeriCastleCID
	gs.Castle.BeriWorldCastle.Aid = float64(cid)
	gs.Castle.BeriWorldCastle.MapKingdomID = castle.BeriWorldKingdomID
	if cfg.BeriMapX > 0 {
		gs.Castle.BeriWorldCastle.MapX = cfg.BeriMapX
	}
	if cfg.BeriMapY > 0 {
		gs.Castle.BeriWorldCastle.MapY = cfg.BeriMapY
	}
	gs.UpsertPlayerCastleLocation(castle.BeriWorldKingdomID, cid, cfg.BeriMapX, cfg.BeriMapY)
}

// ResolveBeriCastle returns the Beri castle instance id and map coords when known.
// Settings BeriCastleCID (manual or saved) wins, then GCL slot, then alliance list (KID 10).
func ResolveBeriCastle() (castleCID, mapX, mapY int, ok bool) {
	st := Models.GetSettingsState()
	if st != nil && st.AutoBeriWorld.BeriCastleCID > 0 {
		cfg := st.AutoBeriWorld
		return cfg.BeriCastleCID, cfg.BeriMapX, cfg.BeriMapY, true
	}
	gs := Models.GetGameState()
	if gs == nil {
		return 0, 0, 0, false
	}
	if aid := int(gs.Castle.BeriWorldCastle.Aid); aid > 0 {
		return aid, gs.Castle.BeriWorldCastle.MapX, gs.Castle.BeriWorldCastle.MapY, true
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.KingdomID == castle.BeriWorldKingdomID && loc.CastleID > 0 {
			return loc.CastleID, loc.X, loc.Y, true
		}
	}
	return 0, 0, 0, false
}
