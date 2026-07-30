package API

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/AllianceTargets"
	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type allianceTargetAttackPreviewRequest struct {
	SourceCastleID    State.CastleID       `json:"sourceCastleId"`
	KingdomID         State.KingdomID      `json:"kingdomId"`
	TargetX           int                  `json:"targetX"`
	TargetY           int                  `json:"targetY"`
	TargetCastleID    int64                `json:"targetCastleId,omitempty"`
	TargetTypeID      int                  `json:"targetTypeId"`
	TargetLevel       int                  `json:"targetLevel"`
	TargetLegendLevel int                  `json:"targetLegendLevel,omitempty"`
	Preset            AttackPresets.Preset `json:"preset"`
}

type allianceTargetAttackRequirement struct {
	Kind      string `json:"kind"`
	ItemID    int64  `json:"itemId"`
	Required  int64  `json:"required"`
	Available int64  `json:"available"`
}

type allianceTargetAttackPreview struct {
	CommanderID     State.CommanderID                 `json:"commanderId"`
	Capacity        AttackCapacity.LaneCapacity       `json:"capacity"`
	SupportCapacity int64                             `json:"supportCapacity"`
	MaximumWaves    int                               `json:"maximumWaves"`
	PresetWaves     int                               `json:"presetWaves"`
	AppliedWaves    int                               `json:"appliedWaves"`
	TotalTroops     int64                             `json:"totalTroops"`
	TotalTools      int64                             `json:"totalTools"`
	Requirements    []allianceTargetAttackRequirement `json:"requirements"`
	Ready           bool                              `json:"ready"`
}

func (server *Server) handleAllianceTargets(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil || server.config.AllianceTargets == nil {
		writeError(writer, http.StatusServiceUnavailable, "alliance_targets_unavailable", "Alliance target state is unavailable")
		return
	}
	var gameData *GameData.Store
	if server.config.GameData != nil {
		gameData, _ = server.config.GameData.Current()
	}
	values := request.URL.Query()
	page, _ := strconv.Atoi(values.Get("page"))
	query := AllianceTargets.Query{
		Search: values.Get("search"), Status: values.Get("status"),
		Sort: values.Get("sort"), Direction: values.Get("direction"), Page: page,
		IncludeAlliances: values.Get("includeAlliances") != "0" && values.Get("includeAlliances") != "false",
	}
	ctx, cancel := context.WithTimeout(request.Context(), 15*time.Second)
	defer cancel()
	view, err := server.config.AllianceTargets.View(
		ctx,
		server.config.State.Snapshot(),
		gameData,
		values.Get("server"),
		values.Get("allianceId"),
		values.Get("refresh") == "1" || values.Get("refresh") == "true",
		query,
	)
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "alliance_targets_failed", err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, view)
}

