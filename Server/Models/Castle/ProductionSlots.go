package castle

// BarracksProductionSlot is one **PS** (active) or **QS[].P** entry from Empire **spl** / **bup**.spl.
// WID/TUA identify the recipe or unit type; PID/SPID distinguish concurrent batches (e.g. refinery “manual” orders).
type BarracksProductionSlot struct {
	WID  int `json:"wid"`
	TUA  int `json:"tua"`
	RCT  int `json:"rct,omitempty"`
	ICT  int `json:"ict,omitempty"`
	PID  int `json:"pid,omitempty"`
	SPID int `json:"spid,omitempty"`
}

// BarracksProductionQueue holds one **spl** strip for a single LID on this castle (see SlotProductionByLID).
type BarracksProductionQueue struct {
	LID           int                      `json:"lid"`
	Active        *BarracksProductionSlot  `json:"active,omitempty"`
	Queued        []BarracksProductionSlot `json:"queued"`
	QueueCapacity int                      `json:"queueCapacity,omitempty"`
	VIPSlots      int                      `json:"vipSlots,omitempty"`
	TCT           int                      `json:"tct,omitempty"`
}
