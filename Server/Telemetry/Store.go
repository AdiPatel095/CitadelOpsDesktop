package Telemetry

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Protocol"
)

const (
	ChannelWebSocketGame   = "websocket_game"
	ChannelAppSend         = "app_send"
	ChannelAutoBird        = "autobird"
	ChannelAutoStation     = "autostation"
	ChannelAutoRecruit     = "autorecruit"
	ChannelAutoTool        = "autotool"
	ChannelAutoSceatRes    = "autosceatres"
	ChannelAutoHospital    = "autohospital"
	ChannelAutoTCI         = "autotci"
	ChannelAutoBeriWorld   = "autoberiworld"
	ChannelAutoFoodBalance = "autofoodbalance"
	ChannelAutoEquipment   = "autoequipmentcleanup"
	ChannelAutoTowers      = "autotowers"
	ChannelAutoInvasion    = "autoinvasion"
	ChannelAutoNomad       = "autonomad"
	ChannelAutoAdvisor     = "autoadvisor"
	ChannelAutoKhan        = "autokhan"
	ChannelAutoStorm       = "autostorm"
	ChannelRift            = "rift"

	maxPendingAppCommands       = 1024
	webSocketGameRotation       = 3 * time.Hour
	tailReadBlockSize     int64 = 64 * 1024
	tailMaxReadBytes      int64 = 16 * 1024 * 1024
)

// Channel identifies one persistent logger view in the dashboard.
type Channel struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var knownChannels = []Channel{
	{ID: ChannelWebSocketGame, Label: "All Game Traffic", Description: "Every game WebSocket frame, including actions performed directly in the game."},
	{ID: ChannelAppSend, Label: "Citadel Commands", Description: "Commands sent by Citadel and the matching replies received from the game."},
	{ID: ChannelAutoBird, Label: "Auto Bird", Description: "Completed Auto Bird troop movements and problems requiring attention."},
	{ID: ChannelAutoStation, Label: "Auto Station", Description: "Completed station and recall movements and problems requiring attention."},
	{ID: ChannelAutoRecruit, Label: "Auto Recruit", Description: "Completed troop queues and problems requiring attention."},
	{ID: ChannelAutoTool, Label: "Auto Tool", Description: "Completed tool queues and problems requiring attention."},
	{ID: ChannelAutoSceatRes, Label: "Auto Sceat Resources", Description: "Completed crafting and resource actions and problems requiring attention."},
	{ID: ChannelAutoHospital, Label: "Auto Hospital", Description: "Completed hospital actions and problems requiring attention."},
	{ID: ChannelAutoTCI, Label: "Auto TCI", Description: "Completed construction-item equips, upgrades, and purchases and problems requiring attention."},
	{ID: ChannelAutoBeriWorld, Label: "Auto Berimond World", Description: "Completed Berimond troop transfers and problems requiring attention."},
	{ID: ChannelAutoFoodBalance, Label: "Auto Food Balance", Description: "Completed food and mead shipments and problems requiring attention."},
	{ID: ChannelAutoEquipment, Label: "Auto Equipment Cleanup", Description: "Completed equipment cleanup actions and problems requiring attention."},
	{ID: ChannelAutoTowers, Label: "Auto Towers", Description: "Launched tower attacks and problems requiring attention."},
	{ID: ChannelAutoInvasion, Label: "Auto Invasion", Description: "Launched Foreign Lords and Bloodcrow attacks and problems requiring attention."},
	{ID: ChannelAutoNomad, Label: "Auto Nomad / Samurai", Description: "Launched Nomad and Samurai attacks and other completed event actions."},
	{ID: ChannelAutoAdvisor, Label: "Auto Advisor", Description: "Launched advisor attacks and other completed advisor actions."},
	{ID: ChannelAutoKhan, Label: "Auto Khan", Description: "Completed Khan attacks, defense, and protection actions."},
	{ID: ChannelAutoStorm, Label: "Auto Storm", Description: "Completed Storm attacks, construction, logistics, and shop purchases."},
	{ID: ChannelRift, Label: "Rift", Description: "Launched Rift attacks and other completed Rift actions."},
}

