//go:build ignore

// Archived Beri World automation (2026-03). Excluded from builds via //go:build ignore.
// To restore: copy this file to ../BeriWorld.go, remove the build tag and this header comment,
// then re-wire Client Sidebar + AuthContext and IncomingMessagesFrontend start/stop cases.

package GameFunctions

import (
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	beriWorldCancel context.CancelFunc
	beriWorldMu     sync.Mutex

	// IdealBeriMap stores the ideal layout for the Beri World castle.
	// Key: WID (Building Type ID), Value: Array of [x, y, rotation] arrays.
	IdealBeriMap = map[int][][]int{
		233: {{180, 180, 0}},                                              // Main Tent
		246: {{186, 180, 0}, {192, 180, 0}, {198, 180, 0}, {204, 180, 0}}, // Supply Storehouses
		339: {
			{180, 186, 1}, {185, 186, 1}, {190, 186, 1}, {195, 186, 1},
			{202, 185, 0}, {206, 185, 0}, {210, 185, 0},
			{180, 190, 0}, {184, 190, 0}, {188, 190, 0}, {192, 190, 0}, {196, 190, 0}, {200, 190, 0}, {204, 190, 0}, {208, 190, 0}, {212, 190, 0}, {216, 190, 0}, {220, 190, 0}, {224, 190, 0}, {228, 190, 0}, {232, 190, 0}, {236, 190, 0},
			{180, 195, 0}, {184, 195, 0}, {188, 195, 0}, {192, 195, 0}, {196, 195, 0}, {200, 195, 0}, {204, 195, 0}, {208, 195, 0}, {212, 195, 0}, {216, 195, 0}, {220, 195, 0}, {224, 195, 0}, {228, 195, 0}, {232, 195, 0}, {236, 195, 0},
			{180, 200, 0}, {184, 200, 0}, {188, 200, 0}, {192, 200, 0}, {196, 200, 0}, {200, 200, 0}, {204, 200, 0}, {208, 200, 0}, {212, 200, 0}, {216, 200, 0}, {220, 200, 0}, {224, 200, 0}, {228, 200, 0}, {232, 200, 0}, {236, 200, 0},
		}, // Drill Grounds
		242: {
			{180, 210, 0}, {185, 210, 0}, {190, 210, 0}, {195, 210, 0}, {200, 210, 0}, {205, 210, 0}, {210, 210, 0}, {215, 210, 0}, {220, 210, 0}, {225, 210, 0}, {230, 210, 0}, {235, 210, 0},
			{180, 215, 0}, {185, 215, 0}, {190, 215, 0}, {195, 215, 0}, {200, 215, 0}, {205, 215, 0}, {210, 215, 0}, {215, 215, 0}, {220, 215, 0}, {225, 215, 0}, {230, 215, 0}, {235, 215, 0},
			{180, 220, 0}, {185, 220, 0}, {190, 220, 0}, {195, 220, 0}, {200, 220, 0}, {205, 220, 0}, {210, 220, 0}, {215, 220, 0}, {220, 220, 0}, {225, 220, 0}, {230, 220, 0}, {235, 220, 0},
			{180, 225, 0}, {185, 225, 0}, {190, 225, 0}, {195, 225, 0}, {200, 225, 0}, {205, 225, 0}, {210, 225, 0}, {215, 225, 0}, {220, 225, 0}, {225, 225, 0}, {230, 225, 0}, {235, 225, 0},
			{180, 230, 0}, {185, 230, 0}, {190, 230, 0}, {195, 230, 0}, {200, 230, 0}, {205, 230, 0}, {210, 230, 0}, {215, 230, 0}, {220, 230, 0}, {225, 230, 0}, {230, 230, 0}, {235, 230, 0},
			{180, 235, 0}, {185, 235, 0}, {190, 235, 0}, {195, 235, 0}, {200, 235, 0}, {205, 235, 0}, {210, 235, 0}, {215, 235, 0}, {220, 235, 0}, {225, 235, 0}, {230, 235, 0}, {235, 235, 0},
		}, // Small Tents
		247: {
			{225, 185, 0},
		},
		468: {
			{225, 180, 0},
		},
		627: {
			{230, 180, 0},
		},
		622: {
			{235, 180, 0},
		},
	}
)

// IsBeriWorldRunning returns true if the Beri World goroutine is currently active
func IsBeriWorldRunning() bool {
	beriWorldMu.Lock()
	defer beriWorldMu.Unlock()
	return beriWorldCancel != nil
}

// StartBeriWorld starts the Beri World goroutine. If already running, it does nothing.
func StartBeriWorld() {
	beriWorldMu.Lock()
	defer beriWorldMu.Unlock()

	Models.GetSettingsState().BeriWorldEnabled = true

	if beriWorldCancel != nil {
		log.Println("[INFO] Beri World is already running.")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	beriWorldCancel = cancel

	log.Println("[INFO] Starting Beri World event loop...")
	go runBeriWorld(ctx)
}

// StopBeriWorld stops the Beri World goroutine if running.
func StopBeriWorld() {
	beriWorldMu.Lock()
	defer beriWorldMu.Unlock()

	Models.GetSettingsState().BeriWorldEnabled = false

	if beriWorldCancel == nil {
		return
	}

	log.Println("[INFO] Stopping Beri World event loop...")
	beriWorldCancel()
	beriWorldCancel = nil
}

// runBeriWorld is the main event loop for the Beri World feature.
func runBeriWorld(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[INFO] Beri World execution halted.")
			return
		case <-ticker.C:
			if !Models.GetSettingsState().BeriWorldEnabled {
				continue
			}
			processBeriWorld()
		}
	}
}

