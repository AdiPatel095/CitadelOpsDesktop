package main

import (
	"CitadelDesktop/Server/GameData"
	_ "embed"
	"encoding/json"
	"net/http"

	"golang.org/x/text/language"
)

const missingFrontendEnglish = "Build Client/ or run the Vite development server for the desktop UI."

// This independent bootstrap catalog remains available when no Client bundle exists.
//
//go:embed FrontendLocales.json
var bootstrapCatalogJSON []byte

type bootstrapTranslation struct {
	Text         string `json:"text"`
	SourceSHA256 string `json:"sourceSha256"`
	SHA256       string `json:"sha256"`
}
type bootstrapTranslations struct {
	SchemaVersion int `json:"schemaVersion"`
	Source        struct {
		Key    string `json:"key"`
		Text   string `json:"text"`
		SHA256 string `json:"sha256"`
	} `json:"source"`
	Authorship struct {
		Method              string `json:"method"`
		NativeSpeakerReview bool   `json:"nativeSpeakerReview"`
	} `json:"authorship"`
	Translations map[string]bootstrapTranslation `json:"translations"`
}

var bootstrapCatalog = func() bootstrapTranslations {
	var catalog bootstrapTranslations
	if err := json.Unmarshal(bootstrapCatalogJSON, &catalog); err != nil {
		panic(err)
	}
	return catalog
}()
var bootstrapLocales = GameData.OfficialLocales()
var bootstrapMatcher = func() language.Matcher {
	tags := make([]language.Tag, len(bootstrapLocales))
	for i, locale := range bootstrapLocales {
		tags[i] = language.MustParse(locale.Code)
		// The official service uses no for its Bokmål catalog. CLDR otherwise
		// treats the Norwegian macrolanguage as a weaker match than Danish.
		if locale.Code == "no" {
			tags[i] = language.MustParse("nb")
		}
	}
	return language.NewMatcher(tags)
}()

func bootstrapLocale(request *http.Request) string {
	// Explicit selection follows the API allowlist, including underscore aliases.
	// An unsupported explicit choice retains the default English response.
	if request.URL.Query().Has("locale") {
		locale, err := GameData.NormalizeLocale(request.URL.Query().Get("locale"))
		if err == nil {
			return locale.Code
		}
		return "en"
	}
	header := request.Header.Get("Accept-Language")
	// Bound untrusted negotiation input without changing the default response.
	if len(header) > 8192 {
		return "en"
	}
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil || len(tags) == 0 {
		return "en"
	}
	_, index, confidence := bootstrapMatcher.Match(tags...)
	if confidence == language.No {
		return "en"
	}
	return bootstrapLocales[index].Code
}
