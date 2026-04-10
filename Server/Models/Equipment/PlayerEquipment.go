package equipment

// PlayerEquipment groups equipment storage, gems, and commander/castellan actual models from GLI/GEI-style payloads.
type PlayerEquipment struct {
	EquipmentStorage []EquipmentModel
	GemsStorage      []Gem
	NonRelicGemIDs   map[float64]float64
	CommActualArray  []CommActualModel
	CastActualArray  []CastActualModel
}
