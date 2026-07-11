package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
	"sync"
)

// InitiateDetails parses the bundled gbd JSON and updates game state. Missing sections are skipped (logged); never panics the process.
func InitiateDetails(data string) {
	var jsonDataMap map[string]interface{}
	if err := json.Unmarshal([]byte(data), &jsonDataMap); err != nil {
		log.Printf("[parser] gbd unmarshal: %v", err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(8)

	go func() {
		defer wg.Done()
		gcuMap, ok := jsonDataMap["gcu"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid gcu")
			return
		}
		UpdateCoins(gcuMap)
	}()

	go func() {
		defer wg.Done()
		sceArray, ok := jsonDataMap["sce"].([]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid sce")
			return
		}
		UpdateSCE(sceArray)
	}()

	go func() {
		defer wg.Done()
		gmuMap, ok := jsonDataMap["gmu"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid gmu")
			return
		}
		UpdateMight(gmuMap)
	}()

	go func() {
		defer wg.Done()
		ufaMap, ok := jsonDataMap["ufa"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid ufa")
			return
		}
		UpdateGlory(ufaMap)
	}()

	go func() {
		defer wg.Done()
		ufpMap, ok := jsonDataMap["ufp"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid ufp")
			return
		}
		UpdateGallantry(ufpMap)
	}()

	go func() {
		defer wg.Done()
		gclMap, ok := jsonDataMap["gcl"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid gcl")
			return
		}
		dclMap, ok := jsonDataMap["dcl"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid dcl")
			return
		}
		CastleDetailParser(gclMap, dclMap)
		gliMap, ok := jsonDataMap["gli"].(map[string]interface{})
		if !ok {
			log.Printf("[parser] gbd: missing or invalid gli")
			return
		}
		UpdateEquipmentList(gliMap)
	}()

	go func() {
		defer wg.Done()
		if galMap, ok := jsonDataMap["gal"].(map[string]interface{}); ok {
			UpdateAlliance(galMap)
		} else {
			log.Printf("[parser] gbd: gal missing or not an object")
		}
		if gpiMap, ok := jsonDataMap["gpi"].(map[string]interface{}); ok {
			UpdatePlayerInfo(gpiMap)
		} else {
			log.Printf("[parser] gbd: gpi missing or not an object")
		}
		if vipMap, ok := jsonDataMap["vip"].(map[string]interface{}); ok {
			UpdateVIPInfo(vipMap)
		}
		if subscriptionMap, ok := jsonDataMap["sie"].(map[string]interface{}); ok {
			UpdateSubscriptionInfo(subscriptionMap)
		}
		if subscriptionMap, ok := jsonDataMap["upc"].(map[string]interface{}); ok {
			UpdateSubscriptionInfo(subscriptionMap)
		}
	}()

	go func() {
		defer wg.Done()
		HandleLoginInboxBattleReports(jsonDataMap)
		HandleLoginInboxSpyReports(jsonDataMap)
	}()

	wg.Wait()
	if Models.GetGameState().PlayerID > 0 {
		go Models.PersistGameStateSnapshot()
	}
}
