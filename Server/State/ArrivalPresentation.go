package State

import (
	"CitadelDesktop/Server/Localization"
	"strconv"
	"time"
)

// ArrivalOrderDescriptor preserves the exact protocol identities and timestamps.
func ArrivalOrderDescriptor(commander CommanderID, arrival time.Time, previous CommanderID, previousArrival time.Time) *Localization.Message {
	return Localization.New("server.state.arrival_order.inversion", "commander {commander} arrives at {arrival} before commander {previous} at {previousArrival}", Localization.Params{"commander": strconv.FormatInt(int64(commander), 10), "arrival": arrival.Format(time.RFC3339Nano), "previous": strconv.FormatInt(int64(previous), 10), "previousArrival": previousArrival.Format(time.RFC3339Nano)})
}