var featureActivityEvents = map[string]struct{}{
	"ACTION": {}, "ALLIANCE HELP": {}, "ATTACK": {}, "BUILDING": {}, "CONSTRUCTION": {},
	"CRAFTING": {}, "DEFENSE": {}, "EQUIPMENT": {}, "ESPIONAGE": {}, "EVENT": {},
	"HOSPITAL": {}, "PURCHASE": {}, "QUEUE": {}, "TIME SKIP": {}, "TRANSPORT": {},
}

type pendingAppCommand struct {
	responseOpcodes []string
}

type webSocketGameSession struct {
	path string
	slot int64
}

type persistenceEntry struct {
	channel    string
	line       string
	observedAt time.Time
	flushed    chan struct{}
}

// Store keeps a short live tail for each channel and persists the complete logger stream.
// Persistence is optional so replay and unit-test callers can use the in-memory store alone.
type Store struct {
	mu       sync.RWMutex
	capacity int
	lines    map[string][]string

	pendingAppCommand []pendingAppCommand

	fileMu           sync.Mutex
	channelsDir      string
	files            map[string]*os.File
	filePaths        map[string]string
	webSocketSession *webSocketGameSession

	persistMu     sync.Mutex
	persistQueue  []persistenceEntry
	persistHead   int
	persistWake   chan struct{}
	persistDone   chan struct{}
	persistClosed bool
}

func NewStore(capacity int) *Store {
	if capacity < 100 {
		capacity = 100
	}
	store := &Store{
		capacity:    capacity,
		lines:       map[string][]string{},
		files:       map[string]*os.File{},
		filePaths:   map[string]string{},
		persistWake: make(chan struct{}, 1),
		persistDone: make(chan struct{}),
	}
	go store.runPersistence()
	return store
}

// SetDataDir enables persistent channel logs below DataDir/Logs/channels.
func (store *Store) SetDataDir(dataDir string) error {
	if store == nil {
		return nil
	}
	directory := filepath.Join(strings.TrimSpace(dataDir), "Logs", "channels")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	store.flushPersistence()
	store.fileMu.Lock()
	store.closeFilesLocked()
	store.channelsDir = directory
	store.fileMu.Unlock()
	return nil
}

// BeginWebSocketGameSession starts a fresh rotating capture when a game session is started.
func (store *Store) BeginWebSocketGameSession() {
	if store == nil {
		return
	}
	store.flushPersistence()
	store.fileMu.Lock()
	defer store.fileMu.Unlock()
	_, _ = store.ensureWebSocketGameSessionLocked(time.Now(), true)
}

// Record receives every decoded game frame from the ingest pipeline. The websocket channel
// deliberately stores the complete original wire payload, including login and long responses.
func (store *Store) Record(frame Protocol.CommittedFrame, reduceErr error) {
	if store == nil {
		return
	}
	direction := logDirection(frame.Frame.Direction)
	if frame.Frame.Direction == Protocol.DirectionInbound && frame.Frame.ResponseCode != nil && *frame.Frame.ResponseCode != 0 {
		direction = "ERROR"
	}
	payload := strings.TrimSpace(frame.Frame.Raw)
	if payload == "" {
		payload = strings.TrimSpace(string(frame.Frame.Payload))
		if payload == "" {
			payload = strings.TrimSpace(frame.Frame.PayloadText)
		}
	}
	store.append(ChannelWebSocketGame, formatLine(frame.Frame.ReceivedAt, direction, frame.Frame.Opcode, payload))
	if frame.Frame.Direction == Protocol.DirectionInbound {
		store.recordMatchingAppResponse(frame.Frame, payload)
	}
	if reduceErr != nil {
		store.append(ChannelWebSocketGame, formatLine(frame.Frame.ReceivedAt, "ERROR", frame.Frame.Opcode, reduceErr.Error()))
	}
}

