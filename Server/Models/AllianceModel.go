package Models

// Alliance represents the player's alliance information
type Alliance struct {
	AID                   int                    `json:"aid"` // Ally Identification
	BirdLocations         []BirdLocation         `json:"birdLocations"`
	PlayerCastleLocations []PlayerCastleLocation `json:"playerCastleLocations"` // Player's own castle locations
}

type BirdLocation struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	KingdomID  int `json:"kingdomID"`
	BirdTime   int `json:"birdTime"`
	CastleType int `json:"castleType"` // 1=main, 4=outpost, 12=KW castle
}

// GAMMovement represents a parsed movement from GAM message
type GAMMovement struct {
	MID         int     `json:"mid"`
	PT          int     `json:"pt"` // Past Time (seconds)
	TT          int     `json:"tt"` // Total Time (one-way, seconds)
	D           int     `json:"d"`  // Direction (0=out, 1=back)
	KID         int     `json:"kid"`
	SID         int     `json:"sid"`
	OID         int     `json:"oid"`
	TargetX     int     `json:"targetX"`     // From TA array
	TargetY     int     `json:"targetY"`     // From TA array
	SourceX     int     `json:"sourceX"`     // From SA array
	SourceY     int     `json:"sourceY"`     // From SA array
	CommanderID int     `json:"commanderID"` // From UM.L.ID
	TroopArray  [][]int `json:"troopArray"`  // From A field: [[troopID, count], ...]
}

// PlayerCastleLocation represents one of the player's castle locations
type PlayerCastleLocation struct {
	KingdomID int `json:"kingdomID"`
	CastleID  int `json:"castleID"`
	X         int `json:"x"`
	Y         int `json:"y"`
}
