package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	fortressAttackDialogFreshness = 30 * time.Second
	fortressPersonalCooldown      = 120 * time.Hour
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
	SourceCastleID     State.CastleID      `json:"sourceCastleId"`
	KingdomID          State.KingdomID     `json:"kingdomId"`
	TargetX            int                 `json:"targetX"`
	TargetY            int                 `json:"targetY"`
	CommanderIDs       []State.CommanderID `json:"commanderIds"`
	HorseTravelBoostID int                 `json:"horseTravelBoostId"`
	DailyAttackLimit   int64               `json:"dailyAttackLimit"`
	// Deprecated: accept in-flight legacy requests, but never enforce this threshold.
	MinimumCommanderSpeed float64 `json:"minimumCommanderSpeedBonus,omitempty"`
}

type fortressResolvedAttackRequest struct {
	fortressAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

type fortressTargetVerificationRequest struct {
	SourceCastleID     State.CastleID  `json:"sourceCastleId"`
	KingdomID          State.KingdomID `json:"kingdomId"`
	TargetX            int             `json:"targetX"`
	TargetY            int             `json:"targetY"`
	RequireDialogReady bool            `json:"requireDialogReady,omitempty"`
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
		Name: "Discover the full fortress map", NameDescriptor: Localization.New("server.app.discover_the_full_fortress.96e1c545", "Discover the full fortress map", nil), Action: "fortress.scan.full", ActionArguments: normalizedArguments,
	}))
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10), "map:" + strconv.Itoa(int(request.KingdomID))},
		Summary: fmt.Sprintf("Discover every fortress across %s", castleLabel(source)), SummaryDescriptor: Localization.New("server.app.discover_every_fortress_across.7acb24af", "Discover every fortress across {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))}), Steps: steps,
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
	steps = append(steps, commandStep("Refresh fortress cooldown", "gaa", payload, "gaa", Localization.New("server.app.refresh_fortress_cooldown.454352d1", "Refresh fortress cooldown", nil)))
	return Intent.Plan{
		Claims:  []string{"castle-focus", "map:" + strconv.Itoa(int(request.KingdomID))},
		Summary: fmt.Sprintf("Refresh fortress at %d:%d", request.TargetX, request.TargetY), SummaryDescriptor: Localization.New("server.app.refresh_fortress_at_p.34a49543", "Refresh fortress at {p0}:{p1}", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}), Steps: steps,
	}, nil
}

