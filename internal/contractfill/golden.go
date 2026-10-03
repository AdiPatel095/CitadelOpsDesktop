package contractfill

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Golden compares payload and required-ness metadata, or updates both together.
func Golden(t *testing.T, directory, name string, value any, update bool) {
	t.Helper()
	for suffix, document := range map[string]any{".json": value, ".keys.json": Keys(value)} {
		raw, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, '\n')
		path := filepath.Join(directory, name+suffix)
		if update {
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		committed, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(raw, committed) {
			t.Fatalf("sender fixture drift: %s; run sync-cell-contracts.mjs", path)
		}
	}
}

func ReadReceiver(t *testing.T, directory, name string) ([]byte, map[string]bool, map[string]string) {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(directory, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]bool
	var optional map[string]string
	for path, target := range map[string]any{filepath.Join(directory, name+".keys.json"): &keys, filepath.Join(directory, "..", "optional-keys.json"): &optional} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	return payload, keys, optional
}

// Inventory fails if a fixture is missing or has no corresponding test case.
func Inventory(t *testing.T, directory string, names []string, bodies ...string) {
	t.Helper()
	expected := map[string]bool{}
	for _, name := range names {
		expected[name+".json"] = true
		expected[name+".keys.json"] = true
	}
	for _, name := range bodies {
		expected[name+".json"] = true
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || !expected[entry.Name()] {
			t.Fatalf("uncovered contract fixture: %s", entry.Name())
		}
		delete(expected, entry.Name())
	}
	for name := range expected {
		t.Fatalf("missing contract fixture: %s", name)
	}
}

// GoldenBody records the real handler body without inventing a response type
// or required-ness metadata for a map acknowledgement the backend ignores.
func GoldenBody(t *testing.T, directory, name string, raw []byte, update bool) {
	t.Helper()
	var canonical bytes.Buffer
	if err := json.Indent(&canonical, bytes.TrimSpace(raw), "", "  "); err != nil {
		t.Fatal(err)
	}
	expected := append(canonical.Bytes(), '\n')
	path := filepath.Join(directory, name+".json")
	if update {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, expected, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, committed) {
		t.Fatalf("sender fixture drift: %s; run sync-cell-contracts.mjs", path)
	}
}
