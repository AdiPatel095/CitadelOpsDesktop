package GameParser

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	battlereport "CitadelDesktop/Server/Models/BattleReport"
	"CitadelDesktop/Server/ResponseRegistry"
)

const battleReportWireWait = 12 * time.Second

var (
	battleReportFetchOnce  sync.Once
	battleReportFetchQueue = make(chan battlereport.Capture, 128)
	battleReportQueuedMu   sync.Mutex
	battleReportQueued     = map[string]time.Time{}
)

func HandleSNESharedBattleReports(payload string) {
	captures, err := battlereport.RecordSNEPayload(payload)
	if err != nil {
		log.Printf("[battle-report] sne parse: %v", err)
		return
	}
	for _, capture := range captures {
		queueBattleReportFetch(capture)
	}
}

func queueBattleReportFetch(capture battlereport.Capture) {
	if capture.MID <= 0 {
		return
	}
	startBattleReportFetchWorker()

	key := capture.ID
	if key == "" {
		key = fmt.Sprintf("%d-%d", capture.MID, capture.LID)
	}
	now := time.Now()
	battleReportQueuedMu.Lock()
	if last, ok := battleReportQueued[key]; ok && now.Sub(last) < 2*time.Minute {
		battleReportQueuedMu.Unlock()
		return
	}
	battleReportQueued[key] = now
	for queuedKey, queuedAt := range battleReportQueued {
		if now.Sub(queuedAt) > 15*time.Minute {
			delete(battleReportQueued, queuedKey)
		}
	}
	battleReportQueuedMu.Unlock()

	select {
	case battleReportFetchQueue <- capture:
	default:
		go func() {
			select {
			case battleReportFetchQueue <- capture:
			case <-time.After(5 * time.Second):
				log.Printf("[battle-report] fetch queue full; dropped MID %d", capture.MID)
			}
		}()
	}
}

func startBattleReportFetchWorker() {
	battleReportFetchOnce.Do(func() {
		go battleReportFetchWorker()
	})
}

func battleReportFetchWorker() {
	for capture := range battleReportFetchQueue {
		if err := fetchBattleReportCapture(capture); err != nil {
			log.Printf("[battle-report] fetch MID %d LID %d: %v", capture.MID, capture.LID, err)
		}
	}
}

func fetchBattleReportCapture(capture battlereport.Capture) error {
	if capture.Wire == nil {
		capture.Wire = map[string]string{}
	}

	var failures []string
	if bls, wire, err := requestBattleReportPayload("bls", func() {
		GameCommands.SendBLS(capture.MID, 0)
	}); err != nil {
		failures = append(failures, fmt.Sprintf("bls: %v", err))
	} else {
		capture.BLS = bls
		setBattleReportWire(&capture, "bls", wire)
		if lid := battleReportLID(bls); lid > 0 {
			capture.LID = lid
			capture.ID = fmt.Sprintf("%d-%d", capture.MID, capture.LID)
		}
		parsed := battlereport.ParseCapture(&capture)
		if parsed.ID != "" && !battlereport.ReportHasBothPlayers(parsed) {
			if err := battlereport.DeleteCapture(capture); err != nil {
				log.Printf("[battle-report] discard local cleanup %s: %v", capture.ID, err)
			}
			log.Printf("[battle-report] discarded %s without parsed players on both sides", capture.ID)
			return nil
		}
		persistBattleReportStage(capture, "bls")
	}

	if capture.LID <= 0 {
		failures = append(failures, "missing LID")
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}

	if blm, wire, err := requestBattleReportPayload("blm", func() {
		GameCommands.SendBLM(capture.LID)
	}); err != nil {
		failures = append(failures, fmt.Sprintf("blm: %v", err))
	} else {
		capture.BLM = blm
		setBattleReportWire(&capture, "blm", wire)
		persistBattleReportStage(capture, "blm")
	}

	if bld, wire, err := requestBattleReportPayload("bld", func() {
		GameCommands.SendBLD(capture.LID)
	}); err != nil {
		failures = append(failures, fmt.Sprintf("bld: %v", err))
	} else {
		capture.BLD = bld
		setBattleReportWire(&capture, "bld", wire)
		persistBattleReportStage(capture, "bld")
	}

	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	log.Printf("[battle-report] captured and uploaded %s", capture.ID)
	return nil
}

func requestBattleReportPayload(command string, send func()) (map[string]interface{}, string, error) {
	waiter := ResponseRegistry.Global.RegisterWaiter(strings.ToLower(command), battleReportWireWait)
	defer waiter.Cleanup()

	send()
	parts, err := waiter.WaitWithTimeout()
	if err != nil {
		return nil, "", err
	}
	wire := strings.Join(parts, "%")
	payload, ok := Payload(parts)
	if !ok {
		return nil, wire, fmt.Errorf("missing payload")
	}

	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	var data map[string]interface{}
	if err := decoder.Decode(&data); err != nil {
		return nil, wire, err
	}
	return data, wire, nil
}

func persistBattleReportStage(capture battlereport.Capture, stage string) {
	if err := battlereport.UpsertCapture(capture); err != nil {
		log.Printf("[battle-report] local save %s %s: %v", stage, capture.ID, err)
		return
	}
	if err := battlereport.UploadCaptureToCloud(capture); err != nil {
		log.Printf("[battle-report] cloud upload %s %s: %v", stage, capture.ID, err)
		return
	}
	log.Printf("[battle-report] cloud uploaded %s %s", stage, capture.ID)
}

func setBattleReportWire(capture *battlereport.Capture, key, wire string) {
	if wire == "" {
		return
	}
	if capture.Wire == nil {
		capture.Wire = map[string]string{}
	}
	capture.Wire[strings.ToLower(key)] = wire
}

func battleReportLID(bls map[string]interface{}) int64 {
	if lid, ok := battleReportInt64(bls["LID"]); ok && lid > 0 {
		return lid
	}
	if ai, ok := bls["AI"].(map[string]interface{}); ok {
		if lid, ok := battleReportInt64(ai["LID"]); ok && lid > 0 {
			return lid
		}
	}
	return 0
}

func battleReportInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		return int64(v), true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}
