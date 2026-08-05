package Reports

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	reportPollInterval      = time.Second
	reportRetryDelay        = time.Minute
	reportArchiveQueueLimit = 256
)

type battleArchiveTask struct {
	messageID int64
	report    BattleReport
	capture   State.BattleReportCapture
	pvp       bool
}

type battleArchiveResult struct {
	messageID int64
	err       error
}

type Manager struct {
	state     *State.Store
	history   *History.Store
	analytics *SQLiteStore
	cloud     *CloudUploader
	intents   interface {
		Submit(context.Context, Intent.Request) Intent.Receipt
	}
	nextAttempt    map[int64]time.Time
	archived       map[int64]struct{}
	pending        map[int64]struct{}
	archiveQueue   chan battleArchiveTask
	archiveResults chan battleArchiveResult
	persistArchive func(context.Context, battleArchiveTask) error
	workers        sync.WaitGroup
	done           chan struct{}
	started        atomic.Bool
}

func NewManager(state *State.Store, history *History.Store, intents interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}, analytics ...*SQLiteStore) *Manager {
	manager := &Manager{
		state: state, history: history, intents: intents,
		nextAttempt: map[int64]time.Time{}, archived: map[int64]struct{}{}, pending: map[int64]struct{}{},
		archiveQueue:   make(chan battleArchiveTask, reportArchiveQueueLimit),
		archiveResults: make(chan battleArchiveResult, reportArchiveQueueLimit),
		done:           make(chan struct{}),
	}
	if len(analytics) > 0 {
		manager.analytics = analytics[0]
	}
	if manager.analytics != nil {
		manager.cloud = NewCloudUploader(state, history, manager.analytics, NewCloudClient(CloudConfig{}))
	}
	return manager
}

func (manager *Manager) CloudClient() *CloudClient {
	if manager == nil || manager.cloud == nil {
		return nil
	}
	return manager.cloud.Client()
}

func (manager *Manager) Run(ctx context.Context) {
	if manager == nil || manager.state == nil || manager.history == nil || manager.intents == nil {
		return
	}
	if !manager.started.CompareAndSwap(false, true) {
		return
	}
	defer close(manager.done)
	manager.loadArchivedMessages()
	if manager.analytics != nil {
		manager.workers.Add(1)
		go func() {
			defer manager.workers.Done()
			manager.runArchiveWriter(ctx)
		}()
	}
	if manager.cloud != nil {
		manager.workers.Add(1)
		go func() {
			defer manager.workers.Done()
			manager.cloud.Run(ctx)
		}()
	}
	defer manager.workers.Wait()
	ticker := time.NewTicker(reportPollInterval)
	defer ticker.Stop()
	manager.processNext(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case result := <-manager.archiveResults:
			manager.handleArchiveResult(result)
		case <-ticker.C:
			manager.processNext(ctx)
		}
	}
}

func (manager *Manager) processNext(ctx context.Context) {
	snapshot := manager.state.ReadOnlyView()
	notices := orderedNotices(snapshot.Reports.Notices)
	for _, notice := range notices {
		if _, pending := manager.pending[notice.MessageID]; pending {
			continue
		}
		if _, archived := manager.archived[notice.MessageID]; archived {
			if notice.Status != "archived" {
				manager.completeNotice(notice.MessageID)
				snapshot = manager.state.ReadOnlyView()
			}
			continue
		}
		if notice.Status == "archived" {
			if _, hasSpy := snapshot.Reports.SpyCaptures[notice.MessageID]; hasSpy {
				manager.completeNotice(notice.MessageID)
				snapshot = manager.state.ReadOnlyView()
			} else if _, hasBattle := snapshot.Reports.BattleCaptures[notice.MessageID]; hasBattle {
				manager.completeNotice(notice.MessageID)
				snapshot = manager.state.ReadOnlyView()
			}
			continue
		}
		if notice.Status == "expired" || notice.Status == "ignored" || notice.Status == "unavailable" {
			continue
		}
		if next := manager.nextAttempt[notice.MessageID]; !next.IsZero() && time.Now().Before(next) {
			continue
		}
		switch notice.TypeID {
		case 3:
			if capture, exists := snapshot.Reports.SpyCaptures[notice.MessageID]; exists {
				manager.archiveSpy(ctx, notice, capture)
				snapshot = manager.state.ReadOnlyView()
				continue
			}
			if !snapshot.Session.LoggedIn || !snapshot.Session.SocketReady {
				continue
			}
			manager.fetch(ctx, notice, "report.spy.fetch", map[string]any{"messageId": notice.MessageID})
			return
		case 6:
			if !strings.Contains(notice.BattleKey, "#") {
				manager.setNoticeStatus(notice.MessageID, "ignored")
				continue
			}
			capture := snapshot.Reports.BattleCaptures[notice.MessageID]
			if len(capture.Summary) > 0 && len(capture.Waves) > 0 && len(capture.Details) > 0 {
				manager.archiveBattle(ctx, snapshot, notice, capture)
				snapshot = manager.state.ReadOnlyView()
				continue
			}
			if !snapshot.Session.LoggedIn || !snapshot.Session.SocketReady {
				continue
			}
			if len(capture.Summary) == 0 {
				manager.fetch(ctx, notice, "report.battle.summary", map[string]any{"messageId": notice.MessageID})
				return
			}
			if capture.ReportID <= 0 {
				manager.setNoticeStatus(notice.MessageID, "error")
				manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
				return
			}
			manager.fetch(ctx, notice, "report.battle.details", map[string]any{
				"messageId": notice.MessageID, "reportId": capture.ReportID,
			})
			return
		default:
			manager.setNoticeStatus(notice.MessageID, "ignored")
			continue
		}
	}
}

