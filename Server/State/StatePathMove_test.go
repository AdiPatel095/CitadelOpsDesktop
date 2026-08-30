package State

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceStateFileReplacesExistingDestination(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.json")
	destination := filepath.Join(directory, "destination.json")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := replaceStateFile(source, destination); err != nil {
		t.Fatalf("replace state file: %v", err)
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" {
		t.Fatalf("replacement contents = %q, want new", contents)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement source still exists: %v", err)
	}
}

func TestReplaceStateFilePreservesMoveFailure(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "missing.json")
	destination := filepath.Join(directory, "destination.json")
	err := replaceStateFile(source, destination)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replace missing state file error = %v, want not-exist", err)
	}
}

func TestMoveStateDirectoryMovesWithoutReplacing(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "state.json"), []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := moveStateDirectory(source, destination); err != nil {
		t.Fatalf("move state directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "state.json")); err != nil {
		t.Fatalf("moved state file: %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("moved state directory still exists: %v", err)
	}
}
