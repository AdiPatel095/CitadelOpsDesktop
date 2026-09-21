package Automation

import (
	"CitadelDesktop/Server/Localization"
	"fmt"
	"time"
	_ "time/tzdata"

	"CitadelDesktop/Server/State"
)

const (
	// The rotating Great Empire events observed on the official wire end at
	// 09:30 and open at 10:00 Berlin time. The server can take a few minutes to
	// publish the new inventory, so an empty snapshot at exactly 10:00 is not a
	// safe all-day negative result.
	limitedEventOpeningHour  = 10
	limitedEventOpeningGrace = 5 * time.Minute
)

var limitedEventLocation = func() *time.Location {
	location, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.FixedZone("CET", 60*60)
	}
	return location
}()

func limitedEventGate(
	state State.GameState,
	now time.Time,
	eventIDs []int64,
	label string,
) (Decision, bool) {
	if _, active := state.AnyEventAvailable(eventIDs, now); active {
		return Decision{}, false
	}
	observedAt := state.EventScores.Inventory.ObservedAt
	if observedAt.IsZero() {
		// A fresh session baseline normally contains `sei`. Until one has been
		// observed, retain the policy's existing authoritative checks rather
		// than treating missing state as proof that the event is unavailable.
		return Decision{}, false
	}

	opening := limitedEventOpeningAtOrBefore(now)
	graceEndsAt := opening.Add(limitedEventOpeningGrace)
	if !now.Before(opening) && now.Before(graceEndsAt) {
		return Decision{
			Status: "opening-check",
			Detail: fmt.Sprintf(
				"Waiting for the authoritative %s inventory to settle after the 10:00 Europe/Berlin opening check",
				label,
			), DetailDescriptor: Localization.New("server.automation.waiting_for_the_authoritative.eeb23a21", "Waiting for the authoritative {p0} inventory to settle after the 10:00 Europe/Berlin opening check", Localization.Params{"p0": fmt.Sprintf("%s", label)}),
			NextCheckAt: graceEndsAt,
		}, true
	}

	detail := fmt.Sprintf(
		"%s is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates",
		label,
	)
	var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.p_is_not_active.fe1460d7", "{p0} is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates", Localization.Params{"p0": fmt.Sprintf("%s", label)})
	if observedAt.Before(opening) {
		detail = fmt.Sprintf(
			"No authoritative %s inventory arrived after the latest opening; this lane remains softly locked until an event update",
			label,
		)
		detailLocalizationMessage = Localization.New("server.automation.no_authoritative_p_inventory.13781d1e", "No authoritative {p0} inventory arrived after the latest opening; this lane remains softly locked until an event update", Localization.Params{"p0": fmt.Sprintf("%s", label)})
	}
	return Decision{
		Status: "soft-locked",
		Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
		NextCheckAt: limitedEventOpeningAfter(now),
	}, true
}

func limitedEventOpeningAtOrBefore(now time.Time) time.Time {
	local := now.In(limitedEventLocation)
	opening := time.Date(
		local.Year(), local.Month(), local.Day(), limitedEventOpeningHour, 0, 0, 0, limitedEventLocation,
	)
	if opening.After(local) {
		opening = opening.AddDate(0, 0, -1)
	}
	return opening.UTC()
}

func limitedEventOpeningAfter(now time.Time) time.Time {
	local := now.In(limitedEventLocation)
	opening := time.Date(
		local.Year(), local.Month(), local.Day(), limitedEventOpeningHour, 0, 0, 0, limitedEventLocation,
	)
	if !opening.After(local) {
		opening = opening.AddDate(0, 0, 1)
	}
	return opening.UTC()
}
