package GameData

import (
	"encoding/json"
	"fmt"
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
	metadata LanguageMetadata
	values   map[string]string
}

func DecodeLanguage(raw []byte, metadata LanguageMetadata) (*LanguageStore, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("decode official language document: %w", err)
	}
	values := make(map[string]string, len(document))
	for key, value := range document {
		if key == "@metadata" {
			continue
		}
		var text string
		if json.Unmarshal(value, &text) == nil {
			values[key] = text
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("official language document contains no strings")
	}
	metadata.LoadedAt = time.Now().UTC()
	return &LanguageStore{metadata: metadata, values: values}, nil
}

func (store *LanguageStore) Metadata() LanguageMetadata {
	return store.metadata
}

func (store *LanguageStore) Text(key string) (string, bool) {
	if store == nil {
		return "", false
	}
	value, ok := store.values[key]
	return value, ok
}

func (store *LanguageStore) Resolve(keys ...string) (string, bool) {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := store.Text(key); ok && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	return "", false
}

func (store *LanguageStore) ResolveMany(keys []string) map[string]string {
	resolved := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := store.Text(key); ok {
			resolved[key] = value
		}
	}
	return resolved
}
