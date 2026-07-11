package featureview

import (
	"context"
	"errors"
	"fmt"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
)

const (
	autoBeriWorldWireWait     = 8 * time.Second
	autoBeriWorldFucPollTick  = 75 * time.Millisecond
	autoBeriWorldPostKutPause = 500 * time.Millisecond
	defaultKutCastleCID       = -1
	defaultTroopSpaceCheckSec = 30
	minTroopSpaceCheckSec     = 5
	maxTroopSpaceCheckSec     = 3600
)

var errFucTroopAmountMissing = errors.New("no troop amount in fuc response")

func maybeTransferBeriTroops(ctx context.Context) {
	cfg := Models.GetSettingsState().AutoBeriWorld
	if cfg.TransferTroopWID <= 0 {
		Logging.AutoBeriWorldLog("transfer", "skipped (no transferTroopWID in settings)")
		return
	}
	if _, scidOK := ResolveKutSourceSCID(); !scidOK {
		Logging.AutoBeriWorldLog("transfer", "skipped (main castle SCID unknown — need login GCL)")
		return
	}

	beriCID, _, _, beriOK := ResolveBeriCastle()
	if !beriOK || beriCID <= 0 {
		Logging.AutoBeriWorldLog("transfer", "skipped (beri castle CID unknown)")
		return
	}

	if ctx.Err() != nil {
		return
	}
	lease, ok := Automation.Acquire(ctx, Automation.Request{
		Owner:    Automation.OwnerAutoBeri,
		Priority: Automation.PriorityAutoBeri,
		Reason:   "Beri troop transfer",
		Claims: []Automation.Claim{
			Automation.ExclusiveClaim(Automation.ClaimTransport),
			Automation.ExclusiveClaim("beri:troop-transfer"),
		},
		MaxHold: 3*autoBeriWorldWireWait + autoBeriWorldPostKutPause,
	})
	if !ok {
		return
	}
	defer lease.Release()
	troopAmount, err := runBeriTroopSpaceCheck(ctx, beriCID, cfg.TransferTroopWID, lease)
	if err != nil {
		Logging.AutoBeriWorldLogf("transfer", "troop-space check failed: %v", err)
		return
	}
	Logging.AutoBeriWorldLogf("transfer", "fuc troop amount=%d (min=%d)", troopAmount, cfg.MinTroopsToTransfer)
	if troopAmount <= 0 {
		Logging.AutoBeriWorldLog("transfer", "skipped (fuc amount=0)")
		return
	}
	if cfg.MinTroopsToTransfer > 0 && troopAmount < cfg.MinTroopsToTransfer {
		Logging.AutoBeriWorldLog("transfer", "skipped (below minimum — wait for next check)")
		return
	}

	kutAmount := kutTroopAmountFromFuc(troopAmount)
	if kutAmount <= 0 {
		Logging.AutoBeriWorldLog("transfer", "skipped (fuc amount=0)")
		return
	}

	scid, _ := ResolveKutSourceSCID()
	kutCID := cfg.KutCastleCID
	if kutCID == 0 {
		kutCID = defaultKutCastleCID
	}

	Logging.AutoBeriWorldLogf("transfer", "kut SCID=%d CID=%d unit=%d amt=%d (from fuc)", scid, kutCID, cfg.TransferTroopWID, kutAmount)
	wKut := ResponseRegistry.Global.RegisterWaiter("kut", autoBeriWorldWireWait)
	GameCommands.SendBeriKutTransfer(scid, kutCID, cfg.TransferTroopWID, kutAmount, lease)
	if _, err := wKut.WaitWithTimeout(); err != nil {
		Logging.AutoBeriWorldLogf("transfer", "kut wait: %v", err)
		wKut.Cleanup()
		return
	}
	wKut.Cleanup()

	select {
	case <-ctx.Done():
		return
	case <-time.After(autoBeriWorldPostKutPause):
	}

	wMsk := ResponseRegistry.Global.RegisterWaiter("msk", autoBeriWorldWireWait)
	GameCommands.SendBeriMskSpeedup(lease)
	if _, err := wMsk.WaitWithTimeout(); err != nil {
		Logging.AutoBeriWorldLogf("transfer", "msk wait: %v", err)
	} else {
		Logging.AutoBeriWorldLog("transfer", "msk speed-up sent")
	}
	wMsk.Cleanup()
}

// kutTroopAmountFromFuc returns the troop count for kut A[[unit,amount]] from the latest **fuc** parse.
func kutTroopAmountFromFuc(fallback int) int {
	if gs := Models.GetGameState(); gs != nil {
		if amt, _ := gs.AutoBeriWorldFucResult(); amt > 0 {
			return amt
		}
	}
	return fallback
}

// waitForFucTroopAmount polls until a parsed **fuc** amount is stored (first non-zero wins).
func waitForFucTroopAmount(ctx context.Context, timeout time.Duration) int {
	return waitForFucTroopAmountAtLeast(ctx, timeout, 0)
}

// waitForFucTroopAmountAtLeast polls until session amount exceeds baseline or timeout (returns best seen).
func waitForFucTroopAmountAtLeast(ctx context.Context, timeout time.Duration, baseline int) int {
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(autoBeriWorldFucPollTick)
	defer tick.Stop()
	best := baseline
	for {
		if gs := Models.GetGameState(); gs != nil {
			if amt, _ := gs.AutoBeriWorldFucResult(); amt > best {
				best = amt
				if baseline == 0 {
					return best
				}
			}
		}
		if time.Now().After(deadline) {
			return best
		}
		select {
		case <-ctx.Done():
			return best
		case <-tick.C:
		}
	}
}

// runBeriTroopSpaceCheck sends **fuc** and returns the kut send amount from the **fuc** response.
func runBeriTroopSpaceCheck(ctx context.Context, beriCastleCID, unitWID int, lease *Automation.Lease) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if gs := Models.GetGameState(); gs != nil {
		gs.ClearAutoBeriWorldFucResult()
	}

	GameCommands.SendBeriFucTroopSpaceCheck(beriCastleCID, lease)
	amt := waitForFucTroopAmount(ctx, autoBeriWorldWireWait)
	if amt <= 0 {
		return 0, fmt.Errorf("%w (unitWID=%d)", errFucTroopAmountMissing, unitWID)
	}
	return amt, nil
}
