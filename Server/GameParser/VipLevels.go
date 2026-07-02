package GameParser

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
)

var (
	vipRecruitmentBonusSlotsOnce sync.Once
	vipRecruitmentBonusSlotsByID map[int]int
)

var fallbackVIPRecruitmentBonusSlotsByID = map[int]int{
	1:  0,
	2:  0,
	3:  0,
	4:  1,
	5:  1,
	6:  1,
	7:  2,
	8:  2,
	9:  2,
	10: 3,
}

func buildVIPRecruitmentBonusSlotsByID() {
	vipRecruitmentBonusSlotsByID = make(map[int]int, len(fallbackVIPRecruitmentBonusSlotsByID))
	for level, slots := range fallbackVIPRecruitmentBonusSlotsByID {
		vipRecruitmentBonusSlotsByID[level] = slots
	}

	b, err := serverdata.ReadVIPLevelsJSON()
	if err != nil {
		return
	}
	var rows []struct {
		VIPLevelID            string `json:"vipLevelID"`
		RecruitmentBonusSlots string `json:"recruitmentBonusSlots"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, r := range rows {
		level, err := strconv.Atoi(strings.TrimSpace(r.VIPLevelID))
		if err != nil || level <= 0 {
			continue
		}
		slots, _ := strconv.Atoi(strings.TrimSpace(r.RecruitmentBonusSlots))
		vipRecruitmentBonusSlotsByID[level] = slots
	}
}

// VIPRecruitmentBonusSlots returns extra queued recruit slots for a VIP level.
func VIPRecruitmentBonusSlots(level int) (int, bool) {
	if level <= 0 {
		return 0, false
	}
	vipRecruitmentBonusSlotsOnce.Do(buildVIPRecruitmentBonusSlotsByID)
	slots, ok := vipRecruitmentBonusSlotsByID[level]
	return slots, ok
}
