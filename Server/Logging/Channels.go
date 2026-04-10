package Logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Known channel IDs (each maps to Logs/channels/<id>.log).
const (
	ChannelWebSocketGame = "websocket_game"
)

// ChannelMeta describes a dashboard log channel.
type ChannelMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// KnownChannels is the registry of channels the UI can show; add new entries as you add writers.
var KnownChannels = []ChannelMeta{
	{ID: ChannelWebSocketGame, Label: "Game WebSocket"},
}

var (
	channelsDir string
	channelMu   sync.Mutex
	channelFDs  = make(map[string]*os.File)
)

// InitChannelLogs creates Logs/channels and prepares per-channel appenders.
func InitChannelLogs() error {
	channelsDir = filepath.Join("Logs", "channels")
	return os.MkdirAll(channelsDir, 0755)
}

// ChannelsDir returns the resolved channels directory (after InitChannelLogs).
func ChannelsDir() string {
	return channelsDir
}

func channelPath(id string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return -1
	}, id)
	if safe == "" {
		safe = "unknown"
	}
	return filepath.Join(channelsDir, safe+".log")
}

func getChannelFile(id string) (*os.File, error) {
	channelMu.Lock()
	defer channelMu.Unlock()
	if f, ok := channelFDs[id]; ok {
		return f, nil
	}
	path := channelPath(id)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	channelFDs[id] = f
	return f, nil
}

// AppendChannelLine appends one line: timestamp [DIRECTION] [cmdType] payload
// direction should be SEND or RECV; cmdType is typically payload split by "%" index 2.
func AppendChannelLine(channelID, direction, cmdType, payload string) {
	f, err := getChannelFile(channelID)
	if err != nil {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05.000000")
	line := fmt.Sprintf("%s [%s] [%s] %s\n", ts, strings.ToUpper(direction), cmdType, payload)
	channelMu.Lock()
	_, _ = f.WriteString(line)
	channelMu.Unlock()
}

// CloseChannelLogs closes open channel file handles.
func CloseChannelLogs() {
	channelMu.Lock()
	defer channelMu.Unlock()
	for _, f := range channelFDs {
		_ = f.Close()
	}
	channelFDs = make(map[string]*os.File)
}
