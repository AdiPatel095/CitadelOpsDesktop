package castleview

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

// DecorationStorageShortfall is one decoration type that is still short after **sin** storage + on-map SOB pickup counts.
type DecorationStorageShortfall struct {
	WID   int    `json:"wid"`
	Count int    `json:"count"`
	Name  string `json:"name"`
	Line  string `json:"line"` // "1x Rose Bush" — same style as castle decoration summary lines
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

// StartDecorationPresetApply runs apply in a goroutine. onProgress receives short status strings (SOB/EBU steps).
// onAlert is optional; use for user-facing alerts.
// onStorageMismatch is optional; called when **sin** shows insufficient stock — includes human-readable lines like the decoration hover summary.
func StartDecorationPresetApply(castleID, kingdomID, mapX, mapY int, storageKey, presetID string, onProgress func(string), onAlert func(category string, message string), onStorageMismatch func([]DecorationStorageShortfall)) {
	preset, ok := dec.LookupPresetForKey(storageKey, castleID, presetID)
	if !ok {
		if onAlert != nil {
			onAlert("red", "Decoration preset not found for this castle.")
		}
		return
	}

	placerMu.Lock()
	if placerRunning {
		placerMu.Unlock()
		if onAlert != nil {
			onAlert("yellow", "A decoration preset apply is already running.")
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
		msg := runDecorationApply(ctx, castleID, kingdomID, mapX, mapY, preset, onProgress, onAlert, onStorageMismatch)
		if onProgress != nil && msg != "" && decorationApplyProgressShouldEmit(msg) {
			onProgress(msg)
		}
	}(runCtx)
}

// decorationApplyProgressShouldEmit avoids pushing error/cancel lines to the in-card footer; those use SendAlert (onAlert) instead.
func decorationApplyProgressShouldEmit(msg string) bool {
	s := strings.TrimSpace(msg)
	if strings.HasPrefix(s, "error:") || strings.HasPrefix(s, "cancelled:") {
		return false
	}
	// Internal steps — not shown in the UI progress strip (global alerts or dedicated storage panel handle these).
	if strings.HasPrefix(s, "sin:") || strings.HasPrefix(s, "storage:") {
		return false
	}
	return true
}

func computeDecorationStorageDelta(work *workCastle, preset dec.NamedPreset) (ebuNeed map[int]int, sobReturn map[int]int) {
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
	ebuNeed = make(map[int]int)
	for pi, ok := range satisfied {
		if !ok {
			ebuNeed[desired[pi].WID]++
		}
	}
	sobReturn = make(map[int]int)
	for ci, ok := range used {
		if !ok {
			sobReturn[current[ci].b.BuildingID]++
		}
	}
	return ebuNeed, sobReturn
}

// decorationStorageShortfalls lists each WID still short after combining **sin** counts with on-map pickups (SOB return).
func decorationStorageShortfalls(sto map[int]int, ebuNeed, sobReturn map[int]int) []DecorationStorageShortfall {
	type row struct {
		wid                 int
		short               int
		name, line, sortKey string
	}
	var tmp []row
	for wid, need := range ebuNeed {
		if need <= 0 {
			continue
		}
		have := sto[wid] + sobReturn[wid]
		if have >= need {
			continue
		}
		short := need - have
		name := dec.ResolvedWodDisplayName(wid)
		line := fmt.Sprintf("%dx %s", short, name)
		tmp = append(tmp, row{wid: wid, short: short, name: name, line: line, sortKey: strings.ToLower(name)})
	}
	sort.Slice(tmp, func(i, j int) bool {
		if tmp[i].sortKey != tmp[j].sortKey {
			return tmp[i].sortKey < tmp[j].sortKey
		}
		return tmp[i].wid < tmp[j].wid
	})
	out := make([]DecorationStorageShortfall, len(tmp))
	for i, r := range tmp {
		out[i] = DecorationStorageShortfall{WID: r.wid, Count: r.short, Name: r.name, Line: r.line}
	}
	return out
}

const sinResponseWait = 8 * time.Second

func runDecorationApply(ctx context.Context, castleID, kingdomID, mapX, mapY int, preset dec.NamedPreset, onProgress func(string), onAlert func(category string, message string), onStorageMismatch func([]DecorationStorageShortfall)) string {
	prog := func(s string) {
		if onProgress != nil {
			onProgress(s)
		}
	}
	alert := func(category, msg string) {
		if onAlert != nil {
			onAlert(category, msg)
		}
	}

	if ctx.Err() != nil {
		return "cancelled"
	}
	if !ResponseRegistry.LoginStatus {
		alert("red", "Decoration apply cancelled: not logged in to the game.")
		return "error: not logged in"
	}
	if len(preset.Items) == 0 {
		return "complete: empty preset"
	}

	// One JAA/JCA at start so GameState + workCastle match the live castle.
	if !GameParser.FocusPlayerCastleWithRetry(kingdomID, castleID, mapX, mapY) {
		alert("red", "Decoration apply failed: could not focus the castle (JAA/JCA timed out).")
		return "error: focus castle (JAA/JCA) timed out"
	}
	time.Sleep(200 * time.Millisecond)

	gs := Models.GetGameState()
	c0 := gs.GetCastleByID(castleID)
	if c0 == nil {
		alert("red", "Decoration apply failed: castle not found in session state.")
		return "error: castle not in GameState"
	}
	work := copyWorkFromCastle(c0)

	ebuNeed, sobReturn := computeDecorationStorageDelta(&work, preset)
	if len(ebuNeed) > 0 {
		waiter := ResponseRegistry.Global.RegisterWaiter("sin", sinResponseWait)
		GameCommands.SendSIN()
		sinParts, err := waiter.WaitWithTimeout()
		waiter.Cleanup()
		if err != nil {
			alert("red", "Decoration apply cancelled: timed out waiting for storage (sin).")
			return ""
		}
		if ctx.Err() != nil {
			return "cancelled"
		}
		sto, err := GameParser.ParseDecorationStorageCountsFromSINFrame(sinParts)
		if err != nil {
			alert("red", fmt.Sprintf("Decoration apply cancelled: could not parse storage (sin): %v", err))
			return ""
		}
		shortfalls := decorationStorageShortfalls(sto, ebuNeed, sobReturn)
		if len(shortfalls) > 0 {
			if onStorageMismatch != nil {
				onStorageMismatch(shortfalls)
			}
			// Red banner + bullet list are shown by the frontend Alerts stack (persistent until dismissed or apply completes).
			return ""
		}
	}

	const maxSteps = 500
	workOIDSeq := 0 // negative OIDs for optimistic EBU rows (not sent to the game)
	done := false
	for step := 0; step < maxSteps; step++ {
		if ctx.Err() != nil {
			return "cancelled"
		}
		if !ResponseRegistry.LoginStatus {
			alert("red", "Decoration apply stopped: game session is no longer logged in.")
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
				msg := fmt.Sprintf("error: unmatched work decoration without server OID (WID %d @ %d,%d)", b.BuildingID, b.X, b.Y)
				alert("red", "Decoration apply failed: internal layout state (missing server OID for a decoration).")
				return msg
			}
			if dec.DecorationSOBBlockedWID(b.BuildingID) {
				msg := fmt.Sprintf("error: cannot SOB blocked WID %d (OID %d)", b.BuildingID, b.OID)
				alert("red", fmt.Sprintf("Decoration apply failed: cannot pick up this decoration type (WID %d).", b.BuildingID))
				return msg
			}
			sobOID = b.OID
			break
		}

		if sobOID != 0 {
			prog(fmt.Sprintf("sob OID %d", sobOID))
			GameCommands.SendSOB(castleID, sobOID)
			if !removeDecorationByOID(&work, sobOID) {
				alert("red", "Decoration apply failed: internal state after SOB.")
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
		alert("red", "Decoration apply failed: too many steps (layout did not converge).")
		return "error: max steps exceeded"
	}

	prog("verify layout (JAA)")
	if !GameParser.FocusPlayerCastleWithRetry(kingdomID, castleID, mapX, mapY) {
		alert("red", "Decoration apply failed: could not verify layout (final JAA timed out).")
		return "error: final JAA verify timed out"
	}
	time.Sleep(200 * time.Millisecond)

	gs = Models.GetGameState()
	cFinal := gs.GetCastleByID(castleID)
	if cFinal == nil {
		alert("red", "Decoration apply failed: castle missing from state after verify.")
		return "error: castle not in GameState after verify"
	}
	if !decorationLayoutMatchesPreset(cFinal, preset) {
		alert("red", "Decoration apply failed: live castle layout still does not match the preset.")
		return "error: layout mismatch after apply (preset vs live castle)"
	}
	alert("green", "Decoration preset applied successfully.")
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
