package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"strconv"
	"strings"
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

func constructionPurchaseDescriptor(input Intent.PlanningContext, itemID int64, itemName string, level, amount int64, castleName string) *Localization.Message {
	key := input.GameData.DefinitionNameKey(input.Language, "constructionItems", itemID)
	if key == "" {
		params := Localization.Params{"amount": amount, "itemID": strconv.FormatInt(itemID, 10), "castle": castleName}
		if level > 0 {
			params["level"] = level
			return Localization.New("server.app.buy_construction_item_id_with_level", "Buy {amount, number} x construction item {itemID} (level {level, number}) from {castle}", params)
		}
		return Localization.New("server.app.buy_construction_item_id", "Buy {amount, number} x construction item {itemID} from {castle}", params)
	}
	params := Localization.Params{"amount": amount, "item": itemName, "castle": castleName}
	var message *Localization.Message
	if level > 0 {
		params["level"] = level
		message = Localization.New("server.app.buy_construction_item_with_level", "Buy {amount, number} x {item} (level {level, number}) from {castle}", params)
	} else {
		message = Localization.New("server.app.buy_construction_item_named", "Buy {amount, number} x {item} from {castle}", params)
	}
	return message.WithGameParam("item", key, itemName)
}

func packagePurchaseDescriptor(input Intent.PlanningContext, product GameData.AutoBuyerPackage, amount int64) *Localization.Message {
	// An atomic unit package has complete source identity and quantity. Other
	// bundles require their own structured recipe; do not translate only the
	// surrounding sentence while concealing an opaque English bundle label.
	switch strings.ToLower(strings.TrimSpace(product.PackageType)) {
	case "soldier", "tool":
	default:
		return nil
	}
	unitKey := input.GameData.DefinitionNameKey(input.Language, "units", product.UnitID)
	if unitKey == "" || product.UnitAmount <= 0 {
		return nil
	}
	unitName, _ := input.Language.Resolve(unitKey)
	message := Localization.New("server.app.buy_unit_packages", "Buy {amount, plural, one {# package} other {# packages}} of {unitAmount, number} {unit} for {cost, number} {currency}", Localization.Params{"amount": amount, "unitAmount": product.UnitAmount, "unit": unitName, "cost": amount * product.Price.Amount, "currency": product.Price.Name})
	message = message.WithGameParam("unit", unitKey, unitName)
	message = priceNameDescriptor(message, input, "currency", product.Price)
	if message.GameParams["currency"].Key == "" {
		return nil
	}
	return message
}