// RecordRaw preserves an invalid or incomplete websocket frame which could not reach a reducer.
func (store *Store) RecordRaw(raw string, direction Protocol.Direction, observedAt time.Time, decodeErr error) {
	if store == nil {
		return
	}
	detail := strings.TrimSpace(raw)
	if decodeErr != nil {
		if detail != "" {
			detail += " "
		}
		detail += "decode error=" + decodeErr.Error()
	}
	store.append(ChannelWebSocketGame, formatLine(observedAt, logDirection(direction), "UNKNOWN", detail))
}

// RecordAppOutbound records a command dispatched through Citadel, keeping manual game traffic
// out of the app-only channel. The next matching inbound opcode is added as a MATCH line.
func (store *Store) RecordAppOutbound(payload string, _ string) {
	if store == nil {
		return
	}
	now := time.Now()
	frame, err := Protocol.Decode(payload, Protocol.DirectionOutbound, now)
	opcode := "UNKNOWN"
	if err == nil {
		opcode = frame.Opcode
	}
	line := formatLine(now, "SEND", opcode, strings.TrimSpace(payload))
	store.mu.Lock()
	store.pendingAppCommand = append(store.pendingAppCommand, pendingAppCommand{
		responseOpcodes: appResponseOpcodes(opcode),
	})
	if len(store.pendingAppCommand) > maxPendingAppCommands {
		store.pendingAppCommand = store.pendingAppCommand[len(store.pendingAppCommand)-maxPendingAppCommands:]
	}
	store.appendLocked(ChannelAppSend, line, now)
	store.mu.Unlock()
}

// RecordFeature preserves the legacy feature logger API. Non-activity legacy entries
// remain on disk but are omitted from feature tails.
func (store *Store) RecordFeature(channel string, event string, detail string) {
	if store == nil || !isKnownChannel(channel) {
		return
	}
	store.append(channel, formatLine(time.Now(), "INFO", event, detail))
}

// RecordFeatureActivity adds one user-facing completed action or issue to an automation or Rift channel.
func (store *Store) RecordFeatureActivity(actor string, intent string, severity string, event string, detail string) {
	channel := featureChannelForActor(actor)
	if channel == "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(intent)), "rift.") {
		channel = ChannelRift
	}
	if channel == "" {
		return
	}
	severity = strings.ToUpper(strings.TrimSpace(severity))
	if severity != "WARN" && severity != "ERROR" {
		severity = "INFO"
	}
	event = strings.ToUpper(strings.TrimSpace(event))
	if _, exists := featureActivityEvents[event]; !exists {
		event = "ACTION"
	}
	store.append(channel, formatLine(time.Now(), severity, event, strings.TrimSpace(detail)))
}

func (store *Store) Channels() []Channel {
	return append([]Channel(nil), knownChannels...)
}

func (store *Store) Tail(channel string, limit int) []string {
	if store == nil || !isKnownChannel(channel) {
		return []string{}
	}
	if limit < 1 {
		limit = 100
	}
	if limit > store.capacity {
		limit = store.capacity
	}

	store.mu.RLock()
	lines := tailLines(store.lines[channel], limit)
	if isFeatureChannel(channel) {
		lines = tailFeatureActivityLines(store.lines[channel], limit)
	}
	store.mu.RUnlock()
	store.flushPersistence()
	store.fileMu.Lock()
	path := store.activeChannelPathLocked(channel)
	store.fileMu.Unlock()
	if path == "" {
		return lines
	}
	readLimit := limit
	if isFeatureChannel(channel) {
		readLimit = min(limit*16, 100_000)
	}
	persisted, err := tailNonEmptyLines(path, readLimit)
	if err == nil {
		if isFeatureChannel(channel) {
			return tailFeatureActivityLines(persisted, limit)
		}
		return persisted
	}
	return lines
}

