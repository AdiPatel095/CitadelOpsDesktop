package GameParser

import (
	"sync/atomic"

	"CitadelDesktop/Server/Models"
)

// NotifyGlobalResourcesChanged is wired by FrontendWebsocket to push globalResourceUpdate after **gcu**.
var NotifyGlobalResourcesChanged func()

var coinUpdateGeneration uint64

// CoinUpdateGeneration increments on each **gcu** coin/ruby apply (used to sync upgrade loop with live balances).
func CoinUpdateGeneration() uint64 {
	return atomic.LoadUint64(&coinUpdateGeneration)
}

func bumpCoinUpdateGeneration() {
	atomic.AddUint64(&coinUpdateGeneration, 1)
}

func UpdateCoins(gcuMap map[string]interface{}) {
	gs := Models.GetGameState()
	changed := false
	// Live **gcu** during **ere** upgrades: spend decreases **C1**; **C2** is rubies (see websocket captures).
	if coins, ok := gcuMap["C1"].(float64); ok {
		gs.GlobalResources.Coins = coins
		changed = true
	}
	if rubies, ok := gcuMap["C2"].(float64); ok {
		gs.GlobalResources.Rubies = rubies
		changed = true
	}
	if changed {
		bumpCoinUpdateGeneration()
	}
	if NotifyGlobalResourcesChanged != nil {
		NotifyGlobalResourcesChanged()
	}
}

// UpdateCoinsFromPayload applies **gcu**-shaped or nested `gcu` coin updates from a response body.
func UpdateCoinsFromPayload(payload map[string]interface{}) {
	if payload == nil {
		return
	}
	if nested, ok := payload["gcu"].(map[string]interface{}); ok {
		UpdateCoins(nested)
		return
	}
	if _, ok := payload["C1"].(float64); ok {
		UpdateCoins(payload)
		return
	}
	if _, ok := payload["C2"].(float64); ok {
		UpdateCoins(payload)
	}
}

// UpdateSCEFromPayload applies nested `sce` account currency updates from a response body.
func UpdateSCEFromPayload(payload map[string]interface{}) {
	if payload == nil {
		return
	}
	if nested, ok := payload["sce"].([]interface{}); ok {
		UpdateSCE(nested)
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
	changed := false
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
		if updateResourceByLabel(label, value) {
			changed = true
		}
	}
	if changed && NotifyGlobalResourcesChanged != nil {
		NotifyGlobalResourcesChanged()
	}
}

func updateResourceByLabel(label string, value float64) bool {
	playerResources := &Models.GetGameState().GlobalResources
	switch label {
	case "STP":
		playerResources.Sceat = value
	case "IDCT":
		playerResources.Ducat = value
	case "LM":
		playerResources.ConstToken = value
	case "LT":
		playerResources.LegendaryToken = value
		playerResources.UpgrToken = value
	case "SLWT":
		playerResources.AfflTix = value
	case "PL":
		playerResources.Plaster = value
	case "DST":
		playerResources.DrgScale = value
	case "DSS":
		playerResources.DrgSpl = value
	case "DGA":
		playerResources.DrgGlassArrow = value
	case "DSAM":
		playerResources.DrgScaleArmor = value
	case "DSAW":
		playerResources.DrgScaleArrow = value
	case "TFA":
		playerResources.TwinFlameAxes = value
	case "CO1":
		playerResources.Component1 = value
	case "CO2":
		playerResources.Component2 = value
	case "CO3":
		playerResources.Component3 = value
	case "CO4":
		playerResources.Component4 = value
	case "CO5":
		playerResources.Component5 = value
	case "CO6":
		playerResources.Component6 = value
	case "CO7":
		playerResources.Component7 = value
	case "CO8":
		playerResources.Component8 = value
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
	default:
		return false
	}
	return true
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

// UpdateVIPInfo parses gbd.vip and stores the active player VIP state.
func UpdateVIPInfo(vipMap map[string]interface{}) {
	Models.GetGameState().VIP = Models.VipState{
		Points:       jsonIntFromMap(vipMap, "VP"),
		Level:        jsonIntFromMap(vipMap, "VRL"),
		RemainingSec: jsonIntFromMap(vipMap, "VRS"),
		Upgrade:      jsonIntFromMap(vipMap, "UPG"),
	}
}
