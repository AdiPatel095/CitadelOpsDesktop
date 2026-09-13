package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	fortressAttackDialogFreshness = 30 * time.Second
	fortressPersonalCooldown      = 120 * time.Hour
	fortressMaximumSpeedPercent   = 100
	fortressMapChunkSize          = 90
	fortressMapMaximumChunk       = 20
	fortressMapBoundaryPadding    = 2
	fortressMapChunkDelay         = 200 * time.Millisecond
	fortressMapResponseTimeout    = 8 * time.Second
	fortressMapFullScanTimeout    = 5 * time.Minute
)

type fortressMapRequest struct {
	SourceCastleID State.CastleID  `json:"sourceCastleId"`
	KingdomID      State.KingdomID `json:"kingdomId"`
	TargetX        int             `json:"targetX,omitempty"`
	TargetY        int             `json:"targetY,omitempty"`
	ScanStartedAt  time.Time       `json:"scanStartedAt,omitempty"`
}

type fortressMapChunk struct {
	X int
	Y int
}

type fortressFullMapScanResult struct {
	Windows        []towerMapWindow
	ContentWindows []towerMapWindow
	FailedChunks   []fortressMapChunk
}

type fortressMapWindowScanner func(context.Context, towerMapWindow) (bool, error)

type fortressAttackRequest struct {
	SourceCastleID        State.CastleID      `json:"sourceCastleId"`
	KingdomID             State.KingdomID     `json:"kingdomId"`
	TargetX               int                 `json:"targetX"`
	TargetY               int                 `json:"targetY"`
	CommanderIDs          []State.CommanderID `json:"commanderIds"`
	HorseTravelBoostID    int                 `json:"horseTravelBoostId"`
	DailyAttackLimit      int64               `json:"dailyAttackLimit"`
	MinimumCommanderSpeed float64             `json:"minimumCommanderSpeedBonus"`
}

type fortressResolvedAttackRequest struct {
	fortressAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

func planFortressMapScan(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := fortressMapContext(input, arguments, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	request.ScanStartedAt = time.Now().UTC()
	normalizedArguments, _ := json.Marshal(request)
	steps := make([]Intent.Step, 0, 2)
	if !source.Focused {
		focus := castleFocusStep(source)
		focus.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, focus)
	}
	steps = append(steps, Intent.RebuildOnResume(Intent.Step{
		Name: "Discover the full fortress map", Action: "fortress.scan.full", ActionArguments: normalizedArguments,
	}))
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10), "map:" + strconv.Itoa(int(request.KingdomID))},
		Summary: fmt.Sprintf("Discover every fortress across %s", castleLabel(source)), Steps: steps,
	}, nil
}

func planFortressTargetRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := fortressMapContext(input, arguments, true)
	if err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"KID": request.KingdomID, "AX1": request.TargetX, "AY1": request.TargetY,
		"AX2": request.TargetX, "AY2": request.TargetY,
	})
	steps := make([]Intent.Step, 0, 2)
	if !source.Focused {
		steps = append(steps, castleFocusStep(source))
	}
	steps = append(steps, commandStep("Refresh fortress cooldown", "gaa", payload, "gaa"))
	return Intent.Plan{
		Claims:  []string{"castle-focus", "map:" + strconv.Itoa(int(request.KingdomID))},
		Summary: fmt.Sprintf("Refresh fortress at %d:%d", request.TargetX, request.TargetY), Steps: steps,
	}, nil
}

func fortressMapContext(input Intent.PlanningContext, arguments json.RawMessage, targeted bool) (fortressMapRequest, State.CastleState, error) {
	var request fortressMapRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, err
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found || source.ID <= 0 || source.KingdomID != request.KingdomID || source.SlotType != 12 || request.KingdomID < 1 || request.KingdomID > 3 {
		return request, State.CastleState{}, fmt.Errorf("fortress source must be the owned main castle in kingdom %d", request.KingdomID)
	}
	if targeted {
		target, exists := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
		if !exists || target.TypeID != State.MapTypeKingdomFortress {
			return request, State.CastleState{}, fmt.Errorf("fortress at %d:%d is not in the current map state", request.TargetX, request.TargetY)
		}
	}
	return request, source, nil
}

