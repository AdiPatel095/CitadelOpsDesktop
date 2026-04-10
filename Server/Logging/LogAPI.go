package Logging

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// RegisterLogHandlers registers list + tail endpoints for dashboard log channels.
func RegisterLogHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/logs/channels", handleListChannels)
	mux.HandleFunc("GET /api/logs/{channel}/tail", handleChannelTail)
}

func handleListChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"channels": KnownChannels})
}

func handleChannelTail(w http.ResponseWriter, r *http.Request) {
	channelID := r.PathValue("channel")
	if channelID == "" {
		http.Error(w, "missing channel", http.StatusBadRequest)
		return
	}
	if !isKnownChannel(channelID) {
		http.NotFound(w, r)
		return
	}
	n := 500
	if q := r.URL.Query().Get("n"); q != "" {
		if v, err := strconv.Atoi(q); err == nil && v > 0 {
			n = v
			if n > 10000 {
				n = 10000
			}
		}
	}
	path := channelPath(channelID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"lines": []string{}})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	all := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	var lines []string
	for _, ln := range all {
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": lines})
}

func isKnownChannel(id string) bool {
	for _, c := range KnownChannels {
		if c.ID == id {
			return true
		}
	}
	return false
}