func (store *Store) Close() {
	if store == nil {
		return
	}
	store.persistMu.Lock()
	if !store.persistClosed {
		store.persistClosed = true
	}
	store.persistMu.Unlock()
	store.wakePersistence()
	<-store.persistDone
	store.fileMu.Lock()
	store.closeFilesLocked()
	store.fileMu.Unlock()
}

func (store *Store) recordMatchingAppResponse(frame Protocol.Frame, payload string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	match := -1
	for index, pending := range store.pendingAppCommand {
		if containsOpcode(pending.responseOpcodes, frame.Opcode) {
			match = index
			break
		}
	}
	if match < 0 {
		return
	}
	copy(store.pendingAppCommand[match:], store.pendingAppCommand[match+1:])
	store.pendingAppCommand = store.pendingAppCommand[:len(store.pendingAppCommand)-1]
	line := formatLine(frame.ReceivedAt, "MATCH", frame.Opcode, payload)
	store.appendLocked(ChannelAppSend, line, frame.ReceivedAt)
}

func appResponseOpcodes(opcode string) []string {
	switch strings.ToLower(strings.TrimSpace(opcode)) {
	case "jca":
		return []string{"jaa"}
	case "ahr":
		return []string{"ahh", "ahr"}
	default:
		return []string{strings.ToLower(strings.TrimSpace(opcode))}
	}
}

func containsOpcode(values []string, opcode string) bool {
	opcode = strings.ToLower(strings.TrimSpace(opcode))
	for _, value := range values {
		if value == opcode {
			return true
		}
	}
	return false
}

func (store *Store) append(channel string, line string) {
	store.mu.Lock()
	store.appendLocked(channel, line, time.Now())
	store.mu.Unlock()
}

func (store *Store) appendLocked(channel string, line string, observedAt time.Time) {
	lines := append(store.lines[channel], line)
	if len(lines) > store.capacity {
		copy(lines, lines[len(lines)-store.capacity:])
		lines = lines[:store.capacity]
	}
	store.lines[channel] = lines
	store.persistMu.Lock()
	if !store.persistClosed {
		store.persistQueue = append(store.persistQueue, persistenceEntry{
			channel: channel, line: line, observedAt: observedAt,
		})
	}
	store.persistMu.Unlock()
	store.wakePersistence()
}

func (store *Store) runPersistence() {
	defer close(store.persistDone)
	for {
		entry, ok := store.nextPersistence()
		if !ok {
			return
		}
		if entry.flushed != nil {
			close(entry.flushed)
			continue
		}
		store.fileMu.Lock()
		if store.channelsDir != "" {
			file, err := store.channelFileLocked(entry.channel, entry.observedAt)
			if err == nil {
				_, _ = file.WriteString(entry.line + "\n")
			}
		}
		store.fileMu.Unlock()
	}
}

func (store *Store) nextPersistence() (persistenceEntry, bool) {
	for {
		store.persistMu.Lock()
		if store.persistHead < len(store.persistQueue) {
			entry := store.persistQueue[store.persistHead]
			store.persistQueue[store.persistHead] = persistenceEntry{}
			store.persistHead++
			if store.persistHead == len(store.persistQueue) {
				store.persistQueue = nil
				store.persistHead = 0
			}
			store.persistMu.Unlock()
			return entry, true
		}
		closed := store.persistClosed
		store.persistMu.Unlock()
		if closed {
			return persistenceEntry{}, false
		}
		<-store.persistWake
	}
}

func (store *Store) wakePersistence() {
	select {
	case store.persistWake <- struct{}{}:
	default:
	}
}

