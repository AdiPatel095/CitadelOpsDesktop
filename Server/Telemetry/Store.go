package Telemetry

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	maxPendingAppCommands            = 1024
	channelLogRotation               = 3 * time.Hour
	persistenceBatchWindow           = 5 * time.Millisecond
	persistenceWriteBufferSize       = 256 * 1024
	diagnosticLiveTailMaxBytes       = 8 * 1024 * 1024
	tailReadBlockSize          int64 = 64 * 1024
	tailMaxReadBytes           int64 = 16 * 1024 * 1024
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
	{ID: ChannelAutoBeriWorld, Label: "Auto Berimond World", Description: "Completed Berimond troop transfers, tower attacks, and problems requiring attention."},
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

var attackFeatureChannels = []string{
	ChannelAutoTowers,
	ChannelAutoInvasion,
	ChannelAutoNomad,
	ChannelAutoAdvisor,
	ChannelAutoKhan,
	ChannelAutoBeriWorld,
	ChannelAutoStorm,
}

type pendingAppCommand struct {
	responseOpcodes []string
}

type channelLogSession struct {
	path string
	slot int64
}

type persistenceEntry struct {
	channel    string
	line       string
	observedAt time.Time
	flushed    chan struct{}
}

// memoryTail is an append-only window with an amortized O(1) eviction path.
// Cleared entries release large raw websocket payloads before the backing slice
// is compacted.
type memoryTail struct {
	lines []string
	head  int
	bytes int
}

func (tail *memoryTail) append(line string, maxLines int, maxBytes int) {
	tail.lines = append(tail.lines, line)
	tail.bytes += len(line)
	for tail.head < len(tail.lines) &&
		(len(tail.lines)-tail.head > maxLines || (maxBytes > 0 && tail.bytes > maxBytes)) {
		tail.bytes -= len(tail.lines[tail.head])
		tail.lines[tail.head] = ""
		tail.head++
	}
	if tail.head >= 1024 && tail.head*2 >= len(tail.lines) {
		active := copy(tail.lines, tail.lines[tail.head:])
		clear(tail.lines[active:])
		tail.lines = tail.lines[:active]
		tail.head = 0
	}
}

func (tail *memoryTail) activeLines() []string {
	if tail == nil || tail.head >= len(tail.lines) {
		return nil
	}
	return tail.lines[tail.head:]
}

// Store keeps a short live tail for each channel and persists the complete logger stream.
// Persistence is optional so replay and unit-test callers can use the in-memory store alone.
type Store struct {
	mu       sync.RWMutex
	capacity int
	tails    map[string]*memoryTail

	persistenceEnabled atomic.Bool

	pendingAppCommand []pendingAppCommand

	fileMu          sync.Mutex
	channelsDir     string
	files           map[string]*os.File
	buffers         map[string]*bufio.Writer
	filePaths       map[string]string
	channelSessions map[string]*channelLogSession

	persistMu     sync.Mutex
	persistQueue  []persistenceEntry
	persistWake   chan struct{}
	persistDone   chan struct{}
	persistClosed bool

	attackMu       sync.Mutex
	attackLaunches map[string][]time.Time

	retentionLifecycleMu sync.Mutex
	retentionWake        chan struct{}
	retentionStop        chan struct{}
	retentionDone        chan struct{}
	retentionStarted     bool
	retentionStopped     bool
	retentionFileMu      sync.RWMutex
}

