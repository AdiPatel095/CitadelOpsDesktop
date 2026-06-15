package equipmentview

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	equip "CitadelDesktop/Server/Models/Equipment"
	"CitadelDesktop/Server/ResponseRegistry"
)

const (
	maxUpgradeLevel          = 50
	upgradeEQEquipment       = 1
	upgradeEQGem             = 0
	upgradeDefaultC2         = 0
	upgradeIgnoredErrorCode  = "227" // no item payload — skip level update, retry next ere
	upgradeInsufficientCoins = "10"  // not enough coins for next upgrade step
	upgradeMenuWait          = 2 * time.Second
	defaultUpgradeEreDelayMs = 50
	minUpgradeEreDelayMs     = 10
	maxUpgradeEreDelayMs     = 5000
	upgradeResponseWait      = 8 * time.Second
	upgradeCoinUpdateWait    = 2 * time.Second
)

// UpgradeResult is the outcome of one or more **ere** upgrade steps.
type UpgradeResult struct {
	Success      bool   `json:"success"`
	Code         string `json:"code,omitempty"`
	Message      string `json:"message"`
	StartLevel   int    `json:"startLevel"`
	FinalLevel   int    `json:"finalLevel"`
	UpgradesDone int    `json:"upgradesDone"`
	TargetLevel  int    `json:"targetLevel"`
}

// SlotUpgradeInfo describes one equippable slot for the upgrade UI.
type SlotUpgradeInfo struct {
	SlotNumber int     `json:"slotNumber"`
	ItemID     float64 `json:"itemId"`
	Level      float64 `json:"level"`
	Label      string  `json:"label"`
}

// UpgradeInfoPayload is returned to the frontend for upgrade modals.
type UpgradeInfoPayload struct {
	Equipment []SlotUpgradeInfo `json:"equipment"`
	Gems      []SlotUpgradeInfo `json:"gems"`
}

var equipmentSlotLabels = map[int]string{
	1: "Armor",
	2: "Weapon",
	3: "Helmet",
	4: "Artifact",
	6: "Hero",
}

var gemSlotLabels = map[int]string{
	1: "Gem Slot 1",
	2: "Gem Slot 2",
	3: "Gem Slot 3",
	4: "Gem Slot 4",
}

// BuildUpgradeInfo collects equipped item/gem levels for a commander or castellan.
func BuildUpgradeInfo(equipmentMode string, targetIndex int) UpgradeInfoPayload {
	out := UpgradeInfoPayload{
		Equipment: []SlotUpgradeInfo{},
		Gems:      []SlotUpgradeInfo{},
	}
	for slot, label := range equipmentSlotLabels {
		id := equippedItemID(equipmentMode, targetIndex, slot)
		if id == 0 {
			continue
		}
		lvl, _ := findItemLevel(id, upgradeEQEquipment)
		out.Equipment = append(out.Equipment, SlotUpgradeInfo{
			SlotNumber: slot,
			ItemID:     id,
			Level:      lvl,
			Label:      label,
		})
	}
	for slot, label := range gemSlotLabels {
		gemID := equippedGemID(equipmentMode, targetIndex, slot)
		if gemID == 0 {
			continue
		}
		lvl, _ := findItemLevel(gemID, upgradeEQGem)
		out.Gems = append(out.Gems, SlotUpgradeInfo{
			SlotNumber: slot,
			ItemID:     gemID,
			Level:      lvl,
			Label:      label,
		})
	}
	return out
}

// UpgradeEquipmentToLevel runs **ere** (EQ=1) until targetLevel or an error.
// RIID is resolved once from the equipped slot before the upgrade loop starts.
func UpgradeEquipmentToLevel(equipmentMode string, targetIndex int, slotNumber int, expectedItemID float64, targetLevel int) UpgradeResult {
	riid := equippedItemID(equipmentMode, targetIndex, slotNumber)
	if riid == 0 {
		return UpgradeResult{Success: false, Message: "No equipment in that slot"}
	}
	if expectedItemID != 0 && riid != expectedItemID {
		log.Printf("[Upgrade] equipment id mismatch slot %d: client %.0f live %.0f (using live)", slotNumber, expectedItemID, riid)
	}
	return upgradeToLevel(riid, upgradeEQEquipment, targetLevel)
}

