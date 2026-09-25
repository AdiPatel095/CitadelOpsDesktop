package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
	"strconv"
	"strings"
)

// Role grammar belongs to the complete outer template, never an English noun parameter.
func buyerSpecialistDescriptor(variant string, specialist GameData.AutoBuyerSpecialist, params Localization.Params) *Localization.Message {
	switch variant {
	case "renew":
		switch specialist.ID {
		case 0:
			return Localization.New("server.automation.buyer_specialist.renew.0", "Renew wood overseer by 7 days toward the {days, number}-day floor", params)
		case 1:
			return Localization.New("server.automation.buyer_specialist.renew.1", "Renew stone overseer by 7 days toward the {days, number}-day floor", params)
		case 2:
			return Localization.New("server.automation.buyer_specialist.renew.2", "Renew food overseer by 7 days toward the {days, number}-day floor", params)
		case 3:
			return Localization.New("server.automation.buyer_specialist.renew.3", "Renew honey overseer by 7 days toward the {days, number}-day floor", params)
		case 4:
			return Localization.New("server.automation.buyer_specialist.renew.4", "Renew mead overseer by 7 days toward the {days, number}-day floor", params)
		case 5:
			return Localization.New("server.automation.buyer_specialist.renew.5", "Renew beef overseer by 7 days toward the {days, number}-day floor", params)
		case 6:
			return Localization.New("server.automation.buyer_specialist.renew.6", "Renew marauder by 7 days toward the {days, number}-day floor", params)
		case 8:
			return Localization.New("server.automation.buyer_specialist.renew.8", "Renew tax collector by 7 days toward the {days, number}-day floor", params)
		case 10:
			return Localization.New("server.automation.buyer_specialist.renew.10", "Renew drill instructor by 7 days toward the {days, number}-day floor", params)
		}
	case "balance":
		switch specialist.ID {
		case 0:
			return Localization.New("server.automation.buyer_specialist.balance.0", "Waiting for a fresh current-session ruby balance before renewing wood overseer", params)
		case 1:
			return Localization.New("server.automation.buyer_specialist.balance.1", "Waiting for a fresh current-session ruby balance before renewing stone overseer", params)
		case 2:
			return Localization.New("server.automation.buyer_specialist.balance.2", "Waiting for a fresh current-session ruby balance before renewing food overseer", params)
		case 3:
			return Localization.New("server.automation.buyer_specialist.balance.3", "Waiting for a fresh current-session ruby balance before renewing honey overseer", params)
		case 4:
			return Localization.New("server.automation.buyer_specialist.balance.4", "Waiting for a fresh current-session ruby balance before renewing mead overseer", params)
		case 5:
			return Localization.New("server.automation.buyer_specialist.balance.5", "Waiting for a fresh current-session ruby balance before renewing beef overseer", params)
		case 6:
			return Localization.New("server.automation.buyer_specialist.balance.6", "Waiting for a fresh current-session ruby balance before renewing marauder", params)
		case 8:
			return Localization.New("server.automation.buyer_specialist.balance.8", "Waiting for a fresh current-session ruby balance before renewing tax collector", params)
		case 10:
			return Localization.New("server.automation.buyer_specialist.balance.10", "Waiting for a fresh current-session ruby balance before renewing drill instructor", params)
		}
	case "cost":
		switch specialist.ID {
		case 0:
			return Localization.New("server.automation.buyer_specialist.cost.0", "Waiting for {cost, number} rubies above reserve to renew wood overseer", params)
		case 1:
			return Localization.New("server.automation.buyer_specialist.cost.1", "Waiting for {cost, number} rubies above reserve to renew stone overseer", params)
		case 2:
			return Localization.New("server.automation.buyer_specialist.cost.2", "Waiting for {cost, number} rubies above reserve to renew food overseer", params)
		case 3:
			return Localization.New("server.automation.buyer_specialist.cost.3", "Waiting for {cost, number} rubies above reserve to renew honey overseer", params)
		case 4:
			return Localization.New("server.automation.buyer_specialist.cost.4", "Waiting for {cost, number} rubies above reserve to renew mead overseer", params)
		case 5:
			return Localization.New("server.automation.buyer_specialist.cost.5", "Waiting for {cost, number} rubies above reserve to renew beef overseer", params)
		case 6:
			return Localization.New("server.automation.buyer_specialist.cost.6", "Waiting for {cost, number} rubies above reserve to renew marauder", params)
		case 8:
			return Localization.New("server.automation.buyer_specialist.cost.8", "Waiting for {cost, number} rubies above reserve to renew tax collector", params)
		case 10:
			return Localization.New("server.automation.buyer_specialist.cost.10", "Waiting for {cost, number} rubies above reserve to renew drill instructor", params)
		}
	case "floor":
		switch specialist.ID {
		case 0:
			return Localization.New("server.automation.buyer_specialist.floor.0", "The wood overseer floor must be between {minimum, number} and 365 days", params)
		case 1:
			return Localization.New("server.automation.buyer_specialist.floor.1", "The stone overseer floor must be between {minimum, number} and 365 days", params)
		case 2:
			return Localization.New("server.automation.buyer_specialist.floor.2", "The food overseer floor must be between {minimum, number} and 365 days", params)
		case 3:
			return Localization.New("server.automation.buyer_specialist.floor.3", "The honey overseer floor must be between {minimum, number} and 365 days", params)
		case 4:
			return Localization.New("server.automation.buyer_specialist.floor.4", "The mead overseer floor must be between {minimum, number} and 365 days", params)
		case 5:
			return Localization.New("server.automation.buyer_specialist.floor.5", "The beef overseer floor must be between {minimum, number} and 365 days", params)
		case 6:
			return Localization.New("server.automation.buyer_specialist.floor.6", "The marauder floor must be between {minimum, number} and 365 days", params)
		case 8:
			return Localization.New("server.automation.buyer_specialist.floor.8", "The tax collector floor must be between {minimum, number} and 365 days", params)
		case 10:
			return Localization.New("server.automation.buyer_specialist.floor.10", "The drill instructor floor must be between {minimum, number} and 365 days", params)
		}
	case "ceiling":
		switch specialist.ID {
		case 0:
			return Localization.New("server.automation.buyer_specialist.ceiling.0", "The wood overseer ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 1:
			return Localization.New("server.automation.buyer_specialist.ceiling.1", "The stone overseer ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 2:
			return Localization.New("server.automation.buyer_specialist.ceiling.2", "The food overseer ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 3:
			return Localization.New("server.automation.buyer_specialist.ceiling.3", "The honey overseer ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 4:
			return Localization.New("server.automation.buyer_specialist.ceiling.4", "The mead overseer ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 5:
			return Localization.New("server.automation.buyer_specialist.ceiling.5", "The beef overseer ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 6:
			return Localization.New("server.automation.buyer_specialist.ceiling.6", "The marauder ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 8:
			return Localization.New("server.automation.buyer_specialist.ceiling.8", "The tax collector ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		case 10:
			return Localization.New("server.automation.buyer_specialist.ceiling.10", "The drill instructor ruby ceiling must cover its validated maximum cost of {cost, number}", params)
		}
	}
	return nil
}

// IDs remain exact when a package is a compound bundle without an atomic name.
func buyerPackageRuleDescriptor(variant string, product GameData.AutoBuyerPackage, params Localization.Params) *Localization.Message {
	values := Localization.Params{"packageID": strconv.FormatInt(product.PackageID, 10)}
	for key, value := range params {
		values[key] = value
	}
	switch variant {
	case "target":
		return Localization.New("server.automation.buyer_package.target", "Package {packageID} target must be between 1 and its stock limit {stock, number}", values)
	case "reserve":
		return Localization.New("server.automation.buyer_package.reserve", "Package {packageID} reserve and ruby ceiling cannot be negative", values)
	case "ceiling":
		return Localization.New("server.automation.buyer_package.ceiling", "Package {packageID} needs an explicit ruby ceiling of at least {cost, number}", values)
	case "level":
		return Localization.New("server.automation.buyer_package.level", "Package {packageID} is not available at the current player level", values)
	case "advertised":
		return Localization.New("server.automation.buyer_package.advertised", "Package {packageID} is not advertised by its current shop", values)
	case "destination":
		return Localization.New("server.automation.buyer_package.destination", "Package {packageID} is unavailable at {castle}", values)
	case "disabled":
		return Localization.New("server.automation.buyer_package.disabled", "Ruby shop purchases are disabled for package {packageID}", values)
	case "spent":
		return Localization.New("server.automation.buyer_package.spent", "Package {packageID} reached its ruby ceiling for this stock reset", values)
	}
	return nil
}

func buyerPriceDescriptor(message *Localization.Message, snapshot Snapshot, price GameData.AutoBuyerPrice) *Localization.Message {
	if price.ResourceID > 0 {
		return message.WithGameParam("currency", snapshot.GameData.DefinitionNameKey(snapshot.Language, "resources", price.ResourceID), price.Name)
	}
	if price.CurrencyID > 0 {
		return message.WithGameParam("currency", snapshot.GameData.DefinitionNameKey(snapshot.Language, "currencies", price.CurrencyID), price.Name)
	}
	return nil
}

func buyerPackagePurchaseDescriptor(snapshot Snapshot, product GameData.AutoBuyerPackage, amount int64, waiting bool) *Localization.Message {
	params := Localization.Params{"packageID": strconv.FormatInt(product.PackageID, 10), "amount": amount, "cost": amount * product.Price.Amount, "currency": product.Price.Name}
	var message *Localization.Message
	if waiting {
		params["cost"] = product.Price.Amount
		delete(params, "amount")
		message = Localization.New("server.automation.buyer_package.waiting_cost", "Waiting for {cost, number} {currency} above reserve to buy package {packageID}", params)
	} else {
		message = Localization.New("server.automation.buyer_package.purchase", "Buy {amount, plural, one {# package} other {# packages}} with ID {packageID} for {cost, number} {currency}", params)
	}
	// Atomic packages also carry the verified official unit identity and quantity.
	if kind := strings.ToLower(strings.TrimSpace(product.PackageType)); kind == "soldier" || kind == "tool" {
		key := snapshot.GameData.DefinitionNameKey(snapshot.Language, "units", product.UnitID)
		if key != "" && product.UnitAmount > 0 {
			unit, _ := snapshot.Language.Resolve(key)
			delete(params, "packageID")
			params["unit"] = unit
			params["unitAmount"] = product.UnitAmount
			if waiting {
				message = Localization.New("server.automation.buyer_package.waiting_unit_cost", "Waiting for {cost, number} {currency} above reserve to buy a package of {unitAmount, number} {unit}", params)
			} else {
				message = Localization.New("server.automation.buyer_package.purchase_units", "Buy {amount, plural, one {# package} other {# packages}} of {unitAmount, number} {unit} for {cost, number} {currency}", params)
			}
			message = message.WithGameParam("unit", key, unit)
		}
	}
	return buyerPriceDescriptor(message, snapshot, product.Price)
}

func buyerFeastRuleDescriptor(variant string, feast GameData.AutoBuyerFeast, cost int64) *Localization.Message {
	params := Localization.Params{"feastID": strconv.FormatInt(feast.ID, 10), "cost": cost}
	switch variant {
	case "invalid":
		delete(params, "cost")
		return Localization.New("server.automation.buyer_feast.invalid", "Feast {feastID} duration, reserve, or ruby ceiling is invalid", params)
	case "permission":
		return Localization.New("server.automation.buyer_feast.permission", "Feast {feastID} needs explicit ruby permission and a ceiling of at least {cost, number}", params)
	}
	return nil
}
