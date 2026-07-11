// Package GameFocus is the focus-specific compatibility adapter over the
// generic Automation control plane.
package GameFocus

import (
	"context"
	"time"

	"CitadelDesktop/Server/Automation"
)

type Priority = Automation.Priority

const (
	PriorityBackground  = Automation.PriorityBackground
	PriorityAutoTool    = Automation.PriorityAutoTool
	PriorityRecruit     = Automation.PriorityRecruit
	PriorityHospital    = Automation.PriorityHospital
	PriorityAutoBird    = Automation.PriorityAutoBird
	PriorityAutoTCI     = Automation.PriorityAutoTCI
	PriorityAutoStation = Automation.PriorityAutoStation
	PriorityManual      = Automation.PriorityManual
)

const (
	OwnerManual       = Automation.OwnerManual
	OwnerAutoTCI      = Automation.OwnerAutoTCI
	OwnerAutoBird     = Automation.OwnerAutoBird
	OwnerAutoStation  = Automation.OwnerAutoStation
	OwnerAutoHospital = Automation.OwnerAutoHospital
	OwnerAutoRecruit  = Automation.OwnerAutoRecruit
	OwnerAutoTool     = Automation.OwnerAutoTool
	OwnerDecoration   = Automation.OwnerDecoration
)

const defaultManualIdleHold = 30 * time.Second

type Claim = Automation.Claim
type Lease = Automation.Lease

type Request struct {
	Owner           string
	Priority        Priority
	Reason          string
	MaxHold         time.Duration
	Deadline        time.Time
	Claims          []Claim
	PreemptLower    bool
	SupersedeManual bool
}

func ExclusiveClaim(key string) Claim {
	return Automation.ExclusiveClaim(key)
}

func SharedClaim(key string) Claim {
	return Automation.SharedClaim(key)
}

func CastleClaim(castleID int, domain string) Claim {
	return Automation.CastleClaim(castleID, domain)
}

func Acquire(ctx context.Context, req Request) (*Lease, bool) {
	claims := make([]Claim, 0, len(req.Claims)+1)
	claims = append(claims, Automation.ExclusiveClaim(Automation.ClaimGameFocus))
	claims = append(claims, req.Claims...)
	return Automation.Acquire(ctx, Automation.Request{
		Owner:        req.Owner,
		Priority:     req.Priority,
		Reason:       req.Reason,
		Claims:       claims,
		Deadline:     req.Deadline,
		MaxHold:      req.MaxHold,
		PreemptLower: req.PreemptLower || req.Owner == OwnerAutoStation,
		SupersedeOwners: func() []string {
			if req.SupersedeManual {
				return []string{Automation.OwnerManualFocus}
			}
			return nil
		}(),
	})
}

func RecordManualActivity(reason string, hold time.Duration) {
	if hold <= 0 {
		hold = defaultManualIdleHold
	}
	Automation.Hold(Automation.Request{
		Owner:        Automation.OwnerManualFocus,
		Priority:     PriorityManual,
		Reason:       reason,
		Claims:       []Automation.Claim{Automation.ExclusiveClaim(Automation.ClaimGameFocus)},
		PreemptLower: true,
		PreemptEqual: true,
		Protected:    true,
	}, hold)
}

func DefaultPriority(owner string) Priority {
	return Automation.DefaultPriority(owner)
}
