package GameCommands

import (
	"CitadelDesktop/Server/ResponseRegistry"
	"time"
)

// CDSHBWPTT is one (HBW, PTT) pair for EmpireEx_21 **cds**.
// HBW=0, PTT=0 is rejected (cds code 5 in captures); these two are what the app uses in practice.
type CDSHBWPTT struct {
	HBW int
	PTT int
}

// Active CDS pairs (websocket_game.log, 2026-04-10).
var (
	// CDSHBWNegOnePTTOne is the common client-style bird send (-1 / 1).
	CDSHBWNegOnePTTOne = CDSHBWPTT{HBW: -1, PTT: 1}
	// CDSHBW1001PTTZero is the alternate working pair (1001 / 0).
	CDSHBW1001PTTZero = CDSHBWPTT{HBW: 1001, PTT: 0}
)

// Archived pairs (also returned cds code 0 in manual tests); kept for reference only — not used by the app.
//   {HBW: 1002, PTT: 0}
//   {HBW: 1003, PTT: 0}

const cdsPTTThreshold = 50000

// cdsPairsForGamePTT returns up to two pairs: preferred first (from GameState.GlobalResources.PTT), then the other as fallback.
// When game PTT > threshold, prefer the CDS pair that sends PTT=1 (HBW -1); otherwise prefer 1001/0.
func cdsPairsForGamePTT(gamePTT float64) []CDSHBWPTT {
	if gamePTT > cdsPTTThreshold {
		return []CDSHBWPTT{CDSHBWNegOnePTTOne, CDSHBW1001PTTZero}
	}
	return []CDSHBWPTT{CDSHBW1001PTTZero, CDSHBWNegOnePTTOne}
}

const cdsWaitPerVariant = 5 * time.Second

// cdsIncomingOK reports whether %-split **cds** response indicates success (code "0" at index 4).
func cdsIncomingOK(parts []string) bool {
	return len(parts) > 4 && parts[4] == "0"
}

// SendCDSUntilSuccess sends **cds** using HBW/PTT pairs chosen from gamePTT (GlobalResources.PTT):
//   - gamePTT > 50000 → try (-1,1) first, then (1001,0)
//   - otherwise       → try (1001,0) first, then (-1,1)
//
// Stops on first cds code 0.
// onCDSResponse is optional: invoked with a copy of the inbound **cds** frame (%-split) before the waiter unblocks, so callers can parse **gam**-like JSON (e.g. **TT**).
func SendCDSUntilSuccess(castleAID, targetX, targetY, sdiLID, delayHours int, gamePTT float64, troopsJSON string, onCDSResponse func([]string)) bool {
	pairs := cdsPairsForGamePTT(gamePTT)
	for i, v := range pairs {
		waiter := ResponseRegistry.Global.RegisterWaiterWithDeliver("cds", cdsWaitPerVariant, onCDSResponse)
		SendCDS(castleAID, targetX, targetY, sdiLID, delayHours, v.HBW, v.PTT, troopsJSON)
		resp, err := waiter.WaitWithTimeout()
		waiter.Cleanup()
		if err == nil && cdsIncomingOK(resp) {
			return true
		}
		if i+1 < len(pairs) {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return false
}
