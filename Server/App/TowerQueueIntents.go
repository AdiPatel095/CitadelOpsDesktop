package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const towerQueueDeferralDuration = 30 * time.Second

type towerQueueScanRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
	Radius         int            `json:"radius"`
	ScanStartedAt  time.Time      `json:"scanStartedAt"`
}

type towerQueueEntryRequest struct {
	SourceCastleID State.CastleID  `json:"sourceCastleId"`
	KingdomID      State.KingdomID `json:"kingdomId"`
	TargetX        int             `json:"targetX"`
	TargetY        int             `json:"targetY"`
}

type towerQueueTargetRefreshRequest struct {
	towerQueueEntryRequest
	RefreshStartedAt time.Time `json:"refreshStartedAt"`
}

type towerMapWindow struct {
	X1 int
	Y1 int
	X2 int
	Y2 int
}

func planTowerQueueScan(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := towerQueueScanContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	windows := towerMapScanWindows(source, request.Radius)
	steps := make([]Intent.Step, 0, len(windows)+2)
	if !source.Focused {
		steps = append(steps, castleFocusStep(source))
	}
	for index, window := range windows {
		payload, _ := json.Marshal(struct {
			KingdomID State.KingdomID `json:"KID"`
			X1        int             `json:"AX1"`
			Y1        int             `json:"AY1"`
			X2        int             `json:"AX2"`
			Y2        int             `json:"AY2"`
		}{source.KingdomID, window.X1, window.Y1, window.X2, window.Y2})
		steps = append(steps, commandStep(
			fmt.Sprintf("Refresh tower map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa",
		))
	}
	steps = append(steps, Intent.Step{
		Name: "Capture fresh tower batch", Action: "tower.queue.capture", ActionArguments: append(json.RawMessage(nil), arguments...),
	})
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"map:" + strconv.FormatInt(int64(source.KingdomID), 10),
		},
		Summary: fmt.Sprintf("Refresh complete tower map around %s", castleLabel(source)),
		Steps:   steps,
	}, nil
}

func planTowerQueueTargetRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request towerQueueTargetRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.SourceCastleID <= 0 || request.RefreshStartedAt.IsZero() {
		return Intent.Plan{}, fmt.Errorf("tower queue target refresh requires a source castle and start time")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return Intent.Plan{}, fmt.Errorf("tower queue source castle %d is not in the current player state", request.SourceCastleID)
	}
	if request.KingdomID != source.KingdomID {
		return Intent.Plan{}, fmt.Errorf("tower queue target must be in source castle kingdom %d", source.KingdomID)
	}
	queued := false
	for _, entry := range input.State.TowerQueue.EntriesByCastle[request.SourceCastleID] {
		if entry.KingdomID == request.KingdomID && entry.TargetX == request.TargetX && entry.TargetY == request.TargetY {
			queued = true
			break
		}
	}
	if !queued {
		return Intent.Plan{}, fmt.Errorf("tower target %d:%d is no longer queued", request.TargetX, request.TargetY)
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X1        int             `json:"AX1"`
		Y1        int             `json:"AY1"`
		X2        int             `json:"AX2"`
		Y2        int             `json:"AY2"`
	}{request.KingdomID, request.TargetX, request.TargetY, request.TargetX, request.TargetY})
	normalizedArguments, _ := json.Marshal(request)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "map:" + strconv.FormatInt(int64(request.KingdomID), 10),
		},
		Summary: fmt.Sprintf("Refresh queued tower %d:%d and rotate it if still stale", request.TargetX, request.TargetY),
		Steps: []Intent.Step{
			commandStep("Refresh queued tower", "gaa", payload, "gaa"),
			{Name: "Rotate unchanged tower behind ready targets", Action: "tower.queue.rotate_stale", ActionArguments: normalizedArguments},
		},
	}, nil
}

func towerMapScanWindows(source State.CastleState, radius int) []towerMapWindow {
	const maximumTilesPerWindow = 2500
	minimumX := source.X - radius
	maximumX := source.X + radius
	minimumY := source.Y - radius
	maximumY := source.Y + radius
	width := maximumX - minimumX + 1
	height := maximumY - minimumY + 1
	windowCount := min(5, max(1, (width*height+maximumTilesPerWindow-1)/maximumTilesPerWindow))
	baseWidth := width / windowCount
	remainder := width % windowCount
	windows := make([]towerMapWindow, 0, windowCount)
	nextX := minimumX
	for index := 0; index < windowCount; index++ {
		currentWidth := baseWidth
		if index < remainder {
			currentWidth++
		}
		window := towerMapWindow{X1: nextX, Y1: minimumY, X2: nextX + currentWidth - 1, Y2: maximumY}
		windows = append(windows, window)
		nextX = window.X2 + 1
	}
	return windows
}

