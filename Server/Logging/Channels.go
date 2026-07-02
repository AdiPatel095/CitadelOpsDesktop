package Logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Known channel IDs.
const (
	ChannelWebSocketGame = "websocket_game"
	ChannelAutoBird      = "autobird"
	ChannelAutoRecruit   = "autorecruit"
	ChannelAutoTool      = "autotool"
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
	{ID: ChannelAutoRecruit, Label: "Auto Recruit"},
	{ID: ChannelAutoTool, Label: "Auto Tool"},
	{ID: ChannelAutoTCI, Label: "Auto TCI"},
	{ID: ChannelAutoBeriWorld, Label: "Auto Beri World"},
	{ID: ChannelRift, Label: "Rift"},
}

const webSocketGameLogRotation = 3 * time.Hour

type rotatingChannelSession struct {
	Path     string
	UnixSlot int64
}

var (
	channelsDir             string
	channelMu               sync.Mutex
	channelFDs              = make(map[string]*os.File)
	channelFDPaths          = make(map[string]string)
	webSocketGameLogSession *rotatingChannelSession
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

// BeginWebSocketGameLogSession starts a fresh websocket_game log file for a newly-created game websocket.
func BeginWebSocketGameLogSession() {
	channelMu.Lock()
	defer channelMu.Unlock()

	now := time.Now()
	if _, err := ensureWebSocketGameLogSessionLocked(now, true); err != nil {
		log.Printf("[logging] start websocket_game session: %v", err)
		return
	}
	if _, err := getChannelFileLocked(ChannelWebSocketGame, now); err != nil {
		log.Printf("[logging] open websocket_game session: %v", err)
	}
}

func safeChannelName(id string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return -1
	}, id)
	if safe == "" {
		safe = "unknown"
	}
	return safe
}

func flatChannelPath(id string) string {
	safe := safeChannelName(id)
	return filepath.Join(channelsDir, safe+".log")
}

func activeChannelPath(id string) string {
	channelMu.Lock()
	defer channelMu.Unlock()

	if id == ChannelWebSocketGame {
		if webSocketGameLogSession != nil && webSocketGameLogSession.Path != "" {
			return webSocketGameLogSession.Path
		}
		if latest := latestWebSocketGameLogPathLocked(); latest != "" {
			return latest
		}
	}
	return flatChannelPath(id)
}

func getChannelFileLocked(id string, now time.Time) (*os.File, error) {
	path, err := writableChannelPathLocked(id, now)
	if err != nil {
		return nil, err
	}
	if f, ok := channelFDs[id]; ok && channelFDPaths[id] == path {
		return f, nil
	}
	closeChannelFileLocked(id)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	channelFDs[id] = f
	channelFDPaths[id] = path
	return f, nil
}

func writableChannelPathLocked(id string, now time.Time) (string, error) {
	if id == ChannelWebSocketGame {
		return ensureWebSocketGameLogSessionLocked(now, false)
	}
	return flatChannelPath(id), nil
}

func ensureWebSocketGameLogSessionLocked(now time.Time, forceNew bool) (string, error) {
	slot := unixRotationSlot(now)
	if !forceNew && webSocketGameLogSession != nil && webSocketGameLogSession.UnixSlot == slot {
		return webSocketGameLogSession.Path, nil
	}

	oldPath := ""
	if webSocketGameLogSession != nil {
		oldPath = webSocketGameLogSession.Path
	}
	path, err := nextWebSocketGameLogPathLocked(now)
	if err != nil {
		return "", err
	}
	webSocketGameLogSession = &rotatingChannelSession{
		Path:     path,
		UnixSlot: slot,
	}
	if oldPath != "" && oldPath != path {
		closeChannelFileLocked(ChannelWebSocketGame)
	}
	return path, nil
}

func unixRotationSlot(now time.Time) int64 {
	return now.Unix() / int64(webSocketGameLogRotation/time.Second)
}

func webSocketGameLogDirLocked() string {
	return filepath.Join(channelsDir, safeChannelName(ChannelWebSocketGame))
}