func (store *Store) flushPersistence() {
	flushed := make(chan struct{})
	store.persistMu.Lock()
	if store.persistClosed {
		store.persistMu.Unlock()
		<-store.persistDone
		return
	}
	store.persistQueue = append(store.persistQueue, persistenceEntry{flushed: flushed})
	store.persistMu.Unlock()
	store.wakePersistence()
	<-flushed
}

func (store *Store) channelFileLocked(channel string, now time.Time) (*os.File, error) {
	path, err := store.writableChannelPathLocked(channel, now)
	if err != nil {
		return nil, err
	}
	if file, exists := store.files[channel]; exists && store.filePaths[channel] == path {
		return file, nil
	}
	store.closeFileLocked(channel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	store.files[channel] = file
	store.filePaths[channel] = path
	return file, nil
}

func (store *Store) writableChannelPathLocked(channel string, now time.Time) (string, error) {
	if channel != ChannelWebSocketGame {
		return filepath.Join(store.channelsDir, channel+".log"), nil
	}
	return store.ensureWebSocketGameSessionLocked(now, false)
}

func (store *Store) activeChannelPathLocked(channel string) string {
	if store.channelsDir == "" {
		return ""
	}
	if channel != ChannelWebSocketGame {
		return filepath.Join(store.channelsDir, channel+".log")
	}
	if store.webSocketSession != nil {
		return store.webSocketSession.path
	}
	return latestWebSocketGamePath(filepath.Join(store.channelsDir, ChannelWebSocketGame))
}

func (store *Store) ensureWebSocketGameSessionLocked(now time.Time, forceNew bool) (string, error) {
	slot := now.Unix() / int64(webSocketGameRotation/time.Second)
	if !forceNew && store.webSocketSession != nil && store.webSocketSession.slot == slot {
		return store.webSocketSession.path, nil
	}
	directory := filepath.Join(store.channelsDir, ChannelWebSocketGame)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path, err := nextWebSocketGamePath(directory, now)
	if err != nil {
		return "", err
	}
	if store.webSocketSession == nil || store.webSocketSession.path != path {
		store.closeFileLocked(ChannelWebSocketGame)
	}
	store.webSocketSession = &webSocketGameSession{path: path, slot: slot}
	return path, nil
}

func (store *Store) closeFileLocked(channel string) {
	if file := store.files[channel]; file != nil {
		_ = file.Close()
	}
	delete(store.files, channel)
	delete(store.filePaths, channel)
}

func (store *Store) closeFilesLocked() {
	for channel := range store.files {
		store.closeFileLocked(channel)
	}
	store.webSocketSession = nil
}

func nextWebSocketGamePath(directory string, now time.Time) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", err
	}
	prefix := now.Format("2006-01-02") + "-"
	maxSuffix := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		suffix, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(entry.Name(), prefix), ".log"))
		if err == nil && suffix > maxSuffix {
			maxSuffix = suffix
		}
	}
	return filepath.Join(directory, fmt.Sprintf("%s%d.log", prefix, maxSuffix+1)), nil
}

func latestWebSocketGamePath(directory string) string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ""
	}
	var latest string
	var modifiedAt time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latest == "" || info.ModTime().After(modifiedAt) {
			latest = filepath.Join(directory, entry.Name())
			modifiedAt = info.ModTime()
		}
	}
	return latest
}

func tailNonEmptyLines(path string, limit int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() == 0 {
		return []string{}, err
	}

	position := info.Size()
	readSize := int64(0)
	newlines := 0
	chunks := make([][]byte, 0, 4)
	for position > 0 && readSize < tailMaxReadBytes && newlines <= limit {
		blockSize := min(tailReadBlockSize, tailMaxReadBytes-readSize, position)
		position -= blockSize
		block := make([]byte, blockSize)
		if _, err := file.ReadAt(block, position); err != nil && err != io.EOF {
			return nil, err
		}
		newlines += bytes.Count(block, []byte{'\n'})
		chunks = append(chunks, block)
		readSize += blockSize
	}
	data := make([]byte, 0, readSize)
	for index := len(chunks) - 1; index >= 0; index-- {
		data = append(data, chunks[index]...)
	}
	return tailLines(strings.FieldsFunc(strings.ReplaceAll(string(data), "\r\n", "\n"), func(character rune) bool {
		return character == '\n'
	}), limit), nil
}

