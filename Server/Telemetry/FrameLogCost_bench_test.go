package Telemetry

import (
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

// BenchmarkRecordRawGameFrame10KB is the per-frame cost a hosted runtime no
// longer pays: the complete payload is formatted, copied into the in-memory
// tail and queued for the persistent writer. A store is created with the
// desktop's settings and a data directory, exactly as before CIT-47.
func BenchmarkRecordRawGameFrame10KB(b *testing.B) {
	store := NewStore(5000)
	if err := store.SetDataDir(b.TempDir()); err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	payload := "%xt%gam%1%0%" + strings.Repeat(`{"M":[{"MID":123456,"T":1,"SID":42}]}`, 270) + "%"
	now := time.Now()
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		store.RecordRaw(payload, Protocol.DirectionInbound, now, nil)
	}
}
