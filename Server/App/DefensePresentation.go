package App

import (
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"strconv"
)

// User castle and preset names stay verbatim parameters. An unnamed castle uses
// an explicit ID variant so the generated noun is part of the translated template.
func defenseSummaryDescriptor(kind string, castle State.CastleState, preset string) *Localization.Message {
	params := Localization.Params{"castle": castle.Name, "id": strconv.FormatInt(int64(castle.ID), 10), "preset": preset}
	switch kind {
	case "refresh":
		if castle.Name != "" {
			return Localization.New("server.app.defense_summary.refresh.named", "Refresh defense setup for {castle}", Localization.Params{"castle": params["castle"]})
		}
		return Localization.New("server.app.defense_summary.refresh.id", "Refresh defense setup for castle {id}", Localization.Params{"id": params["id"]})
	case "gates":
		if castle.Name != "" {
			return Localization.New("server.app.defense_summary.gates.named", "Open gates at {castle}", Localization.Params{"castle": params["castle"]})
		}
		return Localization.New("server.app.defense_summary.gates.id", "Open gates at castle {id}", Localization.Params{"id": params["id"]})
	case "wall":
		if castle.Name != "" {
			return Localization.New("server.app.defense_summary.wall.named", "Update defense wall setup for {castle}", Localization.Params{"castle": params["castle"]})
		}
		return Localization.New("server.app.defense_summary.wall.id", "Update defense wall setup for castle {id}", Localization.Params{"id": params["id"]})
	case "moat":
		if castle.Name != "" {
			return Localization.New("server.app.defense_summary.moat.named", "Update defense moat setup for {castle}", Localization.Params{"castle": params["castle"]})
		}
		return Localization.New("server.app.defense_summary.moat.id", "Update defense moat setup for castle {id}", Localization.Params{"id": params["id"]})
	case "keep":
		if castle.Name != "" {
			return Localization.New("server.app.defense_summary.keep.named", "Update defense keep setup for {castle}", Localization.Params{"castle": params["castle"]})
		}
		return Localization.New("server.app.defense_summary.keep.id", "Update defense keep setup for castle {id}", Localization.Params{"id": params["id"]})
	case "preset":
		if castle.Name != "" {
			return Localization.New("server.app.defense_summary.preset.named", "Apply defense preset {preset} to {castle}", Localization.Params{"castle": params["castle"], "preset": preset})
		}
		return Localization.New("server.app.defense_summary.preset.id", "Apply defense preset {preset} to castle {id}", Localization.Params{"id": params["id"], "preset": preset})
	}
	return nil
}
