package movement

import "time"

// BirdMovement represents a tracked bird movement for auto bird logic
type BirdMovement struct {
	MovementID        int       `json:"movementID"`
	CastleID          int       `json:"castleID"`
	ReturnTime        time.Time `json:"returnTime"`
	OneWayTimeSeconds int       `json:"oneWayTimeSeconds"`
	DelayHrs          int       `json:"delayHrs"`
}
