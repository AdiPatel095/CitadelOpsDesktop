package GameParser

import mapstate "CitadelDesktop/Server/Models/MapState"

// Map node and castle-slot type constants from GAA/GCL/ain captures.
//
// Refresh type inventory:
//
//	go run ./cmd/ScanGaaAfterTime/  — RECV gaa after cutoff (default 12:49 local / Eastern)
//	go run ./cmd/ScanGaaLog/        — full websocket_game.log
//	go run ./cmd/ScanGaaTypes/      — Logs/JSONExamples/*.json
//
// Wire reference: Logs/JSONExamples/GaaWireExamples.json
// Latest full viewport: Logs/JSONExamples/GaaInboundFresh.json

// --- Outbound **gaa** (client requests map viewport) -----------------------------
//
// Wire: %xt%EmpireEx_21%gaa%1%{"KID":0,"AX1":1196,"AY1":1144,"AX2":1208,"AY2":1156}%
// AX1/AY1 and AX2/AY2 are inclusive map bounds (~13×13 tiles in captures; ~20-tile radius around focus).
// Example: Logs/JSONExamples/GaaOutboundFresh.json

// --- Inbound **gaa** map tile types (AI[i][0]) ---------------------------------
//
// Row layout: [0]=type, [1]=X, [2]=Y, then length-specific tail (see MapParser.ParseGAAMessage).

const (
	// GaaNodeCastleMain — player main-castle tile.
	// len 4: [1,X,Y,-1] stub (no full metadata; common in fresh 2026-06-07 captures).
	// len 20: full castle row with name, levels, instance ids.
	GaaNodeCastleMain = 1

	// GaaNodeKingdomTower — len 7; kingdom tower / cooldown POI. [2,X,Y,-1,towerId,cooldownSec,flag]
	GaaNodeKingdomTower = 2

	// GaaNodeCastleCapital — len 20; capital-class castle tile (sample name "Takinas Capital").
	// Distinct from GaaNodeCastleMain on the map layer; verify vs GCL slot type 6.
	GaaNodeCastleCapital = 3

	// GaaNodeCastleOccupied — len 20; occupied castle / outpost / third-party keep on map.
	// Same type id for outposts and named foreign keeps in viewport (e.g. "OKAVANGO DELTA", "DV2 OP3").
	// Placeholder rows use playerId -300 when tile is reserved but empty.
	GaaNodeCastleOccupied = 4

	// GaaNodeAllianceCamp — len 9; small named alliance structure (sample ".45").
	GaaNodeAllianceCamp = 10

	// GaaNodeUnknown11 — len 8; seen in log scan — label TBD.
	GaaNodeUnknown11 = 11

	// GaaNodeCastleForeignKingdom — len 20; ice/desert/fire/storm castle on main map (sample "Ice Castle").
	// Mirrors CastleSlotForeign (12) on the player AP list, but this is the map-node type id.
	GaaNodeCastleForeignKingdom = 12

	// GaaNodeCastleUnknown22 — len 20; rare castle variant (sample "THE GREAT") — label TBD.
	GaaNodeCastleUnknown22 = 22

	// GaaNodeCoordMarker — len 8; coord label in name field (sample "214:942"). Bird/scout marker?
	GaaNodeCoordMarker = 23

	// GaaNodeUnknown25 — len 9; includes cooldown-like field (14400 in sample) — label TBD.
	GaaNodeUnknown25 = 25

	// GaaNodeMonument — len 10; world monument on the map layer (AI[]), not AP slot type.
	// Confirmed: (695,799) e.g. "RAFAEL MONU".
	GaaNodeMonument = 26

	// GaaNodeNomadCamp — len 12; limited-time nomad camp on KID 0 (Sat–Sun–Mon event).
	// Sample coords: (1164,1160), (1166,1165). Confirmed in-game 2026-06.
	GaaNodeNomadCamp = 27

	// GaaNodeResource — len 9; world resource tile on the map layer (e.g. "Magnetite").
	// Not the same namespace as CastleSlotLaboratory (28) on gaa.OI.AP[].
	GaaNodeResource = 28

	// GaaNodeUnknown29 — len 12; similar shape to type 27 — label TBD.
	GaaNodeUnknown29 = 29

	// GaaNodeTerrainEmpty — len 3: [31,X,Y] plain passable tile (most frequent node in viewport scans).
	GaaNodeTerrainEmpty = 31

	// GaaNodeUnknown34 — len 11; adi.json only (not in 2026-06-07 log batch) — label TBD.
	GaaNodeUnknown34 = 34

	// GaaNodeKhanCamp — len 13; Khan camp on KID 0 (nomad event; same Sat–Sun–Mon as GaaNodeNomadCamp).
	// Confirmed: (512,445) in-game 2026-06.
	GaaNodeKhanCamp = 35

	// GaaNodeUnknown37 — len 12; frequent in dense viewport — label TBD (timed map effect?).
	GaaNodeUnknown37 = 37

	// GaaNodeUnknown38 — len 12; includes 3600 sec field in samples — label TBD (1h cooldown?).
	GaaNodeUnknown38 = 38

	// GaaNodeRift — len 18; Rift POI on KID 0 (confirmed (1159,1163) post-12:49 scan 2026-06-07).
	// Not nomad camp — nomad camp is GaaNodeNomadCamp (27).
	GaaNodeRift = 43
)

