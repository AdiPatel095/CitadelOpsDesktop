package castle

// CastleTroops represents troop counts for a single castle location
type CastleTroops struct {
	KingdomID   int            `json:"kingdomID"`
	CastleID    int            `json:"castleID"`
	X           int            `json:"x"`
	Y           int            `json:"y"`
	Troops      map[int]int    `json:"troops"`      // unitID -> count (troops only, no tools) - Legacy/I
	TroopsI     map[int]int    `json:"troopsI"`     // Units currently in castle
	TroopsTU    map[int]int    `json:"troopsTU"`    // Units travelling
	TroopsHI    map[int]int    `json:"troopsHI"`    // Units in hospital
	TroopsSHI   map[int]int    `json:"troopsSHI"`   // Units in special hospital
	TroopsMixed map[int]int    `json:"troopsMixed"` // Combined I + TU
	BGRows      []BuildingData `json:"bgRows,omitempty"`
	BDRows      []BuildingData `json:"bdRows,omitempty"`
}
