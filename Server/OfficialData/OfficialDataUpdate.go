package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxOfficialAssetBytes       = 20 << 20
	defaultOfficialItemVersion  = "https://empire-html5.goodgamestudios.com/default/items/ItemsVersion.properties"
	defaultOfficialItemPattern  = "https://empire-html5.goodgamestudios.com/default/items/items_v{version}.json"
	defaultOfficialLangMetadata = "https://langserv.public.ggs-ep.com/12/fr/@metadata"
	defaultOfficialLangPattern  = "https://langserv.public.ggs-ep.com/12@{version}/{language}/*"
)

type OfficialCatalog struct {
	Name     string `json:"name"`
	ItemsURL string `json:"itemsUrl"`
	IndexURL string `json:"indexUrl"`
}

type OfficialDataManifest struct {
	Catalogs []OfficialCatalog `json:"catalogs"`
}

type OfficialItemCatalogSource struct {
	VersionURL string
	ItemsURL   string
	Template   string
	Version    string
}

type PublicOfficialDataFetcher struct {
	Client     *http.Client
	ServerData string
	DryRun     bool
}

type AppReadyConverter struct {
	Client          *http.Client
	ServerData      string
	ClientData      string
	FetchAsset      bool
	PrunePNG        bool
	DryRun          bool
	FailedAssetURLs map[string]struct{}
	AssetURLsByType map[string][]string
}

type stringListFlag []string

func (flagValue *stringListFlag) String() string {
	return strings.Join(*flagValue, ",")
}

func (flagValue *stringListFlag) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*flagValue = append(*flagValue, part)
		}
	}
	return nil
}

func main() {
	var catalogs stringListFlag
	baseURL := flag.String("base-url", strings.TrimSpace(os.Getenv("GGE_OFFICIAL_DATA_BASE_URL")), "official data base URL; catalog paths are derived as <base>/<catalog>/{items,index}.json")
	manifestPath := flag.String("manifest", strings.TrimSpace(os.Getenv("GGE_OFFICIAL_DATA_MANIFEST")), "optional JSON manifest with explicit catalog URLs")
	officialURL := flag.String("official-url", defaultOfficialURL(), "official GGE ItemsVersion.properties, items_v<version>.json, or items directory URL")
	fetchLang := flag.Bool("fetch-lang", true, "update LangEn.json from the official language service")
	serverData := flag.String("server-data", "Server/Data", "official Server/Data directory")
	clientData := flag.String("client-data", filepath.Join("Client", "public", "game-data"), "app-ready client game-data directory")
	fetchAssets := flag.Bool("fetch-assets", false, "download missing local assets from each row image_url")
	prunePNG := flag.Bool("prune-png", false, "remove PNG assets when the same catalog image exists as WebP")
	dryRun := flag.Bool("dry-run", false, "log planned writes without changing files")
	flag.Var(&catalogs, "catalog", "catalog to update; can be repeated or comma-separated")
	flag.Parse()

	if len(catalogs) == 0 {
		catalogs = stringListFlag{
			"tools",
			"troops",
			"decorations",
			"resources",
			"effects",
			"effect_caps",
			"equipment_effects",
			"relic_effects",
			"equipments",
			"gems",
			"equipment_sets",
		}
	}

	client := &http.Client{Timeout: 45 * time.Second}
	ctx := context.Background()

	if strings.TrimSpace(*manifestPath) != "" || strings.TrimSpace(*baseURL) != "" {
		officialCatalogs, err := officialCatalogList(catalogs, *baseURL, *manifestPath)
		if err != nil {
			fatal(err)
		}
		fetcher := PublicOfficialDataFetcher{Client: client, ServerData: *serverData, DryRun: *dryRun}
		for _, catalog := range officialCatalogs {
			if err := fetcher.FetchCatalog(ctx, catalog); err != nil {
				fatal(err)
			}
		}
	} else if strings.TrimSpace(*officialURL) != "" {
		source, err := resolveOfficialItemCatalogSource(ctx, client, *officialURL)
		if err != nil {
			fatal(err)
		}
		fetcher := PublicOfficialDataFetcher{Client: client, ServerData: *serverData, DryRun: *dryRun}
		if err := fetcher.FetchOfficialItemCatalog(ctx, source, catalogs); err != nil {
			fatal(err)
		}
	} else {
		fmt.Println("No official data URL configured; converting existing Server/Data only.")
	}
	if *fetchLang {
		fetcher := PublicOfficialDataFetcher{Client: client, ServerData: *serverData, DryRun: *dryRun}
		if err := fetcher.FetchOfficialLanguage(ctx, "en"); err != nil {
			fatal(err)
		}
	}

	converter := AppReadyConverter{
		Client:          client,
		ServerData:      *serverData,
		ClientData:      *clientData,
		FetchAsset:      *fetchAssets,
		PrunePNG:        *prunePNG,
		DryRun:          *dryRun,
		FailedAssetURLs: map[string]struct{}{},
		AssetURLsByType: map[string][]string{},
	}
	if *fetchAssets {
		if err := converter.LoadAssetURLIndex(catalogs); err != nil {
			fatal(err)
		}
	}
	for _, catalogName := range catalogs {
		if err := converter.ConvertCatalog(ctx, catalogName); err != nil {
			fatal(err)
		}
	}
}

