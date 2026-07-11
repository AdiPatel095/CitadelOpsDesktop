package GameParser

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
	reportnotice "CitadelDesktop/Server/Models/ReportNotice"
	spyreport "CitadelDesktop/Server/Models/SpyReport"
	"CitadelDesktop/Server/ResponseRegistry"
)

var (
	spyReportQueueOnce sync.Once
	spyReportQueue     = make(chan spyreport.Notice, 128)
	spyReportQueuedMu  sync.Mutex
	spyReportQueued    = map[int64]time.Time{}
)

func HandleSNESpyReports(payload string) {
	notices, err := spyreport.NoticesFromSNEPayload(payload)
	if err != nil {
		log.Printf("[spy-report] sne parse: %v", err)
		return
	}
	for _, notice := range notices {
		notice.AutoShare = spyNoticeOwnedByCurrentPlayer(notice)
		queueSpyReportFetch(notice)
	}
}

func HandleLoginInboxSpyReports(gbd map[string]interface{}) {
	sne, ok := gbd["sne"].(map[string]interface{})
	if !ok {
		return
	}
	payload, err := json.Marshal(sne)
	if err != nil {
		return
	}
	notices, err := spyreport.NoticesFromSNEPayload(string(payload))
	if err != nil {
		return
	}
	for _, notice := range notices {
		notice.AutoShare = false
		queueSpyReportFetch(notice)
	}
}

func queueSpyReportFetch(notice spyreport.Notice) {
	if notice.MID <= 0 || !reportnotice.IsSpyFetchableRow(notice.SNERow) {
		return
	}
	spyReportQueueOnce.Do(func() { go spyReportFetchWorker() })
	now := time.Now()
	spyReportQueuedMu.Lock()
	if previous, ok := spyReportQueued[notice.MID]; ok && now.Sub(previous) < 2*time.Minute {
		spyReportQueuedMu.Unlock()
		return
	}
	spyReportQueued[notice.MID] = now
	for mid, queuedAt := range spyReportQueued {
		if now.Sub(queuedAt) > 15*time.Minute {
			delete(spyReportQueued, mid)
		}
	}
	spyReportQueuedMu.Unlock()
	select {
	case spyReportQueue <- notice:
	default:
		log.Printf("[spy-report] fetch queue full; dropped MID %d", notice.MID)
	}
}

func spyReportFetchWorker() {
	for notice := range spyReportQueue {
		waiter := ResponseRegistry.Global.RegisterWaiter("bsd", battleReportWireWait)
		GameCommands.QueueBackgroundPayload(GameCommands.BSDPayload(notice.MID))
		parts, err := waiter.WaitWithTimeout()
		waiter.Cleanup()
		if err != nil {
			log.Printf("[spy-report] fetch MID %d: %v", notice.MID, err)
			continue
		}
		payload, err := reportResponsePayload(parts)
		if err != nil {
			log.Printf("[spy-report] fetch MID %d: %v", notice.MID, err)
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(payload))
		decoder.UseNumber()
		var bsd map[string]interface{}
		if err := decoder.Decode(&bsd); err != nil {
			log.Printf("[spy-report] fetch MID %d: %v", notice.MID, err)
			continue
		}
		capture := spyreport.Capture{Version: 1, MID: notice.MID, BattleKey: notice.BattleKey, SNERow: notice.SNERow, CapturedAtUnixMillis: time.Now().UnixMilli(), BSD: bsd}
		if err := spyreport.UpsertCapture(capture); err != nil {
			log.Printf("[spy-report] save MID %d: %v", notice.MID, err)
			continue
		}
		members := Models.GetGameState().Alliance.Members
		memberIDs := make([]int, 0, len(members))
		for _, member := range members {
			if member.PlayerID > 0 {
				memberIDs = append(memberIDs, member.PlayerID)
			}
		}
		parsed := spyreport.ParseCapture(capture)
		if notice.AutoShare && parsed.Status != "failed" && spyreport.IsPlayerCastleTarget(capture) {
			recipients := allianceShareRecipients(members, Models.GetGameState().PlayerID)
			if len(recipients) > 0 {
				GameCommands.QueueBackgroundPayload(GameCommands.MFSPayload(capture.MID, recipients))
				log.Printf("[spy-report] shared MID %d with %d alliance members", capture.MID, len(recipients))
			}
		}
		go func(saved spyreport.Capture, roster []int) {
			if err := spyreport.UploadCaptureToCloud(saved, roster); err != nil {
				log.Printf("[spy-report] cloud upload MID %d: %v", saved.MID, err)
			}
		}(capture, memberIDs)
		log.Printf("[spy-report] captured MID %d", notice.MID)
	}
}

func allianceShareRecipients(members []Models.AllianceMember, currentPlayerID int) []int {
	recipients := make([]int, 0, len(members))
	seen := make(map[int]struct{}, len(members))
	for _, member := range members {
		playerID := member.PlayerID
		if playerID <= 0 || playerID == currentPlayerID {
			continue
		}
		if _, exists := seen[playerID]; exists {
			continue
		}
		seen[playerID] = struct{}{}
		recipients = append(recipients, playerID)
	}
	return recipients
}

func spyNoticeOwnedByCurrentPlayer(notice spyreport.Notice) bool {
	if len(notice.SNERow) <= 4 {
		return true
	}
	switch value := notice.SNERow[4].(type) {
	case json.Number:
		id, _ := value.Int64()
		return id <= 0
	case float64:
		return value <= 0
	case int:
		return value <= 0
	case int64:
		return value <= 0
	}
	return false
}
