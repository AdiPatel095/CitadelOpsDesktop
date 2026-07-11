package Toolkit

import (
	"fmt"
	"strings"

	"CitadelDesktop/Server/Models"
)

type contextCastleSelector struct {
	ID      int    `json:"id,omitempty"`
	Key     string `json:"key,omitempty"`
	Name    string `json:"name,omitempty"`
	Focused bool   `json:"focused,omitempty"`
}

func contextCastleSelectorSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          description,
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"id":      schemaProperty("integer", "Castle instance AID."),
			"key":     schemaProperty("string", "Citadel castle key such as mainCastle, outpost1, iceCastle, or beriWorldCastle."),
			"name":    schemaProperty("string", "Exact castle display name, matched case-insensitively."),
			"focused": schemaProperty("boolean", "Resolve the currently focused player castle."),
		},
	}
}

func resolveContextCastle(selector contextCastleSelector) (ContextCastleReference, []ContextResolution, error) {
	selectionCount := 0
	if selector.ID > 0 {
		selectionCount++
	}
	if strings.TrimSpace(selector.Key) != "" {
		selectionCount++
	}
	if strings.TrimSpace(selector.Name) != "" {
		selectionCount++
	}
	if selector.Focused {
		selectionCount++
	}
	if selectionCount != 1 {
		return ContextCastleReference{}, nil, toolError("invalid_arguments", "castle selector must set exactly one of id, key, name, or focused")
	}

	gameState := Models.GetGameState()
	selectedID := selector.ID
	selectorSource := "input.id"
	if selector.Focused {
		selectedID = gameState.CastleFocus.CastleAID
		selectorSource = "state.castleFocus"
		if selectedID <= 0 {
			return ContextCastleReference{}, nil, toolError("context_unavailable", "no player castle is currently focused")
		}
	}

	var matches []castleSlot
	key := normalizeCastleSelectorValue(selector.Key)
	name := strings.TrimSpace(selector.Name)
	for _, slot := range liveCastleSlots() {
		if slot.Info == nil || slot.Info.Aid <= 0 {
			continue
		}
		switch {
		case selectedID > 0 && int(slot.Info.Aid) == selectedID:
			matches = append(matches, slot)
		case key != "" && normalizeCastleSelectorValue(slot.Key) == key:
			matches = append(matches, slot)
		case name != "" && strings.EqualFold(strings.TrimSpace(slot.Info.Name), name):
			matches = append(matches, slot)
		}
	}
	if len(matches) == 0 {
		return ContextCastleReference{}, nil, toolError("not_found", "castle selector did not match a live player castle")
	}
	if len(matches) > 1 {
		return ContextCastleReference{}, nil, toolError("ambiguous_context", "castle selector matched %d castles; use id or key", len(matches))
	}
	selected := matches[0]
	if selector.ID == 0 && !selector.Focused {
		if key != "" {
			selectorSource = "input.key"
		} else {
			selectorSource = "input.name"
		}
	}

	castleID := int(selected.Info.Aid)
	kingdomID := defaultKingdomForCastleType(selected.Type)
	if selected.Info.MapKingdomID != 0 {
		kingdomID = selected.Info.MapKingdomID
	} else if selected.Info.Troops.KingdomID != 0 {
		kingdomID = selected.Info.Troops.KingdomID
	}
	mapX, mapY := selected.Info.MapX, selected.Info.MapY
	coordinateSource := "state.castle.map"
	if mapX == 0 && mapY == 0 {
		mapX, mapY = selected.Info.Troops.X, selected.Info.Troops.Y
		coordinateSource = "state.castle.troops"
	}
	if mapX == 0 && mapY == 0 {
		if x, y, ok := gameState.ResolveCastleMapCoords(castleID, kingdomID); ok {
			mapX, mapY = x, y
			coordinateSource = "state.alliance.locations"
		}
	}
	if contextCastleFocusUsesMap(kingdomID) && mapX == 0 && mapY == 0 {
		return ContextCastleReference{}, nil, toolError("context_unavailable", "castle %d needs map coordinates for kingdom %d, but none are loaded", castleID, kingdomID)
	}
	castleName := strings.TrimSpace(selected.Info.Name)
	if castleName == "" {
		castleName = fmt.Sprintf("Castle %d", castleID)
	}
	reference := ContextCastleReference{
		Key:       selected.Key,
		Type:      selected.Type,
		CastleID:  castleID,
		Name:      castleName,
		KingdomID: kingdomID,
		MapX:      mapX,
		MapY:      mapY,
	}
	resolutions := []ContextResolution{
		{Field: "castle", Value: reference, Source: selectorSource, Detail: "Resolved against live player castle slots"},
		{Field: "castle.kingdomId", Value: kingdomID, Source: "state.castle", Detail: "Map kingdom used by focus and command payloads"},
	}
	if mapX != 0 || mapY != 0 {
		resolutions = append(resolutions, ContextResolution{
			Field:  "castle.coordinates",
			Value:  map[string]int{"x": mapX, "y": mapY},
			Source: coordinateSource,
		})
	}
	return reference, resolutions, nil
}

func normalizeCastleSelectorValue(value string) string {
	return strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(value)))
}

func defaultKingdomForCastleType(castleType string) int {
	switch castleType {
	case "ice":
		return 2
	case "desert":
		return 1
	case "dungeon":
		return 3
	case "storm":
		return 4
	case "beri_world":
		return 10
	default:
		return 0
	}
}

func contextCastleFocusUsesMap(kingdomID int) bool {
	return kingdomID == 0 || kingdomID == 4 || kingdomID == 10
}
