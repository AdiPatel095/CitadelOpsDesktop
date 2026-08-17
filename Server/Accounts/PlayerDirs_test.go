package Accounts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlayerKeyIsStableAndPathSafe(t *testing.T) {
	cases := map[string]string{
		playerKey("ep-live-us1-game.goodgamestudios.com", 145046): "ep-live-us1-game-goodgamestudios-com-p145046",
		playerKey("EP-Live US1_game", 42):                         "ep-live-us1-game-p42",
		playerKey("  EmpireEx_21  ", 7):                           "empireex-21-p7",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("playerKey = %q, want %q", got, want)
		}
	}
	if playerKey("wss://host:443/path", 1) != playerKey("WSS://HOST:443/PATH", 1) {
		t.Fatal("player keys must be case-insensitive")
	}
	for _, key := range []string{playerKey("wss://host:443", 9)} {
		if strings.ContainsAny(key, "/\\: ") {
			t.Fatalf("player key %q is not path safe", key)
		}
	}
}

func TestPlayerBindingsRoundTripAndTolerateAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Accounts", playerBindingsFileName)
	loaded, err := loadPlayerBindings(path)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("missing registry should load empty: %v %v", loaded, err)
	}
	want := map[string]string{"acct-1": "world-p1", "acct-2": "world-p2"}
	if err := savePlayerBindings(path, want); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadPlayerBindings(path)
	if err != nil {
		t.Fatal(err)
	}
	for runtimeID, key := range want {
		if loaded[runtimeID] != key {
			t.Fatalf("binding %s = %q, want %q", runtimeID, loaded[runtimeID], key)
		}
	}
}

func TestMergeStagingIntoPlayerDir(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "Accounts", "acct-fresh")
	player := filepath.Join(root, playerDirsName, "world-p1")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The corpus has the long history; staging has today's rows plus the
	// freshest installed credentials and an unrelated state component.
	write(filepath.Join(player, "History", "PlayerSamples.jsonl"), "old-1\nold-2\n")
	write(filepath.Join(staging, "History", "PlayerSamples.jsonl"), "new-1\n")
	write(filepath.Join(staging, "History", "SpyReports.jsonl"), "spy-1\n")
	write(filepath.Join(staging, "Session", "LoginCredentials.json"), `{"fresh":true}`)
	write(filepath.Join(staging, "State", "Components", "x.json"), "{}")

	if err := mergeStagingIntoPlayerDir(staging, player, "123"); err != nil {
		t.Fatal(err)
	}

	samples, err := os.ReadFile(filepath.Join(player, "History", "PlayerSamples.jsonl"))
	if err != nil || string(samples) != "old-1\nold-2\nnew-1\n" {
		t.Fatalf("merged samples = %q (%v)", samples, err)
	}
	spies, err := os.ReadFile(filepath.Join(player, "History", "SpyReports.jsonl"))
	if err != nil || string(spies) != "spy-1\n" {
		t.Fatalf("new collection = %q (%v)", spies, err)
	}
	credentials, err := os.ReadFile(filepath.Join(player, "Session", "LoginCredentials.json"))
	if err != nil || string(credentials) != `{"fresh":true}` {
		t.Fatalf("credentials = %q (%v)", credentials, err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatal("staging directory should have been retired")
	}
	if _, err := os.Stat(staging + ".superseded-123"); err != nil {
		t.Fatal("staging directory should survive as a superseded copy")
	}
	// The corpus state, not the staging stub, is what the runtime resumes on.
	if _, err := os.Stat(filepath.Join(player, "State", "Components", "x.json")); !os.IsNotExist(err) {
		t.Fatal("staging state must not overwrite the corpus")
	}
}

func TestAccountDataDirResolvesThroughBinding(t *testing.T) {
	root := t.TempDir()
	supervisor := &Supervisor{
		config:         Config{DataRoot: root},
		playerBindings: map[string]string{"acct-bound": "world-p9"},
	}
	bound, err := supervisor.accountDataDir(AccountID("acct-bound"), "")
	if err != nil {
		t.Fatal(err)
	}
	if bound != filepath.Join(root, playerDirsName, "world-p9") {
		t.Fatalf("bound dir = %q", bound)
	}
	fresh, err := supervisor.accountDataDir(AccountID("acct-fresh"), "")
	if err != nil {
		t.Fatal(err)
	}
	if fresh != filepath.Join(root, "Accounts", "acct-fresh") {
		t.Fatalf("staging dir = %q", fresh)
	}
	// Explicit desktop roots stay untouched, and escapes are still refused.
	if _, err := supervisor.accountDataDir(AccountID("acct-x"), root); err != nil {
		t.Fatalf("desktop root: %v", err)
	}
	if _, err := supervisor.accountDataDir(AccountID("acct-x"), filepath.Join(root, "..", "outside")); err == nil {
		t.Fatal("escape must be refused")
	}
}