func nextWebSocketGameLogPathLocked(now time.Time) (string, error) {
	dir := webSocketGameLogDirLocked()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	prefix := now.Format("2006-01-02") + "-"
	maxSuffix := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		rawSuffix := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".log")
		suffix, err := strconv.Atoi(rawSuffix)
		if err == nil && suffix > maxSuffix {
			maxSuffix = suffix
		}
	}
	return filepath.Join(dir, fmt.Sprintf("%s%d.log", prefix, maxSuffix+1)), nil
}

func latestWebSocketGameLogPathLocked() string {
	dir := webSocketGameLogDirLocked()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latestPath string
	var latestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latestPath == "" || info.ModTime().After(latestMod) {
			latestPath = filepath.Join(dir, entry.Name())
			latestMod = info.ModTime()
		}
	}
	return latestPath
}

func closeChannelFileLocked(id string) {
	if f, ok := channelFDs[id]; ok {
		_ = f.Close()
	}
	delete(channelFDs, id)
	delete(channelFDPaths, id)
}

// AppendChannelLine appends one line: timestamp [DIRECTION] [cmdType] payload
// direction should be SEND or RECV; cmdType is typically payload split by "%" index 2.
func AppendChannelLine(channelID, direction, cmdType, payload string) {
	now := time.Now()
	ts := now.Format("2006-01-02 15:04:05.000000")
	line := fmt.Sprintf("%s [%s] [%s] %s\n", ts, strings.ToUpper(direction), cmdType, payload)
	channelMu.Lock()
	defer channelMu.Unlock()

	f, err := getChannelFileLocked(channelID, now)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line)
}

// AppendAutoBirdLine records an AutoBird action (direction INFO, event as cmdType).
func AppendAutoBirdLine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelAutoBird, "INFO", event, detail)
}

// AppendAutoRecruitLine records an Auto Recruit action (direction INFO, event as cmdType).
func AppendAutoRecruitLine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelAutoRecruit, "INFO", event, detail)
}

// AppendAutoRecruitSendPayload records an outbound game wire frame on the Auto Recruit channel.
func AppendAutoRecruitSendPayload(payload string) {
	op := wireOpcodeFromPayload(payload)
	if op == "" {
		op = "UNKNOWN"
	}
	AppendChannelLine(ChannelAutoRecruit, "SEND", op, payload)
}

// AppendAutoToolLine records an Auto Tool action (direction INFO, event as cmdType).
func AppendAutoToolLine(event, detail string) {
	if event == "" {
		event = "event"
	}
	AppendChannelLine(ChannelAutoTool, "INFO", event, detail)
}

// AppendAutoToolSendPayload records an outbound game wire frame on the Auto Tool channel.
func AppendAutoToolSendPayload(payload string) {
	op := wireOpcodeFromPayload(payload)
	if op == "" {
		op = "UNKNOWN"
	}
	AppendChannelLine(ChannelAutoTool, "SEND", op, payload)
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

// AutoRecruitLog writes to the main log and the Auto Recruit dashboard channel.
func AutoRecruitLog(event, detail string) {
	if detail != "" {
		log.Printf("[AutoRecruit] %s: %s", event, detail)
	} else {
		log.Printf("[AutoRecruit] %s", event)
	}
	AppendAutoRecruitLine(event, detail)
}

// AutoRecruitLogf formats detail and calls [AutoRecruitLog].
func AutoRecruitLogf(event, format string, args ...any) {
	AutoRecruitLog(event, fmt.Sprintf(format, args...))
}

// AutoToolLog writes to the main log and the Auto Tool dashboard channel.
func AutoToolLog(event, detail string) {
	if detail != "" {
		log.Printf("[AutoTool] %s: %s", event, detail)
	} else {
		log.Printf("[AutoTool] %s", event)
	}
	AppendAutoToolLine(event, detail)
}

// AutoToolLogf formats detail and calls [AutoToolLog].
func AutoToolLogf(event, format string, args ...any) {
	AutoToolLog(event, fmt.Sprintf(format, args...))
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
	channelFDPaths = make(map[string]string)
	webSocketGameLogSession = nil
}