func fortressMapContext(input Intent.PlanningContext, arguments json.RawMessage, targeted bool) (fortressMapRequest, State.CastleState, error) {
	var request fortressMapRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, err
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found || source.ID <= 0 || source.KingdomID != request.KingdomID || source.SlotType != 12 || request.KingdomID < 1 || request.KingdomID > 3 {
		return request, State.CastleState{}, Localization.WithError(fmt.Errorf("fortress source must be the owned main castle in kingdom %d", request.KingdomID), Localization.New("server.app.fortress_source_must_be.2f94f62d", "fortress source must be the owned main castle in kingdom {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.KingdomID)}))
	}
	if targeted {
		target, exists := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
		if !exists || target.TypeID != State.MapTypeKingdomFortress {
			return request, State.CastleState{}, Localization.WithError(fmt.Errorf("fortress at %d:%d is not in the current map state", request.TargetX, request.TargetY), Localization.New("server.app.fortress_at_p_p.e5f83a4b", "fortress at {p0}:{p1} is not in the current map state", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
		}
	}
	return request, source, nil
}

func (application *Application) scanFullFortressMap(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.Session == nil || application.Ingest == nil {
		return Localization.WithError(fmt.Errorf("Fortress full-map scan dependencies are unavailable"), Localization.New("server.app.fortress_full_map_scan.2d1e4b8b", "Fortress full-map scan dependencies are unavailable", nil))
	}
	request, source, err := fortressMapContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments, false)
	if err != nil {
		return err
	}
	if request.ScanStartedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Fortress full-map scan is missing its start timestamp"), Localization.New("server.app.fortress_full_map_scan.cd73a84b", "Fortress full-map scan is missing its start timestamp", nil))
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
		return result, Localization.WithError(fmt.Errorf("Fortress full-map scanner is required"), Localization.New("server.app.fortress_full_map_scanner.e916c958", "Fortress full-map scanner is required", nil))
	}
	if source.X < 0 || source.Y < 0 {
		return result, Localization.WithError(fmt.Errorf("Fortress source coordinates are invalid"), Localization.New("server.app.fortress_source_coordinates_are.e9139a9f", "Fortress source coordinates are invalid", nil))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	start := fortressMapChunk{X: source.X / fortressMapChunkSize, Y: source.Y / fortressMapChunkSize}
	if start.X > fortressMapMaximumChunk || start.Y > fortressMapMaximumChunk {
		return result, Localization.WithError(fmt.Errorf("Fortress source lies beyond the full-map scan safety envelope"), Localization.New("server.app.fortress_source_lies_beyond.53ef0e98", "Fortress source lies beyond the full-map scan safety envelope", nil))
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
			failed = append(failed, Localization.WithError(fmt.Errorf("chunk %d:%d: %w", chunk.X, chunk.Y, err), Localization.ErrorContext(Localization.New("server.app.chunk_p_p.f2982535", "chunk {p0}:{p1}", Localization.Params{"p0": chunk.X, "p1": chunk.Y}), err)))
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
		failed = append(failed, Localization.WithError(fmt.Errorf("map content reached the %d-coordinate chunk safety boundary", fortressMapMaximumChunk), Localization.New("server.app.map_content_reached_the.2e7d0523", "map content reached the {p0}-coordinate chunk safety boundary", Localization.Params{"p0": fortressMapMaximumChunk})))
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
		return false, Localization.WithError(fmt.Errorf("Fortress map sender and response observer are required"), Localization.New("server.app.fortress_map_sender_and.c914eef4", "Fortress map sender and response observer are required", nil))
	}
	if !sender.CorrelatesResponses() {
		return false, Localization.WithError(fmt.Errorf("Fortress full-map scan requires correlated websocket responses"), Localization.New("server.app.fortress_full_map_scan.c1fa7b38", "Fortress full-map scan requires correlated websocket responses", nil))
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
		return false, Localization.WithError(fmt.Errorf("encode Fortress map window: %w", err), Localization.ErrorContext(Localization.New("server.app.encode_fortress_map_window.d904f92d", "encode Fortress map window", nil), err))
	}
	wire, err := Protocol.Encode(Protocol.Command{Namespace: sender.Namespace(), Opcode: "gaa", Payload: payload})
	if err != nil {
		return false, Localization.WithError(fmt.Errorf("build Fortress map window: %w", err), Localization.ErrorContext(Localization.New("server.app.build_fortress_map_window.f182be52", "build Fortress map window", nil), err))
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
			return false, Localization.WithError(fmt.Errorf("send Fortress map window: %w", err), Localization.ErrorContext(Localization.New("server.app.send_fortress_map_window.4a1f5e23", "send Fortress map window", nil), err))
		}
		if err := sender.WaitForAutomationUnlocked(requestContext); err != nil {
			return false, Localization.WithError(fmt.Errorf("Fortress map scan timed out while paused: %w", err), Localization.ErrorContext(Localization.New("server.app.fortress_map_scan_timed.22888a7e", "Fortress map scan timed out while paused", nil), err))
		}
	}
	var response Protocol.CommittedFrame
	select {
	case received, open := <-frames:
		if !open {
			return false, Localization.WithError(fmt.Errorf("Fortress map response watcher closed"), Localization.New("server.app.fortress_map_response_watcher.cfc2ff89", "Fortress map response watcher closed", nil))
		}
		response = received
	case <-requestContext.Done():
		return false, fmt.Errorf("Fortress map response timed out: %w", requestContext.Err())
	}
	if response.Frame.ResponseToken != responseToken {
		return false, Localization.WithError(fmt.Errorf("Fortress map response token changed"), Localization.New("server.app.fortress_map_response_token.b85c49ef", "Fortress map response token changed", nil))
	}
	forgetCommit := true
	defer func() {
		if forgetCommit {
			observer.ForgetCommitted(response.IngressID)
		}
	}()
	committed, err := observer.WaitCommitted(requestContext, response.IngressID)
	if err != nil {
		return false, Localization.WithError(fmt.Errorf("commit Fortress map response: %w", err), Localization.ErrorContext(Localization.New("server.app.commit_fortress_map_response.4f9a2a87", "commit Fortress map response", nil), err))
	}
	forgetCommit = false
	if committed.Frame.ResponseCode == nil {
		return false, Localization.WithError(fmt.Errorf("Fortress map response did not include a result code"), Localization.New("server.app.fortress_map_response_did.ddb71dd0", "Fortress map response did not include a result code", nil))
	}
	if *committed.Frame.ResponseCode != 0 {
		return false, Intent.NewResponseCodeError(language, committed.Frame.Opcode, *committed.Frame.ResponseCode)
	}
	if committed.ReduceError != "" {
		return false, Localization.WithError(fmt.Errorf("Fortress map response state reduction failed: %s", committed.ReduceError), Localization.New("server.app.fortress_map_response_state.6eb5e569", "Fortress map response state reduction failed: {p0}", Localization.Params{"p0": fmt.Sprintf("%s", committed.ReduceError)}))
	}
	var decoded struct {
		KingdomID State.KingdomID   `json:"KID"`
		Nodes     []json.RawMessage `json:"AI"`
	}
	if err := json.Unmarshal(committed.Frame.Payload, &decoded); err != nil {
		return false, Localization.WithError(fmt.Errorf("decode Fortress map response: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_fortress_map_response.cd09bb42", "decode Fortress map response", nil), err))
	}
	if decoded.KingdomID != kingdomID {
		return false, Localization.WithError(fmt.Errorf("Fortress map response changed kingdoms from %d to %d", kingdomID, decoded.KingdomID), Localization.New("server.app.fortress_map_response_changed.b0ad1d24", "Fortress map response changed kingdoms from {p0} to {p1}", Localization.Params{"p0": fmt.Sprintf("%d", kingdomID), "p1": fmt.Sprintf("%d", decoded.KingdomID)}))
	}
	return len(decoded.Nodes) > 0, nil
}

func (application *Application) captureFullFortressMap(request fortressMapRequest, windows []towerMapWindow) error {
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("Fortress map state is unavailable"), Localization.New("server.app.fortress_map_state_is.138aefe6", "Fortress map state is unavailable", nil))
	}
	if request.ScanStartedAt.IsZero() || len(windows) == 0 {
		return Localization.WithError(fmt.Errorf("Fortress full-map capture requires a completed scan"), Localization.New("server.app.fortress_full_map_capture.d81ab9ff", "Fortress full-map capture requires a completed scan", nil))
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentWorldMap), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || source.KingdomID != request.KingdomID || source.SlotType != 12 {
			return nil, false, Localization.WithError(fmt.Errorf("Fortress source castle changed before full-map capture"), Localization.New("server.app.fortress_source_castle_changed.fda58850", "Fortress source castle changed before full-map capture", nil))
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
	verificationArguments, _ := json.Marshal(fortressTargetVerificationRequest{
		SourceCastleID: source.ID, KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
	})
	contextPayload, _ := json.Marshal(map[string]any{
		"SX": source.X, "SY": source.Y, "TX": target.X, "TY": target.Y,
		"KID": target.KingdomID, "LID": commander, "_citadelTargetTypeId": target.TypeID,
		"_citadelFortressVerification": json.RawMessage(verificationArguments),
	})
	targetRefreshPayload, _ := json.Marshal(map[string]any{
		"KID": target.KingdomID, "AX1": target.X, "AY1": target.Y,
		"AX2": target.X, "AY2": target.Y,
	})
	targetRefreshStep := contextCommandStep("Verify fortress cooldown immediately before launch", "gaa", targetRefreshPayload, "gaa").WithNameDescriptor(Localization.New("server.app.verify_fortress_cooldown_immediately.9702b64b", "Verify fortress cooldown immediately before launch", nil))
	targetRefreshStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	targetRefreshStep.FinalDispatchAction = "fortress.target.verification.arm"
	targetRefreshStep.FinalDispatchArguments = verificationArguments
	steps := make([]Intent.Step, 0, 7)
	steps = append(steps, generalSkillsContextSteps(input.State, commander, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	steps = append(steps,
		targetRefreshStep,
		Intent.Step{Name: "Require exact fortress availability", NameDescriptor: Localization.New("server.app.require_exact_fortress_availability.08220fab", "Require exact fortress availability", nil), Action: "fortress.target.verification.guard", ActionArguments: verificationArguments},
		deferredCRACommandStep("Build and launch fastest fortress attack", "fortress.attack.build", resolvedArguments, contextPayload, Localization.New("server.app.build_and_launch_fastest.37cd6d3f", "Build and launch fastest fortress attack", nil)),
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoFortress, SourceCastleID: source.ID, CommanderID: commander,
			KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
		}),
	)
	return Intent.Plan{
		Claims:    fortressAttackClaims(source, target, commander),
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoFortress", Affinity: "kingdom:" + strconv.Itoa(int(target.KingdomID))},
		Summary:   fmt.Sprintf("Attack kingdom fortress at %d:%d from %s", target.X, target.Y, castleLabel(source)), SummaryDescriptor: Localization.New("server.app.attack_kingdom_fortress_at.af41f7d5", "Attack kingdom fortress at {p0}:{p1} from {p2}", Localization.Params{"p0": target.X, "p1": target.Y, "p2": fmt.Sprintf("%s", castleLabel(source))}),
		Steps: steps,
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
	_, source, target, currentCommander, err := fortressAttackContext(input, mustMarshalFortressAttackRequest(request.fortressAttackRequest), now, true)
	if err != nil {
		return Intent.Step{}, err
	}
	if currentCommander != request.CommanderID {
		return Intent.Step{}, fmt.Errorf("%w: fastest available fortress commander changed before launch", Intent.ErrPlanStale)
	}
	dialog := input.State.AttackDialog
	if !fortressAttackDialogFreshForTarget(dialog, source, target, now) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: current attack-dialog context does not match fortress %d:%d", Intent.ErrPlanStale, target.X, target.Y), Localization.New("server.app.intent_plan_became_stale.0d8fd6b3", "intent plan became stale before dispatch: current attack-dialog context does not match fortress {p1}:{p2}", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	if dialog.Target.TowerCooldownRemaining > 0 {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: fortress at %d:%d is on cooldown", Intent.ErrPlanStale, target.X, target.Y), Localization.New("server.app.intent_plan_became_stale.d34da5cd", "intent plan became stale before dispatch: fortress at {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve fortress horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_fortress_horse_travel.7e4df96c", "resolve fortress horse travel boost", nil), err))
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build fortress CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_fortress_cra_payload.015a5cbb", "build fortress CRA payload", nil), err))
	}
	step := commandStep(fmt.Sprintf("Attack fortress at %d:%d", target.X, target.Y), "cra", body, "cra", Localization.New("server.app.attack_fortress_at_p.a8ac22d1", "Attack fortress at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y}))
	step.FinalDispatchAction = "fortress.target.verification.guard"
	step.FinalDispatchArguments, _ = json.Marshal(fortressTargetVerificationRequest{
		SourceCastleID: source.ID, KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
		RequireDialogReady: true,
	})
	return step, nil
}

func (application *Application) armFortressTargetVerification(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("fortress target verification state is unavailable"), Localization.New("server.app.fortress_target_verification_state.970a9399", "fortress target verification state is unavailable", nil))
	}
	var request fortressTargetVerificationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	operationID := strings.TrimSpace(metadata.OperationID)
	responseToken := strings.TrimSpace(metadata.ResponseToken)
	if operationID == "" || responseToken == "" || metadata.ConnectionGeneration == 0 {
		return Localization.WithError(fmt.Errorf("fortress target verification requires correlated current-session transport metadata"), Localization.New("server.app.fortress_target_verification_requires.e7b6141e", "fortress target verification requires correlated current-session transport metadata", nil))
	}
	protocol := application.State.ProtocolContext()
	_, err := application.State.ApplyComponents(State.Components(State.ComponentSession), func(gameState *State.GameState) ([]string, bool, error) {
		source, found := gameState.Castles[request.SourceCastleID]
		if !found || !source.Focused || source.KingdomID != request.KingdomID || source.SlotType != 12 ||
			protocol.FocusedCastleID != source.ID || protocol.ConnectionGeneration != gameState.Session.ConnectionGeneration ||
			metadata.ConnectionGeneration != gameState.Session.ConnectionGeneration {
			return nil, false, Localization.WithError(fmt.Errorf("%w: fortress source focus or session changed before exact target refresh", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.b649d479", "intent plan became stale before dispatch: fortress source focus or session changed before exact target refresh", nil))
		}
		verification := State.FortressTargetVerification{
			SourceCastleID: request.SourceCastleID, KingdomID: request.KingdomID,
			TargetX: request.TargetX, TargetY: request.TargetY,
			OperationID: operationID, ResponseToken: responseToken,
			SessionGeneration:    gameState.Session.Generation,
			ConnectionGeneration: gameState.Session.ConnectionGeneration,
			FocusEpoch:           protocol.FocusEpoch, FocusSubcontext: protocol.FocusSubcontext,
			ArmedAt: time.Now().UTC(),
		}
		gameState.Session.FortressTargetVerification = verification
		return []string{"fortress-target-verification"}, true, nil
	})
	return err
}

func (application *Application) guardFortressTargetVerification(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("fortress target verification state is unavailable"), Localization.New("server.app.fortress_target_verification_state.970a9399", "fortress target verification state is unavailable", nil))
	}
	var request fortressTargetVerificationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	now := time.Now().UTC()
	state := application.State.ReadOnlyView()
	proof := state.Session.FortressTargetVerification
	protocol := application.State.ProtocolContext()
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	fail := func(reason string) error {
		return Localization.WithError(fmt.Errorf("%w: fortress at %d:%d needs a new exact availability check: %s", Intent.ErrPlanStale, request.TargetX, request.TargetY, reason), Localization.New("server.app.intent_plan_became_stale.a3ae7a65", "intent plan became stale before dispatch: fortress at {p1}:{p2} needs a new exact availability check: {p3}", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY), "p3": fmt.Sprintf("%s", reason)}))
	}
	if operationID == "" || proof.OperationID != operationID || proof.ResponseToken == "" ||
		proof.SourceCastleID != request.SourceCastleID || proof.KingdomID != request.KingdomID ||
		proof.TargetX != request.TargetX || proof.TargetY != request.TargetY {
		return fail("the response does not belong to this attack attempt")
	}
	if !proof.Complete || !proof.Available {
		reason := proof.Failure
		if reason == "" {
			reason = "the exact response did not confirm availability"
		}
		return fail(reason)
	}
	expectedFocusEpoch := proof.FocusEpoch
	if proof.FocusSubcontext != State.FocusSubcontextMap {
		expectedFocusEpoch++
	}
	if proof.SessionGeneration != state.Session.Generation ||
		proof.ConnectionGeneration == 0 || proof.ConnectionGeneration != state.Session.ConnectionGeneration ||
		protocol.SessionGeneration != state.Session.Generation ||
		protocol.ConnectionGeneration != state.Session.ConnectionGeneration ||
		expectedFocusEpoch != protocol.FocusEpoch || protocol.FocusedCastleID != request.SourceCastleID ||
		protocol.FocusSubcontext != State.FocusSubcontextMap {
		return fail("the game session or castle focus changed")
	}
	if proof.ObservedAt.IsZero() || now.Before(proof.ObservedAt) || now.Sub(proof.ObservedAt) > fortressAttackDialogFreshness {
		return fail("the exact response is stale")
	}
	target, found := state.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !found || target.TypeID != State.MapTypeKingdomFortress || !target.ObservedAt.Equal(proof.ObservedAt) {
		return fail("the current target projection no longer matches the exact response")
	}
	if remaining := fortressCooldownRemaining(state, target, now); remaining > 0 {
		return fail(fmt.Sprintf("the target is unavailable for %s", (time.Duration(remaining) * time.Second).Round(time.Second)))
	}
	if request.RequireDialogReady {
		source, found := state.Castles[request.SourceCastleID]
		if !found || !fortressAttackDialogFreshForTarget(state.AttackDialog, source, target, now) ||
			state.AttackDialog.Target.TowerCooldownRemaining > 0 {
			return fail("the attack dialog no longer confirms availability")
		}
	}
	return nil
}

