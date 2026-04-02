// Command genwoddisplaynames builds Server/Data/WodDisplayNames.json by resolving every
// buildings.json row the same way General's Camp does: GGS lang keys deco_{type}_name, then
// the items.json "name" field (see forum/overviews/decorations/script.js getName).
//
// By default reads the committed English deco-name subset at Server/Data/LangPack/EmpireDecoLang-en.json
// (no network). Refresh that file from GGS when strings drift:
//
//	go run ./Server/cmd/genwoddisplaynames -update-lang
//
// Usage: from repo root: go run ./Server/cmd/genwoddisplaynames
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	langMetaURL = "https://langserv.public.ggs-ep.com/12/fr/@metadata"
	langEnFmt   = "https://langserv.public.ggs-ep.com/12@%s/en/*"
)

type metaDoc struct {
	Meta struct {
		VersionNo  string `json:"versionNo"`
		DeployTime any    `json:"deployTime"`
	} `json:"@metadata"`
}

type langPackFile struct {
	LangVersion    string            `json:"langVersion"`
	LangDeployTime any               `json:"langDeployTime,omitempty"`
	ExtractedAt    string            `json:"extractedAt"`
	SourceURL      string            `json:"sourceUrl"`
	Note           string            `json:"note"`
	Strings        map[string]string `json:"strings"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "genwoddisplaynames: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	updateLang := flag.Bool("update-lang", false, "fetch GGS English pack, rewrite LangPack/EmpireDecoLang-en.json (deco_*_name keys only), then generate")
	flag.Parse()

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	dataDir := filepath.Join(repoRoot, "Server", "Data")
	buildingsPath := filepath.Join(dataDir, "EmpireItems", "buildings.json")
	metaPath := filepath.Join(dataDir, "EmpireItemsMeta.json")
	langPackPath := filepath.Join(dataDir, "LangPack", "EmpireDecoLang-en.json")

	metaRaw, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read EmpireItemsMeta: %w", err)
	}
	var itemsMeta struct {
		CastleItemXMLVersion string `json:"castleItemXMLVersion"`
	}
	if err := json.Unmarshal(metaRaw, &itemsMeta); err != nil {
		return err
	}

	client := &http.Client{Timeout: 120 * time.Second}

	var langVer string
	var langMap map[string]string

	if *updateLang {
		if err := fetchAndWriteLangPack(client, langPackPath); err != nil {
			return err
		}
	}

	langVer, langMap, err = readLangPack(langPackPath)
	if err != nil {
		return err
	}

	buildingsRaw, err := os.ReadFile(buildingsPath)
	if err != nil {
		return fmt.Errorf("read buildings: %w", err)
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(buildingsRaw, &rows); err != nil {
		return err
	}

	names := make(map[string]string, len(rows))
	for _, m := range rows {
		wid := intFromAny(m["wodID"])
		if wid <= 0 {
			continue
		}
		typ, _ := m["type"].(string)
		jsonName, _ := m["name"].(string)
		display := generalscampStyleDisplayName(typ, jsonName, langMap)
		if display == "" {
			display = fmt.Sprintf("WID %d", wid)
		}
		names[strconv.Itoa(wid)] = display
	}

	outDoc := map[string]interface{}{
		"langVersion":  langVer,
		"itemsVersion": itemsMeta.CastleItemXMLVersion,
		"sourceNote":   "Names resolved like generalscamp.github.io/forum/overviews/decorations (deco_{type}_name from local LangPack + buildings name fallback). Regenerate: go run ./Server/cmd/genwoddisplaynames",
		"names":        names,
	}
	outPath := filepath.Join(dataDir, "WodDisplayNames.json")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(outDoc); err != nil {
		return err
	}
	fmt.Printf("Wrote %s (%d wodIDs, lang=%s items=%s)\n", outPath, len(names), langVer, itemsMeta.CastleItemXMLVersion)
	return nil
}

func readLangPack(path string) (version string, strings map[string]string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read lang pack %s: %w (run with -update-lang to fetch from GGS)", path, err)
	}
	var doc langPackFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", nil, fmt.Errorf("parse lang pack: %w", err)
	}
	if len(doc.Strings) == 0 {
		return "", nil, fmt.Errorf("lang pack has no strings")
	}
	if doc.LangVersion == "" {
		doc.LangVersion = "unknown"
	}
	return doc.LangVersion, doc.Strings, nil
}

func fetchAndWriteLangPack(c *http.Client, outPath string) error {
	ver, deployTime, err := fetchLangMeta(c)
	if err != nil {
		return fmt.Errorf("lang metadata: %w", err)
	}
	fullMap, err := fetchEnglishLangFull(c, ver)
	if err != nil {
		return fmt.Errorf("lang en: %w", err)
	}
	slim := make(map[string]string)
	for k, v := range fullMap {
		kl := strings.ToLower(k)
		if strings.HasPrefix(kl, "deco_") && strings.HasSuffix(kl, "_name") && strings.TrimSpace(v) != "" {
			slim[kl] = strings.TrimSpace(v)
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	doc := langPackFile{
		LangVersion:    ver,
		LangDeployTime: deployTime,
		ExtractedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		SourceURL:      fmt.Sprintf(langEnFmt, ver),
		Note:           "Subset of GGS English pack: keys deco_*_name only. Used offline by genwoddisplaynames. Refresh: go run ./Server/cmd/genwoddisplaynames -update-lang",
		Strings:        slim,
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	fmt.Printf("Wrote %s (%d deco_*_name keys, langVersion=%s)\n", outPath, len(slim), ver)
	return nil
}

func fetchLangMeta(c *http.Client) (version string, deployTime any, err error) {
	res, err := c.Get(langMetaURL)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return "", nil, fmt.Errorf("metadata HTTP %d: %s", res.StatusCode, string(b[:min(200, len(b))]))
	}
	var doc metaDoc
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return "", nil, err
	}
	if doc.Meta.VersionNo == "" {
		return "", nil, fmt.Errorf("empty versionNo in metadata")
	}
	return doc.Meta.VersionNo, doc.Meta.DeployTime, nil
}

func fetchEnglishLangFull(c *http.Client, version string) (map[string]string, error) {
	url := fmt.Sprintf(langEnFmt, version)
	res, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("lang HTTP %d: %s", res.StatusCode, string(b[:min(200, len(b))]))
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if k == "@metadata" {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) != nil {
			continue
		}
		if s != "" {
			out[k] = s
		}
	}
	return out, nil
}

func generalscampStyleDisplayName(typ, jsonName string, lang map[string]string) string {
	typeStr := typ
	keyOrig := strings.ToLower("deco_" + typeStr + "_name")
	keyLower := strings.ToLower("deco_" + strings.ToLower(typeStr) + "_name")
	keyFirst := ""
	if len(typeStr) > 0 {
		keyFirst = strings.ToLower("deco_" + strings.ToLower(typeStr[:1]) + typeStr[1:] + "_name")
	}
	for _, k := range []string{keyOrig, keyLower, keyFirst} {
		if k == "" {
			continue
		}
		if v := lang[k]; strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	if t := strings.TrimSpace(jsonName); t != "" {
		return t
	}
	if strings.TrimSpace(typeStr) != "" {
		return strings.TrimSpace(typeStr)
	}
	return ""
}

func intFromAny(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "Server", "Data", "EmpireItemsMeta.json")); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find repo root with go.mod and Server/Data/EmpireItemsMeta.json (cwd=%s)", wd)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
