// Command fetchggeassets downloads the live GGE HTML5 ggs.dll bundle, parses embedded
// ItemVersions / InterfaceVersions path maps, optionally mirrors .png and .json under
// Server/Data/GGEHtml5Assets/mirror/, and writes manifest.json.
//
// Run from repository root:
//
//	go run ./Server/cmd/fetchggeassets
//
//	go run ./Server/cmd/fetchggeassets -skip-download
//	go run ./Server/cmd/fetchggeassets -webp
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	html5Origin     = "https://empire-html5.goodgamestudios.com"
	defaultIndex    = html5Origin + "/default/index.html"
	assetsPrefix    = "/default/assets/"
	itemFillAnchor  = "ItemVersions.prototype.fill=function(){"
	ivFillAnchor    = "InterfaceVersions.prototype.fill=function(){"
	ivFillEndMarker = "},InterfaceVersions}();"
)

var assetAssignRE = regexp.MustCompile(`this\.assets\.([A-Za-z0-9_]+)="([^"]+)"`)

func main() {
	rootFlag := flag.String("root", "", "repository root (contains go.mod); default: walk up from cwd")
	outDir := flag.String("out", "", "output directory; default: <root>/Server/Data/GGEHtml5Assets")
	skipDownload := flag.Bool("skip-download", false, "only fetch dll, parse, write manifest (no mirror files)")
	includeWebp := flag.Bool("webp", false, "also try downloading .webp for each asset")
	workers := flag.Int("workers", 24, "parallel download workers")
	timeout := flag.Duration("timeout", 2*time.Minute, "per-request HTTP timeout")
	flag.Parse()

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dest := *outDir
	if dest == "" {
		dest = filepath.Join(root, "Server", "Data", "GGEHtml5Assets")
	}
	mirrorDir := filepath.Join(dest, "mirror")
	manifestPath := filepath.Join(dest, "manifest.json")

	client := &http.Client{Timeout: *timeout}

	fmt.Fprintf(os.Stderr, "fetch index %s\n", defaultIndex)
	indexHTML, err := fetchBytes(client, defaultIndex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "index:", err)
		os.Exit(1)
	}
	dllRel, err := extractDLLPath(string(indexHTML))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dllURL := html5Origin + "/default/" + dllRel
	fmt.Fprintf(os.Stderr, "fetch %s\n", dllURL)
	dllBody, err := fetchBytes(client, dllURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dll:", err)
		os.Exit(1)
	}
	dllStr := string(dllBody)

	ivEntries, err := parseInterfaceAssets(dllStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse interface:", err)
		os.Exit(1)
	}
	itemEntries, err := parseItemAssets(dllStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse item:", err)
		os.Exit(1)
	}

	all := append(ivEntries, itemEntries...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Category != all[j].Category {
			return all[i].Category < all[j].Category
		}
		return all[i].Key < all[j].Key
	})

	assetsBase := html5Origin + assetsPrefix
	mf := Manifest{
		FetchedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		IndexURL:      defaultIndex,
		GGSDllURL:     dllURL,
		AssetsBaseURL: assetsBase,
		Assets:        make([]AssetRecord, 0, len(all)),
	}

	exts := []string{".png", ".json"}
	if *includeWebp {
		exts = append(exts, ".webp")
	}

	if *skipDownload {
		for _, e := range all {
			ar := AssetRecord{Key: e.Key, Category: e.Category, BasePath: e.BasePath, Files: nil}
			for _, ext := range exts {
				ar.Files = append(ar.Files, FileRecord{
					Ext: ext,
					URL: assetsBase + e.BasePath + ext,
				})
			}
			mf.Assets = append(mf.Assets, ar)
		}
		if err := writeManifest(manifestPath, &mf); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d assets, skip-download)\n", manifestPath, len(mf.Assets))
		return
	}

	if err := os.MkdirAll(mirrorDir, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var done uint64
	total := uint64(len(all) * len(exts))

	jobs := make(chan AssetEntry, len(all))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				rec := AssetRecord{Key: e.Key, Category: e.Category, BasePath: e.BasePath}
				for _, ext := range exts {
					url := assetsBase + e.BasePath + ext
					relFile := filepath.FromSlash(e.BasePath + ext)
					if strings.Contains(relFile, "..") {
						rec.Files = append(rec.Files, FileRecord{Ext: ext, URL: url, Err: "unsafe path"})
						atomic.AddUint64(&done, 1)
						continue
					}
					localPath := filepath.Join(mirrorDir, relFile)
					fr := FileRecord{Ext: ext, URL: url, Path: relFile}
					st, n, err := downloadToFile(client, url, localPath)
					fr.HTTPStatus = st
					if err != nil {
						fr.Err = err.Error()
					} else if st == http.StatusOK {
						fr.OK = true
						fr.Size = n
					}
					rec.Files = append(rec.Files, fr)
					atomic.AddUint64(&done, 1)
				}
				mu.Lock()
				mf.Assets = append(mf.Assets, rec)
				mu.Unlock()
				if atomic.LoadUint64(&done)%500 == 0 {
					fmt.Fprintf(os.Stderr, "progress %d / %d file attempts\n", atomic.LoadUint64(&done), total)
				}
			}
		}()
	}
	for _, e := range all {
		jobs <- e
	}
	close(jobs)
	wg.Wait()

	sort.Slice(mf.Assets, func(i, j int) bool {
		if mf.Assets[i].Category != mf.Assets[j].Category {
			return mf.Assets[i].Category < mf.Assets[j].Category
		}
		return mf.Assets[i].Key < mf.Assets[j].Key
	})

	if err := writeManifest(manifestPath, &mf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var okCount, failCount int
	for _, a := range mf.Assets {
		for _, f := range a.Files {
			if f.OK {
				okCount++
			} else if f.HTTPStatus != http.StatusNotFound {
				failCount++
			}
		}
	}
	fmt.Printf("wrote %s and mirror under %s (%d logical assets, %d ok files, %d non-404 errors)\n",
		manifestPath, mirrorDir, len(mf.Assets), okCount, failCount)
}

