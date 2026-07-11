package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
)

// NotifyRiftMapCoordsChanged is wired by FrontendWebsocket to push riftMapCoords after **gaa**.
var NotifyRiftMapCoordsChanged func()

type GAAResponse struct {
	KID int                `json:"KID"`
	OI  []Models.MapPlayer `json:"OI"`
	AI  [][]interface{}    `json:"AI"`
}

func ParseGAAMessage(payload string) {
	var gaa GAAResponse
	err := json.Unmarshal([]byte(payload), &gaa)
	if err != nil {
		log.Printf("[MapParser] Error unmarshalling GAA payload: %v", err)
		return
	}

	mapState := Models.GetMapState()
	kid := gaa.KID

	// Parse Map Nodes (AI array)
	for _, item := range gaa.AI {
		if len(item) < 3 {
			continue // Avoid out of bounds
		}

		node := Models.MapNode{
			RawData: item,
		}

		length := len(item)
		if length < 3 {
			continue
		}

		// 1st: Type, 2nd: X, 3rd: Y (Constant across all nodes). See MapCastleTypes.go.
		if val, ok := item[0].(float64); ok {
			node.Type = int(val)
		}
		if val, ok := item[1].(float64); ok {
			node.X = int(val)
		}
		if val, ok := item[2].(float64); ok {
			node.Y = int(val)
		}

		switch length {
		case GaaNodeRowLenTerrain:
			// [31,X,Y] empty passable tile — no extra fields.
		case GaaNodeRowLenCastleStub:
			// [1,X,Y,-1] castle anchor without full metadata.
			if val, ok := item[3].(float64); ok {
				node.ID = int(val)
			}
		case GaaNodeRowLenTower: // Kingdom Tower (type GaaNodeKingdomTower)
			if val, ok := item[3].(float64); ok {
				node.ID = int(val)
			}
			if val, ok := item[4].(float64); ok {
				node.Level = int(val)
			}
			if val, ok := item[5].(float64); ok {
				node.CooldownSeconds = int(val)
			}
		case GaaNodeRowLenCooldownMarker: // Cooldown node / coord marker (types 11, 23, …)
			if val, ok := item[5].(float64); ok {
				node.CooldownSeconds = int(val)
			}
			if val, ok := item[6].(string); ok {
				node.LastHitter = val
				node.Name = val
			}
		case GaaNodeRowLenNamedOwner: // Named node (resource, camp, …)
			if val, ok := item[3].(float64); ok {
				node.ID = int(val)
			}
			if val, ok := item[4].(float64); ok {
				node.PlayerID = int(val)
			}
			if val, ok := item[8].(string); ok {
				node.Name = val
			}
		case GaaNodeRowLenMonument:
			if val, ok := item[3].(float64); ok {
				node.CastleID = int(val)
			}
			if val, ok := item[4].(float64); ok {
				node.PlayerID = int(val)
			}
			for _, prop := range item[3:] {
				if strVal, ok := prop.(string); ok && strVal != "" {
					node.Name = strVal
					break
				}
			}
		case GaaNodeRowLenCastle: // Castle rows (types 1, 3, 4, 12, 22, …)
			if val, ok := item[3].(float64); ok {
				node.CastleID = int(val)
			}
			if val, ok := item[4].(float64); ok {
				node.PlayerID = int(val)
			}
			if val, ok := item[5].(float64); ok {
				node.Level = int(val) // Keep level
			}
			if val, ok := item[6].(float64); ok {
				node.WallLevel = int(val)
			}
			if val, ok := item[7].(float64); ok {
				node.GateLevel = int(val)
			}
			if val, ok := item[10].(string); ok {
				node.Name = val // Castle Name
			}
			if val, ok := item[14].(float64); ok {
				node.MoatLevel = int(val)
			}
		default:
			// Fallback for generic/unknown lengths
			if length > 3 {
				if val, ok := item[3].(float64); ok {
					node.ID = int(val)
				}
			}
			// Search for first string as name
			if length > 3 {
				for _, prop := range item[3:] {
					if strVal, ok := prop.(string); ok {
						node.Name = strVal
						break
					}
				}
			}
		}

		mapState.AddNode(kid, node)
	}

	if len(gaa.AI) > 0 && NotifyRiftMapCoordsChanged != nil {
		notify := kid == 0
		if !notify {
			for _, item := range gaa.AI {
				if len(item) < 1 {
					continue
				}
				if t, ok := item[0].(float64); ok && int(t) == GaaNodeRift {
					notify = true
					break
				}
			}
		}
		if notify {
			NotifyRiftMapCoordsChanged()
		}
	}
}
