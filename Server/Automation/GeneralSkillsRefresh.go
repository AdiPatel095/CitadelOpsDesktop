package Automation

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/Intent"
)

// generalSkillsRefreshDecision converts an attack-capacity failure caused by
// an unobserved general skill set into the decision that fixes it: a
// "general.skills.refresh" request (C2S "gie").
//
// Attack policies resolve capacity inside Evaluate, before any plan exists;
// the only step that ever observed general skills lived inside the attack
// plan those policies refused to emit. A commander with a general assigned
// therefore waited forever ("Cannot calculate … requirements: general N
// skills have not been observed") on any runtime whose state started without
// the roster — the game never volunteers it. Emitting the refresh breaks the
// deadlock; the next evaluation resolves normally.
func generalSkillsRefreshDecision(err error, now time.Time, metrics map[string]float64) (Decision, bool) {
	var unobserved *AttackCapacity.GeneralSkillsUnobservedError
	if !errors.As(err, &unobserved) {
		return Decision{}, false
	}
	return Decision{
		Status: "ready",
		Detail: fmt.Sprintf(
			"Refresh general %d skills for commander %d before calculating attack capacity",
			unobserved.GeneralID, unobserved.CommanderID,
		),
		NextCheckAt:         now.Add(2 * time.Second),
		Metrics:             metrics,
		Request:             &Intent.Request{Name: "general.skills.refresh", Arguments: json.RawMessage(`{}`)},
		ReevaluateOnSuccess: true,
	}, true
}
