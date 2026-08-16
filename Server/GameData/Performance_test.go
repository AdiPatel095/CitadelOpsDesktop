package GameData

import (
	"os"
	"strconv"
	"testing"
)

func benchmarkOfficialStore(benchmark *testing.B) *Store {
	benchmark.Helper()
	path := os.Getenv("CITADEL_BENCHMARK_ITEMS_PATH")
	if path == "" {
		path = "../../Data/Profiles/Amos_Burton/GameData/Items/Items-v782.08.json"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		benchmark.Skipf("read official item benchmark data: %v", err)
	}
	store, err := DecodeStore(raw, SourceMetadata{ItemVersion: "benchmark"})
	if err != nil {
		benchmark.Fatal(err)
	}
	return store
}

func benchmarkStormIsleRaw(benchmark *testing.B, store *Store) ([]byte, int64) {
	benchmark.Helper()
	catalog, err := store.Catalog("isles")
	if err != nil {
		benchmark.Fatal(err)
	}
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		definition, valid := decodeStormIsle(record)
		if valid {
			return raw, definition.ID
		}
	}
	benchmark.Skip("official catalog has no supported Storm isle")
	return nil, 0
}

func BenchmarkStormIsleLegacyDecodeCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	raw, _ := benchmarkStormIsleRaw(benchmark, store)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		record, err := DecodeRecord(raw)
		if err != nil {
			benchmark.Fatal(err)
		}
		if _, found := decodeStormIsle(record); !found {
			benchmark.Fatal("Storm isle disappeared")
		}
	}
}

func BenchmarkStormIsleViewCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	_, id := benchmarkStormIsleRaw(benchmark, store)
	if _, found := store.StormIsleView(id); !found {
		benchmark.Fatal("Storm isle is missing")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := store.StormIsleView(id); !found {
			benchmark.Fatal("Storm isle disappeared")
		}
	}
}

func BenchmarkCraftingRecipeLegacyDecodeCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	definitions := store.CraftingRecipesView()
	if len(definitions) == 0 {
		benchmark.Skip("official catalog has no valid crafting recipe")
	}
	catalog, err := store.Catalog("craftingRecipes")
	if err != nil {
		benchmark.Fatal(err)
	}
	raw, found := catalog.Find(strconv.FormatInt(definitions[0].ID, 10))
	if !found {
		benchmark.Fatal("crafting recipe is missing")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			benchmark.Fatal(decodeErr)
		}
		for field := range record {
			if len(field) >= 4 && field[:4] == "cost" {
				_, _ = record.Float64(field)
			}
		}
	}
}

func BenchmarkCraftingRecipeViewCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	definitions := store.CraftingRecipesView()
	if len(definitions) == 0 {
		benchmark.Skip("official catalog has no valid crafting recipe")
	}
	id := definitions[0].ID
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := store.CraftingRecipeView(id); !found {
			benchmark.Fatal("crafting recipe disappeared")
		}
	}
}

func BenchmarkResourceLookupLegacyScanCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, err := store.Catalog("resources")
	if err != nil {
		benchmark.Fatal(err)
	}
	rows := catalog.Rows()
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		found := false
		for _, raw := range rows {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			jsonKey, _ := record.String("JSONKey")
			if jsonKey == "C1" {
				found = true
				break
			}
		}
		if !found {
			benchmark.Fatal("C1 resource disappeared")
		}
	}
}

func BenchmarkResourceLookupCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	if _, found := store.ResourceIDForJSONKey("C1"); !found {
		benchmark.Fatal("C1 resource is missing")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := store.ResourceIDForJSONKey("C1"); !found {
			benchmark.Fatal("C1 resource disappeared")
		}
	}
}

func benchmarkEquipmentEffectRaw(benchmark *testing.B, store *Store) (*Catalog, []byte, string) {
	benchmark.Helper()
	catalog, err := store.Catalog("equipment_effects")
	if err != nil {
		benchmark.Fatal(err)
	}
	primaryKey := catalog.Summary().PrimaryKey
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, validID := scalarKey(record[primaryKey])
		value, validValue := record.Int64("effectID")
		if validID && validValue && value > 0 {
			return catalog, raw, id
		}
	}
	benchmark.Skip("official catalog has no supported equipment effect")
	return nil, nil, ""
}

