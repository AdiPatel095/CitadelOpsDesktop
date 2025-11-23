package models

// GemStat defines the structure for a single statistic on a gem.
type GemStat struct {
	ID      int       `json:"id"`
	Percent int       `json:"percent"`
	Value   []float64 `json:"value"`
}

// Gem defines the structure for a gem model.
// It's designed to be parsed from a nested array of values.
type Gem struct {
	ID            int       `json:"id"`
	PlaceHolder23 int       `json:"placeHolder23"`
	PlaceHolder24 int       `json:"placeHolder24"`
	PlaceHolder25 int       `json:"placeHolder25"`
	GemStats      []GemStat `json:"gemStats"`
	GemLevel      int       `json:"gemLevel"`
}

// GemSlot defines the structure for a gem slot, which contains the gem itself.
// This matches the nested array structure like [slotNumber, type, ?, [gem_details]].
type GemSlot struct {
	SlotNumber    int  `json:"slotNumber"`
	PlaceHolder21 int  `json:"placeHolder21"`
	PlaceHolder22 int  `json:"placeHolder22"`
	Gem           *Gem `json:"gem"`
}
