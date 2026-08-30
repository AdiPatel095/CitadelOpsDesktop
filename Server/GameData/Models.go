package GameData

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SourceMetadata struct {
	ItemVersion     string    `json:"itemVersion"`
	Language        string    `json:"language,omitempty"`
	LanguageVersion string    `json:"languageVersion,omitempty"`
	SourceURL       string    `json:"sourceUrl"`
	DigestSHA256    string    `json:"digestSha256"`
	FetchedAt       time.Time `json:"fetchedAt"`
	LoadedAt        time.Time `json:"loadedAt"`
}

type CatalogSummary struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Count      int      `json:"count"`
	PrimaryKey string   `json:"primaryKey,omitempty"`
	Fields     []string `json:"fields,omitempty"`
}

type Record map[string]json.RawMessage

func DecodeRecord(raw json.RawMessage) (Record, error) {
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	return record, nil
}

func (record Record) String(field string) (string, bool) {
	raw, ok := record[field]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func (record Record) Int64(field string) (int64, bool) {
	raw, ok := record[field]
	if !ok {
		return 0, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	switch typed := value.(type) {
	case json.Number:
		integer, err := typed.Int64()
		return integer, err == nil
	case string:
		integer, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return integer, err == nil
	default:
		return 0, false
	}
}

func (record Record) Float64(field string) (float64, bool) {
	raw, ok := record[field]
	if !ok {
		return 0, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	switch typed := value.(type) {
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

type Catalog struct {
	name         string
	primaryKey   string
	fields       []string
	rows         []json.RawMessage
	indexOnce    sync.Once
	index        map[string]int
	secondaryMu  sync.Mutex
	secondary    map[string]map[string]int
	int64Mu      sync.Mutex
	int64Index   map[catalogInt64IndexKey]map[string]int64
	float64Mu    sync.Mutex
	float64Index map[catalogInt64IndexKey]map[string]float64
}

type catalogInt64IndexKey struct {
	lookupField string
	valueField  string
}

func newCatalog(name string, raw json.RawMessage) (*Catalog, error) {
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("decode catalog %s: %w", name, err)
	}
	fields, primaryKey := discoverSchema(rows)
	if configured := collectionPrimaryKeys[name]; configured != "" && containsString(fields, configured) {
		primaryKey = configured
	}
	return &Catalog{
		name: name, primaryKey: primaryKey, fields: fields, rows: rows,
		secondary: map[string]map[string]int{}, int64Index: map[catalogInt64IndexKey]map[string]int64{},
		float64Index: map[catalogInt64IndexKey]map[string]float64{},
	}, nil
}

func (catalog *Catalog) Summary() CatalogSummary {
	return CatalogSummary{
		Name:       catalog.name,
		Kind:       "array",
		Count:      len(catalog.rows),
		PrimaryKey: catalog.primaryKey,
		Fields:     append([]string(nil), catalog.fields...),
	}
}

func (catalog *Catalog) Rows() []json.RawMessage {
	rows := make([]json.RawMessage, len(catalog.rows))
	for index, row := range catalog.rows {
		rows[index] = append(json.RawMessage(nil), row...)
	}
	return rows
}

func (catalog *Catalog) Find(id string) (json.RawMessage, bool) {
	index, ok := catalog.findIndex(id)
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), catalog.rows[index]...), true
}

// Contains reports whether the official primary-key row exists without
// cloning or decoding its raw JSON after the shared primary index is built.
func (catalog *Catalog) Contains(id string) bool {
	if catalog == nil {
		return false
	}
	_, found := catalog.findIndex(id)
	return found
}

func (catalog *Catalog) findIndex(id string) (int, bool) {
	id = strings.TrimSpace(id)
	if id == "" || catalog.primaryKey == "" {
		return 0, false
	}
	catalog.indexOnce.Do(func() {
		catalog.index = make(map[string]int, len(catalog.rows))
		for index, row := range catalog.rows {
			record, err := DecodeRecord(row)
			if err != nil {
				continue
			}
			if key, ok := scalarKey(record[catalog.primaryKey]); ok {
				catalog.index[key] = index
			}
		}
	})
	index, ok := catalog.index[id]
	return index, ok
}

func (catalog *Catalog) FindByField(field string, value string) (json.RawMessage, bool) {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	if field == "" || value == "" {
		return nil, false
	}
	if field == catalog.primaryKey {
		return catalog.Find(value)
	}
	catalog.secondaryMu.Lock()
	index, loaded := catalog.secondary[field]
	if !loaded {
		index = make(map[string]int, len(catalog.rows))
		for rowIndex, row := range catalog.rows {
			record, err := DecodeRecord(row)
			if err != nil {
				continue
			}
			if key, ok := scalarKey(record[field]); ok {
				index[key] = rowIndex
			}
		}
		catalog.secondary[field] = index
	}
	rowIndex, ok := index[value]
	catalog.secondaryMu.Unlock()
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), catalog.rows[rowIndex]...), true
}

