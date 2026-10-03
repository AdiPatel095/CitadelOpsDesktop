package Automation

import "time"

// Maya's CIT-122 Q1 decision: bootstrap only while more than twelve hours
// remain. The first own TT report replaces this exception with the ETA bound.
const stormUnknownEtaWindow = 12 * time.Hour

func stormUnknownEtaAllowed(now, end time.Time) bool {
	return end.Sub(now) > stormUnknownEtaWindow
}
