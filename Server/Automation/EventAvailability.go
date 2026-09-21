package Automation

import (
	"CitadelDesktop/Server/GameData"
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
			), DetailDescriptor: limitedEventDescriptor(eventIDs, "opening"),
			NextCheckAt: graceEndsAt,
		}, true
	}

	detail := fmt.Sprintf(
		"%s is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates",
		label,
	)
	var detailLocalizationMessage *Localization.Message = limitedEventDescriptor(eventIDs, "inactive")
	if observedAt.Before(opening) {
		detail = fmt.Sprintf(
			"No authoritative %s inventory arrived after the latest opening; this lane remains softly locked until an event update",
			label,
		)
		detailLocalizationMessage = limitedEventDescriptor(eventIDs, "unobserved")
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

// Select complete templates by source event identity, never by a rendered label.
// Unknown event families retain the complete legacy fallback.
func limitedEventDescriptor(ids []int64, phase string) *Localization.Message {
	family := ""
	contains := func(id int64) bool {
		for _, value := range ids {
			if value == id {
				return true
			}
		}
		return false
	}
	switch {
	case len(ids) == 2 && contains(nomadEventID) && contains(samuraiEventID):
		family = "nomad_samurai"
	case len(ids) == 2 && contains(foreignLordsEventID) && contains(bloodcrowEventID):
		family = "invasion"
	case len(ids) == 1 && ids[0] == autoKhanEventID:
		family = "khan"
	case len(ids) == 1 && ids[0] == GameData.BerimondEventID:
		family = "berimond"
	}
	switch family + "." + phase {
	case "nomad_samurai.opening":
		return Localization.New("server.automation.event_gate.nomad_samurai.opening", "Waiting for the authoritative Nomad or Samurai event inventory to settle after the 10:00 Europe/Berlin opening check", nil)
	case "nomad_samurai.inactive":
		return Localization.New("server.automation.event_gate.nomad_samurai.inactive", "Nomad or Samurai event is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates", nil)
	case "nomad_samurai.unobserved":
		return Localization.New("server.automation.event_gate.nomad_samurai.unobserved", "No authoritative Nomad or Samurai event inventory arrived after the latest opening; this lane remains softly locked until an event update", nil)
	case "invasion.opening":
		return Localization.New("server.automation.event_gate.invasion.opening", "Waiting for the authoritative Foreign Lords or Bloodcrow event inventory to settle after the 10:00 Europe/Berlin opening check", nil)
	case "invasion.inactive":
		return Localization.New("server.automation.event_gate.invasion.inactive", "Foreign Lords or Bloodcrow event is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates", nil)
	case "invasion.unobserved":
		return Localization.New("server.automation.event_gate.invasion.unobserved", "No authoritative Foreign Lords or Bloodcrow event inventory arrived after the latest opening; this lane remains softly locked until an event update", nil)
	case "khan.opening":
		return Localization.New("server.automation.event_gate.khan.opening", "Waiting for the authoritative Nomad Khan event inventory to settle after the 10:00 Europe/Berlin opening check", nil)
	case "khan.inactive":
		return Localization.New("server.automation.event_gate.khan.inactive", "Nomad Khan event is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates", nil)
	case "khan.unobserved":
		return Localization.New("server.automation.event_gate.khan.unobserved", "No authoritative Nomad Khan event inventory arrived after the latest opening; this lane remains softly locked until an event update", nil)
	case "berimond.opening":
		return Localization.New("server.automation.event_gate.berimond.opening", "Waiting for the authoritative Battle for Berimond inventory to settle after the 10:00 Europe/Berlin opening check", nil)
	case "berimond.inactive":
		return Localization.New("server.automation.event_gate.berimond.inactive", "Battle for Berimond is not active in the latest confirmed event list; this lane will resume after the event opens or the list updates", nil)
	case "berimond.unobserved":
		return Localization.New("server.automation.event_gate.berimond.unobserved", "No authoritative Battle for Berimond inventory arrived after the latest opening; this lane remains softly locked until an event update", nil)
	}
	return nil
}
