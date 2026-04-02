package castle

// CastleFocus is the player castle currently in view in the game client (from JAA).
type CastleFocus struct {
	CastleAID int `json:"castleAID"`
	KingdomID int `json:"kingdomID"`
	MapPX     int `json:"mapPX"`
	MapPY     int `json:"mapPY"`
}