func (manager *Manager) fetch(ctx context.Context, notice State.ReportNotice, name string, argumentsValue map[string]any) {
	arguments, _ := json.Marshal(argumentsValue)
	manager.setNoticeStatus(notice.MessageID, "fetching")
	receipt := manager.intents.Submit(ctx, Intent.Request{
		Name: name, Actor: "report-manager", Arguments: arguments,
	})
	if receipt.Status == Intent.StatusSucceeded {
		manager.setNoticeStatus(notice.MessageID, "pending")
		delete(manager.nextAttempt, notice.MessageID)
		return
	}
	status := "error"
	lowerError := strings.ToLower(receipt.Error)
	if strings.Contains(receipt.Error, "130") || strings.Contains(receipt.Error, "66") ||
		strings.Contains(lowerError, "unavailable") || strings.Contains(lowerError, "deleted") {
		status = "unavailable"
	}
	manager.setNoticeStatus(notice.MessageID, status)
	manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
}

func (manager *Manager) archiveSpy(ctx context.Context, notice State.ReportNotice, capture State.SpyReportCapture) {
	report, err := ParseSpyCapture(capture)
	if err != nil {
		manager.setNoticeStatus(notice.MessageID, "error")
		manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
		return
	}
	if err := manager.history.Append(History.CollectionSpyReports, report); err != nil {
		manager.setNoticeStatus(notice.MessageID, "error")
		manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
		return
	}
	manager.archived[notice.MessageID] = struct{}{}
	if notice.OwnedByPlayer && report.Status != "failed" && report.Castle.ID > 0 && report.Target.ID > 0 && report.Target.Dummy != nil && !*report.Target.Dummy {
		arguments, _ := json.Marshal(map[string]int64{"messageId": notice.MessageID})
		manager.intents.Submit(ctx, Intent.Request{
			Name: "report.spy.share", Actor: "report-manager", Arguments: arguments,
		})
	}
	manager.completeNotice(notice.MessageID)
}

func (manager *Manager) archiveBattle(ctx context.Context, snapshot State.GameState, notice State.ReportNotice, capture State.BattleReportCapture) {
	report, err := ParseBattleCapture(capture, snapshot.Player.ID)
	if err != nil {
		manager.setNoticeStatus(notice.MessageID, "error")
		manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
		return
	}
	report.AccountUID = snapshot.Account.UID
	report.WorldID = snapshot.Account.WorldID
	report.PlayerID = int64(snapshot.Player.ID)
	report = enrichBattleReportAllianceIDs(report, snapshot)
	if manager.analytics == nil {
		if err := manager.history.Append(History.CollectionBattleReports, report); err != nil {
			manager.setNoticeStatus(notice.MessageID, "error")
			manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
			return
		}
	} else {
		manager.enqueueBattleArchive(battleArchiveTask{
			messageID: notice.MessageID,
			report:    report,
			capture:   capture,
			pvp:       IsPvPBattleReport(report),
		})
		return
	}
	manager.archived[notice.MessageID] = struct{}{}
	manager.completeNotice(notice.MessageID)
}

