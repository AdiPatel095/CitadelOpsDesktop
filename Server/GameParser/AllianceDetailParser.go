package GameParser

import (
	"CitadelDesktop/Server/Models"
	spyreport "CitadelDesktop/Server/Models/SpyReport"
	"encoding/json"
	"fmt"
)

var NotifyAllianceInfoUpdated func()

// formatDuration converts seconds to a readable string like "2d 1h 23m 45s"
func formatDuration(seconds int) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, secs)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// ParseAllianceInfo processes the ain (Alliance Info) response from the game server
// and updates the Alliance data in GameState, specifically extracting bird locations
// for members with non-zero RPT (bird post time)
func ParseAllianceInfo(data string) {
	var allianceData map[string]interface{}
	err := json.Unmarshal([]byte(data), &allianceData)
	if err != nil {
		return
	}

	gs := Models.GetGameState()

	// Get the "A" wrapper object first
	aWrapper, ok := allianceData["A"].(map[string]interface{})
	if !ok {
		return
	}
	// Alliance Targets can request another alliance's live AIN roster. Keep that
	// response out of the signed-in alliance state used by AutoBird.
	if responseAID := intFromMapAny(aWrapper, "AID"); responseAID > 0 && gs.Alliance.AID > 0 && responseAID != gs.Alliance.AID {
		return
	}

	// Get the "M" (members) array
	membersRaw, ok := aWrapper["M"].([]interface{})
	if !ok {
		return
	}

	// Clear existing roster-derived alliance data
	gs.Alliance.Members = nil
	gs.Alliance.BirdLocations = nil

	// Process each member
	for _, memberRaw := range membersRaw {
		member, ok := memberRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if parsed := allianceMemberFromAIN(member); parsed.PlayerID > 0 || parsed.Name != "" {
			gs.Alliance.Members = append(gs.Alliance.Members, parsed)
		}

		// Check RPT (bird post time) - skip if zero
		rpt, ok := member["RPT"].(float64)
		if !ok || rpt == 0 {
			continue
		}

		// Get AP (castle positions array)
		apRaw, ok := member["AP"].([]interface{})
		if !ok {
			continue
		}

		// Process each castle location for this player
		for _, castleRaw := range apRaw {
			castle, ok := castleRaw.([]interface{})

			// Log array length for debugging
			if ok && len(castle) > 0 {
				// one-time log sample could be useful, but might spam
			}

			if !ok || len(castle) < 4 {
				continue
			}

			kingdomID, ok1 := castle[0].(float64)
			x, ok2 := castle[2].(float64)
			y, ok3 := castle[3].(float64)

			// Get Castle Type (index 4 - 5th element) if available
			castleTypeInt := 0
			if len(castle) > 4 {
				switch v := castle[4].(type) {
				case float64:
					castleTypeInt = int(v)
				case int:
					castleTypeInt = v
				case string:
					fmt.Sscanf(v, "%d", &castleTypeInt)
				}
			}

			if !ok1 || !ok2 || !ok3 {
				continue
			}

			birdLocation := Models.BirdLocation{
				X:          int(x),
				Y:          int(y),
				KingdomID:  int(kingdomID),
				BirdTime:   int(rpt),
				CastleType: castleTypeInt,
			}
			gs.Alliance.BirdLocations = append(gs.Alliance.BirdLocations, birdLocation)
		}
	}

	if NotifyAllianceInfoUpdated != nil {
		go NotifyAllianceInfoUpdated()
	}
	memberIDs := make([]int, 0, len(gs.Alliance.Members))
	for _, member := range gs.Alliance.Members {
		if member.PlayerID > 0 {
			memberIDs = append(memberIDs, member.PlayerID)
		}
	}
	go spyreport.RefreshAllianceSession(gs.PlayerID, gs.Alliance.AID, memberIDs)
}

func allianceMemberFromAIN(member map[string]interface{}) Models.AllianceMember {
	return Models.AllianceMember{
		PlayerID: intFromMapAny(member, "OID", "PID", "ID"),
		Name:     stringFromMapAny(member, "N", "PN", "Name"),
	}
}

func stringFromMapAny(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok {
			return value
		}
	}
	return ""
}

// getKingdomName returns a readable name for the kingdom ID
func getKingdomName(id int) string {
	switch id {
	case 0:
		return "Green Kingdom"
	case 1:
		return "Desert Kingdom"
	case 2:
		return "Ice Kingdom"
	case 3:
		return "Dungeon Kingdom"
	case 4:
		return "Storm Kingdom"
	default:
		return fmt.Sprintf("Kingdom %d", id)
	}
}
