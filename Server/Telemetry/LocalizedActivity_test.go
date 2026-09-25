package Telemetry

import (
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalizedActivityPersistenceKeepsRecordIdentity(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(100)
	if err := store.SetDataDir(dir); err != nil {
		t.Fatal(err)
	}
	store.RecordFeatureActivity("automation:autoTowers", "attack.launch", "INFO", "ATTACK", "Legacy attack")
	descriptor := Localization.New("activity.attack", "Attack {name}", Localization.Params{"name": "Castle {raw}"})
	store.RecordFeatureActivityMessage("automation:autoTowers", "attack.launch", "INFO", "ATTACK", "Attack Castle {raw}", descriptor)
	descriptor.Params["name"] = "mutated"
	before := store.Tail(ChannelAutoTowers, 10)
	entries, ok := store.UserFacingEntries(ChannelAutoTowers, 10)
	if !ok || len(entries) != 2 || entries[0].TranslationStatus != "untranslated" || entries[1].TranslationStatus != "structured" {
		t.Fatalf("%+v", entries)
	}
	if entries[1].MessageDescriptor.Params["name"] != "Castle {raw}" {
		t.Fatal("descriptor mutation leaked")
	}
	store.Close()
	restored := NewStore(100)
	if err := restored.SetDataDir(dir); err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	after := restored.Tail(ChannelAutoTowers, 10)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("raw lines changed: %v / %v", before, after)
	}
	entries, ok = restored.UserFacingEntries(ChannelAutoTowers, 10)
	if !ok || len(entries) != 2 || entries[1].MessageDescriptor.Params["name"] != "Castle {raw}" {
		t.Fatalf("restart lost association %+v", entries)
	}
	if restored.AttackLaunchCounts(time.Now())[ChannelAutoTowers] != 2 {
		t.Fatal("restart changed attack count")
	}
	if _, ok := restored.UserFacingEntries(ChannelAppSend, 10); ok {
		t.Fatal("diagnostic channel exposed")
	}
	paths := channelLogPathsNewest(restored.channelsDir, ChannelAutoTowers)
	file, err := os.OpenFile(paths[0], os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString(`{"schema":"citadel.activity.v1","line":"2026-09-20 20:00:00.000000 [INFO] [ATTACK] truncated`)
	file.Close()
	entries, _ = restored.UserFacingEntries(ChannelAutoTowers, 10)
	if len(entries) != 2 || entries[1].MessageDescriptor.Params["name"] != "Castle {raw}" {
		t.Fatalf("truncation shifted descriptors %+v", entries)
	}
}

func TestLocalizedActivityRetentionAndUnsafeDetails(t *testing.T) {
	observed := time.Now().Add(-48 * time.Hour).Truncate(time.Microsecond)
	record := encodeActivityRecord(formatLine(observed, "INFO", "ATTACK", "Attack"), Localization.New("attack", "Attack", nil))
	actual, ok := persistentLogTimestamp(record)
	if !ok || !actual.Equal(observed) {
		t.Fatalf("timestamp %v %v", actual, ok)
	}
	store := NewStore(10)
	defer store.Close()
	store.RecordFeatureActivityMessage("automation:autoTowers", "attack.launch", "ERROR", "ATTACK", "payload CRA AID=123", Localization.New("unsafe", "payload {value}", Localization.Params{"value": "private"}))
	entries, _ := store.UserFacingEntries(ChannelAutoTowers, 10)
	if len(entries) != 1 || entries[0].MessageDescriptor != nil || strings.Contains(entries[0].Line, "AID") {
		t.Fatalf("sanitization mismatch %+v", entries)
	}
}

func TestLocalizedActivityRotationAndChannelActivityTruncation(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(100)
	if err := store.SetDataDir(dir); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-4 * time.Hour)
	recent := time.Now()
	first := formatLine(old, "INFO", "ATTACK", "First")
	second := formatLine(recent, "INFO", "ATTACK", "Second")
	store.mu.Lock()
	store.appendLocked(ChannelActivity, first, old)
	store.appendLocked(ChannelActivity, encodeActivityRecord(second, Localization.New("second", "Second", nil)), recent)
	store.mu.Unlock()
	store.Close()
	restored := NewStore(100)
	if err := restored.SetDataDir(dir); err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	paths := channelLogPathsNewest(restored.channelsDir, ChannelActivity)
	if len(paths) != 2 {
		t.Fatalf("rotation paths=%v", paths)
	}
	f, err := os.OpenFile(paths[0], os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"schema":"citadel.activity.v1","line":"2026-09-20 20:00:00.000000 [INFO] [ATTACK] incomplete`)
	f.Close()
	entries, ok := restored.UserFacingEntries(ChannelActivity, 10)
	if !ok || len(entries) != 2 || entries[0].Line != first || entries[1].Line != second || entries[1].MessageDescriptor.Key != "second" {
		t.Fatalf("rotation/truncation misassociated entries: %+v", entries)
	}
}

func TestTelemetryFiniteKeyFamilyMatchesSourceCatalog(t *testing.T) {
	raw, err := os.ReadFile("../Localization/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]string
	if err = json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	store := NewStore(10)
	defer store.Close()
	for _, channel := range store.Channels() {
		if catalog[channel.LabelDescriptor.Key] != channel.Label || catalog[channel.DescriptionDescriptor.Key] != channel.Description {
			t.Fatalf("channel keys missing for %s", channel.ID)
		}
	}
}