// UpgradeGemToLevel runs **ere** (EQ=0) until targetLevel or an error.
// RIID is resolved once from the equipped gem slot before the upgrade loop starts.
func UpgradeGemToLevel(equipmentMode string, targetIndex int, slotNumber int, expectedGemID float64, targetLevel int) UpgradeResult {
	riid := equippedGemID(equipmentMode, targetIndex, slotNumber)
	if riid == 0 {
		return UpgradeResult{Success: false, Message: "No gem in that slot"}
	}
	if expectedGemID != 0 && riid != expectedGemID {
		log.Printf("[Upgrade] gem id mismatch slot %d: client %.0f live %.0f (using live)", slotNumber, expectedGemID, riid)
	}
	return upgradeToLevel(riid, upgradeEQGem, targetLevel)
}

// UpgradeBlockedByCoinReserve reports whether upgrades should be denied at the current coin balance.
func UpgradeBlockedByCoinReserve() (blocked bool, message string) {
	settings := Models.GetSettingsState()
	coins := Models.GetGameState().GlobalResources.Coins
	if !settings.CoinsUnderUpgradeReserve(coins) {
		return false, ""
	}
	return true, upgradeCoinReserveMessage(coins, settings.UpgradeCoinReserveThreshold())
}

func upgradeToLevel(riid float64, eqFlag int, targetLevel int) UpgradeResult {
	if targetLevel > maxUpgradeLevel {
		return UpgradeResult{Success: false, Message: fmt.Sprintf("Target level cannot exceed %d", maxUpgradeLevel)}
	}
	if targetLevel < 1 {
		return UpgradeResult{Success: false, Message: "Target level must be at least 1"}
	}

	if blocked, msg := UpgradeBlockedByCoinReserve(); blocked {
		log.Printf("[Upgrade] stopping before loop: %s", msg)
		return UpgradeResult{Success: false, Message: msg}
	}

	GameCommands.SendUpgradeMenuRefresh()
	time.Sleep(upgradeMenuWait)

	currentLevel, ok := findItemLevel(riid, eqFlag)
	if !ok {
		return UpgradeResult{Success: false, Message: "Item not found — refresh equipment and try again"}
	}
	startLevel := int(currentLevel)
	if targetLevel <= startLevel {
		return UpgradeResult{
			Success:     false,
			Message:     fmt.Sprintf("Target level must be above current level (%d)", startLevel),
			StartLevel:  startLevel,
			FinalLevel:  startLevel,
			TargetLevel: targetLevel,
		}
	}

	log.Printf("[Upgrade] starting loop RIID=%.0f EQ=%d level %d -> %d", riid, eqFlag, startLevel, targetLevel)

	done := 0
	level := startLevel
	for level < targetLevel {
		if blocked, msg := UpgradeBlockedByCoinReserve(); blocked {
			log.Printf("[Upgrade] stopping: %s", msg)
			return UpgradeResult{
				Success:      false,
				Message:      msg,
				StartLevel:   startLevel,
				FinalLevel:   level,
				UpgradesDone: done,
				TargetLevel:  targetLevel,
			}
		}

		newLevel, code, ignored, err := sendSingleUpgrade(riid, eqFlag)
		if err != nil {
			return UpgradeResult{
				Success:      false,
				Code:         code,
				Message:      err.Error(),
				StartLevel:   startLevel,
				FinalLevel:   level,
				UpgradesDone: done,
				TargetLevel:  targetLevel,
			}
		}
		if ignored {
			log.Printf("[Upgrade] ignoring response code %s (no item info), retrying", code)
			time.Sleep(upgradeEreStepDelay())
			continue
		}
		done++
		if newLevel > 0 {
			level = newLevel
		} else {
			level++
		}
		// Game pushes **gcu** after each successful **ere**; wait in sendSingleUpgrade, then re-check reserve.
		if blocked, msg := UpgradeBlockedByCoinReserve(); blocked {
			log.Printf("[Upgrade] stopping: %s", msg)
			return UpgradeResult{
				Success:      false,
				Message:      msg,
				StartLevel:   startLevel,
				FinalLevel:   level,
				UpgradesDone: done,
				TargetLevel:  targetLevel,
			}
		}
		time.Sleep(upgradeEreStepDelay())
	}

	return UpgradeResult{
		Success:      true,
		Message:      fmt.Sprintf("Upgraded to level %d (%d step(s))", level, done),
		StartLevel:   startLevel,
		FinalLevel:   level,
		UpgradesDone: done,
		TargetLevel:  targetLevel,
	}
}

