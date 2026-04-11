package GameParser

import (
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/GameCommands"
)

// lastGAMParsedNs advances after each full GAM parse (see MovementParser).
var lastGAMParsedNs int64

func markGAMParsed() {
	atomic.StoreInt64(&lastGAMParsedNs, time.Now().UnixNano())
}

// AwaitNextGAMAfter waits until markGAMParsed() runs after prevNs (typically after SendGAM).
func AwaitNextGAMAfter(prevNs int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&lastGAMParsedNs) > prevNs {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// SendGAMAndWait sends **gam** and waits until movements are parsed into GameState.
func SendGAMAndWait(timeout time.Duration) bool {
	prev := atomic.LoadInt64(&lastGAMParsedNs)
	GameCommands.SendGAM()
	return AwaitNextGAMAfter(prev, timeout)
}
