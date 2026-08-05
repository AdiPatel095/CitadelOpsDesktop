package WorldIntel

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDesktopStoreRestrictsDatabasePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses ACLs rather than Unix mode bits")
	}
	directory := t.TempDir()
	store, err := OpenDesktopStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	info, err := os.Stat(filepath.Join(directory, "Runtime", "WorldIntelligence.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("database permissions = %o, want 600", permissions)
	}
}

func TestDesktopStorePersistsCredentialsAndDeduplicatesBatches(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDesktopStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.Credentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Credentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first.InstallationID) != 32 || len(first.Secret) != 64 {
		t.Fatalf("credentials were not stable: %#v %#v", first, second)
	}

	now := time.Now().UTC().Truncate(time.Second)
	batch, err := FinalizeBatch(ObservationBatch{
		WorldID: "world.example", CapturedAt: now,
		Players: []PlayerObservation{{PlayerID: 1, Name: "Player", Source: "account", ObservedAt: now}},
	})
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := store.Enqueue(ctx, batch)
	if err != nil || !inserted {
		t.Fatalf("first enqueue = %v, %v", inserted, err)
	}
	inserted, err = store.Enqueue(ctx, batch)
	if err != nil || inserted {
		t.Fatalf("duplicate enqueue = %v, %v", inserted, err)
	}
	pending, err := store.Pending(ctx, time.Now().Add(time.Second), 10)
	if err != nil || len(pending) != 1 || pending[0].Batch.BatchID != batch.BatchID {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	if err := store.Confirm(ctx, batch.BatchID, now); err != nil {
		t.Fatal(err)
	}
	if status := store.Status(ctx); status.Pending != 0 || status.LastUploadAt == nil {
		t.Fatalf("status = %#v", status)
	}
}