func sendSingleUpgrade(riid float64, eqFlag int) (newLevel int, code string, ignored bool, err error) {
	waitTypes := []string{"ere"}
	if eqFlag == upgradeEQEquipment {
		waitTypes = append(waitTypes, "eqe")
	} else {
		waitTypes = append(waitTypes, "gsue", "guse")
	}

	ch, cleanup := registerMultiWaiters(waitTypes, upgradeResponseWait)
	defer cleanup()

	coinGenBefore := GameParser.CoinUpdateGeneration()
	GameCommands.SendERE(riid, eqFlag, upgradeDefaultC2)

	response, waitErr := waitOnChannel(ch, upgradeResponseWait)
	if waitErr != nil {
		return 0, "", false, fmt.Errorf("timeout waiting for upgrade response")
	}

	ok, skip, parsedLevel, respCode, msg := parseUpgradeResponse(response, eqFlag)
	if skip {
		return 0, respCode, true, nil
	}
	if !ok {
		return 0, respCode, false, fmt.Errorf("%s", msg)
	}

	waitForCoinUpdateAfter(coinGenBefore, upgradeCoinUpdateWait)
	return parsedLevel, respCode, false, nil
}

func waitForCoinUpdateAfter(since uint64, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if GameParser.CoinUpdateGeneration() > since {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	log.Printf("[Upgrade] coin update wait timed out (generation still %d)", since)
}

func registerMultiWaiters(types []string, timeout time.Duration) (chan []string, func()) {
	ch := make(chan []string, 1)
	var waiters []*ResponseRegistry.ResponseWaiter
	var once sync.Once

	deliver := func(parts []string) {
		once.Do(func() {
			select {
			case ch <- parts:
			default:
			}
		})
	}

	for _, t := range types {
		w := ResponseRegistry.Global.RegisterWaiterWithDeliver(t, timeout, deliver)
		waiters = append(waiters, w)
	}

	cleanup := func() {
		for _, w := range waiters {
			w.Cleanup()
		}
	}
	return ch, cleanup
}

func waitOnChannel(ch chan []string, timeout time.Duration) ([]string, error) {
	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
		return nil, ResponseRegistry.ErrTimeout
	}
}

func parseUpgradeResponse(parts []string, eqFlag int) (success bool, ignored bool, newLevel int, code string, message string) {
	if len(parts) <= 4 {
		return false, false, 0, "", "Invalid response format"
	}

	if len(parts) > 5 && parts[5] != "" {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(parts[5]), &payload); err == nil {
			GameParser.UpdateCoinsFromPayload(payload)
			if eqFlag == upgradeEQEquipment {
				if lvl, ok := GameParser.EquipmentLevelFromEQEPayload(payload); ok {
					return true, false, lvl, "0", "Success"
				}
			}
		}
	}

	code = parts[4]
	if code == upgradeIgnoredErrorCode {
		return false, true, 0, code, ""
	}
	if code == "0" {
		return true, false, 0, code, "Success"
	}
	return false, false, 0, code, upgradeErrorMessage(code)
}

func upgradeErrorMessage(code string) string {
	switch code {
	case upgradeInsufficientCoins:
		return "Insufficient coins"
	case "451", "452", "453":
		return "Insufficient relic shards or coins"
	default:
		return fmt.Sprintf("Upgrade failed (code %s) — insufficient resources or item at max level", code)
	}
}

