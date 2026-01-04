package Models

// CastModel defines the structure for a cast model.
type CastActualModel struct {
	ID            float64          `json:"id"`
	PlaceHolder12 float64          `json:"placeHolder12"`
	PlaceHolder14 float64          `json:"PlaceHolder14"`
	CastleID      float64          `json:"castleID"`
	Name          string           `json:"name"`
	PlaceHolder15 float64          `json:"placeHolder15"`
	PlaceHolder16 float64          `json:"placeHolder16"`
	PlaceHolder17 float64          `json:"placeHolder17"`
	PlaceHolder18 float64          `json:"placeHolder18"`
	PlaceHolder19 float64          `json:"placeHolder19"`
	PlaceHolder20 float64          `json:"placeHolder20"`
	Equipment     []EquipmentModel `json:"equipment"`
}
