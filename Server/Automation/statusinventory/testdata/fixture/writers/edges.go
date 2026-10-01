package writers

import (
	"fixture/automation"
	"fixture/state"
)

type helper struct{}

func (helper) status() string                             { return "method" }
func namedResult() (status string)                        { status = "named-result"; return }
func nestedTuple() (string, string)                       { return pick() }
func escaped(d *automation.Decision, status string)       { d.Status = status }
func neverCalled(d *automation.Decision, status string)   { d.Status = status }
func variadic(d *automation.Decision, statuses ...string) { d.Status = statuses[0] }

type dynamic interface{ status() string }

func edges(d *automation.Decision, cur *state.AutomationState, h dynamic) {
	s := "closure-a"
	change := func() { s = "closure-b" }
	change()
	d.Status = s
	d.Status = string(named)
	d.Status = helper{}.status()
	d.Status = namedResult()
	d.Status, d.Detail = nestedTuple()
	addressed := "address-initial"
	_ = &addressed
	d.Status = addressed
	d.Status += "suffix"
	_ = automation.Decision{"unkeyed", "detail"}
	_ = []automation.Decision{{"implicit-unkeyed", "detail"}}
	for _, cur.Status = range []string{"range"} {
	}
	fn := escaped
	fn(d, "escaped-call")
	variadic(d, "variadic-call")
	d.Status = h.status()
	local := func() string { return "func-value" }
	d.Status = local()
}
