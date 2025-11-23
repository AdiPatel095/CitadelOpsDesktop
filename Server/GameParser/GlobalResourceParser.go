package GameParser

import (
	"CitadelDesktop/Server/Models"
	"log"
)

func UpdateCoins(gcuMap map[string]interface{}) {
	coins, ok := gcuMap["C1"].(float64)
	if ok {
		Models.GetPlayerGlobalResources().Coins = coins
	}
	rubies, ok := gcuMap["C2"].(float64)
	if ok {
		Models.GetPlayerGlobalResources().Rubies = rubies
	}
}

func UpdateSCE(sceArray []interface{}) {
	for _, item := range sceArray {
		valueArray, ok := item.([]interface{})
		if !ok || len(valueArray) != 2 {
			log.Printf("Skipping invalid item in sceArray: %v", item)
			continue
		}

		label, labelOk := valueArray[0].(string)
		value, valueOk := valueArray[1].(float64)

		if !labelOk || !valueOk {
			log.Printf("Skipping item with invalid types in sceArray: %v", valueArray)
			continue
		}
		updateResourceByLabel(label, value)
	}
}

func updateResourceByLabel(label string, value float64) {
	playerResources := Models.GetPlayerGlobalResources()
	switch label {
	case "STP":
		playerResources.Sceat = value
	case "IDCT":
		playerResources.Ducat = value
	case "LM":
		playerResources.ConstToken = value
	case "LT":
		playerResources.UpgrToken = value
	case "SLWT":
		playerResources.AfflTix = value
	case "PL":
		playerResources.Plaster = value
	case "DST":
		playerResources.DrgScale = value
	case "DSS":
		playerResources.DrgSpl = value
	case "RF":
		playerResources.RelicShard = value
	case "MS1":
		playerResources.Min1 = value
	case "MS2":
		playerResources.Min5 = value
	case "MS3":
		playerResources.Min10 = value
	case "MS4":
		playerResources.Min30 = value
	case "MS5":
		playerResources.Hr1 = value
	case "MS6":
		playerResources.Hr5 = value
	case "MS7":
		playerResources.Hr24 = value
	}
}
