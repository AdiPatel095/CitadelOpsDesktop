package ResponseRegistry

import (
	"strings"
	"sync"
	"sync/atomic"

	"CitadelDesktop/Server/Logging"
)

const maxPendingAppSendOpcodes = 1024

var (
	appSendPendingMu sync.Mutex
	appSendPending   []string

	// pendingSkipRiftCRACapture counts Citadel-queued **cra** frames so outbound capture skips replays.
	pendingSkipRiftCRACapture int32
)

// effectiveWireOpcode matches GameParser.MessageRouter: EmpireEx frames use the opcode at index 3;
// other frames (e.g. lli) use index 2.
func effectiveWireOpcode(parts []string) string {
	if len(parts) <= 2 {
		return ""
	}
	cmd := parts[2]
	if strings.HasPrefix(cmd, "EmpireEx_") {
		if len(parts) > 3 {
			return parts[3]
		}
		return ""
	}
	return cmd
}

// logAppOutboundPayload records a Citadel-queued outbound frame on the app_send channel and
// remembers its opcode so the next matching inbound frame can be correlated.
func logAppOutboundPayload(payload string) {
	parts := strings.Split(payload, "%")
	op := effectiveWireOpcode(parts)
	if op == "" {
		op = "UNKNOWN"
	}

	appSendPendingMu.Lock()
	appSendPending = append(appSendPending, op)
	if len(appSendPending) > maxPendingAppSendOpcodes {
		// Drop oldest to bound memory if something goes wrong.
		appSendPending = appSendPending[len(appSendPending)-maxPendingAppSendOpcodes:]
	}
	appSendPendingMu.Unlock()

	Logging.AppendChannelLine(Logging.ChannelAppSend, "SEND", op, payload)
	if op == "cra" {
		MarkAppOutboundCRACaptureSkip()
	}
}

// MarkAppOutboundCRACaptureSkip records one Citadel-queued **cra** so Rift capture ignores the mirrored SEND hook.
func MarkAppOutboundCRACaptureSkip() {
	atomic.AddInt32(&pendingSkipRiftCRACapture, 1)
}

// TryConsumeAppOutboundCRACaptureSkip returns true when the next outbound **cra** should not be re-captured.
func TryConsumeAppOutboundCRACaptureSkip() bool {
	for {
		n := atomic.LoadInt32(&pendingSkipRiftCRACapture)
		if n <= 0 {
			return false
		}
		if atomic.CompareAndSwapInt32(&pendingSkipRiftCRACapture, n, n-1) {
			return true
		}
	}
}

// LogIncomingGameWireParts mirrors inbound frames to log channels (Game WebSocket RECV + app_send MATCH).
// Call from MessageRouter (and for frames too short for the router, from incomingMessageParserStartup).
func LogIncomingGameWireParts(messageParts []string) {
	if len(messageParts) == 0 {
		return
	}
	payload := strings.Join(messageParts, "%")
	msgType := gameRecvLogCmdType(messageParts)
	Logging.AppendChannelLine(Logging.ChannelWebSocketGame, "RECV", msgType, payload)
	logAppInboundIfMatched(payload)
}

func gameRecvLogCmdType(parts []string) string {
	if len(parts) > 2 {
		return parts[2]
	}
	return "UNKNOWN"
}

// logAppInboundIfMatched appends to the app_send channel when the game returns a frame whose
// opcode matches the next pending opcode from our outbound lanes (FIFO by actual send order).
func logAppInboundIfMatched(payload string) {
	parts := strings.Split(payload, "%")
	inOp := effectiveWireOpcode(parts)
	if inOp == "" {
		return
	}

	appSendPendingMu.Lock()
	defer appSendPendingMu.Unlock()
	if len(appSendPending) == 0 {
		return
	}
	if appSendPending[0] != inOp {
		return
	}
	appSendPending = appSendPending[1:]
	Logging.AppendChannelLine(Logging.ChannelAppSend, "MATCH", inOp, payload)
}
