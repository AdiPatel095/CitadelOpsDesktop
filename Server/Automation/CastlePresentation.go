package Automation

import (
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"strconv"
)

// Whole variants preserve generated castle-ID prose while keeping player names
// literal. The parameter map is copied before adding presentation identity.
func castleDecisionDescriptor(variant string, castle State.CastleState, values Localization.Params) *Localization.Message {
	params := Localization.Params{}
	for key, value := range values {
		params[key] = value
	}
	if castle.Name != "" {
		params["castle"] = castle.Name
		variant += ".named"
	} else {
		params["castleID"] = strconv.FormatInt(int64(castle.ID), 10)
		variant += ".id"
	}
	switch variant {
	case "bird_refresh_target.named":
		return Localization.New("server.automation.castle_message.bird_refresh_target.named", "Refresh changed or expired Auto Bird target for {castle}", params)
	case "bird_refresh_target.id":
		return Localization.New("server.automation.castle_message.bird_refresh_target.id", "Refresh changed or expired Auto Bird target for castle {castleID}", params)
	case "bird_refresh_expired.named":
		return Localization.New("server.automation.castle_message.bird_refresh_expired.named", "Refresh expired Auto Bird troop inventory for {castle}", params)
	case "bird_refresh_expired.id":
		return Localization.New("server.automation.castle_message.bird_refresh_expired.id", "Refresh expired Auto Bird troop inventory for castle {castleID}", params)
	case "bird_refresh_inventory.named":
		return Localization.New("server.automation.castle_message.bird_refresh_inventory.named", "Refresh the latest troop inventory for {castle}", params)
	case "bird_refresh_inventory.id":
		return Localization.New("server.automation.castle_message.bird_refresh_inventory.id", "Refresh the latest troop inventory for castle {castleID}", params)
	case "bird_restart_settings.named":
		return Localization.New("server.automation.castle_message.bird_restart_settings.named", "Restart after relevant automation settings changed for {castle}", params)
	case "bird_restart_settings.id":
		return Localization.New("server.automation.castle_message.bird_restart_settings.id", "Restart after relevant automation settings changed for castle {castleID}", params)
	case "bird_restart_preset.named":
		return Localization.New("server.automation.castle_message.bird_restart_preset.named", "Restart after the Auto Bird preset changed for {castle}", params)
	case "bird_restart_preset.id":
		return Localization.New("server.automation.castle_message.bird_restart_preset.id", "Restart after the Auto Bird preset changed for castle {castleID}", params)
	case "bird_discover.named":
		return Localization.New("server.automation.castle_message.bird_discover.named", "Run this castle's independent AIN target discovery for {castle}", params)
	case "bird_discover.id":
		return Localization.New("server.automation.castle_message.bird_discover.id", "Run this castle's independent AIN target discovery for castle {castleID}", params)
	case "bird_dispatch.named":
		return Localization.New("server.automation.castle_message.bird_dispatch.named", "Dispatch {troops, number} freshly inventoried troops from {castle} to bird target {target}", params)
	case "bird_dispatch.id":
		return Localization.New("server.automation.castle_message.bird_dispatch.id", "Dispatch {troops, number} freshly inventoried troops from castle {castleID} to bird target {target}", params)
	case "station_evacuate.named":
		return Localization.New("server.automation.castle_message.station_evacuate.named", "Evacuating {troops, number} troops from {castle}", params)
	case "station_evacuate.id":
		return Localization.New("server.automation.castle_message.station_evacuate.id", "Evacuating {troops, number} troops from castle {castleID}", params)
	case "fortress_discover.named":
		return Localization.New("server.automation.castle_message.fortress_discover.named", "Discover every fortress across {castle}", params)
	case "fortress_discover.id":
		return Localization.New("server.automation.castle_message.fortress_discover.id", "Discover every fortress across castle {castleID}", params)
	case "fortress_refresh_purchased.named":
		return Localization.New("server.automation.castle_message.fortress_refresh_purchased.named", "Refresh purchased Direwolves at {castle}", params)
	case "fortress_refresh_purchased.id":
		return Localization.New("server.automation.castle_message.fortress_refresh_purchased.id", "Refresh purchased Direwolves at castle {castleID}", params)
	case "fortress_refresh_destination.named":
		return Localization.New("server.automation.castle_message.fortress_refresh_destination.named", "Refresh destination Direwolves at {castle}", params)
	case "fortress_refresh_destination.id":
		return Localization.New("server.automation.castle_message.fortress_refresh_destination.id", "Refresh destination Direwolves at castle {castleID}", params)
	case "fortress_allocate.named":
		return Localization.New("server.automation.castle_message.fortress_allocate.named", "Allocate {troops, number} Direwolves to {castle}", params)
	case "fortress_allocate.id":
		return Localization.New("server.automation.castle_message.fortress_allocate.id", "Allocate {troops, number} Direwolves to castle {castleID}", params)
	case "fortress_refresh_arrived.named":
		return Localization.New("server.automation.castle_message.fortress_refresh_arrived.named", "Refresh arrived Direwolves at {castle}", params)
	case "fortress_refresh_arrived.id":
		return Localization.New("server.automation.castle_message.fortress_refresh_arrived.id", "Refresh arrived Direwolves at castle {castleID}", params)
	case "fortress_settle_transfer.named":
		return Localization.New("server.automation.castle_message.fortress_settle_transfer.named", "Settle completed Direwolf transfer to {castle}", params)
	case "fortress_settle_transfer.id":
		return Localization.New("server.automation.castle_message.fortress_settle_transfer.id", "Settle completed Direwolf transfer to castle {castleID}", params)
	}
	return nil
}