func BenchmarkEquipmentEffectLegacyDecodeCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	_, raw, _ := benchmarkEquipmentEffectRaw(benchmark, store)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		record, err := DecodeRecord(raw)
		if err != nil {
			benchmark.Fatal(err)
		}
		if _, found := record.Int64("effectID"); !found {
			benchmark.Fatal("equipment effect disappeared")
		}
	}
}

func BenchmarkEquipmentEffectScalarCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, _, id := benchmarkEquipmentEffectRaw(benchmark, store)
	if _, found := catalog.Int64(id, "effectID"); !found {
		benchmark.Fatal("equipment effect is missing")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := catalog.Int64(id, "effectID"); !found {
			benchmark.Fatal("equipment effect disappeared")
		}
	}
}

func benchmarkUnitSupplyRaw(benchmark *testing.B, store *Store) (*Catalog, []byte, string) {
	benchmark.Helper()
	catalog, err := store.Catalog("units")
	if err != nil {
		benchmark.Fatal(err)
	}
	primaryKey := catalog.Summary().PrimaryKey
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, validID := scalarKey(record[primaryKey])
		value, validValue := record.Float64("foodSupply")
		if validID && validValue && value > 0 {
			return catalog, raw, id
		}
	}
	benchmark.Skip("official catalog has no unit food-supply scalar")
	return nil, nil, ""
}

func BenchmarkUnitSupplyLegacyDecodeCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	_, raw, _ := benchmarkUnitSupplyRaw(benchmark, store)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		record, err := DecodeRecord(raw)
		if err != nil {
			benchmark.Fatal(err)
		}
		if _, found := record.Float64("foodSupply"); !found {
			benchmark.Fatal("unit food supply disappeared")
		}
	}
}

func BenchmarkUnitSupplyScalarCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, _, id := benchmarkUnitSupplyRaw(benchmark, store)
	if _, found := catalog.Float64(id, "foodSupply"); !found {
		benchmark.Fatal("unit food supply is missing")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := catalog.Float64(id, "foodSupply"); !found {
			benchmark.Fatal("unit food supply disappeared")
		}
	}
}

func BenchmarkConstructionCatalogLegacyDecodeCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, err := store.Catalog("constructionItems")
	if err != nil {
		benchmark.Fatal(err)
	}
	benchmark.ReportMetric(float64(len(catalog.rows)), "rows")
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		count := 0
		for _, raw := range catalog.rows {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, _ := record.Int64("constructionItemID")
			groupID, _ := record.Int64("constructionItemGroupID")
			_, _ = record.Int64("level")
			_, _ = record.Int64("slotTypeID")
			if id > 0 && groupID > 0 && ConstructionItemVariantKey(record) != "" {
				count++
			}
		}
		if count == 0 {
			benchmark.Fatal("construction item catalog disappeared")
		}
	}
}

func BenchmarkConstructionCatalogViewCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, err := store.ConstructionItemCatalog()
	if err != nil {
		benchmark.Fatal(err)
	}
	var id int64
	for candidate := range catalog.byID {
		id = candidate
		break
	}
	if id <= 0 {
		benchmark.Skip("official catalog has no construction items")
	}
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		definition, found := catalog.DefinitionView(id)
		if !found || len(catalog.TiersView(definition.VariantKey)) == 0 {
			benchmark.Fatal("construction item disappeared")
		}
	}
}

func BenchmarkUnitKindLegacyDecodeCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, err := store.Catalog("units")
	if err != nil || len(catalog.rows) == 0 {
		benchmark.Skip("official unit catalog is unavailable")
	}
	raw := catalog.rows[0]
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			benchmark.Fatal(decodeErr)
		}
		_ = IsToolRecord(record)
	}
}

func BenchmarkUnitKindViewCurrentData(benchmark *testing.B) {
	store := benchmarkOfficialStore(benchmark)
	catalog, err := store.Catalog("units")
	if err != nil || len(catalog.rows) == 0 {
		benchmark.Skip("official unit catalog is unavailable")
	}
	record, err := DecodeRecord(catalog.rows[0])
	if err != nil {
		benchmark.Fatal(err)
	}
	id, found := record.Int64("wodID")
	if !found {
		benchmark.Skip("official unit has no id")
	}
	_, _ = store.UnitIsTool(id)
	benchmark.ReportAllocs()
	benchmark.ResetTimer()
	for benchmark.Loop() {
		if _, found := store.UnitIsTool(id); !found {
			benchmark.Fatal("unit kind disappeared")
		}
	}
}
