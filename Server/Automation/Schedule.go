package Automation

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Configuration"
)

type weeklySchedule struct {
	Enabled            bool                 `json:"enabled"`
	TimeZone           string               `json:"timeZone"`
	SlotOptionsEnabled bool                 `json:"slotOptionsEnabled,omitempty"`
	Slots              []weeklyScheduleSlot `json:"slots"`
}

type weeklyScheduleSlot struct {
	Day         int            `json:"day"`
	StartMinute int            `json:"startMinute"`
	EndMinute   int            `json:"endMinute"`
	Options     map[string]any `json:"options,omitempty"`
}

type weeklyScheduleResolution struct {
	Allowed            bool
	Next               time.Time
	SlotOptionsEnabled bool
	Options            map[string]any
	ValidUntil         time.Time
}

// parsedScheduler is the scheduler section decoded once per content: the typed
// weekly schedules (for resolution) and their raw JSON (for fingerprints).
type parsedScheduler struct {
	// valid: every schedule decoded (an invalid document allows everything).
	valid bool
	// outerValid: the document itself decoded, so raw is usable for fingerprints.
	outerValid bool
	schedules  map[string]weeklySchedule
	raw        map[string]json.RawMessage
}

var schedulerMemo sectionMemo[*parsedScheduler]

func parseScheduler(raw json.RawMessage) *parsedScheduler {
	return schedulerMemo.get(raw, func() *parsedScheduler {
		configurationParses.Add(1)
		result := &parsedScheduler{}
		if len(raw) == 0 {
			result.valid, result.outerValid = true, true
			return result
		}
		var document struct {
			FeatureSchedules map[string]json.RawMessage `json:"featureSchedules"`
		}
		if json.Unmarshal(raw, &document) != nil {
			return result
		}
		result.outerValid = true
		result.raw = document.FeatureSchedules
		result.schedules = make(map[string]weeklySchedule, len(document.FeatureSchedules))
		for key, value := range document.FeatureSchedules {
			var schedule weeklySchedule
			if json.Unmarshal(value, &schedule) == nil {
				result.schedules[key] = schedule
			} else {
				// One malformed schedule invalidates the whole document, as decoding it
				// into map[string]weeklySchedule always did.
				return &parsedScheduler{outerValid: true, raw: document.FeatureSchedules}
			}
		}
		result.valid = true
		return result
	})
}

var locationMemo sync.Map // zone name -> *time.Location (nil for unknown names)

func scheduleLocation(zone string) *time.Location {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return time.Local
	}
	if cached, found := locationMemo.Load(zone); found {
		if location, _ := cached.(*time.Location); location != nil {
			return location
		}
		return time.Local
	}
	loaded, err := time.LoadLocation(zone)
	if err != nil {
		locationMemo.Store(zone, (*time.Location)(nil))
		return time.Local
	}
	locationMemo.Store(zone, loaded)
	return loaded
}

func resolveWeeklySchedule(configuration Configuration.Snapshot, featureID string, now time.Time) weeklyScheduleResolution {
	result := weeklyScheduleResolution{Allowed: true}
	raw := configuration.Sections["scheduler"]
	if len(raw) == 0 {
		return result
	}
	document := parseScheduler(raw)
	if !document.valid {
		return result
	}
	schedule, exists := document.schedules[featureID]
	if !exists || !schedule.Enabled {
		return result
	}
	location := scheduleLocation(schedule.TimeZone)
	localNow := now.In(location)
	minute := localNow.Hour()*60 + localNow.Minute()
	day := int(localNow.Weekday())
	for _, slot := range schedule.Slots {
		if validScheduleSlot(slot) && slot.Day == day && minute >= slot.StartMinute && minute < slot.EndMinute {
			result.SlotOptionsEnabled = schedule.SlotOptionsEnabled
			result.Options = slot.Options
			result.ValidUntil = time.Date(
				localNow.Year(), localNow.Month(), localNow.Day(),
				0, slot.EndMinute, 0, 0, location,
			).UTC()
			return result
		}
	}
	result.Allowed = false
	result.SlotOptionsEnabled = schedule.SlotOptionsEnabled
	result.Next = nextScheduleStart(schedule.Slots, localNow)
	return result
}

func scheduleAllows(configuration Configuration.Snapshot, featureID string, now time.Time) (bool, time.Time) {
	resolved := resolveWeeklySchedule(configuration, featureID, now)
	return resolved.Allowed, resolved.Next
}

func nextScheduleStart(slots []weeklyScheduleSlot, localNow time.Time) time.Time {
	valid := make([]weeklyScheduleSlot, 0, len(slots))
	for _, slot := range slots {
		if validScheduleSlot(slot) {
			valid = append(valid, slot)
		}
	}
	sort.Slice(valid, func(left, right int) bool {
		if valid[left].Day != valid[right].Day {
			return valid[left].Day < valid[right].Day
		}
		return valid[left].StartMinute < valid[right].StartMinute
	})
	for offset := 0; offset <= 7; offset++ {
		candidateDay := localNow.AddDate(0, 0, offset)
		weekday := int(candidateDay.Weekday())
		for _, slot := range valid {
			if slot.Day != weekday {
				continue
			}
			start := time.Date(
				candidateDay.Year(), candidateDay.Month(), candidateDay.Day(),
				0, slot.StartMinute, 0, 0, candidateDay.Location(),
			)
			if start.After(localNow) {
				return start.UTC()
			}
		}
	}
	return time.Time{}
}

func validScheduleSlot(slot weeklyScheduleSlot) bool {
	return slot.Day >= 0 && slot.Day <= 6 && slot.StartMinute >= 0 && slot.StartMinute < 1440 &&
		slot.EndMinute > slot.StartMinute && slot.EndMinute <= 1440
}
