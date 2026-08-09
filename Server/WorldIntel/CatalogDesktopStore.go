package WorldIntel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (store *DesktopStore) EnqueueCatalog(ctx context.Context, snapshot CatalogDatasetSnapshot) (bool, error) {
	if store == nil || store.db == nil {
		return false, fmt.Errorf("world intelligence store is unavailable")
	}
	if err := ValidateFinalizedCatalogSnapshot(snapshot); err != nil {
		return false, err
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return false, fmt.Errorf("encode catalog outbox snapshot: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := store.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO world_intel_catalog_outbox (
			snapshot_id, payload_json, created_at, next_attempt_at
		) VALUES (?, ?, ?, ?)
	`, snapshot.SnapshotID, payload, now, now)
	if err != nil {
		store.recordError(err)
		return false, fmt.Errorf("queue catalog snapshot: %w", err)
	}
	if _, err := store.db.ExecContext(ctx, `
		DELETE FROM world_intel_catalog_outbox
		WHERE snapshot_id IN (
			SELECT snapshot_id FROM world_intel_catalog_outbox
			ORDER BY created_at DESC
			LIMIT -1 OFFSET ?
		)
	`, maximumOutboxRows); err != nil {
		store.recordError(err)
		return false, fmt.Errorf("bound catalog outbox: %w", err)
	}
	store.setMetadata(ctx, "last_captured_at", snapshot.CapturedAt.Format(time.RFC3339Nano))
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (store *DesktopStore) PendingCatalog(ctx context.Context, now time.Time, limit int) ([]QueuedCatalogSnapshot, error) {
	if store == nil || store.db == nil {
		return nil, fmt.Errorf("world intelligence store is unavailable")
	}
	if limit <= 0 || limit > desktopBatchLimit {
		limit = desktopBatchLimit
	}
	rows, err := store.db.QueryContext(ctx, `
		SELECT payload_json, attempt_count
		FROM world_intel_catalog_outbox
		WHERE next_attempt_at <= ?
		ORDER BY created_at ASC
		LIMIT ?
	`, now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		store.recordError(err)
		return nil, fmt.Errorf("list catalog outbox: %w", err)
	}
	defer rows.Close()
	result := make([]QueuedCatalogSnapshot, 0, limit)
	for rows.Next() {
		var payload []byte
		var attempts int
		if err := rows.Scan(&payload, &attempts); err != nil {
			return nil, fmt.Errorf("scan catalog outbox: %w", err)
		}
		var snapshot CatalogDatasetSnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			return nil, fmt.Errorf("decode catalog outbox snapshot: %w", err)
		}
		result = append(result, QueuedCatalogSnapshot{Snapshot: snapshot, Attempts: attempts})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read catalog outbox: %w", err)
	}
	return result, nil
}

func (store *DesktopStore) ConfirmCatalog(ctx context.Context, snapshotID string, uploadedAt time.Time) error {
	if _, err := store.db.ExecContext(ctx, `DELETE FROM world_intel_catalog_outbox WHERE snapshot_id = ?`, snapshotID); err != nil {
		store.recordError(err)
		return fmt.Errorf("confirm catalog upload: %w", err)
	}
	store.setMetadata(ctx, "last_upload_at", uploadedAt.UTC().Format(time.RFC3339Nano))
	store.setMetadata(ctx, "last_upload_error", "")
	return nil
}

func (store *DesktopStore) FailCatalog(ctx context.Context, snapshotID string, attempts int, message string, now time.Time) error {
	attempts++
	delay := time.Duration(1<<min(attempts, 10)) * time.Second
	if delay > time.Hour {
		delay = time.Hour
	}
	message = strings.TrimSpace(message)
	if len(message) > 1_000 {
		message = message[:1_000]
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE world_intel_catalog_outbox
		SET attempt_count = ?, next_attempt_at = ?, last_error = ?
		WHERE snapshot_id = ?
	`, attempts, now.UTC().Add(delay).Format(time.RFC3339Nano), message, snapshotID); err != nil {
		store.recordError(err)
		return fmt.Errorf("defer catalog upload: %w", err)
	}
	store.setMetadata(ctx, "last_upload_error", message)
	return nil
}

func (store *DesktopStore) RecordCatalogSuccess(
	ctx context.Context,
	bucket time.Time,
	completedAt time.Time,
	version string,
	sourceDigest string,
	collectionRevision int,
	datasets int,
) {
	store.setMetadata(ctx, "last_catalog_bucket", bucket.UTC().Format(time.RFC3339Nano))
	store.setMetadata(ctx, "last_catalog_at", completedAt.UTC().Format(time.RFC3339Nano))
	store.setMetadata(ctx, "last_catalog_version", strings.TrimSpace(version))
	store.setMetadata(ctx, "last_catalog_source_digest", strings.TrimSpace(sourceDigest))
	store.setMetadata(ctx, "last_catalog_collection_revision", fmt.Sprintf("%d", collectionRevision))
	store.setMetadata(ctx, "last_catalog_datasets", fmt.Sprintf("%d", datasets))
	store.setMetadata(ctx, "last_catalog_error", "")
}

func (store *DesktopStore) CatalogCollectionCurrent(ctx context.Context, sourceDigest string, collectionRevision int) bool {
	digest, digestFound, digestErr := metadataValue(ctx, store.db, "last_catalog_source_digest")
	revision, revisionFound, revisionErr := metadataValue(ctx, store.db, "last_catalog_collection_revision")
	return digestErr == nil && revisionErr == nil && digestFound && revisionFound &&
		strings.EqualFold(strings.TrimSpace(digest), strings.TrimSpace(sourceDigest)) && revision == fmt.Sprintf("%d", collectionRevision)
}

func (store *DesktopStore) RecordCatalogError(ctx context.Context, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 1_000 {
		message = message[:1_000]
	}
	store.setMetadata(ctx, "last_catalog_error", message)
}

func (store *DesktopStore) LastCatalogBucket(ctx context.Context) (time.Time, bool) {
	if store == nil || store.db == nil {
		return time.Time{}, false
	}
	value, found, err := metadataValue(ctx, store.db, "last_catalog_bucket")
	if err != nil || !found {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed.UTC(), err == nil
}
