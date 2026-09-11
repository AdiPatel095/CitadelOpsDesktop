package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type invasionMapScanRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
	Radius         int            `json:"radius"`
	ScanStartedAt  time.Time      `json:"scanStartedAt"`
	// Bounds turns the scan into a single-window neighborhood refresh (at
	// most one gaa query) that is AUTHORITATIVE for its rectangle: invasion
	// observations inside it that the game did not return again are dropped.
	// It never advances the full-scan clock, so candidates outside the box
	// stay eligible. The policy uses it right before picking a target so the
	// pick is made from a set the game confirmed seconds ago.
	Bounds *State.StormMapBounds `json:"bounds,omitempty"`
}

// invasionNeighborhoodTileLimit caps a bounded scan at one gaa window, the
// same tile budget the full sweep uses per window.
const invasionNeighborhoodTileLimit = 2500

type invasionDifficultyRequest struct {
	EventID      int64 `json:"eventId"`
	DifficultyID int64 `json:"difficultyId"`
}

type invasionAttackRequest struct {
	SourceCastleID      State.CastleID       `json:"sourceCastleId"`
	EventID             int64                `json:"eventId"`
	EventEndsAt         time.Time            `json:"eventEndsAt"`
	ScoreTarget         int64                `json:"scoreTarget"`
	MinimumRemainingSec int64                `json:"minimumRemainingSec"`
	TargetTypeID        int                  `json:"targetTypeId"`
	KingdomID           State.KingdomID      `json:"kingdomId"`
	TargetX             int                  `json:"targetX"`
	TargetY             int                  `json:"targetY"`
	TargetObjectID      int64                `json:"targetObjectId,omitempty"`
	FortifyCurrency     string               `json:"fortifyCurrency,omitempty"`
	Preset              AttackPresets.Preset `json:"preset"`
	CommanderIDs        []State.CommanderID  `json:"commanderIds"`
	HorseTravelBoostID  int                  `json:"horseTravelBoostId"`
	DailyAttackLimit    int64                `json:"dailyAttackLimit"`
}

var invasionFortifyOptions = map[string]string{
	"GTO": "Gold tokens",
	"STO": "Silver tokens",
	"KM":  "Khan medals",
	"ST":  "Samurai tokens",
	"KT":  "Khan tablets",
	"C2":  "Rubies",
}

func invasionFortifyCurrencyLabel(currency string) string {
	if label := invasionFortifyOptions[currency]; label != "" {
		return label
	}
	return currency
}

func invasionFortifyCurrencyAllowedWithoutSnapshot(currency string) bool {
	switch currency {
	case "GTO", "STO", "KM", "ST":
		return true
	default:
		return false
	}
}

type resolvedInvasionAttackRequest struct {
	invasionAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

type invasionTargetReservationRequest struct {
	KingdomID        State.KingdomID
	EventID          int64
	OccurrenceEndsAt time.Time
	TargetTypeID     int
	TargetX          int
	TargetY          int
	SourceCastleID   State.CastleID
	SourceX          int
	SourceY          int
	SourceKnown      bool
	CommanderID      State.CommanderID
	CommanderKnown   bool
	TargetOnly       bool
}

type invasionTargetReconcileRequest struct {
	// FocusCastleID is only the currently configured castle used to issue the
	// GAM or GAA refresh. The reservation retains the original launch castle for
	// movement attribution even when that castle has since been lost.
	FocusCastleID      State.CastleID   `json:"sourceCastleId"`
	KingdomID          State.KingdomID  `json:"kingdomId"`
	EventID            int64            `json:"eventId"`
	OccurrenceEndsAt   time.Time        `json:"occurrenceEndsAt,omitempty"`
	TargetTypeID       int              `json:"targetTypeId"`
	TargetX            int              `json:"targetX"`
	TargetY            int              `json:"targetY"`
	OperationID        string           `json:"operationId"`
	ReservedAt         time.Time        `json:"reservedAt"`
	ReconcileAfter     time.Time        `json:"reconcileAfter,omitempty"`
	MatchedMovementID  State.MovementID `json:"matchedMovementId,omitempty"`
	ReconcileStartedAt time.Time        `json:"reconcileStartedAt,omitempty"`
}

func decodeInvasionTargetReservationRequest(arguments json.RawMessage) (invasionTargetReservationRequest, error) {
	var resolved resolvedInvasionAttackRequest
	if err := json.Unmarshal(arguments, &resolved); err != nil {
		return invasionTargetReservationRequest{}, err
	}
	if resolved.TargetTypeID > 0 {
		return invasionTargetReservationRequest{
			KingdomID: resolved.KingdomID, EventID: resolved.EventID, OccurrenceEndsAt: resolved.EventEndsAt,
			TargetTypeID: resolved.TargetTypeID,
			TargetX:      resolved.TargetX, TargetY: resolved.TargetY, SourceCastleID: resolved.SourceCastleID,
			CommanderID: resolved.CommanderID, CommanderKnown: true,
		}, nil
	}
	var route struct {
		KingdomID        State.KingdomID    `json:"KID"`
		EventID          int64              `json:"_citadelEventId"`
		OccurrenceEndsAt time.Time          `json:"_citadelEventEndsAt"`
		TargetTypeID     int                `json:"_citadelTargetTypeId"`
		TargetX          int                `json:"TX"`
		TargetY          int                `json:"TY"`
		SourceCastleID   State.CastleID     `json:"_citadelSourceCastleId"`
		SourceX          *int               `json:"SX"`
		SourceY          *int               `json:"SY"`
		CommanderID      *State.CommanderID `json:"LID"`
		TargetOnly       bool               `json:"_citadelTargetOnly"`
	}
	if err := json.Unmarshal(arguments, &route); err != nil {
		return invasionTargetReservationRequest{}, err
	}
	request := invasionTargetReservationRequest{
		KingdomID: route.KingdomID, EventID: route.EventID, OccurrenceEndsAt: route.OccurrenceEndsAt,
		TargetTypeID: route.TargetTypeID,
		TargetX:      route.TargetX, TargetY: route.TargetY, SourceCastleID: route.SourceCastleID,
	}
	if route.SourceX != nil && route.SourceY != nil {
		request.SourceX, request.SourceY, request.SourceKnown = *route.SourceX, *route.SourceY, true
	}
	request.TargetOnly = route.TargetOnly
	if route.CommanderID != nil && !route.TargetOnly {
		request.CommanderID = *route.CommanderID
		request.CommanderKnown = true
	}
	return request, nil
}

func invasionTargetOnlyReservationArguments(arguments json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &fields); err != nil {
		return nil, fmt.Errorf("decode invasion target probe reservation: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("invasion target probe reservation requires an object")
	}
	fields["_citadelTargetOnly"] = json.RawMessage(`true`)
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("encode invasion target probe reservation: %w", err)
	}
	return encoded, nil
}

