package Outbound

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrAutomationLocked = errors.New("background operation paused by the bot lock")

type Lane string

const (
	LaneCommand      Lane = "command"
	LaneAttackLaunch Lane = "attack-launch"
)

type Priority int

const (
	PriorityBackground  Priority = 10
	PriorityService     Priority = 20
	PriorityAutoBeri    Priority = 30
	PriorityAutoKhan    Priority = 35
	PriorityAutoTool    Priority = 40
	PriorityRecruit     Priority = 40
	PriorityAutoSceat   Priority = 45
	PriorityHospital    Priority = 50
	PriorityAutoTCI     Priority = 70
	PriorityAutoBird    Priority = 80
	PriorityAutoStation Priority = 90
	PriorityScheduled   Priority = 95
	PriorityInteractive Priority = 100
)

const (
	MinimumPriority       Priority = 1
	MaximumPriority       Priority = PriorityInteractive
	priorityAgingInterval          = time.Second
)

// AgedPriority raises long-waiting work one priority point per interval. The
// cap preserves the interactive ceiling while eventually allowing the oldest
// conflicting work to win the FIFO tie instead of starving indefinitely.
func AgedPriority(priority Priority, enqueuedAt time.Time, now time.Time) Priority {
	if priority < MinimumPriority {
		priority = MinimumPriority
	}
	if priority >= MaximumPriority || enqueuedAt.IsZero() || !now.After(enqueuedAt) {
		return min(priority, MaximumPriority)
	}
	bonus := Priority(now.Sub(enqueuedAt) / priorityAgingInterval)
	if bonus >= MaximumPriority-priority {
		return MaximumPriority
	}
	return priority + bonus
}

type Metadata struct {
	OperationID           string
	Intent                string
	Actor                 string
	SubmittedAt           time.Time
	Priority              Priority
	Deadline              time.Time
	ConnectionGeneration  uint64
	ResponseToken         string
	ResponseOpcodes       []string
	ResponseTimeoutMillis int
}

type metadataContextKey struct{}

func ResolvePriority(actor string, requested Priority) (Priority, error) {
	if requested != 0 {
		if requested < MinimumPriority || requested > MaximumPriority {
			return 0, fmt.Errorf("priority must be between %d and %d", MinimumPriority, MaximumPriority)
		}
		return requested, nil
	}
	return DefaultPriority(actor), nil
}

func DefaultPriority(actor string) Priority {
	actor = strings.ToLower(strings.TrimSpace(actor))
	switch {
	case strings.HasPrefix(actor, "scheduler"):
		return PriorityScheduled
	case actor == "report-manager":
		return PriorityService
	case strings.HasPrefix(actor, "automation:"):
		policy := strings.TrimPrefix(actor, "automation:")
		switch policy {
		case "autostation":
			return PriorityAutoStation
		case "autotci":
			return PriorityAutoTCI
		case "autobird":
			return PriorityAutoBird
		case "autohospital":
			return PriorityHospital
		case "autosceatres":
			return PriorityAutoSceat
		case "autorecruit":
			return PriorityRecruit
		case "autotool":
			return PriorityAutoTool
		case "autoberiworld":
			return PriorityAutoBeri
		case "autokhan":
			return PriorityAutoKhan
		default:
			return PriorityBackground
		}
	default:
		return PriorityInteractive
	}
}

func YieldsToAutomationLock(actor string) bool {
	actor = strings.ToLower(strings.TrimSpace(actor))
	return strings.HasPrefix(actor, "automation:") || strings.HasPrefix(actor, "scheduler:") ||
		strings.HasPrefix(actor, "background:")
}

func YieldsToDirectTraffic(actor string) bool {
	actor = strings.ToLower(strings.TrimSpace(actor))
	return YieldsToAutomationLock(actor) && actor != "automation:autostation"
}

func WithMetadata(ctx context.Context, metadata Metadata) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if metadata.Priority < MinimumPriority || metadata.Priority > MaximumPriority {
		metadata.Priority = DefaultPriority(metadata.Actor)
	}
	if deadline, ok := ctx.Deadline(); ok && (metadata.Deadline.IsZero() || deadline.Before(metadata.Deadline)) {
		metadata.Deadline = deadline
	}
	return context.WithValue(ctx, metadataContextKey{}, metadata)
}

func MetadataFromContext(ctx context.Context) Metadata {
	if ctx == nil {
		return Metadata{Priority: PriorityInteractive}
	}
	metadata, _ := ctx.Value(metadataContextKey{}).(Metadata)
	if metadata.Priority < MinimumPriority || metadata.Priority > MaximumPriority {
		metadata.Priority = DefaultPriority(metadata.Actor)
	}
	if deadline, ok := ctx.Deadline(); ok && (metadata.Deadline.IsZero() || deadline.Before(metadata.Deadline)) {
		metadata.Deadline = deadline
	}
	return metadata
}
