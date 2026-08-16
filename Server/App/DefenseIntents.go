package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	defenseWallFlankToolSlotCount  = 4
	defenseWallMiddleToolSlotCount = 6
	defenseMoatToolSlotCount       = 1
	defenseKeepToolSlotCount       = 3
)

type defenseRefreshRequest struct {
	CastleID State.CastleID `json:"castleId"`
}

type defenseOpenGateRequest struct {
	CastleID              State.CastleID `json:"castleId"`
	RequireIncomingAttack bool           `json:"requireIncomingAttack,omitempty"`
	RequireProtectionMode bool           `json:"requireProtectionMode,omitempty"`
}

type defenseRefreshVerification struct {
	CastleID                    State.CastleID `json:"castleId"`
	PreviousDefenseObservedAt   time.Time      `json:"previousDefenseObservedAt"`
	PreviousInventoryObservedAt time.Time      `json:"previousInventoryObservedAt"`
}

type defenseKeepUpdateRequest struct {
	CastleID           State.CastleID          `json:"castleId"`
	MAUCT              int64                   `json:"mauct"`
	UnitTypePercent    int                     `json:"unitTypePercent"`
	PrimaryToolSlots   []State.DefenseToolSlot `json:"primaryToolSlots"`
	SecondaryToolSlots []State.DefenseToolSlot `json:"secondaryToolSlots"`
}

type defenseWallUpdateRequest struct {
	CastleID State.CastleID           `json:"castleId"`
	Left     State.DefenseWallSection `json:"left"`
	Middle   State.DefenseWallSection `json:"middle"`
	Right    State.DefenseWallSection `json:"right"`
}

type defenseWallResolvedRequest struct {
	defenseWallUpdateRequest
	PreviousDefenseObservedAt   time.Time `json:"previousDefenseObservedAt"`
	PreviousInventoryObservedAt time.Time `json:"previousInventoryObservedAt"`
}

type defenseMoatUpdateRequest struct {
	CastleID        State.CastleID          `json:"castleId"`
	LeftToolSlots   []State.DefenseToolSlot `json:"leftToolSlots"`
	MiddleToolSlots []State.DefenseToolSlot `json:"middleToolSlots"`
	RightToolSlots  []State.DefenseToolSlot `json:"rightToolSlots"`
}

type defenseMoatResolvedRequest struct {
	defenseMoatUpdateRequest
	PreviousDefenseObservedAt   time.Time `json:"previousDefenseObservedAt"`
	PreviousInventoryObservedAt time.Time `json:"previousInventoryObservedAt"`
}

type defenseKeepResolvedRequest struct {
	defenseKeepUpdateRequest
	PreviousDefenseObservedAt   time.Time `json:"previousDefenseObservedAt"`
	PreviousInventoryObservedAt time.Time `json:"previousInventoryObservedAt"`
}

func planDefenseRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request defenseRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	verification, _ := json.Marshal(defenseRefreshVerification{
		CastleID: request.CastleID, PreviousDefenseObservedAt: castle.Defense.ObservedAt,
		PreviousInventoryObservedAt: castle.Defense.InventoryObservedAt,
	})
	steps := defenseRefreshSteps(castle)
	steps = append(steps, Intent.Step{
		Name: "Verify defense refresh", Action: "defense.verify_refresh", ActionArguments: verification,
	})
	return Intent.Plan{
		Claims: defenseClaims(castle.ID), Summary: "Refresh defense setup for " + castleLabel(castle), Steps: steps,
	}, nil
}

