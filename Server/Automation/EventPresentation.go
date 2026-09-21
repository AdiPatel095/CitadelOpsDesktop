package Automation

import (
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"strconv"
	"strings"
)

// Full event variants keep grammatical event names out of primitive parameters.
func nomadEventDescriptor(eventID int64, variant string, params Localization.Params) *Localization.Message {
	family := ""
	switch eventID {
	case nomadEventID:
		family = "nomad"
	case samuraiEventID:
		family = "samurai"
	default:
		return nil
	}
	switch family + "." + variant {
	case "nomad.advisor_start":
		return Localization.New("server.automation.event_message.nomad.advisor_start", "Start Nomad at difficulty {difficulty} before activating the advisor", params)
	case "nomad.advisor_difficulty":
		return Localization.New("server.automation.event_message.nomad.advisor_difficulty", "The active Nomad run uses difficulty {current}; configured difficulty {configured} applies to the next event", params)
	case "nomad.advisor_discover":
		return Localization.New("server.automation.event_message.nomad.advisor_discover", "Discover the four Nomad camps for Advisor targeting", params)
	case "nomad.advisor_launch":
		return Localization.New("server.automation.event_message.nomad.advisor_launch", "Launch {attacks, number} advisor attacks against Nomad camp {x}:{y}", params)
	case "nomad.nomad_start":
		return Localization.New("server.automation.event_message.nomad.nomad_start", "Start Nomad at difficulty {difficulty}", params)
	case "nomad.nomad_difficulty":
		return Localization.New("server.automation.event_message.nomad.nomad_difficulty", "The active Nomad run already uses difficulty {current}; configured difficulty {configured} applies to the next run", params)
	case "nomad.nomad_missing_preset":
		return Localization.New("server.automation.event_message.nomad.nomad_missing_preset", "The selected Nomad attack preset no longer exists", params)
	case "nomad.nomad_discover_named":
		return Localization.New("server.automation.event_message.nomad.nomad_discover_named", "Discover the four Nomad camps around {castle}", params)
	case "nomad.nomad_discover_id":
		return Localization.New("server.automation.event_message.nomad.nomad_discover_id", "Discover the four Nomad camps around castle {castleID}", params)
	case "samurai.advisor_start":
		return Localization.New("server.automation.event_message.samurai.advisor_start", "Start Samurai at difficulty {difficulty} before activating the advisor", params)
	case "samurai.advisor_difficulty":
		return Localization.New("server.automation.event_message.samurai.advisor_difficulty", "The active Samurai run uses difficulty {current}; configured difficulty {configured} applies to the next event", params)
	case "samurai.advisor_discover":
		return Localization.New("server.automation.event_message.samurai.advisor_discover", "Discover the four Samurai camps for Advisor targeting", params)
	case "samurai.advisor_launch":
		return Localization.New("server.automation.event_message.samurai.advisor_launch", "Launch {attacks, number} advisor attacks against Samurai camp {x}:{y}", params)
	case "samurai.nomad_start":
		return Localization.New("server.automation.event_message.samurai.nomad_start", "Start Samurai at difficulty {difficulty}", params)
	case "samurai.nomad_difficulty":
		return Localization.New("server.automation.event_message.samurai.nomad_difficulty", "The active Samurai run already uses difficulty {current}; configured difficulty {configured} applies to the next run", params)
	case "samurai.nomad_missing_preset":
		return Localization.New("server.automation.event_message.samurai.nomad_missing_preset", "The selected Samurai attack preset no longer exists", params)
	case "samurai.nomad_discover_named":
		return Localization.New("server.automation.event_message.samurai.nomad_discover_named", "Discover the four Samurai camps around {castle}", params)
	case "samurai.nomad_discover_id":
		return Localization.New("server.automation.event_message.samurai.nomad_discover_id", "Discover the four Samurai camps around castle {castleID}", params)
	}
	return nil
}
func nomadDiscoverDescriptor(eventID int64, castle State.CastleState) *Localization.Message {
	if name := strings.TrimSpace(castle.Name); name != "" {
		return nomadEventDescriptor(eventID, "nomad_discover_named", Localization.Params{"castle": name})
	}
	return nomadEventDescriptor(eventID, "nomad_discover_id", Localization.Params{"castleID": strconv.FormatInt(int64(castle.ID), 10)})
}
