package GameData

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type LanguageMetadata struct {
	Language  string    `json:"language"`
	Version   string    `json:"version"`
	Branch    string    `json:"branch,omitempty"`
	SourceURL string    `json:"sourceUrl"`
	FetchedAt time.Time `json:"fetchedAt"`
	LoadedAt  time.Time `json:"loadedAt"`
}

type LanguageStore struct {
	fallback *LanguageStore
	metadata LanguageMetadata
	values   map[string]string
	folded   map[string]string
}

func DecodeLanguage(raw []byte, metadata LanguageMetadata) (*LanguageStore, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode official language document: %w", err)
	}
	values := make(map[string]string, len(document))
	folded := make(map[string]string, len(document))
	ambiguousFolds := make(map[string]struct{})
	for key, value := range document {
		if key == "@metadata" {
			continue
		}
		var text string
		if json.Unmarshal(value, &text) == nil {
			values[key] = text
			foldedKey := strings.ToLower(key)
			if _, ambiguous := ambiguousFolds[foldedKey]; ambiguous {
				continue
			}
			if _, collision := folded[foldedKey]; collision {
				delete(folded, foldedKey)
				ambiguousFolds[foldedKey] = struct{}{}
				continue
			}
			folded[foldedKey] = text
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("official language document contains no strings")
	}
	metadata.LoadedAt = time.Now().UTC()
	return &LanguageStore{metadata: metadata, values: values, folded: folded}, nil
}

func (store *LanguageStore) Metadata() LanguageMetadata {
	return store.metadata
}

func (store *LanguageStore) Text(key string) (string, bool) {
	if store == nil {
		return "", false
	}
	value, ok := store.values[key]
	if (!ok || strings.TrimSpace(value) == "") && store.fallback != nil {
		return store.fallback.Text(key)
	}
	return value, ok
}

func (store *LanguageStore) Resolve(keys ...string) (string, bool) {
	if store == nil {
		return "", false
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := store.values[key]; ok && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	if store.fallback != nil {
		return store.fallback.Resolve(keys...)
	}
	return "", false
}

func (store *LanguageStore) ResolveMany(keys []string) map[string]string {
	resolved := make(map[string]string, len(keys))
	if store == nil {
		return resolved
	}
	for _, key := range keys {
		if value, ok := store.values[key]; ok && strings.TrimSpace(value) != "" {
			resolved[key] = value
			continue
		}
		if value, ok := store.folded[strings.ToLower(key)]; ok && strings.TrimSpace(value) != "" {
			resolved[key] = value
			continue
		}
		if store.fallback != nil {
			for k, v := range store.fallback.ResolveMany([]string{key}) {
				resolved[k] = v
			}
		}
	}
	return resolved
}

// Values returns an independent complete dictionary. Placeholders and markup
// remain plain strings; clients must never interpret these values as HTML.
func (store *LanguageStore) Values() map[string]string {
	values := map[string]string{}
	if store.fallback != nil {
		values = store.fallback.Values()
	}
	for key, value := range store.values {
		if strings.TrimSpace(value) == "" && strings.TrimSpace(values[key]) != "" {
			continue
		}
		values[key] = value
	}
	return values
}

// FallbackKeys identifies values in the complete dictionary supplied by English.
func (store *LanguageStore) FallbackKeys() []string {
	if store == nil || store.fallback == nil {
		return nil
	}
	keys := []string{}
	for key := range store.fallback.values {
		if value, exists := store.values[key]; (!exists || strings.TrimSpace(value) == "") && strings.TrimSpace(store.fallback.values[key]) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// FallbackKeysFor scopes missing-key provenance to a partial lookup response.
func (store *LanguageStore) FallbackKeysFor(keys []string) []string {
	if store == nil || store.fallback == nil {
		return nil
	}
	result := []string{}
	seen := map[string]bool{}
	for _, key := range keys {
		if strings.TrimSpace(store.values[key]) != "" || strings.TrimSpace(store.folded[strings.ToLower(key)]) != "" || seen[key] {
			continue
		}
		if _, exists := store.fallback.ResolveMany([]string{key})[key]; exists {
			result = append(result, key)
			seen[key] = true
		}
	}
	sort.Strings(result)
	return result
}
