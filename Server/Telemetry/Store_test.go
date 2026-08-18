package Telemetry

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

func TestPersistentLoggingDoesNotBlockCommandRecording(t *testing.T) {
	store := NewStore(100)
	if err := store.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	store.fileMu.Lock()
	recorded := make(chan struct{})
	go func() {
		store.RecordAppOutbound(`%xt%EmpireEx_21%bup%1%{}%`, "automation:autoRecruit")
		close(recorded)
	}()
	select {
	case <-recorded:
	case <-time.After(50 * time.Millisecond):
		store.fileMu.Unlock()
		t.Fatal("command recording waited for persistent log I/O")
	}
	store.fileMu.Unlock()
	store.Close()
	paths := channelLogPathsNewest(store.channelsDir, ChannelAppSend)
	if len(paths) != 1 {
		t.Fatalf("persistent app log paths = %v, want one rotated file", paths)
	}
	contents, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "[SEND] [bup]") {
		t.Fatalf("persistent app log = %q", contents)
	}
}

func TestPersistentDiagnosticTailDoesNotRetainOversizedRawFrame(t *testing.T) {
	store := NewStore(100)
	if err := store.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	line := strings.Repeat("x", diagnosticLiveTailMaxBytes+1)
	store.mu.Lock()
	store.appendLocked(ChannelWebSocketGame, line, time.Now())
	store.mu.Unlock()
	store.flushPersistence()

	store.mu.RLock()
	tail := store.tails[ChannelWebSocketGame]
	retainedLines := len(tail.activeLines())
	retainedBytes := tail.bytes
	store.mu.RUnlock()
	if retainedLines != 0 || retainedBytes != 0 {
		t.Fatalf("oversized live tail retained %d lines and %d bytes", retainedLines, retainedBytes)
	}

	paths := channelLogPathsNewest(store.channelsDir, ChannelWebSocketGame)
	if len(paths) != 1 {
		t.Fatalf("persistent websocket paths = %v, want one", paths)
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(line)+1) {
		t.Fatalf("persistent websocket bytes = %d, want %d", info.Size(), len(line)+1)
	}
}

func TestPersistentBatchPreservesLineOrder(t *testing.T) {
	store := NewStore(100)
	if err := store.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	const lineCount = 1000
	for index := range lineCount {
		store.append(ChannelWebSocketGame, fmt.Sprintf("line-%04d", index))
	}
	store.flushPersistence()
	paths := channelLogPathsNewest(store.channelsDir, ChannelWebSocketGame)
	if len(paths) != 1 {
		t.Fatalf("persistent websocket paths = %v, want one", paths)
	}
	contents, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	if len(lines) != lineCount {
		t.Fatalf("persistent lines = %d, want %d", len(lines), lineCount)
	}
	for index, line := range lines {
		expected := fmt.Sprintf("line-%04d", index)
		if line != expected {
			t.Fatalf("persistent line %d = %q, want %q", index, line, expected)
		}
	}
}

func TestStoreSeparatesGameAndCitadelCommandLogs(t *testing.T) {
	store := NewStore(100)
	store.RecordAppOutbound(`%xt%EmpireEx_21%gam%1%{"source":"citadel"}%`, "automation:autoBird")
	store.RecordFeatureActivity(
		"automation:autoBird", "troops.station", "INFO", "TRANSPORT", "Stationed 500 troops from Main Castle",
	)

	inboundRaw := `%xt%EmpireEx_21%gam%1%{"source":"game"}%`
	inbound, err := Protocol.Decode(inboundRaw, Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatalf("decode inbound frame: %v", err)
	}
	store.Record(Protocol.CommittedFrame{Frame: inbound}, nil)
	manualRaw := `%xt%EmpireEx_21%gcl%1%{"source":"manual-game"}%`
	manual, err := Protocol.Decode(manualRaw, Protocol.DirectionOutbound, time.Now())
	if err != nil {
		t.Fatalf("decode manual frame: %v", err)
	}
	store.Record(Protocol.CommittedFrame{Frame: manual}, nil)

	loginRaw := `%xt%EmpireEx_21%lli%1%{"token":"kept-in-full"}%`
	login, err := Protocol.Decode(loginRaw, Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatalf("decode login frame: %v", err)
	}
	store.Record(Protocol.CommittedFrame{Frame: login}, nil)

	gameLog := strings.Join(store.Tail(ChannelWebSocketGame, 10), "\n")
	if !strings.Contains(gameLog, `"token":"kept-in-full"`) || !strings.Contains(gameLog, `"source":"manual-game"`) {
		t.Fatalf("websocket log did not retain every full raw frame: %q", gameLog)
	}

	appLog := strings.Join(store.Tail(ChannelAppSend, 10), "\n")
	if !strings.Contains(appLog, "[SEND] [gam]") || !strings.Contains(appLog, "[MATCH] [gam]") {
		t.Fatalf("app command log = %q, want send and matching response", appLog)
	}
	if strings.Contains(appLog, "manual-game") {
		t.Fatalf("app command log included manual game traffic: %q", appLog)
	}

	autoBirdLog := strings.Join(store.Tail(ChannelAutoBird, 10), "\n")
	if !strings.Contains(autoBirdLog, "[INFO] [TRANSPORT] Stationed 500 troops from Main Castle") {
		t.Fatalf("Auto Bird log = %q, want one user-facing activity", autoBirdLog)
	}
	if strings.Contains(autoBirdLog, "[SEND]") || strings.Contains(autoBirdLog, "[MATCH]") ||
		strings.Contains(autoBirdLog, "intent=") || strings.Contains(autoBirdLog, "operation=") {
		t.Fatalf("Auto Bird log contains diagnostic details: %q", autoBirdLog)
	}
}

