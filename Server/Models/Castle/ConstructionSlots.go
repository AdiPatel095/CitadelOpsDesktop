package castle

// GCAConstructionBuilding is one element of JAA gca.CI: construction items (TCI) slotted
// on a single building instance (OID matches gca.BD/BG row [1]).
type GCAConstructionBuilding struct {
	OID   int                   `json:"oid"`
	Slots []GCAConstructionSlot `json:"slots"`
}

// GCAConstructionSlot is CIL: CID is construction_items constructionItemID (not BD/BG wodID).
// S is a slot index / game flags (often 0). RemainingSec is from wire "RS" when set: seconds left on
// a timed TCI (counts down in rpc/ubc/jaa). Level is filled from items.json by CID.
type GCAConstructionSlot struct {
	CID          int  `json:"cid"`
	S            int  `json:"s"`
	RemainingSec *int `json:"remainingSec,omitempty"`
	Level        int  `json:"level,omitempty"`
}