// Int64 returns one immutable scalar from the row identified by the catalog's
// official primary key. The compact field index is built once and then shared
// by every account using this game-data generation; callers do not repeatedly
// decode the same official JSON record on reducer and automation hot paths.
func (catalog *Catalog) Int64(id string, field string) (int64, bool) {
	if catalog == nil || catalog.primaryKey == "" {
		return 0, false
	}
	return catalog.Int64ByField(catalog.primaryKey, id, field)
}

// Int64ByField returns one immutable scalar from a row selected by another
// scalar field. Only the requested lookup/value pair is retained, avoiding a
// decoded copy of every official record while still making repeated lookups
// allocation-free after the first use.
func (catalog *Catalog) Int64ByField(lookupField string, lookupValue string, valueField string) (int64, bool) {
	if catalog == nil {
		return 0, false
	}
	lookupField = strings.TrimSpace(lookupField)
	lookupValue = strings.TrimSpace(lookupValue)
	valueField = strings.TrimSpace(valueField)
	if lookupField == "" || lookupValue == "" || valueField == "" {
		return 0, false
	}
	key := catalogInt64IndexKey{lookupField: lookupField, valueField: valueField}
	catalog.int64Mu.Lock()
	index, loaded := catalog.int64Index[key]
	if !loaded {
		index = make(map[string]int64, len(catalog.rows))
		for _, raw := range catalog.rows {
			record, err := DecodeRecord(raw)
			if err != nil {
				continue
			}
			rowKey, ok := scalarKey(record[lookupField])
			if !ok {
				continue
			}
			value, ok := record.Int64(valueField)
			if ok {
				index[rowKey] = value
			}
		}
		catalog.int64Index[key] = index
	}
	value, found := index[lookupValue]
	catalog.int64Mu.Unlock()
	return value, found
}

// Float64 returns one immutable numeric scalar from the row identified by the
// catalog's official primary key. Like Int64, the compact field index is built
// once per game-data generation and shared by every account in the process.
// This keeps recurring automation evaluations from decoding the same official
// JSON records on every state wake.
func (catalog *Catalog) Float64(id string, field string) (float64, bool) {
	if catalog == nil || catalog.primaryKey == "" {
		return 0, false
	}
	return catalog.Float64ByField(catalog.primaryKey, id, field)
}