func NewStore(capacity int) *Store {
	if capacity < 100 {
		capacity = 100
	}
	store := &Store{
		capacity:        capacity,
		tails:           map[string]*memoryTail{},
		files:           map[string]*os.File{},
		buffers:         map[string]*bufio.Writer{},
		filePaths:       map[string]string{},
		channelSessions: map[string]*channelLogSession{},
		persistWake:     make(chan struct{}, 1),
		persistDone:     make(chan struct{}),
		attackLaunches:  map[string][]time.Time{},
		retentionWake:   make(chan struct{}, 1),
		retentionStop:   make(chan struct{}),
		retentionDone:   make(chan struct{}),
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
	loadedAt := time.Now()
	loadedAttackLaunches := loadAttackLaunches(directory, loadedAt.Add(-time.Hour))
	store.retentionFileMu.Lock()
	store.flushPersistence()
	store.fileMu.Lock()
	store.closeFilesLocked()
	store.channelsDir = directory
	store.persistenceEnabled.Store(true)
	store.fileMu.Unlock()
	store.retentionFileMu.Unlock()
	store.attackMu.Lock()
	for channel, launches := range store.attackLaunches {
		for _, launchedAt := range launches {
			if !launchedAt.Before(loadedAt) {
				loadedAttackLaunches[channel] = append(loadedAttackLaunches[channel], launchedAt)
			}
		}
	}
	store.attackLaunches = loadedAttackLaunches
	store.attackMu.Unlock()
	store.startRetention()
	store.wakeRetention()
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
	_, _ = store.ensureChannelSessionLocked(ChannelWebSocketGame, time.Now(), true)
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
	observedAt := time.Now()
	store.append(channel, formatLine(observedAt, severity, event, strings.TrimSpace(detail)))
	if severity == "INFO" && event == "ATTACK" {
		store.recordAttackLaunch(channel, observedAt)
	}
}

func (store *Store) Channels() []Channel {
	return append([]Channel(nil), knownChannels...)
}

// AttackLaunchCounts returns the confirmed launches recorded during the rolling hour ending at now.
func (store *Store) AttackLaunchCounts(now time.Time) map[string]int {
	counts := make(map[string]int, len(attackFeatureChannels))
	if store == nil {
		return counts
	}
	if now.IsZero() {
		now = time.Now()
	}
	since := now.Add(-time.Hour)
	store.attackMu.Lock()
	defer store.attackMu.Unlock()
	for _, channel := range attackFeatureChannels {
		launches := store.attackLaunches[channel]
		firstIncluded := 0
		for firstIncluded < len(launches) && launches[firstIncluded].Before(since) {
			firstIncluded++
		}
		if firstIncluded > 0 {
			launches = append([]time.Time(nil), launches[firstIncluded:]...)
			store.attackLaunches[channel] = launches
		}
		counts[channel] = len(launches)
	}
	return counts
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
	memoryLines := store.tails[channel].activeLines()
	lines := tailLines(memoryLines, limit)
	if isFeatureChannel(channel) {
		lines = tailFeatureActivityLines(memoryLines, limit)
	}
	store.mu.RUnlock()
	store.flushPersistence()
	store.retentionFileMu.RLock()
	defer store.retentionFileMu.RUnlock()
	store.fileMu.Lock()
	paths := channelLogPathsNewest(store.channelsDir, channel)
	store.fileMu.Unlock()
	if len(paths) == 0 {
		return lines
	}
	readLimit := limit
	if isFeatureChannel(channel) {
		readLimit = min(limit*16, 100_000)
	}
	persisted, err := tailNonEmptyLinesFromPaths(paths, readLimit)
	if err == nil {
		if isFeatureChannel(channel) {
			return tailFeatureActivityLines(persisted, limit)
		}
		return persisted
	}
	return lines
}

func (store *Store) recordAttackLaunch(channel string, observedAt time.Time) {
	store.attackMu.Lock()
	launches := store.attackLaunches[channel]
	cutoff := observedAt.Add(-time.Hour)
	firstIncluded := 0
	for firstIncluded < len(launches) && launches[firstIncluded].Before(cutoff) {
		firstIncluded++
	}
	if firstIncluded > 0 {
		launches = append([]time.Time(nil), launches[firstIncluded:]...)
	}
	store.attackLaunches[channel] = append(launches, observedAt)
	store.attackMu.Unlock()
}

func (store *Store) Close() {
	if store == nil {
		return
	}
	store.stopRetention()
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
	case "lta":
		return []string{"gam"}
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
	tail := store.tails[channel]
	if tail == nil {
		tail = &memoryTail{}
		store.tails[channel] = tail
	}
	maxBytes := 0
	if store.persistenceEnabled.Load() && isDiagnosticChannel(channel) {
		maxBytes = diagnosticLiveTailMaxBytes
	}
	tail.append(line, store.capacity, maxBytes)
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
		batch, ok := store.nextPersistenceBatch()
		if !ok {
			return
		}
		store.persistBatch(batch)
	}
}

func (store *Store) nextPersistenceBatch() ([]persistenceEntry, bool) {
	for {
		store.persistMu.Lock()
		if len(store.persistQueue) > 0 {
			flushFirst := store.persistQueue[0].flushed != nil
			store.persistMu.Unlock()
			if !flushFirst {
				time.Sleep(persistenceBatchWindow)
			}
			store.persistMu.Lock()
			batch := store.persistQueue
			store.persistQueue = nil
			store.persistMu.Unlock()
			return batch, true
		}
		closed := store.persistClosed
		store.persistMu.Unlock()
		if closed {
			return nil, false
		}
		<-store.persistWake
	}
}

func (store *Store) persistBatch(batch []persistenceEntry) {
	store.fileMu.Lock()
	touched := make(map[string]struct{}, 2)
	for index := range batch {
		entry := &batch[index]
		if entry.flushed != nil {
			store.flushBuffersLocked(touched)
			clear(touched)
			close(entry.flushed)
			continue
		}
		if store.channelsDir != "" {
			writer, err := store.channelBufferLocked(entry.channel, entry.observedAt)
			if err == nil {
				_, _ = writer.WriteString(entry.line)
				_ = writer.WriteByte('\n')
				touched[entry.channel] = struct{}{}
			}
		}
		*entry = persistenceEntry{}
	}
	store.flushBuffersLocked(touched)
	store.fileMu.Unlock()
}

