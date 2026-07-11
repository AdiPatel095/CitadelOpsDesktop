package equipmentview

import (
	"context"
	"time"

	"CitadelDesktop/Server/Automation"
)

func acquireEquipmentControl(reason string, reservesResources bool, maxHold time.Duration) (*Automation.Lease, context.CancelFunc, bool) {
	if maxHold <= 0 {
		maxHold = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), maxHold)
	claims := []Automation.Claim{Automation.ExclusiveClaim(Automation.ClaimEquipment)}
	if reservesResources {
		claims = append(claims, Automation.ExclusiveClaim(Automation.ClaimAccountResources))
	}
	lease, ok := Automation.Acquire(ctx, Automation.Request{
		Owner:        Automation.OwnerManual,
		Priority:     Automation.PriorityManual,
		Reason:       reason,
		Claims:       claims,
		MaxHold:      maxHold,
		PreemptLower: true,
	})
	if !ok {
		cancel()
		return nil, func() {}, false
	}
	return lease, cancel, true
}
