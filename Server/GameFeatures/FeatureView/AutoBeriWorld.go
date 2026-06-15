package featureview

import (
	"context"
	"sync"
	"time"

	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
)

var (
	autoBeriWorldCancel     context.CancelFunc
	autoBeriWorldMu         sync.Mutex
	autoBeriWorldNextWakeUp int64 // unix ms, 0 if unknown
)

const (
	autoBeriWorldDefaultCheckSec = 30
	autoBeriWorldMinCheckSec     = 5
)

// IsAutoBeriWorldRunning reports whether the Auto Beri World loop is active.
func IsAutoBeriWorldRunning() bool {
	autoBeriWorldMu.Lock()
	defer autoBeriWorldMu.Unlock()
	return autoBeriWorldCancel != nil
}

// GetAutoBeriWorldNextWakeUp returns the next pass time in unix milliseconds (0 if not sleeping / unknown).
func GetAutoBeriWorldNextWakeUp() int64 {
	autoBeriWorldMu.Lock()
	defer autoBeriWorldMu.Unlock()
	return autoBeriWorldNextWakeUp
}

func setAutoBeriWorldNextWake(t time.Time) {
	autoBeriWorldMu.Lock()
	autoBeriWorldNextWakeUp = t.UnixMilli()
	next := autoBeriWorldNextWakeUp
	autoBeriWorldMu.Unlock()
	if ResponseRegistry.SendAutoBeriWorldStatusFunc != nil {
		go ResponseRegistry.SendAutoBeriWorldStatusFunc(true, next)
	}
}

// StartAutoBeriWorld starts the Auto Beri World goroutine (no-op if already running).
func StartAutoBeriWorld() {
	autoBeriWorldMu.Lock()
	defer autoBeriWorldMu.Unlock()
	if autoBeriWorldCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	autoBeriWorldCancel = cancel
	SyncBeriCastleFromSettings()
	SyncKutSourceFromMainCastle()
	go runAutoBeriWorld(ctx)
}

// StopAutoBeriWorld stops the Auto Beri World loop.
func StopAutoBeriWorld() {
	autoBeriWorldMu.Lock()
	defer autoBeriWorldMu.Unlock()
	if autoBeriWorldCancel != nil {
		autoBeriWorldCancel()
		autoBeriWorldCancel = nil
	}
	autoBeriWorldNextWakeUp = 0
	if ResponseRegistry.SendAutoBeriWorldStatusFunc != nil {
		go ResponseRegistry.SendAutoBeriWorldStatusFunc(false, 0)
	}
}

func autoBeriWorldCheckInterval() time.Duration {
	cfg := Models.GetSettingsState().AutoBeriWorld
	sec := cfg.TroopSpaceCheckIntervalSec
	if sec < autoBeriWorldMinCheckSec {
		sec = autoBeriWorldDefaultCheckSec
	}
	return time.Duration(sec) * time.Second
}

func runAutoBeriWorld(ctx context.Context) {
	Logging.AutoBeriWorldLog("started", "")
	defer Logging.AutoBeriWorldLog("stopped", "")

	for {
		if !EnsureGameSessionOrReload(ctx) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(autoBeriWorldCheckInterval()):
				continue
			}
		}

		runAutoBeriWorldPass(ctx)

		next := time.Now().Add(autoBeriWorldCheckInterval())
		setAutoBeriWorldNextWake(next)
		Logging.AutoBeriWorldLogf("sleep", "next check in %s at %s", autoBeriWorldCheckInterval(), next.Format(time.RFC3339))

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
		}
	}
}

// runAutoBeriWorldPass runs the troop-space check and kut/msk transfer (no JAA focus).
func runAutoBeriWorldPass(ctx context.Context) {
	if beriCID, _, _, ok := ResolveBeriCastle(); ok {
		Logging.AutoBeriWorldLogf("pass", "beri CID=%d (settings)", beriCID)
	} else {
		Logging.AutoBeriWorldLog("pass", "beri CID unknown — set Beri castle CID in settings")
	}
	maybeTransferBeriTroops(ctx)
}
