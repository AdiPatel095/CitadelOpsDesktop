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

var CastActualArray struct {
	MainCastleCast    CastActualModel `json:"mainCastleCast"`
	Outpost1Cast      CastActualModel `json:"outpost1Cast"`
	Outpost2Cast      CastActualModel `json:"outpost2Cast"`
	Outpost3Cast      CastActualModel `json:"outpost3Cast"`
	IceCastleCast     CastActualModel `json:"iceCastleCast"`
	DesertCastleCast  CastActualModel `json:"desertCastleCast"`
	DungeonCastleCast CastActualModel `json:"dungeonCastleCast"`
	StormCastleCast   CastActualModel `json:"stormCastleCast"`
	ExtraCast1        CastActualModel `json:"extraCast1"`
	ExtraCast2        CastActualModel `json:"extraCast2"`
	ExtraCast3        CastActualModel `json:"extraCast3"`
	ExtraCast4        CastActualModel `json:"extraCast4"`
	ExtraCast5        CastActualModel `json:"extraCast5"`
}
