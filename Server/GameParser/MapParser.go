package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
)

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

		// 1st: Type, 2nd: X, 3rd: Y (Constant across all nodes)
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
		case 7: // Kingdom Tower
			if val, ok := item[3].(float64); ok {
				node.ID = int(val)
			}
			if val, ok := item[4].(float64); ok {
				node.Level = int(val)
			}
			if val, ok := item[5].(float64); ok {
				node.CooldownSeconds = int(val)
			}
		case 8: // Node with Cooldown & Last Hitter
			if val, ok := item[5].(float64); ok {
				node.CooldownSeconds = int(val)
			}
			if val, ok := item[6].(string); ok {
				node.LastHitter = val
			}
		case 9: // Named Node with Owner
			if val, ok := item[4].(float64); ok {
				node.PlayerID = int(val)
			}
			if val, ok := item[8].(string); ok {
				node.Name = val
			}
		case 20: // Castle
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
			if val, ok := item[8].(string); ok {
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

	// Optional: Debug print to confirm processing
	// log.Printf("[MapParser] Processed %d map players, %d nodes", len(gaa.OI), len(gaa.AI))
}
