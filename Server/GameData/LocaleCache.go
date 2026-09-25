package GameData

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const localeCacheLimit = 4
const localeCacheTTL = 30 * time.Minute
const localeRetryTTL = 30 * time.Second

type LocaleResolution struct {
	FallbackKeys     []string `json:"fallbackKeys,omitempty"`
	FallbackKeyCount int      `json:"fallbackKeyCount"`
	RequestedLocale  string   `json:"requestedLocale"`
	ResolvedLocale   string   `json:"resolvedLocale"`
	Fallback         bool     `json:"fallback"`
	Source           string   `json:"source"`                   // live, cache, or default
	FallbackLocale   string   `json:"fallbackLocale,omitempty"` // missing keys
}

type localeCacheEntry struct {
	runtimeLanguage *LanguageStore
	err             error
	language        *LanguageStore
	resolution      LocaleResolution
	expires         time.Time
	used            time.Time
}
type localeLoad struct {
	done  chan struct{}
	entry *localeCacheEntry
	err   error
}

// LanguageFor resolves viewer language without mutating the runtime language.
// At most two downloads run concurrently, one per locale, with four retained
// dictionaries. Each caller may cancel independently of other waiters.
func (manager *Manager) LanguageFor(ctx context.Context, code string) (*LanguageStore, LocaleResolution, error) {
	locale, err := NormalizeLocale(code)
	if err != nil {
		return nil, LocaleResolution{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, LocaleResolution{}, err
	}
	runtimeLanguage, _ := manager.Language()
	manager.localeMu.Lock()
	if manager.localeCache == nil {
		manager.localeCache = map[string]*localeCacheEntry{}
		manager.localeLoads = map[string]*localeLoad{}
		manager.localeSlots = make(chan struct{}, 2)
	}
	if entry := manager.localeCache[locale.Code]; entry != nil && entry.runtimeLanguage == runtimeLanguage && time.Now().Before(entry.expires) {
		entry.used = time.Now()
		manager.localeMu.Unlock()
		return entry.language, entry.resolution, entry.err
	}
	pending := manager.localeLoads[locale.Code]
	if pending == nil {
		pending = &localeLoad{done: make(chan struct{})}
		manager.localeLoads[locale.Code] = pending
		go manager.loadLocale(locale, pending)
	}
	manager.localeMu.Unlock()
	select {
	case <-ctx.Done():
		return nil, LocaleResolution{}, ctx.Err()
	case <-pending.done:
		if pending.err != nil {
			return nil, LocaleResolution{}, pending.err
		}
		return pending.entry.language, pending.entry.resolution, nil
	}
}

func (manager *Manager) loadLocale(locale Locale, pending *localeLoad) {
	runtimeLanguage, _ := manager.Language()
	ctx, cancel := context.WithTimeout(context.Background(), manager.config.RequestTimeout)
	defer cancel()
	var entry *localeCacheEntry
	var err error
	select {
	case manager.localeSlots <- struct{}{}:
		entry, err = manager.resolveLocale(ctx, locale)
		<-manager.localeSlots
	case <-ctx.Done():
		err = ctx.Err()
	}
	manager.localeMu.Lock()
	defer manager.localeMu.Unlock()
	if err != nil {
		entry = newLocaleEntry(nil, LocaleResolution{}, localeRetryTTL)
		entry.err = err
	}
	{
		if len(manager.localeCache) >= localeCacheLimit {
			oldest := ""
			for key, cached := range manager.localeCache {
				if oldest == "" || cached.used.Before(manager.localeCache[oldest].used) {
					oldest = key
				}
			}
			delete(manager.localeCache, oldest)
		}
		manager.localeCache[locale.Code] = entry
	}
	entry.runtimeLanguage = runtimeLanguage
	pending.entry, pending.err = entry, err
	delete(manager.localeLoads, locale.Code)
	close(pending.done)
}

func (manager *Manager) resolveLocale(ctx context.Context, locale Locale) (*localeCacheEntry, error) {
	resolution := LocaleResolution{RequestedLocale: locale.Code, ResolvedLocale: locale.Code, Source: "live"}
	// The default runtime dictionary is already immutable and refreshed centrally.
	var english *LanguageStore
	if language, ready := manager.Language(); ready && language.Metadata().Language == "en" {
		english = language
	}
	if locale.Code == "en" && english != nil {
		resolution.Source = "default"
		return newLocaleEntry(english, resolution, localeCacheTTL), nil
	}
	language, source, err := manager.fetchLocale(ctx, locale)
	if err == nil {
		resolution.Source = source
	}
	ttl := localeCacheTTL
	if err != nil {
		language, err = manager.cachedLocale(locale)
		resolution.Source = "cache"
		ttl = localeRetryTTL
	}
	if english == nil {
		en, _ := NormalizeLocale("en")
		english, _ = manager.cachedLocale(en)
	}
	if english == nil && locale.Code != "en" {
		en, _ := NormalizeLocale("en")
		english, _, _ = manager.fetchLocale(ctx, en)
	}
	if err != nil {
		if english == nil {
			return nil, fmt.Errorf("official language %s and English fallback unavailable", locale.Code)
		}
		language = english
		resolution.ResolvedLocale, resolution.Fallback, resolution.Source = "en", true, "default"
	} else if locale.Code != "en" && english != nil {
		// A shallow immutable view adds per-key fallback without changing shared data.
		language = &LanguageStore{metadata: language.metadata, values: language.values, folded: language.folded, fallback: english}
		resolution.FallbackLocale = "en"
		resolution.FallbackKeys = language.FallbackKeys()
		resolution.FallbackKeyCount = len(resolution.FallbackKeys)
	}
	return newLocaleEntry(language, resolution, ttl), nil
}

func newLocaleEntry(language *LanguageStore, resolution LocaleResolution, ttl time.Duration) *localeCacheEntry {
	now := time.Now()
	return &localeCacheEntry{language: language, resolution: resolution, used: now, expires: now.Add(ttl)}
}

func safeLanguageVersion(version string) bool {
	if version == "" || len(version) > 32 {
		return false
	}
	for _, r := range version {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (manager *Manager) fetchLocale(ctx context.Context, locale Locale) (*LanguageStore, string, error) {
	raw, err := manager.fetch(ctx, manager.config.LanguageMetadataURL, 1<<20)
	if err != nil {
		return nil, "", err
	}
	var document struct {
		Metadata struct {
			Version string `json:"versionNo"`
			Branch  string `json:"branch"`
		} `json:"@metadata"`
	}
	if err = json.Unmarshal(raw, &document); err != nil {
		return nil, "", err
	}
	version := document.Metadata.Version
	if !safeLanguageVersion(version) {
		return nil, "", fmt.Errorf("unsafe official language version")
	}
	url := strings.NewReplacer("{version}", version, "{language}", locale.GameCode).Replace(manager.config.LanguageURL)
	path := filepath.Join(manager.config.CacheDir, "Language-"+locale.GameCode+"-v"+version+".json")
	if language, err := manager.readLocale(path, locale, version, url); err == nil {
		return language, "cache", nil
	}
	raw, err = manager.fetch(ctx, url, manager.config.LanguageMaxBytes)
	if err != nil {
		return nil, "", err
	}
	language, err := decodeLocale(raw, locale, version, url, time.Now().UTC())
	if err != nil {
		return nil, "", err
	}
	// Persist only validated documents. A read-only cache must not prevent use.
	if manager.config.CacheDir != "" && os.MkdirAll(manager.config.CacheDir, 0o755) == nil {
		if file, err := os.CreateTemp(manager.config.CacheDir, ".Language-*.json"); err == nil {
			temp := file.Name()
			_, writeErr := file.Write(raw)
			closeErr := file.Close()
			if writeErr == nil && closeErr == nil {
				_ = os.Rename(temp, path)
			}
			_ = os.Remove(temp)
		}
	}
	return language, "live", nil
}

func decodeLocale(raw []byte, locale Locale, version, url string, fetched time.Time) (*LanguageStore, error) {
	var document struct {
		Metadata struct {
			Version string `json:"versionNo"`
			Branch  string `json:"branch"`
		} `json:"@metadata"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if document.Metadata.Version != version {
		return nil, fmt.Errorf("official language version mismatch")
	}
	return DecodeLanguage(raw, LanguageMetadata{Language: locale.Code, Version: version, Branch: document.Metadata.Branch, SourceURL: url, FetchedAt: fetched})
}

func (manager *Manager) readLocale(path string, locale Locale, version, url string) (*LanguageStore, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > manager.config.LanguageMaxBytes {
		return nil, fmt.Errorf("cached language too large")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeLocale(raw, locale, version, url, info.ModTime().UTC())
}

func (manager *Manager) cachedLocale(locale Locale) (*LanguageStore, error) {
	if manager.config.CacheDir == "" {
		return nil, fmt.Errorf("language cache unavailable")
	}
	prefix := "Language-" + locale.GameCode + "-v"
	matches, err := filepath.Glob(filepath.Join(manager.config.CacheDir, prefix+"*.json"))
	if err != nil {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool {
		a := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(matches[i]), prefix), ".json")
		b := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(matches[j]), prefix), ".json")
		if len(a) != len(b) {
			return len(a) > len(b)
		}
		return a > b
	})
	for _, path := range matches {
		version := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), prefix), ".json")
		if !safeLanguageVersion(version) {
			continue
		}
		url := strings.NewReplacer("{version}", version, "{language}", locale.GameCode).Replace(manager.config.LanguageURL)
		if language, err := manager.readLocale(path, locale, version, url); err == nil {
			return language, nil
		}
	}
	return nil, fmt.Errorf("no validated cached language %s", locale.Code)
}

// CraftingCatalogWithLanguage creates an isolated projection over immutable
// item data; it never copies mutexes or changes the shared runtime manager.
func (manager *Manager) CraftingCatalogWithLanguage(language *LanguageStore) (CraftingCatalog, error) {
	store, _ := manager.Current()
	view := &Manager{store: store, language: language}
	return view.CraftingCatalog()
}
