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

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type defensePresetKeepRequest struct {
	MAUCT           int64 `json:"mauct"`
	UnitTypePercent int   `json:"unitTypePercent"`
}

type defensePresetApplyRequest struct {
	CastleID   State.CastleID            `json:"castleId"`
	PresetID   string                    `json:"presetId,omitempty"`
	PresetName string                    `json:"presetName"`
	Wall       defensePresetWallRequest  `json:"wall"`
	Moat       defensePresetMoatRequest  `json:"moat"`
	Keep       *defensePresetKeepRequest `json:"keep,omitempty"`
	KhanGuard  *khanLaneGuardRequest     `json:"khanGuard,omitempty"`
}

type defensePresetWallRequest struct {
	Left   State.DefenseWallSection `json:"left"`
	Middle State.DefenseWallSection `json:"middle"`
	Right  State.DefenseWallSection `json:"right"`
}

type defensePresetMoatRequest struct {
	LeftToolSlots   []State.DefenseToolSlot `json:"leftToolSlots"`
	MiddleToolSlots []State.DefenseToolSlot `json:"middleToolSlots"`
	RightToolSlots  []State.DefenseToolSlot `json:"rightToolSlots"`
}

type defensePresetResolvedRequest struct {
	defensePresetApplyRequest
	PreviousDefenseObservedAt   time.Time `json:"previousDefenseObservedAt"`
	PreviousInventoryObservedAt time.Time `json:"previousInventoryObservedAt"`
}