func planDefenseOpenGate(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request defenseOpenGateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	now := time.Now().UTC()
	if request.RequireProtectionMode && !input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, fmt.Errorf("purchased Protection Mode is no longer preparing or active")
	}
	if request.RequireIncomingAttack {
		incoming := false
		input.State.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
			if movement.TargetCastleID == castle.ID && State.IsIncomingPlayerAttack(input.State, movement, now) {
				incoming = true
				return false
			}
			return true
		})
		if !incoming {
			return Intent.Plan{}, fmt.Errorf("castle %d no longer has an incoming player attack", castle.ID)
		}
	}
	if castle.Defense.OpenGateUntil != nil && castle.Defense.OpenGateUntil.After(now) {
		return Intent.Plan{}, fmt.Errorf("castle %d gates are already open until %s", castle.ID, castle.Defense.OpenGateUntil.UTC().Format(time.RFC3339))
	}
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
		Cooldown  int             `json:"CD"`
	}{castle.ID, castle.KingdomID, 0})
	id := strconv.FormatInt(int64(castle.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle:" + id, "defense:" + id, "account-resources"},
		Summary: "Open gates at " + castleLabel(castle),
		Steps:   []Intent.Step{commandStep("Open castle gates for six hours", "mos", payload, "mos")},
	}, nil
}

