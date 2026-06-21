package Logging

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	tailReadBlockSize = int64(64 * 1024)
	tailMaxReadBytes  = int64(16 * 1024 * 1024)
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
	path := activeChannelPath(channelID)
	lines, err := tailNonEmptyLines(path, n)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"lines": []string{}})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": lines})
}

func tailNonEmptyLines(path string, n int) ([]string, error) {
	if n <= 0 {
		return []string{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := stat.Size()
	if size == 0 {
		return []string{}, nil
	}

	readSize := int64(0)
	pos := size
	chunks := make([][]byte, 0, 4)
	newlines := 0
	for pos > 0 && readSize < tailMaxReadBytes && newlines <= n {
		blockSize := tailReadBlockSize
		remainingCap := tailMaxReadBytes - readSize
		if blockSize > remainingCap {
			blockSize = remainingCap
		}
		if blockSize > pos {
			blockSize = pos
		}
		pos -= blockSize

		block := make([]byte, blockSize)
		if _, err := f.ReadAt(block, pos); err != nil && err != io.EOF {
			return nil, err
		}
		newlines += bytes.Count(block, []byte{'\n'})
		chunks = append(chunks, block)
		readSize += blockSize
	}

	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	data := make([]byte, 0, total)
	for i := len(chunks) - 1; i >= 0; i-- {
		data = append(data, chunks[i]...)
	}

	all := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := make([]string, 0, min(n, len(all)))
	for _, ln := range all {
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func isKnownChannel(id string) bool {
	for _, c := range KnownChannels {
		if c.ID == id {
			return true
		}
	}
	return false
}