type Manifest struct {
	FetchedAt     string        `json:"fetchedAt"`
	IndexURL      string        `json:"indexUrl"`
	GGSDllURL     string        `json:"ggsDllUrl"`
	AssetsBaseURL string        `json:"assetsBaseUrl"`
	Assets        []AssetRecord `json:"assets"`
}

type AssetRecord struct {
	Key      string       `json:"key"`
	Category string       `json:"category"`
	BasePath string       `json:"basePath"`
	Files    []FileRecord `json:"files"`
}

type FileRecord struct {
	Ext        string `json:"ext"`
	URL        string `json:"url"`
	Path       string `json:"path,omitempty"`
	OK         bool   `json:"ok"`
	Size       int64  `json:"size,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Err        string `json:"err,omitempty"`
}

type AssetEntry struct {
	Key      string
	Category string
	BasePath string
}

func writeManifest(path string, mf *Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

func extractDLLPath(html string) (string, error) {
	re := regexp.MustCompile(`dll/ggs\.dll\.[a-f0-9]+\.js`)
	m := re.FindString(html)
	if m == "" {
		return "", fmt.Errorf("ggs.dll path not found in index.html")
	}
	return m, nil
}

func parseInterfaceAssets(dll string) ([]AssetEntry, error) {
	i := strings.Index(dll, ivFillAnchor)
	if i < 0 {
		return nil, fmt.Errorf("%q not found", ivFillAnchor)
	}
	j := strings.Index(dll[i:], ivFillEndMarker)
	if j < 0 {
		return nil, fmt.Errorf("%q not found after interface fill", ivFillEndMarker)
	}
	chunk := dll[i : i+j]
	var out []AssetEntry
	for _, m := range assetAssignRE.FindAllStringSubmatch(chunk, -1) {
		out = append(out, AssetEntry{Key: m[1], Category: "interface", BasePath: m[2]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no interface assets parsed")
	}
	return out, nil
}

func parseItemAssets(dll string) ([]AssetEntry, error) {
	i := strings.Index(dll, itemFillAnchor)
	if i < 0 {
		return nil, fmt.Errorf("%q not found", itemFillAnchor)
	}
	openBrace := i + len(itemFillAnchor) - 1
	if openBrace < 0 || dll[openBrace] != '{' {
		return nil, fmt.Errorf("expected '{' before item fill body")
	}
	closeBrace, err := matchingBrace(dll, openBrace)
	if err != nil {
		return nil, err
	}
	body := dll[openBrace+1 : closeBrace]
	var out []AssetEntry
	for _, m := range assetAssignRE.FindAllStringSubmatch(body, -1) {
		out = append(out, AssetEntry{Key: m[1], Category: "item", BasePath: m[2]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no item assets parsed")
	}
	return out, nil
}

func matchingBrace(s string, openIdx int) (int, error) {
	if openIdx >= len(s) || s[openIdx] != '{' {
		return -1, fmt.Errorf("matchingBrace: not at '{'")
	}
	depth := 0
	for k := openIdx; k < len(s); k++ {
		switch s[k] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return k, nil
			}
		}
	}
	return -1, fmt.Errorf("unclosed '{' in item fill")
}

func downloadToFile(client *http.Client, url, dest string) (status int, n int64, err error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	status = resp.StatusCode
	if status != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return status, 0, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return status, 0, err
	}
	var buf bytes.Buffer
	n64, err := io.Copy(&buf, resp.Body)
	if err != nil {
		return status, 0, err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0644); err != nil {
		return status, 0, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return status, 0, err
	}
	return status, n64, nil
}

func fetchBytes(client *http.Client, url string) ([]byte, error) {
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