type invasionTargetVerificationRequest struct {
	Request          resolvedInvasionAttackRequest `json:"request"`
	RefreshStartedAt time.Time                     `json:"refreshStartedAt"`
}

func planInvasionTargetReconcile(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request invasionTargetReconcileRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.OperationID = strings.TrimSpace(request.OperationID)
	occurrence, occurrenceKnown := input.State.LookupEventOccurrence(request.EventID)
	occurrenceAdvanced := !request.OccurrenceEndsAt.IsZero() && occurrenceKnown &&
		!State.SameEventOccurrence(request.OccurrenceEndsAt, occurrence.EndsAt)
	if request.TargetX < 0 || request.TargetY < 0 || request.OperationID == "" || request.ReservedAt.IsZero() ||
		request.MatchedMovementID <= 0 && request.FocusCastleID <= 0 && !occurrenceAdvanced {
		return Intent.Plan{}, fmt.Errorf("invasion reservation reconciliation requires a complete target boundary")
	}
	reservation, reserved := input.State.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY)
	if !reserved || reservation.OperationID != request.OperationID ||
		reservation.EventID != request.EventID ||
		reservation.TargetTypeID != request.TargetTypeID ||
		!reservation.OccurrenceEndsAt.Equal(request.OccurrenceEndsAt) ||
		!reservation.ReservedAt.Equal(request.ReservedAt) ||
		!reservation.ReconcileAfter.Equal(request.ReconcileAfter) {
		return Intent.Plan{}, fmt.Errorf("%w: invasion target reservation changed before reconciliation", Intent.ErrPlanStale)
	}
	claim := fmt.Sprintf("invasion-target:%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
	if occurrenceAdvanced {
		verification, _ := json.Marshal(request)
		return Intent.Plan{
			Claims:  []string{claim},
			Summary: fmt.Sprintf("Release prior-occurrence invasion reservation at %d:%d", request.TargetX, request.TargetY),
			Steps: []Intent.Step{{
				Name: "Release prior-occurrence invasion reservation", Action: "invasion.target.reconcile",
				ActionArguments: verification,
			}},
		}, nil
	}
	if request.MatchedMovementID > 0 {
		movement, matched := State.InvasionReservationMovement(input.State, reservation)
		if !matched || movement.ID != request.MatchedMovementID {
			return Intent.Plan{}, fmt.Errorf("%w: confirmed invasion movement changed before reconciliation", Intent.ErrPlanStale)
		}
		verification, _ := json.Marshal(request)
		return Intent.Plan{
			Claims:  []string{claim},
			Summary: fmt.Sprintf("Record confirmed invasion launch at %d:%d", request.TargetX, request.TargetY),
			Steps: []Intent.Step{{
				Name: "Record confirmed invasion target launch", Action: "invasion.target.reconcile",
				ActionArguments: verification,
			}},
		}, nil
	}
	source, exists := input.State.Castles[request.FocusCastleID]
	if !exists || source.KingdomID != request.KingdomID {
		return Intent.Plan{}, fmt.Errorf("invasion reconciliation focus castle %d is unavailable", request.FocusCastleID)
	}
	dueAt := reservation.ReservedAt.Add(State.InvasionTargetReservationReconcileGrace)
	if reservation.ReconcileAfter.After(dueAt) {
		dueAt = reservation.ReconcileAfter
	}
	if time.Now().UTC().Before(dueAt) {
		return Intent.Plan{}, fmt.Errorf("%w: invasion target reservation is still settling", Intent.ErrPlanStale)
	}
	request.ReconcileStartedAt = time.Now().UTC()
	verification, _ := json.Marshal(request)
	steps := make([]Intent.Step, 0, 3)
	if !source.Focused {
		steps = append(steps, castleFocusStep(source))
	}
	if reservation.CommanderKnown {
		gam := contextCommandStep("Refresh movements for invasion launch reconciliation", "gam", json.RawMessage(`{}`), "gam")
		gam.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, gam)
	} else {
		gaaPayload, _ := json.Marshal(struct {
			KingdomID State.KingdomID `json:"KID"`
			X1        int             `json:"AX1"`
			Y1        int             `json:"AY1"`
			X2        int             `json:"AX2"`
			Y2        int             `json:"AY2"`
		}{request.KingdomID, request.TargetX, request.TargetY, request.TargetX, request.TargetY})
		gaa := contextCommandStep("Refresh invasion target for launch reconciliation", "gaa", gaaPayload, "gaa")
		gaa.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, gaa)
	}
	steps = append(steps, Intent.Step{
		Name:   "Reconcile unresolved invasion target launch",
		Action: "invasion.target.reconcile", ActionArguments: verification,
	})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + castleID,
			"map:" + strconv.FormatInt(int64(request.KingdomID), 10),
			claim,
		},
		Summary: fmt.Sprintf("Reconcile unresolved invasion launch at %d:%d", request.TargetX, request.TargetY),
		Steps:   steps,
	}, nil
}

func planInvasionMapScan(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := invasionMapScanContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	windows := towerMapScanWindows(source, request.Radius)
	if request.Bounds != nil {
		// A neighborhood refresh is exactly one window over the requested box.
		windows = []towerMapWindow{{
			X1: request.Bounds.X1, Y1: request.Bounds.Y1, X2: request.Bounds.X2, Y2: request.Bounds.Y2,
		}}
	}
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
			fmt.Sprintf("Refresh invasion map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa",
		))
	}
	steps = append(steps, Intent.Step{
		Name: "Record invasion map scan", Action: "invasion.scan.capture", ActionArguments: append(json.RawMessage(nil), arguments...),
	})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: fmt.Sprintf("Refresh invasion targets around %s", castleLabel(source)), Steps: steps,
	}, nil
}

func planInvasionDifficulty(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request invasionDifficultyRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	if _, supported := invasionMapTypeForEvent(request.EventID); !supported {
		return Intent.Plan{}, fmt.Errorf("event %d is not supported by Auto Invasion", request.EventID)
	}
	difficulty, valid := input.GameData.ScalableEvent(request.EventID, request.DifficultyID)
	if !valid {
		return Intent.Plan{}, fmt.Errorf("difficulty %d is not valid for event %d", request.DifficultyID, request.EventID)
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !input.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return Intent.Plan{}, fmt.Errorf("difficulty %d is not unlocked by this player's achievements", request.DifficultyID)
	}
	score, active := input.State.ActiveScalableEventScore()
	if !active || score.EventID != request.EventID {
		return Intent.Plan{}, fmt.Errorf("event %d is no longer active", request.EventID)
	}
	if score.DifficultyID > 0 {
		return Intent.Plan{}, fmt.Errorf("event %d already selected difficulty %d", request.EventID, score.DifficultyID)
	}
	payload, _ := json.Marshal(struct {
		EventID      int64 `json:"EID"`
		DifficultyID int64 `json:"EDID"`
		PremiumUsed  int   `json:"C2U"`
	}{request.EventID, request.DifficultyID, 0})
	return Intent.Plan{
		Claims:  []string{"event-difficulty"},
		Summary: fmt.Sprintf("Select difficulty %d for invasion event %d", request.DifficultyID, request.EventID),
		Steps:   []Intent.Step{commandStep("Select invasion event difficulty", "sede", payload, "sede")},
	}, nil
}

func planInvasionAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := invasionAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	var commanderSelection *craCommanderSelectionRequest
	if request.CommanderIDs != nil {
		if len(request.CommanderIDs) == 0 {
			return Intent.Plan{}, fmt.Errorf("no commanders are assigned to Auto Invasion")
		}
		commanderSelection = &craCommanderSelectionRequest{
			Candidates: request.CommanderIDs,
			Count:      1,
			Strategy:   "lowest_id",
		}
	}
	resolution, err := resolveCRACommanders(
		input.State,
		commanderSelection,
		craCommanderSelectionOptions{Holds: input.CommanderHolds, DefaultCount: 1, RequireAvailable: true},
	)
	if err != nil {
		if errors.Is(err, errCRACommanderUnavailable) {
			return Intent.Plan{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
		}
		return Intent.Plan{}, err
	}
	commanderID := resolution.Selected[0]
	resolvedRequest := resolvedInvasionAttackRequest{invasionAttackRequest: request, CommanderID: commanderID}
	resolvedArguments, _ := json.Marshal(resolvedRequest)
	contextPayload, _ := json.Marshal(struct {
		SourceX        int               `json:"SX"`
		SourceY        int               `json:"SY"`
		TargetX        int               `json:"TX"`
		TargetY        int               `json:"TY"`
		KingdomID      State.KingdomID   `json:"KID"`
		CommanderID    State.CommanderID `json:"LID"`
		TargetTypeID   int               `json:"_citadelTargetTypeId"`
		EventID        int64             `json:"_citadelEventId"`
		EventEndsAt    time.Time         `json:"_citadelEventEndsAt"`
		SourceCastleID State.CastleID    `json:"_citadelSourceCastleId"`
		InvasionGuard  json.RawMessage   `json:"_citadelInvasionGuard"`
	}{
		SourceX: source.X, SourceY: source.Y, TargetX: target.X, TargetY: target.Y,
		KingdomID: target.KingdomID, CommanderID: commanderID, TargetTypeID: target.TypeID,
		EventID: request.EventID, EventEndsAt: request.EventEndsAt, SourceCastleID: source.ID,
		InvasionGuard: resolvedArguments,
	})
	consumeArguments, _ := json.Marshal(map[string]any{
		"kingdomId": target.KingdomID, "targetTypeId": target.TypeID,
		"targetX": target.X, "targetY": target.Y, "targetObjectId": target.ObjectID,
	})
	steps := make([]Intent.Step, 0, 5)
	if input.State.Player.LegendSkills.ObservedAt.IsZero() ||
		time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute {
		steps = append(steps, contextCommandStep(
			"Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl",
		))
	}
	steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	refreshStartedAt := time.Now().UTC()
	refreshPayload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X1        int             `json:"AX1"`
		Y1        int             `json:"AY1"`
		X2        int             `json:"AX2"`
		Y2        int             `json:"AY2"`
	}{target.KingdomID, target.X, target.Y, target.X, target.Y})
	refreshStep := contextCommandStep("Refresh selected invasion target", "gaa", refreshPayload, "gaa")
	refreshStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	verificationArguments, _ := json.Marshal(invasionTargetVerificationRequest{
		Request:          resolvedInvasionAttackRequest{invasionAttackRequest: request, CommanderID: commanderID},
		RefreshStartedAt: refreshStartedAt,
	})
	steps = append(steps,
		refreshStep,
		Intent.Step{Name: "Verify refreshed invasion target", Action: "invasion.target.guard", ActionArguments: verificationArguments},
	)
	if request.FortifyCurrency != "" && !invasionTargetFortified(input.State, target) {
		fortifyArguments, _ := json.Marshal(request)
		fortifyPayload, _ := json.Marshal(struct {
			X        int    `json:"XPOS"`
			Y        int    `json:"YPOS"`
			Currency string `json:"RCK"`
		}{target.X, target.Y, request.FortifyCurrency})
		steps = append(steps,
			Intent.Step{Name: "Verify invasion fortification currency", Action: "invasion.fortify.guard", ActionArguments: fortifyArguments},
			commandStep("Fortify invasion castle with "+invasionFortifyCurrencyLabel(request.FortifyCurrency), "rae", fortifyPayload, "rae"),
		)
	}
	launchStep := deferredCRACommandStep(
		"Build and launch capacity-adjusted invasion attack", "invasion.attack.build", resolvedArguments, contextPayload,
	)
	steps = append(steps,
		launchStep,
		Intent.Step{Name: "Consume invasion target", Action: "invasion.target.consume", ActionArguments: consumeArguments},
		Intent.Step{Name: "Record invasion launch", Action: "invasion.attack.capture", ActionArguments: resolvedArguments},
	)
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		"map:" + strconv.FormatInt(int64(target.KingdomID), 10),
		fmt.Sprintf("invasion-target:%d:%d:%d", target.KingdomID, target.X, target.Y),
	}
	if request.FortifyCurrency != "" {
		claims = append(claims, "account-resources")
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{commanderID})...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoInvasion", Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Attack invasion castle at %d:%d with %s", target.X, target.Y, request.Preset.Name),
		Steps:   steps,
	}, nil
}

func invasionMapScanContext(input Intent.PlanningContext, arguments json.RawMessage) (invasionMapScanRequest, State.CastleState, error) {
	var request invasionMapScanRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return invasionMapScanRequest{}, State.CastleState{}, err
	}
	if request.SourceCastleID <= 0 || request.Radius < 1 || request.Radius > 50 {
		return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion map scan requires a source castle and radius between 1 and 50")
	}
	if bounds := request.Bounds; bounds != nil {
		if !bounds.IsValid() {
			return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion neighborhood scan bounds are invalid")
		}
		if (bounds.X2-bounds.X1+1)*(bounds.Y2-bounds.Y1+1) > invasionNeighborhoodTileLimit {
			return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf(
				"invasion neighborhood scan may cover at most %d tiles", invasionNeighborhoodTileLimit,
			)
		}
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion source castle %d is unavailable", request.SourceCastleID)
	}
	if source.KingdomID != 0 {
		return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion source castle must be in the Great Empire")
	}
	return request, source, nil
}

