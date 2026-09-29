package State

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Rejected-target deferral is an internal safety heuristic (CIT-13), not a
// server-derived duration. It is additive: map cooldowns, TowerCooldownState,
// commander holds, daily limits and lane locks still apply independently.
const (
	AttackTargetRejectionBase         = time.Hour
	AttackTargetRejectionCap          = 24 * time.Hour // fortress global cooldown
	AttackTargetRejectionPersonalBase = 6 * time.Hour
	AttackTargetRejectionPersonalCap  = 120 * time.Hour // fortress personal lockout
	AttackTargetRejectionLimit        = 256
)

// AttackTargetRejection records one target the game refused with COOLING_DOWN
// at the attack dialog (ABI 95) or at launch (CRA 95). Count is the number of
// consecutive rejections of the same target; each doubles the deferral.
type AttackTargetRejection struct {
	KingdomID    KingdomID `json:"kingdomId"`
	TargetTypeID int       `json:"targetTypeId"`
	X            int       `json:"x"`
	Y            int       `json:"y"`
	Opcode       string    `json:"opcode"`
	Code         int       `json:"code"`
	OperationID  string    `json:"operationId,omitempty"`
	ObservedAt   time.Time `json:"observedAt"`
	Until        time.Time `json:"until"`
	Count        int       `json:"count"`
	Personal     bool      `json:"personal,omitempty"`
}

// Key identifies the rejected target for policy details.
func (rejection AttackTargetRejection) Key() string {
	return fmt.Sprintf("%d:%d:%d:%d", rejection.KingdomID, rejection.TargetTypeID, rejection.X, rejection.Y)
}

// Detail is the policy/guard explanation for a deferred target.
func (rejection AttackTargetRejection) Detail() string {
	return fmt.Sprintf(
		"Deferred: target %d:%d rejected by %s %d until %s",
		rejection.X, rejection.Y, strings.ToUpper(rejection.Opcode), rejection.Code,
		rejection.Until.UTC().Format(time.RFC3339),
	)
}

// retainedUntil keeps an expired record for one more cap period so that a
// rejection soon after its deferral ends still counts as consecutive.
func (rejection AttackTargetRejection) retainedUntil() time.Time {
	if rejection.Personal {
		return rejection.Until.Add(AttackTargetRejectionPersonalCap)
	}
	return rejection.Until.Add(AttackTargetRejectionCap)
}

func attackTargetRejectionMatches(rejection AttackTargetRejection, kingdomID KingdomID, targetTypeID, x, y int) bool {
	return rejection.KingdomID == kingdomID && rejection.X == x && rejection.Y == y &&
		(targetTypeID <= 0 || rejection.TargetTypeID <= 0 || rejection.TargetTypeID == targetTypeID)
}

// AttackTargetRejectedAt returns the active rejection for the target. A fresh
// map observation never clears it; only its deferral end, a confirmed launch
// or an own victory does.
func AttackTargetRejectedAt(
	gameState GameState,
	kingdomID KingdomID,
	targetTypeID, x, y int,
	now time.Time,
) (AttackTargetRejection, bool) {
	for _, rejection := range gameState.AttackAnalytics.RejectedTargets {
		if attackTargetRejectionMatches(rejection, kingdomID, targetTypeID, x, y) && now.Before(rejection.Until) {
			return rejection, true
		}
	}
	return AttackTargetRejection{}, false
}

// RecordAttackTargetRejection stores or extends a rejection. Deferral is base
// × 2^(Count-1), capped; Personal uses the fortress personal-lockout bounds.
// Expired records are retained for one cap period after Until so that a
// rejection right after the deferral ends still counts as consecutive. The registry is
// pruned and bounded on every write. The same operation is recorded once.
func RecordAttackTargetRejection(gameState *GameState, rejection AttackTargetRejection) (AttackTargetRejection, bool) {
	if gameState == nil || rejection.ObservedAt.IsZero() || rejection.X < 0 || rejection.Y < 0 {
		return AttackTargetRejection{}, false
	}
	rejection.Opcode = strings.ToLower(strings.TrimSpace(rejection.Opcode))
	now := rejection.ObservedAt
	next := make([]AttackTargetRejection, 0, len(gameState.AttackAnalytics.RejectedTargets)+1)
	rejection.Count = 1
	for _, existing := range gameState.AttackAnalytics.RejectedTargets {
		if attackTargetRejectionMatches(existing, rejection.KingdomID, rejection.TargetTypeID, rejection.X, rejection.Y) {
			if rejection.OperationID != "" && existing.OperationID == rejection.OperationID {
				return existing, false
			}
			if !now.Before(existing.retainedUntil()) {
				continue // retention elapsed: no longer consecutive
			}
			rejection.Count = existing.Count + 1
			rejection.Personal = rejection.Personal || existing.Personal
			if rejection.TargetTypeID <= 0 {
				rejection.TargetTypeID = existing.TargetTypeID
			}
			continue
		}
		if now.Before(existing.retainedUntil()) {
			next = append(next, existing)
		}
	}
	base, maximum := AttackTargetRejectionBase, AttackTargetRejectionCap
	if rejection.Personal {
		base, maximum = AttackTargetRejectionPersonalBase, AttackTargetRejectionPersonalCap
	}
	deferral := base
	for step := 1; step < rejection.Count && deferral < maximum; step++ {
		deferral *= 2
	}
	deferral = min(deferral, maximum)
	rejection.Until = rejection.ObservedAt.Add(deferral)
	next = append(next, rejection)
	if len(next) > AttackTargetRejectionLimit {
		sort.SliceStable(next, func(left, right int) bool { return next[left].ObservedAt.Before(next[right].ObservedAt) })
		next = append([]AttackTargetRejection(nil), next[len(next)-AttackTargetRejectionLimit:]...)
	}
	gameState.SetAttackTargetRejections(next)
	return rejection, true
}

// ClearAttackTargetRejection removes the target's record after a confirmed
// launch or an own victory. The consecutive count restarts afterwards.
func ClearAttackTargetRejection(gameState *GameState, kingdomID KingdomID, targetTypeID, x, y int) bool {
	if gameState == nil || len(gameState.AttackAnalytics.RejectedTargets) == 0 {
		return false
	}
	next := make([]AttackTargetRejection, 0, len(gameState.AttackAnalytics.RejectedTargets))
	for _, existing := range gameState.AttackAnalytics.RejectedTargets {
		if !attackTargetRejectionMatches(existing, kingdomID, targetTypeID, x, y) {
			next = append(next, existing)
		}
	}
	if len(next) == len(gameState.AttackAnalytics.RejectedTargets) {
		return false
	}
	gameState.SetAttackTargetRejections(next)
	return true
}
