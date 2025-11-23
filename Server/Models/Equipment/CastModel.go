package models

// CastModel defines the structure for a cast model.
type CastModel struct {
	ID             int              `json:"id"`
	PlaceHolder12  int              `json:"placeHolder12"`
	CastlePosition int              `json:"castlePosition"`
	PlaceHolder14  int              `json:"placeHolder14"`
	Name           string           `json:"name"`
	PlaceHolder15  int              `json:"placeHolder15"`
	PlaceHolder16  int              `json:"placeHolder16"`
	PlaceHolder17  int              `json:"placeHolder17"`
	PlaceHolder18  int              `json:"placeHolder18"`
	PlaceHolder19  int              `json:"placeHolder19"`
	PlaceHolder20  int              `json:"placeHolder20"`
	Equipment      []EquipmentModel `json:"equipment"`
}
