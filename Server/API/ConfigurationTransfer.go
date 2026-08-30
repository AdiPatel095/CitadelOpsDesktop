package API

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
)

const (
	settingsBundleFormat      = "citadelops-settings"
	settingsBundleVersion     = 1
	maxSettingsBundleBytes    = 32 << 20
	maxSettingsBundleSections = 4096
)

type settingsBundle struct {
	Format            string                      `json:"format"`
	FormatVersion     int                         `json:"formatVersion"`
	ExportedAt        time.Time                   `json:"exportedAt"`
	AppVersion        string                      `json:"appVersion,omitempty"`
	Configuration     settingsBundleConfiguration `json:"configuration"`
	ClientPreferences map[string]string           `json:"clientPreferences,omitempty"`
}

type settingsBundleConfiguration struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Sections      map[string]json.RawMessage `json:"sections"`
}

type settingsImportResult struct {
	ImportedSections          int       `json:"importedSections"`
	ChangedSections           int       `json:"changedSections"`
	IncludedClientPreferences int       `json:"includedClientPreferences"`
	Revision                  uint64    `json:"revision"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

func (server *Server) handleConfigurationExport(writer http.ResponseWriter, _ *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	snapshot := server.config.Configuration.Snapshot()
	// Research consent is intentionally device-local. A portable settings file
	// must never opt another installation into game actions or data upload.
	delete(snapshot.Sections, Reports.BattleResearchConfigurationSection)
	// Player-history retention is installation-specific: a desktop disk may be
	// unbounded while hosted storage has a server-enforced maximum.
	delete(snapshot.Sections, History.PlayerSamplesConfigurationSection)
	bundle := settingsBundle{
		Format:        settingsBundleFormat,
		FormatVersion: settingsBundleVersion,
		ExportedAt:    time.Now().UTC(),
		AppVersion:    server.config.Version,
		Configuration: settingsBundleConfiguration{
			SchemaVersion: snapshot.SchemaVersion,
			Sections:      snapshot.Sections,
		},
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="CitadelOps-Settings-%s.json"`,
		bundle.ExportedAt.Format("2006-01-02"),
	))
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(bundle)
}

func (server *Server) handleConfigurationImport(writer http.ResponseWriter, request *http.Request) {
	if server.config.BackgroundOnly || server.externalConfigurationAuthority.Load() {
		writeError(writer, http.StatusConflict, "configuration_control_plane_owned", "Hosted account settings must be imported through the account control plane")
		return
	}
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	bundle, err := decodeSettingsBundle(writer, request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_settings_bundle", err.Error())
		return
	}
	if bundle.Format != settingsBundleFormat || bundle.FormatVersion != settingsBundleVersion {
		writeError(
			writer,
			http.StatusUnprocessableEntity,
			"unsupported_settings_bundle",
			fmt.Sprintf("Expected %s format version %d", settingsBundleFormat, settingsBundleVersion),
		)
		return
	}
	if bundle.Configuration.SchemaVersion != Configuration.SchemaVersion {
		writeError(
			writer,
			http.StatusUnprocessableEntity,
			"unsupported_configuration_schema",
			fmt.Sprintf(
				"Settings use configuration schema %d; this app supports schema %d",
				bundle.Configuration.SchemaVersion,
				Configuration.SchemaVersion,
			),
		)
		return
	}
	// Ignore consent found in hand-edited or older bundles as well as omitting it
	// from new exports. Each installation must obtain its own explicit opt-in.
	delete(bundle.Configuration.Sections, Reports.BattleResearchConfigurationSection)
	// Ignore an installation-specific storage policy from older or hand-edited
	// bundles so an unlimited desktop choice cannot be carried onto a host.
	delete(bundle.Configuration.Sections, History.PlayerSamplesConfigurationSection)
	sectionCount := len(bundle.Configuration.Sections)
	if sectionCount == 0 {
		writeError(writer, http.StatusUnprocessableEntity, "empty_settings_bundle", "Settings bundle contains no configuration sections")
		return
	}
	if sectionCount > maxSettingsBundleSections {
		writeError(
			writer,
			http.StatusRequestEntityTooLarge,
			"too_many_settings_sections",
			fmt.Sprintf("Settings bundle may contain at most %d sections", maxSettingsBundleSections),
		)
		return
	}
	snapshot, changed, err := server.config.Configuration.UpdateMany(bundle.Configuration.Sections)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "settings_import_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, settingsImportResult{
		ImportedSections:          sectionCount,
		ChangedSections:           len(changed),
		IncludedClientPreferences: len(bundle.ClientPreferences),
		Revision:                  snapshot.Revision,
		UpdatedAt:                 snapshot.UpdatedAt,
	})
}

func decodeSettingsBundle(writer http.ResponseWriter, request *http.Request) (settingsBundle, error) {
	var bundle settingsBundle
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxSettingsBundleBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bundle); err != nil {
		return settingsBundle{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return settingsBundle{}, fmt.Errorf("settings bundle must contain one JSON document")
		}
		return settingsBundle{}, err
	}
	return bundle, nil
}
