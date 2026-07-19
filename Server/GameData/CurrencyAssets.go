package GameData

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	currencyDLLPattern  = regexp.MustCompile(`dll/ggs\.dll\.([A-Za-z0-9]+)\.js`)
	currencyIconPattern = regexp.MustCompile(`(Collectables/(?:[A-Za-z0-9_-]+/)?Collectable_Currency_([A-Za-z0-9_-]+)/Collectable_Currency_[A-Za-z0-9_-]+--[0-9]+)`)
)

type CurrencyAssetManifest struct {
	Version   string            `json:"version"`
	SourceURL string            `json:"sourceUrl"`
	FetchedAt time.Time         `json:"fetchedAt"`
	Icons     map[string]string `json:"icons"`
}

func (manager *Manager) CurrencyAssets(ctx context.Context) (CurrencyAssetManifest, error) {
	manager.assetMu.Lock()
	defer manager.assetMu.Unlock()
	if manager.currencyAssets != nil {
		return cloneCurrencyAssetManifest(*manager.currencyAssets), nil
	}

	manifest, err := manager.refreshCurrencyAssets(ctx)
	if err != nil {
		manifest, err = manager.loadNewestCurrencyAssetCache()
		if err != nil {
			return CurrencyAssetManifest{}, fmt.Errorf("resolve official currency icons: %w", err)
		}
	}
	manager.currencyAssets = &manifest
	return cloneCurrencyAssetManifest(manifest), nil
}

func (manager *Manager) refreshCurrencyAssets(ctx context.Context) (CurrencyAssetManifest, error) {
	if strings.TrimSpace(manager.config.CacheDir) == "" {
		return CurrencyAssetManifest{}, fmt.Errorf("official-data cache directory is required")
	}
	if err := os.MkdirAll(manager.config.CacheDir, 0o755); err != nil {
		return CurrencyAssetManifest{}, fmt.Errorf("create official-data cache: %w", err)
	}

	indexRaw, err := manager.fetch(ctx, manager.config.AssetIndexURL, 2<<20)
	if err != nil {
		return CurrencyAssetManifest{}, fmt.Errorf("fetch official asset index: %w", err)
	}
	dllMatch := currencyDLLPattern.FindSubmatch(indexRaw)
	if len(dllMatch) != 2 {
		return CurrencyAssetManifest{}, fmt.Errorf("official asset index does not reference the game DLL")
	}
	version := string(dllMatch[1])
	dllURL, err := resolveAssetURL(manager.config.AssetIndexURL, string(dllMatch[0]))
	if err != nil {
		return CurrencyAssetManifest{}, err
	}
	cachePath := filepath.Join(manager.config.CacheDir, "CurrencyAssets-"+version+".json")
	if cached, cacheErr := loadCurrencyAssetFile(cachePath); cacheErr == nil {
		return cached, nil
	}

	dllRaw, err := manager.fetch(ctx, dllURL, manager.config.AssetDLLMaxBytes)
	if err != nil {
		return CurrencyAssetManifest{}, fmt.Errorf("fetch official game DLL: %w", err)
	}
	icons := parseCurrencyIconURLs(dllRaw, manager.config.AssetRootURL)
	if len(icons) == 0 {
		return CurrencyAssetManifest{}, fmt.Errorf("official game DLL contains no currency icons")
	}
	manifest := CurrencyAssetManifest{
		Version: version, SourceURL: dllURL, FetchedAt: time.Now().UTC(), Icons: icons,
	}
	if err := writeCurrencyAssetFile(cachePath, manifest); err != nil {
		return CurrencyAssetManifest{}, fmt.Errorf("cache official currency icons: %w", err)
	}
	return manifest, nil
}

func (manager *Manager) loadNewestCurrencyAssetCache() (CurrencyAssetManifest, error) {
	matches, err := filepath.Glob(filepath.Join(manager.config.CacheDir, "CurrencyAssets-*.json"))
	if err != nil {
		return CurrencyAssetManifest{}, err
	}
	if len(matches) == 0 {
		return CurrencyAssetManifest{}, fmt.Errorf("no cached official currency asset map")
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
		if manifest, loadErr := loadCurrencyAssetFile(path); loadErr == nil {
			return manifest, nil
		}
	}
	return CurrencyAssetManifest{}, fmt.Errorf("cached official currency asset maps are invalid")
}

func parseCurrencyIconURLs(raw []byte, assetRoot string) map[string]string {
	root := strings.TrimRight(assetRoot, "/") + "/"
	icons := map[string]string{}
	for _, match := range currencyIconPattern.FindAllSubmatch(raw, -1) {
		if len(match) != 3 {
			continue
		}
		assetName := string(match[2])
		if assetName == "" {
			continue
		}
		icons[assetName] = root + string(match[1]) + ".webp"
	}
	return icons
}

func resolveAssetURL(indexURL string, relative string) (string, error) {
	base, err := url.Parse(indexURL)
	if err != nil {
		return "", fmt.Errorf("parse official asset index URL: %w", err)
	}
	reference, err := url.Parse(relative)
	if err != nil {
		return "", fmt.Errorf("parse official game DLL URL: %w", err)
	}
	return base.ResolveReference(reference).String(), nil
}

func loadCurrencyAssetFile(path string) (CurrencyAssetManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CurrencyAssetManifest{}, err
	}
	var manifest CurrencyAssetManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return CurrencyAssetManifest{}, err
	}
	if strings.TrimSpace(manifest.Version) == "" || len(manifest.Icons) == 0 {
		return CurrencyAssetManifest{}, fmt.Errorf("currency asset map is empty")
	}
	return manifest, nil
}

func writeCurrencyAssetFile(path string, manifest CurrencyAssetManifest) (returnErr error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".CurrencyAssets-*.json")
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
	if _, err := temporary.Write(raw); err != nil {
		return err
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
	return os.Rename(temporaryName, path)
}

func cloneCurrencyAssetManifest(source CurrencyAssetManifest) CurrencyAssetManifest {
	clone := source
	clone.Icons = make(map[string]string, len(source.Icons))
	for name, iconURL := range source.Icons {
		clone.Icons[name] = iconURL
	}
	return clone
}
