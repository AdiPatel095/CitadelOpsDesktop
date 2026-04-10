package GameFunctions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	dec "CitadelDesktop/Server/Models/Decoration"
	"CitadelDesktop/Server/ResponseRegistry"
)

type decoRow struct {
	b     Models.BuildingData
	layer dec.PresetLayer
}

var (
	placerMu      sync.Mutex
	placerRunning bool
	placerCtx     context.Context
	placerCancel  context.CancelFunc
)

func collectDecorationRows(c *Models.PlayerCastleInfo) []decoRow {
	if c == nil {
		return nil
	}
	return collectDecorationRowsFromSlices(c.BGRows, c.BDRows)
}

func collectDecorationRowsFromSlices(bg, bd []Models.BuildingData) []decoRow {
	var out []decoRow
	for _, b := range bg {
		if Models.IsDecorationPickupCandidateWID(b.BuildingID) {
			out = append(out, decoRow{b: b, layer: dec.LayerBG})
		}
	}
	for _, b := range bd {
		if Models.IsDecorationPickupCandidateWID(b.BuildingID) {
			out = append(out, decoRow{b: b, layer: dec.LayerBD})
		}
	}
	return out
}

// workCastle holds BG/BD used only for the apply loop. GameState updates on JAA lag behind SOB/EBU,
// so we mirror changes here between the initial and final JAA.
type workCastle struct {
	bg []Models.BuildingData
	bd []Models.BuildingData
}

func copyWorkFromCastle(c *Models.PlayerCastleInfo) workCastle {
	if c == nil {
		return workCastle{}
	}
	w := workCastle{
		bg: make([]Models.BuildingData, len(c.BGRows)),
		bd: make([]Models.BuildingData, len(c.BDRows)),
	}
	copy(w.bg, c.BGRows)
	copy(w.bd, c.BDRows)
	return w
}

func (w *workCastle) collect() []decoRow {
	return collectDecorationRowsFromSlices(w.bg, w.bd)
}

func removeDecorationByOID(w *workCastle, oid int) bool {
	for i, b := range w.bg {
		if b.OID == oid {
			w.bg = append(w.bg[:i], w.bg[i+1:]...)
			return true
		}
	}
	for i, b := range w.bd {
		if b.OID == oid {
			w.bd = append(w.bd[:i], w.bd[i+1:]...)
			return true
		}
	}
	return false
}

// appendPresetPlacementToWork adds a row we assume the client applied after EBU. OID must stay negative
// (work-local only) so we never SendSOB(0) and removeDecorationByOID stays unambiguous.
func appendPresetPlacementToWork(w *workCastle, p dec.PresetPlacement, workOID int) {
	b := Models.BuildingData{
		BuildingID: p.WID,
		X:          p.X,
		Y:          p.Y,
		R:          p.R,
		OID:        workOID,
	}
	if p.Layer == dec.LayerBG {
		w.bg = append(w.bg, b)
	} else {
		w.bd = append(w.bd, b)
	}
}

