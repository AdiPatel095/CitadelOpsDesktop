package Toolkit

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Models"
	mapstate "CitadelDesktop/Server/Models/MapState"
	"CitadelDesktop/Server/ResponseRegistry"
)

type stateCoordinate struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type stateReadInput struct {
	Scope       string            `json:"scope"`
	CastleID    int               `json:"castleId,omitempty"`
	KingdomID   *int              `json:"kingdomId,omitempty"`
	Coordinates []stateCoordinate `json:"coordinates,omitempty"`
}

type stateAwaitInput struct {
	StateKey     string `json:"stateKey"`
	AfterVersion uint64 `json:"afterVersion"`
	TimeoutMS    int    `json:"timeoutMs,omitempty"`
}

type stateCastleSummary struct {
	Key       string                       `json:"key"`
	Type      string                       `json:"type"`
	CastleID  int                          `json:"castleId"`
	Name      string                       `json:"name"`
	KingdomID int                          `json:"kingdomId"`
	MapX      int                          `json:"mapX"`
	MapY      int                          `json:"mapY"`
	Resources Models.CastleResourcesAmount `json:"resources"`
}

func registerStateTools(harness *Harness) error {
	coordinateSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"x": schemaProperty("integer", "World-map X coordinate."),
			"y": schemaProperty("integer", "World-map Y coordinate."),
		},
		"required": []string{"x", "y"},
	}
	schema := objectSchema(map[string]interface{}{
		"scope": enumProperty(
			"The smallest state slice needed for the current decision.",
			"summary", "all", "player", "castles", "castle", "focus", "resources", "movements",
			"equipment", "alliance", "tci", "transports", "map", "settings", "connection", "features",
			"automation", "rift", "last_known",
		),
		"castleId":  schemaProperty("integer", "Required for scope=castle."),
		"kingdomId": schemaProperty("integer", "Required for scope=map; KID 0 is valid."),
		"coordinates": map[string]interface{}{
			"type":        "array",
			"description": "Required for scope=map. Limited to 100 exact tiles to control context size.",
			"items":       coordinateSchema,
			"maxItems":    100,
		},
	}, "scope")

	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.state.read",
			Description: "Read a captured JSON view of live Citadel/game state. Prefer summary or a narrow scope; all excludes world-map tiles, and map reads require exact coordinates.",
			InputSchema: schema,
			Effect:      EffectRead,
			Tags:        []string{"state", "read"},
		},
		Handler: readState,
	}); err != nil {
		return err
	}

	return harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.state.await",
			Description: "Wait until a parser-observed state key advances beyond a command's returned afterVersion cursor. Then call citadel.state.read for the updated value.",
			InputSchema: objectSchema(map[string]interface{}{
				"stateKey":     schemaProperty("string", "Response opcode/state key returned by citadel.command.send."),
				"afterVersion": schemaProperty("integer", "Version cursor captured before the command was queued."),
				"timeoutMs":    schemaProperty("integer", "Optional wait timeout from 1 to 120000 milliseconds; default 10000."),
			}, "stateKey", "afterVersion"),
			Effect: EffectRead,
			Tags:   []string{"state", "read", "wait"},
		},
		Handler: awaitState,
	})
}

