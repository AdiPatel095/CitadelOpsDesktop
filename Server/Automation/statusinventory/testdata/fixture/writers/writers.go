package writers

import (
	"fixture/automation"
	"fixture/state"
)

const named = "named"

type unrelated struct{ Status string }

func writes(d *automation.Decision, cur *state.AutomationState, branch bool, m map[string]string, k string) {
	s := "hibernating"
	d.Status = s
	var local string
	if branch {
		local = "branch-a"
	} else {
		local = "branch-b"
	}
	d.Status = local
	cur.Status, cur.Detail = "parallel", "detail"
	cur.Status, cur.Detail = pick()
	set(d, "parameter-a")
	set(d, "parameter-b")
	cur.Status = d.Status
	d.Status = m[k]
	_ = &cur.Status
	_ = automation.Decision{Status: named}
	_ = unrelated{Status: "unrelated"}
}
func pick() (string, string) {
	return "tuple", "detail"
}
func set(d *automation.Decision, status string) { d.Status = status }