func (application *Application) scanFullFortressMap(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.Session == nil || application.Ingest == nil {
		return fmt.Errorf("Fortress full-map scan dependencies are unavailable")
	}
	request, source, err := fortressMapContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments, false)
	if err != nil {
		return err
	}
	if request.ScanStartedAt.IsZero() {
		return fmt.Errorf("Fortress full-map scan is missing its start timestamp")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanContext, cancel := context.WithTimeout(ctx, fortressMapFullScanTimeout)
	defer cancel()

	var language *GameData.LanguageStore
	if application.GameData != nil {
		language, _ = application.GameData.Language()
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(scanContext).OperationID)
	if operationID == "" {
		operationID = "fortress-map"
	}
	tokenRoot := fmt.Sprintf("%s/fortress-gaa/%d", operationID, time.Now().UTC().UnixNano())
	requestNumber := 0
	lastRequestAt := time.Time{}
	result, scanErr := discoverFullFortressMap(scanContext, source, func(windowContext context.Context, window towerMapWindow) (bool, error) {
		if !lastRequestAt.IsZero() {
			wait := fortressMapChunkDelay - time.Since(lastRequestAt)
			if wait > 0 {
				timer := time.NewTimer(wait)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-windowContext.Done():
					return false, windowContext.Err()
				}
			}
		}
		requestNumber++
		lastRequestAt = time.Now()
		return runFortressMapGAAWindow(
			windowContext, application.Session, application.Ingest, language,
			request.KingdomID, window, fmt.Sprintf("%s/%d", tokenRoot, requestNumber),
		)
	})
	if scanErr != nil {
		return scanErr
	}
	return application.captureFullFortressMap(request, result.Windows)
}

func discoverFullFortressMap(
	ctx context.Context,
	source State.CastleState,
	scan fortressMapWindowScanner,
) (fortressFullMapScanResult, error) {
	result := fortressFullMapScanResult{}
	if scan == nil {
		return result, fmt.Errorf("Fortress full-map scanner is required")
	}
	if source.X < 0 || source.Y < 0 {
		return result, fmt.Errorf("Fortress source coordinates are invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := fortressMapChunk{X: source.X / fortressMapChunkSize, Y: source.Y / fortressMapChunkSize}
	if start.X > fortressMapMaximumChunk || start.Y > fortressMapMaximumChunk {
		return result, fmt.Errorf("Fortress source lies beyond the full-map scan safety envelope")
	}
	queue := []fortressMapChunk{start}
	enqueued := map[fortressMapChunk]struct{}{start: {}}
	visited := map[fortressMapChunk]struct{}{}
	minContentX, maxContentX := start.X, start.X
	minContentY, maxContentY := start.Y, start.Y
	touchedMaximumBoundary := false
	failed := make([]error, 0)

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return result, fmt.Errorf("Fortress full-map scan stopped after %d windows: %w", len(result.Windows), ctx.Err())
		default:
		}
		chunk := queue[0]
		queue = queue[1:]
		if _, seen := visited[chunk]; seen {
			continue
		}
		visited[chunk] = struct{}{}
		window := fortressMapChunkWindow(chunk)
		result.Windows = append(result.Windows, window)
		hasContent, err := scan(ctx, window)
		if err != nil {
			result.FailedChunks = append(result.FailedChunks, chunk)
			failed = append(failed, fmt.Errorf("chunk %d:%d: %w", chunk.X, chunk.Y, err))
			hasContent = true
		} else if hasContent {
			result.ContentWindows = append(result.ContentWindows, window)
		}
		if hasContent {
			minContentX = min(minContentX, chunk.X)
			maxContentX = max(maxContentX, chunk.X)
			minContentY = min(minContentY, chunk.Y)
			maxContentY = max(maxContentY, chunk.Y)
		}
		for _, neighbor := range []fortressMapChunk{
			{X: chunk.X - 1, Y: chunk.Y}, {X: chunk.X + 1, Y: chunk.Y},
			{X: chunk.X, Y: chunk.Y - 1}, {X: chunk.X, Y: chunk.Y + 1},
		} {
			if neighbor.X < 0 || neighbor.Y < 0 {
				continue
			}
			if neighbor.X > fortressMapMaximumChunk || neighbor.Y > fortressMapMaximumChunk {
				if hasContent {
					touchedMaximumBoundary = true
				}
				continue
			}
			if _, seen := enqueued[neighbor]; seen {
				continue
			}
			insideBoundaryPadding := neighbor.X >= minContentX-fortressMapBoundaryPadding &&
				neighbor.X <= maxContentX+fortressMapBoundaryPadding &&
				neighbor.Y >= minContentY-fortressMapBoundaryPadding &&
				neighbor.Y <= maxContentY+fortressMapBoundaryPadding
			if !hasContent && !insideBoundaryPadding {
				continue
			}
			enqueued[neighbor] = struct{}{}
			queue = append(queue, neighbor)
		}
	}
	if touchedMaximumBoundary {
		failed = append(failed, fmt.Errorf("map content reached the %d-coordinate chunk safety boundary", fortressMapMaximumChunk))
	}
	if len(failed) > 0 {
		return result, fmt.Errorf("Fortress full-map scan is incomplete: %w", errors.Join(failed...))
	}
	return result, nil
}

