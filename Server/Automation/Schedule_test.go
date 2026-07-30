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
