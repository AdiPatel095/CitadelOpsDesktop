package Automation

import (
	"testing"
	"time"
)

func TestStormUnknownEtaBootstrapWindow(t *testing.T) {
	for _, margin := range []time.Duration{time.Second, 0, -time.Second} {
		t.Run(margin.String(), func(t *testing.T) {
			state, data, source, target, now := stormArrivalFixture(t)
			row := state.KingdomTransport.Unlocks[4]
			row.EventEndsAt = now.Add(stormUnknownEtaWindow + margin)
			state.KingdomTransport.Unlocks[4] = row
			block := StormAttackArrivalBlock(&state, data, source, target, -1, now)
			if (block == nil) != (margin > 0) {
				t.Fatalf("unknown ETA margin %s: block=%+v", margin, block)
			}
			// The exception never applies to KUT; its official base time remains required.
			if StormKingdomArrivalBlock(&state, nil, 4, now) == nil {
				t.Fatal("KUT bootstrapped without an official transfer time")
			}
		})
	}
}

func TestStormUnknownEtaFirstReportReplacesBootstrap(t *testing.T) {
	state, data, source, target, now := stormArrivalFixture(t)
	option := int64(-1)
	if StormAttackArrivalBlock(&state, data, source, target, -1, now) != nil {
		t.Fatal("bootstrap blocked with more than twelve hours left")
	}
	addStormArrivalReport(t, &state, source, target.X, 25*60*60, 91007, now.Add(-30*time.Second), now, &option)
	if StormAttackArrivalBlock(&state, data, source, target, -1, now) == nil {
		t.Fatal("bootstrap overrode an observed late arrival")
	}
}