func fortressMapChunkWindow(chunk fortressMapChunk) towerMapWindow {
	x1, y1 := chunk.X*fortressMapChunkSize, chunk.Y*fortressMapChunkSize
	return towerMapWindow{X1: x1, Y1: y1, X2: x1 + fortressMapChunkSize - 1, Y2: y1 + fortressMapChunkSize - 1}
}

func runFortressMapGAAWindow(
	ctx context.Context,
	sender mapGAASender,
	observer mapGAAObserver,
	language *GameData.LanguageStore,
	kingdomID State.KingdomID,
	window towerMapWindow,
	responseToken string,
) (bool, error) {
	if sender == nil || observer == nil {
		return false, fmt.Errorf("Fortress map sender and response observer are required")
	}
	if !sender.CorrelatesResponses() {
		return false, fmt.Errorf("Fortress full-map scan requires correlated websocket responses")
	}
	requestContext, cancelRequest := context.WithTimeout(ctx, fortressMapResponseTimeout)
	defer cancelRequest()
	payload, err := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X1        int             `json:"AX1"`
		Y1        int             `json:"AY1"`
		X2        int             `json:"AX2"`
		Y2        int             `json:"AY2"`
	}{kingdomID, window.X1, window.Y1, window.X2, window.Y2})
	if err != nil {
		return false, fmt.Errorf("encode Fortress map window: %w", err)
	}
	wire, err := Protocol.Encode(Protocol.Command{Namespace: sender.Namespace(), Opcode: "gaa", Payload: payload})
	if err != nil {
		return false, fmt.Errorf("build Fortress map window: %w", err)
	}
	frames, cancelWatch := observer.WatchWireResponse("gaa", responseToken)
	defer cancelWatch()
	metadata := Outbound.MetadataFromContext(requestContext)
	metadata.ResponseToken = responseToken
	metadata.ResponseOpcodes = []string{"gaa"}
	metadata.ResponseTimeoutMillis = int(fortressMapResponseTimeout / time.Millisecond)
	sendContext := Outbound.WithMetadata(requestContext, metadata)
	for {
		err = sender.Send(sendContext, wire)
		if err == nil {
			break
		}
		if !errors.Is(err, Outbound.ErrAutomationLocked) || Outbound.IsIndeterminate(err) {
			return false, fmt.Errorf("send Fortress map window: %w", err)
		}
		if err := sender.WaitForAutomationUnlocked(requestContext); err != nil {
			return false, fmt.Errorf("Fortress map scan timed out while paused: %w", err)
		}
	}
	var response Protocol.CommittedFrame
	select {
	case received, open := <-frames:
		if !open {
			return false, fmt.Errorf("Fortress map response watcher closed")
		}
		response = received
	case <-requestContext.Done():
		return false, fmt.Errorf("Fortress map response timed out: %w", requestContext.Err())
	}
	if response.Frame.ResponseToken != responseToken {
		return false, fmt.Errorf("Fortress map response token changed")
	}
	forgetCommit := true
	defer func() {
		if forgetCommit {
			observer.ForgetCommitted(response.IngressID)
		}
	}()
	committed, err := observer.WaitCommitted(requestContext, response.IngressID)
	if err != nil {
		return false, fmt.Errorf("commit Fortress map response: %w", err)
	}
	forgetCommit = false
	if committed.Frame.ResponseCode == nil {
		return false, fmt.Errorf("Fortress map response did not include a result code")
	}
	if *committed.Frame.ResponseCode != 0 {
		return false, Intent.NewResponseCodeError(language, committed.Frame.Opcode, *committed.Frame.ResponseCode)
	}
	if committed.ReduceError != "" {
		return false, fmt.Errorf("Fortress map response state reduction failed: %s", committed.ReduceError)
	}
	var decoded struct {
		KingdomID State.KingdomID   `json:"KID"`
		Nodes     []json.RawMessage `json:"AI"`
	}
	if err := json.Unmarshal(committed.Frame.Payload, &decoded); err != nil {
		return false, fmt.Errorf("decode Fortress map response: %w", err)
	}
	if decoded.KingdomID != kingdomID {
		return false, fmt.Errorf("Fortress map response changed kingdoms from %d to %d", kingdomID, decoded.KingdomID)
	}
	return len(decoded.Nodes) > 0, nil
}