func fortressAttackContext(input Intent.PlanningContext, arguments json.RawMessage, now time.Time, requireFreshObservation bool) (fortressAttackRequest, State.CastleState, State.MapObservation, State.CommanderID, error) {
	var request fortressAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	request.MinimumCommanderSpeed = 0 // Legacy field is accepted, then omitted from new steps.
	if input.GameData == nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if _, err := input.GameData.FortressDirewolf(); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, 0, err
	}
	source, found := input.State.Castles[request.SourceCastleID]
	if !found || source.KingdomID != request.KingdomID || source.SlotType != 12 {
		return request, State.CastleState{}, State.MapObservation{}, 0, Localization.WithError(fmt.Errorf("fortress source must be the owned main castle in kingdom %d", request.KingdomID), Localization.New("server.app.fortress_source_must_be.2f94f62d", "fortress source must be the owned main castle in kingdom {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.KingdomID)}))
	}
	target, found := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !found || target.TypeID != State.MapTypeKingdomFortress {
		return request, State.CastleState{}, State.MapObservation{}, 0, Localization.WithError(fmt.Errorf("fortress at %d:%d is not in the current map state", request.TargetX, request.TargetY), Localization.New("server.app.fortress_at_p_p.e5f83a4b", "fortress at {p0}:{p1} is not in the current map state", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if requireFreshObservation && (target.ObservedAt.IsZero() || now.Before(target.ObservedAt) || now.Sub(target.ObservedAt) > fortressAttackDialogFreshness) {
		return request, State.CastleState{}, State.MapObservation{}, 0, Localization.WithError(fmt.Errorf("%w: fortress at %d:%d needs a fresh map observation", Intent.ErrPlanStale, target.X, target.Y), Localization.New("server.app.intent_plan_became_stale.5fdc0f10", "intent plan became stale before dispatch: fortress at {p1}:{p2} needs a fresh map observation", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	if remaining := fortressCooldownRemaining(input.State, target, now); remaining > 0 {
		return request, State.CastleState{}, State.MapObservation{}, 0, fmt.Errorf("%w: fortress at %d:%d is unavailable for %s", Intent.ErrPlanStale, target.X, target.Y, (time.Duration(remaining) * time.Second).Round(time.Second))
	}
	if State.AttackFeatureTargetPendingAt(input.State, State.AttackFeatureAutoFortress, target.KingdomID, target.TypeID, target.X, target.Y, now) {
		return request, State.CastleState{}, State.MapObservation{}, 0, Localization.WithError(fmt.Errorf("%w: fortress at %d:%d already has an unsettled attack", Intent.ErrPlanStale, target.X, target.Y), Localization.New("server.app.intent_plan_became_stale.57437f4b", "intent plan became stale before dispatch: fortress at {p1}:{p2} already has an unsettled attack", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	commander, err := fortressCommander(input, request.CommanderIDs, source, target)
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
) (State.CommanderID, error) {
	if len(configured) == 0 {
		return 0, Localization.WithError(fmt.Errorf("no commander is assigned to Auto Fortress"), Localization.New("server.app.no_commander_is_assigned.76aaf2ee", "no commander is assigned to Auto Fortress", nil))
	}
	candidates, err := validatedCommanderCandidates(input.State, configured)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	now := time.Now().UTC()
	var selected State.CommanderID
	best, found := 0.0, false
	for _, id := range candidates {
		commander := input.State.Commanders[id]
		if !commander.Available || State.CommanderHasActiveMovementAt(input.State, id, now) ||
			State.InvasionCommanderReserved(input.State, id) || input.CommanderHolds != nil && input.CommanderHolds.CommanderHeldAt(id, now) {
			continue
		}
		speed, err := resolveFortressCommanderSpeed(input.State, input.GameData, source, target, id)
		if err != nil {
			continue
		}
		if !found || speed > best {
			selected, best, found = id, speed, true
		}
	}
	if !found {
		return 0, fmt.Errorf("%w: no assigned Auto Fortress commander is available with resolvable travel speed", Intent.ErrPlanStale)
	}
	return selected, nil
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
		return 0, Localization.WithError(fmt.Errorf("resolve fortress commander speed: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_fortress_commander_speed.1e908252", "resolve fortress commander speed", nil), err))
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
		return AttackCapacity.Result{}, Localization.WithError(fmt.Errorf("resolve fortress attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_fortress_attack_capacity.2a017c60", "resolve fortress attack capacity", nil), err))
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
