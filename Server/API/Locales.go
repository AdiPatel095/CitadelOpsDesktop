package API

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
	"fmt"
	"net/http"
)

func (server *Server) handleLocales(writer http.ResponseWriter, _ *http.Request) {
	response := map[string]any{
		"schemaVersion": 1, "defaultLocale": "en", "locales": GameData.OfficialLocales(),
		"manifestVerifiedVersion": GameData.LocaleManifestVersion, "manifestSourceUrl": GameData.LocaleManifestSource,
		"sourceUrl":      GameData.DefaultLanguageMetadataURL,
		"fallbackPolicy": "requested locale cache, then English; missing keys use English",
	}
	if server.config.GameData != nil {
		if language, ready := server.config.GameData.Language(); ready {
			response["officialVersion"] = language.Metadata().Version
		}
	}
	writeJSON(writer, http.StatusOK, response)
}

func (server *Server) requestLanguage(writer http.ResponseWriter, request *http.Request) (*GameData.LanguageStore, GameData.LocaleResolution, bool) {
	code := request.URL.Query().Get("locale")
	if _, err := GameData.NormalizeLocale(code); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_locale", err.Error(), Localization.New("server.api.unsupported_locale", "Unsupported locale {locale}", Localization.Params{"locale": fmt.Sprintf("%q", code)}))
		return nil, GameData.LocaleResolution{}, false
	}
	if server.config.GameData == nil {
		writeError(writer, http.StatusServiceUnavailable, "game_data_unavailable", "Official game data is unavailable", Localization.New("server.api.official_game_data_is.c5e55e7e", "Official game data is unavailable", nil))
		return nil, GameData.LocaleResolution{}, false
	}
	// Preserve the original unparameterized runtime behavior and availability.
	if code == "" {
		if language, ready := server.config.GameData.Language(); ready {
			locale := language.Metadata().Language
			return language, GameData.LocaleResolution{RequestedLocale: locale, ResolvedLocale: locale, Source: "default"}, true
		}
		writeError(writer, http.StatusServiceUnavailable, "language_unavailable", "Official language data is unavailable", Localization.New("server.api.official_language_data_is.803160d4", "Official language data is unavailable", nil))
		return nil, GameData.LocaleResolution{}, false
	}
	language, resolution, err := server.config.GameData.LanguageFor(request.Context(), code)
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "language_unavailable", "Official language data is unavailable", Localization.New("server.api.official_language_data_is.803160d4", "Official language data is unavailable", nil))
		return nil, resolution, false
	}
	return language, resolution, true
}

func (server *Server) handleTranslations(writer http.ResponseWriter, request *http.Request) {
	language, resolution, ok := server.requestLanguage(writer, request)
	if !ok {
		return
	}
	writer.Header().Set("Cache-Control", "private, max-age=30")
	writeJSON(writer, http.StatusOK, map[string]any{
		"metadata": language.Metadata(), "values": language.Values(),
		"requestedLocale": resolution.RequestedLocale, "resolvedLocale": resolution.ResolvedLocale,
		"fallback": resolution.Fallback, "locale": resolution,
	})
}
