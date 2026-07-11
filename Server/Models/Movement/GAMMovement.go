package movement

// GAMMovement represents a parsed movement from GAM message.
type GAMMovement struct {
	MID          int     `json:"mid"`
	PT           int     `json:"pt"` // Past Time (seconds) — snapshot at parse time, advanced via ReceivedUnix
	TT           int     `json:"tt"` // Total Time (one-way, seconds)
	D            int     `json:"d"`  // Direction (0=out, 1=back)
	KID          int     `json:"kid"`
	SID          int     `json:"sid"`
	OID          int     `json:"oid"`
	MovementType int     `json:"movementType,omitempty"` // M.T; spy missions are type 3.
	SpyCount     int     `json:"spyCount,omitempty"`     // A.S.SC on CSM/GAM spy movements.
	TargetType   int     `json:"targetType"`             // From TA[0] (gaa map-node / target tile type)
	TargetX      int     `json:"targetX"`                // From TA array
	TargetY      int     `json:"targetY"`                // From TA array
	SourceX      int     `json:"sourceX"`                // From SA array
	SourceY      int     `json:"sourceY"`                // From SA array
	CommanderID  int     `json:"commanderID"`            // From UM.L.ID
	TroopArray   [][]int `json:"troopArray"`             // From A field: [[troopID, count], ...]
	// PWD/TWD (UM.PWD/UM.TWD): the "posted" wait window at the destination. A support/bird leg flies out
	// (TT), then sits posted for TWD seconds (PWD = elapsed). PT advances through TT then the wait, so a
	// posted leg has PT ≈ TT+PWD and stays live until TT+TWD. Without this a long bird post is wrongly
	// treated as a completed out-and-back and drops off the list mid-wait.
	PWD int `json:"pwd,omitempty"`
	TWD int `json:"twd,omitempty"`
	// ReceivedUnix is when this leg was parsed; PT is advanced by wall-clock elapsed since then.
	ReceivedUnix int64 `json:"receivedUnix,omitempty"`
}

// EffectivePT returns PT advanced by real time elapsed since the frame was received.
// The game does not push fresh frames for a leg once it stops moving, so PT must be aged locally.
func (m GAMMovement) EffectivePT(nowUnix int64) int {
	if m.ReceivedUnix <= 0 {
		return m.PT
	}
	elapsed := nowUnix - m.ReceivedUnix
	if elapsed < 0 {
		elapsed = 0
	}
	return m.PT + int(elapsed)
}

// IsComplete reports whether a timed leg (TT>0) has finished by wall clock.
func (m GAMMovement) IsComplete(nowUnix int64) bool {
	if m.TT <= 0 {
		return false
	}
	return m.EffectivePT(nowUnix) >= m.TT
}