func (manager *Manager) enqueueBattleArchive(task battleArchiveTask) {
	if _, pending := manager.pending[task.messageID]; pending {
		return
	}
	manager.pending[task.messageID] = struct{}{}
	manager.setNoticeStatus(task.messageID, "archiving")
	select {
	case manager.archiveQueue <- task:
	default:
		delete(manager.pending, task.messageID)
		manager.setNoticeStatus(task.messageID, "error")
		manager.nextAttempt[task.messageID] = time.Now().Add(reportRetryDelay)
	}
}

func (manager *Manager) runArchiveWriter(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-manager.archiveQueue:
			var err error
			if manager.persistArchive != nil {
				err = manager.persistArchive(ctx, task)
			} else if task.pvp {
				if manager.cloud == nil {
					err = context.Canceled
				} else {
					_, err = manager.cloud.Enqueue(ctx, task.report, task.capture)
				}
			} else {
				err = manager.analytics.Save(ctx, task.report)
			}
			select {
			case manager.archiveResults <- battleArchiveResult{messageID: task.messageID, err: err}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (manager *Manager) handleArchiveResult(result battleArchiveResult) {
	delete(manager.pending, result.messageID)
	if result.err != nil {
		manager.setNoticeStatus(result.messageID, "error")
		manager.nextAttempt[result.messageID] = time.Now().Add(reportRetryDelay)
		return
	}
	manager.archived[result.messageID] = struct{}{}
	manager.completeNotice(result.messageID)
}

func (manager *Manager) Wait() {
	if manager == nil || manager.done == nil {
		return
	}
	<-manager.done
}

func (manager *Manager) loadArchivedMessages() {
	for _, collection := range []string{History.CollectionSpyReports, History.CollectionBattleReports} {
		rows, err := manager.history.Read(collection, time.Time{}, 100_000)
		if err != nil {
			continue
		}
		for _, row := range rows {
			var report struct {
				MessageID int64 `json:"mid"`
			}
			if json.Unmarshal(row, &report) == nil && report.MessageID > 0 {
				manager.archived[report.MessageID] = struct{}{}
			}
		}
	}
	if manager.analytics != nil {
		snapshot := manager.state.ReadOnlyView()
		messageIDs, err := manager.analytics.ArchivedMessageIDs(context.Background(), BattleReportQuery{
			AccountUID: snapshot.Account.UID,
			WorldID:    snapshot.Account.WorldID,
			PlayerID:   int64(snapshot.Player.ID),
		})
		if err == nil {
			for _, messageID := range messageIDs {
				manager.archived[messageID] = struct{}{}
			}
		}
	}
}

func (manager *Manager) setNoticeStatus(messageID int64, status string) {
	_, _ = manager.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		notice, exists := gameState.Reports.Notices[messageID]
		if !exists || notice.Status == status {
			return nil, false, nil
		}
		if reportNoticeTerminal(notice.Status) {
			return nil, false, nil
		}
		notice.Status = status
		gameState.Reports.Notices[messageID] = notice
		return []string{"reports"}, true, nil
	})
}

func reportNoticeTerminal(status string) bool {
	switch status {
	case "archived", "expired", "ignored", "unavailable":
		return true
	default:
		return false
	}
}

func (manager *Manager) completeNotice(messageID int64) {
	_, _ = manager.state.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		notice, exists := gameState.Reports.Notices[messageID]
		if !exists {
			return nil, false, nil
		}
		notice.Status = "archived"
		gameState.Reports.Notices[messageID] = notice
		delete(gameState.Reports.SpyCaptures, messageID)
		capture := gameState.Reports.BattleCaptures[messageID]
		delete(gameState.Reports.BattleCaptures, messageID)
		if capture.ReportID > 0 && gameState.Reports.ActiveBattleReport == capture.ReportID {
			gameState.Reports.ActiveBattleReport = 0
		}
		return []string{"reports"}, true, nil
	})
	delete(manager.nextAttempt, messageID)
}

func orderedNotices(notices map[int64]State.ReportNotice) []State.ReportNotice {
	result := make([]State.ReportNotice, 0, len(notices))
	for _, notice := range notices {
		result = append(result, notice)
	}
	sort.Slice(result, func(left, right int) bool {
		if !result[left].ObservedAt.Equal(result[right].ObservedAt) {
			return result[left].ObservedAt.Before(result[right].ObservedAt)
		}
		return result[left].MessageID < result[right].MessageID
	})
	return result
}
