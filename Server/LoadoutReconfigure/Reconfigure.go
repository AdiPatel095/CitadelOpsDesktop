package LoadoutReconfigure

import (
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/Models"
	"time"
)

type ReconfigurePayload struct {
	HardwareID          string  `json:"hardwareID"`
	EquipmentMode       string  `json:"equipmentMode"`
	CombatMode          string  `json:"combatMode"`
	InterTierMultiplier float64 `json:"interTierMultiplier"`
	IntraTierMultiplier float64 `json:"intraTierMultiplier"`
	TargetIndex         int     `json:"targetIndex"`
	Stats               []struct {
		Stat     string `json:"stat"`
		Tier     int    `json:"tier"`
		Position int    `json:"position"`
	} `json:"stats"`
}

func ReconfigureCommander(payload ReconfigurePayload) Models.CommStatModel {
	// 1. Prepare Game State (Verify/Refresh Data if needed)
	// The original code sent Update messages via websocket. We might still want to do that?
	// But assuming data is in Models.EquipmentStorage, we run the optimizer.

	// Trigger refresh first if safest, or assume client handles it.
	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)
	// ... (This sync logic might need to remain if the data is stale)
	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	// For now, calculating based on current state.

	// Filter storage for Commander equipment (EquipType == 2)
	filteredStorage := make([]Models.EquipmentModel, 0, len(Models.EquipmentStorage))
	for _, item := range Models.EquipmentStorage {
		if item.EquipType == 2 {
			filteredStorage = append(filteredStorage, item)
		}
	}

	// Pass global storage to the optimizer

	result := RunOptimization(payload, filteredStorage, Models.GemsStorage)
	return result
}
