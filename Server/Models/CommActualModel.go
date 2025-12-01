package Models

// CommModel defines the structure for a comm model.
type CommActualModel struct {
	ID              float64          `json:"id"`
	PlaceHolder12   float64          `json:"placeHolder12"`
	VisiblePosition float64          `json:"visiblePosition"`
	Name            string           `json:"name"`
	PlaceHolder14   float64          `json:"placeHolder14"`
	PlaceHolder15   float64          `json:"placeHolder15"`
	PlaceHolder16   float64          `json:"placeHolder16"`
	PlaceHolder17   float64          `json:"placeHolder17"`
	Equipment       []EquipmentModel `json:"equipment"`
}

var CommActualArray []CommActualModel