// decorationLayoutMatchesPreset checks server-side castle rows (after JAA) match the preset multiset.
func decorationLayoutMatchesPreset(c *Models.PlayerCastleInfo, preset dec.NamedPreset) bool {
	if c == nil {
		return false
	}
	current := collectDecorationRows(c)
	if len(current) != len(preset.Items) {
		return false
	}
	used := make([]bool, len(current))
	for _, p := range preset.Items {
		found := false
		for ci := range current {
			if used[ci] {
				continue
			}
			if placementMatches(p, current[ci].b, current[ci].layer) {
				used[ci] = true
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func placementMatches(p dec.PresetPlacement, b Models.BuildingData, layer dec.PresetLayer) bool {
	return p.WID == b.BuildingID && p.X == b.X && p.Y == b.Y && p.R == b.R && p.Layer == layer
}

// CancelDecorationApply stops an in-progress preset apply, if any.
func CancelDecorationApply() {
	placerMu.Lock()
	defer placerMu.Unlock()
	if placerCancel != nil {
		placerCancel()
	}
}

// IsDecorationApplyRunning reports whether a preset apply goroutine is active.
func IsDecorationApplyRunning() bool {
	placerMu.Lock()
	defer placerMu.Unlock()
	return placerRunning
}

// StartDecorationPresetApply runs apply in a goroutine. onProgress receives short status strings.
func StartDecorationPresetApply(castleID, kingdomID, mapX, mapY int, presetID string, onProgress func(string)) {
	preset, ok := dec.LookupPreset(castleID, presetID)
	if !ok {
		if onProgress != nil {
			onProgress("error: preset not found")
		}
		return
	}

	placerMu.Lock()
	if placerRunning {
		placerMu.Unlock()
		if onProgress != nil {
			onProgress("error: apply already running")
		}
		return
	}
	if placerCancel != nil {
		placerCancel()
	}
	runCtx, cancel := context.WithCancel(context.Background())
	placerCtx = runCtx
	placerCancel = cancel
	placerRunning = true
	placerMu.Unlock()

	go func(ctx context.Context) {
		defer func() {
			placerMu.Lock()
			if placerCtx == ctx {
				placerRunning = false
				placerCancel = nil
				placerCtx = nil
			}
			placerMu.Unlock()
			cancel()
		}()
		msg := runDecorationApply(ctx, castleID, kingdomID, mapX, mapY, preset, onProgress)
		if onProgress != nil && msg != "" {
			onProgress(msg)
		}
	}(runCtx)
}

func runDecorationApply(ctx context.Context, castleID, kingdomID, mapX, mapY int, preset dec.NamedPreset, onProgress func(string)) string {
	if ctx.Err() != nil {
		return "cancelled"
	}
	if !ResponseRegistry.LoginStatus {
		return "error: not logged in"
	}
	if len(preset.Items) == 0 {
		return "complete: empty preset"
	}

	prog := func(s string) {
		if onProgress != nil {
			onProgress(s)
		}
	}

	// One JAA/JCA at start so GameState + workCastle match the live castle.
	if !GameParser.FocusPlayerCastleWithRetry(kingdomID, castleID, mapX, mapY) {
		return "error: focus castle (JAA/JCA) timed out"
	}
	time.Sleep(200 * time.Millisecond)

	gs := Models.GetGameState()
	c0 := gs.GetCastleByID(castleID)
	if c0 == nil {
		return "error: castle not in GameState"
	}
	work := copyWorkFromCastle(c0)

	const maxSteps = 500
	workOIDSeq := 0 // negative OIDs for optimistic EBU rows (not sent to the game)
	done := false
	for step := 0; step < maxSteps; step++ {
		if ctx.Err() != nil {
			return "cancelled"
		}
		if !ResponseRegistry.LoginStatus {
			return "error: not logged in"
		}

		current := work.collect()
		desired := preset.Items

		used := make([]bool, len(current))
		satisfied := make([]bool, len(desired))

		for pi := range desired {
			for ci := range current {
				if used[ci] {
					continue
				}
				if placementMatches(desired[pi], current[ci].b, current[ci].layer) {
					used[ci] = true
					satisfied[pi] = true
					break
				}
			}
		}

		var sobOID int
		for ci := range current {
			if used[ci] {
				continue
			}
			b := current[ci].b
			if b.OID <= 0 {
				return fmt.Sprintf("error: unmatched work decoration without server OID (WID %d @ %d,%d)", b.BuildingID, b.X, b.Y)
			}
			if dec.DecorationSOBBlockedWID(b.BuildingID) {
				return fmt.Sprintf("error: cannot SOB blocked WID %d (OID %d)", b.BuildingID, b.OID)
			}
			sobOID = b.OID
			break
		}

		if sobOID != 0 {
			prog(fmt.Sprintf("sob OID %d", sobOID))
			GameCommands.SendSOB(castleID, sobOID)
			if !removeDecorationByOID(&work, sobOID) {
				return fmt.Sprintf("error: internal SOB state (OID %d)", sobOID)
			}
			time.Sleep(150 * time.Millisecond)
			continue
		}

		ebuPI := -1
		for pi := range desired {
			if satisfied[pi] {
				continue
			}
			ebuPI = pi
			break
		}

		if ebuPI >= 0 {
			ep := desired[ebuPI]
			prog(fmt.Sprintf("ebu WID %d @ %d,%d R%d", ep.WID, ep.X, ep.Y, ep.R))
			GameCommands.SendEBUWithParams(ep.WID, ep.X, ep.Y, ep.R, 0, -1, -1)
			workOIDSeq--
			appendPresetPlacementToWork(&work, ep, workOIDSeq)
			time.Sleep(150 * time.Millisecond)
			continue
		}

		done = true
		break
	}
	if !done {
		return "error: max steps exceeded"
	}

	prog("verify layout (JAA)")
	if !GameParser.FocusPlayerCastleWithRetry(kingdomID, castleID, mapX, mapY) {
		return "error: final JAA verify timed out"
	}
	time.Sleep(200 * time.Millisecond)

	gs = Models.GetGameState()
	cFinal := gs.GetCastleByID(castleID)
	if cFinal == nil {
		return "error: castle not in GameState after verify"
	}
	if !decorationLayoutMatchesPreset(cFinal, preset) {
		return "error: layout mismatch after apply (preset vs live castle)"
	}
	return "complete"
}

// BuildPresetPlacementsFromCastle scans BG/BD and returns decoration placements for saving (focus must match castle).
func BuildPresetPlacementsFromCastle(c *Models.PlayerCastleInfo) []dec.PresetPlacement {
	if c == nil {
		return nil
	}
	var out []dec.PresetPlacement
	for _, b := range c.BGRows {
		if Models.IsDecorationPickupCandidateWID(b.BuildingID) {
			out = append(out, dec.PresetPlacement{
				WID: b.BuildingID, X: b.X, Y: b.Y, R: b.R, Layer: dec.LayerBG,
			})
		}
	}
	for _, b := range c.BDRows {
		if Models.IsDecorationPickupCandidateWID(b.BuildingID) {
			out = append(out, dec.PresetPlacement{
				WID: b.BuildingID, X: b.X, Y: b.Y, R: b.R, Layer: dec.LayerBD,
			})
		}
	}
	return out
}