func (application *Application) captureFullFortressMap(request fortressMapRequest, windows []towerMapWindow) error {
	if application == nil || application.State == nil {
		return fmt.Errorf("Fortress map state is unavailable")
	}
	if request.ScanStartedAt.IsZero() || len(windows) == 0 {
		return fmt.Errorf("Fortress full-map capture requires a completed scan")
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentWorldMap), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || source.KingdomID != request.KingdomID || source.SlotType != 12 {
			return nil, false, fmt.Errorf("Fortress source castle changed before full-map capture")
		}
		stale := make([]string, 0)
		gameState.RangeMapObservationsByKind(request.KingdomID, State.MapProjectionFortress, func(key string, target State.MapObservation) bool {
			if target.ObservedAt.Before(request.ScanStartedAt) && fortressWindowsContain(windows, target.X, target.Y) {
				stale = append(stale, key)
			}
			return true
		})
		changed := false
		for _, key := range stale {
			changed = gameState.DeleteMapObservation(request.KingdomID, key) || changed
		}
		if !changed {
			return nil, false, nil
		}
		return []string{"map-fortress"}, true, nil
	})
	return err
}

func fortressWindowsContain(windows []towerMapWindow, x, y int) bool {
	for _, window := range windows {
		if x >= window.X1 && x <= window.X2 && y >= window.Y1 && y <= window.Y2 {
			return true
		}
	}
	return false
}

func planFortressAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, commander, err := fortressAttackContext(input, arguments, time.Now().UTC(), false)
	if err != nil {
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	resolvedArguments, _ := json.Marshal(fortressResolvedAttackRequest{fortressAttackRequest: request, CommanderID: commander})
	contextPayload, _ := json.Marshal(map[string]any{
		"SX": source.X, "SY": source.Y, "TX": target.X, "TY": target.Y,
		"KID": target.KingdomID, "LID": commander,
	})
	targetRefreshPayload, _ := json.Marshal(map[string]any{
		"KID": target.KingdomID, "AX1": target.X, "AY1": target.Y,
		"AX2": target.X, "AY2": target.Y,
	})
	targetRefreshStep := contextCommandStep("Verify fortress cooldown immediately before launch", "gaa", targetRefreshPayload, "gaa")
	targetRefreshStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	steps := make([]Intent.Step, 0, 7)
	steps = append(steps, generalSkillsContextSteps(input.State, commander, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	steps = append(steps,
		targetRefreshStep,
		deferredCRACommandStep("Build and launch fastest fortress attack", "fortress.attack.build", resolvedArguments, contextPayload),
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoFortress, SourceCastleID: source.ID, CommanderID: commander,
			KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
		}),
	)
	return Intent.Plan{
		Claims:    fortressAttackClaims(source, target, commander),
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoFortress", Affinity: "kingdom:" + strconv.Itoa(int(target.KingdomID))},
		Summary:   fmt.Sprintf("Attack kingdom fortress at %d:%d from %s", target.X, target.Y, castleLabel(source)),
		Steps:     steps,
	}, nil
}

func (application *Application) resolveFortressAttackStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request fortressResolvedAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	return buildFortressAttackStep(input, request)
}

