package App

import (
	"fmt"
	"strings"
	"time"

	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

// attackTargetRejectedError refuses a target the game rejected with ABI/CRA 95
// (CIT-13). It is deliberately not Intent.ErrPlanStale: replanning the same
// target cannot succeed, so the policy must choose another target.
func attackTargetRejectedError(rejection State.AttackTargetRejection) error {
	return Localization.WithError(fmt.Errorf("%s", rejection.Detail()), Localization.New(
		"server.app.deferred_target_p_p.0fa69e17", "Deferred: target {p0}:{p1} rejected by {p2} {p3} until {p4}",
		Localization.Params{
			"p0": rejection.X, "p1": rejection.Y, "p2": strings.ToUpper(rejection.Opcode),
			"p3": rejection.Code, "p4": rejection.Until.UTC().Format(time.RFC3339),
		},
	))
}

func refuseRejectedAttackTarget(gameState State.GameState, kingdomID State.KingdomID, typeID, x, y int, now time.Time) error {
	if rejection, rejected := State.AttackTargetRejectedAt(&gameState, kingdomID, typeID, x, y, now); rejected {
		return attackTargetRejectedError(rejection)
	}
	return nil
}