func (fetcher PublicOfficialDataFetcher) FetchOfficialLanguage(ctx context.Context, language string) error {
	metadata, err := fetcher.fetchBytes(ctx, defaultOfficialLangMetadata, 0)
	if err != nil {
		return err
	}
	var root struct {
		Metadata struct {
			Version string `json:"versionNo"`
		} `json:"@metadata"`
	}
	if err := json.Unmarshal(metadata, &root); err != nil {
		return fmt.Errorf("decode official language metadata: %w", err)
	}
	version := strings.TrimSpace(root.Metadata.Version)
	if version == "" {
		return fmt.Errorf("official language metadata has no versionNo")
	}
	languageURL := strings.NewReplacer(
		"{version}", url.PathEscape(version),
		"{language}", url.PathEscape(language),
	).Replace(defaultOfficialLangPattern)
	target := filepath.Join(fetcher.ServerData, "Lang"+strings.ToUpper(language[:1])+language[1:]+".json")
	fmt.Printf("official language %s %s\n", language, version)
	return fetcher.fetchJSON(ctx, languageURL, target)
}

func officialCatalogList(catalogs []string, baseURL string, manifestPath string) ([]OfficialCatalog, error) {
	if manifestPath != "" {
		raw, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, err
		}
		var manifest OfficialDataManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			return nil, err
		}
		return manifest.Catalogs, nil
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, nil
	}
	out := make([]OfficialCatalog, 0, len(catalogs))
	for _, name := range catalogs {
		out = append(out, OfficialCatalog{
			Name:     name,
			ItemsURL: baseURL + "/" + name + "/items.json",
			IndexURL: baseURL + "/" + name + "/index.json",
		})
	}
	return out, nil
}

func defaultOfficialURL() string {
	if value := strings.TrimSpace(os.Getenv("GGE_OFFICIAL_DATA_URL")); value != "" {
		return value
	}
	return defaultOfficialItemVersion
}

