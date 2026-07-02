package settings

import (
	"encoding/json"
	"sort"
	"time"
)

const (
	FeatureScheduleDayMinutes = 24 * 60
	FeatureScheduleMinSlot    = 15
)

// FeatureScheduleSlot is one weekly allowed runtime window.
// Day follows time.Weekday: Sunday=0 through Saturday=6.
type FeatureScheduleSlot struct {
	ID          string                 `json:"id,omitempty"`
	Day         int                    `json:"day"`
	StartMinute int                    `json:"startMinute"`
	EndMinute   int                    `json:"endMinute"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// FeatureSchedule is an opt-in weekly runtime limit for one automation feature.
type FeatureSchedule struct {
	Enabled            bool                  `json:"enabled"`
	TimeZone           string                `json:"timeZone,omitempty"`
	SlotOptionsEnabled bool                  `json:"slotOptionsEnabled,omitempty"`
	Slots              []FeatureScheduleSlot `json:"slots"`
}

func clampScheduleMinute(v int) int {
	if v < 0 {
		return 0
	}
	if v > FeatureScheduleDayMinutes {
		return FeatureScheduleDayMinutes
	}
	return v
}

func normalizeScheduleSlot(slot FeatureScheduleSlot) (FeatureScheduleSlot, bool) {
	if slot.Day < 0 {
		slot.Day = 0
	}
	if slot.Day > 6 {
		slot.Day = 6
	}
	slot.StartMinute = clampScheduleMinute(slot.StartMinute)
	slot.EndMinute = clampScheduleMinute(slot.EndMinute)
	if slot.EndMinute-slot.StartMinute < FeatureScheduleMinSlot {
		return FeatureScheduleSlot{}, false
	}
	slot.Options = normalizeScheduleSlotOptions(slot.Options)
	return slot, true
}

func normalizeScheduleSlotOptions(options map[string]interface{}) map[string]interface{} {
	if len(options) == 0 {
		return nil
	}

	normalized := make(map[string]interface{}, len(options))
	for key, value := range options {
		if key == "" {
			continue
		}
		data, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var normalizedValue interface{}
		if err := json.Unmarshal(data, &normalizedValue); err != nil {
			continue
		}
		normalized[key] = normalizedValue
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// NormalizeFeatureSchedule clamps days/minutes and removes invalid slots.
func NormalizeFeatureSchedule(schedule FeatureSchedule) FeatureSchedule {
	if schedule.Slots == nil {
		schedule.Slots = []FeatureScheduleSlot{}
		return schedule
	}
	slots := make([]FeatureScheduleSlot, 0, len(schedule.Slots))
	for _, slot := range schedule.Slots {
		normalized, ok := normalizeScheduleSlot(slot)
		if ok {
			slots = append(slots, normalized)
		}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].Day != slots[j].Day {
			return slots[i].Day < slots[j].Day
		}
		if slots[i].StartMinute != slots[j].StartMinute {
			return slots[i].StartMinute < slots[j].StartMinute
		}
		return slots[i].EndMinute < slots[j].EndMinute
	})
	schedule.Slots = slots
	return schedule
}

// NormalizeFeatureSchedules normalizes every feature schedule and returns a non-nil map.
func NormalizeFeatureSchedules(schedules map[string]FeatureSchedule) map[string]FeatureSchedule {
	if schedules == nil {
		return map[string]FeatureSchedule{}
	}
	normalized := make(map[string]FeatureSchedule, len(schedules))
	for featureID, schedule := range schedules {
		if featureID == "" {
			continue
		}
		normalized[featureID] = NormalizeFeatureSchedule(schedule)
	}
	return normalized
}

// Allows reports whether now falls inside the schedule. Disabled schedules do not restrict runtime.
func (schedule FeatureSchedule) Allows(now time.Time) bool {
	schedule = NormalizeFeatureSchedule(schedule)
	if !schedule.Enabled {
		return true
	}
	_, ok := schedule.ActiveSlot(now)
	return ok
}

// ActiveSlot returns the matching slot for now when the schedule is enabled.
func (schedule FeatureSchedule) ActiveSlot(now time.Time) (FeatureScheduleSlot, bool) {
	schedule = NormalizeFeatureSchedule(schedule)
	if !schedule.Enabled {
		return FeatureScheduleSlot{}, false
	}
	if schedule.TimeZone != "" {
		if loc, err := time.LoadLocation(schedule.TimeZone); err == nil {
			now = now.In(loc)
		}
	}
	day := int(now.Weekday())
	minute := now.Hour()*60 + now.Minute()
	for _, slot := range schedule.Slots {
		if slot.Day == day && minute >= slot.StartMinute && minute < slot.EndMinute {
			return slot, true
		}
	}
	return FeatureScheduleSlot{}, false
}

// NextStartAfter returns the next slot start after now for an enabled schedule.
func (schedule FeatureSchedule) NextStartAfter(now time.Time) (time.Time, bool) {
	schedule = NormalizeFeatureSchedule(schedule)
	if !schedule.Enabled || len(schedule.Slots) == 0 {
		return time.Time{}, false
	}
	loc := now.Location()
	if schedule.TimeZone != "" {
		if loaded, err := time.LoadLocation(schedule.TimeZone); err == nil {
			loc = loaded
		}
	}
	localNow := now.In(loc)
	for dayOffset := 0; dayOffset <= 7; dayOffset++ {
		day := localNow.AddDate(0, 0, dayOffset)
		weekday := int(day.Weekday())
		midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
		for _, slot := range schedule.Slots {
			if slot.Day != weekday {
				continue
			}
			start := midnight.Add(time.Duration(slot.StartMinute) * time.Minute)
			if !start.Before(localNow) {
				return start, true
			}
		}
	}
	return time.Time{}, false
}

// FeatureScheduleAllows reports whether a feature may run now. Missing schedules do not restrict runtime.
func (s *SettingsState) FeatureScheduleAllows(featureID string, now time.Time) bool {
	if s == nil || featureID == "" || s.FeatureSchedules == nil {
		return true
	}
	schedule, ok := s.FeatureSchedules[featureID]
	if !ok {
		return true
	}
	return schedule.Allows(now)
}

// NextFeatureScheduleStart returns the next active slot start for an enabled feature schedule.
func (s *SettingsState) NextFeatureScheduleStart(featureID string, now time.Time) (time.Time, bool) {
	if s == nil || featureID == "" || s.FeatureSchedules == nil {
		return time.Time{}, false
	}
	schedule, ok := s.FeatureSchedules[featureID]
	if !ok {
		return time.Time{}, false
	}
	return schedule.NextStartAfter(now)
}

// ActiveFeatureScheduleSlot returns the current scheduled slot for a feature.
func (s *SettingsState) ActiveFeatureScheduleSlot(featureID string, now time.Time) (FeatureScheduleSlot, bool) {
	if s == nil || featureID == "" || s.FeatureSchedules == nil {
		return FeatureScheduleSlot{}, false
	}
	schedule, ok := s.FeatureSchedules[featureID]
	if !ok {
		return FeatureScheduleSlot{}, false
	}
	return schedule.ActiveSlot(now)
}
