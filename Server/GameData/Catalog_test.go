package GameData

import (
	"encoding/json"
	"fmt"
	"sync"
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

func TestCatalogInt64IndexesOfficialScalarsOnceAndConcurrently(t *testing.T) {
	catalog, err := newCatalog("resources", json.RawMessage(`[
		{"resourceID":"1","JSONKey":"wood","level":"7"},
		{"resourceID":2,"JSONKey":"stone","level":8},
		{"resourceID":"3","JSONKey":"food","level":"invalid"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if value, found := catalog.Int64("1", "level"); !found || value != 7 {
		t.Fatalf("primary scalar = %d, %v; want 7, true", value, found)
	}
	if value, found := catalog.Int64ByField("JSONKey", "stone", "resourceID"); !found || value != 2 {
		t.Fatalf("secondary scalar = %d, %v; want 2, true", value, found)
	}
	if _, found := catalog.Int64("3", "level"); found {
		t.Fatal("invalid scalar unexpectedly resolved")
	}
	if _, found := catalog.Int64ByField("JSONKey", "missing", "resourceID"); found {
		t.Fatal("missing scalar unexpectedly resolved")
	}

	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				if value, found := catalog.Int64("2", "level"); !found || value != 8 {
					t.Errorf("concurrent scalar = %d, %v; want 8, true", value, found)
					return
				}
			}
		}()
	}
	wait.Wait()
}

func TestCatalogFloat64IndexesOfficialScalarsOnceAndConcurrently(t *testing.T) {
	catalog, err := newCatalog("units", json.RawMessage(`[
		{"wodID":"1","foodSupply":"7.5"},
		{"wodID":2,"foodSupply":8},
		{"wodID":"3","foodSupply":"invalid"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if value, found := catalog.Float64("1", "foodSupply"); !found || value != 7.5 {
		t.Fatalf("primary scalar = %v, %v; want 7.5, true", value, found)
	}
	if value, found := catalog.Float64ByField("wodID", "2", "foodSupply"); !found || value != 8 {
		t.Fatalf("selected scalar = %v, %v; want 8, true", value, found)
	}
	if _, found := catalog.Float64("3", "foodSupply"); found {
		t.Fatal("invalid scalar unexpectedly resolved")
	}
	if _, found := catalog.Float64("missing", "foodSupply"); found {
		t.Fatal("missing scalar unexpectedly resolved")
	}

	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				if value, found := catalog.Float64("2", "foodSupply"); !found || value != 8 {
					t.Errorf("concurrent scalar = %v, %v; want 8, true", value, found)
					return
				}
			}
		}()
	}
	wait.Wait()
}
