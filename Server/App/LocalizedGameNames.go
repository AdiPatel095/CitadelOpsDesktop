package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
)

func gameNameDescriptor(message *Localization.Message, input Intent.PlanningContext, parameter, collection string, id int64, fallback string) *Localization.Message {
	return message.WithGameParam(parameter, input.GameData.DefinitionNameKey(input.Language, collection, id), fallback)
}

func priceNameDescriptor(message *Localization.Message, input Intent.PlanningContext, parameter string, price GameData.AutoBuyerPrice) *Localization.Message {
	if price.ResourceID > 0 {
		return gameNameDescriptor(message, input, parameter, "resources", price.ResourceID, price.Name)
	}
	if price.CurrencyID > 0 {
		return gameNameDescriptor(message, input, parameter, "currencies", price.CurrencyID, price.Name)
	}
	return message
}