func (server *Server) handleAllianceTargetAttackPreview(writer http.ResponseWriter, request *http.Request) {
	if server.config.State == nil || server.config.GameData == nil {
		writeError(writer, http.StatusServiceUnavailable, "attack_preview_unavailable", "Attack preview state is unavailable")
		return
	}
	gameData, ready := server.config.GameData.Current()
	if !ready || gameData == nil {
		writeError(writer, http.StatusServiceUnavailable, "game_data_unavailable", "Official game data is unavailable")
		return
	}
	var input allianceTargetAttackPreviewRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.SourceCastleID <= 0 || input.KingdomID != 0 || input.TargetX < 0 || input.TargetY < 0 {
		writeError(writer, http.StatusBadRequest, "invalid_request", "A Great Empire source and valid target coordinates are required")
		return
	}
	if input.TargetTypeID <= 0 || input.TargetLevel <= 0 {
		writeError(writer, http.StatusBadRequest, "invalid_request", "Target castle type and player level are required")
		return
	}
	if err := AttackPresets.Validate(input.Preset); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_preset", err.Error())
		return
	}

	gameState := server.config.State.Snapshot()
	source, exists := gameState.Castles[input.SourceCastleID]
	if !exists || source.KingdomID != input.KingdomID {
		writeError(writer, http.StatusUnprocessableEntity, "source_unavailable", "The selected source is not an owned Great Empire castle")
		return
	}
	if source.UnitsObservedAt.IsZero() {
		writeError(writer, http.StatusUnprocessableEntity, "inventory_unavailable", "The selected castle inventory has not been observed")
		return
	}
	commanderID, found := firstAvailableCommander(gameState, time.Now().UTC())
	if !found {
		writeError(writer, http.StatusUnprocessableEntity, "commander_unavailable", "No commander is currently free")
		return
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(gameState, gameData, AttackCapacity.Request{
		SourceCastleID: input.SourceCastleID,
		CommanderID:    commanderID,
		Target: AttackCapacity.TargetContext{
			ID: "alliance-target-preview",
			Map: &AttackCapacity.MapTarget{
				KingdomID: input.KingdomID, TypeID: input.TargetTypeID, X: input.TargetX, Y: input.TargetY,
				ObjectID: input.TargetCastleID, Level: input.TargetLevel,
			},
			Level: input.TargetLevel, CastleTypeID: input.TargetTypeID, PvP: true,
			LegendaryFight: gameState.Player.LegendLevel > 0 && input.TargetLegendLevel > 0,
		},
	})
	if err != nil {
		writeError(writer, http.StatusUnprocessableEntity, "attack_capacity_failed", err.Error())
		return
	}
	limited := AttackPresets.LimitToCapacity(input.Preset, capacity)
	requirements, totalTroops, totalTools, formationTroops := allianceTargetRequirements(limited, source)
	formationReady := formationTroops > 0
	for _, requirement := range requirements {
		if requirement.Required > requirement.Available {
			formationReady = false
			break
		}
	}
	writeJSON(writer, http.StatusOK, allianceTargetAttackPreview{
		CommanderID: commanderID, Capacity: capacity.Capacity, SupportCapacity: capacity.SupportCapacity,
		MaximumWaves: capacity.MaximumWaves,
		PresetWaves:  len(input.Preset.Waves), AppliedWaves: len(limited.Waves),
		TotalTroops: totalTroops, TotalTools: totalTools, Requirements: requirements, Ready: formationReady,
	})
}

func firstAvailableCommander(gameState State.GameState, now time.Time) (State.CommanderID, bool) {
	ids := make([]State.CommanderID, 0, len(gameState.Commanders))
	for id, commander := range gameState.Commanders {
		if id >= 0 && commander.Available && !State.CommanderHasActiveMovementAt(gameState, id, now) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	if len(ids) == 0 {
		return 0, false
	}
	return ids[0], true
}

func allianceTargetRequirements(
	preset AttackPresets.Preset,
	source State.CastleState,
) ([]allianceTargetAttackRequirement, int64, int64, int64) {
	type requirementKey struct {
		kind   string
		itemID int64
	}
	required := map[requirementKey]int64{}
	var totalTroops, totalTools, formationTroops int64
	add := func(kind string, slots []AttackPresets.Slot) {
		for _, slot := range slots {
			if slot.ItemID == nil || *slot.ItemID <= 0 || slot.Quantity <= 0 {
				continue
			}
			required[requirementKey{kind: kind, itemID: *slot.ItemID}] += slot.Quantity
			if kind == "troop" {
				totalTroops += slot.Quantity
			} else {
				totalTools += slot.Quantity
			}
		}
	}
	for _, wave := range preset.Waves {
		for _, lane := range []AttackPresets.Lane{wave.Left, wave.Middle, wave.Right} {
			for _, slot := range lane.Troops {
				if slot.ItemID != nil && *slot.ItemID > 0 && slot.Quantity > 0 {
					formationTroops += slot.Quantity
				}
			}
			add("troop", lane.Troops)
			add("tool", lane.Tools)
		}
	}
	add("troop", preset.CourtyardSupport.Troops)
	add("tool", preset.CourtyardSupport.Tools)
	keys := make([]requirementKey, 0, len(required))
	for key := range required {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].kind != keys[right].kind {
			return keys[left].kind < keys[right].kind
		}
		return keys[left].itemID < keys[right].itemID
	})
	result := make([]allianceTargetAttackRequirement, 0, len(keys))
	for _, key := range keys {
		result = append(result, allianceTargetAttackRequirement{
			Kind: key.kind, ItemID: key.itemID, Required: required[key],
			Available: source.Units.Stationed[State.UnitID(key.itemID)],
		})
	}
	return result, totalTroops, totalTools, formationTroops
}