func planDefenseWallUpdate(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request defenseWallUpdateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, err := validateDefenseWallRequest(input, request, false, time.Time{}, time.Time{})
	if err != nil {
		return Intent.Plan{}, err
	}
	resolvedRequest := defenseWallResolvedRequest{
		defenseWallUpdateRequest:    request,
		PreviousDefenseObservedAt:   castle.Defense.ObservedAt,
		PreviousInventoryObservedAt: castle.Defense.InventoryObservedAt,
	}
	resolvedArguments, _ := json.Marshal(resolvedRequest)
	steps := defenseRefreshSteps(castle)
	steps = append(steps, Intent.Step{
		Name: "Apply defense wall setup", Resolver: "defense.wall.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfw", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	steps = append(steps, Intent.Step{
		Name: "Verify defense wall setup", Action: "defense.wall.verify", ActionArguments: resolvedArguments,
	})
	return Intent.Plan{
		Claims: defenseClaims(castle.ID), Summary: "Update defense wall setup for " + castleLabel(castle), Steps: steps,
	}, nil
}

func planDefenseMoatUpdate(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request defenseMoatUpdateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, err := validateDefenseMoatRequest(input, request, false, time.Time{}, time.Time{})
	if err != nil {
		return Intent.Plan{}, err
	}
	resolvedRequest := defenseMoatResolvedRequest{
		defenseMoatUpdateRequest:    request,
		PreviousDefenseObservedAt:   castle.Defense.ObservedAt,
		PreviousInventoryObservedAt: castle.Defense.InventoryObservedAt,
	}
	resolvedArguments, _ := json.Marshal(resolvedRequest)
	steps := defenseRefreshSteps(castle)
	steps = append(steps, Intent.Step{
		Name: "Apply defense moat setup", Resolver: "defense.moat.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfm", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	steps = append(steps, Intent.Step{
		Name: "Verify defense moat setup", Action: "defense.moat.verify", ActionArguments: resolvedArguments,
	})
	return Intent.Plan{
		Claims: defenseClaims(castle.ID), Summary: "Update defense moat setup for " + castleLabel(castle), Steps: steps,
	}, nil
}

func planDefenseKeepUpdate(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request defenseKeepUpdateRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, err := validateDefenseKeepRequest(input, request, false, time.Time{}, time.Time{})
	if err != nil {
		return Intent.Plan{}, err
	}
	resolvedRequest := defenseKeepResolvedRequest{
		defenseKeepUpdateRequest:    request,
		PreviousDefenseObservedAt:   castle.Defense.ObservedAt,
		PreviousInventoryObservedAt: castle.Defense.InventoryObservedAt,
	}
	resolvedArguments, _ := json.Marshal(resolvedRequest)
	steps := defenseRefreshSteps(castle)
	steps = append(steps, Intent.Step{
		Name: "Apply defense keep setup", Resolver: "defense.keep.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfk", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	steps = append(steps, Intent.Step{
		Name: "Verify defense keep setup", Action: "defense.keep.verify", ActionArguments: resolvedArguments,
	})
	return Intent.Plan{
		Claims: defenseClaims(castle.ID), Summary: "Update defense keep setup for " + castleLabel(castle), Steps: steps,
	}, nil
}

func resolveDefenseKeepStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request defenseKeepResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	castle, _, err := validateDefenseKeepRequest(
		input, request.defenseKeepUpdateRequest, true,
		request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt,
	)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, err := json.Marshal(struct {
		X                  int       `json:"CX"`
		Y                  int       `json:"CY"`
		CastleID           int64     `json:"AID"`
		MAUCT              int64     `json:"MAUCT"`
		UnitTypePercent    int       `json:"UC"`
		PrimaryToolSlots   [][]int64 `json:"S"`
		SecondaryToolSlots [][]int64 `json:"STS"`
	}{
		X: castle.X, Y: castle.Y, CastleID: int64(castle.ID), MAUCT: request.MAUCT,
		UnitTypePercent:    request.UnitTypePercent,
		PrimaryToolSlots:   defenseToolSlotRows(request.PrimaryToolSlots),
		SecondaryToolSlots: defenseToolSlotRows(request.SecondaryToolSlots),
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build DFK payload: %w", err)
	}
	return commandStep("Apply defense keep setup", "dfk", payload, "dfk"), nil
}

func resolveDefenseWallStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request defenseWallResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	castle, _, err := validateDefenseWallRequest(
		input, request.defenseWallUpdateRequest, true,
		request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt,
	)
	if err != nil {
		return Intent.Step{}, err
	}
	type wallSection struct {
		ToolSlots       [][]int64 `json:"S"`
		UnitPercent     int       `json:"UP"`
		UnitTypePercent int       `json:"UC"`
	}
	payload, err := json.Marshal(struct {
		X        int         `json:"CX"`
		Y        int         `json:"CY"`
		CastleID int64       `json:"AID"`
		Left     wallSection `json:"L"`
		Middle   wallSection `json:"M"`
		Right    wallSection `json:"R"`
	}{
		X: castle.X, Y: castle.Y, CastleID: int64(castle.ID),
		Left: wallSection{
			ToolSlots:   defenseWallFlankRows(request.Left.ToolSlots),
			UnitPercent: request.Left.UnitPercent, UnitTypePercent: request.Left.UnitTypePercent,
		},
		Middle: wallSection{
			ToolSlots:   defenseToolSlotRows(request.Middle.ToolSlots),
			UnitPercent: request.Middle.UnitPercent, UnitTypePercent: request.Middle.UnitTypePercent,
		},
		Right: wallSection{
			ToolSlots:   defenseWallFlankRows(request.Right.ToolSlots),
			UnitPercent: request.Right.UnitPercent, UnitTypePercent: request.Right.UnitTypePercent,
		},
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build DFW payload: %w", err)
	}
	return commandStep("Apply defense wall setup", "dfw", payload, "dfw"), nil
}

func resolveDefenseMoatStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request defenseMoatResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	castle, _, err := validateDefenseMoatRequest(
		input, request.defenseMoatUpdateRequest, true,
		request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt,
	)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, err := json.Marshal(struct {
		X               int       `json:"CX"`
		Y               int       `json:"CY"`
		CastleID        int64     `json:"AID"`
		LeftToolSlots   [][]int64 `json:"LS"`
		MiddleToolSlots [][]int64 `json:"MS"`
		RightToolSlots  [][]int64 `json:"RS"`
	}{
		X: castle.X, Y: castle.Y, CastleID: int64(castle.ID),
		LeftToolSlots:   defenseToolSlotRows(request.LeftToolSlots),
		MiddleToolSlots: defenseToolSlotRows(request.MiddleToolSlots),
		RightToolSlots:  defenseToolSlotRows(request.RightToolSlots),
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build DFM payload: %w", err)
	}
	return commandStep("Apply defense moat setup", "dfm", payload, "dfm"), nil
}

func (application *Application) verifyDefenseRefresh(_ context.Context, arguments json.RawMessage) error {
	var verification defenseRefreshVerification
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return err
	}
	castle, found := application.State.ReadOnlyView().Castles[verification.CastleID]
	if !found {
		return fmt.Errorf("castle %d is no longer in the current player state", verification.CastleID)
	}
	return verifyDefenseObservation(castle, verification.PreviousDefenseObservedAt, verification.PreviousInventoryObservedAt)
}

func (application *Application) verifyDefenseKeep(_ context.Context, arguments json.RawMessage) error {
	var request defenseKeepResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	castle, found := application.State.ReadOnlyView().Castles[request.CastleID]
	if !found {
		return fmt.Errorf("castle %d is no longer in the current player state", request.CastleID)
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	keep := castle.Defense.Keep
	if keep.MAUCT != request.MAUCT || keep.UnitTypePercent != request.UnitTypePercent ||
		!reflect.DeepEqual(keep.PrimaryToolSlots, request.PrimaryToolSlots) ||
		!reflect.DeepEqual(keep.SecondaryToolSlots, request.SecondaryToolSlots) {
		return fmt.Errorf("castle %d defense keep setup did not match the requested DFK values", request.CastleID)
	}
	return nil
}

func (application *Application) verifyDefenseWall(_ context.Context, arguments json.RawMessage) error {
	var request defenseWallResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	castle, found := application.State.ReadOnlyView().Castles[request.CastleID]
	if !found {
		return fmt.Errorf("castle %d is no longer in the current player state", request.CastleID)
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	wall := castle.Defense.Wall
	if !reflect.DeepEqual(wall.Left, request.Left) || !reflect.DeepEqual(wall.Middle, request.Middle) ||
		!reflect.DeepEqual(wall.Right, request.Right) {
		return fmt.Errorf("castle %d defense wall setup did not match the requested DFW values", request.CastleID)
	}
	return nil
}

func (application *Application) verifyDefenseMoat(_ context.Context, arguments json.RawMessage) error {
	var request defenseMoatResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	castle, found := application.State.ReadOnlyView().Castles[request.CastleID]
	if !found {
		return fmt.Errorf("castle %d is no longer in the current player state", request.CastleID)
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	moat := castle.Defense.Moat
	if !reflect.DeepEqual(moat.LeftToolSlots, request.LeftToolSlots) ||
		!reflect.DeepEqual(moat.MiddleToolSlots, request.MiddleToolSlots) ||
		!reflect.DeepEqual(moat.RightToolSlots, request.RightToolSlots) {
		return fmt.Errorf("castle %d defense moat setup did not match the requested DFM values", request.CastleID)
	}
	return nil
}

func verifyDefenseObservation(castle State.CastleState, previousDefense, previousInventory time.Time) error {
	if castle.Defense.ObservedAt.IsZero() || !castle.Defense.ObservedAt.After(previousDefense) {
		return fmt.Errorf("castle %d did not return a fresh DFC defense snapshot", castle.ID)
	}
	if castle.Defense.InventoryObservedAt.IsZero() || !castle.Defense.InventoryObservedAt.After(previousInventory) {
		return fmt.Errorf("castle %d did not return a fresh DFC defense inventory", castle.ID)
	}
	return nil
}

func defenseRefreshSteps(castle State.CastleState) []Intent.Step {
	return []Intent.Step{castleFocusStep(castle), defenseContextStep(castle)}
}

func defenseContextStep(castle State.CastleState) Intent.Step {
	payload, _ := json.Marshal(struct {
		X         int   `json:"CX"`
		Y         int   `json:"CY"`
		CastleID  int64 `json:"AID"`
		KingdomID int64 `json:"KID"`
	}{castle.X, castle.Y, int64(castle.ID), -1})
	return contextCommandStep("Refresh castle defense", "dfc", payload, "dfc")
}

func defenseCastle(input Intent.PlanningContext, castleID State.CastleID) (State.CastleState, error) {
	castle, found := input.State.Castles[castleID]
	if castleID <= 0 || !found {
		return State.CastleState{}, fmt.Errorf("castle %d is not in the current player state", castleID)
	}
	if castle.KingdomID != 0 {
		return State.CastleState{}, fmt.Errorf("defense interaction is only capture-confirmed for primary-kingdom castles")
	}
	return castle, nil
}

func validateDefenseKeepRequest(
	input Intent.PlanningContext,
	request defenseKeepUpdateRequest,
	requireFresh bool,
	previousDefense time.Time,
	previousInventory time.Time,
) (State.CastleState, map[State.UnitID]int64, error) {
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return State.CastleState{}, nil, err
	}
	if request.MAUCT < 0 {
		return State.CastleState{}, nil, fmt.Errorf("mauct must not be negative")
	}
	if request.UnitTypePercent < 0 || request.UnitTypePercent > 100 {
		return State.CastleState{}, nil, fmt.Errorf("unitTypePercent must be between 0 and 100")
	}
	if castle.Defense.ObservedAt.IsZero() {
		return State.CastleState{}, nil, fmt.Errorf("castle %d defense setup has not been observed; run defense.refresh first", castle.ID)
	}
	if err := validateDefenseKeepSlotCounts(request.PrimaryToolSlots, request.SecondaryToolSlots); err != nil {
		return State.CastleState{}, nil, err
	}
	primaryRequired, err := validateDefenseToolSlotsForTypes(
		input.GameData, map[int]bool{5: true}, request.PrimaryToolSlots,
	)
	if err != nil {
		return State.CastleState{}, nil, fmt.Errorf("keep tool slots: %w", err)
	}
	secondaryRequired, err := validateDefenseToolSlotsForTypes(
		input.GameData, map[int]bool{6: true}, request.SecondaryToolSlots,
	)
	if err != nil {
		return State.CastleState{}, nil, fmt.Errorf("Sceat support tool slots: %w", err)
	}
	required, err := combineDefenseToolRequirements(primaryRequired, secondaryRequired)
	if err != nil {
		return State.CastleState{}, nil, err
	}
	if !requireFresh {
		return castle, required, nil
	}
	if err := verifyDefenseObservation(castle, previousDefense, previousInventory); err != nil {
		return State.CastleState{}, nil, err
	}
	if err := validateDefenseToolAvailability(
		castle, required,
		castle.Defense.Keep.PrimaryToolSlots,
		castle.Defense.Keep.SecondaryToolSlots,
	); err != nil {
		return State.CastleState{}, nil, err
	}
	return castle, required, nil
}

func validateDefenseKeepSlotCounts(
	primary []State.DefenseToolSlot,
	secondary []State.DefenseToolSlot,
) error {
	if len(primary) != defenseKeepToolSlotCount {
		return fmt.Errorf("primaryToolSlots must contain exactly %d keep tool slots", defenseKeepToolSlotCount)
	}
	if len(secondary) != defenseKeepToolSlotCount {
		return fmt.Errorf("secondaryToolSlots must contain exactly %d Sceat support tool slots", defenseKeepToolSlotCount)
	}
	return nil
}

func validateDefenseWallRequest(
	input Intent.PlanningContext,
	request defenseWallUpdateRequest,
	requireFresh bool,
	previousDefense time.Time,
	previousInventory time.Time,
) (State.CastleState, map[State.UnitID]int64, error) {
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return State.CastleState{}, nil, err
	}
	if castle.Defense.ObservedAt.IsZero() {
		return State.CastleState{}, nil, fmt.Errorf("castle %d defense setup has not been observed; run defense.refresh first", castle.ID)
	}
	if err := validateDefenseWallSlotCounts(request.Left.ToolSlots, request.Middle.ToolSlots, request.Right.ToolSlots); err != nil {
		return State.CastleState{}, nil, err
	}
	sections := []struct {
		name     string
		request  State.DefenseWallSection
		observed State.DefenseWallSection
	}{
		{name: "left", request: request.Left, observed: castle.Defense.Wall.Left},
		{name: "middle", request: request.Middle, observed: castle.Defense.Wall.Middle},
		{name: "right", request: request.Right, observed: castle.Defense.Wall.Right},
	}
	for _, section := range sections {
		if len(section.request.ToolSlots) != len(section.observed.ToolSlots) {
			return State.CastleState{}, nil, fmt.Errorf(
				"%s.toolSlots must contain exactly %d slots from the current DFC snapshot",
				section.name, len(section.observed.ToolSlots),
			)
		}
		if section.request.UnitPercent < 0 || section.request.UnitPercent > 100 {
			return State.CastleState{}, nil, fmt.Errorf("%s.unitPercent must be between 0 and 100", section.name)
		}
		if section.request.UnitTypePercent < 0 || section.request.UnitTypePercent > 100 {
			return State.CastleState{}, nil, fmt.Errorf("%s.unitTypePercent must be between 0 and 100", section.name)
		}
	}
	if request.Left.UnitPercent+request.Middle.UnitPercent+request.Right.UnitPercent != 100 {
		return State.CastleState{}, nil, fmt.Errorf("left, middle, and right unitPercent values must total 100")
	}
	required, err := validateDefenseWallToolSlots(input.GameData, request.Left, request.Middle, request.Right)
	if err != nil {
		return State.CastleState{}, nil, err
	}
	if !requireFresh {
		return castle, required, nil
	}
	if err := verifyDefenseObservation(castle, previousDefense, previousInventory); err != nil {
		return State.CastleState{}, nil, err
	}
	if err := validateDefenseToolAvailability(
		castle, required,
		castle.Defense.Wall.Left.ToolSlots,
		castle.Defense.Wall.Middle.ToolSlots,
		castle.Defense.Wall.Right.ToolSlots,
	); err != nil {
		return State.CastleState{}, nil, err
	}
	return castle, required, nil
}

func validateDefenseWallSlotCounts(
	left []State.DefenseToolSlot,
	middle []State.DefenseToolSlot,
	right []State.DefenseToolSlot,
) error {
	if len(left) != defenseWallFlankToolSlotCount {
		return fmt.Errorf("left.toolSlots must contain exactly %d wall slots", defenseWallFlankToolSlotCount)
	}
	if len(middle) != defenseWallMiddleToolSlotCount {
		return fmt.Errorf("middle.toolSlots must contain exactly %d ordered slots: four wall and two gate", defenseWallMiddleToolSlotCount)
	}
	if len(right) != defenseWallFlankToolSlotCount {
		return fmt.Errorf("right.toolSlots must contain exactly %d wall slots", defenseWallFlankToolSlotCount)
	}
	return nil
}

func validateDefenseMoatSlotCounts(
	left []State.DefenseToolSlot,
	middle []State.DefenseToolSlot,
	right []State.DefenseToolSlot,
) error {
	if len(left) != defenseMoatToolSlotCount {
		return fmt.Errorf("leftToolSlots must contain exactly one moat slot")
	}
	if len(middle) != defenseMoatToolSlotCount {
		return fmt.Errorf("middleToolSlots must contain exactly one moat slot")
	}
	if len(right) != defenseMoatToolSlotCount {
		return fmt.Errorf("rightToolSlots must contain exactly one moat slot")
	}
	return nil
}

func validateDefenseWallToolSlots(
	gameData *GameData.Store,
	left State.DefenseWallSection,
	middle State.DefenseWallSection,
	right State.DefenseWallSection,
) (map[State.UnitID]int64, error) {
	middleWallSlots := make([]State.DefenseToolSlot, 0, 4)
	middleGateSlots := make([]State.DefenseToolSlot, 0, 2)
	for index, slot := range middle.ToolSlots {
		if isDefenseWallMiddleGateSlot(index) {
			middleGateSlots = append(middleGateSlots, slot)
		} else {
			middleWallSlots = append(middleWallSlots, slot)
		}
	}
	wallRequired, err := validateDefenseToolSlotsForTypes(
		gameData, map[int]bool{1: true},
		left.ToolSlots, middleWallSlots, right.ToolSlots,
	)
	if err != nil {
		return nil, fmt.Errorf("wall tool slots: %w", err)
	}
	gateRequired, err := validateDefenseToolSlotsForTypes(
		gameData, map[int]bool{2: true},
		middleGateSlots,
	)
	if err != nil {
		return nil, fmt.Errorf("middle gate tool slots: %w", err)
	}
	return combineDefenseToolRequirements(wallRequired, gateRequired)
}

// DFW M.S is positional: zero-based indexes 1 and 4 are the two gate slots.
func isDefenseWallMiddleGateSlot(index int) bool {
	return index == 1 || index == 4
}

func validateDefenseMoatRequest(
	input Intent.PlanningContext,
	request defenseMoatUpdateRequest,
	requireFresh bool,
	previousDefense time.Time,
	previousInventory time.Time,
) (State.CastleState, map[State.UnitID]int64, error) {
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return State.CastleState{}, nil, err
	}
	if castle.Defense.ObservedAt.IsZero() {
		return State.CastleState{}, nil, fmt.Errorf("castle %d defense setup has not been observed; run defense.refresh first", castle.ID)
	}
	if err := validateDefenseMoatSlotCounts(
		request.LeftToolSlots,
		request.MiddleToolSlots,
		request.RightToolSlots,
	); err != nil {
		return State.CastleState{}, nil, err
	}
	groups := []struct {
		name     string
		request  []State.DefenseToolSlot
		observed []State.DefenseToolSlot
	}{
		{name: "leftToolSlots", request: request.LeftToolSlots, observed: castle.Defense.Moat.LeftToolSlots},
		{name: "middleToolSlots", request: request.MiddleToolSlots, observed: castle.Defense.Moat.MiddleToolSlots},
		{name: "rightToolSlots", request: request.RightToolSlots, observed: castle.Defense.Moat.RightToolSlots},
	}
	for _, group := range groups {
		if len(group.request) != len(group.observed) {
			return State.CastleState{}, nil, fmt.Errorf(
				"%s must contain exactly %d slots from the current DFC snapshot", group.name, len(group.observed),
			)
		}
	}
	required, err := validateDefenseToolSlotsForTypes(
		input.GameData, map[int]bool{4: true},
		request.LeftToolSlots, request.MiddleToolSlots, request.RightToolSlots,
	)
	if err != nil {
		return State.CastleState{}, nil, err
	}
	if !requireFresh {
		return castle, required, nil
	}
	if err := verifyDefenseObservation(castle, previousDefense, previousInventory); err != nil {
		return State.CastleState{}, nil, err
	}
	if err := validateDefenseToolAvailability(
		castle, required,
		castle.Defense.Moat.LeftToolSlots,
		castle.Defense.Moat.MiddleToolSlots,
		castle.Defense.Moat.RightToolSlots,
	); err != nil {
		return State.CastleState{}, nil, err
	}
	return castle, required, nil
}

func validateDefenseToolSlots(
	gameData *GameData.Store,
	groups ...[]State.DefenseToolSlot,
) (map[State.UnitID]int64, error) {
	return validateDefenseToolSlotsForTypes(gameData, nil, groups...)
}

func validateDefenseToolSlotsForTypes(
	gameData *GameData.Store,
	allowedSlotTypes map[int]bool,
	groups ...[]State.DefenseToolSlot,
) (map[State.UnitID]int64, error) {
	required := map[State.UnitID]int64{}
	var catalog *GameData.Catalog
	for _, slots := range groups {
		for _, slot := range slots {
			if slot.DefinitionID == -1 {
				if slot.Amount != 0 {
					return nil, fmt.Errorf("empty defense tool slots must use definitionId -1 and amount 0")
				}
				continue
			}
			if slot.DefinitionID <= 0 || slot.Amount <= 0 {
				return nil, fmt.Errorf("nonempty defense tool slots require a positive definitionId and amount")
			}
			if slot.Amount > 999 {
				return nil, fmt.Errorf("defense tool slot amounts must not exceed 999")
			}
			if gameData == nil {
				return nil, fmt.Errorf("official game data is unavailable")
			}
			if catalog == nil {
				var err error
				catalog, err = gameData.Catalog("units")
				if err != nil {
					return nil, err
				}
			}
			raw, found := catalog.Find(strconv.FormatInt(int64(slot.DefinitionID), 10))
			if !found {
				return nil, fmt.Errorf("defense tool %d is not in the official units catalog", slot.DefinitionID)
			}
			record, err := GameData.DecodeRecord(raw)
			if err != nil {
				return nil, fmt.Errorf("decode defense tool %d: %w", slot.DefinitionID, err)
			}
			if !GameData.IsToolRecord(record) || !isDefenseToolRecord(record) {
				return nil, fmt.Errorf("units definition %d is not a defense tool", slot.DefinitionID)
			}
			if len(allowedSlotTypes) > 0 && !defenseToolSupportsAnySlotType(record, allowedSlotTypes) {
				return nil, fmt.Errorf("defense tool %d is not valid for this defense section", slot.DefinitionID)
			}
			if required[slot.DefinitionID] > math.MaxInt64-slot.Amount {
				return nil, fmt.Errorf("defense tool %d amount exceeds the supported range", slot.DefinitionID)
			}
			required[slot.DefinitionID] += slot.Amount
		}
	}
	return required, nil
}

func isDefenseToolRecord(record GameData.Record) bool {
	typ, ok := record.String("typ")
	if !ok {
		return false
	}
	typ = strings.TrimSpace(typ)
	return strings.EqualFold(typ, "defence") || strings.EqualFold(typ, "defense")
}

func validateDefenseSlotRows(groups ...[]State.DefenseToolSlot) error {
	for _, slots := range groups {
		for _, slot := range slots {
			if slot.DefinitionID == -1 && slot.Amount == 0 {
				continue
			}
			if slot.DefinitionID <= 0 || slot.Amount <= 0 {
				return fmt.Errorf("slot rows must use either [-1,0] or two positive values")
			}
		}
	}
	return nil
}

func defenseToolSupportsAnySlotType(record GameData.Record, allowed map[int]bool) bool {
	if textValue, ok := record.String("slotTypes"); ok {
		for _, value := range strings.Split(textValue, ",") {
			slotType, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil && allowed[slotType] {
				return true
			}
		}
		return false
	}
	var rawValues []json.RawMessage
	if json.Unmarshal(record["slotTypes"], &rawValues) != nil {
		return false
	}
	for _, raw := range rawValues {
		var numeric int
		if json.Unmarshal(raw, &numeric) == nil && allowed[numeric] {
			return true
		}
		var textValue string
		if json.Unmarshal(raw, &textValue) == nil {
			slotType, err := strconv.Atoi(strings.TrimSpace(textValue))
			if err == nil && allowed[slotType] {
				return true
			}
		}
	}
	return false
}

func validateDefenseToolAvailability(
	castle State.CastleState,
	required map[State.UnitID]int64,
	releasedGroups ...[]State.DefenseToolSlot,
) error {
	released := map[State.UnitID]int64{}
	for _, slots := range releasedGroups {
		for _, slot := range slots {
			if slot.DefinitionID <= 0 || slot.Amount <= 0 {
				continue
			}
			if released[slot.DefinitionID] > math.MaxInt64-slot.Amount {
				return fmt.Errorf("currently assigned defense tool %d amount exceeds the supported range", slot.DefinitionID)
			}
			released[slot.DefinitionID] += slot.Amount
		}
	}
	for definitionID, amount := range required {
		available := castle.Defense.Inventory[definitionID]
		if available > math.MaxInt64-released[definitionID] {
			return fmt.Errorf("defense tool %d availability exceeds the supported range", definitionID)
		}
		available += released[definitionID]
		if available < amount {
			return fmt.Errorf(
				"defense tool %d requires %d but castle %d has %d available after releasing the current section setup",
				definitionID, amount, castle.ID, available,
			)
		}
	}
	return nil
}

func defenseToolSlotRows(slots []State.DefenseToolSlot) [][]int64 {
	rows := make([][]int64, 0, len(slots))
	for _, slot := range slots {
		rows = append(rows, []int64{int64(slot.DefinitionID), slot.Amount})
	}
	return rows
}

func defenseWallFlankRows(slots []State.DefenseToolSlot) [][]int64 {
	rows := defenseToolSlotRows(slots)
	return append(rows, []int64{-1, 0})
}

func defenseClaims(castleID State.CastleID) []string {
	id := strconv.FormatInt(int64(castleID), 10)
	return []string{"castle-focus", "castle:" + id, "defense:" + id}
}
