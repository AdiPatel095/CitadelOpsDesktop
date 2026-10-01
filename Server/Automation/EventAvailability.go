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
	state *State.GameState,
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
				"Waiting for the game to confirm %s after today's 10:00 (Berlin time) event update",
				label,
			), DetailDescriptor: limitedEventDescriptor(eventIDs, "opening"),
			NextCheckAt: graceEndsAt,
		}, true
	}

	detail := fmt.Sprintf(
		"%s isn't running right now. This resumes when the event opens.",
		label,
	)
	var detailLocalizationMessage *Localization.Message = limitedEventDescriptor(eventIDs, "inactive")
	if observedAt.Before(opening) {
		detail = fmt.Sprintf(
			"The game hasn't confirmed %s since the last 10:00 (Berlin time) event update. This waits until it does.",
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
		return Localization.New("server.automation.event_gate.nomad_samurai.opening", "Waiting for the game to confirm Nomad or Samurai event after today's 10:00 (Berlin time) event update", nil)
	case "nomad_samurai.inactive":
		return Localization.New("server.automation.event_gate.nomad_samurai.inactive", "Nomad or Samurai event isn't running right now. This resumes when the event opens.", nil)
	case "nomad_samurai.unobserved":
		return Localization.New("server.automation.event_gate.nomad_samurai.unobserved", "The game hasn't confirmed Nomad or Samurai event since the last 10:00 (Berlin time) event update. This waits until it does.", nil)
	case "invasion.opening":
		return Localization.New("server.automation.event_gate.invasion.opening", "Waiting for the game to confirm Foreign Lords or Bloodcrow event after today's 10:00 (Berlin time) event update", nil)
	case "invasion.inactive":
		return Localization.New("server.automation.event_gate.invasion.inactive", "Foreign Lords or Bloodcrow event isn't running right now. This resumes when the event opens.", nil)
	case "invasion.unobserved":
		return Localization.New("server.automation.event_gate.invasion.unobserved", "The game hasn't confirmed Foreign Lords or Bloodcrow event since the last 10:00 (Berlin time) event update. This waits until it does.", nil)
	case "khan.opening":
		return Localization.New("server.automation.event_gate.khan.opening", "Waiting for the game to confirm Nomad Khan event after today's 10:00 (Berlin time) event update", nil)
	case "khan.inactive":
		return Localization.New("server.automation.event_gate.khan.inactive", "Nomad Khan event isn't running right now. This resumes when the event opens.", nil)
	case "khan.unobserved":
		return Localization.New("server.automation.event_gate.khan.unobserved", "The game hasn't confirmed Nomad Khan event since the last 10:00 (Berlin time) event update. This waits until it does.", nil)
	case "berimond.opening":
		return Localization.New("server.automation.event_gate.berimond.opening", "Waiting for the game to confirm Battle for Berimond after today's 10:00 (Berlin time) event update", nil)
	case "berimond.inactive":
		return Localization.New("server.automation.event_gate.berimond.inactive", "Battle for Berimond isn't running right now. This resumes when the event opens.", nil)
	case "berimond.unobserved":
		return Localization.New("server.automation.event_gate.berimond.unobserved", "The game hasn't confirmed Battle for Berimond since the last 10:00 (Berlin time) event update. This waits until it does.", nil)
	}
	return nil
}
