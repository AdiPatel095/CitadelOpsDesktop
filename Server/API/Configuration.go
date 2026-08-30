package API

import (
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
		writeError(writer, http.StatusServiceUnavailable, "configuration_unavailable", "Configuration store is unavailable")
		return
	}
	var input configurationUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumConfigurationUpdateBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("configuration update must contain exactly one JSON object")
		}
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	section := request.PathValue("section")
	if section == History.PlayerSamplesConfigurationSection {
		writeError(
			writer,
			http.StatusConflict,
			"configuration_requires_retention_apply",
			"My Stats retention must be updated through its durable retention endpoint",
		)
		return
	}
	if (server.config.BackgroundOnly || server.externalConfigurationAuthority.Load()) &&
		section != Reports.BattleResearchConfigurationSection {
		writeError(writer, http.StatusConflict, "configuration_control_plane_owned", "Hosted account settings must be saved through the account control plane")
		return
	}
	if err := Configuration.Validate(section, input.Value); err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "configuration_invalid", err.Error())
		return
	}
	if input.ExpectedValue != nil {
		if err := Configuration.Validate(section, *input.ExpectedValue); err != nil {
			writeError(writer, http.StatusUnprocessableEntity, "configuration_invalid", fmt.Sprintf("expected value: %v", err))
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
		if errors.Is(err, Configuration.ErrExternalAuthority) {
			writeError(writer, http.StatusConflict, "configuration_control_plane_owned", "Hosted account settings must be saved through the account control plane")
			return
		}
		if strings.Contains(err.Error(), "configuration revision changed") ||
			strings.Contains(err.Error(), "configuration section") && strings.HasSuffix(err.Error(), "changed") {
			writeError(writer, http.StatusConflict, "configuration_conflict", err.Error())
			return
		}
		writeError(writer, http.StatusInternalServerError, "configuration_update_failed", err.Error())
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, snapshot)
}
