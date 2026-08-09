package WorldIntel

import (
	"context"
	"fmt"
	"time"
)

const (
	catalogCollectionRevision = 1
	catalogRefreshInterval    = 6 * time.Hour
)

func (service *DesktopService) runCatalogCollector(ctx context.Context) {
	ticker := time.NewTicker(scannerPoll)
	defer ticker.Stop()
	service.tryScheduledCatalogCollection(ctx, time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			service.tryScheduledCatalogCollection(ctx, now.UTC())
		}
	}
}

func (service *DesktopService) tryScheduledCatalogCollection(ctx context.Context, now time.Time) {
	if service == nil || service.store == nil || service.gameData == nil {
		return
	}
	settings := service.Settings()
	if !settings.collectsCatalog() {
		return
	}
	bucket, due := collectorBucket(now, settings.CollectorSlot, settings.CollectorSlots)
	if !due {
		return
	}
	if lastBucket, found := service.store.LastCatalogBucket(ctx); found && lastBucket.Equal(bucket) {
		return
	}
	service.collectionMu.Lock()
	if service.collectionInProgress || now.Before(service.nextCollectionRetryAt) {
		service.collectionMu.Unlock()
		return
	}
	service.collectionInProgress = true
	service.collectedDatasets = 0
	shouldRefresh := service.refreshCatalog != nil && now.Sub(service.lastCatalogRefresh) >= catalogRefreshInterval
	service.collectionMu.Unlock()
	if shouldRefresh {
		if err := service.refreshCatalog(ctx); err != nil {
			service.finishCatalogCollection(ctx, now, fmt.Errorf("refresh official item catalog: %w", err))
			return
		}
		service.collectionMu.Lock()
		service.lastCatalogRefresh = time.Now().UTC()
		service.collectionMu.Unlock()
	}

	current, found := service.gameData.Current()
	if !found {
		service.finishCatalogCollection(ctx, now, fmt.Errorf("official item catalog is unavailable"))
		return
	}
	metadata := current.Metadata()
	if service.store.CatalogCollectionCurrent(ctx, metadata.DigestSHA256, catalogCollectionRevision) {
		status := service.store.Status(ctx)
		service.store.RecordCatalogSuccess(
			ctx, bucket, time.Now().UTC(), metadata.ItemVersion, metadata.DigestSHA256,
			catalogCollectionRevision, status.CatalogDatasets,
		)
		service.finishCatalogCollection(ctx, time.Now().UTC(), nil)
		return
	}
	snapshots, err := buildOfficialCatalogSnapshots(current, bucket, settings.CollectorPlayerID)
	if err == nil {
		inserted := false
		for index, snapshot := range snapshots {
			var snapshotInserted bool
			snapshotInserted, err = service.store.EnqueueCatalog(ctx, snapshot)
			if err != nil {
				break
			}
			inserted = inserted || snapshotInserted
			service.setCatalogProgress(index + 1)
		}
		if err == nil {
			completedAt := time.Now().UTC()
			service.store.RecordCatalogSuccess(
				ctx, bucket, completedAt, metadata.ItemVersion, metadata.DigestSHA256,
				catalogCollectionRevision, len(snapshots),
			)
			if inserted {
				service.signal()
			}
		}
	}
	service.finishCatalogCollection(ctx, time.Now().UTC(), err)
}

func (service *DesktopService) finishCatalogCollection(ctx context.Context, now time.Time, err error) {
	service.collectionMu.Lock()
	service.collectionInProgress = false
	if err != nil {
		service.nextCollectionRetryAt = now.Add(scannerRetryWait)
	} else {
		service.nextCollectionRetryAt = time.Time{}
	}
	service.collectionMu.Unlock()
	if err != nil && ctx.Err() == nil {
		service.store.RecordCatalogError(ctx, err.Error())
	}
}

func (service *DesktopService) setCatalogProgress(datasets int) {
	service.collectionMu.Lock()
	service.collectedDatasets = datasets
	service.collectionMu.Unlock()
}

func collectorBucket(now time.Time, slot int, slots int) (time.Time, bool) {
	bucket := now.UTC().Truncate(captureBucket)
	if slots < 1 || slots > 8 || slot < 0 || slot >= slots {
		return bucket, false
	}
	index := bucket.Unix() / int64(captureBucket/time.Second)
	return bucket, int(index%int64(slots)) == slot
}

func nextCollectorSlot(now time.Time, slot int, slots int) time.Time {
	candidate := now.UTC().Truncate(captureBucket).Add(captureBucket)
	for attempts := 0; attempts < max(slots, 1); attempts++ {
		if _, due := collectorBucket(candidate, slot, slots); due {
			return candidate
		}
		candidate = candidate.Add(captureBucket)
	}
	return time.Time{}
}
