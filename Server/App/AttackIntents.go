package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	riftMapTypeID            = 43
	maidenSupportEffectID    = 121
	maidenSupportMinimum     = 300
	maidenSupportMaximum     = 1050
	maidenProbeCountPerFlank = 11
)

func planSpyLaunch(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceCastleID State.CastleID  `json:"sourceCastleId,omitempty"`
		TargetX        int             `json:"targetX"`
		TargetY        int             `json:"targetY"`
		KingdomID      State.KingdomID `json:"kingdomId,omitempty"`
		SpyCount       int             `json:"spyCount,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.TargetX < 0 || request.TargetY < 0 {
		return Intent.Plan{}, fmt.Errorf("target coordinates must be non-negative")
	}
	source, err := sourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.SpyCount == 0 {
		request.SpyCount = 1
	}
	if request.SpyCount < 1 || request.SpyCount > 100 {
		return Intent.Plan{}, fmt.Errorf("spyCount must be between 1 and 100")
	}
	payload, _ := json.Marshal(struct {
		SourceID State.CastleID  `json:"SID"`
		TargetX  int             `json:"TX"`
		TargetY  int             `json:"TY"`
		SpyCount int             `json:"SC"`
		SpyType  int             `json:"ST"`
		Success  int             `json:"SE"`
		Booster  int             `json:"HBW"`
		Kingdom  State.KingdomID `json:"KID"`
		Travel   int             `json:"PTT"`
		Delay    int             `json:"SD"`
	}{source.ID, request.TargetX, request.TargetY, request.SpyCount, 0, 100, -1, request.KingdomID, 1, 0})
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(source.ID), 10),
			fmt.Sprintf("spy-target:%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY),
		},
		Summary: fmt.Sprintf("Spy on %d:%d with %d agent(s)", request.TargetX, request.TargetY, request.SpyCount),
		Steps:   []Intent.Step{commandStep("Launch spy mission", "csm", payload, "csm")},
	}, nil
}

func planMaidenCommsWave(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceX *int         `json:"sourceX,omitempty"`
		SourceY *int         `json:"sourceY,omitempty"`
		UnitID  State.UnitID `json:"unitWodID"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, err := sourceCastle(input.State, 0)
	if err != nil {
		return Intent.Plan{}, err
	}
	sourceX, sourceY := source.X, source.Y
	if request.SourceX != nil && request.SourceY != nil {
		sourceX, sourceY = *request.SourceX, *request.SourceY
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, fmt.Errorf("unitWodID must identify a probe unit in the main castle")
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	units, err := input.GameData.Catalog("units")
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, exists := units.Find(strconv.FormatInt(int64(request.UnitID), 10)); !exists {
		return Intent.Plan{}, fmt.Errorf("unit %d is not in the official catalog", request.UnitID)
	}
	availableUnits := source.Units.Stationed[request.UnitID]
	if availableUnits < maidenProbeCountPerFlank*3 {
		return Intent.Plan{}, fmt.Errorf("main castle has %d of unit %d; at least %d are required", availableUnits, request.UnitID, maidenProbeCountPerFlank*3)
	}
	target, ok := riftTarget(input.State)
	if !ok {
		return Intent.Plan{}, fmt.Errorf("the Rift map tile is unknown; refresh the surrounding map first")
	}
	commanders := eligibleMaidenCommanders(input.State)
	maximumByStock := int(availableUnits / (maidenProbeCountPerFlank * 3))
	if len(commanders) > maximumByStock {
		commanders = commanders[:maximumByStock]
	}
	if len(commanders) == 0 {
		return Intent.Plan{}, fmt.Errorf("no free commander has a shield-maiden relic in the supported effect range")
	}
	steps := make([]Intent.Step, 0, len(commanders))
	claims := []string{"rift-maiden-wave", "castle:" + strconv.FormatInt(int64(source.ID), 10)}
	for _, commanderID := range commanders {
		body := maidenAttackBody(sourceX, sourceY, target, commanderID, request.UnitID)
		payload, _ := json.Marshal(body)
		steps = append(steps, commandStep(fmt.Sprintf("Launch Rift probe with commander %d", commanderID), "cra", payload, "cra"))
		claims = append(claims, "commander:"+strconv.FormatInt(int64(commanderID), 10))
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Launch %d shield-maiden Rift probe(s)", len(commanders)),
		Steps:   steps,
	}, nil
}

func (application *Application) planRiftReplay(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LaunchID    string `json:"launchId"`
		CommanderID *int64 `json:"commanderID,omitempty"`
		SourceX     *int   `json:"sourceX,omitempty"`
		SourceY     *int   `json:"sourceY,omitempty"`
		ArriveAt    int64  `json:"arriveAtUnix,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.LaunchID = strings.TrimSpace(request.LaunchID)
	if request.LaunchID == "" {
		return Intent.Plan{}, fmt.Errorf("launchId is required")
	}
	if request.ArriveAt > time.Now().Unix()+30 {
		return Intent.Plan{}, fmt.Errorf("scheduled Rift replay is not available until the 2.0 operation scheduler owns delayed intents")
	}
	raw, ok := application.Configuration.Section("rift.launches")
	if !ok {
		return Intent.Plan{}, fmt.Errorf("no Rift launch templates have been captured")
	}
	var document struct {
		Launches []struct {
			ID   string          `json:"id"`
			Body json.RawMessage `json:"body"`
		} `json:"launches"`
	}
	if json.Unmarshal(raw, &document) != nil {
		return Intent.Plan{}, fmt.Errorf("Rift launch configuration is invalid")
	}
	var body json.RawMessage
	for _, candidate := range document.Launches {
		if candidate.ID == request.LaunchID {
			body = candidate.Body
			break
		}
	}
	if len(body) == 0 {
		return Intent.Plan{}, fmt.Errorf("Rift launch %q has no captured 2.0 command body", request.LaunchID)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		return Intent.Plan{}, fmt.Errorf("Rift launch %q has an invalid command body", request.LaunchID)
	}
	if request.CommanderID != nil {
		fields["LID"], _ = json.Marshal(*request.CommanderID)
	}
	if request.SourceX != nil {
		fields["SX"], _ = json.Marshal(*request.SourceX)
	}
	if request.SourceY != nil {
		fields["SY"], _ = json.Marshal(*request.SourceY)
	}
	commanderID := State.CommanderID(rawMapInt(fields, "LID"))
	commander, exists := input.State.Commanders[commanderID]
	if !exists || !commander.Available {
		return Intent.Plan{}, fmt.Errorf("commander %d is not currently available", commanderID)
	}
	canonical, _ := json.Marshal(fields)
	return Intent.Plan{
		Claims:  []string{"rift-launch:" + request.LaunchID, "commander:" + strconv.FormatInt(int64(commanderID), 10)},
		Summary: fmt.Sprintf("Replay Rift launch %s with commander %d", request.LaunchID, commanderID),
		Steps:   []Intent.Step{commandStep("Replay Rift launch", "cra", canonical, "cra")},
	}, nil
}

func planDecorationPreset(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID  State.CastleID  `json:"castleId"`
		KingdomID State.KingdomID `json:"kingdomId,omitempty"`
		PresetID  string          `json:"presetId"`
		Items     []struct {
			WID   State.BuildingID `json:"wid"`
			X     int              `json:"x"`
			Y     int              `json:"y"`
			R     int              `json:"r"`
			Layer string           `json:"layer,omitempty"`
		} `json:"items"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	if len(request.Items) > 500 {
		return Intent.Plan{}, fmt.Errorf("decoration presets may contain at most 500 placements")
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	buildings, err := input.GameData.Catalog("buildings")
	if err != nil {
		return Intent.Plan{}, err
	}
	for _, item := range request.Items {
		if item.WID <= 0 || item.X < 0 || item.Y < 0 || item.R < 0 || item.R > 3 {
			return Intent.Plan{}, fmt.Errorf("preset %q contains an invalid placement", request.PresetID)
		}
		raw, found := buildings.Find(strconv.FormatInt(int64(item.WID), 10))
		if !found {
			return Intent.Plan{}, fmt.Errorf("building definition %d is not in the official catalog", item.WID)
		}
		record, _ := GameData.DecodeRecord(raw)
		if !officialDecoration(record) {
			return Intent.Plan{}, fmt.Errorf("building definition %d is not an official decoration", item.WID)
		}
	}
	matched := make([]bool, len(request.Items))
	remove := make([]State.BuildingInstanceID, 0)
	for instanceID, building := range castle.Buildings {
		raw, found := buildings.Find(strconv.FormatInt(int64(building.DefinitionID), 10))
		if !found {
			continue
		}
		record, _ := GameData.DecodeRecord(raw)
		if !officialDecoration(record) {
			continue
		}
		match := -1
		for index, item := range request.Items {
			if !matched[index] && item.WID == building.DefinitionID && item.X == building.GridX && item.Y == building.GridY && item.R == building.Rotation {
				match = index
				break
			}
		}
		if match >= 0 {
			matched[match] = true
		} else {
			remove = append(remove, instanceID)
		}
	}
	sort.Slice(remove, func(left, right int) bool { return remove[left] < remove[right] })
	steps := castleContextSteps(castle)
	if len(remove) > 0 || unmatchedCount(matched) > 0 {
		steps = append(steps, Intent.Step{
			Name: "Refresh decoration storage", Opcode: "sin", AwaitOpcode: "sin", TimeoutMillis: 10_000,
			SuccessCodes: []int{0}, Command: Protocol.Command{Opcode: "sin", Bare: true},
		})
	}
	for _, instanceID := range remove {
		payload, _ := json.Marshal(struct {
			CastleID   State.CastleID           `json:"CID"`
			InstanceID State.BuildingInstanceID `json:"OID"`
		}{castle.ID, instanceID})
		steps = append(steps, commandStep(fmt.Sprintf("Store decoration %d", instanceID), "sob", payload, "sob"))
	}
	for index, item := range request.Items {
		if matched[index] {
			continue
		}
		payload, _ := json.Marshal(struct {
			WID   State.BuildingID `json:"WID"`
			X     int              `json:"X"`
			Y     int              `json:"Y"`
			R     int              `json:"R"`
			Power int              `json:"PWR"`
			Order int              `json:"PO"`
			Owner int              `json:"DOID"`
		}{item.WID, item.X, item.Y, item.R, 0, -1, -1})
		steps = append(steps, commandStep(fmt.Sprintf("Place decoration %d", item.WID), "ebu", payload, "ebu"))
	}
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10), "decoration-layout"},
		Summary: fmt.Sprintf("Apply decoration preset %s to %s (%d removals, %d placements)", request.PresetID, castleLabel(castle), len(remove), unmatchedCount(matched)),
		Steps:   steps,
	}, nil
}

