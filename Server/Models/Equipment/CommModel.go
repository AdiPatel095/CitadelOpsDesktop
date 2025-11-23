package models

// CommModel defines the structure for a comm model.
type CommModel struct {
	ID            int              `json:"id"`
	PlaceHolder12 int              `json:"placeHolder12"`
	PlaceHolder13 int              `json:"placeHolder13"`
	Name          string           `json:"name"`
	PlaceHolder14 int              `json:"placeHolder14"`
	PlaceHolder15 int              `json:"placeHolder15"`
	PlaceHolder16 int              `json:"placeHolder16"`
	PlaceHolder17 int              `json:"placeHolder17"`
	Equipment     []EquipmentModel `json:"equipment"`
}
