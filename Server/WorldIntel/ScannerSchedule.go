package WorldIntel

import (
	"context"
	"fmt"
	"time"
)

func (service *DesktopService) runScanner(ctx context.Context) {
	ticker := time.NewTicker(scannerPoll)
	defer ticker.Stop()
	service.tryScheduledScan(ctx, time.Now().UTC())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			service.tryScheduledScan(ctx, now.UTC())
		}
	}
}

func (service *DesktopService) tryScheduledScan(ctx context.Context, now time.Time) {
	if service == nil || service.store == nil || service.state == nil || service.intents == nil {
		return
	}
	settings := service.Settings()
	snapshot := service.state.Snapshot()
	collectorPlayerID := int64(snapshot.Player.ID)
	if !settings.collectsFor(collectorPlayerID) {
		return
	}
	bucket, due := collectorBucket(now, settings.CollectorSlot, settings.CollectorSlots)
	if !due {
		return
	}
	if lastBucket, found := service.store.LastScanBucket(ctx); found && lastBucket.Equal(bucket) {
		return
	}
	service.scanMu.Lock()
	if service.scanInProgress || now.Before(service.nextScanRetryAt) {
		service.scanMu.Unlock()
		return
	}
	service.scanInProgress = true
	service.scannedPlayers = 0
	service.scanMu.Unlock()

	worldID := NormalizeWorldID(snapshot.Account.WorldID)
	if worldID == "" {
		worldID = NormalizeWorldID(snapshot.Session.ServerURL)
	}
	if worldID == "" {
		service.finishScanAttempt(ctx, now, fmt.Errorf("connected game world is unavailable"))
		return
	}
	if err := service.requireCollectorSession(worldID, collectorPlayerID); err != nil {
		service.finishScanAttempt(ctx, now, err)
		return
	}

	scanContext, cancel := context.WithTimeout(ctx, scannerTimeout)
	batches, players, err := service.scanPublicLeaderboards(scanContext, worldID, collectorPlayerID, bucket)
	cancel()
	service.setScanProgress(players)
	if err == nil {
		inserted := false
		for _, batch := range batches {
			var batchInserted bool
			batchInserted, err = service.store.Enqueue(ctx, batch)
			if err != nil {
				break
			}
			inserted = inserted || batchInserted
		}
		if err == nil {
			completedAt := time.Now().UTC()
			service.store.RecordScanSuccess(ctx, bucket, completedAt)
			if inserted {
				service.signal()
			}
		}
	}
	service.finishScanAttempt(ctx, time.Now().UTC(), err)
}

func (service *DesktopService) finishScanAttempt(ctx context.Context, now time.Time, err error) {
	service.scanMu.Lock()
	service.scanInProgress = false
	if err != nil {
		service.nextScanRetryAt = now.Add(scannerRetryWait)
	} else {
		service.nextScanRetryAt = time.Time{}
	}
	service.scanMu.Unlock()
	if err != nil && ctx.Err() == nil {
		service.store.RecordScanError(ctx, err.Error())
	}
}

func (service *DesktopService) setScanProgress(players int) {
	if service == nil {
		return
	}
	service.scanMu.Lock()
	service.scannedPlayers = players
	service.scanMu.Unlock()
}

func (service *DesktopService) requireCollectorSession(worldID string, collectorPlayerID int64) error {
	if service == nil || service.state == nil {
		return fmt.Errorf("game state is unavailable")
	}
	snapshot := service.state.Snapshot()
	if !snapshot.Session.LoggedIn || !snapshot.Session.SocketReady ||
		snapshot.Session.Generation != snapshot.Session.BaselineGeneration {
		return fmt.Errorf("collector session is not connected with an authoritative baseline")
	}
	if int64(snapshot.Player.ID) != collectorPlayerID || !service.Settings().collectsFor(collectorPlayerID) {
		return fmt.Errorf("collector account assignment changed during the scan")
	}
	currentWorldID := NormalizeWorldID(snapshot.Account.WorldID)
	if currentWorldID == "" {
		currentWorldID = NormalizeWorldID(snapshot.Session.ServerURL)
	}
	if currentWorldID != NormalizeWorldID(worldID) {
		return fmt.Errorf("collector game world changed during the scan")
	}
	return nil
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