func sourceCastle(state State.GameState, requested State.CastleID) (State.CastleState, error) {
	if requested > 0 {
		castle, ok := state.Castles[requested]
		if !ok {
			return State.CastleState{}, fmt.Errorf("source castle %d is not owned by the current player", requested)
		}
		return castle, nil
	}
	for _, castle := range state.Castles {
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return castle, nil
		}
	}
	return State.CastleState{}, fmt.Errorf("the main castle is not known")
}

func riftTarget(state State.GameState) (State.MapObservation, bool) {
	for _, kingdom := range state.Map {
		for _, observation := range kingdom {
			if observation.TypeID == riftMapTypeID {
				return observation, true
			}
		}
	}
	return State.MapObservation{}, false
}

func eligibleMaidenCommanders(state State.GameState) []State.CommanderID {
	eligible := map[State.CommanderID]struct{}{}
	for _, equipment := range state.Inventory.Equipment {
		if equipment.WearerKind != "commander" || (equipment.RarityID != 5 && equipment.RarityID != 15) {
			continue
		}
		values := equipment.Effects[maidenSupportEffectID]
		if len(values) == 0 {
			continue
		}
		value := values[len(values)-1]
		commanderID := State.CommanderID(equipment.WearerID)
		commander, ok := state.Commanders[commanderID]
		if ok && commanderID > 0 && commander.Available && value >= maidenSupportMinimum && value <= maidenSupportMaximum {
			eligible[commanderID] = struct{}{}
		}
	}
	result := make([]State.CommanderID, 0, len(eligible))
	for id := range eligible {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

type attackPair [2]int64

type attackFlank struct {
	Tools []attackPair `json:"T"`
	Units []attackPair `json:"U"`
}

type attackWave struct {
	Left   attackFlank `json:"L"`
	Right  attackFlank `json:"R"`
	Middle attackFlank `json:"M"`
}

type attackBody struct {
	SourceX            int               `json:"SX"`
	SourceY            int               `json:"SY"`
	TargetX            int               `json:"TX"`
	TargetY            int               `json:"TY"`
	Kingdom            State.KingdomID   `json:"KID"`
	Leader             State.CommanderID `json:"LID"`
	WaitHours          int               `json:"WT"`
	Booster            int               `json:"HBW"`
	BoosterCost        int               `json:"BPC"`
	AttackType         int               `json:"ATT"`
	Valid              int               `json:"AV"`
	LootPriority       int               `json:"LP"`
	FormationCost      int               `json:"FC"`
	PremiumTravel      int               `json:"PTT"`
	StartDelay         int               `json:"SD"`
	InstantCapture     int               `json:"ICA"`
	Cooldown           int               `json:"CD"`
	Waves              []attackWave      `json:"A"`
	Books              []any             `json:"BKS"`
	AttackSupportTools []int64           `json:"AST"`
	SupportTroops      []attackPair      `json:"RW"`
	AttackSupportCount int               `json:"ASCT"`
}

func maidenAttackBody(sourceX, sourceY int, target State.MapObservation, commanderID State.CommanderID, unitID State.UnitID) attackBody {
	empty := attackPair{-1, 0}
	pair := attackPair{int64(unitID), maidenProbeCountPerFlank}
	wave := attackWave{
		Left:  attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{pair, empty}},
		Right: attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{pair, empty}},
		Middle: attackFlank{
			Tools: []attackPair{empty, empty, empty},
			Units: []attackPair{pair, empty, empty, empty, empty, empty},
		},
	}
	return attackBody{
		SourceX: sourceX, SourceY: sourceY, TargetX: target.X, TargetY: target.Y,
		Kingdom: target.KingdomID, Leader: commanderID, Booster: -1, Valid: 1,
		PremiumTravel: 1, Cooldown: 99, Waves: []attackWave{wave}, Books: []any{},
		AttackSupportTools: []int64{-1, -1, -1},
		SupportTroops:      []attackPair{empty, empty, empty, empty, empty, empty, empty, empty},
	}
}

func officialDecoration(record GameData.Record) bool {
	ground, _ := record.String("buildingGroundType")
	category, _ := record.String("shopCategory")
	name, _ := record.String("name")
	typeName, _ := record.String("type")
	return ground == "DECO" || category == "DECO" || name == "Deco" || strings.Contains(strings.ToLower(typeName), "deco")
}

func unmatchedCount(matched []bool) int {
	count := 0
	for _, value := range matched {
		if !value {
			count++
		}
	}
	return count
}

func rawMapInt(values map[string]json.RawMessage, key string) int64 {
	var value int64
	_ = json.Unmarshal(values[key], &value)
	return value
}
