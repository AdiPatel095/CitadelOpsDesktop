package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
)

func TestWeeklyScheduleReturnsNextWindow(t *testing.T) {
	snapshot := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{
			"featureSchedules":{"autoTCI":{"enabled":true,"timeZone":"UTC","slots":[
				{"day":6,"startMinute":780,"endMinute":840}
			]}}
		}`),
	}}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	allowed, next := scheduleAllows(snapshot, "autoTCI", now)
	if allowed {
		t.Fatal("expected schedule to be inactive before its window")
	}
	want := time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next start = %s, want %s", next, want)
	}
	allowed, _ = scheduleAllows(snapshot, "autoTCI", want.Add(10*time.Minute))
	if !allowed {
		t.Fatal("expected schedule to be active inside its window")
	}
}

func TestWeeklyScheduleResolvesActiveSlotOptionsAndExpiry(t *testing.T) {
	snapshot := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{
			"featureSchedules":{"autoRecruit:77":{
				"enabled":true,"timeZone":"America/New_York","slotOptionsEnabled":true,"slots":[
					{"day":3,"startMinute":1200,"endMinute":1260,"options":{"unitID":2069}}
				]
			}}
		}`),
	}}
	now := time.Date(2026, 8, 6, 0, 30, 0, 0, time.UTC)
	resolved := resolveWeeklySchedule(snapshot, "autoRecruit:77", now)
	if !resolved.Allowed || !resolved.SlotOptionsEnabled {
		t.Fatalf("schedule resolution = %#v, want active slot options", resolved)
	}
	if definitionID, valid := productionScheduleDefinitionID(resolved.Options, "unitID"); !valid || definitionID != 2069 {
		t.Fatalf("scheduled unit = %d valid=%t, want 2069", definitionID, valid)
	}
	wantUntil := time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC)
	if !resolved.ValidUntil.Equal(wantUntil) {
		t.Fatalf("schedule validity = %s, want %s", resolved.ValidUntil, wantUntil)
	}
}
