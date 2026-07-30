package Reports

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/State"
)

const cloudReportRetryInterval = 30 * time.Second

type CloudOutboxBackfill struct {
	Queued  int
	Skipped int
}

type CloudUploader struct {
	state   *State.Store
	history *History.Store
	store   *SQLiteStore
	client  *CloudClient
	wake    chan struct{}
	started atomic.Bool
}

func NewCloudUploader(
	state *State.Store,
	history *History.Store,
	store *SQLiteStore,
	client *CloudClient,
) *CloudUploader {
	return &CloudUploader{
		state: state, history: history, store: store, client: client,
		wake: make(chan struct{}, 1),
	}
}

func (uploader *CloudUploader) Client() *CloudClient {
	if uploader == nil {
		return nil
	}
	return uploader.client
}

func (uploader *CloudUploader) Enqueue(
	ctx context.Context,
	report BattleReport,
	capture State.BattleReportCapture,
) (bool, error) {
	if uploader == nil || uploader.store == nil {
		return false, fmt.Errorf("cloud battle report uploader is unavailable")
	}
	if uploader.state != nil {
		report = enrichBattleReportAllianceIDs(report, uploader.state.ReadOnlyView())
	}
	envelope, eligible, err := buildCloudEnvelopeFromCapture(report, capture)
	if err != nil || !eligible {
		return eligible, err
	}
	if err := uploader.store.QueueCloudReport(ctx, envelope); err != nil {
		return true, err
	}
	uploader.signal()
	return true, nil
}

func (uploader *CloudUploader) Run(ctx context.Context) {
	if uploader == nil || uploader.store == nil || uploader.client == nil {
		return
	}
	if !uploader.started.CompareAndSwap(false, true) {
		return
	}
	ticker := time.NewTicker(cloudReportRetryInterval)
	defer ticker.Stop()
	for {
		processed, err := uploader.processNext(ctx)
		if err != nil {
			log.Printf("[battle-report] cloud outbox retry pending: %v", err)
		}
		if processed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(250 * time.Millisecond):
				continue
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-uploader.wake:
		case <-ticker.C:
		}
	}
}

func (uploader *CloudUploader) processNext(ctx context.Context) (bool, error) {
	pending, err := uploader.store.PendingCloudReports(ctx, cloudReportBatchMax)
	if err != nil || len(pending) == 0 {
		return false, err
	}
	remote, err := uploader.client.RemoteLIDs(ctx)
	if err != nil {
		return false, err
	}
	if confirmed := confirmedCloudLIDs(pending, remote); len(confirmed) > 0 {
		if err := uploader.finalizeConfirmed(ctx, confirmed); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := uploader.client.Upload(ctx, pending); err != nil {
		return false, err
	}
	remote, err = uploader.client.RemoteLIDs(ctx)
	if err != nil {
		return false, fmt.Errorf("confirm uploaded cloud battle reports: %w", err)
	}
	confirmed := confirmedCloudLIDs(pending, remote)
	if len(confirmed) == 0 {
		return false, fmt.Errorf("uploaded battle reports are not visible in cloud yet")
	}
	if err := uploader.finalizeConfirmed(ctx, confirmed); err != nil {
		return false, err
	}
	return true, nil
}

func (uploader *CloudUploader) finalizeConfirmed(ctx context.Context, lids []int64) error {
	if _, err := PurgeBattleHistoryByLIDs(uploader.history, lids); err != nil {
		return err
	}
	if err := uploader.store.DeleteCloudReports(ctx, lids); err != nil {
		return err
	}
	return nil
}

func (uploader *CloudUploader) signal() {
	if uploader == nil {
		return
	}
	select {
	case uploader.wake <- struct{}{}:
	default:
	}
}

func BackfillCloudOutbox(
	ctx context.Context,
	history *History.Store,
	store *SQLiteStore,
	snapshot State.GameState,
) (CloudOutboxBackfill, error) {
	result := CloudOutboxBackfill{}
	if history == nil || store == nil {
		return result, nil
	}
	rows, err := history.Read(History.CollectionBattleReports, time.Time{}, battleReportAnalyticsLimit)
	if err != nil {
		return result, fmt.Errorf("read battle history for cloud outbox: %w", err)
	}
	if len(rows) >= battleReportAnalyticsLimit {
		return result, fmt.Errorf("battle history reached the %d-row safe cloud backfill limit", battleReportAnalyticsLimit)
	}
	queued := make(map[int64]struct{})
	for _, row := range rows {
		var report BattleReport
		if json.Unmarshal(row, &report) != nil || !IsPvPBattleReport(report) {
			result.Skipped++
			continue
		}
		report = enrichBattleReportAllianceIDs(report, snapshot)
		envelope, eligible, err := buildCloudEnvelopeFromReport(report)
		if err != nil {
			return CloudOutboxBackfill{}, fmt.Errorf("queue historical PvP report LID %d: %w", report.LID, err)
		}
		if !eligible {
			result.Skipped++
			continue
		}
		if err := store.QueueCloudReport(ctx, envelope); err != nil {
			return CloudOutboxBackfill{}, err
		}
		queued[report.LID] = struct{}{}
	}
	result.Queued = len(queued)
	return result, nil
}

func confirmedCloudLIDs(
	reports []cloudBattleReportEnvelope,
	remote map[int64]struct{},
) []int64 {
	result := make([]int64, 0, len(reports))
	for _, report := range reports {
		if _, exists := remote[report.LID]; exists {
			result = append(result, report.LID)
		}
	}
	return result
}

func enrichBattleReportAllianceIDs(report BattleReport, snapshot State.GameState) BattleReport {
	alliances := make(map[State.AllianceID]State.AllianceState, len(snapshot.Alliances)+1)
	for id, alliance := range snapshot.Alliances {
		alliances[id] = alliance
	}
	if snapshot.Alliance.ID > 0 {
		alliances[snapshot.Alliance.ID] = snapshot.Alliance
	}
	byName := make(map[string]State.AllianceID, len(alliances))
	for id, alliance := range alliances {
		name := strings.ToLower(strings.TrimSpace(alliance.Name))
		if id <= 0 || name == "" {
			continue
		}
		if existing, found := byName[name]; found && existing != id {
			byName[name] = 0
			continue
		}
		byName[name] = id
	}
	resolve := func(combatant *BattleCombatant) *BattleCombatant {
		if combatant == nil {
			return nil
		}
		copyValue := *combatant
		if copyValue.AllianceID > 0 {
			return &copyValue
		}
		if State.PlayerID(copyValue.PlayerID) == snapshot.Player.ID {
			copyValue.AllianceID = int64(snapshot.Player.AllianceID)
			if copyValue.AllianceID <= 0 {
				copyValue.AllianceID = int64(snapshot.Alliance.ID)
			}
		}
		if copyValue.AllianceID <= 0 {
			for id, alliance := range alliances {
				for _, member := range alliance.Members {
					if int64(member.PlayerID) == copyValue.PlayerID {
						copyValue.AllianceID = int64(id)
						break
					}
				}
				if copyValue.AllianceID > 0 {
					break
				}
			}
		}
		if copyValue.AllianceID <= 0 {
			copyValue.AllianceID = int64(byName[strings.ToLower(strings.TrimSpace(copyValue.Alliance))])
		}
		return &copyValue
	}
	report.Attacker = resolve(report.Attacker)
	report.Defender = resolve(report.Defender)
	return report
}
