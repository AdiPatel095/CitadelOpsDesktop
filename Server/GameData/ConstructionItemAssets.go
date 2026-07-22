package GameData

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	constructionItemIconPattern = regexp.MustCompile(`this\.assets\.(ConstructionItem_[A-Za-z0-9_]+)="itemassets/(ConstructionItems/[A-Za-z0-9_/-]+)"`)
	buildingIconPattern         = regexp.MustCompile(`this\.assets\.([A-Za-z0-9_]+)="itemassets/(Building/[A-Za-z0-9_/-]+)"`)
)

type ConstructionItemAssetManifest struct {
	Version       string            `json:"version"`
	SourceURL     string            `json:"sourceUrl"`
	FetchedAt     time.Time         `json:"fetchedAt"`
	Icons         map[string]string `json:"icons"`
	BuildingIcons map[string]string `json:"buildingIcons"`
}

func (manager *Manager) ConstructionItemAssets(ctx context.Context) (ConstructionItemAssetManifest, error) {
	manager.assetMu.Lock()
	defer manager.assetMu.Unlock()
	if manager.constructionItemAssets != nil {
		return cloneConstructionItemAssetManifest(*manager.constructionItemAssets), nil
	}

	manifest, err := manager.refreshConstructionItemAssets(ctx)
	if err != nil {
		manifest, err = manager.loadNewestConstructionItemAssetCache()
		if err != nil {
			return ConstructionItemAssetManifest{}, fmt.Errorf("resolve official construction item icons: %w", err)
		}
	}
	manager.constructionItemAssets = &manifest
	return cloneConstructionItemAssetManifest(manifest), nil
}

func (manager *Manager) refreshConstructionItemAssets(ctx context.Context) (ConstructionItemAssetManifest, error) {
	if strings.TrimSpace(manager.config.CacheDir) == "" {
		return ConstructionItemAssetManifest{}, fmt.Errorf("official-data cache directory is required")
	}
	if err := os.MkdirAll(manager.config.CacheDir, 0o755); err != nil {
		return ConstructionItemAssetManifest{}, fmt.Errorf("create official-data cache: %w", err)
	}

	indexRaw, err := manager.fetch(ctx, manager.config.AssetIndexURL, 2<<20)
	if err != nil {
		return ConstructionItemAssetManifest{}, fmt.Errorf("fetch official asset index: %w", err)
	}
	dllMatch := currencyDLLPattern.FindSubmatch(indexRaw)
	if len(dllMatch) != 2 {
		return ConstructionItemAssetManifest{}, fmt.Errorf("official asset index does not reference the game DLL")
	}
	version := string(dllMatch[1])
	dllURL, err := resolveAssetURL(manager.config.AssetIndexURL, string(dllMatch[0]))
	if err != nil {
		return ConstructionItemAssetManifest{}, err
	}
	cachePath := filepath.Join(manager.config.CacheDir, "ConstructionItemAssets-"+version+".json")
	if cached, cacheErr := loadConstructionItemAssetFile(cachePath); cacheErr == nil && len(cached.BuildingIcons) > 0 {
		return cached, nil
	}

	dllRaw, err := manager.fetch(ctx, dllURL, manager.config.AssetDLLMaxBytes)
	if err != nil {
		return ConstructionItemAssetManifest{}, fmt.Errorf("fetch official game DLL: %w", err)
	}
	icons := parseConstructionItemIconURLs(dllRaw, manager.config.AssetRootURL)
	if len(icons) == 0 {
		return ConstructionItemAssetManifest{}, fmt.Errorf("official game DLL contains no construction item icons")
	}
	buildingIcons := parseBuildingIconURLs(dllRaw, manager.config.AssetRootURL)
	if len(buildingIcons) == 0 {
		return ConstructionItemAssetManifest{}, fmt.Errorf("official game DLL contains no building icons")
	}
	manifest := ConstructionItemAssetManifest{
		Version: version, SourceURL: dllURL, FetchedAt: time.Now().UTC(), Icons: icons, BuildingIcons: buildingIcons,
	}
	if err := writeConstructionItemAssetFile(cachePath, manifest); err != nil {
		return ConstructionItemAssetManifest{}, fmt.Errorf("cache official construction item icons: %w", err)
	}
	return manifest, nil
}

func (manager *Manager) loadNewestConstructionItemAssetCache() (ConstructionItemAssetManifest, error) {
	matches, err := filepath.Glob(filepath.Join(manager.config.CacheDir, "ConstructionItemAssets-*.json"))
	if err != nil {
		return ConstructionItemAssetManifest{}, err
	}
	if len(matches) == 0 {
		return ConstructionItemAssetManifest{}, fmt.Errorf("no cached official construction item asset map")
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
		if manifest, loadErr := loadConstructionItemAssetFile(path); loadErr == nil {
			return manifest, nil
		}
	}
	return ConstructionItemAssetManifest{}, fmt.Errorf("cached official construction item asset maps are invalid")
}

func parseConstructionItemIconURLs(raw []byte, assetRoot string) map[string]string {
	root := strings.TrimRight(assetRoot, "/") + "/"
	icons := map[string]string{}
	for _, match := range constructionItemIconPattern.FindAllSubmatch(raw, -1) {
		if len(match) != 3 {
			continue
		}
		assetName := string(match[1])
		path := string(match[2])
		if assetName == "" || path == "" {
			continue
		}
		icons[assetName] = root + path + ".webp"
	}
	return icons
}

func parseBuildingIconURLs(raw []byte, assetRoot string) map[string]string {
	root := strings.TrimRight(assetRoot, "/") + "/"
	icons := map[string]string{}
	for _, match := range buildingIconPattern.FindAllSubmatch(raw, -1) {
		if len(match) != 3 {
			continue
		}
		assetName := string(match[1])
		path := string(match[2])
		if assetName == "" || path == "" {
			continue
		}
		icons[assetName] = root + path + ".webp"
	}
	return icons
}

func loadConstructionItemAssetFile(path string) (ConstructionItemAssetManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ConstructionItemAssetManifest{}, err
	}
	var manifest ConstructionItemAssetManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ConstructionItemAssetManifest{}, err
	}
	if strings.TrimSpace(manifest.Version) == "" || len(manifest.Icons) == 0 {
		return ConstructionItemAssetManifest{}, fmt.Errorf("construction item asset map is empty")
	}
	return manifest, nil
}

func writeConstructionItemAssetFile(path string, manifest ConstructionItemAssetManifest) (returnErr error) {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ConstructionItemAssets-*.json")
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

func cloneConstructionItemAssetManifest(source ConstructionItemAssetManifest) ConstructionItemAssetManifest {
	clone := source
	clone.Icons = make(map[string]string, len(source.Icons))
	for name, iconURL := range source.Icons {
		clone.Icons[name] = iconURL
	}
	clone.BuildingIcons = make(map[string]string, len(source.BuildingIcons))
	for name, iconURL := range source.BuildingIcons {
		clone.BuildingIcons[name] = iconURL
	}
	return clone
}
