package Telemetry

import (
	"os"
	"path/filepath"
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
	contents, err := os.ReadFile(filepath.Join(store.channelsDir, ChannelAppSend+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "[SEND] [bup]") {
		t.Fatalf("persistent app log = %q", contents)
	}
}

func TestStoreSeparatesGameAndCitadelCommandLogs(t *testing.T) {
	store := NewStore(100)
	store.RecordAppOutbound(`%xt%EmpireEx_21%gam%1%{"source":"citadel"}%`, "automation:autoBird")

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
	if !strings.Contains(autoBirdLog, "[SEND] [gam]") || !strings.Contains(autoBirdLog, "[MATCH] [gam]") {
		t.Fatalf("Auto Bird log = %q, want dispatched command and matching response", autoBirdLog)
	}
}

func TestFeatureChannelForActorIncludesCurrentAutomations(t *testing.T) {
	tests := map[string]string{
		"automation:autoTowers":     ChannelAutoTowers,
		"automation:autoInvasion":   ChannelAutoInvasion,
		"automation:autoNomad":      ChannelAutoNomad,
		"automation:autoKhan":       ChannelAutoKhan,
		"automation:autoStorm":      ChannelAutoStorm,
		"ui:auto-equipment-cleanup": ChannelAutoEquipment,
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