func parseOfficialItemCatalogURL(rawURL string) (OfficialItemCatalogSource, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return OfficialItemCatalogSource{}, fmt.Errorf("official URL is required")
	}
	if !isHTTPURL(rawURL) {
		return OfficialItemCatalogSource{
			Version:  rawURL,
			ItemsURL: officialItemURLForVersion(defaultOfficialItemPattern, rawURL),
		}, nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return OfficialItemCatalogSource{}, err
	}
	source := OfficialItemCatalogSource{}
	cleanPath := strings.TrimRight(parsed.Path, "/")
	fileName := path.Base(cleanPath)
	dirName := path.Dir(cleanPath)
	switch {
	case fileName == "ItemsVersion.properties":
		source.VersionURL = rawURL
		source.Template = officialURLWithPath(parsed, path.Join(dirName, "items_v{version}.json"))
	case strings.HasPrefix(fileName, "items_v") && strings.HasSuffix(fileName, ".json"):
		source.Version = strings.TrimSuffix(strings.TrimPrefix(fileName, "items_v"), ".json")
		source.VersionURL = officialURLWithPath(parsed, path.Join(dirName, "ItemsVersion.properties"))
		source.Template = officialURLWithPath(parsed, path.Join(dirName, "items_v{version}.json"))
		source.ItemsURL = officialItemURLForVersion(source.Template, source.Version)
	case fileName == "items":
		source.VersionURL = officialURLWithPath(parsed, path.Join(cleanPath, "ItemsVersion.properties"))
		source.Template = officialURLWithPath(parsed, path.Join(cleanPath, "items_v{version}.json"))
	default:
		if strings.Contains(rawURL, "{version}") {
			source.Template = rawURL
		} else {
			return OfficialItemCatalogSource{}, fmt.Errorf("unsupported official URL shape: %s", rawURL)
		}
	}
	return source, nil
}

func resolveOfficialItemCatalogSource(ctx context.Context, client *http.Client, rawURL string) (OfficialItemCatalogSource, error) {
	source, err := parseOfficialItemCatalogURL(rawURL)
	if err != nil {
		return source, err
	}
	if source.VersionURL != "" {
		fetcher := PublicOfficialDataFetcher{Client: client}
		raw, err := fetcher.fetchBytes(ctx, source.VersionURL, 0)
		if err != nil {
			return source, err
		}
		version, err := parseOfficialItemVersion(raw)
		if err != nil {
			return source, err
		}
		source.Version = version
		if source.Template != "" {
			source.ItemsURL = officialItemURLForVersion(source.Template, source.Version)
		}
	}
	if source.ItemsURL == "" {
		source.ItemsURL = officialItemURLForVersion(source.Template, source.Version)
	}
	if source.ItemsURL == "" {
		return source, fmt.Errorf("could not resolve official item catalog URL from %s", rawURL)
	}
	return source, nil
}

func parseOfficialItemVersion(raw []byte) (string, error) {
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "CastleItemXMLVersion" {
			version := strings.TrimSpace(value)
			if version == "" {
				break
			}
			return version, nil
		}
	}
	return "", fmt.Errorf("CastleItemXMLVersion not found")
}

func officialItemURLForVersion(template string, version string) string {
	version = strings.TrimSpace(version)
	if template == "" || version == "" {
		return ""
	}
	template = strings.ReplaceAll(template, "{version}", version)
	return strings.ReplaceAll(template, "%7Bversion%7D", url.PathEscape(version))
}

func officialURLWithPath(parsed *url.URL, value string) string {
	clone := *parsed
	clone.Path = value
	clone.RawQuery = ""
	clone.Fragment = ""
	return clone.String()
}

func (fetcher PublicOfficialDataFetcher) FetchCatalog(ctx context.Context, catalog OfficialCatalog) error {
	name := cleanCatalogName(catalog.Name)
	if name == "" {
		return fmt.Errorf("catalog name is required")
	}
	for _, file := range []struct {
		url  string
		name string
	}{
		{url: catalog.ItemsURL, name: "items.json"},
		{url: catalog.IndexURL, name: "index.json"},
	} {
		if strings.TrimSpace(file.url) == "" {
			continue
		}
		target := filepath.Join(fetcher.ServerData, name, file.name)
		if err := fetcher.fetchJSON(ctx, file.url, target); err != nil {
			return err
		}
	}
	return nil
}

