package movement

// PlayerMovement groups auto-bird tracking and GAM-derived active movements.
type PlayerMovement struct {
	BirdMovements   map[int][]BirdMovement `json:"birdMovements"`   // CastleID -> active bird movements
	ActiveMovements []GAMMovement          `json:"activeMovements"` // Parsed from GAM message(s); single source of truth for attack busy state
	// CommanderByMID maps a movement MID → commander wire LID learned from the outbound leg, so a return
	// reusing the same MID without UM.L.ID can be re-attached to its commander deterministically.
	CommanderByMID map[int]int `json:"-"`
}
