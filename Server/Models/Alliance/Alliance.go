package alliance

// Alliance represents the player's alliance information.
type Alliance struct {
	AID                   int                    `json:"aid"` // Ally Identification
	Members               []AllianceMember       `json:"members"`
	BirdLocations         []BirdLocation         `json:"birdLocations"`
	PlayerCastleLocations []PlayerCastleLocation `json:"playerCastleLocations"` // Player's own castle locations
}

type AllianceMember struct {
	PlayerID int    `json:"playerID"`
	Name     string `json:"name"`
}

type BirdLocation struct {
	X          int `json:"x"`
	Y          int `json:"y"`
	KingdomID  int `json:"kingdomID"`
	BirdTime   int `json:"birdTime"`
	CastleType int `json:"castleType"` // CastleSlot* — see GameParser.MapCastleTypes.go
}

// PlayerCastleLocation represents one of the player's castle locations.
type PlayerCastleLocation struct {
	KingdomID int `json:"kingdomID"`
	CastleID  int `json:"castleID"`
	X         int `json:"x"`
	Y         int `json:"y"`
}
