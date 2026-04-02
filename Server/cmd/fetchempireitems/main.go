// Command fetchempireitems downloads GGE items_v*.json, normalizes decoration rows in buildings,
// and writes pretty-printed per-section files under Server/Data/EmpireItems plus EmpireItemsMeta.json.
//
// Run from repository root (directory containing go.mod):
//
//	go run ./Server/cmd/fetchempireitems
//
// Optional flags allow custom root or HTTP timeouts for background / automation use.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"CitadelDesktop/Server/internal/empireitems"
)

const (
	itemsVersionURL = "https://empire-html5.goodgamestudios.com/default/items/ItemsVersion.properties"
	itemsBaseURL    = "https://empire-html5.goodgamestudios.com/default/items"
)

func main() {
	rootFlag := flag.String("root", "", "repository root (contains go.mod); default: walk up from cwd")
	timeout := flag.Duration("timeout", 5*time.Minute, "HTTP client timeout for download")
	flag.Parse()

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dataDir := filepath.Join(root, "Server", "Data")
	itemsDir := filepath.Join(dataDir, "EmpireItems")

	client := &http.Client{Timeout: *timeout}

	ver, err := fetchItemsVersion(client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "version:", err)
		os.Exit(1)
	}
	itemsURL := fmt.Sprintf("%s/items_v%s.json", itemsBaseURL, ver)
	raw, err := fetchBody(client, itemsURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "items:", err)
		os.Exit(1)
	}

	var rootObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rootObj); err != nil {
		fmt.Fprintln(os.Stderr, "parse items json:", err)
		os.Exit(1)
	}

	if br, ok := rootObj["buildings"]; ok {
		var buildings []map[string]any
		if err := json.Unmarshal(br, &buildings); err == nil {
			empireitems.NormalizeDecoBuildings(buildings)
			out, err := json.Marshal(buildings)
			if err != nil {
				fmt.Fprintln(os.Stderr, "re-marshal buildings:", err)
				os.Exit(1)
			}
			rootObj["buildings"] = out
		}
	}

	if err := os.MkdirAll(itemsDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	entries, err := os.ReadDir(itemsDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		_ = os.Remove(filepath.Join(itemsDir, e.Name()))
	}

	keys := make([]string, 0, len(rootObj))
	for k := range rootObj {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		var v any
		if err := json.Unmarshal(rootObj[k], &v); err != nil {
			fmt.Fprintln(os.Stderr, "section", k, err)
			os.Exit(1)
		}
		pretty, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "indent", k, err)
			os.Exit(1)
		}
		path := filepath.Join(itemsDir, k+".json")
		if err := os.WriteFile(path, append(pretty, '\n'), 0644); err != nil {
			fmt.Fprintln(os.Stderr, path, err)
			os.Exit(1)
		}
	}

	meta := map[string]any{
		"castleItemXMLVersion": ver,
		"sectionCount":         len(rootObj),
		"fetchedAt":            time.Now().UTC().Format(time.RFC3339Nano),
		"sourceUrl":            itemsURL,
		"itemsDirectory":       "EmpireItems",
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	metaPath := filepath.Join(dataDir, "EmpireItemsMeta.json")
	if err := os.WriteFile(metaPath, append(metaBytes, '\n'), 0644); err != nil {
		fmt.Fprintln(os.Stderr, metaPath, err)
		os.Exit(1)
	}

	_ = os.Remove(filepath.Join(dataDir, "EmpireItems.json"))

	fmt.Printf("wrote %d sections under %s, version %s\n", len(rootObj), itemsDir, ver)
}

func resolveRoot(flagRoot string) (string, error) {
	if flagRoot != "" {
		abs, err := filepath.Abs(flagRoot)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
			return "", fmt.Errorf("-root %q: go.mod not found", abs)
		}
		return abs, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found from cwd; use -root")
		}
		dir = parent
	}
}

func fetchItemsVersion(client *http.Client) (string, error) {
	body, err := fetchBody(client, itemsVersionURL)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(body))
	const p = "CastleItemXMLVersion="
	if !strings.HasPrefix(line, p) {
		return "", fmt.Errorf("unexpected version line: %q", line)
	}
	return strings.TrimSpace(strings.TrimPrefix(line, p)), nil
}

func fetchBody(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%s: %s (%s)", url, resp.Status, strings.TrimSpace(string(slurp)))
	}
	return io.ReadAll(resp.Body)
}
