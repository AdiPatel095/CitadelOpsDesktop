package Intent

import "CitadelDesktop/Server/Localization"

// StormPlanStatusDescriptor keeps the finite Storm plan at the root. A list
// cannot be moved into context; unknown or composed explanations stay raw.
func StormPlanStatusDescriptor(action *Localization.Message, status, raw string, explanation *Localization.Message, ordinal, total int) *Localization.Message {
	if action == nil || action.Key != "server.storm.purchase_plan" || action.Context != nil || action.ListParams == nil {
		return nil
	}
	copy := Localization.Clone(action)
	if copy == nil {
		return nil
	}
	params := copy.Params
	params["ordinal"], params["total"] = ordinal, total
	var outer *Localization.Message
	switch status {
	case "completed":
		outer = Localization.New("server.storm.completed", "Completed purchase of {purchases} from Luna for {cost, number} Aquamarine at {castle}", params)
	case "completed_batch":
		outer = Localization.New("server.storm.completed_batch", "Completed purchase of {purchases} from Luna for {cost, number} Aquamarine at {castle} ({ordinal, number} of {total, number})", params)
	case "failed":
		outer = Localization.New("server.storm.failed", "Could not complete purchase of {purchases} from Luna for {cost, number} Aquamarine at {castle}.", params)
	case "partial":
		outer = Localization.New("server.storm.partial", "Purchase of {purchases} from Luna for {cost, number} Aquamarine at {castle} completed only in part.", params)
	case "unconfirmed":
		outer = Localization.New("server.storm.unconfirmed", "We could not confirm whether purchase of {purchases} from Luna for {cost, number} Aquamarine at {castle} completed.", params)
	case "failed_activity":
		if explanation == nil || explanation.Context != nil || explanation.ListParams != nil {
			return nil
		}
		outer = Localization.New("server.storm.failed_activity", "Could not complete purchase of {purchases} from Luna for {cost, number} Aquamarine at {castle}. {explanation}", params)
		copy.ListParams["explanation"] = []*Localization.Message{explanation}
	default:
		return nil
	}
	return Localization.WithLists(outer, raw, copy.ListParams)
}
