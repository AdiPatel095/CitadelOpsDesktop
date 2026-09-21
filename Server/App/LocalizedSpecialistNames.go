package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
)

// These finite whole-message variants keep custom role nouns in the outer ICU
// template, where translators can inflect them with the owning sentence.
func specialistNameDescriptor(message *Localization.Message, specialist GameData.AutoBuyerSpecialist) *Localization.Message {
	if message == nil {
		return nil
	}
	var template *Localization.Message
	parameter := ""
	switch message.Key {
	case "server.app.renew_p_by_days.310dce39":
		parameter = "p0"
		switch specialist.ID {
		case 0:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.0", "Renew wood overseer by 7 days within a {p1}-ruby ceiling", nil)
		case 1:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.1", "Renew stone overseer by 7 days within a {p1}-ruby ceiling", nil)
		case 2:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.2", "Renew food overseer by 7 days within a {p1}-ruby ceiling", nil)
		case 3:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.3", "Renew honey overseer by 7 days within a {p1}-ruby ceiling", nil)
		case 4:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.4", "Renew mead overseer by 7 days within a {p1}-ruby ceiling", nil)
		case 5:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.5", "Renew beef overseer by 7 days within a {p1}-ruby ceiling", nil)
		case 6:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.6", "Renew marauder by 7 days within a {p1}-ruby ceiling", nil)
		case 8:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.8", "Renew tax collector by 7 days within a {p1}-ruby ceiling", nil)
		case 10:
			template = Localization.New("server.app.renew_p_by_days.310dce39.specialist.10", "Renew drill instructor by 7 days within a {p1}-ruby ceiling", nil)
		}
	case "server.app.intent_plan_became_stale.abd93795":
		parameter = "p1"
		switch specialist.ID {
		case 0:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.0", "Intent plan became stale before dispatch: wood overseer timer changed", nil)
		case 1:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.1", "Intent plan became stale before dispatch: stone overseer timer changed", nil)
		case 2:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.2", "Intent plan became stale before dispatch: food overseer timer changed", nil)
		case 3:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.3", "Intent plan became stale before dispatch: honey overseer timer changed", nil)
		case 4:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.4", "Intent plan became stale before dispatch: mead overseer timer changed", nil)
		case 5:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.5", "Intent plan became stale before dispatch: beef overseer timer changed", nil)
		case 6:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.6", "Intent plan became stale before dispatch: marauder timer changed", nil)
		case 8:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.8", "Intent plan became stale before dispatch: tax collector timer changed", nil)
		case 10:
			template = Localization.New("server.app.intent_plan_became_stale.abd93795.specialist.10", "Intent plan became stale before dispatch: drill instructor timer changed", nil)
		}
	case "server.app.intent_plan_became_stale.4e87347b":
		parameter = "p1"
		switch specialist.ID {
		case 0:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.0", "Intent plan became stale before dispatch: wood overseer already meets its configured floor", nil)
		case 1:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.1", "Intent plan became stale before dispatch: stone overseer already meets its configured floor", nil)
		case 2:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.2", "Intent plan became stale before dispatch: food overseer already meets its configured floor", nil)
		case 3:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.3", "Intent plan became stale before dispatch: honey overseer already meets its configured floor", nil)
		case 4:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.4", "Intent plan became stale before dispatch: mead overseer already meets its configured floor", nil)
		case 5:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.5", "Intent plan became stale before dispatch: beef overseer already meets its configured floor", nil)
		case 6:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.6", "Intent plan became stale before dispatch: marauder already meets its configured floor", nil)
		case 8:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.8", "Intent plan became stale before dispatch: tax collector already meets its configured floor", nil)
		case 10:
			template = Localization.New("server.app.intent_plan_became_stale.4e87347b.specialist.10", "Intent plan became stale before dispatch: drill instructor already meets its configured floor", nil)
		}
	case "server.app.intent_plan_became_stale.3e9144d6":
		parameter = "p1"
		switch specialist.ID {
		case 0:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.0", "Intent plan became stale before dispatch: wood overseer requires up to {p2} rubies above reserve", nil)
		case 1:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.1", "Intent plan became stale before dispatch: stone overseer requires up to {p2} rubies above reserve", nil)
		case 2:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.2", "Intent plan became stale before dispatch: food overseer requires up to {p2} rubies above reserve", nil)
		case 3:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.3", "Intent plan became stale before dispatch: honey overseer requires up to {p2} rubies above reserve", nil)
		case 4:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.4", "Intent plan became stale before dispatch: mead overseer requires up to {p2} rubies above reserve", nil)
		case 5:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.5", "Intent plan became stale before dispatch: beef overseer requires up to {p2} rubies above reserve", nil)
		case 6:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.6", "Intent plan became stale before dispatch: marauder requires up to {p2} rubies above reserve", nil)
		case 8:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.8", "Intent plan became stale before dispatch: tax collector requires up to {p2} rubies above reserve", nil)
		case 10:
			template = Localization.New("server.app.intent_plan_became_stale.3e9144d6.specialist.10", "Intent plan became stale before dispatch: drill instructor requires up to {p2} rubies above reserve", nil)
		}
	case "server.app.p_renewal_was_not.3e56e590":
		parameter = "p0"
		switch specialist.ID {
		case 0:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.0", "Wood overseer renewal was not confirmed by the refreshed specialist timer", nil)
		case 1:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.1", "Stone overseer renewal was not confirmed by the refreshed specialist timer", nil)
		case 2:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.2", "Food overseer renewal was not confirmed by the refreshed specialist timer", nil)
		case 3:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.3", "Honey overseer renewal was not confirmed by the refreshed specialist timer", nil)
		case 4:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.4", "Mead overseer renewal was not confirmed by the refreshed specialist timer", nil)
		case 5:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.5", "Beef overseer renewal was not confirmed by the refreshed specialist timer", nil)
		case 6:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.6", "Marauder renewal was not confirmed by the refreshed specialist timer", nil)
		case 8:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.8", "Tax collector renewal was not confirmed by the refreshed specialist timer", nil)
		case 10:
			template = Localization.New("server.app.p_renewal_was_not.3e56e590.specialist.10", "Drill instructor renewal was not confirmed by the refreshed specialist timer", nil)
		}
	}
	if template == nil {
		return nil
	}
	result := Localization.Clone(message)
	result.Key, result.Fallback = template.Key, template.Fallback
	delete(result.Params, parameter)
	delete(result.GameParams, parameter)
	return result
}