func invasionAttackContext(input Intent.PlanningContext, arguments json.RawMessage) (invasionAttackRequest, State.CastleState, State.MapObservation, error) {
	var request invasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(time.Now().UTC()) {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{},
			fmt.Errorf("invasion attacks are disabled while Protection Mode is preparing or active")
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if request.SourceCastleID <= 0 || request.EventID <= 0 || request.EventEndsAt.IsZero() ||
		request.ScoreTarget <= 0 || request.TargetTypeID <= 0 {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"invasion attack requires source, event occurrence, score target, and target type",
		)
	}
	request.FortifyCurrency = strings.ToUpper(strings.TrimSpace(request.FortifyCurrency))
	if request.FortifyCurrency != "" && request.FortifyCurrency != "C2" {
		if len(input.State.Invasion.FortifyCurrencies) > 0 {
			if !input.State.Invasion.SupportsFortifyCurrency(request.FortifyCurrency) {
				return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
					"fortification currency %s is unavailable for event %d", request.FortifyCurrency, request.EventID,
				)
			}
		} else if !invasionFortifyCurrencyAllowedWithoutSnapshot(request.FortifyCurrency) {
			return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("unsupported invasion fortification currency %q", request.FortifyCurrency)
		}
	}
	expectedTypeID, supported := invasionMapTypeForEvent(request.EventID)
	if !supported || expectedTypeID != request.TargetTypeID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("event %d does not use invasion target type %d", request.EventID, request.TargetTypeID)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion source castle %d is unavailable", request.SourceCastleID)
	}
	if source.KingdomID != 0 || request.KingdomID != source.KingdomID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion attack source and target must be in the Great Empire")
	}
	score, active := input.State.ActiveScalableEventScore()
	if !active || score.EventID != request.EventID ||
		!State.SameEventOccurrence(request.EventEndsAt, State.ScalableEventEndsAt(score)) {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: invasion event %d occurrence changed",
			Intent.ErrPlanStale, request.EventID,
		)
	}
	target, exists := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || target.TypeID != request.TargetTypeID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion target %d:%d is no longer available", request.TargetX, request.TargetY)
	}
	if request.TargetObjectID > 0 && target.ObjectID > 0 && target.ObjectID != request.TargetObjectID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion target %d:%d changed appearance", request.TargetX, request.TargetY)
	}
	now := time.Now().UTC()
	if !target.InvasionAvailabilityKnown || target.Level <= 0 {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: invasion target %d:%d does not have confirmed attack availability",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	if target.InvasionProtected || input.State.Invasion.TargetUnavailable(request.KingdomID, request.TargetX, request.TargetY) {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: invasion target %d:%d is hidden or protected",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	if State.AttackFeatureTargetPendingAt(
		input.State, State.AttackFeatureAutoInvasion, request.KingdomID, request.TargetTypeID,
		request.TargetX, request.TargetY, now,
	) {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: invasion target %d:%d has a prior attack awaiting settlement",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	if _, reserved := input.State.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY); reserved {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: invasion target %d:%d has an unresolved launch",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	if State.AnyActiveMovementAtMapTarget(input.State, State.MapTargetKey{
		KingdomID: request.KingdomID, TypeID: request.TargetTypeID, X: request.TargetX, Y: request.TargetY,
	}, now) {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: invasion target %d:%d already has an active movement",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	return request, source, target, nil
}

func invasionAttackSetup(preset AttackPresets.Preset) attackSetupRequest {
	waves := make([]attackSetupWaveRequest, 0, len(preset.Waves))
	for _, wave := range preset.Waves {
		waves = append(waves, attackSetupWaveRequest{
			Left: invasionAttackLane(wave.Left), Middle: invasionAttackLane(wave.Middle), Right: invasionAttackLane(wave.Right),
		})
	}
	convert := func(slots []AttackPresets.Slot) []attackSetupSlotRequest {
		result := make([]attackSetupSlotRequest, len(slots))
		for index, slot := range slots {
			result[index] = attackSetupSlotRequest{ItemID: slot.ItemID, Quantity: slot.Quantity}
		}
		return result
	}
	return attackSetupRequest{
		Name: preset.Name, UseTroopFamilies: preset.UseTroopFamilies,
		Waves: waves,
		CourtyardSupport: attackSetupCourtyardSupport{
			Troops: convert(preset.CourtyardSupport.Troops),
			Tools:  convert(preset.CourtyardSupport.Tools),
		},
	}
}

func invasionAttackLane(lane AttackPresets.Lane) attackSetupLaneRequest {
	convert := func(slots []AttackPresets.Slot) []attackSetupSlotRequest {
		result := make([]attackSetupSlotRequest, len(slots))
		for index, slot := range slots {
			result[index] = attackSetupSlotRequest{ItemID: slot.ItemID, Quantity: slot.Quantity}
		}
		return result
	}
	return attackSetupLaneRequest{Troops: convert(lane.Troops), Tools: convert(lane.Tools)}
}

func invasionAttackBody(
	source State.CastleState,
	target State.MapObservation,
	commanderID State.CommanderID,
	setup builtAttackSetup,
) attackBody {
	return attackBody{
		SourceX: source.X, SourceY: source.Y, TargetX: target.X, TargetY: target.Y,
		Kingdom: target.KingdomID, Leader: commanderID, Booster: -1, Valid: 1,
		PremiumTravel: 1, Cooldown: 99, Waves: setup.Waves, Books: []any{},
		AttackSupportTools: setup.SupportTools,
		SupportTroops:      setup.SupportTroops,
	}
}

func (application *Application) resolveInvasionAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request resolvedInvasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	capacity, source, target, err := resolveInvasionAttackCapacity(input, request.invasionAttackRequest, request.CommanderID)
	if err != nil {
		return Intent.Step{}, err
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(request.Preset, capacity))
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build invasion preset %q: %w", request.Preset.Name, err)
	}
	attack := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, fmt.Errorf("resolve invasion horse travel boost: %w", err)
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build invasion CRA payload: %w", err)
	}
	step := commandStep(fmt.Sprintf("Attack invasion castle at %d:%d", target.X, target.Y), "cra", body, "cra")
	step.StaleCodes = []int{95}
	step.PreDispatchAction = "invasion.target.reserve"
	step.PreDispatchArguments = append(json.RawMessage(nil), arguments...)
	step.DefinitiveSendFailureAction = "invasion.target.release"
	step.DefinitiveSendFailureArguments = append(json.RawMessage(nil), arguments...)
	step.DefinitiveResponseFailureAction = "invasion.target.release"
	step.DefinitiveResponseFailureArguments = append(json.RawMessage(nil), arguments...)
	step.StaleResponseAction = "invasion.target.cooldown"
	step.StaleResponseArguments = append(json.RawMessage(nil), arguments...)
	step.ResponseProjectionFailureIndeterminate = true
	return step, nil
}

func resolveInvasionAttackCapacity(
	input Intent.PlanningContext,
	request invasionAttackRequest,
	commanderID State.CommanderID,
) (AttackCapacity.Result, State.CastleState, State.MapObservation, error) {
	_, source, target, err := invasionAttackContext(input, mustMarshalInvasionAttackRequest(request))
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, err
	}
	commander, exists := input.State.Commanders[commanderID]
	if !exists || !commander.Available || State.CommanderHasActiveMovementAt(input.State, commanderID, time.Now().UTC()) ||
		State.InvasionCommanderReserved(input.State, commanderID) {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"%w: commander %d is no longer available", Intent.ErrPlanStale, commanderID,
		)
	}
	level := target.Level
	if level <= 0 {
		level = int(target.ObjectID)
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("invasion:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: level,
			},
			Level: level, CastleTypeID: target.TypeID, PvP: true, LegendaryFight: true,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("resolve invasion attack capacity: %w", err)
	}
	return capacity, source, target, nil
}

