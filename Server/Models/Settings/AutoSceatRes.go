package settings

import "strings"

const (
	DefaultAutoSceatResCheckIntervalSec = 300
	MinAutoSceatResCheckIntervalSec     = 30
	MaxAutoSceatResCheckIntervalSec     = 86400

	DefaultAutoSceatResMinimumShipmentSize      = 10000
	DefaultAutoSceatResSourceReservePercent     = 10
	DefaultAutoSceatResOverflowThresholdPercent = 90
	MaxAutoSceatResRecipeRepeat                 = 100
	MaxAutoSceatResRentedQueueSlots             = 3
)

var validAutoSceatResTimeSkips = map[string]bool{
	"MS1": true,
	"MS2": true,
	"MS3": true,
	"MS4": true,
	"MS5": true,
	"MS6": true,
	"MS7": true,
}

// AutoSceatRecipeStep is one entry in a building's repeating recipe cycle.
type AutoSceatRecipeStep struct {
	RecipeID int `json:"recipeID"`
	Repeat   int `json:"repeat"`
}

// AutoSceatBuildingPlan configures one crafting queue (CQID) on one castle.
type AutoSceatBuildingPlan struct {
	Enabled            bool                  `json:"enabled"`
	Steps              []AutoSceatRecipeStep `json:"steps"`
	Cursor             int                   `json:"cursor,omitempty"`
	AutoRentActiveSlot bool                  `json:"autoRentActiveSlot"`
	AutoRentQueueSlots int                   `json:"autoRentQueueSlots"`
}

// AutoSceatCastlePlan holds queue plans by crafting queue id (1 refinery, 2 toolsmith,
// 3 dragon hoard, 4 dragon forge).
type AutoSceatCastlePlan struct {
	Buildings map[int]AutoSceatBuildingPlan `json:"buildings"`
}

// AutoSceatResConfig is the persisted Auto Sceat Resources configuration.
type AutoSceatResConfig struct {
	CheckIntervalSec         int                         `json:"checkIntervalSec"`
	MinimumShipmentSize      int                         `json:"minimumShipmentSize"`
	SourceReservePercent     int                         `json:"sourceReservePercent"`
	OverflowThresholdPercent int                         `json:"overflowThresholdPercent"`
	AutoKingdomTransport     bool                        `json:"autoKingdomTransport"`
	UseKingdomTimeSkips      bool                        `json:"useKingdomTimeSkips"`
	AllowedTimeSkips         []string                    `json:"allowedTimeSkips"`
	TimeSkipReserve          map[string]int              `json:"timeSkipReserve"`
	UseStormBuffer           bool                        `json:"useStormBuffer"`
	AllowRubyRecipes         bool                        `json:"allowRubyRecipes"`
	UseRubyOverflowSkip      bool                        `json:"useRubyOverflowSkip"`
	MinimumCoinReserve       float64                     `json:"minimumCoinReserve"`
	MinimumRubyReserve       float64                     `json:"minimumRubyReserve"`
	Castles                  map[int]AutoSceatCastlePlan `json:"castles"`
}

// DefaultAutoSceatResConfig returns safe defaults. Automation and rentals remain opt-in.
func DefaultAutoSceatResConfig() AutoSceatResConfig {
	return AutoSceatResConfig{
		CheckIntervalSec:         DefaultAutoSceatResCheckIntervalSec,
		MinimumShipmentSize:      DefaultAutoSceatResMinimumShipmentSize,
		SourceReservePercent:     DefaultAutoSceatResSourceReservePercent,
		OverflowThresholdPercent: DefaultAutoSceatResOverflowThresholdPercent,
		AllowedTimeSkips:         []string{"MS5"},
		TimeSkipReserve:          make(map[string]int),
		UseStormBuffer:           true,
		Castles:                  make(map[int]AutoSceatCastlePlan),
	}
}