func planDefensePresetApply(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request defensePresetApplyRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if err := validateDefensePresetShape(request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.KhanGuard != nil {
		if request.KhanGuard.MainCastleID != request.CastleID {
			return Intent.Plan{}, fmt.Errorf("Khan defense guard does not match castle %d", request.CastleID)
		}
		if err := validateKhanLaneGuard(input.State, input.GameData, *request.KhanGuard, time.Now().UTC()); err != nil {
			return Intent.Plan{}, err
		}
	}
	resolved := defensePresetResolvedRequest{
		defensePresetApplyRequest:   request,
		PreviousDefenseObservedAt:   castle.Defense.ObservedAt,
		PreviousInventoryObservedAt: castle.Defense.InventoryObservedAt,
	}
	resolvedArguments, _ := json.Marshal(resolved)
	moatArguments, _ := json.Marshal(defenseMoatResolvedRequest{
		defenseMoatUpdateRequest: defenseMoatUpdateRequest{
			CastleID:        request.CastleID,
			LeftToolSlots:   request.Moat.LeftToolSlots,
			MiddleToolSlots: request.Moat.MiddleToolSlots,
			RightToolSlots:  request.Moat.RightToolSlots,
		},
		PreviousDefenseObservedAt:   castle.Defense.ObservedAt,
		PreviousInventoryObservedAt: castle.Defense.InventoryObservedAt,
	})

	steps := make([]Intent.Step, 0, 15)
	var khanGuardStep Intent.Step
	if request.KhanGuard != nil {
		guardArguments, _ := json.Marshal(khanLaneGuardActionRequest{KhanGuard: *request.KhanGuard})
		khanGuardStep = Intent.Step{
			Name:   "Recheck Auto Khan safety gates",
			Action: "khan.lane.guard", ActionArguments: guardArguments,
		}
	}
	if request.KhanGuard != nil {
		steps = append(steps, khanGuardStep)
	}
	steps = append(steps, defenseRefreshSteps(castle)...)
	if request.KhanGuard != nil {
		steps = append(steps, khanGuardStep)
	}
	steps = append(steps, Intent.Step{
		Name: "Apply defense preset wall", Resolver: "defense.preset.wall.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfw", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	if request.KhanGuard != nil {
		steps = append(steps, khanGuardStep)
	}
	steps = append(steps, Intent.Step{
		Name: "Apply defense preset moat", Resolver: "defense.moat.build", ResolverArguments: moatArguments,
		AwaitOpcode: "dfm", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	if request.Keep != nil {
		if request.KhanGuard != nil {
			steps = append(steps, khanGuardStep)
		}
		steps = append(steps, Intent.Step{
			Name: "Apply defense preset keep", Resolver: "defense.preset.keep.build", ResolverArguments: resolvedArguments,
			AwaitOpcode: "dfk", TimeoutMillis: 10_000, SuccessCodes: []int{0},
		})
		steps = append(steps, defenseContextStep(castle))
	}
	steps = append(steps, Intent.Step{
		Name: "Verify defense preset", Action: "defense.preset.verify", ActionArguments: resolvedArguments,
	})

	claims := defenseClaims(castle.ID)
	if request.KhanGuard != nil {
		// The Khan loop restocks the wall while the chain keeps attacking from
		// the same castle, so this claims the defense setup and the focus it
		// moves rather than the whole castle.
		claims = []string{"castle-focus", "defense:" + strconv.FormatInt(int64(castle.ID), 10), "khan-lane:defense"}
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: "Apply defense preset " + strings.TrimSpace(request.PresetName) + " to " + castleLabel(castle),
		Steps:   steps,
	}, nil
}

func resolveDefensePresetWallStep(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request defensePresetResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	wallRequest := request.wallUpdateRequest()
	moatRequest := request.moatUpdateRequest()
	castle, wallRequired, err := validateDefenseWallRequest(
		input, wallRequest, false, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt,
	)
	if err != nil {
		return Intent.Step{}, err
	}
	_, moatRequired, err := validateDefenseMoatRequest(
		input, moatRequest, false, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt,
	)
	if err != nil {
		return Intent.Step{}, err
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return Intent.Step{}, err
	}
	if err := validateDefenseToolAvailability(
		castle, wallRequired,
		castle.Defense.Wall.Left.ToolSlots,
		castle.Defense.Wall.Middle.ToolSlots,
		castle.Defense.Wall.Right.ToolSlots,
	); err != nil {
		return Intent.Step{}, err
	}
	combinedRequired, err := combineDefenseToolRequirements(wallRequired, moatRequired)
	if err != nil {
		return Intent.Step{}, err
	}
	if err := validateDefenseToolAvailability(
		castle, combinedRequired,
		castle.Defense.Wall.Left.ToolSlots,
		castle.Defense.Wall.Middle.ToolSlots,
		castle.Defense.Wall.Right.ToolSlots,
		castle.Defense.Moat.LeftToolSlots,
		castle.Defense.Moat.MiddleToolSlots,
		castle.Defense.Moat.RightToolSlots,
	); err != nil {
		return Intent.Step{}, err
	}
	if request.Keep != nil {
		if _, _, err := validateDefenseKeepRequest(input, request.keepUpdateRequest(castle), true,
			request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
			return Intent.Step{}, err
		}
	}

	wallArguments, _ := json.Marshal(defenseWallResolvedRequest{
		defenseWallUpdateRequest:    wallRequest,
		PreviousDefenseObservedAt:   request.PreviousDefenseObservedAt,
		PreviousInventoryObservedAt: request.PreviousInventoryObservedAt,
	})
	return resolveDefenseWallStep(ctx, input, wallArguments)
}

func resolveDefensePresetKeepStep(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request defensePresetResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if request.Keep == nil {
		return Intent.Step{}, fmt.Errorf("defense preset does not include keep settings")
	}
	castle, err := defenseCastle(input, request.CastleID)
	if err != nil {
		return Intent.Step{}, err
	}
	keepArguments, _ := json.Marshal(defenseKeepResolvedRequest{
		defenseKeepUpdateRequest:    request.keepUpdateRequest(castle),
		PreviousDefenseObservedAt:   request.PreviousDefenseObservedAt,
		PreviousInventoryObservedAt: request.PreviousInventoryObservedAt,
	})
	return resolveDefenseKeepStep(ctx, input, keepArguments)
}

func (application *Application) verifyDefensePreset(_ context.Context, arguments json.RawMessage) error {
	var request defensePresetResolvedRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	castle, found := application.State.Snapshot().Castles[request.CastleID]
	if !found {
		return fmt.Errorf("castle %d is no longer in the current player state", request.CastleID)
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	wall := castle.Defense.Wall
	if !reflect.DeepEqual(wall.Left, request.Wall.Left) ||
		!reflect.DeepEqual(wall.Middle, request.Wall.Middle) ||
		!reflect.DeepEqual(wall.Right, request.Wall.Right) {
		return fmt.Errorf("castle %d defense wall setup did not match preset %q", request.CastleID, request.PresetName)
	}
	moat := castle.Defense.Moat
	if !reflect.DeepEqual(moat.LeftToolSlots, request.Moat.LeftToolSlots) ||
		!reflect.DeepEqual(moat.MiddleToolSlots, request.Moat.MiddleToolSlots) ||
		!reflect.DeepEqual(moat.RightToolSlots, request.Moat.RightToolSlots) {
		return fmt.Errorf("castle %d defense moat setup did not match preset %q", request.CastleID, request.PresetName)
	}
	if request.Keep != nil && (castle.Defense.Keep.MAUCT != request.Keep.MAUCT ||
		castle.Defense.Keep.UnitTypePercent != request.Keep.UnitTypePercent) {
		return fmt.Errorf("castle %d defense keep setup did not match preset %q", request.CastleID, request.PresetName)
	}
	return nil
}

func validateDefensePresetShape(request defensePresetApplyRequest) error {
	name := strings.TrimSpace(request.PresetName)
	if name == "" {
		return fmt.Errorf("presetName is required")
	}
	if len(name) > 120 {
		return fmt.Errorf("presetName must not exceed 120 characters")
	}
	if len(request.PresetID) > 200 {
		return fmt.Errorf("presetId must not exceed 200 characters")
	}
	sections := []struct {
		name    string
		section State.DefenseWallSection
	}{
		{name: "wall.left", section: request.Wall.Left},
		{name: "wall.middle", section: request.Wall.Middle},
		{name: "wall.right", section: request.Wall.Right},
	}
	for _, candidate := range sections {
		if candidate.section.UnitPercent < 0 || candidate.section.UnitPercent > 100 {
			return fmt.Errorf("%s.unitPercent must be between 0 and 100", candidate.name)
		}
		if candidate.section.UnitTypePercent < 0 || candidate.section.UnitTypePercent > 100 {
			return fmt.Errorf("%s.unitTypePercent must be between 0 and 100", candidate.name)
		}
	}
	if request.Wall.Left.UnitPercent+request.Wall.Middle.UnitPercent+request.Wall.Right.UnitPercent != 100 {
		return fmt.Errorf("wall left, middle, and right unitPercent values must total 100")
	}
	if err := validateDefenseSlotRows(
		request.Wall.Left.ToolSlots,
		request.Wall.Middle.ToolSlots,
		request.Wall.Right.ToolSlots,
		request.Moat.LeftToolSlots,
		request.Moat.MiddleToolSlots,
		request.Moat.RightToolSlots,
	); err != nil {
		return err
	}
	if request.Keep != nil {
		if request.Keep.MAUCT < 0 {
			return fmt.Errorf("keep.mauct must not be negative")
		}
		if request.Keep.UnitTypePercent < 0 || request.Keep.UnitTypePercent > 100 {
			return fmt.Errorf("keep.unitTypePercent must be between 0 and 100")
		}
	}
	return nil
}

func (request defensePresetResolvedRequest) wallUpdateRequest() defenseWallUpdateRequest {
	return defenseWallUpdateRequest{
		CastleID: request.CastleID,
		Left:     request.Wall.Left,
		Middle:   request.Wall.Middle,
		Right:    request.Wall.Right,
	}
}

func (request defensePresetResolvedRequest) moatUpdateRequest() defenseMoatUpdateRequest {
	return defenseMoatUpdateRequest{
		CastleID:        request.CastleID,
		LeftToolSlots:   request.Moat.LeftToolSlots,
		MiddleToolSlots: request.Moat.MiddleToolSlots,
		RightToolSlots:  request.Moat.RightToolSlots,
	}
}

func (request defensePresetResolvedRequest) keepUpdateRequest(castle State.CastleState) defenseKeepUpdateRequest {
	return defenseKeepUpdateRequest{
		CastleID:           request.CastleID,
		MAUCT:              request.Keep.MAUCT,
		UnitTypePercent:    request.Keep.UnitTypePercent,
		PrimaryToolSlots:   castle.Defense.Keep.PrimaryToolSlots,
		SecondaryToolSlots: castle.Defense.Keep.SecondaryToolSlots,
	}
}

func combineDefenseToolRequirements(groups ...map[State.UnitID]int64) (map[State.UnitID]int64, error) {
	combined := map[State.UnitID]int64{}
	for _, group := range groups {
		for definitionID, amount := range group {
			if amount > math.MaxInt64-combined[definitionID] {
				return nil, fmt.Errorf("defense tool %d amount exceeds the supported range", definitionID)
			}
			combined[definitionID] += amount
		}
	}
	return combined, nil
}
