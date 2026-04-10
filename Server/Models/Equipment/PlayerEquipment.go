package equipment

// PlayerEquipment groups equipment storage, gems, and commander/castellan actual models from GLI/GEI-style payloads.
type PlayerEquipment struct {
	EquipmentStorage []EquipmentModel    `json:"equipmentStorage"`
	GemsStorage      []Gem               `json:"gemsStorage"`
	NonRelicGemIDs   map[float64]float64 `json:"nonRelicGemIds"`
	CommActualArray  []CommActualModel   `json:"commActualArray"`
	CastActualArray  []CastActualModel   `json:"castActualArray"`
}