func tailLines(lines []string, limit int) []string {
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return append([]string(nil), lines...)
}

func tailFeatureActivityLines(lines []string, limit int) []string {
	filtered := make([]string, 0, min(len(lines), limit))
	for _, line := range lines {
		if isFeatureActivityLine(line) {
			filtered = append(filtered, line)
		}
	}
	return tailLines(filtered, limit)
}

func isFeatureActivityLine(line string) bool {
	firstOpen := strings.IndexByte(line, '[')
	if firstOpen < 0 {
		return false
	}
	firstClose := strings.IndexByte(line[firstOpen+1:], ']')
	if firstClose < 0 {
		return false
	}
	firstClose += firstOpen + 1
	severity := strings.TrimSpace(line[firstOpen+1 : firstClose])
	if severity != "INFO" && severity != "WARN" && severity != "ERROR" {
		return false
	}
	remainder := strings.TrimSpace(line[firstClose+1:])
	if !strings.HasPrefix(remainder, "[") {
		return false
	}
	secondClose := strings.IndexByte(remainder[1:], ']')
	if secondClose < 0 {
		return false
	}
	event := strings.TrimSpace(remainder[1 : secondClose+1])
	_, exists := featureActivityEvents[event]
	return exists
}

func logDirection(direction Protocol.Direction) string {
	switch direction {
	case Protocol.DirectionInbound:
		return "RECV"
	case Protocol.DirectionOutbound:
		return "SEND"
	default:
		return strings.ToUpper(string(direction))
	}
}

func formatLine(observedAt time.Time, direction string, opcode string, payload string) string {
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if strings.TrimSpace(opcode) == "" {
		opcode = "UNKNOWN"
	}
	return fmt.Sprintf("%s [%s] [%s] %s", observedAt.Local().Format("2006-01-02 15:04:05.000000"), strings.ToUpper(direction), opcode, payload)
}

func featureChannelForActor(actor string) string {
	actor = strings.ToLower(strings.TrimSpace(actor))
	if strings.HasPrefix(actor, "automation:") {
		actor = strings.TrimPrefix(actor, "automation:")
	}
	switch actor {
	case "autobird":
		return ChannelAutoBird
	case "autostation":
		return ChannelAutoStation
	case "autorecruit":
		return ChannelAutoRecruit
	case "autotool":
		return ChannelAutoTool
	case "autosceatres":
		return ChannelAutoSceatRes
	case "autohospital":
		return ChannelAutoHospital
	case "autotci":
		return ChannelAutoTCI
	case "autoberiworld":
		return ChannelAutoBeriWorld
	case "autofoodbalance":
		return ChannelAutoFoodBalance
	case "autoequipmentcleanup", "auto-equipment-cleanup", "ui:auto-equipment-cleanup":
		return ChannelAutoEquipment
	case "autotowers":
		return ChannelAutoTowers
	case "autoinvasion":
		return ChannelAutoInvasion
	case "autonomad":
		return ChannelAutoNomad
	case "autoadvisor":
		return ChannelAutoAdvisor
	case "autokhan":
		return ChannelAutoKhan
	case "autostorm":
		return ChannelAutoStorm
	case "scheduler:rift", "rift":
		return ChannelRift
	default:
		return ""
	}
}

func isKnownChannel(channel string) bool {
	for _, known := range knownChannels {
		if known.ID == channel {
			return true
		}
	}
	return false
}

func isFeatureChannel(channel string) bool {
	return isKnownChannel(channel) && channel != ChannelWebSocketGame && channel != ChannelAppSend
}