func (store *Store) flushBuffersLocked(channels map[string]struct{}) {
	for channel := range channels {
		if writer := store.buffers[channel]; writer != nil {
			_ = writer.Flush()
		}
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

func (store *Store) channelBufferLocked(channel string, now time.Time) (*bufio.Writer, error) {
	file, err := store.channelFileLocked(channel, now)
	if err != nil {
		return nil, err
	}
	if writer := store.buffers[channel]; writer != nil {
		return writer, nil
	}
	writer := bufio.NewWriterSize(file, persistenceWriteBufferSize)
	store.buffers[channel] = writer
	return writer, nil
}

func (store *Store) writableChannelPathLocked(channel string, now time.Time) (string, error) {
	return store.ensureChannelSessionLocked(channel, now, false)
}

func (store *Store) ensureChannelSessionLocked(channel string, now time.Time, forceNew bool) (string, error) {
	slot := now.Unix() / int64(channelLogRotation/time.Second)
	if !forceNew && store.channelSessions[channel] != nil && store.channelSessions[channel].slot == slot {
		return store.channelSessions[channel].path, nil
	}
	directory := filepath.Join(store.channelsDir, channel)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	path, err := nextChannelLogPath(directory, now)
	if err != nil {
		return "", err
	}
	if store.channelSessions[channel] == nil || store.channelSessions[channel].path != path {
		store.closeFileLocked(channel)
	}
	store.channelSessions[channel] = &channelLogSession{path: path, slot: slot}
	return path, nil
}

func (store *Store) closeFileLocked(channel string) {
	if writer := store.buffers[channel]; writer != nil {
		_ = writer.Flush()
	}
	if file := store.files[channel]; file != nil {
		_ = file.Close()
	}
	delete(store.buffers, channel)
	delete(store.files, channel)
	delete(store.filePaths, channel)
}

func (store *Store) closeFilesLocked() {
	for channel := range store.files {
		store.closeFileLocked(channel)
	}
	store.channelSessions = map[string]*channelLogSession{}
}

func nextChannelLogPath(directory string, now time.Time) (string, error) {
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

func loadAttackLaunches(directory string, since time.Time) map[string][]time.Time {
	launches := make(map[string][]time.Time, len(attackFeatureChannels))
	for _, channel := range attackFeatureChannels {
		launches[channel] = []time.Time{}
		for _, path := range channelLogPathsNewest(directory, channel) {
			lines, err := tailNonEmptyLines(path, 100_000)
			if err != nil {
				continue
			}
			for _, line := range lines {
				observedAt, severity, event, ok := parseFeatureActivityLine(line)
				if ok && severity == "INFO" && event == "ATTACK" && !observedAt.Before(since) {
					launches[channel] = append(launches[channel], observedAt)
				}
			}
		}
		sort.Slice(launches[channel], func(left int, right int) bool {
			return launches[channel][left].Before(launches[channel][right])
		})
	}
	return launches
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
	_, _, ok := featureActivityLineTokens(line)
	return ok
}

func parseFeatureActivityLine(line string) (time.Time, string, string, bool) {
	firstOpen := strings.IndexByte(line, '[')
	if firstOpen < 0 {
		return time.Time{}, "", "", false
	}
	observedAt, err := time.ParseInLocation(
		"2006-01-02 15:04:05.000000",
		strings.TrimSpace(line[:firstOpen]),
		time.Local,
	)
	if err != nil {
		return time.Time{}, "", "", false
	}
	severity, event, ok := featureActivityLineTokens(line)
	return observedAt, severity, event, ok
}

func featureActivityLineTokens(line string) (string, string, bool) {
	firstOpen := strings.IndexByte(line, '[')
	if firstOpen < 0 {
		return "", "", false
	}
	firstClose := strings.IndexByte(line[firstOpen+1:], ']')
	if firstClose < 0 {
		return "", "", false
	}
	firstClose += firstOpen + 1
	severity := strings.TrimSpace(line[firstOpen+1 : firstClose])
	if severity != "INFO" && severity != "WARN" && severity != "ERROR" {
		return "", "", false
	}
	remainder := strings.TrimSpace(line[firstClose+1:])
	if !strings.HasPrefix(remainder, "[") {
		return "", "", false
	}
	secondClose := strings.IndexByte(remainder[1:], ']')
	if secondClose < 0 {
		return "", "", false
	}
	event := strings.TrimSpace(remainder[1 : secondClose+1])
	_, exists := featureActivityEvents[event]
	return severity, event, exists
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
	if strings.HasPrefix(actor, "autokhan:") {
		return ChannelAutoKhan
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

func isDiagnosticChannel(channel string) bool {
	return channel == ChannelWebSocketGame || channel == ChannelAppSend
}