func awaitState(ctx context.Context, raw json.RawMessage) (interface{}, error) {
	input, err := decodeStrict[stateAwaitInput](raw)
	if err != nil {
		return nil, err
	}
	stateKey := strings.ToLower(strings.TrimSpace(input.StateKey))
	if !validStateKey(stateKey) {
		return nil, toolError("invalid_arguments", "stateKey must contain 1 to 64 lowercase letters, digits, colon, underscore, or hyphen")
	}
	timeoutMS := input.TimeoutMS
	if timeoutMS == 0 {
		timeoutMS = 10000
	}
	if timeoutMS < 1 || timeoutMS > 120000 {
		return nil, toolError("invalid_arguments", "timeoutMs must be between 1 and 120000")
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
	defer cancel()
	stamp, ok := Automation.AwaitStateAfter(waitCtx, stateKey, input.AfterVersion)
	if !ok {
		return nil, toolError("state_timeout", "state %q did not advance beyond version %d within %dms", stateKey, input.AfterVersion, timeoutMS)
	}
	return map[string]interface{}{"stateKey": stateKey, "stamp": stamp}, nil
}

func validStateKey(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != ':' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func readState(_ context.Context, raw json.RawMessage) (interface{}, error) {
	input, err := decodeStrict[stateReadInput](raw)
	if err != nil {
		return nil, err
	}
	gameState := Models.GetGameState()
	capturedAt := time.Now().UnixMilli()

	switch input.Scope {
	case "summary":
		return stateSummary(capturedAt), nil
	case "all":
		return map[string]interface{}{
			"capturedAtUnixMs": capturedAt,
			"connection":       ResponseRegistry.GetGameConnectionStatus(),
			"gameState":        gameState,
			"settings":         Models.GetSettingsState(),
			"features":         featureStatuses(),
			"automation": map[string]interface{}{
				"coordinator":   Automation.Snapshot(),
				"commandQueues": automationCommandQueueSummary(),
				"stateVersions": Automation.State.All(),
			},
		}, nil
	case "player":
		return map[string]interface{}{
			"capturedAtUnixMs": capturedAt,
			"playerId":         gameState.PlayerID,
			"playerName":       gameState.PlayerName,
			"vip":              gameState.VIP,
			"subscriptions":    gameState.Subscriptions,
		}, nil
	case "castles":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "castles": gameState.Castle}, nil
	case "castle":
		if input.CastleID <= 0 {
			return nil, toolError("invalid_arguments", "castleId must be positive for scope=castle")
		}
		castleInfo := gameState.GetCastleByID(input.CastleID)
		if castleInfo == nil {
			return nil, toolError("not_found", "castle %d is not present in live state", input.CastleID)
		}
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "castle": castleInfo}, nil
	case "focus":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "focus": Models.CastleFocusMessagePayload()}, nil
	case "resources":
		return map[string]interface{}{
			"capturedAtUnixMs": capturedAt,
			"global":           gameState.GlobalResources,
			"castles":          castleResourceSnapshots(),
		}, nil
	case "movements":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "movement": Models.MovementUpdatePayload()}, nil
	case "equipment":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "equipment": gameState.Equipment}, nil
	case "alliance":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "alliance": gameState.Alliance}, nil
	case "tci":
		return map[string]interface{}{
			"capturedAtUnixMs":     capturedAt,
			"session":              gameState.Tci,
			"constructionByCastle": constructionItemsByCastle(),
		}, nil
	case "transports":
		return map[string]interface{}{
			"capturedAtUnixMs": capturedAt,
			"kingdom":          gameState.KingdomTransportSnapshot(),
			"market":           gameState.MarketTransportSnapshot(),
		}, nil
	case "map":
		return readMapState(capturedAt, input)
	case "settings":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "settings": Models.GetSettingsState()}, nil
	case "connection":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "connection": ResponseRegistry.GetGameConnectionStatus()}, nil
	case "features":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "features": featureStatuses()}, nil
	case "automation":
		return map[string]interface{}{
			"capturedAtUnixMs": capturedAt,
			"coordinator":      Automation.Snapshot(),
			"commandQueues":    automationCommandQueueSummary(),
			"stateVersions":    Automation.State.All(),
		}, nil
	case "rift":
		return map[string]interface{}{"capturedAtUnixMs": capturedAt, "rift": Models.RiftMapCoordsPayload()}, nil
	case "last_known":
		snapshot, snapshotErr := Models.ReadGameStateSnapshotMap()
		if snapshotErr != nil {
			return nil, toolError("not_found", "last-known snapshot is unavailable: %v", snapshotErr)
		}
		return snapshot, nil
	default:
		return nil, toolError("invalid_arguments", "unsupported state scope %q", input.Scope)
	}
}