func buildFortressAttackStep(input Intent.PlanningContext, request fortressResolvedAttackRequest) (Intent.Step, error) {
	now := time.Now().UTC()
	_, source, target, _, err := fortressAttackContext(input, mustMarshalFortressAttackRequest(request.fortressAttackRequest), now, true)
	if err != nil {
		return Intent.Step{}, err
	}
	dialog := input.State.AttackDialog
	if !fortressAttackDialogFreshForTarget(dialog, source, target, now) {
		return Intent.Step{}, fmt.Errorf("%w: current attack-dialog context does not match fortress %d:%d", Intent.ErrPlanStale, target.X, target.Y)
	}
	if dialog.Target.TowerCooldownRemaining > 0 {
		return Intent.Step{}, fmt.Errorf("%w: fortress at %d:%d is on cooldown", Intent.ErrPlanStale, target.X, target.Y)
	}
	capacity, err := resolveFortressAttackCapacity(input, source, target, request.CommanderID, true)
	if err != nil {
		return Intent.Step{}, err
	}
	required := capacity.Capacity.Left + capacity.Capacity.Right
	if err := requireFreshFortressUnits(source, required); err != nil {
		return Intent.Step{}, err
	}
	attack := towerAttackBody(source, target, request.CommanderID, State.UnitID(GameData.DirewolfUnitID), capacity.Capacity.Left, capacity.Capacity.Right)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, fmt.Errorf("resolve fortress horse travel boost: %w", err)
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build fortress CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack fortress at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
}

