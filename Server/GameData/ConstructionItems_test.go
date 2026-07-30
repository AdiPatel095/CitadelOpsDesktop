package GameData

import (
	"encoding/json"
	"testing"
)

func TestConstructionItemVariantKeyMatchesPickerGrouping(t *testing.T) {
	decode := func(raw string) Record {
		t.Helper()
		record, err := DecodeRecord(json.RawMessage(raw))
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	first := decode(`{"constructionItemID":101,"constructionItemGroupID":18,"name":"AnniversaryDwelling","duration":3600,"effects":"20&5+0,10&2+0","level":1,"slotTypeID":1}`)
	secondTier := decode(`{"constructionItemID":102,"constructionItemGroupID":18,"name":"AnniversaryDwelling","duration":7200,"effects":"10&4+0,20&10+0","level":2,"slotTypeID":1}`)
	differentDesign := decode(`{"constructionItemID":301,"constructionItemGroupID":18,"name":"BlackFridayDwelling","duration":3600,"effects":"10&20+0,20&20+0","level":4,"slotTypeID":1}`)

	if ConstructionItemVariantKey(first) != ConstructionItemVariantKey(secondTier) {
		t.Fatal("tiers with the same picker signature were split")
	}
	if ConstructionItemVariantKey(first) == ConstructionItemVariantKey(differentDesign) {
		t.Fatal("different picker designs sharing an official group ID were merged")
	}
}
