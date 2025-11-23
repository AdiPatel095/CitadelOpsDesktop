package models

// EquipStat defines the structure for an equipment's statistics.
type EquipStat struct {
	ID      int       `json:"id"`
	Percent int       `json:"percent"`
	Value   []float64 `json:"value"`
}

// Equipment defines the structure for an equipment model.
type EquipmentModel struct {
	ID              int         `json:"id"`
	EquipSlotNumber int         `json:"equipSlotNumber"`
	EquipType       int         `json:"equipType"`
	EquipRarity     int         `json:"equipRarity"`
	PlaceHolder6    int         `json:"placeHolder6"`
	EquipStats      []EquipStat `json:"equipStats"`
	PlaceHolder7    int         `json:"placeHolder7"`
	PlaceHolder8    int         `json:"placeHolder8"`
	EquipLevel      int         `json:"equipLevel"`
	PlaceHolder9    int         `json:"placeHolder9"`
	PlaceHolder10   int         `json:"placeHolder10"`
	PlaceHolder11   int         `json:"placeHolder11"`
	GemSlot         *GemSlot    `json:"gem"`
}
