package GameParser

import (
	"CitadelDesktop/Server/Models"
)

func UpdateCoins(gcuMap map[string]interface{}) {
	gs := Models.GetGameState()
	coins, ok := gcuMap["C1"].(float64)
	if ok {
		gs.GlobalResources.Coins = coins
	}
	rubies, ok := gcuMap["C2"].(float64)
	if ok {
		gs.GlobalResources.Rubies = rubies
	}
}

func UpdateMight(gmuMap map[string]interface{}) {
	might, ok := gmuMap["MP"].(float64)
	if ok {
		Models.GetGameState().GlobalResources.MightPt = might
	}
}

func UpdateGlory(ufaMap map[string]interface{}) {
	glory, ok := ufaMap["CF"].(float64)
	if ok {
		Models.GetGameState().GlobalResources.GloryPt = glory
	}
}

func UpdateGallantry(ufpMap map[string]interface{}) {
	gallantry, ok := ufpMap["CFP"].(float64)
	if ok {
		Models.GetGameState().GlobalResources.GallanPt = gallantry
	}
}

func UpdateSCE(sceArray []interface{}) {
	for _, item := range sceArray {
		valueArray, ok := item.([]interface{})
		if !ok || len(valueArray) != 2 {
			continue
		}

		label, labelOk := valueArray[0].(string)
		value, valueOk := valueArray[1].(float64)

		if !labelOk || !valueOk {
			continue
		}
		updateResourceByLabel(label, value)
	}
}

func updateResourceByLabel(label string, value float64) {
	playerResources := &Models.GetGameState().GlobalResources
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
	case "PTT":
		playerResources.PTT = value
	}
}

// UpdateAlliance parses the alliance data from gal and updates the global Alliance
func UpdateAlliance(galMap map[string]interface{}) {
	aid, ok := galMap["AID"].(float64)
	if ok {
		Models.GetGameState().Alliance.AID = int(aid)
	}
}

// UpdatePlayerInfo parses the player information from gpi and stores the PlayerID
func UpdatePlayerInfo(gpiMap map[string]interface{}) {
	pid, ok := gpiMap["PID"].(float64)
	if ok {
		Models.GetGameState().PlayerID = int(pid)
	}
}
