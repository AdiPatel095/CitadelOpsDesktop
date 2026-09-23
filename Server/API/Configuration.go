package API

import (
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Reports"
)

const maximumConfigurationUpdateBytes = (1 << 20) + (16 << 10)

type configurationUpdateRequest struct {
	Value            json.RawMessage  `json:"value"`
	ExpectedRevision *uint64          `json:"expectedRevision,omitempty"`
	ExpectedValue    *json.RawMessage `json:"expectedValue,omitempty"`
}

// handleConfigurationUpdate writes directly to the durable configuration
// store. It intentionally has no intent-engine or game-session dependency:
// tenant settings remain editable while the game socket is stopped, cooling
// down, reconnecting, or otherwise unavailable.
func (server *Server) handleConfigurationUpdate(writer http.ResponseWriter, request *http.Request) {
	if server.config.Configuration == nil {
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable", Localization.New("server.api.configuration_store_is_unavailable.623f75fa", "Configuration store is unavailable", nil))
		return
	}
	var input configurationUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumConfigurationUpdateBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeErrorFromError(writer, http.StatusBadRequest, "invalid_request", err)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = Localization.WithError(fmt.Errorf("configuration update must contain exactly one JSON object"), Localization.New("server.api.configuration_update_must_contain.b7eb8212", "configuration update must contain exactly one JSON object", nil))
		}
		writeErrorFromError(writer, http.StatusBadRequest, "invalid_request", err)
		return
	}
	section := request.PathValue("section")
	if section == Reports.BattleResearchConfigurationSection {
		writeError(
			writer,
			http.StatusGone,
			"configuration_section_retired",
			"Experimental Battle Research settings have been removed", Localization.New("server.api.experimental_battle_research_settings.878d9340", "Experimental Battle Research settings have been removed", nil),
		)
		return
	}
	if section == History.PlayerSamplesConfigurationSection {
		writeError(
			writer,
			http.StatusConflict,
			"configuration_requires_retention_apply",
			"My Stats retention must be updated through its durable retention endpoint", Localization.New("server.api.my_stats_retention_must.d59c4907", "My Stats retention must be updated through its durable retention endpoint", nil),
		)
		return
	}
	if server.config.BackgroundOnly || server.externalConfigurationAuthority.Load() {
		writeError(writer, http.StatusConflict, "configuration_control_plane_owned", "Hosted account settings must be saved through the account control plane", Localization.New("server.api.hosted_account_settings_must.248f745f", "Hosted account settings must be saved through the account control plane", nil))
		return
	}
	if err := Configuration.Validate(section, input.Value); err != nil {
		writeErrorFromError(writer, http.StatusUnprocessableEntity, "configuration_invalid", err)
		return
	}
	if input.ExpectedValue != nil {
		if err := Configuration.Validate(section, *input.ExpectedValue); err != nil {
			writeError(writer, http.StatusUnprocessableEntity, "configuration_invalid", fmt.Sprintf("expected value: %v", err), Localization.New("server.api.expected_value_p.bfcae137", "expected value: {p0}", Localization.Params{"p0": fmt.Sprintf("%v", err)}))
			return
		}
	}
	snapshot, err := server.config.Configuration.UpdateConditional(
		section,
		input.Value,
		input.ExpectedRevision,
		input.ExpectedValue,
	)
	if err != nil {
		if errors.Is(err, Configuration.ErrInvalidUpdate) {
			writeErrorFromError(writer, http.StatusUnprocessableEntity, "configuration_invalid", err)
			return
		}
		if errors.Is(err, Configuration.ErrExternalAuthority) {
			writeError(writer, http.StatusConflict, "configuration_control_plane_owned", "Hosted account settings must be saved through the account control plane", Localization.New("server.api.hosted_account_settings_must.248f745f", "Hosted account settings must be saved through the account control plane", nil))
			return
		}
		if strings.Contains(err.Error(), "configuration revision changed") ||
			strings.Contains(err.Error(), "configuration section") && strings.HasSuffix(err.Error(), "changed") {
			writeErrorFromError(writer, http.StatusConflict, "configuration_conflict", err)
			return
		}
		writeErrorFromError(writer, http.StatusInternalServerError, "configuration_update_failed", err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, snapshot)
}