func processBeriWorld() {
	gs := Models.GetGameState()
	if gs.Castle.BeriWorldCastle.Aid == 0 {
		return
	}

	// 1. Fetch Fresh Data (Perform JAA)
	log.Printf("[BeriWorld] Fetching fresh data for Beri Castle (AID %.0f)", gs.Castle.BeriWorldCastle.Aid)
	// FetchCastleTroops uses JAA and populates buildings now
	troops := GameParser.FetchCastleTroops(10, int(gs.Castle.BeriWorldCastle.Aid), int(gs.Castle.BeriWorldCastle.Troops.X), int(gs.Castle.BeriWorldCastle.Troops.Y))
	if troops == nil {
		log.Println("[BeriWorld] Failed to fetch castle data via JAA.")
		return
	}

	castle := gs.GetCastleByID(int(gs.Castle.BeriWorldCastle.Aid))
	if castle == nil {
		return
	}
	// Data is already saved to GS by FetchCastleTroops, but we ensure consistency here
	Models.SetCastleBuildingRows(castle, troops.BGRows, troops.BDRows)
	castle.Troops.TroopsI = troops.TroopsI

	// 2. Expansion Check
	if !expansionCheck() {
		performRubyExpansionCheck()
	}

	// 3. Building Position / Move Check
	movedAny := moveBuildingsToIdeal(castle)
	if movedAny {
		return // Wait for next tick if we moved something to let state update
	}

	// 4. Build Commands
	performBuildCommands(castle)
}

// expansionCheck is a placeholder for future expansion logic.
func expansionCheck() bool {
	// For now, return false to always check ruby costs from log if needed.
	return false
}

func performRubyExpansionCheck() {
	gs := Models.GetGameState()
	content, err := os.ReadFile("ebe_costs.log")
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "EBE: ") {
			continue
		}

		// Format: EBE: {payload} | Actual Cost: {cost} | ...
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		payloadStr := strings.TrimSpace(strings.TrimPrefix(parts[0], "EBE: "))
		costPart := strings.TrimSpace(strings.TrimPrefix(parts[1], "Actual Cost: "))
		cost, _ := strconv.ParseFloat(costPart, 64)

		if gs.GlobalResources.Rubies >= cost {
			// In a real scenario, we'd check if this expansion is already owned.
			// Sending ebe command:
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ebe%%1%%%s%%`, payloadStr)
			log.Printf("[BeriWorld] Sending expansion (cost %.0f): %s", cost, payloadStr)
			ResponseRegistry.OutgoingMessages <- []byte(payload)
			time.Sleep(1 * time.Second) // Small delay
		}
	}
}

func moveBuildingsToIdeal(castle *Models.PlayerCastleInfo) bool {
	all := castle.AllBuildingRows()
	for i := range all {
		b := &all[i]
		idealsForType, exists := IdealBeriMap[b.BuildingID]
		if !exists {
			continue
		}

		// Is current position one of the ideal positions?
		isCurrentIdeal := false
		for _, ip := range idealsForType {
			if b.X == ip[0] && b.Y == ip[1] {
				isCurrentIdeal = true
				break
			}
		}

		if !isCurrentIdeal {
			// Building at non-ideal location. Find an empty ideal location.
			var targetPos []int
			for _, ip := range idealsForType {
				occupied := false
				for _, otherB := range all {
					if otherB.X == ip[0] && otherB.Y == ip[1] {
						occupied = true
						break
					}
				}
				if !occupied {
					targetPos = ip
					break
				}
			}

			if targetPos != nil {
				log.Printf("[BeriWorld] Moving building WID %d (OID %d) from (%d, %d) to ideal (%d, %d, R:%d)",
					b.BuildingID, b.OID, b.X, b.Y, targetPos[0], targetPos[1], targetPos[2])
				payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%emo%%1%%{"OID":%d,"X":%d,"Y":%d,"R":%d}%%`, b.OID, targetPos[0], targetPos[1], targetPos[2])
				ResponseRegistry.OutgoingMessages <- []byte(payload)
				return true
			}
		}
	}
	return false
}

func performBuildCommands(castle *Models.PlayerCastleInfo) {
	if !hasEnoughResources(castle) {
		log.Println("[BeriWorld] Not enough resources to build. Waiting...")
		return
	}

	for wid, ideals := range IdealBeriMap {
		for _, ip := range ideals {
			x, y, r := ip[0], ip[1], ip[2]

			alreadyBuilt := false
			for _, b := range castle.AllBuildingRows() {
				if b.BuildingID == wid && b.X == x && b.Y == y {
					alreadyBuilt = true
					break
				}
			}

			if !alreadyBuilt {
				log.Printf("[BeriWorld] Proposed building WID %d at (%d, %d, R:%d)", wid, x, y, r)
				payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ebu%%1%%{"WID":%d,"X":%d,"Y":%d,"R":%d,"PWR":0,"PO":-1,"DOID":-1}%%`, wid, x, y, r)
				ResponseRegistry.OutgoingMessages <- []byte(payload)
				time.Sleep(2 * time.Second)
				return // Build one at a time
			}
		}
	}
}

func hasEnoughResources(castle *Models.PlayerCastleInfo) bool {
	// Minimum safety threshold for wood and stone
	return castle.Amount.WoodAmount > 3000 && castle.Amount.StoneAmount > 3000
}
