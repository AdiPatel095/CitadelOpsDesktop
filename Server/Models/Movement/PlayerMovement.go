package movement

// SDIContext stores the last parsed SDI context for a given source castle.
// It is used to populate required fields for subsequent CDS sends.
type SDIContext struct {
	LID          int   `json:"lid"`          // route/context id used by CDS
	ReceivedUnix int64 `json:"receivedUnix"` // unix nanos when last updated
}

// PlayerMovement groups auto-bird tracking and GAM-derived active movements.
type PlayerMovement struct {
	BirdMovements   map[int][]BirdMovement `json:"birdMovements"`   // CastleID -> active bird movements
	ActiveMovements []GAMMovement          `json:"activeMovements"` // Parsed from GAM message(s)
	LastSDI         map[int]SDIContext     `json:"lastSdi"`         // SourceCastleID -> last SDI context
}