func clampAutoSceatResInt(value, min, max, fallback int) int {
	if value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// Normalize removes invalid ids, clamps controls, and gives every map a stable JSON shape.
func (c AutoSceatResConfig) Normalize() AutoSceatResConfig {
	cfg := DefaultAutoSceatResConfig()
	cfg.CheckIntervalSec = clampAutoSceatResInt(c.CheckIntervalSec, MinAutoSceatResCheckIntervalSec, MaxAutoSceatResCheckIntervalSec, DefaultAutoSceatResCheckIntervalSec)
	cfg.MinimumShipmentSize = c.MinimumShipmentSize
	if cfg.MinimumShipmentSize < 0 {
		cfg.MinimumShipmentSize = 0
	}
	cfg.SourceReservePercent = c.SourceReservePercent
	if cfg.SourceReservePercent < 0 {
		cfg.SourceReservePercent = 0
	}
	if cfg.SourceReservePercent > 95 {
		cfg.SourceReservePercent = 95
	}
	cfg.OverflowThresholdPercent = clampAutoSceatResInt(c.OverflowThresholdPercent, 50, 100, DefaultAutoSceatResOverflowThresholdPercent)
	cfg.AutoKingdomTransport = c.AutoKingdomTransport
	cfg.UseKingdomTimeSkips = c.UseKingdomTimeSkips
	cfg.UseStormBuffer = c.UseStormBuffer
	cfg.AllowRubyRecipes = c.AllowRubyRecipes
	cfg.UseRubyOverflowSkip = c.UseRubyOverflowSkip
	cfg.MinimumCoinReserve = c.MinimumCoinReserve
	if cfg.MinimumCoinReserve < 0 {
		cfg.MinimumCoinReserve = 0
	}
	cfg.MinimumRubyReserve = c.MinimumRubyReserve
	if cfg.MinimumRubyReserve < 0 {
		cfg.MinimumRubyReserve = 0
	}

	cfg.AllowedTimeSkips = cfg.AllowedTimeSkips[:0]
	seenSkips := make(map[string]bool)
	for _, raw := range c.AllowedTimeSkips {
		id := strings.ToUpper(strings.TrimSpace(raw))
		if !validAutoSceatResTimeSkips[id] || seenSkips[id] {
			continue
		}
		seenSkips[id] = true
		cfg.AllowedTimeSkips = append(cfg.AllowedTimeSkips, id)
	}
	if len(cfg.AllowedTimeSkips) == 0 {
		cfg.AllowedTimeSkips = []string{"MS5"}
	}

	for id, reserve := range c.TimeSkipReserve {
		id = strings.ToUpper(strings.TrimSpace(id))
		if validAutoSceatResTimeSkips[id] && reserve > 0 {
			cfg.TimeSkipReserve[id] = reserve
		}
	}

	for castleID, rawCastle := range c.Castles {
		if castleID <= 0 {
			continue
		}
		castlePlan := AutoSceatCastlePlan{Buildings: make(map[int]AutoSceatBuildingPlan)}
		for queueID, rawBuilding := range rawCastle.Buildings {
			if queueID < 1 || queueID > 4 {
				continue
			}
			building := AutoSceatBuildingPlan{
				Enabled:            rawBuilding.Enabled,
				AutoRentActiveSlot: rawBuilding.AutoRentActiveSlot,
				AutoRentQueueSlots: rawBuilding.AutoRentQueueSlots,
			}
			if building.AutoRentQueueSlots < 0 {
				building.AutoRentQueueSlots = 0
			}
			if building.AutoRentQueueSlots > MaxAutoSceatResRentedQueueSlots {
				building.AutoRentQueueSlots = MaxAutoSceatResRentedQueueSlots
			}
			for _, step := range rawBuilding.Steps {
				if step.RecipeID <= 0 {
					continue
				}
				if step.Repeat < 1 {
					step.Repeat = 1
				}
				if step.Repeat > MaxAutoSceatResRecipeRepeat {
					step.Repeat = MaxAutoSceatResRecipeRepeat
				}
				building.Steps = append(building.Steps, step)
			}
			cycleLength := building.CycleLength()
			if cycleLength > 0 {
				building.Cursor = rawBuilding.Cursor % cycleLength
				if building.Cursor < 0 {
					building.Cursor += cycleLength
				}
			}
			castlePlan.Buildings[queueID] = building
		}
		cfg.Castles[castleID] = castlePlan
	}

	return cfg
}

// CycleLength returns the number of logical queue entries after repeat counts are expanded.
func (p AutoSceatBuildingPlan) CycleLength() int {
	total := 0
	for _, step := range p.Steps {
		if step.RecipeID > 0 && step.Repeat > 0 {
			total += step.Repeat
		}
	}
	return total
}

// RecipeAtCursor returns the next configured recipe without allocating an expanded cycle.
func (p AutoSceatBuildingPlan) RecipeAtCursor() (int, bool) {
	cycleLength := p.CycleLength()
	if cycleLength == 0 {
		return 0, false
	}
	cursor := p.Cursor % cycleLength
	if cursor < 0 {
		cursor += cycleLength
	}
	for _, step := range p.Steps {
		if step.RecipeID <= 0 || step.Repeat <= 0 {
			continue
		}
		if cursor < step.Repeat {
			return step.RecipeID, true
		}
		cursor -= step.Repeat
	}
	return 0, false
}