func (fetcher PublicOfficialDataFetcher) FetchOfficialItemCatalog(ctx context.Context, source OfficialItemCatalogSource, catalogs []string) error {
	raw, err := fetcher.fetchBytes(ctx, source.ItemsURL, 0)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root map[string]interface{}
	if err := decoder.Decode(&root); err != nil {
		return fmt.Errorf("decode %s: %w", source.ItemsURL, err)
	}
	if source.Version != "" {
		fmt.Printf("official item catalog %s\n", source.Version)
	}
	for _, catalogName := range catalogs {
		catalogName = cleanCatalogName(catalogName)
		if catalogName == "" {
			continue
		}
		rows, err := officialRowsForCatalog(root, catalogName)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Fprintf(os.Stderr, "official warning: no rows found for catalog %s\n", catalogName)
			continue
		}
		enrichOfficialRows(fetcher.ServerData, catalogName, rows)
		itemsJSON, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		indexJSON, err := json.MarshalIndent(buildOfficialIndex(rows), "", "  ")
		if err != nil {
			return err
		}
		if err := writeFile(filepath.Join(fetcher.ServerData, catalogName, "items.json"), append(itemsJSON, '\n'), fetcher.DryRun); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(fetcher.ServerData, catalogName, "index.json"), append(indexJSON, '\n'), fetcher.DryRun); err != nil {
			return err
		}
		fmt.Printf("official split %-16s %d rows\n", catalogName, len(rows))
	}
	return nil
}

func officialRowsForCatalog(root map[string]interface{}, catalogName string) ([]map[string]interface{}, error) {
	key, filter := officialCatalogSource(catalogName)
	rawRows, ok := root[key].([]interface{})
	if !ok {
		return nil, fmt.Errorf("official catalog %s missing source key %s", catalogName, key)
	}
	rows := make([]map[string]interface{}, 0, len(rawRows))
	for _, rawRow := range rawRows {
		row, ok := rawRow.(map[string]interface{})
		if !ok || !filter(row) {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func officialCatalogSource(catalogName string) (string, func(map[string]interface{}) bool) {
	switch catalogName {
	case "tools":
		return "units", officialUnitIsTool
	case "troops":
		return "units", func(map[string]interface{}) bool { return true }
	case "decorations":
		return "buildings", officialBuildingIsDecoration
	case "equipment_effects", "equipment_sets":
		return catalogName, func(map[string]interface{}) bool { return true }
	case "effect_types":
		return "effecttypes", func(map[string]interface{}) bool { return true }
	default:
		return officialCatalogKey(catalogName), func(map[string]interface{}) bool { return true }
	}
}

func officialCatalogKey(catalogName string) string {
	parts := strings.Split(catalogName, "_")
	if len(parts) == 1 {
		return catalogName
	}
	out := parts[0]
	for _, part := range parts[1:] {
		if part == "" {
			continue
		}
		out += strings.ToUpper(part[:1]) + part[1:]
	}
	return out
}

func officialUnitIsTool(row map[string]interface{}) bool {
	if stringField(row, "slotTypes") != "" {
		return true
	}
	switch stringField(row, "name") {
	case "Workshop", "Dworkshop", "Eventtool", "Elitetool":
		return true
	default:
		return false
	}
}

func officialBuildingIsDecoration(row map[string]interface{}) bool {
	return stringField(row, "buildingGroundType") == "DECO" ||
		stringField(row, "shopCategory") == "DECO" ||
		stringField(row, "name") == "Deco" ||
		strings.Contains(stringField(row, "type"), "Deco")
}

func enrichOfficialRows(serverData string, catalogName string, rows []map[string]interface{}) {
	raw, err := os.ReadFile(filepath.Join(serverData, catalogName, "items.json"))
	if err != nil {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var existing []map[string]interface{}
	if err := decoder.Decode(&existing); err != nil {
		return
	}
	byKey := map[string]map[string]interface{}{}
	byID := map[int64]map[string]interface{}{}
	for _, row := range existing {
		id := rowID(row)
		if id <= 0 {
			continue
		}
		byID[id] = row
		byKey[officialEnrichmentKey(row)] = row
	}
	for _, row := range rows {
		source := byKey[officialEnrichmentKey(row)]
		if source == nil {
			source = byID[rowID(row)]
		}
		if source == nil {
			continue
		}
		copyIfMissing(row, source, "_display_name")
		copyIfMissing(row, source, "image_url")
		copyIfMissing(row, source, "image_local")
	}
}

func officialEnrichmentKey(row map[string]interface{}) string {
	return strconv.FormatInt(rowID(row), 10) + ":" + stringField(row, "type")
}

func copyIfMissing(row map[string]interface{}, source map[string]interface{}, key string) {
	if !isEmptyJSONValue(row[key]) {
		return
	}
	value := source[key]
	if isEmptyJSONValue(value) {
		return
	}
	row[key] = value
}

func isEmptyJSONValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func buildOfficialIndex(rows []map[string]interface{}) []map[string]interface{} {
	index := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		id := rowID(row)
		if id <= 0 {
			continue
		}
		entry := map[string]interface{}{
			"id":    id,
			"name":  officialDisplayName(row),
			"type":  stringField(row, "type"),
			"image": nil,
		}
		if imageURL := stringField(row, "image_url"); imageURL != "" {
			entry["image"] = imageURL
		} else if imageURL := stringField(row, "image"); imageURL != "" {
			entry["image"] = imageURL
		}
		if level, ok := row["level"]; ok && !isEmptyJSONValue(level) {
			entry["level"] = normalizeIndexValue(level)
		}
		index = append(index, entry)
	}
	return index
}

func officialDisplayName(row map[string]interface{}) string {
	if displayName := stringField(row, "_display_name"); displayName != "" {
		return displayName
	}
	if name := stringField(row, "name"); name != "" {
		return name
	}
	return "Unknown"
}

func normalizeIndexValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		if decimal, err := typed.Float64(); err == nil {
			return decimal
		}
	case string:
		if integer, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return integer
		}
	}
	return value
}