// Float64ByField returns one immutable numeric scalar from a row selected by
// another scalar field. Missing and invalid values are intentionally absent
// from the index so callers retain the same fail-closed behavior as decoding
// the source record directly.
func (catalog *Catalog) Float64ByField(lookupField string, lookupValue string, valueField string) (float64, bool) {
	if catalog == nil {
		return 0, false
	}
	lookupField = strings.TrimSpace(lookupField)
	lookupValue = strings.TrimSpace(lookupValue)
	valueField = strings.TrimSpace(valueField)
	if lookupField == "" || lookupValue == "" || valueField == "" {
		return 0, false
	}
	key := catalogInt64IndexKey{lookupField: lookupField, valueField: valueField}
	catalog.float64Mu.Lock()
	index, loaded := catalog.float64Index[key]
	if !loaded {
		index = make(map[string]float64, len(catalog.rows))
		for _, raw := range catalog.rows {
			record, err := DecodeRecord(raw)
			if err != nil {
				continue
			}
			rowKey, ok := scalarKey(record[lookupField])
			if !ok {
				continue
			}
			value, ok := record.Float64(valueField)
			if ok {
				index[rowKey] = value
			}
		}
		catalog.float64Index[key] = index
	}
	value, found := index[lookupValue]
	catalog.float64Mu.Unlock()
	return value, found
}

type candidateStats struct {
	present int
	values  map[string]struct{}
}

var preferredPrimaryKeys = []string{
	"wodID",
	"constructionItemID",
	"packageID",
	"resourceID",
	"currencyID",
	"effectID",
	"equipmentID",
	"gemID",
	"rewardID",
	"craftingRecipeId",
	"id",
	"ID",
}

var collectionPrimaryKeys = map[string]string{
	"achievements":                 "achievementID",
	"buildings":                    "wodID",
	"units":                        "wodID",
	"horses":                       "wodID",
	"constructionItems":            "constructionItemID",
	"packages":                     "packageID",
	"resources":                    "resourceID",
	"currencies":                   "currencyID",
	"effects":                      "effectID",
	"effecttypes":                  "effectTypeID",
	"effectCaps":                   "capID",
	"equipment_effects":            "equipmentEffectID",
	"generalSkills":                "skillID",
	"legendskills":                 "skillID",
	"sceatSkills":                  "skillID",
	"relicEffects":                 "id",
	"relicTypes":                   "id",
	"equipments":                   "equipmentID",
	"gems":                         "gemID",
	"rewards":                      "rewardID",
	"craftingRecipes":              "craftingRecipeId",
	"eventAutoScalingCamps":        "eventAutoScalingCampID",
	"eventAutoScalingDifficulties": "difficultyID",
	"kingdoms":                     "kID",
}

func discoverSchema(rows []json.RawMessage) ([]string, string) {
	const sampleLimit = 512
	limit := len(rows)
	if limit > sampleLimit {
		limit = sampleLimit
	}
	fieldSet := map[string]struct{}{}
	stats := map[string]*candidateStats{}
	for _, raw := range rows[:limit] {
		record, err := DecodeRecord(raw)
		if err != nil {
			continue
		}
		for field, value := range record {
			fieldSet[field] = struct{}{}
			key, ok := scalarKey(value)
			if !ok {
				continue
			}
			stat := stats[field]
			if stat == nil {
				stat = &candidateStats{values: map[string]struct{}{}}
				stats[field] = stat
			}
			stat.present++
			stat.values[key] = struct{}{}
		}
	}

	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	preference := map[string]int{}
	for index, field := range preferredPrimaryKeys {
		preference[field] = len(preferredPrimaryKeys) - index
	}
	bestField := ""
	bestScore := -1
	for field, stat := range stats {
		if stat.present == 0 || len(stat.values) != stat.present {
			continue
		}
		if limit > 0 && stat.present*4 < limit*3 {
			continue
		}
		idLike := strings.EqualFold(field, "id") || strings.HasSuffix(strings.ToLower(field), "id")
		if !idLike {
			continue
		}
		score := stat.present*100 + preference[field]*10
		if score > bestScore || score == bestScore && field < bestField {
			bestField = field
			bestScore = score
		}
	}
	return fields, bestField
}

func scalarKey(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false
	}
	switch typed := value.(type) {
	case json.Number:
		return typed.String(), true
	case string:
		return typed, typed != ""
	default:
		return "", false
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
