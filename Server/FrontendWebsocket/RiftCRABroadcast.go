package FrontendWebsocket

import (
	"sync"

	"CitadelDesktop/Server/GameParser"
)

var (
	riftCRABroadcastCoalesceMu sync.Mutex
	riftCRABroadcastPending    bool
)

// ScheduleSendRiftCRALaunch coalesces bursty refresh requests (delete + busy notify + capture)
// into one async payload build + broadcast so websocket handlers do not block on file/schedule locks.
func ScheduleSendRiftCRALaunch() {
	riftCRABroadcastCoalesceMu.Lock()
	if riftCRABroadcastPending {
		riftCRABroadcastCoalesceMu.Unlock()
		return
	}
	riftCRABroadcastPending = true
	riftCRABroadcastCoalesceMu.Unlock()

	go func() {
		defer func() {
			riftCRABroadcastCoalesceMu.Lock()
			riftCRABroadcastPending = false
			riftCRABroadcastCoalesceMu.Unlock()
		}()
		payload := GameParser.RiftCRALaunchWirePayload()
		SendFrontendMessage("riftCRALaunch", payload, "")
	}()
}