// GaaNodeRowLen* documents expected AI row lengths (multiple lengths per type are valid).
const (
	GaaNodeRowLenCastleStub     = 4
	GaaNodeRowLenTower          = 7
	GaaNodeRowLenCooldownMarker = 8
	GaaNodeRowLenNamedOwner     = 9
	GaaNodeRowLenMonument       = 10
	GaaNodeRowLenUnknown34      = 11
	GaaNodeRowLenTimedEffect    = 12
	GaaNodeRowLenKhanCamp       = 13
	GaaNodeRowLenRift           = 18
	GaaNodeRowLenCastle         = 20
	GaaNodeRowLenTerrain        = 3
)

// --- Player castle slot types (GCL AI[] details[0], gaa.OI.AP[][4], ain AP[][4]) ---
//
// AP row: [kingdomId, castleInstanceId, mapX, mapY, castleSlotType].
// For CastleSlotForeign (12), kingdomId is 1=desert, 2=ice, 3=fire, 4=storm.

const (
	CastleSlotMain       = 1  // main castle (KID 0)
	CastleSlotUnknown3   = 3  // rare in AP scan (n=4) — likely metropolis or special; verify
	CastleSlotOutpost    = 4  // outpost (KID 0)
	CastleSlotMetropolis = 5  // GCL KID 0; not in latest AP scan
	CastleSlotCapital    = 6  // GCL KID 0; not in latest AP scan
	CastleSlotForeign    = 12 // ice/desert/dungeon/storm; AP[0]=sub-kingdom
	CastleSlotUnknown22  = 22 // AP scan n=5 — label TBD
	CastleSlotUnknown23  = 23 // AP scan n=27 — label TBD
	CastleSlotUnknown24  = 24 // AP scan n=44 — label TBD
	CastleSlotMonument   = 26 // player monument building; confirmed (695,799) KID 0
	CastleSlotLaboratory = 28 // player laboratory building; confirmed (708,786) KID 0
)

// GaaCastleNodeTypes are gaa AI[0] values that use the len-20 castle row parser branch.
var GaaCastleNodeTypes = map[int]string{
	GaaNodeCastleMain:           "main",
	GaaNodeCastleCapital:        "capital",
	GaaNodeCastleOccupied:       "occupied/outpost",
	GaaNodeCastleForeignKingdom: "foreign kingdom",
	GaaNodeCastleUnknown22:      "unknown-22",
}

// GaaNodeTypeLabels re-exports mapstate labels for callers in GameParser.
var GaaNodeTypeLabels = mapstate.GaaNodeTypeLabels

// CastleSlotTypeLabels re-exports AP/GCL slot labels.
var CastleSlotTypeLabels = mapstate.CastleSlotTypeLabels

// IsBirdTargetCastleSlot returns true for ain/AP castle types AutoBird treats as valid bird posts.
func IsBirdTargetCastleSlot(ct int) bool {
	switch ct {
	case 0, CastleSlotMain, CastleSlotOutpost, CastleSlotForeign:
		return true
	default:
		return false
	}
}

// LabelGaaNodeType returns a human label for a gaa map-node type id.
func LabelGaaNodeType(typeID int) string {
	return mapstate.LabelGaaNodeType(typeID)
}

// LabelCastleSlotType returns a human label for AP/GCL castle slot type id.
func LabelCastleSlotType(typeID int) string {
	return mapstate.LabelCastleSlotType(typeID)
}
