package featureview

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Paths"
)

var (
	tciLangMap  map[string]string
	tciLangOnce sync.Once
)

func parseLangPackJSON(b []byte) map[string]string {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		log.Println("[TCI Language] Failed to parse language data:", err)
		return nil
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		if strVal, ok := v.(string); ok {
			result[strings.ToLower(k)] = strVal
		}
	}
	return result
}

func langEnCachePath() string {
	return filepath.Join(Paths.DataDir(), "LangEn.json")
}

func downloadLanguagePack() map[string]string {
	log.Println("[TCI Language] Downloading language pack...")

	metaResp, err := http.Get("https://langserv.public.ggs-ep.com/12/en/@metadata")
	if err != nil {
		log.Println("[TCI Language] Failed to fetch metadata:", err)
		return nil
	}
	defer metaResp.Body.Close()

	var meta struct {
		Metadata struct {
			VersionNo int `json:"versionNo"`
		} `json:"@metadata"`
	}
	if err := json.NewDecoder(metaResp.Body).Decode(&meta); err != nil {
		log.Println("[TCI Language] Failed to decode metadata:", err)
		return nil
	}

	url := fmt.Sprintf("https://langserv.public.ggs-ep.com/12@%d/en/*", meta.Metadata.VersionNo)
	langResp, err := http.Get(url)
	if err != nil {
		log.Println("[TCI Language] Failed to fetch language data:", err)
		return nil
	}
	defer langResp.Body.Close()

	langBytes, err := io.ReadAll(langResp.Body)
	if err != nil {
		log.Println("[TCI Language] Failed to read language data:", err)
		return nil
	}

	if err := os.WriteFile(langEnCachePath(), langBytes, 0644); err != nil {
		log.Println("[TCI Language] Failed to cache language data:", err)
	}

	return parseLangPackJSON(langBytes)
}

func loadLanguagePack() map[string]string {
	if b, err := serverdata.ReadLangEnJSON(); err == nil && len(b) > 0 {
		if m := parseLangPackJSON(b); m != nil {
			return m
		}
	} else if err != nil {
		log.Println("[TCI Language] embedded/disk LangEn:", err)
	}

	if b, err := os.ReadFile(langEnCachePath()); err == nil {
		if m := parseLangPackJSON(b); m != nil {
			return m
		}
	}

	return downloadLanguagePack()
}

// GetTCIDisplayName resolves the display name the same way as GeneralsCamp getCIName (GGE LangEn ci_* keys).
func GetTCIDisplayName(name string) string {
	tciLangOnce.Do(func() {
		tciLangMap = loadLanguagePack()
	})

	if tciLangMap == nil {
		return name
	}

	rawName := strings.ToLower(name)
	prefixes := []string{"appearance", "primary", "secondary"}
	suffixes := []string{"", "_premium"}

	for _, p := range prefixes {
		for _, s := range suffixes {
			key := fmt.Sprintf("ci_%s_%s%s", p, rawName, s)
			if val, ok := tciLangMap[key]; ok {
				return val
			}
		}
	}

	for _, s := range suffixes {
		key := fmt.Sprintf("ci_%s%s", rawName, s)
		if val, ok := tciLangMap[key]; ok {
			return val
		}
	}

	if val, ok := tciLangMap[rawName]; ok {
		return val
	}

	return name
}

// GetTCIEffectName resolves translated effect labels (GeneralsCamp effect tooltips).
func GetTCIEffectName(effectName string) string {
	tciLangOnce.Do(func() {
		tciLangMap = loadLanguagePack()
	})

	if tciLangMap == nil {
		return effectName
	}

	lower := strings.ToLower(effectName)
	keys := []string{
		fmt.Sprintf("ci_effect_%s_tt", lower),
		fmt.Sprintf("effect_name_%s", lower),
		fmt.Sprintf("ci_effect_%s", lower),
		fmt.Sprintf("subscription_effect_description_%s", lower),
	}

	for _, k := range keys {
		if val, ok := tciLangMap[k]; ok {
			return val
		}
	}
	return effectName
}
