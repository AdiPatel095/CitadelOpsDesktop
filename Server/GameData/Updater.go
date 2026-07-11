package GameData

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultVersionURL          = "https://empire-html5.goodgamestudios.com/default/items/ItemsVersion.properties"
	DefaultItemsURL            = "https://empire-html5.goodgamestudios.com/default/items/items_v{version}.json"
	DefaultLanguageMetadataURL = "https://langserv.public.ggs-ep.com/12/fr/@metadata"
	DefaultLanguageURL         = "https://langserv.public.ggs-ep.com/12@{version}/{language}/*"
	DefaultMaxBytes            = int64(256 << 20)
	DefaultLanguageMaxBytes    = int64(64 << 20)
)

type UpdaterConfig struct {
	CacheDir            string
	VersionURL          string
	ItemsURL            string
	LanguageMetadataURL string
	LanguageURL         string
	Language            string
	Client              *http.Client
	MaxBytes            int64
	LanguageMaxBytes    int64
	RequestTimeout      time.Duration
}

type Manager struct {
	config UpdaterConfig

	mu       sync.RWMutex
	store    *Store
	language *LanguageStore
	lastErr  error
}

func NewManager(config UpdaterConfig) *Manager {
	if config.VersionURL == "" {
		config.VersionURL = DefaultVersionURL
	}
	if config.ItemsURL == "" {
		config.ItemsURL = DefaultItemsURL
	}
	if config.LanguageMetadataURL == "" {
		config.LanguageMetadataURL = DefaultLanguageMetadataURL
	}
	if config.LanguageURL == "" {
		config.LanguageURL = DefaultLanguageURL
	}
	if config.Language == "" {
		config.Language = "en"
	}
	if config.MaxBytes <= 0 {
		config.MaxBytes = DefaultMaxBytes
	}
	if config.LanguageMaxBytes <= 0 {
		config.LanguageMaxBytes = DefaultLanguageMaxBytes
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 60 * time.Second
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: config.RequestTimeout}
	}
	return &Manager{config: config}
}

func (manager *Manager) Initialize(ctx context.Context) error {
	if err := manager.Refresh(ctx); err == nil {
		return nil
	} else {
		manager.mu.Lock()
		manager.lastErr = err
		manager.mu.Unlock()
	}
	if fallbackErr := manager.loadNewestCache(); fallbackErr != nil {
		return fmt.Errorf("refresh official data: %v; load cache: %w", manager.LastError(), fallbackErr)
	}
	return nil
}

func (manager *Manager) LoadCache() error {
	return manager.loadNewestCache()
}

func (manager *Manager) Refresh(ctx context.Context) error {
	if strings.TrimSpace(manager.config.CacheDir) == "" {
		return fmt.Errorf("official-data cache directory is required")
	}
	if err := os.MkdirAll(manager.config.CacheDir, 0o755); err != nil {
		return fmt.Errorf("create official-data cache: %w", err)
	}

	versionRaw, err := manager.fetch(ctx, manager.config.VersionURL, 1<<20)
	if err != nil {
		return fmt.Errorf("fetch official item version: %w", err)
	}
	version, err := parseItemVersion(string(versionRaw))
	if err != nil {
		return err
	}
	itemURL := strings.ReplaceAll(manager.config.ItemsURL, "{version}", version)
	cachePath := filepath.Join(manager.config.CacheDir, "Items-v"+version+".json")
	store, loadErr := loadStoreFile(cachePath, version, itemURL, time.Time{})
	if loadErr != nil {
		if err := manager.downloadAtomic(ctx, itemURL, cachePath, manager.config.MaxBytes); err != nil {
			return fmt.Errorf("download official item data %s: %w", version, err)
		}
		store, err = loadStoreFile(cachePath, version, itemURL, time.Now().UTC())
		if err != nil {
			return err
		}
	}
	language, warning, languageErr := manager.refreshLanguage(ctx)
	if languageErr != nil {
		return languageErr
	}
	store.metadata.Language = language.Metadata().Language
	store.metadata.LanguageVersion = language.Metadata().Version

	manager.mu.Lock()
	manager.store = store
	manager.language = language
	manager.lastErr = warning
	manager.mu.Unlock()
	return nil
}

func (manager *Manager) Current() (*Store, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.store, manager.store != nil
}

func (manager *Manager) Language() (*LanguageStore, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.language, manager.language != nil
}

func (manager *Manager) LastError() error {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.lastErr
}

