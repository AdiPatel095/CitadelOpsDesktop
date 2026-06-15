package Logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Known channel IDs (each maps to Logs/channels/<id>.log).
const (
	ChannelWebSocketGame = "websocket_game"
	ChannelAutoBird      = "autobird"
	ChannelAutoTCI       = "autotci"
	ChannelAutoBeriWorld = "autoberiworld"
	ChannelRift          = "rift"
	// ChannelAppSend is Citadel-queued game commands (OutgoingMessages) plus inbound frames
	// that match those sends (FIFO by opcode).
	ChannelAppSend = "app_send"
)

// ChannelMeta describes a dashboard log channel.
type ChannelMeta struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// KnownChannels is the registry of channels the UI can show; add new entries as you add writers.
var KnownChannels = []ChannelMeta{
	{ID: ChannelWebSocketGame, Label: "Game WebSocket"},
	{ID: ChannelAppSend, Label: "Citadel sends"},
	{ID: ChannelAutoBird, Label: "AutoBird"},
	{ID: ChannelAutoTCI, Label: "Auto TCI"},
	{ID: ChannelAutoBeriWorld, Label: "Auto Beri World"},
	{ID: ChannelRift, Label: "Rift"},
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

// AppendAutoBirdLine records an AutoBird action (direction INFO, event as cmdType).
func AppendAutoBirdLine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelAutoBird, "INFO", event, detail)
}

// AppendAutoTCILine records an AutoTCI action (direction INFO, event as cmdType).
func AppendAutoTCILine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelAutoTCI, "INFO", event, detail)
}

// AppendAutoTCISendPayload records an outbound game wire frame on the Auto TCI channel.
func AppendAutoTCISendPayload(payload string) {
	op := wireOpcodeFromPayload(payload)
	if op == "" {
		op = "UNKNOWN"
	}
	AppendChannelLine(ChannelAutoTCI, "SEND", op, payload)
}

func wireOpcodeFromPayload(payload string) string {
	parts := strings.Split(payload, "%")
	if len(parts) <= 2 {
		return ""
	}
	cmd := parts[2]
	if strings.HasPrefix(cmd, "EmpireEx_") {
		if len(parts) > 3 {
			return parts[3]
		}
		return ""
	}
	return cmd
}

// AutoTCILog writes to the main log and the Auto TCI dashboard channel.
func AutoTCILog(event, detail string) {
	if detail != "" {
		log.Printf("[AutoTCI] %s: %s", event, detail)
	} else {
		log.Printf("[AutoTCI] %s", event)
	}
	AppendAutoTCILine(event, detail)
}

// AutoTCILogf formats detail and calls [AutoTCILog].
func AutoTCILogf(event, format string, args ...any) {
	AutoTCILog(event, fmt.Sprintf(format, args...))
}

// AppendAutoBeriWorldLine records an Auto Beri World action (direction INFO, event as cmdType).
func AppendAutoBeriWorldLine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelAutoBeriWorld, "INFO", event, detail)
}

// AppendAutoBeriWorldSendPayload records an outbound game wire frame on the Auto Beri World channel.
func AppendAutoBeriWorldSendPayload(payload string) {
	op := wireOpcodeFromPayload(payload)
	if op == "" {
		op = "UNKNOWN"
	}
	AppendChannelLine(ChannelAutoBeriWorld, "SEND", op, payload)
}

// AutoBeriWorldLog writes to the main log and the Auto Beri World dashboard channel.
func AutoBeriWorldLog(event, detail string) {
	if detail != "" {
		log.Printf("[AutoBeriWorld] %s: %s", event, detail)
	} else {
		log.Printf("[AutoBeriWorld] %s", event)
	}
	AppendAutoBeriWorldLine(event, detail)
}

// AutoBeriWorldLogf formats detail and calls [AutoBeriWorldLog].
func AutoBeriWorldLogf(event, format string, args ...any) {
	AutoBeriWorldLog(event, fmt.Sprintf(format, args...))
}

// AppendRiftLine records a Rift action (direction INFO, event as cmdType).
func AppendRiftLine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelRift, "INFO", event, detail)
}

// AppendRiftSendPayload records an outbound game wire frame on the Rift channel.
func AppendRiftSendPayload(payload string) {
	op := wireOpcodeFromPayload(payload)
	if op == "" {
		op = "UNKNOWN"
	}
	AppendChannelLine(ChannelRift, "SEND", op, payload)
}

// RiftLog writes to the main log and the Rift dashboard channel.
func RiftLog(event, detail string) {
	if detail != "" {
		log.Printf("[Rift] %s: %s", event, detail)
	} else {
		log.Printf("[Rift] %s", event)
	}
	AppendRiftLine(event, detail)
}

// RiftLogf formats detail and calls [RiftLog].
func RiftLogf(event, format string, args ...any) {
	RiftLog(event, fmt.Sprintf(format, args...))
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
