package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models/Castle"
)

const hospitalMaxQueueSlots = 5

// HospitalQueueCapacityBreakdown describes the queue-slot count unlocked by the hospital building.
type HospitalQueueCapacityBreakdown struct {
	BuildingOID   int
	BuildingWID   int
	BuildingLevel int
	HospitalSlots int
	Source        string
}

var (
	hospitalSlotsOnce sync.Once
	hospitalSlotsByID map[int]int
)

var fallbackHospitalSlotsByID = map[int]int{
	1: 1, 2: 1, 3: 2, 4: 2, 463: 3, 464: 3, 465: 4, 466: 4, 467: 5, 5: 5, 1940: 5,
	468: 1, 20: 1, 21: 2, 22: 2, 23: 3, 80: 3, 308: 4, 309: 4, 311: 5, 312: 5,
}

func buildHospitalSlotsByID() {
	hospitalSlotsByID = make(map[int]int, len(fallbackHospitalSlotsByID))
	for wid, slots := range fallbackHospitalSlotsByID {
		hospitalSlotsByID[wid] = slots
	}

	b, err := serverdata.ReadBuildingsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		WodID         int    `json:"wodID"`
		Name          string `json:"name"`
		HospitalSlots string `json:"hospitalSlots"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		if r.WodID <= 0 || !strings.Contains(strings.ToLower(strings.TrimSpace(r.Name)), "hospital") {
			continue
		}
		slots, err := strconv.Atoi(strings.TrimSpace(r.HospitalSlots))
		if err != nil || slots <= 0 {
			continue
		}
		if slots > hospitalMaxQueueSlots {
			slots = hospitalMaxQueueSlots
		}
		hospitalSlotsByID[r.WodID] = slots
	}
}

func hospitalSlotsForWID(wid int) (int, bool) {
	if wid <= 0 {
		return 0, false
	}
	hospitalSlotsOnce.Do(buildHospitalSlotsByID)
	slots, ok := hospitalSlotsByID[wid]
	return slots, ok && slots > 0
}

func hospitalSlotsForLevel(level int) int {
	switch {
	case level >= 9:
		return 5
	case level >= 7:
		return 4
	case level >= 5:
		return 3
	case level >= 3:
		return 2
	case level >= 1:
		return 1
	default:
		return 0
	}
}

// HospitalQueueCapacity returns the number of healing queue slots unlocked by a castle's hospital.
func HospitalQueueCapacity(c *castle.PlayerCastleInfo) int {
	return HospitalQueueCapacityDetails(c).HospitalSlots
}

// HospitalQueueCapacityDetails returns the best hospital queue-slot count visible in the castle rows.
func HospitalQueueCapacityDetails(c *castle.PlayerCastleInfo) HospitalQueueCapacityBreakdown {
	if c == nil {
		return HospitalQueueCapacityBreakdown{}
	}
	best := HospitalQueueCapacityBreakdown{}
	for _, row := range c.AllBuildingRows() {
		slots, ok := hospitalSlotsForWID(row.BuildingID)
		source := "building-data"
		if !ok {
			name := strings.ToLower(strings.TrimSpace(row.Name))
			if !strings.Contains(name, "hospital") {
				continue
			}
			slots = hospitalSlotsForLevel(buildingLevel(row))
			source = "building-level"
		}
		if slots > hospitalMaxQueueSlots {
			slots = hospitalMaxQueueSlots
		}
		if slots > best.HospitalSlots {
			best = HospitalQueueCapacityBreakdown{
				BuildingOID:   row.OID,
				BuildingWID:   row.BuildingID,
				BuildingLevel: buildingLevel(row),
				HospitalSlots: slots,
				Source:        source,
			}
		}
	}
	return best
}
