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

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type defensePresetKeepRequest struct {
	MAUCT              int64                    `json:"mauct"`
	UnitTypePercent    int                      `json:"unitTypePercent"`
	PrimaryToolSlots   *[]State.DefenseToolSlot `json:"primaryToolSlots,omitempty"`
	SecondaryToolSlots *[]State.DefenseToolSlot `json:"secondaryToolSlots,omitempty"`
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
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("Khan defense guard does not match castle %d", request.CastleID), Localization.New("server.app.khan_defense_guard_does.2287764f", "Khan defense guard does not match castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
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
			Name: "Recheck Auto Khan safety gates", NameDescriptor: Localization.New("server.app.recheck_auto_khan_safety.67340888", "Recheck Auto Khan safety gates", nil),
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
		Name: "Apply defense preset wall", NameDescriptor: Localization.New("server.app.apply_defense_preset_wall.69d11d73", "Apply defense preset wall", nil), Resolver: "defense.preset.wall.build", ResolverArguments: resolvedArguments,
		AwaitOpcode: "dfw", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	if request.KhanGuard != nil {
		steps = append(steps, khanGuardStep)
	}
	steps = append(steps, Intent.Step{
		Name: "Apply defense preset moat", NameDescriptor: Localization.New("server.app.apply_defense_preset_moat.c6462de2", "Apply defense preset moat", nil), Resolver: "defense.moat.build", ResolverArguments: moatArguments,
		AwaitOpcode: "dfm", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	steps = append(steps, defenseContextStep(castle))
	if request.Keep != nil {
		if request.KhanGuard != nil {
			steps = append(steps, khanGuardStep)
		}
		steps = append(steps, Intent.Step{
			Name: "Apply defense preset keep", NameDescriptor: Localization.New("server.app.apply_defense_preset_keep.1076a781", "Apply defense preset keep", nil), Resolver: "defense.preset.keep.build", ResolverArguments: resolvedArguments,
			AwaitOpcode: "dfk", TimeoutMillis: 10_000, SuccessCodes: []int{0},
		})
		steps = append(steps, defenseContextStep(castle))
	}
	steps = append(steps, Intent.Step{
		Name: "Verify defense preset", NameDescriptor: Localization.New("server.app.verify_defense_preset.e5d261ee", "Verify defense preset", nil), Action: "defense.preset.verify", ActionArguments: resolvedArguments,
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
	requiredGroups := []map[State.UnitID]int64{wallRequired, moatRequired}
	if request.Keep != nil {
		_, keepRequired, keepErr := validateDefenseKeepRequest(
			input, request.keepUpdateRequest(castle), false,
			request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt,
		)
		if keepErr != nil {
			return Intent.Step{}, keepErr
		}
		requiredGroups = append(requiredGroups, keepRequired)
	}
	combinedRequired, err := combineDefenseToolRequirements(requiredGroups...)
	if err != nil {
		return Intent.Step{}, err
	}
	releasedGroups := [][]State.DefenseToolSlot{
		castle.Defense.Wall.Left.ToolSlots,
		castle.Defense.Wall.Middle.ToolSlots,
		castle.Defense.Wall.Right.ToolSlots,
		castle.Defense.Moat.LeftToolSlots,
		castle.Defense.Moat.MiddleToolSlots,
		castle.Defense.Moat.RightToolSlots,
	}
	if request.Keep != nil {
		releasedGroups = append(
			releasedGroups,
			castle.Defense.Keep.PrimaryToolSlots,
			castle.Defense.Keep.SecondaryToolSlots,
		)
	}
	if err := validateDefenseToolAvailability(
		castle, combinedRequired, releasedGroups...,
	); err != nil {
		return Intent.Step{}, err
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("defense preset does not include keep settings"), Localization.New("server.app.defense_preset_does_not.2f1bca4a", "defense preset does not include keep settings", nil))
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
	castle, found := application.State.ReadOnlyView().Castles[request.CastleID]
	if !found {
		return Localization.WithError(fmt.Errorf("castle %d is no longer in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_no.eb2a23ef", "castle {p0} is no longer in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if err := verifyDefenseObservation(castle, request.PreviousDefenseObservedAt, request.PreviousInventoryObservedAt); err != nil {
		return err
	}
	wall := castle.Defense.Wall
	if !reflect.DeepEqual(wall.Left, request.Wall.Left) ||
		!reflect.DeepEqual(wall.Middle, request.Wall.Middle) ||
		!reflect.DeepEqual(wall.Right, request.Wall.Right) {
		return Localization.WithError(fmt.Errorf("castle %d defense wall setup did not match preset %q", request.CastleID, request.PresetName), Localization.New("server.app.castle_p_defense_wall.bae35648", "castle {p0} defense wall setup did not match preset {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID), "p1": fmt.Sprintf("%q", request.PresetName)}))
	}
	moat := castle.Defense.Moat
	if !reflect.DeepEqual(moat.LeftToolSlots, request.Moat.LeftToolSlots) ||
		!reflect.DeepEqual(moat.MiddleToolSlots, request.Moat.MiddleToolSlots) ||
		!reflect.DeepEqual(moat.RightToolSlots, request.Moat.RightToolSlots) {
		return Localization.WithError(fmt.Errorf("castle %d defense moat setup did not match preset %q", request.CastleID, request.PresetName), Localization.New("server.app.castle_p_defense_moat.da16cbc0", "castle {p0} defense moat setup did not match preset {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID), "p1": fmt.Sprintf("%q", request.PresetName)}))
	}
	if request.Keep != nil {
		if castle.Defense.Keep.MAUCT != request.Keep.MAUCT ||
			castle.Defense.Keep.UnitTypePercent != request.Keep.UnitTypePercent {
			return Localization.WithError(fmt.Errorf("castle %d defense keep setup did not match preset %q", request.CastleID, request.PresetName), Localization.New("server.app.castle_p_defense_keep.f050e98d", "castle {p0} defense keep setup did not match preset {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID), "p1": fmt.Sprintf("%q", request.PresetName)}))
		}
		if request.Keep.PrimaryToolSlots != nil && request.Keep.SecondaryToolSlots != nil &&
			(!reflect.DeepEqual(castle.Defense.Keep.PrimaryToolSlots, *request.Keep.PrimaryToolSlots) ||
				!reflect.DeepEqual(castle.Defense.Keep.SecondaryToolSlots, *request.Keep.SecondaryToolSlots)) {
			return Localization.WithError(fmt.Errorf("castle %d defense courtyard tools did not match preset %q", request.CastleID, request.PresetName), Localization.New("server.app.castle_p_defense_courtyard.4488d440", "castle {p0} defense courtyard tools did not match preset {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID), "p1": fmt.Sprintf("%q", request.PresetName)}))
		}
	}
	return nil
}

func validateDefensePresetShape(request defensePresetApplyRequest) error {
	name := strings.TrimSpace(request.PresetName)
	if name == "" {
		return Localization.WithError(fmt.Errorf("presetName is required"), Localization.New("server.app.presetname_is_required.96feb493", "presetName is required", nil))
	}
	if len(name) > 120 {
		return Localization.WithError(fmt.Errorf("presetName must not exceed 120 characters"), Localization.New("server.app.presetname_must_not_exceed.2085de0b", "presetName must not exceed 120 characters", nil))
	}
	if len(request.PresetID) > 200 {
		return Localization.WithError(fmt.Errorf("presetId must not exceed 200 characters"), Localization.New("server.app.presetid_must_not_exceed.3c38ae4a", "presetId must not exceed 200 characters", nil))
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
			return Localization.WithError(fmt.Errorf("%s.unitPercent must be between 0 and 100", candidate.name), Localization.New("server.app.p_unitpercent_must_be.7f4c584f", "{p0}.unitPercent must be between 0 and 100", Localization.Params{"p0": fmt.Sprintf("%s", candidate.name)}))
		}
		if candidate.section.UnitTypePercent < 0 || candidate.section.UnitTypePercent > 100 {
			return Localization.WithError(fmt.Errorf("%s.unitTypePercent must be between 0 and 100", candidate.name), Localization.New("server.app.p_unittypepercent_must_be.710cc7ed", "{p0}.unitTypePercent must be between 0 and 100", Localization.Params{"p0": fmt.Sprintf("%s", candidate.name)}))
		}
	}
	if request.Wall.Left.UnitPercent+request.Wall.Middle.UnitPercent+request.Wall.Right.UnitPercent != 100 {
		return Localization.WithError(fmt.Errorf("wall left, middle, and right unitPercent values must total 100"), Localization.New("server.app.wall_left_middle_and.2115d08a", "wall left, middle, and right unitPercent values must total 100", nil))
	}
	if err := validateDefenseWallSlotCounts(
		request.Wall.Left.ToolSlots,
		request.Wall.Middle.ToolSlots,
		request.Wall.Right.ToolSlots,
	); err != nil {
		return err
	}
	if err := validateDefenseMoatSlotCounts(
		request.Moat.LeftToolSlots,
		request.Moat.MiddleToolSlots,
		request.Moat.RightToolSlots,
	); err != nil {
		return err
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
			return Localization.WithError(fmt.Errorf("keep.mauct must not be negative"), Localization.New("server.app.keep_mauct_must_not.256efd1f", "keep.mauct must not be negative", nil))
		}
		if request.Keep.UnitTypePercent < 0 || request.Keep.UnitTypePercent > 100 {
			return Localization.WithError(fmt.Errorf("keep.unitTypePercent must be between 0 and 100"), Localization.New("server.app.keep_unittypepercent_must_be.204a604a", "keep.unitTypePercent must be between 0 and 100", nil))
		}
		hasPrimaryToolSlots := request.Keep.PrimaryToolSlots != nil
		hasSecondaryToolSlots := request.Keep.SecondaryToolSlots != nil
		if hasPrimaryToolSlots != hasSecondaryToolSlots {
			return Localization.WithError(fmt.Errorf("keep must include both primaryToolSlots and secondaryToolSlots or omit both"), Localization.New("server.app.keep_must_include_both.0ff95e4d", "keep must include both primaryToolSlots and secondaryToolSlots or omit both", nil))
		}
		if hasPrimaryToolSlots {
			if err := validateDefenseKeepSlotCounts(
				*request.Keep.PrimaryToolSlots,
				*request.Keep.SecondaryToolSlots,
			); err != nil {
				return Localization.WithError(fmt.Errorf("keep: %w", err), Localization.ErrorContext(Localization.New("server.app.keep.6ca7ea2f", "keep", nil), err))
			}
			if err := validateDefenseSlotRows(
				*request.Keep.PrimaryToolSlots,
				*request.Keep.SecondaryToolSlots,
			); err != nil {
				return Localization.WithError(fmt.Errorf("keep: %w", err), Localization.ErrorContext(Localization.New("server.app.keep.6ca7ea2f", "keep", nil), err))
			}
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
	primaryToolSlots := castle.Defense.Keep.PrimaryToolSlots
	secondaryToolSlots := castle.Defense.Keep.SecondaryToolSlots
	if request.Keep.PrimaryToolSlots != nil && request.Keep.SecondaryToolSlots != nil {
		primaryToolSlots = *request.Keep.PrimaryToolSlots
		secondaryToolSlots = *request.Keep.SecondaryToolSlots
	}
	return defenseKeepUpdateRequest{
		CastleID:           request.CastleID,
		MAUCT:              request.Keep.MAUCT,
		UnitTypePercent:    request.Keep.UnitTypePercent,
		PrimaryToolSlots:   primaryToolSlots,
		SecondaryToolSlots: secondaryToolSlots,
	}
}

func combineDefenseToolRequirements(groups ...map[State.UnitID]int64) (map[State.UnitID]int64, error) {
	combined := map[State.UnitID]int64{}
	for _, group := range groups {
		for definitionID, amount := range group {
			if amount > math.MaxInt64-combined[definitionID] {
				return nil, Localization.WithError(fmt.Errorf("defense tool %d amount exceeds the supported range", definitionID), Localization.New("server.app.defense_tool_p_amount.c5312c6b", "defense tool {p0} amount exceeds the supported range", Localization.Params{"p0": fmt.Sprintf("%d", definitionID)}))
			}
			combined[definitionID] += amount
		}
	}
	return combined, nil
}
