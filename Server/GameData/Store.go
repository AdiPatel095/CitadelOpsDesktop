package GameData

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	metadata    SourceMetadata
	collections map[string]json.RawMessage

	mu                       sync.RWMutex
	catalogs                 map[string]*Catalog
	marketOnce               sync.Once
	marketProjection         marketProjection
	marketErr                error
	constructionShopOnce     sync.Once
	constructionShopProducts map[int64][]ConstructionShopProduct
	constructionShopErr      error
	buildingCatalogOnce      sync.Once
	buildingCatalog          *BuildingCatalog
	buildingCatalogErr       error
	expansionCatalogOnce     sync.Once
	expansionCatalog         *ExpansionCatalog
	expansionCatalogErr      error
	defenseToolShopOnce      sync.Once
	defenseToolShopProducts  map[int64][]DefenseToolShopPackage
	defenseToolShopErr       error
	berimondArmorerOnce      sync.Once
	berimondArmorerProducts  map[int64]BerimondArmorerToolPackage
	berimondArmorerErr       error
}

func DecodeStore(raw []byte, metadata SourceMetadata) (*Store, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode official item document: %w", err)
	}
	for _, required := range []string{"versionInfo", "buildings", "units"} {
		if len(root[required]) == 0 {
			return nil, fmt.Errorf("official item document is missing %s", required)
		}
	}

	collections := make(map[string]json.RawMessage, len(root))
	for name, value := range root {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || trimmed[0] != '[' {
			continue
		}
		collections[name] = append(json.RawMessage(nil), value...)
	}
	if len(collections) == 0 {
		return nil, fmt.Errorf("official item document contains no collections")
	}
	return &Store{metadata: metadata, collections: collections, catalogs: map[string]*Catalog{}}, nil
}

func (store *Store) Metadata() SourceMetadata {
	return store.metadata
}

func (store *Store) CatalogNames() []string {
	names := make([]string, 0, len(store.collections))
	for name := range store.collections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (store *Store) Summaries() []CatalogSummary {
	names := store.CatalogNames()
	summaries := make([]CatalogSummary, 0, len(names))
	for _, name := range names {
		store.mu.RLock()
		loaded := store.catalogs[name]
		store.mu.RUnlock()
		if loaded != nil {
			summaries = append(summaries, loaded.Summary())
			continue
		}
		summaries = append(summaries, CatalogSummary{
			Name:  name,
			Kind:  "array",
			Count: countJSONArray(store.collections[name]),
		})
	}
	return summaries
}

func (store *Store) RawCollection(name string) (json.RawMessage, bool) {
	raw, ok := store.collections[name]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), raw...), true
}

func (store *Store) Catalog(name string) (*Catalog, error) {
	store.mu.RLock()
	loaded := store.catalogs[name]
	store.mu.RUnlock()
	if loaded != nil {
		return loaded, nil
	}

	raw, ok := store.collections[name]
	if !ok {
		return nil, fmt.Errorf("unknown official-data collection %q", name)
	}
	catalog, err := newCatalog(name, raw)
	if err != nil {
		return nil, err
	}
	store.mu.Lock()
	if existing := store.catalogs[name]; existing != nil {
		catalog = existing
	} else {
		store.catalogs[name] = catalog
	}
	store.mu.Unlock()
	return catalog, nil
}

func countJSONArray(raw []byte) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' {
		return 0
	}
	depth := 0
	inString := false
	escaped := false
	hasValue := false
	count := 0
	for index, value := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == '"' {
				inString = false
			}
			continue
		}
		switch value {
		case '"':
			inString = true
			if depth == 1 {
				hasValue = true
			}
		case '[', '{':
			depth++
			if depth == 2 {
				hasValue = true
			}
		case ']', '}':
			depth--
		case ',':
			if depth == 1 {
				count++
			}
		default:
			if depth == 1 && index > 0 && !strings.ContainsRune(" \t\r\n", rune(value)) {
				hasValue = true
			}
		}
	}
	if !hasValue {
		return 0
	}
	return count + 1
}
