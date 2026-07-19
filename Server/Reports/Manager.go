package Reports

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	reportPollInterval = time.Second
	reportRetryDelay   = time.Minute
)

type Manager struct {
	state   *State.Store
	history *History.Store
	intents interface {
		Submit(context.Context, Intent.Request) Intent.Receipt
	}
	nextAttempt map[int64]time.Time
	started     atomic.Bool
}

func NewManager(state *State.Store, history *History.Store, intents interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}) *Manager {
	return &Manager{state: state, history: history, intents: intents, nextAttempt: map[int64]time.Time{}}
}

func (manager *Manager) Run(ctx context.Context) {
	if manager == nil || manager.state == nil || manager.history == nil || manager.intents == nil {
		return
	}
	if !manager.started.CompareAndSwap(false, true) {
		return
	}
	ticker := time.NewTicker(reportPollInterval)
	defer ticker.Stop()
	manager.processNext(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			manager.processNext(ctx)
		}
	}
}

func (manager *Manager) processNext(ctx context.Context) {
	snapshot := manager.state.Snapshot()
	notices := orderedNotices(snapshot.Reports.Notices)
	for _, notice := range notices {
		if notice.Status == "archived" {
			if _, hasSpy := snapshot.Reports.SpyCaptures[notice.MessageID]; hasSpy {
				manager.completeNotice(notice.MessageID)
				snapshot = manager.state.Snapshot()
			} else if _, hasBattle := snapshot.Reports.BattleCaptures[notice.MessageID]; hasBattle {
				manager.completeNotice(notice.MessageID)
				snapshot = manager.state.Snapshot()
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
				snapshot = manager.state.Snapshot()
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
				manager.archiveBattle(snapshot.Player.ID, notice, capture)
				snapshot = manager.state.Snapshot()
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
	if strings.Contains(receipt.Error, "130") || strings.Contains(receipt.Error, "unavailable") {
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
	if notice.OwnedByPlayer && report.Status != "failed" && report.Castle.ID > 0 && report.Target.ID > 0 && report.Target.Dummy != nil && !*report.Target.Dummy {
		arguments, _ := json.Marshal(map[string]int64{"messageId": notice.MessageID})
		manager.intents.Submit(ctx, Intent.Request{
			Name: "report.spy.share", Actor: "report-manager", Arguments: arguments,
		})
	}
	manager.completeNotice(notice.MessageID)
}

func (manager *Manager) archiveBattle(playerID State.PlayerID, notice State.ReportNotice, capture State.BattleReportCapture) {
	report, err := ParseBattleCapture(capture, playerID)
	if err != nil {
		manager.setNoticeStatus(notice.MessageID, "error")
		manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
		return
	}
	if err := manager.history.Append(History.CollectionBattleReports, report); err != nil {
		manager.setNoticeStatus(notice.MessageID, "error")
		manager.nextAttempt[notice.MessageID] = time.Now().Add(reportRetryDelay)
		return
	}
	manager.completeNotice(notice.MessageID)
}

func (manager *Manager) setNoticeStatus(messageID int64, status string) {
	_, _ = manager.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		notice, exists := gameState.Reports.Notices[messageID]
		if !exists || notice.Status == status {
			return nil, false, nil
		}
		notice.Status = status
		gameState.Reports.Notices[messageID] = notice
		return []string{"reports"}, true, nil
	})
}

func (manager *Manager) completeNotice(messageID int64) {
	_, _ = manager.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
