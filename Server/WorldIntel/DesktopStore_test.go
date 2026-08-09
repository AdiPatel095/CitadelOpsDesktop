package WorldIntel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestDesktopStoreQueuesCatalogSnapshotsSeparately(t *testing.T) {
	ctx := context.Background()
	store, err := OpenDesktopStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC().Truncate(time.Second)
	snapshot, err := FinalizeCatalogSnapshot(CatalogDatasetSnapshot{
		Source: OfficialCatalogSource, SourceVersion: "781.02",
		SourceURL:    "https://empire-html5.goodgamestudios.com/default/items/items_v781.02.json",
		SourceDigest: strings.Repeat("a", 64), DatasetKey: "islandrewardranks",
		DatasetLabel: "Storm alliance rank rewards", Category: "storm",
		Rows: json.RawMessage(`[{"cargoPointRequirement":"500"}]`), CapturedAt: now, CollectorPlayerID: 17334928,
	})
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := store.EnqueueCatalog(ctx, snapshot)
	if err != nil || !inserted {
		t.Fatalf("catalog enqueue = %v, %v", inserted, err)
	}
	inserted, err = store.EnqueueCatalog(ctx, snapshot)
	if err != nil || inserted {
		t.Fatalf("duplicate catalog enqueue = %v, %v", inserted, err)
	}
	pending, err := store.PendingCatalog(ctx, now.Add(time.Second), 10)
	if err != nil || len(pending) != 1 || pending[0].Snapshot.SnapshotID != snapshot.SnapshotID {
		t.Fatalf("catalog pending = %#v, %v", pending, err)
	}
	if status := store.Status(ctx); status.Pending != 1 || status.PendingCatalogs != 1 {
		t.Fatalf("catalog status = %#v", status)
	}
	if err := store.ConfirmCatalog(ctx, snapshot.SnapshotID, now); err != nil {
		t.Fatal(err)
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