func fortressAttackContext(input Intent.PlanningContext, arguments json.RawMessage, now time.Time, requireFreshObservation bool) (fortressAttackRequest, State.CastleState, State.MapObservation, State.CommanderID, error) {
	var request fortressAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	if input.GameData == nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("official game data is unavailable")
	}
	if _, err := input.GameData.FortressDirewolf(); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	speedContract, err := input.GameData.FortressRelicSpeed()
	if err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	if speedContract.RelicMaximumPercent != fortressMaximumSpeedPercent {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("official fortress commander speed contract changed; refusing to launch")
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	if request.MinimumCommanderSpeed != speedContract.RelicMaximumPercent {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("fortress commander speed requirement must remain at the 100%% catalog cap")
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found || source.KingdomID != request.KingdomID || source.SlotType != 12 {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("fortress source must be the owned main castle in kingdom %d", request.KingdomID)
	}
	target, found := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !found || target.TypeID != State.MapTypeKingdomFortress {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("fortress at %d:%d is not in the current map state", request.TargetX, request.TargetY)
	}
	if requireFreshObservation && (target.ObservedAt.IsZero() || now.Before(target.ObservedAt) || now.Sub(target.ObservedAt) > fortressAttackDialogFreshness) {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("%w: fortress at %d:%d needs a fresh map observation", Intent.ErrPlanStale, target.X, target.Y)
	}
	if remaining := fortressCooldownRemaining(input.State, target, now); remaining > 0 {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("%w: fortress at %d:%d is unavailable for %s", Intent.ErrPlanStale, target.X, target.Y, (time.Duration(remaining) * time.Second).Round(time.Second))
	}
	if State.AttackFeatureTargetPendingAt(input.State, State.AttackFeatureAutoFortress, target.KingdomID, target.TypeID, target.X, target.Y, now) {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("%w: fortress at %d:%d already has an unsettled attack", Intent.ErrPlanStale, target.X, target.Y)
	}
	commander, err := fortressCommander(input, request.CommanderIDs, source, target, speedContract)
	if err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	return request, source, target, commander, nil
}

func fortressCommander(
	input Intent.PlanningContext,
	configured []State.CommanderID,
	source State.CastleState,
	target State.MapObservation,
	speedContract GameData.FortressRelicSpeedContract,
) (State.CommanderID, error) {
	if len(configured) == 0 {
		return 0, fmt.Errorf("no commander is assigned to Auto Fortress")
	}
	resolution, err := resolveCRACommanders(input.State, &craCommanderSelectionRequest{Candidates: configured, Count: 1, Strategy: "lowest_id"}, craCommanderSelectionOptions{
		Holds: input.CommanderHolds, DefaultCount: 1, RequireAvailable: true,
	})
	if err != nil || len(resolution.Selected) == 0 {
		return 0, fmt.Errorf("%w: no assigned Auto Fortress commander is available", Intent.ErrPlanStale)
	}
	commanderID := resolution.Selected[0]
	relicSpeed, found := EquipmentDomain.CommanderRelic2EffectTotal(input.State, commanderID, speedContract.RelicEffectID)
	if !found || relicSpeed+0.0001 < speedContract.RelicMaximumPercent {
		return 0, fmt.Errorf("%w: commander %d has %.0f%% of the required %.0f%% Relic 2.0 fortress speed bonus", Intent.ErrPlanStale, commanderID, relicSpeed, speedContract.RelicMaximumPercent)
	}
	speed, err := resolveFortressCommanderSpeed(input.State, input.GameData, source, target, commanderID)
	if err != nil {
		return 0, err
	}
	if speed+0.0001 < speedContract.RelicMaximumPercent {
		return 0, fmt.Errorf("%w: commander %d has %.0f%% of the required %.0f%% fortress speed bonus", Intent.ErrPlanStale, commanderID, speed, speedContract.RelicMaximumPercent)
	}
	return commanderID, nil
}

func resolveFortressCommanderSpeed(gameState State.GameState, gameData *GameData.Store, source State.CastleState, target State.MapObservation, commanderID State.CommanderID) (float64, error) {
	result, err := (AttackCapacity.Resolver{}).ResolveTravelSpeed(gameState, gameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID,
		Target: AttackCapacity.TargetContext{
			Map:   &AttackCapacity.MapTarget{KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y, Level: target.Level},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("resolve fortress commander speed: %w", err)
	}
	return result.AppliedPercent, nil
}

func resolveFortressAttackCapacity(input Intent.PlanningContext, source State.CastleState, target State.MapObservation, commanderID State.CommanderID, useDialog bool) (AttackCapacity.Result, error) {
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: useDialog,
		Target: AttackCapacity.TargetContext{
			ID:    fmt.Sprintf("fortress:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map:   &AttackCapacity.MapTarget{KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y, Level: target.Level},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, fmt.Errorf("resolve fortress attack capacity: %w", err)
	}
	return capacity, nil
}

func requireFreshFortressUnits(source State.CastleState, required int64) error {
	available := max(int64(0), source.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
	if required > 0 && available >= required {
		return nil
	}
	return fmt.Errorf("%w: %s has %d Direwolves; full fortress flanks require %d", Intent.ErrPlanStale, castleLabel(source), available, required)
}

func fortressCooldownRemaining(gameState State.GameState, target State.MapObservation, now time.Time) int {
	remaining := towerCooldownRemaining(target, gameState.UpdatedAt, now)
	key := fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)
	if cooldown, found := gameState.LookupTowerCooldown(key); found && cooldown.TargetTypeID == State.MapTypeKingdomFortress {
		personalUntil := cooldown.LastSuccessfulBattleAt.Add(fortressPersonalCooldown)
		if personalUntil.After(now) {
			seconds := int(math.Ceil(personalUntil.Sub(now).Seconds()))
			remaining = max(remaining, seconds)
		}
	}
	return remaining
}

func fortressAttackDialogFreshForTarget(dialog State.AttackDialogState, source State.CastleState, target State.MapObservation, now time.Time) bool {
	return !dialog.ObservedAt.IsZero() && !now.Before(dialog.ObservedAt) && now.Sub(dialog.ObservedAt) <= fortressAttackDialogFreshness &&
		dialog.SourceCastleID == source.ID && dialog.KingdomID == target.KingdomID &&
		dialog.Target.TypeID == State.MapTypeKingdomFortress && dialog.Target.X == target.X && dialog.Target.Y == target.Y
}

func fortressAttackClaims(source State.CastleState, target State.MapObservation, commander State.CommanderID) []string {
	claims := []string{
		"attack-context", "castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		"attack-inventory:" + strconv.FormatInt(int64(source.ID), 10),
		fmt.Sprintf("fortress-target:%d:%d:%d", target.KingdomID, target.X, target.Y),
	}
	return append(claims, craCommanderClaims([]State.CommanderID{commander})...)
}

func mustMarshalFortressAttackRequest(request fortressAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}