func towerQueueScanContext(input Intent.PlanningContext, arguments json.RawMessage) (towerQueueScanRequest, State.CastleState, error) {
	var request towerQueueScanRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return towerQueueScanRequest{}, State.CastleState{}, err
	}
	if request.SourceCastleID <= 0 {
		return towerQueueScanRequest{}, State.CastleState{}, fmt.Errorf("tower queue source castle is required")
	}
	if request.Radius < 1 || request.Radius > 50 {
		return towerQueueScanRequest{}, State.CastleState{}, fmt.Errorf("tower queue radius must be between 1 and 50")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return towerQueueScanRequest{}, State.CastleState{}, fmt.Errorf("tower queue source castle %d is not in the current player state", request.SourceCastleID)
	}
	return request, source, nil
}

func (application *Application) captureTowerQueue(_ context.Context, arguments json.RawMessage) error {
	request, _, err := towerQueueScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentTowerQueue), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("tower queue source castle %d is no longer focused", request.SourceCastleID)
		}
		candidates := make([]State.MapObservation, 0)
		gameState.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionTower, func(_ string, target State.MapObservation) bool {
			if target.TypeID != kingdomTowerMapTypeID ||
				towerQueueDistanceSquared(source, target) > request.Radius*request.Radius ||
				(!request.ScanStartedAt.IsZero() && target.ObservedAt.Before(request.ScanStartedAt)) {
				return true
			}
			candidates = append(candidates, target)
			return true
		})
		sort.Slice(candidates, func(left, right int) bool {
			leftDistance := towerQueueDistanceSquared(source, candidates[left])
			rightDistance := towerQueueDistanceSquared(source, candidates[right])
			if leftDistance != rightDistance {
				return leftDistance < rightDistance
			}
			if candidates[left].Y != candidates[right].Y {
				return candidates[left].Y < candidates[right].Y
			}
			return candidates[left].X < candidates[right].X
		})
		now := time.Now().UTC()
		entries := make([]State.TowerQueueEntry, 0, len(candidates))
		for _, target := range candidates {
			entries = append(entries, State.TowerQueueEntry{
				KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
				MapObservedAt: target.ObservedAt, QueuedAt: now,
			})
		}
		gameState.SetTowerQueueEntries(source.ID, entries)
		gameState.SetTowerQueueLastScannedAt(source.ID, now)
		return []string{"tower-queue"}, true, nil
	})
	return err
}

func (application *Application) consumeTowerQueueEntry(_ context.Context, arguments json.RawMessage) error {
	var request towerQueueEntryRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentTowerQueue), func(gameState *State.GameState) ([]string, bool, error) {
		entries := gameState.MutableTowerQueueEntries(request.SourceCastleID)
		for index, entry := range entries {
			if entry.KingdomID != request.KingdomID || entry.TargetX != request.TargetX || entry.TargetY != request.TargetY {
				continue
			}
			gameState.SetTowerQueueEntries(request.SourceCastleID, append(entries[:index:index], entries[index+1:]...))
			return []string{"tower-queue"}, true, nil
		}
		return nil, false, nil
	})
	return err
}

func (application *Application) deferTowerQueueEntry(_ context.Context, arguments json.RawMessage) error {
	var request towerQueueEntryRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentTowerQueue), func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		if !rotateTowerQueueEntry(gameState, request, now) {
			return nil, false, nil
		}
		gameState.SetTowerQueueLastAttemptedAt(request.SourceCastleID, now)
		return []string{"tower-queue"}, true, nil
	})
	return err
}

func (application *Application) rotateStaleTowerQueueEntry(_ context.Context, arguments json.RawMessage) error {
	var request towerQueueTargetRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentTowerQueue), func(gameState *State.GameState) ([]string, bool, error) {
		target, found := gameState.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
		if found && !target.ObservedAt.IsZero() && !target.ObservedAt.Before(request.RefreshStartedAt) {
			return nil, false, nil
		}
		if !rotateTowerQueueEntry(gameState, request.towerQueueEntryRequest, time.Now().UTC()) {
			return nil, false, nil
		}
		return []string{"tower-queue"}, true, nil
	})
	return err
}

func rotateTowerQueueEntry(gameState *State.GameState, request towerQueueEntryRequest, now time.Time) bool {
	entries := gameState.MutableTowerQueueEntries(request.SourceCastleID)
	for index, entry := range entries {
		if entry.KingdomID != request.KingdomID || entry.TargetX != request.TargetX || entry.TargetY != request.TargetY {
			continue
		}
		entry.QueuedAt = now
		deferredUntil := now.Add(towerQueueDeferralDuration)
		entry.DeferredUntil = &deferredUntil
		rotated := make([]State.TowerQueueEntry, 0, len(entries))
		rotated = append(rotated, entries[:index]...)
		rotated = append(rotated, entries[index+1:]...)
		rotated = append(rotated, entry)
		gameState.SetTowerQueueEntries(request.SourceCastleID, rotated)
		return true
	}
	return false
}

func towerQueueDistanceSquared(castle State.CastleState, target State.MapObservation) int {
	x := target.X - castle.X
	y := target.Y - castle.Y
	return x*x + y*y
}