func (fetcher PublicOfficialDataFetcher) fetchJSON(ctx context.Context, sourceURL string, target string) error {
	raw, err := fetcher.fetchBytes(ctx, sourceURL, 0)
	if err != nil {
		return err
	}
	var value interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode %s: %w", sourceURL, err)
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Printf("official fetch %s -> %s\n", sourceURL, target)
	return writeFile(target, append(formatted, '\n'), fetcher.DryRun)
}

func (fetcher PublicOfficialDataFetcher) fetchBytes(ctx context.Context, sourceURL string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json,*/*")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	resp, err := fetcher.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s returned %s", sourceURL, resp.Status)
	}
	reader := io.Reader(resp.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes+1)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("%s exceeded %d bytes", sourceURL, maxBytes)
	}
	return raw, nil
}

func (converter AppReadyConverter) ConvertCatalog(ctx context.Context, catalogName string) error {
	catalogName = cleanCatalogName(catalogName)
	if catalogName == "" {
		return nil
	}
	for _, fileName := range []string{"items.json", "index.json"} {
		source := filepath.Join(converter.ServerData, catalogName, fileName)
		if _, err := os.Stat(source); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		target := filepath.Join(converter.ClientData, catalogName, fileName)
		if err := converter.convertJSONFile(ctx, catalogName, source, target); err != nil {
			return err
		}
	}
	if converter.PrunePNG {
		if err := converter.pruneCatalogPNGs(catalogName); err != nil {
			return err
		}
	}
	return nil
}

func (converter AppReadyConverter) convertJSONFile(ctx context.Context, catalogName string, source string, target string) error {
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode %s: %w", source, err)
	}
	rows := collectConvertibleRows(value)
	progress := newProgress(fmt.Sprintf("convert %s/%s", catalogName, filepath.Base(source)), len(rows))
	for _, row := range rows {
		if err := converter.convertRow(ctx, catalogName, row); err != nil {
			progress.Done()
			return err
		}
		progress.Step()
	}
	progress.Done()
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(target, append(formatted, '\n'), converter.DryRun)
}

func collectConvertibleRows(value interface{}) []map[string]interface{} {
	rows := []map[string]interface{}{}
	collectConvertibleRowsInto(value, &rows)
	return rows
}

func collectConvertibleRowsInto(value interface{}, rows *[]map[string]interface{}) {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if row, ok := item.(map[string]interface{}); ok {
				if rowID(row) > 0 {
					*rows = append(*rows, row)
				}
			}
		}
	case map[string]interface{}:
		if items, ok := typed["items"]; ok {
			collectConvertibleRowsInto(items, rows)
			return
		}
		for _, item := range typed {
			if row, ok := item.(map[string]interface{}); ok {
				collectConvertibleRowsInto(row, rows)
			}
		}
		if rowID(typed) > 0 {
			*rows = append(*rows, typed)
		}
	}
}

func (converter AppReadyConverter) LoadAssetURLIndex(catalogs []string) error {
	catalogNames, err := converter.assetIndexCatalogs(catalogs)
	if err != nil {
		return err
	}
	for _, catalogName := range catalogNames {
		catalogName = cleanCatalogName(catalogName)
		if catalogName == "" {
			continue
		}
		for _, fileName := range []string{"items.json", "index.json"} {
			source := filepath.Join(converter.ServerData, catalogName, fileName)
			raw, err := os.ReadFile(source)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.UseNumber()
			var value interface{}
			if err := decoder.Decode(&value); err != nil {
				return fmt.Errorf("decode %s: %w", source, err)
			}
			collectAssetURLsByType(value, converter.AssetURLsByType)
		}
	}
	for key, values := range converter.AssetURLsByType {
		sort.SliceStable(values, func(i, j int) bool {
			return assetURLRank(values[i]) < assetURLRank(values[j])
		})
		converter.AssetURLsByType[key] = values
	}
	return nil
}

func (converter AppReadyConverter) assetIndexCatalogs(catalogs []string) ([]string, error) {
	out := []string{}
	for _, catalogName := range catalogs {
		if clean := cleanCatalogName(catalogName); clean != "" {
			out = appendUniqueString(out, clean)
		}
	}
	entries, err := os.ReadDir(converter.ServerData)
	if err != nil {
		return out, err
	}
	dirNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirNames = append(dirNames, entry.Name())
		}
	}
	sort.Strings(dirNames)
	for _, dirName := range dirNames {
		if fileExists(filepath.Join(converter.ServerData, dirName, "items.json")) || fileExists(filepath.Join(converter.ServerData, dirName, "index.json")) {
			out = appendUniqueString(out, dirName)
		}
	}
	return out, nil
}

func collectAssetURLsByType(value interface{}, byType map[string][]string) {
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			collectAssetURLsByType(item, byType)
		}
	case map[string]interface{}:
		rowType := stringField(typed, "type")
		if rowType != "" {
			for _, imageURL := range []string{stringField(typed, "image_url"), stringField(typed, "image")} {
				if isHTTPURL(imageURL) {
					byType[rowType] = appendUniqueString(byType[rowType], imageURL)
				}
			}
		}
		for _, item := range typed {
			switch nested := item.(type) {
			case []interface{}:
				collectAssetURLsByType(nested, byType)
			case map[string]interface{}:
				collectAssetURLsByType(nested, byType)
			}
		}
	}
}

func (converter AppReadyConverter) convertRow(ctx context.Context, catalogName string, row map[string]interface{}) error {
	id := rowID(row)
	if id <= 0 {
		return nil
	}
	assetPath, err := converter.resolveAsset(ctx, catalogName, id, row)
	if err != nil {
		return err
	}
	if assetPath == "" {
		clearAppImageFields(row)
		return nil
	}
	if err := converter.copyAsset(catalogName, assetPath); err != nil {
		return err
	}
	appPath := "/game-data/" + catalogName + "/images/" + filepath.Base(assetPath)
	row["image"] = appPath
	if _, ok := row["image_local"]; ok {
		row["image_local"] = appPath
	}
	if _, ok := row["image_url"]; ok {
		row["image_url"] = appPath
	}
	return nil
}

func clearAppImageFields(row map[string]interface{}) {
	for _, key := range []string{"image", "image_local", "image_url"} {
		if _, ok := row[key]; ok {
			row[key] = nil
		}
	}
}

func (converter AppReadyConverter) resolveAsset(ctx context.Context, catalogName string, id int64, row map[string]interface{}) (string, error) {
	imageURLs := converter.imageURLsForRow(row)
	if converter.FetchAsset {
		if candidate := converter.findAssetByID(catalogName, id, ".webp"); candidate != "" {
			return candidate, nil
		}
		triedWebP := false
		for _, imageURL := range imageURLs {
			if !isHTTPURL(imageURL) || assetExtensionFromURL(imageURL) != ".webp" {
				continue
			}
			triedWebP = true
			assetPath, err := converter.downloadAsset(ctx, catalogName, id, imageURL)
			if err != nil {
				return "", err
			}
			if assetPath != "" {
				return assetPath, nil
			}
		}
		if triedWebP {
			return "", nil
		}
	}
	if local := stringField(row, "image_local"); local != "" {
		if candidate := converter.resolveLocalAsset(catalogName, local); candidate != "" {
			return candidate, nil
		}
	}
	if candidate := converter.findAssetByID(catalogName, id, ""); candidate != "" {
		return candidate, nil
	}
	if !converter.FetchAsset {
		return "", nil
	}
	for _, imageURL := range imageURLs {
		if !isHTTPURL(imageURL) {
			continue
		}
		assetPath, err := converter.downloadAsset(ctx, catalogName, id, imageURL)
		if err != nil {
			return "", err
		}
		if assetPath != "" {
			return assetPath, nil
		}
	}
	return "", nil
}

func (converter AppReadyConverter) imageURLsForRow(row map[string]interface{}) []string {
	out := []string{}
	for _, key := range []string{"image_url", "image"} {
		if value := stringField(row, key); value != "" {
			out = appendUniqueString(out, value)
		}
	}
	if rowType := stringField(row, "type"); rowType != "" {
		for _, value := range converter.AssetURLsByType[rowType] {
			out = appendUniqueString(out, value)
		}
	}
	return out
}

func (converter AppReadyConverter) resolveLocalAsset(catalogName string, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || isHTTPURL(value) {
		return ""
	}
	candidates := []string{}
	if filepath.IsAbs(value) {
		candidates = append(candidates, value)
	} else {
		candidates = append(candidates, filepath.Clean(value))
		candidates = append(candidates, filepath.Join(converter.ServerData, catalogName, "images", filepath.Base(value)))
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func (converter AppReadyConverter) findAssetByID(catalogName string, id int64, ext string) string {
	pattern := filepath.Join(converter.ServerData, catalogName, "images", strconv.FormatInt(id, 10)+"."+strings.TrimPrefix(ext, "."))
	if ext == "" {
		pattern = filepath.Join(converter.ServerData, catalogName, "images", strconv.FormatInt(id, 10)+".*")
	}
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return ""
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return assetExtensionRank(matches[i]) < assetExtensionRank(matches[j])
	})
	return matches[0]
}

func (converter AppReadyConverter) downloadAsset(ctx context.Context, catalogName string, id int64, imageURL string) (string, error) {
	if _, failed := converter.FailedAssetURLs[imageURL]; failed {
		return "", nil
	}
	fetcher := PublicOfficialDataFetcher{Client: converter.Client, DryRun: converter.DryRun}
	raw, err := fetcher.fetchBytes(ctx, imageURL, maxOfficialAssetBytes)
	if err != nil {
		converter.FailedAssetURLs[imageURL] = struct{}{}
		fmt.Fprintf(os.Stderr, "asset warning: skipped %s %d %s: %v\n", catalogName, id, imageURL, err)
		return "", nil
	}
	ext := assetExtensionFromURL(imageURL)
	if ext == "" {
		ext = ".webp"
	}
	target := filepath.Join(converter.ServerData, catalogName, "images", strconv.FormatInt(id, 10)+ext)
	if err := writeFile(target, raw, converter.DryRun); err != nil {
		return "", err
	}
	return target, nil
}

func (converter AppReadyConverter) copyAsset(catalogName string, source string) error {
	target := filepath.Join(converter.ClientData, catalogName, "images", filepath.Base(source))
	if source == target {
		return nil
	}
	if converter.DryRun {
		return nil
	}
	raw, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeFile(target, raw, converter.DryRun)
}

func (converter AppReadyConverter) pruneCatalogPNGs(catalogName string) error {
	for _, imageDir := range []string{
		filepath.Join(converter.ServerData, catalogName, "images"),
		filepath.Join(converter.ClientData, catalogName, "images"),
	} {
		if err := prunePNGsWithWebP(imageDir, converter.DryRun); err != nil {
			return err
		}
	}
	return nil
}

func prunePNGsWithWebP(imageDir string, dryRun bool) error {
	matches, err := filepath.Glob(filepath.Join(imageDir, "*.png"))
	if err != nil {
		return err
	}
	prunable := []string{}
	for _, pngPath := range matches {
		webpPath := strings.TrimSuffix(pngPath, filepath.Ext(pngPath)) + ".webp"
		if !fileExists(webpPath) {
			continue
		}
		prunable = append(prunable, pngPath)
	}
	progress := newProgress(fmt.Sprintf("prune %s", filepath.ToSlash(imageDir)), len(prunable))
	for _, pngPath := range prunable {
		if !dryRun {
			if err := os.Remove(pngPath); err != nil {
				progress.Done()
				return err
			}
		}
		progress.Step()
	}
	progress.Done()
	return nil
}

type progressBar struct {
	label    string
	total    int
	current  int
	width    int
	lastDraw time.Time
}

func newProgress(label string, total int) *progressBar {
	progress := &progressBar{label: label, total: total, width: 24}
	if total > 0 {
		progress.draw(false)
	}
	return progress
}

func (progress *progressBar) Step() {
	if progress == nil || progress.total <= 0 {
		return
	}
	progress.current++
	if progress.current < progress.total && time.Since(progress.lastDraw) >= 120*time.Millisecond {
		progress.draw(false)
	}
}

func (progress *progressBar) Done() {
	if progress == nil || progress.total <= 0 {
		return
	}
	if progress.current < progress.total {
		progress.current = progress.total
	}
	progress.draw(true)
	fmt.Print("\n")
}

func (progress *progressBar) draw(done bool) {
	if progress.total <= 0 {
		return
	}
	if progress.current > progress.total {
		progress.current = progress.total
	}
	filled := progress.width * progress.current / progress.total
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", progress.width-filled)
	percent := 100 * progress.current / progress.total
	status := "working"
	if done || progress.current >= progress.total {
		status = "done"
	}
	fmt.Printf("\r%-34s [%s] %3d%% %d/%d %s", progress.label, bar, percent, progress.current, progress.total, status)
	progress.lastDraw = time.Now()
}

func writeFile(path string, data []byte, dryRun bool) error {
	if dryRun {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func rowID(row map[string]interface{}) int64 {
	for _, key := range []string{
		"id",
		"wodID",
		"wid",
		"resourceID",
		"effectID",
		"effectTypeID",
		"capID",
		"equipmentEffectID",
		"equipmentID",
		"gemID",
		"ID",
	} {
		if id := int64Field(row, key); id > 0 {
			return id
		}
	}
	return 0
}

func int64Field(row map[string]interface{}, key string) int64 {
	switch value := row[key].(type) {
	case json.Number:
		out, _ := value.Int64()
		return out
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case string:
		out, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return out
	default:
		return 0
	}
}

func stringField(row map[string]interface{}, key string) string {
	value, _ := row[key].(string)
	return strings.TrimSpace(value)
}

func cleanCatalogName(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	value = filepath.ToSlash(value)
	if strings.Contains(value, "..") || strings.ContainsAny(value, `\:`) {
		return ""
	}
	return value
}

func assetExtensionRank(path string) int {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".webp":
		return 0
	case ".png":
		return 1
	case ".jpg", ".jpeg":
		return 2
	default:
		return 3
	}
}

func assetURLRank(value string) int {
	rank := 100
	if assetExtensionFromURL(value) == ".webp" {
		rank -= 50
	}
	if strings.Contains(filepath.Base(value), "--") {
		rank -= 25
	}
	return rank
}

func assetExtensionFromURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(parsed.Path))
	if ext != "" {
		return ext
	}
	if contentType := parsed.Query().Get("contentType"); contentType != "" {
		extensions, _ := mime.ExtensionsByType(contentType)
		if len(extensions) > 0 {
			return extensions[0]
		}
	}
	return ""
}

func isHTTPURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
