package GameData

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCatalogFindByFieldIndexesSparseFieldOutsideSchemaSample(t *testing.T) {
	rows := make([]json.RawMessage, 513)
	for index := range rows {
		rows[index] = json.RawMessage(fmt.Sprintf(`{"achievementID":"%d"}`, index+1))
	}
	rows[512] = json.RawMessage(`{"achievementID":"1087","unlocksDifficulty":"8"}`)
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := newCatalog("achievements", raw)
	if err != nil {
		t.Fatal(err)
	}
	if containsString(catalog.fields, "unlocksDifficulty") {
		t.Fatal("sparse field unexpectedly appeared in sampled schema")
	}
	found, ok := catalog.FindByField("unlocksDifficulty", "8")
	if !ok {
		t.Fatal("sparse field lookup did not find difficulty unlock")
	}
	record, err := DecodeRecord(found)
	if err != nil {
		t.Fatal(err)
	}
	achievementID, ok := record.Int64("achievementID")
	if !ok || achievementID != 1087 {
		t.Fatalf("achievementID = %d, %v; want 1087, true", achievementID, ok)
	}
}