func equippedItemID(equipmentMode string, targetIndex, slotNumber int) float64 {
	if equipmentMode == "Commander" {
		if targetIndex < 0 || targetIndex >= len(equip.CommStatArray) {
			return 0
		}
		comm := equip.CommStatArray[targetIndex]
		switch slotNumber {
		case 1:
			return comm.Equip1
		case 2:
			return comm.Equip2
		case 3:
			return comm.Equip3
		case 4:
			return comm.Equip4
		case 6:
			return comm.Hero
		}
		return 0
	}
	if equipmentMode == "Castellan" {
		cast := GetCastellanStat(targetIndex)
		switch slotNumber {
		case 1:
			return cast.Equip1
		case 2:
			return cast.Equip2
		case 3:
			return cast.Equip3
		case 4:
			return cast.Equip4
		case 6:
			return cast.Hero
		}
	}
	return 0
}

func equippedGemID(equipmentMode string, targetIndex, slotNumber int) float64 {
	if equipmentMode == "Commander" {
		if targetIndex < 0 || targetIndex >= len(equip.CommStatArray) {
			return 0
		}
		comm := equip.CommStatArray[targetIndex]
		switch slotNumber {
		case 1:
			return comm.Gem1
		case 2:
			return comm.Gem2
		case 3:
			return comm.Gem3
		case 4:
			return comm.Gem4
		}
		return 0
	}
	if equipmentMode == "Castellan" {
		cast := GetCastellanStat(targetIndex)
		switch slotNumber {
		case 1:
			return cast.Gem1
		case 2:
			return cast.Gem2
		case 3:
			return cast.Gem3
		case 4:
			return cast.Gem4
		}
	}
	return 0
}

func findItemLevel(riid float64, eqFlag int) (float64, bool) {
	gs := Models.GetGameState()
	if eqFlag == upgradeEQEquipment {
		for _, eq := range gs.Equipment.EquipmentStorage {
			if eq.ID == riid {
				return eq.EquipLevel, true
			}
		}
		for i := range gs.Equipment.CommActualArray {
			for _, eq := range gs.Equipment.CommActualArray[i].Equipment {
				if eq.ID == riid {
					return eq.EquipLevel, true
				}
			}
		}
		for i := range gs.Equipment.CastActualArray {
			for _, eq := range gs.Equipment.CastActualArray[i].Equipment {
				if eq.ID == riid {
					return eq.EquipLevel, true
				}
			}
		}
		return 0, false
	}

	for _, gem := range gs.Equipment.GemsStorage {
		if gem.ID == riid {
			return gem.GemLevel, true
		}
	}
	for i := range gs.Equipment.CommActualArray {
		for _, eq := range gs.Equipment.CommActualArray[i].Equipment {
			if eq.GemSlot.Gem != nil && eq.GemSlot.Gem.ID == riid {
				return eq.GemSlot.Gem.GemLevel, true
			}
		}
	}
	for i := range gs.Equipment.CastActualArray {
		for _, eq := range gs.Equipment.CastActualArray[i].Equipment {
			if eq.GemSlot.Gem != nil && eq.GemSlot.Gem.ID == riid {
				return eq.GemSlot.Gem.GemLevel, true
			}
		}
	}
	return 0, false
}

func upgradeEreStepDelay() time.Duration {
	ms := Models.GetSettingsState().UpgradeEreDelayMs
	if ms <= 0 {
		ms = defaultUpgradeEreDelayMs
	}
	if ms < minUpgradeEreDelayMs {
		ms = minUpgradeEreDelayMs
	}
	if ms > maxUpgradeEreDelayMs {
		ms = maxUpgradeEreDelayMs
	}
	return time.Duration(ms) * time.Millisecond
}

func upgradeCoinReserveMessage(coins, threshold float64) string {
	return fmt.Sprintf("Coins under threshold (%.0f <= %.0f)", coins, threshold)
}
