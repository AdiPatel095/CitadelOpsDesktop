package equipment

// Stat defines the structure for an equipment's statistics.
type Stat struct {
	ID      float64   `json:"id"`
	Percent float64   `json:"percent"`
	Value   []float64 `json:"value"`
}

// Equipment defines the structure for an equipment model.
type EquipmentModel struct {
	ID              float64 `json:"id"`
	EquipSlotNumber float64 `json:"equipSlotNumber"`
	EquipType       float64 `json:"equipType"`
	EquipRarity     float64 `json:"equipRarity"`
	PlaceHolder6    float64 `json:"placeHolder6"`
	EquipStats      []Stat  `json:"equipStats"`
	PlaceHolder7    float64 `json:"placeHolder7"`
	PlaceHolder8    float64 `json:"placeHolder8"`
	EquipLevel      float64 `json:"equipLevel"`
	PlaceHolder9    float64 `json:"placeHolder9"`
	PlaceHolder10   float64 `json:"placeHolder10"`
	PlaceHolder11   float64 `json:"placeHolder11"`
	GemSlot         GemSlot `json:"gem"`
}
