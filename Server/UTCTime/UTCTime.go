// Package utctime provides UTC instants steered from public NTP (pool.ntp.org), the standard way
// to align wall time with stratum-1/atomic time sources. Local tick rate still drifts between syncs;
// we refresh the offset periodically.
package utctime

import (
	"log"
	"sync"
	"time"

	"github.com/beevik/ntp"
)

const (
	ntpPool       = "pool.ntp.org"
	syncInterval  = 1 * time.Hour
	ntpReqTimeout = 4 * time.Second
)

var (
	offsetMu      sync.RWMutex
	ntpAdd        time.Duration // add to time.Now() to match last NTP sample
	ntpBackground sync.Once
)

// Now returns the best-estimate current instant in the UTC frame: local monotonic
// time plus the last NTP-reported offset. All AutoTCI deadlines should use this.
func Now() time.Time {
	ntpBackground.Do(startNTPResync)
	offsetMu.RLock()
	a := ntpAdd
	offsetMu.RUnlock()
	return time.Now().Add(a).UTC()
}

// Until returns t.Sub(Now()), same as time.Until when Now is the NTP-steered clock.
func Until(t time.Time) time.Duration {
	return t.Sub(Now())
}

// Since returns Now().Sub(t).
func Since(t time.Time) time.Duration {
	return Now().Sub(t)
}

// UnixMilli is shorthand for Now().UnixMilli().
func UnixMilli() int64 {
	return Now().UnixMilli()
}

func startNTPResync() {
	refreshNTP()
	go func() {
		t := time.NewTicker(syncInterval)
		defer t.Stop()
		for range t.C {
			refreshNTP()
		}
	}()
}

func refreshNTP() {
	type res struct {
		t   time.Time
		err error
	}
	ch := make(chan res, 1)
	go func() {
		tt, err := ntp.Time(ntpPool)
		ch <- res{tt, err}
	}()
	var tServer time.Time
	var err error
	select {
	case r := <-ch:
		tServer, err = r.t, r.err
	case <-time.After(ntpReqTimeout):
		log.Printf("[utctime] NTP (UTC) request to %s timed out", ntpPool)
		return
	}
	if err != nil {
		log.Printf("[utctime] NTP (UTC via %s) sync failed: %v", ntpPool, err)
		return
	}
	// Sample offset: NTP time minus local at the same instant.
	adj := tServer.Sub(time.Now())
	offsetMu.Lock()
	ntpAdd = adj
	offsetMu.Unlock()
}