func (manager *Manager) loadNewestCache() error {
	matches, err := filepath.Glob(filepath.Join(manager.config.CacheDir, "Items-v*.json"))
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no cached official item document")
	}
	sort.Slice(matches, func(left, right int) bool {
		leftInfo, leftErr := os.Stat(matches[left])
		rightInfo, rightErr := os.Stat(matches[right])
		if leftErr != nil || rightErr != nil {
			return matches[left] > matches[right]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
	var failures []string
	for _, path := range matches {
		name := filepath.Base(path)
		version := strings.TrimSuffix(strings.TrimPrefix(name, "Items-v"), ".json")
		url := strings.ReplaceAll(manager.config.ItemsURL, "{version}", version)
		store, loadErr := loadStoreFile(path, version, url, time.Time{})
		if loadErr != nil {
			failures = append(failures, loadErr.Error())
			continue
		}
		manager.mu.Lock()
		manager.store = store
		if language, languageErr := manager.loadNewestLanguageCache(); languageErr == nil {
			manager.language = language
			store.metadata.Language = language.Metadata().Language
			store.metadata.LanguageVersion = language.Metadata().Version
		}
		manager.mu.Unlock()
		return nil
	}
	return fmt.Errorf("cached official item documents are invalid: %s", strings.Join(failures, "; "))
}

func (manager *Manager) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := manager.config.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	limited := io.LimitReader(response.Body, limit+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return raw, nil
}

func (manager *Manager) downloadAtomic(ctx context.Context, url string, destination string, maxBytes int64) (returnErr error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	response, err := manager.config.Client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".Items-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()

	written, err := io.Copy(temporary, io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if written > maxBytes {
		return fmt.Errorf("official document exceeds %d bytes", maxBytes)
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return err
	}
	return nil
}

func (manager *Manager) refreshLanguage(ctx context.Context) (*LanguageStore, error, error) {
	metadataRaw, err := manager.fetch(ctx, manager.config.LanguageMetadataURL, 1<<20)
	if err != nil {
		cached, cacheErr := manager.loadNewestLanguageCache()
		if cacheErr != nil {
			return nil, nil, fmt.Errorf("fetch official language metadata: %w", err)
		}
		return cached, fmt.Errorf("using cached language data: %w", err), nil
	}
	var metadataDocument struct {
		Metadata struct {
			Version string `json:"versionNo"`
			Branch  string `json:"branch"`
		} `json:"@metadata"`
	}
	if err := json.Unmarshal(metadataRaw, &metadataDocument); err != nil {
		return nil, nil, fmt.Errorf("decode official language metadata: %w", err)
	}
	version := strings.TrimSpace(metadataDocument.Metadata.Version)
	if version == "" {
		return nil, nil, fmt.Errorf("official language metadata has no version")
	}
	languageURL := strings.NewReplacer(
		"{version}", version,
		"{language}", manager.config.Language,
	).Replace(manager.config.LanguageURL)
	cachePath := filepath.Join(manager.config.CacheDir, fmt.Sprintf("Language-%s-v%s.json", manager.config.Language, version))
	language, loadErr := loadLanguageFile(
		cachePath, manager.config.Language, version, metadataDocument.Metadata.Branch, languageURL, time.Time{},
	)
	if loadErr == nil {
		return language, nil, nil
	}
	if err := manager.downloadAtomic(ctx, languageURL, cachePath, manager.config.LanguageMaxBytes); err != nil {
		cached, cacheErr := manager.loadNewestLanguageCache()
		if cacheErr != nil {
			return nil, nil, fmt.Errorf("download official language data %s: %w", version, err)
		}
		return cached, fmt.Errorf("using cached language data: %w", err), nil
	}
	language, err = loadLanguageFile(
		cachePath, manager.config.Language, version, metadataDocument.Metadata.Branch, languageURL, time.Now().UTC(),
	)
	return language, nil, err
}

func (manager *Manager) loadNewestLanguageCache() (*LanguageStore, error) {
	pattern := filepath.Join(manager.config.CacheDir, fmt.Sprintf("Language-%s-v*.json", manager.config.Language))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no cached official language document")
	}
	sort.Slice(matches, func(left, right int) bool {
		leftInfo, leftErr := os.Stat(matches[left])
		rightInfo, rightErr := os.Stat(matches[right])
		if leftErr != nil || rightErr != nil {
			return matches[left] > matches[right]
		}
		return leftInfo.ModTime().After(rightInfo.ModTime())
	})
	for _, path := range matches {
		name := filepath.Base(path)
		prefix := "Language-" + manager.config.Language + "-v"
		version := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
		languageURL := strings.NewReplacer(
			"{version}", version,
			"{language}", manager.config.Language,
		).Replace(manager.config.LanguageURL)
		language, loadErr := loadLanguageFile(path, manager.config.Language, version, "", languageURL, time.Time{})
		if loadErr == nil {
			return language, nil
		}
	}
	return nil, fmt.Errorf("cached official language documents are invalid")
}

func loadLanguageFile(
	path string,
	language string,
	version string,
	branch string,
	sourceURL string,
	fetchedAt time.Time,
) (*LanguageStore, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if fetchedAt.IsZero() {
		if info, statErr := os.Stat(path); statErr == nil {
			fetchedAt = info.ModTime().UTC()
		}
	}
	return DecodeLanguage(raw, LanguageMetadata{
		Language: language, Version: version, Branch: branch, SourceURL: sourceURL, FetchedAt: fetchedAt,
	})
}

func loadStoreFile(path string, version string, sourceURL string, fetchedAt time.Time) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	if fetchedAt.IsZero() {
		if info, statErr := os.Stat(path); statErr == nil {
			fetchedAt = info.ModTime().UTC()
		}
	}
	metadata := SourceMetadata{
		ItemVersion:  version,
		SourceURL:    sourceURL,
		DigestSHA256: hex.EncodeToString(digest[:]),
		FetchedAt:    fetchedAt,
		LoadedAt:     time.Now().UTC(),
	}
	return DecodeStore(raw, metadata)
}

func parseItemVersion(raw string) (string, error) {
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "CastleItemXMLVersion" {
			continue
		}
		version := strings.TrimSpace(value)
		if version == "" {
			break
		}
		for _, character := range version {
			if character < '0' || character > '9' {
				if character != '.' && character != '-' && character != '_' {
					return "", fmt.Errorf("official item version contains unsafe character %q", character)
				}
			}
		}
		return version, nil
	}
	return "", fmt.Errorf("CastleItemXMLVersion is missing")
}
