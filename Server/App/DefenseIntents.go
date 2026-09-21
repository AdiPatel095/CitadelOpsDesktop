package App

import (
	"CitadelDesktop/Server/Localization"
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
		Name: "Verify defense refresh", NameDescriptor: Localization.New("server.app.verify_defense_refresh.6077463d", "Verify defense refresh", nil), Action: "defense.verify_refresh", ActionArguments: verification,
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("purchased Protection Mode is no longer preparing or active"), Localization.New("server.app.purchased_protection_mode_is.13cd064a", "purchased Protection Mode is no longer preparing or active", nil))
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
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d no longer has an incoming player attack", castle.ID), Localization.New("server.app.castle_p_no_longer.ae857144", "castle {p0} no longer has an incoming player attack", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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
		Steps:   []Intent.Step{commandStep("Open castle gates for six hours", "mos", payload, "mos", Localization.New("server.app.open_castle_gates_for.6b2b75cf", "Open castle gates for six hours", nil))},
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
		Name: "Apply defense wall setup", NameDescriptor: Localization.New("server.app.apply_defense_wall_setup.15291878", "Apply defense wall setup", nil), Resolver: "defense.wall.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfw", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	steps = append(steps, Intent.Step{
		Name: "Verify defense wall setup", NameDescriptor: Localization.New("server.app.verify_defense_wall_setup.6d508b85", "Verify defense wall setup", nil), Action: "defense.wall.verify", ActionArguments: resolvedArguments,
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
		Name: "Apply defense moat setup", NameDescriptor: Localization.New("server.app.apply_defense_moat_setup.70165552", "Apply defense moat setup", nil), Resolver: "defense.moat.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfm", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	steps = append(steps, Intent.Step{
		Name: "Verify defense moat setup", NameDescriptor: Localization.New("server.app.verify_defense_moat_setup.9400d0d5", "Verify defense moat setup", nil), Action: "defense.moat.verify", ActionArguments: resolvedArguments,
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
		Name: "Apply defense keep setup", NameDescriptor: Localization.New("server.app.apply_defense_keep_setup.c61ca521", "Apply defense keep setup", nil), Resolver: "defense.keep.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfk", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	steps = append(steps, Intent.Step{
		Name: "Verify defense keep setup", NameDescriptor: Localization.New("server.app.verify_defense_keep_setup.178b08dd", "Verify defense keep setup", nil), Action: "defense.keep.verify", ActionArguments: resolvedArguments,
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build DFK payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_dfk_payload.5192bb04", "build DFK payload", nil), err))
	}
	return commandStep("Apply defense keep setup", "dfk", payload, "dfk", Localization.New("server.app.apply_defense_keep_setup.c61ca521", "Apply defense keep setup", nil)), nil
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build DFW payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_dfw_payload.da6d49a5", "build DFW payload", nil), err))
	}
	return commandStep("Apply defense wall setup", "dfw", payload, "dfw", Localization.New("server.app.apply_defense_wall_setup.15291878", "Apply defense wall setup", nil)), nil
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build DFM payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_dfm_payload.9f7da765", "build DFM payload", nil), err))
	}
	return commandStep("Apply defense moat setup", "dfm", payload, "dfm", Localization.New("server.app.apply_defense_moat_setup.70165552", "Apply defense moat setup", nil)), nil
}

func (application *Application) verifyDefenseRefresh(_ context.Context, arguments json.RawMessage) error {
	var verification defenseRefreshVerification
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return err
	}
	castle, found := application.State.ReadOnlyView().Castles[verification.CastleID]
	if !found {
		return Localization.WithError(fmt.Errorf("castle %d is no longer in the current player state", verification.CastleID), Localization.New("server.app.castle_p_is_no.eb2a23ef", "castle {p0} is no longer in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", verification.CastleID)}))
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
		return Localization.WithError(fmt.Errorf("castle %d is no longer in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_no.eb2a23ef", "castle {p0} is no longer in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	keep := castle.Defense.Keep
	if keep.MAUCT != request.MAUCT || keep.UnitTypePercent != request.UnitTypePercent ||
		!reflect.DeepEqual(keep.PrimaryToolSlots, request.PrimaryToolSlots) ||
		!reflect.DeepEqual(keep.SecondaryToolSlots, request.SecondaryToolSlots) {
		return Localization.WithError(fmt.Errorf("castle %d defense keep setup did not match the requested DFK values", request.CastleID), Localization.New("server.app.castle_p_defense_keep.4a90fc43", "castle {p0} defense keep setup did not match the requested DFK values", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
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
		return Localization.WithError(fmt.Errorf("castle %d is no longer in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_no.eb2a23ef", "castle {p0} is no longer in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	wall := castle.Defense.Wall
	if !reflect.DeepEqual(wall.Left, request.Left) || !reflect.DeepEqual(wall.Middle, request.Middle) ||
		!reflect.DeepEqual(wall.Right, request.Right) {
		return Localization.WithError(fmt.Errorf("castle %d defense wall setup did not match the requested DFW values", request.CastleID), Localization.New("server.app.castle_p_defense_wall.28a1d091", "castle {p0} defense wall setup did not match the requested DFW values", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
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
		return Localization.WithError(fmt.Errorf("castle %d is no longer in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_no.eb2a23ef", "castle {p0} is no longer in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	moat := castle.Defense.Moat
	if !reflect.DeepEqual(moat.LeftToolSlots, request.LeftToolSlots) ||
		!reflect.DeepEqual(moat.MiddleToolSlots, request.MiddleToolSlots) ||
		!reflect.DeepEqual(moat.RightToolSlots, request.RightToolSlots) {
		return Localization.WithError(fmt.Errorf("castle %d defense moat setup did not match the requested DFM values", request.CastleID), Localization.New("server.app.castle_p_defense_moat.98f2f86a", "castle {p0} defense moat setup did not match the requested DFM values", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	return nil
}

func verifyDefenseObservation(castle State.CastleState, previousDefense, previousInventory time.Time) error {
	if castle.Defense.ObservedAt.IsZero() || !castle.Defense.ObservedAt.After(previousDefense) {
		return Localization.WithError(fmt.Errorf("castle %d did not return a fresh DFC defense snapshot", castle.ID), Localization.New("server.app.castle_p_did_not.a3e416e8", "castle {p0} did not return a fresh DFC defense snapshot", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	if castle.Defense.InventoryObservedAt.IsZero() || !castle.Defense.InventoryObservedAt.After(previousInventory) {
		return Localization.WithError(fmt.Errorf("castle %d did not return a fresh DFC defense inventory", castle.ID), Localization.New("server.app.castle_p_did_not.c17962ac", "castle {p0} did not return a fresh DFC defense inventory", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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
		return State.CastleState{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", castleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", castleID)}))
	}
	if castle.KingdomID != 0 {
		return State.CastleState{}, Localization.WithError(fmt.Errorf("defense interaction is only capture-confirmed for primary-kingdom castles"), Localization.New("server.app.defense_interaction_is_only.85f0cb5c", "defense interaction is only capture-confirmed for primary-kingdom castles", nil))
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
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("mauct must not be negative"), Localization.New("server.app.mauct_must_not_be.89267798", "mauct must not be negative", nil))
	}
	if request.UnitTypePercent < 0 || request.UnitTypePercent > 100 {
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("unitTypePercent must be between 0 and 100"), Localization.New("server.app.unittypepercent_must_be_between.aa1c2694", "unitTypePercent must be between 0 and 100", nil))
	}
	if castle.Defense.ObservedAt.IsZero() {
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("castle %d defense setup has not been observed; run defense.refresh first", castle.ID), Localization.New("server.app.castle_p_defense_setup.3b044a4b", "castle {p0} defense setup has not been observed; run defense.refresh first", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	if err := validateDefenseKeepSlotCounts(request.PrimaryToolSlots, request.SecondaryToolSlots); err != nil {
		return State.CastleState{}, nil, err
	}
	primaryRequired, err := validateDefenseToolSlotsForTypes(
		input.GameData, map[int]bool{5: true}, request.PrimaryToolSlots,
	)
	if err != nil {
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("keep tool slots: %w", err), Localization.ErrorContext(Localization.New("server.app.keep_tool_slots.100ae599", "keep tool slots", nil), err))
	}
	secondaryRequired, err := validateDefenseToolSlotsForTypes(
		input.GameData, map[int]bool{6: true}, request.SecondaryToolSlots,
	)
	if err != nil {
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Sceat support tool slots: %w", err), Localization.ErrorContext(Localization.New("server.app.sceat_support_tool_slots.6cf39569", "Sceat support tool slots", nil), err))
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
		return Localization.WithError(fmt.Errorf("primaryToolSlots must contain exactly %d keep tool slots", defenseKeepToolSlotCount), Localization.New("server.app.primarytoolslots_must_contain_exactly.f2a5cd27", "primaryToolSlots must contain exactly {p0} keep tool slots", Localization.Params{"p0": defenseKeepToolSlotCount}))
	}
	if len(secondary) != defenseKeepToolSlotCount {
		return Localization.WithError(fmt.Errorf("secondaryToolSlots must contain exactly %d Sceat support tool slots", defenseKeepToolSlotCount), Localization.New("server.app.secondarytoolslots_must_contain_exactly.c3cfe548", "secondaryToolSlots must contain exactly {p0} Sceat support tool slots", Localization.Params{"p0": defenseKeepToolSlotCount}))
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
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("castle %d defense setup has not been observed; run defense.refresh first", castle.ID), Localization.New("server.app.castle_p_defense_setup.3b044a4b", "castle {p0} defense setup has not been observed; run defense.refresh first", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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
			return State.CastleState{}, nil, Localization.WithError(fmt.Errorf(
				"%s.toolSlots must contain exactly %d slots from the current DFC snapshot",
				section.name, len(section.observed.ToolSlots),
			), Localization.New("server.app.p_toolslots_must_contain.a171f61a", "{p0}.toolSlots must contain exactly {p1} slots from the current DFC snapshot", Localization.Params{"p0": fmt.Sprintf("%s", section.name), "p1": len(section.observed.ToolSlots)}))
		}
		if section.request.UnitPercent < 0 || section.request.UnitPercent > 100 {
			return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("%s.unitPercent must be between 0 and 100", section.name), Localization.New("server.app.p_unitpercent_must_be.7f4c584f", "{p0}.unitPercent must be between 0 and 100", Localization.Params{"p0": fmt.Sprintf("%s", section.name)}))
		}
		if section.request.UnitTypePercent < 0 || section.request.UnitTypePercent > 100 {
			return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("%s.unitTypePercent must be between 0 and 100", section.name), Localization.New("server.app.p_unittypepercent_must_be.710cc7ed", "{p0}.unitTypePercent must be between 0 and 100", Localization.Params{"p0": fmt.Sprintf("%s", section.name)}))
		}
	}
	if request.Left.UnitPercent+request.Middle.UnitPercent+request.Right.UnitPercent != 100 {
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("left, middle, and right unitPercent values must total 100"), Localization.New("server.app.left_middle_and_right.d125af07", "left, middle, and right unitPercent values must total 100", nil))
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
		return Localization.WithError(fmt.Errorf("left.toolSlots must contain exactly %d wall slots", defenseWallFlankToolSlotCount), Localization.New("server.app.left_toolslots_must_contain.02918734", "left.toolSlots must contain exactly {p0} wall slots", Localization.Params{"p0": defenseWallFlankToolSlotCount}))
	}
	if len(middle) != defenseWallMiddleToolSlotCount {
		return Localization.WithError(fmt.Errorf("middle.toolSlots must contain exactly %d ordered slots: four wall and two gate", defenseWallMiddleToolSlotCount), Localization.New("server.app.middle_toolslots_must_contain.03024d65", "middle.toolSlots must contain exactly {p0} ordered slots: four wall and two gate", Localization.Params{"p0": fmt.Sprintf("%d", defenseWallMiddleToolSlotCount)}))
	}
	if len(right) != defenseWallFlankToolSlotCount {
		return Localization.WithError(fmt.Errorf("right.toolSlots must contain exactly %d wall slots", defenseWallFlankToolSlotCount), Localization.New("server.app.right_toolslots_must_contain.5651ae8a", "right.toolSlots must contain exactly {p0} wall slots", Localization.Params{"p0": defenseWallFlankToolSlotCount}))
	}
	return nil
}

func validateDefenseMoatSlotCounts(
	left []State.DefenseToolSlot,
	middle []State.DefenseToolSlot,
	right []State.DefenseToolSlot,
) error {
	if len(left) != defenseMoatToolSlotCount {
		return Localization.WithError(fmt.Errorf("leftToolSlots must contain exactly one moat slot"), Localization.New("server.app.lefttoolslots_must_contain_exactly.acaa6cc3", "leftToolSlots must contain exactly one moat slot", nil))
	}
	if len(middle) != defenseMoatToolSlotCount {
		return Localization.WithError(fmt.Errorf("middleToolSlots must contain exactly one moat slot"), Localization.New("server.app.middletoolslots_must_contain_exactly.970193ba", "middleToolSlots must contain exactly one moat slot", nil))
	}
	if len(right) != defenseMoatToolSlotCount {
		return Localization.WithError(fmt.Errorf("rightToolSlots must contain exactly one moat slot"), Localization.New("server.app.righttoolslots_must_contain_exactly.ba1faba4", "rightToolSlots must contain exactly one moat slot", nil))
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
		return nil, Localization.WithError(fmt.Errorf("wall tool slots: %w", err), Localization.ErrorContext(Localization.New("server.app.wall_tool_slots.ead26aeb", "wall tool slots", nil), err))
	}
	gateRequired, err := validateDefenseToolSlotsForTypes(
		gameData, map[int]bool{2: true},
		middleGateSlots,
	)
	if err != nil {
		return nil, Localization.WithError(fmt.Errorf("middle gate tool slots: %w", err), Localization.ErrorContext(Localization.New("server.app.middle_gate_tool_slots.93d1ea63", "middle gate tool slots", nil), err))
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
		return State.CastleState{}, nil, Localization.WithError(fmt.Errorf("castle %d defense setup has not been observed; run defense.refresh first", castle.ID), Localization.New("server.app.castle_p_defense_setup.3b044a4b", "castle {p0} defense setup has not been observed; run defense.refresh first", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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
			return State.CastleState{}, nil, Localization.WithError(fmt.Errorf(
				"%s must contain exactly %d slots from the current DFC snapshot", group.name, len(group.observed),
			), Localization.New("server.app.p_must_contain_exactly.ae91226a", "{p0} must contain exactly {p1} slots from the current DFC snapshot", Localization.Params{"p0": fmt.Sprintf("%s", group.name), "p1": len(group.observed)}))
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
					return nil, Localization.WithError(fmt.Errorf("empty defense tool slots must use definitionId -1 and amount 0"), Localization.New("server.app.empty_defense_tool_slots.cc7164d2", "empty defense tool slots must use definitionId -1 and amount 0", nil))
				}
				continue
			}
			if slot.DefinitionID <= 0 || slot.Amount <= 0 {
				return nil, Localization.WithError(fmt.Errorf("nonempty defense tool slots require a positive definitionId and amount"), Localization.New("server.app.nonempty_defense_tool_slots.a70330c7", "nonempty defense tool slots require a positive definitionId and amount", nil))
			}
			if slot.Amount > 999 {
				return nil, Localization.WithError(fmt.Errorf("defense tool slot amounts must not exceed 999"), Localization.New("server.app.defense_tool_slot_amounts.d535c05c", "defense tool slot amounts must not exceed 999", nil))
			}
			if gameData == nil {
				return nil, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
				return nil, Localization.WithError(fmt.Errorf("defense tool %d is not in the official units catalog", slot.DefinitionID), Localization.New("server.app.defense_tool_p_is.bcea99e5", "defense tool {p0} is not in the official units catalog", Localization.Params{"p0": fmt.Sprintf("%d", slot.DefinitionID)}))
			}
			record, err := GameData.DecodeRecord(raw)
			if err != nil {
				return nil, Localization.WithError(fmt.Errorf("decode defense tool %d: %w", slot.DefinitionID, err), Localization.ErrorContext(Localization.New("server.app.decode_defense_tool_p.d671c743", "decode defense tool {p0}", Localization.Params{"p0": fmt.Sprintf("%d", slot.DefinitionID)}), err))
			}
			if !GameData.IsToolRecord(record) || !isDefenseToolRecord(record) {
				return nil, Localization.WithError(fmt.Errorf("units definition %d is not a defense tool", slot.DefinitionID), Localization.New("server.app.units_definition_p_is.267ed05d", "units definition {p0} is not a defense tool", Localization.Params{"p0": fmt.Sprintf("%d", slot.DefinitionID)}))
			}
			if len(allowedSlotTypes) > 0 && !defenseToolSupportsAnySlotType(record, allowedSlotTypes) {
				return nil, Localization.WithError(fmt.Errorf("defense tool %d is not valid for this defense section", slot.DefinitionID), Localization.New("server.app.defense_tool_p_is.5d3af1d1", "defense tool {p0} is not valid for this defense section", Localization.Params{"p0": fmt.Sprintf("%d", slot.DefinitionID)}))
			}
			if required[slot.DefinitionID] > math.MaxInt64-slot.Amount {
				return nil, Localization.WithError(fmt.Errorf("defense tool %d amount exceeds the supported range", slot.DefinitionID), Localization.New("server.app.defense_tool_p_amount.c5312c6b", "defense tool {p0} amount exceeds the supported range", Localization.Params{"p0": fmt.Sprintf("%d", slot.DefinitionID)}))
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
				return Localization.WithError(fmt.Errorf("slot rows must use either [-1,0] or two positive values"), Localization.New("server.app.slot_rows_must_use.40361dc5", "slot rows must use either [-1,0] or two positive values", nil))
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
				return Localization.WithError(fmt.Errorf("currently assigned defense tool %d amount exceeds the supported range", slot.DefinitionID), Localization.New("server.app.currently_assigned_defense_tool.d06fbe21", "currently assigned defense tool {p0} amount exceeds the supported range", Localization.Params{"p0": fmt.Sprintf("%d", slot.DefinitionID)}))
			}
			released[slot.DefinitionID] += slot.Amount
		}
	}
	for definitionID, amount := range required {
		available := castle.Defense.Inventory[definitionID]
		if available > math.MaxInt64-released[definitionID] {
			return Localization.WithError(fmt.Errorf("defense tool %d availability exceeds the supported range", definitionID), Localization.New("server.app.defense_tool_p_availability.17bcf8a7", "defense tool {p0} availability exceeds the supported range", Localization.Params{"p0": fmt.Sprintf("%d", definitionID)}))
		}
		available += released[definitionID]
		if available < amount {
			return Localization.WithError(fmt.Errorf(
				"defense tool %d requires %d but castle %d has %d available after releasing the current section setup",
				definitionID, amount, castle.ID, available,
			), Localization.New("server.app.defense_tool_p_requires.4308a580", "defense tool {p0} requires {p1} but castle {p2} has {p3} available after releasing the current section setup", Localization.Params{"p0": fmt.Sprintf("%d", definitionID), "p1": amount, "p2": fmt.Sprintf("%d", castle.ID), "p3": available}))
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