func (application *Application) captureInvasionScan(_ context.Context, arguments json.RawMessage) error {
	request, _, err := invasionMapScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentInvasion, State.ComponentWorldMap), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("invasion source castle %d is no longer focused", request.SourceCastleID)
		}
		scannedAt := request.ScanStartedAt.UTC()
		if scannedAt.IsZero() {
			scannedAt = time.Now().UTC()
		}
		if bounds := request.Bounds; bounds != nil {
			// The neighborhood window is authoritative for its rectangle: any
			// invasion castle inside it that the game did not return again is
			// gone (defeated, or the slot reverted to a dynamic area). Drop those
			// phantoms so the pick that follows is made from a confirmed set.
			// The full-scan clock is deliberately left alone — candidates outside
			// the box remain eligible on their last full observation.
			stale := []State.MapObservation{}
			gameState.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionInvasion, func(key string, observation State.MapObservation) bool {
				if observation.X < bounds.X1 || observation.X > bounds.X2 || observation.Y < bounds.Y1 || observation.Y > bounds.Y2 {
					return true
				}
				if observation.ObservedAt.Before(scannedAt) {
					stale = append(stale, observation)
				}
				return true
			})
			changed := false
			for _, target := range stale {
				key := fmt.Sprintf("%d:%d", target.X, target.Y)
				if gameState.DeleteMapObservation(source.KingdomID, key) {
					changed = true
				}
				if gameState.Invasion.ClearTargetFortification(source.KingdomID, target.X, target.Y) {
					changed = true
				}
			}
			if clearUnconfirmedInvasionFortifications(gameState, source.KingdomID, scannedAt, func(x, y int) bool {
				return x >= bounds.X1 && x <= bounds.X2 && y >= bounds.Y1 && y <= bounds.Y2
			}) {
				changed = true
			}
			if !changed {
				return nil, false, nil
			}
			return []string{"map", "map-invasion", "invasion"}, true, nil
		}
		// The complete sweep is authoritative for every invasion coordinate in
		// range. Remove castles that were not returned after ScanStartedAt and
		// clear their fortification marker so a later camp at the same coordinate
		// cannot inherit stale "already fortified" state when HAC was missed.
		stale := []State.MapObservation{}
		gameState.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionInvasion, func(_ string, observation State.MapObservation) bool {
			dx, dy := observation.X-source.X, observation.Y-source.Y
			if dx*dx+dy*dy <= request.Radius*request.Radius && observation.ObservedAt.Before(scannedAt) {
				stale = append(stale, observation)
			}
			return true
		})
		changed := false
		for _, target := range stale {
			key := fmt.Sprintf("%d:%d", target.X, target.Y)
			if current, exists := gameState.LookupMapObservation(source.KingdomID, key); exists && current.TypeID == target.TypeID &&
				current.ObservedAt.Equal(target.ObservedAt) && gameState.DeleteMapObservation(source.KingdomID, key) {
				changed = true
			}
			if gameState.Invasion.ClearTargetFortification(source.KingdomID, target.X, target.Y) {
				changed = true
			}
		}
		if clearUnconfirmedInvasionFortifications(gameState, source.KingdomID, scannedAt, func(x, y int) bool {
			dx, dy := x-source.X, y-source.Y
			return dx*dx+dy*dy <= request.Radius*request.Radius
		}) {
			changed = true
		}
		if gameState.Invasion.LastScannedAt == nil {
			gameState.Invasion.LastScannedAt = map[State.CastleID]time.Time{}
		}
		if !gameState.Invasion.LastScannedAt[request.SourceCastleID].Equal(scannedAt) {
			gameState.Invasion.LastScannedAt[request.SourceCastleID] = scannedAt
			changed = true
		}
		if !changed {
			return nil, false, nil
		}
		return []string{"invasion", "map", "map-invasion"}, true, nil
	})
	return err
}

func (application *Application) captureInvasionLaunch(ctx context.Context, arguments json.RawMessage) error {
	var request resolvedInvasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	event, err := application.State.ApplyComponents(
		State.Components(
			State.ComponentEventScores, State.ComponentAttackAnalytics,
			State.ComponentInvasion, State.ComponentReports,
		),
		func(gameState *State.GameState) ([]string, bool, error) {
			reservation, reserved := gameState.Invasion.TargetReservation(
				request.KingdomID, request.TargetX, request.TargetY,
			)
			if reserved && (operationID == "" || reservation.OperationID != operationID) {
				return nil, false, fmt.Errorf("invasion launch reservation does not belong to this operation")
			}
			if reserved && (reservation.EventID != request.EventID ||
				!reservation.OccurrenceEndsAt.Equal(request.EventEndsAt) ||
				reservation.TargetTypeID != request.TargetTypeID ||
				reservation.SourceCastleID != request.SourceCastleID ||
				!reservation.CommanderKnown || reservation.CommanderID != request.CommanderID) {
				return nil, false, fmt.Errorf("invasion launch reservation boundary changed before capture")
			}
			alreadyRecorded := func(movementID State.MovementID) bool {
				for _, recorded := range gameState.AttackAnalytics.LaunchIDs {
					if recorded == movementID {
						return true
					}
				}
				return false
			}
			if reserved {
				selected, matched := State.InvasionReservationMovement(*gameState, reservation)
				if !matched {
					return nil, false, fmt.Errorf(
						"CRA response did not return commander %d's exact invasion movement", request.CommanderID,
					)
				}
				return recordReconciledInvasionLaunch(gameState, reservation, selected, false)
			}
			recordedMatch := false
			gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
				if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID ||
					movement.KingdomID != request.KingdomID || movement.TargetTypeID != request.TargetTypeID ||
					movement.TargetX != request.TargetX || movement.TargetY != request.TargetY ||
					movement.CommanderID == nil || *movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil {
					return true
				}
				if alreadyRecorded(movement.ID) {
					recordedMatch = true
					return false
				}
				return true
			})
			if recordedMatch {
				// The movement reducer can account and release the reservation
				// before this post-send action runs. Emit a component checkpoint
				// so the successful intent still waits for those exact results to
				// become durable instead of relying on the asynchronous flush. Do
				// this before checking the current event: the occurrence may advance
				// between the accepted CRA and its post-send capture action.
				return []string{"event-scores", "attack-analytics", "invasion", "reports"}, true, nil
			}
			return nil, false, fmt.Errorf("CRA response did not return an already-accounted invasion movement")
		})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func clearUnconfirmedInvasionFortifications(
	gameState *State.GameState,
	kingdomID State.KingdomID,
	scannedAt time.Time,
	inScope func(x, y int) bool,
) bool {
	if gameState == nil || inScope == nil {
		return false
	}
	changed := false
	for key := range gameState.Invasion.FortifiedTargets {
		keyKingdomID, x, y, valid := State.ParseInvasionTargetKey(key)
		if !valid || keyKingdomID != kingdomID || !inScope(x, y) {
			continue
		}
		target, exists := gameState.LookupMapObservation(kingdomID, fmt.Sprintf("%d:%d", x, y))
		confirmed := exists && (target.TypeID == State.MapTypeForeignLord || target.TypeID == State.MapTypeBloodcrow) &&
			!target.ObservedAt.Before(scannedAt)
		if !confirmed && gameState.Invasion.ClearTargetFortification(kingdomID, x, y) {
			changed = true
		}
	}
	return changed
}

