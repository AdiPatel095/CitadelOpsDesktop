package GameData

import (
	"strings"
)

var sovereignResourceJSONKeys = [...]string{"C", "O", "G", "I"}

func (store *Store) ensureResourceLookups() {
	store.resourceLookupOnce.Do(store.loadResourceLookups)
}

func (store *Store) loadResourceLookups() {
	store.resourceIDsByJSONKey = map[string]int64{}
	store.resourceJSONKeysByID = map[int64]string{}
	store.currencyIDsByJSONKey = map[string]int64{}
	if resources, err := store.Catalog("resources"); err == nil {
		for _, raw := range resources.Rows() {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, hasID := record.Int64("resourceID")
			jsonKey, hasKey := record.String("JSONKey")
			jsonKey = strings.ToUpper(strings.TrimSpace(jsonKey))
			if hasID && id > 0 && hasKey && jsonKey != "" {
				store.resourceIDsByJSONKey[jsonKey] = id
				store.resourceJSONKeysByID[id] = jsonKey
			}
		}
	}
	if currencies, err := store.Catalog("currencies"); err == nil {
		for _, raw := range currencies.Rows() {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, hasID := record.Int64("currencyID")
			jsonKey, hasKey := record.String("JSONKey")
			jsonKey = strings.ToUpper(strings.TrimSpace(jsonKey))
			if hasID && id > 0 && hasKey && jsonKey != "" {
				store.currencyIDsByJSONKey[jsonKey] = id
			}
		}
	}
	for _, jsonKey := range sovereignResourceJSONKeys {
		if id := store.resourceIDsByJSONKey[jsonKey]; id > 0 {
			store.sovereignResourceIDs = append(store.sovereignResourceIDs, id)
		}
	}
}

func (store *Store) ResourceIDForJSONKey(jsonKey string) (int64, bool) {
	if store == nil {
		return 0, false
	}
	store.ensureResourceLookups()
	id, found := store.resourceIDsByJSONKey[strings.ToUpper(strings.TrimSpace(jsonKey))]
	return id, found
}

func (store *Store) CurrencyIDForJSONKey(jsonKey string) (int64, bool) {
	if store == nil {
		return 0, false
	}
	store.ensureResourceLookups()
	id, found := store.currencyIDsByJSONKey[strings.ToUpper(strings.TrimSpace(jsonKey))]
	return id, found
}

func (store *Store) ResourceJSONKey(resourceID int64) (string, bool) {
	if store == nil || resourceID <= 0 {
		return "", false
	}
	store.ensureResourceLookups()
	jsonKey, found := store.resourceJSONKeysByID[resourceID]
	return jsonKey, found
}

// SovereignResourceIDsView returns the immutable Store-owned IDs for glass,
// oil, coal, and iron in canonical order.
func (store *Store) SovereignResourceIDsView() []int64 {
	if store == nil {
		return nil
	}
	store.ensureResourceLookups()
	return store.sovereignResourceIDs
}