func stateSummary(capturedAt int64) map[string]interface{} {
	gameState := Models.GetGameState()
	castles := make([]stateCastleSummary, 0, len(liveCastleSlots()))
	for _, slot := range liveCastleSlots() {
		if slot.Info == nil || slot.Info.Aid <= 0 {
			continue
		}
		kingdomID := slot.Info.MapKingdomID
		if kingdomID == 0 && slot.Info.Troops.KingdomID != 0 {
			kingdomID = slot.Info.Troops.KingdomID
		}
		castles = append(castles, stateCastleSummary{
			Key:       slot.Key,
			Type:      slot.Type,
			CastleID:  int(slot.Info.Aid),
			Name:      slot.Info.Name,
			KingdomID: kingdomID,
			MapX:      slot.Info.MapX,
			MapY:      slot.Info.MapY,
			Resources: slot.Info.Amount,
		})
	}
	return map[string]interface{}{
		"capturedAtUnixMs": capturedAt,
		"connection":       ResponseRegistry.GetGameConnectionStatus(),
		"player": map[string]interface{}{
			"id":   gameState.PlayerID,
			"name": gameState.PlayerName,
		},
		"focus":           gameState.CastleFocus,
		"globalResources": gameState.GlobalResources,
		"castles":         castles,
		"movement":        Models.MovementUpdatePayload(),
		"features":        featureStatuses(),
		"automation": map[string]interface{}{
			"coordinator":   Automation.Snapshot(),
			"commandQueues": automationCommandQueueSummary(),
		},
	}
}

func automationCommandQueueSummary() map[Automation.Lane]map[string]interface{} {
	snapshot := Automation.Commands.Snapshot()
	summary := make(map[Automation.Lane]map[string]interface{}, len(snapshot))
	for lane, commands := range snapshot {
		byOwner := make(map[string]int)
		for _, command := range commands {
			byOwner[command.Owner]++
		}
		summary[lane] = map[string]interface{}{
			"count":   len(commands),
			"byOwner": byOwner,
		}
	}
	return summary
}

func castleResourceSnapshots() map[int]Models.CastleResourcesAmount {
	resources := make(map[int]Models.CastleResourcesAmount)
	for _, slot := range liveCastleSlots() {
		if slot.Info != nil && slot.Info.Aid > 0 {
			resources[int(slot.Info.Aid)] = slot.Info.Amount
		}
	}
	return resources
}

func constructionItemsByCastle() map[int]interface{} {
	items := make(map[int]interface{})
	for _, slot := range liveCastleSlots() {
		if slot.Info != nil && slot.Info.Aid > 0 {
			items[int(slot.Info.Aid)] = slot.Info.ConstructionByBuilding
		}
	}
	return items
}

func readMapState(capturedAt int64, input stateReadInput) (interface{}, error) {
	if input.KingdomID == nil {
		return nil, toolError("invalid_arguments", "kingdomId is required for scope=map")
	}
	if len(input.Coordinates) == 0 {
		return nil, toolError("invalid_arguments", "coordinates are required for scope=map")
	}
	if len(input.Coordinates) > 100 {
		return nil, toolError("invalid_arguments", "at most 100 map coordinates may be read at once")
	}
	coordinates := make([][2]int, 0, len(input.Coordinates))
	for _, coordinate := range input.Coordinates {
		coordinates = append(coordinates, [2]int{coordinate.X, coordinate.Y})
	}
	nodes := mapstate.GetMapState().NodesAt(*input.KingdomID, coordinates)
	return map[string]interface{}{
		"capturedAtUnixMs": capturedAt,
		"kingdomId":        *input.KingdomID,
		"nodes":            nodes,
	}, nil
}
