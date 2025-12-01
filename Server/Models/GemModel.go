package Models

// GemStat defines the structure for a single statistic on a gem.

// Gem defines the structure for a gem model.
// It's designed to be parsed from a nested array of values.
type Gem struct {
	ID            float64 `json:"id"`
	PlaceHolder23 float64 `json:"placeHolder23"`
	PlaceHolder24 float64 `json:"placeHolder24"`
	PlaceHolder25 float64 `json:"placeHolder25"`
	GemStats      []Stat  `json:"gemStats"`
	GemLevel      float64 `json:"gemLevel"`
}

// GemSlot defines the structure for a gem slot, which contains the gem itself.
// This matches the nested array structure like [slotNumber, type, ?, [gem_details]].
type GemSlot struct {
	SlotNumber    float64 `json:"slotNumber"`
	PlaceHolder21 float64 `json:"placeHolder21"`
	PlaceHolder22 float64 `json:"placeHolder22"`
	Gem           *Gem    `json:"gem"`
}

var GemsStorage []Gem