func TestWebSocketGameMarksNonzeroResponsesAsErrors(t *testing.T) {
	store := NewStore(100)
	for _, raw := range []string{
		`%xt%gam%1%0%{}%`,
		`%xt%cra%1%256%{}%`,
		`%xt%rae%1%-1%{}%`,
	} {
		frame, err := Protocol.Decode(raw, Protocol.DirectionInbound, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		store.Record(Protocol.CommittedFrame{Frame: frame}, nil)
	}
	outbound, err := Protocol.Decode(`%xt%EmpireEx_21%gam%1%{}%`, Protocol.DirectionOutbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store.Record(Protocol.CommittedFrame{Frame: outbound}, nil)

	gameLog := strings.Join(store.Tail(ChannelWebSocketGame, 10), "\n")
	if !strings.Contains(gameLog, "[RECV] [gam] %xt%gam%1%0%{}%") {
		t.Fatalf("successful reply was not logged as received: %q", gameLog)
	}
	if !strings.Contains(gameLog, "[ERROR] [cra] %xt%cra%1%256%{}%") ||
		!strings.Contains(gameLog, "[ERROR] [rae] %xt%rae%1%-1%{}%") {
		t.Fatalf("nonzero replies were not logged as errors: %q", gameLog)
	}
	if !strings.Contains(gameLog, "[SEND] [gam] %xt%EmpireEx_21%gam%1%{}%") {
		t.Fatalf("outbound sequence token was incorrectly treated as an error: %q", gameLog)
	}
}

func TestFeatureTailHidesLegacyDiagnostics(t *testing.T) {
	store := NewStore(100)
	if err := store.SetDataDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now()
	store.append(ChannelAutoBird, formatLine(now, "INFO", "cycle", "start"))
	store.append(ChannelAutoBird, formatLine(now, "SEND", "gam", `%xt%EmpireEx_21%gam%1%{}%`))
	store.RecordFeatureActivity("automation:autoBird", "troops.station", "INFO", "TRANSPORT", "Stationed 500 troops")
	store.RecordFeatureActivity("automation:autoBird", "troops.station", "ERROR", "TRANSPORT", "Could not station 500 troops: no commander was available")

	log := strings.Join(store.Tail(ChannelAutoBird, 100), "\n")
	if !strings.Contains(log, "Stationed 500 troops") || !strings.Contains(log, "no commander was available") {
		t.Fatalf("feature log = %q, want success and failure activities", log)
	}
	if strings.Contains(log, "[cycle]") || strings.Contains(log, "[SEND]") || strings.Contains(log, "%xt%") {
		t.Fatalf("feature log contains legacy diagnostics: %q", log)
	}
}

func TestAttackLaunchCountsTrackSuccessfulFeatureAttacksAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	store := NewStore(100)
	if err := store.SetDataDir(dataDir); err != nil {
		t.Fatal(err)
	}
	store.RecordFeatureActivity("automation:autoTowers", "tower.attack", "INFO", "ATTACK", "Launched tower attack")
	store.RecordFeatureActivity("automation:autoStorm", "storm.attack", "INFO", "ATTACK", "Launched Storm attack")
	store.RecordFeatureActivity("automation:autoStorm", "storm.attack", "ERROR", "ATTACK", "Could not launch Storm attack")
	store.RecordFeatureActivity("automation:autoStorm", "storm.shop.purchase", "INFO", "PURCHASE", "Purchased tools")

	counts := store.AttackLaunchCounts(time.Now())
	if counts[ChannelAutoTowers] != 1 || counts[ChannelAutoStorm] != 1 {
		t.Fatalf("attack launch counts = %#v", counts)
	}
	store.Close()

	reloaded := NewStore(100)
	if err := reloaded.SetDataDir(dataDir); err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	counts = reloaded.AttackLaunchCounts(time.Now())
	if counts[ChannelAutoTowers] != 1 || counts[ChannelAutoStorm] != 1 {
		t.Fatalf("reloaded attack launch counts = %#v", counts)
	}
}

func TestStoreMatchesMappedAndOutOfOrderAppResponses(t *testing.T) {
	store := NewStore(100)
	store.RecordAppOutbound(`%xt%EmpireEx_21%jca%1%{"CID":10}%`, "automation:autoHospital")
	store.RecordAppOutbound(`%xt%EmpireEx_21%cra%1%{}%`, "automation:autoTowers")

	for _, raw := range []string{
		`%xt%cra%1%0%{}%`,
		`%xt%jaa%1%0%{"KID":0}%`,
	} {
		frame, err := Protocol.Decode(raw, Protocol.DirectionInbound, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		store.Record(Protocol.CommittedFrame{Frame: frame}, nil)
	}
	appLog := strings.Join(store.Tail(ChannelAppSend, 10), "\n")
	if !strings.Contains(appLog, "[MATCH] [cra]") || !strings.Contains(appLog, "[MATCH] [jaa]") {
		t.Fatalf("mapped app responses were not matched: %q", appLog)
	}
	if len(store.pendingAppCommand) != 0 {
		t.Fatalf("pending app commands = %#v", store.pendingAppCommand)
	}
}

func TestStoreMatchesAllianceHelpAHHResponse(t *testing.T) {
	store := NewStore(100)
	store.RecordAppOutbound(`%xt%EmpireEx_21%ahr%1%{"ID":1,"T":2}%`, "automation:autoHospital")
	frame, err := Protocol.Decode(`%xt%ahh%1%0%{"LID":7}%`, Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store.Record(Protocol.CommittedFrame{Frame: frame}, nil)
	appLog := strings.Join(store.Tail(ChannelAppSend, 10), "\n")
	if !strings.Contains(appLog, "[MATCH] [ahh]") {
		t.Fatalf("alliance-help response was not matched: %q", appLog)
	}
}

func TestStoreMatchesKhanLTAWithMovementResponse(t *testing.T) {
	store := NewStore(100)
	store.RecordAppOutbound(`%xt%EmpireEx_21%lta%1%{"AV":0,"EID":72}%`, "automation:autoKhan")
	frame, err := Protocol.Decode(`%xt%gam%1%0%{"M":[]}%`, Protocol.DirectionInbound, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store.Record(Protocol.CommittedFrame{Frame: frame}, nil)
	appLog := strings.Join(store.Tail(ChannelAppSend, 10), "\n")
	if !strings.Contains(appLog, "[MATCH] [gam]") {
		t.Fatalf("Khan LTA movement response was not matched: %q", appLog)
	}
}

func TestFeatureChannelForActorIncludesCurrentAutomations(t *testing.T) {
	tests := map[string]string{
		"automation:autoTowers":        ChannelAutoTowers,
		"automation:autoInvasion":      ChannelAutoInvasion,
		"automation:autoNomad":         ChannelAutoNomad,
		"automation:autoAdvisor":       ChannelAutoAdvisor,
		"automation:autoKhan":          ChannelAutoKhan,
		"automation:autoKhan:cooldown": ChannelAutoKhan,
		"automation:autoKhan:rage":     ChannelAutoKhan,
		"automation:autoStorm":         ChannelAutoStorm,
		"ui:auto-equipment-cleanup":    ChannelAutoEquipment,
	}
	for actor, expected := range tests {
		if actual := featureChannelForActor(actor); actual != expected {
			t.Errorf("featureChannelForActor(%q) = %q, want %q", actor, actual, expected)
		}
	}
}

func TestStoreAlwaysExposesLegacyLoggerChannels(t *testing.T) {
	store := NewStore(100)
	channels := store.Channels()
	if len(channels) != len(knownChannels) {
		t.Fatalf("channel count = %d, want %d", len(channels), len(knownChannels))
	}
	if channels[0].ID != ChannelWebSocketGame || channels[1].ID != ChannelAppSend {
		t.Fatalf("first logger channels = %#v, want websocket then app commands", channels[:2])
	}
}

func BenchmarkMemoryTailAppendAtCapacity(b *testing.B) {
	tail := &memoryTail{}
	for range 5000 {
		tail.append("frame", 5000, 0)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tail.append("frame", 5000, 0)
	}
}

func BenchmarkLegacySliceTailAppendAtCapacity(b *testing.B) {
	lines := make([]string, 5000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		lines = append(lines, "frame")
		copy(lines, lines[len(lines)-5000:])
		lines = lines[:5000]
	}
	_ = lines
}
