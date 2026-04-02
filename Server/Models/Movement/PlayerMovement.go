package movement

// PlayerMovement groups auto-bird tracking and GAM-derived active movements.
type PlayerMovement struct {
	BirdMovements   map[int][]BirdMovement // CastleID -> active bird movements
	ActiveMovements []GAMMovement          // Parsed from GAM message(s)
}