func (application *Application) reserveInvasionTarget(ctx context.Context, arguments json.RawMessage) error {
	// Concrete CRA steps carry the full resolved request. Re-run the entire
	// launch gate here, after every deferred dependency and immediately before
	// the durable reservation/send boundary. ADI probes carry only private route
	// metadata and are guarded by their subsequent concrete CRA path.
	var resolved resolvedInvasionAttackRequest
	if json.Unmarshal(arguments, &resolved) == nil && resolved.EventID > 0 && resolved.TargetTypeID > 0 {
		if err := application.guardInvasionAttack(ctx, arguments); err != nil {
			return err
		}
	}
	request, err := decodeInvasionTargetReservationRequest(arguments)
	if err != nil {
		return err
	}
	if request.EventID <= 0 || request.OccurrenceEndsAt.IsZero() ||
		request.TargetTypeID != State.MapTypeForeignLord && request.TargetTypeID != State.MapTypeBloodcrow ||
		request.TargetX < 0 || request.TargetY < 0 || request.SourceCastleID <= 0 ||
		!request.CommanderKnown && !request.TargetOnly {
		return fmt.Errorf("invasion target reservation requires a valid invasion castle")
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	if operationID == "" {
		return fmt.Errorf("invasion target reservation requires an operation id")
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentInvasion), func(gameState *State.GameState) ([]string, bool, error) {
		if !request.SourceKnown {
			source, exists := gameState.Castles[request.SourceCastleID]
			if !exists || source.KingdomID != request.KingdomID {
				return nil, false, fmt.Errorf("%w: invasion launch source castle changed", Intent.ErrPlanStale)
			}
			request.SourceX, request.SourceY, request.SourceKnown = source.X, source.Y, true
		}
		if current, reserved := gameState.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY); reserved {
			if current.OperationID == operationID {
				if current.EventID != request.EventID || !current.OccurrenceEndsAt.Equal(request.OccurrenceEndsAt) ||
					current.TargetTypeID != request.TargetTypeID || current.SourceCastleID != request.SourceCastleID ||
					!current.SourceKnown || current.SourceX != request.SourceX || current.SourceY != request.SourceY ||
					current.CommanderKnown != request.CommanderKnown ||
					request.CommanderKnown && current.CommanderID != request.CommanderID {
					return nil, false, fmt.Errorf("%w: invasion launch reservation boundary changed", Intent.ErrPlanStale)
				}
				return nil, false, nil
			}
			return nil, false, fmt.Errorf(
				"%w: invasion target %d:%d is reserved by another launch",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			)
		}
		if request.CommanderKnown && State.InvasionCommanderReserved(*gameState, request.CommanderID) {
			return nil, false, fmt.Errorf(
				"%w: invasion commander %d is reserved by another launch",
				Intent.ErrPlanStale, request.CommanderID,
			)
		}
		changed := gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
			KingdomID: request.KingdomID, EventID: request.EventID, OccurrenceEndsAt: request.OccurrenceEndsAt,
			TargetTypeID: request.TargetTypeID,
			X:            request.TargetX, Y: request.TargetY, SourceCastleID: request.SourceCastleID,
			SourceX: request.SourceX, SourceY: request.SourceY, SourceKnown: request.SourceKnown,
			CommanderID: request.CommanderID, CommanderKnown: request.CommanderKnown,
			OperationID: operationID, ReservedAt: time.Now().UTC(),
		})
		return []string{"invasion"}, changed, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) releaseInvasionTarget(ctx context.Context, arguments json.RawMessage) error {
	request, err := decodeInvasionTargetReservationRequest(arguments)
	if err != nil {
		return err
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	if operationID == "" {
		return fmt.Errorf("invasion target release requires an operation id")
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentInvasion), func(gameState *State.GameState) ([]string, bool, error) {
		changed := gameState.Invasion.ReleaseTargetReservation(
			request.KingdomID, request.TargetX, request.TargetY, operationID,
		)
		return []string{"invasion"}, changed, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

// cooldownInvasionTarget handles the game's explicit code 95 rejection. The
// command did not launch, so its commander can be released; the target remains
// unavailable until a strictly newer GAA observation confirms it is attackable
// again. Missing responses and projection failures never take this path.
func (application *Application) cooldownInvasionTarget(ctx context.Context, arguments json.RawMessage) error {
	request, err := decodeInvasionTargetReservationRequest(arguments)
	if err != nil {
		return err
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	if operationID == "" {
		return fmt.Errorf("invasion target cooldown requires an operation id")
	}
	observedAt := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentInvasion), func(gameState *State.GameState) ([]string, bool, error) {
		released := false
		if reservation, reserved := gameState.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY); reserved {
			if reservation.OperationID != operationID || reservation.TargetTypeID != request.TargetTypeID {
				return nil, false, fmt.Errorf("%w: invasion target cooldown boundary changed", Intent.ErrPlanStale)
			}
			released = gameState.Invasion.ReleaseTargetReservation(
				request.KingdomID, request.TargetX, request.TargetY, operationID,
			)
		}
		changed := gameState.Invasion.MarkTargetUnavailable(
			request.KingdomID, request.TargetX, request.TargetY, observedAt,
		)
		return []string{"invasion", "map-invasion"}, changed || released, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) reconcileInvasionTargetReservation(ctx context.Context, arguments json.RawMessage) error {
	var request invasionTargetReconcileRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	request.OperationID = strings.TrimSpace(request.OperationID)
	event, err := application.State.ApplyComponents(
		State.Components(
			State.ComponentInvasion, State.ComponentWorldMap,
			State.ComponentEventScores, State.ComponentAttackAnalytics, State.ComponentReports,
		),
		func(gameState *State.GameState) ([]string, bool, error) {
			reservation, reserved := gameState.Invasion.TargetReservation(
				request.KingdomID, request.TargetX, request.TargetY,
			)
			if !reserved || reservation.OperationID != request.OperationID ||
				reservation.EventID != request.EventID ||
				reservation.TargetTypeID != request.TargetTypeID ||
				!reservation.OccurrenceEndsAt.Equal(request.OccurrenceEndsAt) ||
				!reservation.ReservedAt.Equal(request.ReservedAt) ||
				!reservation.ReconcileAfter.Equal(request.ReconcileAfter) {
				return nil, false, fmt.Errorf("%w: invasion target reservation changed during reconciliation", Intent.ErrPlanStale)
			}
			expectedTypeID, supported := invasionMapTypeForEvent(reservation.EventID)
			occurrence, occurrenceKnown := gameState.LookupEventOccurrence(reservation.EventID)
			if !supported || expectedTypeID != reservation.TargetTypeID ||
				reservation.OccurrenceEndsAt.IsZero() || occurrenceKnown &&
				!State.SameEventOccurrence(reservation.OccurrenceEndsAt, occurrence.EndsAt) {
				// The recurring event advanced (or this is a legacy unbound marker).
				// Release the no-replay lock, but never attach a future occurrence's
				// movement or score to the old launch.
				changed := gameState.Invasion.ReleaseTargetReservation(
					request.KingdomID, request.TargetX, request.TargetY, request.OperationID,
				)
				return []string{"invasion"}, changed, nil
			}
			if request.MatchedMovementID > 0 {
				movement, matched := State.InvasionReservationMovement(*gameState, reservation)
				if !matched || movement.ID != request.MatchedMovementID {
					return nil, false, fmt.Errorf("%w: confirmed invasion movement changed during reconciliation", Intent.ErrPlanStale)
				}
				return recordReconciledInvasionLaunch(gameState, reservation, movement, false)
			}
			if request.ReconcileStartedAt.IsZero() ||
				request.ReconcileStartedAt.Before(reservation.ReservedAt.Add(State.InvasionTargetReservationReconcileGrace)) {
				return nil, false, fmt.Errorf("%w: invasion reconciliation does not have a valid refresh boundary", Intent.ErrPlanStale)
			}
			if reservation.CommanderKnown {
				if gameState.MovementSnapshot.ObservedAt.IsZero() ||
					!gameState.MovementSnapshot.ObservedAt.After(request.ReconcileStartedAt) {
					return nil, false, fmt.Errorf("%w: invasion reconciliation does not have a fresh movement snapshot", Intent.ErrPlanStale)
				}
				if movement, matched := State.InvasionReservationMovement(*gameState, reservation); matched {
					return recordReconciledInvasionLaunch(gameState, reservation, movement, false)
				}
				// GAM responses are scoped and can omit a movement that was accepted
				// by CRA. An empty/partial refresh is therefore only a signal to try
				// again; it must never release the target or commander binding.
				changed := gameState.Invasion.BackoffTargetReservation(
					request.KingdomID, request.TargetX, request.TargetY, request.OperationID,
					time.Now().UTC().Add(State.InvasionTargetReservationReconcileGrace),
				)
				return []string{"invasion"}, changed, nil
			}
			key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
			target, exists := gameState.LookupMapObservation(request.KingdomID, key)
			targetFresh := exists && target.TypeID == request.TargetTypeID && !target.ObservedAt.IsZero() &&
				target.ObservedAt.After(request.ReconcileStartedAt)
			mapChanged := false
			if !targetFresh {
				if exists && target.TypeID == request.TargetTypeID {
					mapChanged = gameState.DeleteMapObservation(request.KingdomID, key)
				}
				if gameState.Invasion.ClearTargetFortification(request.KingdomID, request.TargetX, request.TargetY) {
					mapChanged = true
				}
			}
			if !occurrenceKnown {
				changed := gameState.Invasion.ReleaseTargetReservation(
					request.KingdomID, request.TargetX, request.TargetY, request.OperationID,
				)
				return []string{"invasion"}, changed, nil
			}
			if State.AnyActiveMovementAtMapTarget(*gameState, State.MapTargetKey{
				KingdomID: request.KingdomID, TypeID: request.TargetTypeID,
				X: request.TargetX, Y: request.TargetY,
			}, time.Now().UTC()) {
				return nil, false, fmt.Errorf("%w: the unresolved invasion launch now has an active movement", Intent.ErrPlanStale)
			}
			if !targetFresh {
				reservationChanged := gameState.Invasion.ReleaseTargetReservation(
					request.KingdomID, request.TargetX, request.TargetY, request.OperationID,
				)
				return []string{"invasion", "map-invasion"}, mapChanged || reservationChanged, nil
			}
			if !target.InvasionAvailabilityKnown || target.Level <= 0 {
				changed := gameState.Invasion.DeferTargetReservation(
					request.KingdomID, request.TargetX, request.TargetY, request.OperationID,
					time.Now().UTC().Add(State.InvasionTargetReservationReconcileGrace),
				)
				return []string{"invasion"}, changed, nil
			}
			changed := gameState.Invasion.ReleaseTargetReservation(
				request.KingdomID, request.TargetX, request.TargetY, request.OperationID,
			)
			return []string{"invasion"}, changed, nil
		},
	)
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func recordReconciledInvasionLaunch(
	gameState *State.GameState,
	reservation State.InvasionTargetReservation,
	movement State.MovementState,
	mapChanged bool,
) ([]string, bool, error) {
	result, err := Ingest.ReconcileInvasionReservationMovement(gameState, reservation, movement)
	if err != nil {
		return nil, false, err
	}
	domains := []string{"invasion", "map-invasion", "event-scores", "attack-analytics", "movements"}
	if result.ReportChanged {
		domains = append(domains, "reports")
	}
	return domains, mapChanged || result.Changed(), nil
}

func (application *Application) guardInvasionAttack(ctx context.Context, arguments json.RawMessage) error {
	var request resolvedInvasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	_, source, target, err := invasionAttackContext(Intent.PlanningContext{State: state}, mustMarshalInvasionAttackRequest(request.invasionAttackRequest))
	if err != nil {
		return err
	}
	score, found := state.ActiveScalableEventScore()
	if !found || score.EventID != request.EventID {
		return fmt.Errorf("invasion event %d is no longer active", request.EventID)
	}
	if score.PlayerScore >= request.ScoreTarget {
		return fmt.Errorf("invasion score target reached: %d / %d", score.PlayerScore, request.ScoreTarget)
	}
	if remaining := invasionRemainingSeconds(score, time.Now().UTC()); remaining >= 0 && remaining <= max(0, request.MinimumRemainingSec) {
		return fmt.Errorf("invasion event has only %d seconds remaining", remaining)
	}
	commanderReserved := State.InvasionCommanderReserved(state, request.CommanderID)
	if commanderReserved {
		reservation, ownReservation := state.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY)
		operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
		commanderReserved = !ownReservation || operationID == "" || reservation.OperationID != operationID ||
			!reservation.CommanderKnown || reservation.CommanderID != request.CommanderID
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available || State.CommanderHasActiveMovementAt(state, request.CommanderID, time.Now().UTC()) ||
		commanderReserved {
		return fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID)
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != target.TypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y {
		return fmt.Errorf("current attack dialog does not match invasion target %d:%d", target.X, target.Y)
	}
	return nil
}

func (application *Application) guardInvasionTarget(_ context.Context, arguments json.RawMessage) error {
	var verification invasionTargetVerificationRequest
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return err
	}
	request := verification.Request
	state := application.State.ReadOnlyView()
	_, _, target, err := invasionAttackContext(
		Intent.PlanningContext{State: state},
		mustMarshalInvasionAttackRequest(request.invasionAttackRequest),
	)
	if err != nil {
		return err
	}
	if verification.RefreshStartedAt.IsZero() || target.ObservedAt.IsZero() || target.ObservedAt.Before(verification.RefreshStartedAt) {
		// The 1x1 launch-time refresh is authoritative for this coordinate: the
		// castle is gone (defeated, or the slot reverted to a dynamic area).
		// Drop the phantom so the immediate re-evaluation rotates to the next
		// candidate instead of re-picking this one until the next full scan.
		application.forgetInvasionTarget(target)
		return fmt.Errorf(
			"%w: invasion target %d:%d was not returned by the launch-time map refresh",
			Intent.ErrPlanStale, target.X, target.Y,
		)
	}
	score, found := state.ActiveScalableEventScore()
	if !found || score.EventID != request.EventID {
		return fmt.Errorf("invasion event %d is no longer active", request.EventID)
	}
	if score.PlayerScore >= request.ScoreTarget {
		return fmt.Errorf("invasion score target reached: %d / %d", score.PlayerScore, request.ScoreTarget)
	}
	if remaining := invasionRemainingSeconds(score, time.Now().UTC()); remaining >= 0 && remaining <= max(0, request.MinimumRemainingSec) {
		return fmt.Errorf("invasion event has only %d seconds remaining", remaining)
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available || State.CommanderHasActiveMovementAt(state, request.CommanderID, time.Now().UTC()) ||
		State.InvasionCommanderReserved(state, request.CommanderID) {
		return fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID)
	}
	return nil
}

// forgetInvasionTarget removes a map observation that a launch-time refresh
// failed to confirm. Without this the candidate list keeps offering the
// vanished castle (it was observed after the last full scan) and the policy
// re-plans it back-to-back until the next full scan — a wire-speed loop of
// context refreshes and targeted map queries against the game.
func (application *Application) forgetInvasionTarget(target State.MapObservation) {
	if application == nil || application.State == nil {
		return
	}
	key := fmt.Sprintf("%d:%d", target.X, target.Y)
	_, _ = application.State.ApplyComponents(State.Components(State.ComponentWorldMap, State.ComponentInvasion), func(gameState *State.GameState) ([]string, bool, error) {
		current, exists := gameState.LookupMapObservation(target.KingdomID, key)
		mapChanged := false
		observationUnchanged := exists && current == target
		if observationUnchanged {
			mapChanged = gameState.DeleteMapObservation(target.KingdomID, key)
		}
		fortificationChanged := false
		if !exists || observationUnchanged {
			fortificationChanged = gameState.Invasion.ClearTargetFortification(target.KingdomID, target.X, target.Y)
		}
		if !mapChanged && !fortificationChanged {
			return nil, false, nil
		}
		domains := []string{"map"}
		if domain, retained := State.MapDomainForType(target.TypeID); retained {
			domains = append(domains, domain)
		}
		return append(domains, "invasion"), true, nil
	})
}

func (application *Application) guardInvasionFortify(_ context.Context, arguments json.RawMessage) error {
	var request invasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	request, _, target, err := invasionAttackContext(Intent.PlanningContext{State: state}, mustMarshalInvasionAttackRequest(request))
	if err != nil {
		return err
	}
	if request.FortifyCurrency == "" {
		return fmt.Errorf("invasion fortification currency is required")
	}
	if invasionTargetFortified(state, target) {
		return fmt.Errorf("invasion target %d:%d is already fortified", target.X, target.Y)
	}
	return nil
}

func invasionTargetFortified(gameState State.GameState, target State.MapObservation) bool {
	_, exists := gameState.Invasion.FortifiedTargets[fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)]
	return exists
}

func (application *Application) consumeInvasionTarget(ctx context.Context, arguments json.RawMessage) error {
	var request struct {
		KingdomID      State.KingdomID `json:"kingdomId"`
		TargetTypeID   int             `json:"targetTypeId"`
		TargetX        int             `json:"targetX"`
		TargetY        int             `json:"targetY"`
		TargetObjectID int64           `json:"targetObjectId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	if operationID == "" {
		return fmt.Errorf("invasion target consume requires an operation id")
	}
	_, err := application.State.ApplyComponents(
		State.Components(State.ComponentWorldMap, State.ComponentInvasion),
		func(gameState *State.GameState) ([]string, bool, error) {
			reservation, reserved := gameState.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY)
			if !reserved {
				// Exact movement reconciliation can release the reservation before
				// this post-CRA action runs. Its movement/pending receipt already
				// protects the route; without the original boundary, leave map state
				// untouched instead of risking deletion of a replacement target.
				return nil, false, nil
			}
			if reservation.OperationID != operationID || reservation.TargetTypeID != request.TargetTypeID {
				return nil, false, fmt.Errorf("%w: invasion target consume boundary changed", Intent.ErrPlanStale)
			}
			key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
			mapChanged := false
			current, exists := gameState.LookupMapObservation(request.KingdomID, key)
			matchesConsumedTarget := exists && request.TargetTypeID > 0 && current.TypeID == request.TargetTypeID &&
				(request.TargetObjectID <= 0 || current.ObjectID == request.TargetObjectID) &&
				!current.ObservedAt.After(reservation.ReservedAt)
			if matchesConsumedTarget &&
				(current.TypeID == State.MapTypeForeignLord || current.TypeID == State.MapTypeBloodcrow) {
				mapChanged = gameState.DeleteMapObservation(request.KingdomID, key)
			}
			fortified := false
			if !exists || matchesConsumedTarget {
				fortified = gameState.Invasion.ClearTargetFortification(request.KingdomID, request.TargetX, request.TargetY)
			}
			if !mapChanged && !fortified {
				return nil, false, nil
			}
			return []string{"map-invasion", "invasion"}, true, nil
		})
	return err
}

func mustMarshalInvasionAttackRequest(request invasionAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

func invasionMapTypeForEvent(eventID int64) (int, bool) {
	switch eventID {
	case 71:
		return 21, true
	case 103:
		return 34, true
	default:
		return 0, false
	}
}

func invasionRemainingSeconds(score State.ScalableEventScore, now time.Time) int64 {
	if score.RemainingSec <= 0 {
		return -1
	}
	elapsed := int64(0)
	if !score.ObservedAt.IsZero() && now.After(score.ObservedAt) {
		elapsed = int64(now.Sub(score.ObservedAt) / time.Second)
	}
	return max(0, score.RemainingSec-elapsed)
}
