package GameData

import (
	"fmt"
	"sort"
)

// ConstructionItemTier is the compact official-data projection needed by
// recurring construction automation. It is built once per official-data
// generation and shared by every account in the process.
type ConstructionItemTier struct {
	ID             int64
	GroupID        int64
	VariantKey     string
	Level          int
	Slot           int
	Temporary      bool
	StackSize      int64
	LockRemoval    string
	InternalName   string
	Comment        string
	DisplayNameKey string
}

type ConstructionItemCatalog struct {
	byID      map[int64]ConstructionItemTier
	byVariant map[string][]ConstructionItemTier
}

func (store *Store) ConstructionItemCatalog() (*ConstructionItemCatalog, error) {
	if store == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	store.constructionItemsOnce.Do(func() {
		store.constructionItems, store.constructionItemsErr = buildConstructionItemCatalog(store)
	})
	return store.constructionItems, store.constructionItemsErr
}

// DefinitionView returns immutable catalog-owned metadata. Callers must not
// retain it as mutable state.
func (catalog *ConstructionItemCatalog) DefinitionView(id int64) (ConstructionItemTier, bool) {
	if catalog == nil || id <= 0 {
		return ConstructionItemTier{}, false
	}
	definition, found := catalog.byID[id]
	return definition, found
}

// TiersView returns a catalog-owned immutable slice ordered from highest to
// lowest level. Callers may iterate it but must not mutate it.
func (catalog *ConstructionItemCatalog) TiersView(variantKey string) []ConstructionItemTier {
	if catalog == nil {
		return nil
	}
	return catalog.byVariant[variantKey]
}

func buildConstructionItemCatalog(store *Store) (*ConstructionItemCatalog, error) {
	rows, err := store.Catalog("constructionItems")
	if err != nil {
		return nil, err
	}
	result := &ConstructionItemCatalog{
		byID:      make(map[int64]ConstructionItemTier, len(rows.rows)),
		byVariant: map[string][]ConstructionItemTier{},
	}
	for _, raw := range rows.rows {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, _ := record.Int64("constructionItemID")
		groupID, _ := record.Int64("constructionItemGroupID")
		level, _ := record.Int64("level")
		slot, _ := record.Int64("slotTypeID")
		if id <= 0 || groupID <= 0 {
			continue
		}
		definition := ConstructionItemTier{
			ID: id, GroupID: groupID, VariantKey: ConstructionItemVariantKey(record),
			Level: int(level), Slot: int(slot), Temporary: ConstructionItemIsTemporary(record),
			StackSize: intValue(record, "stackSize"), LockRemoval: stringValue(record, "lockRemoval"),
			InternalName: stringValue(record, "name"), Comment: stringValue(record, "comment2"),
			DisplayNameKey: stringValue(record, "displayNameKey"),
		}
		result.byID[id] = definition
		result.byVariant[definition.VariantKey] = append(result.byVariant[definition.VariantKey], definition)
	}
	for variantKey := range result.byVariant {
		sort.Slice(result.byVariant[variantKey], func(left, right int) bool {
			if result.byVariant[variantKey][left].Level != result.byVariant[variantKey][right].Level {
				return result.byVariant[variantKey][left].Level > result.byVariant[variantKey][right].Level
			}
			return result.byVariant[variantKey][left].ID < result.byVariant[variantKey][right].ID
		})
	}
	return result, nil
}
