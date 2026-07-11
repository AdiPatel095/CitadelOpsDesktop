// Package Automation coordinates feature work and outbound game commands.
// It deliberately has no dependency on GameFeatures, GameParser, GameCommands,
// or ResponseRegistry so those packages can use it without import cycles.
package Automation

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Priority int

const (
	PriorityBackground  Priority = 10
	PriorityAutoBeri    Priority = 30
	PriorityAutoTool    Priority = 40
	PriorityRecruit     Priority = 40
	PriorityAutoSceat   Priority = 45
	PriorityHospital    Priority = 50
	PriorityAutoBird    Priority = 60
	PriorityAutoTCI     Priority = 70
	PriorityAutoStation Priority = 90
	PriorityManual      Priority = 100
)

const (
	OwnerManual       = "manual"
	OwnerManualFocus  = "manualFocusHold"
	OwnerBackground   = "background"
	OwnerAutoTCI      = "autoTCI"
	OwnerAutoBird     = "autoBird"
	OwnerAutoStation  = "autoStation"
	OwnerAutoHospital = "autoHospital"
	OwnerAutoRecruit  = "autoRecruit"
	OwnerAutoTool     = "autoTool"
	OwnerAutoSceatRes = "autoSceatRes"
	OwnerAutoBeri     = "autoBeriWorld"
	OwnerDecoration   = "decoration"
	OwnerAttack       = "attackScheduler"
	OwnerToolkit      = "toolkit"
)

const (
	ClaimGameFocus        = "game:focus"
	ClaimAccountResources = "account:resources"
	ClaimEquipment        = "account:equipment"
	ClaimTCIInventory     = "account:tci-inventory"
	ClaimTransport        = "account:transport"
	ClaimCrafting         = "account:crafting"
)

type ClaimMode uint8

const (
	ClaimShared ClaimMode = iota + 1
	ClaimExclusive
)

type Claim struct {
	Key  string    `json:"key"`
	Mode ClaimMode `json:"mode"`
}

func SharedClaim(key string) Claim {
	return Claim{Key: strings.TrimSpace(key), Mode: ClaimShared}
}

func ExclusiveClaim(key string) Claim {
	return Claim{Key: strings.TrimSpace(key), Mode: ClaimExclusive}
}

func CastleClaim(castleID int, domain string) Claim {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		domain = "action"
	}
	return ExclusiveClaim(fmt.Sprintf("castle:%d:%s", castleID, domain))
}

func CommanderClaim(commanderID int) Claim {
	return ExclusiveClaim(fmt.Sprintf("commander:%d", commanderID))
}

func MovementClaim(movementID int) Claim {
	return ExclusiveClaim(fmt.Sprintf("movement:%d", movementID))
}

func normalizeClaims(claims []Claim) []Claim {
	byKey := make(map[string]ClaimMode, len(claims))
	for _, claim := range claims {
		key := strings.TrimSpace(claim.Key)
		if key == "" {
			continue
		}
		mode := claim.Mode
		if mode != ClaimShared && mode != ClaimExclusive {
			mode = ClaimExclusive
		}
		if existing, ok := byKey[key]; !ok || mode == ClaimExclusive || existing == 0 {
			byKey[key] = mode
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Claim, 0, len(keys))
	for _, key := range keys {
		out = append(out, Claim{Key: key, Mode: byKey[key]})
	}
	return out
}

func DefaultPriority(owner string) Priority {
	switch owner {
	case OwnerManual, OwnerManualFocus, OwnerDecoration, OwnerToolkit:
		return PriorityManual
	case OwnerAutoStation:
		return PriorityAutoStation
	case OwnerAutoTCI:
		return PriorityAutoTCI
	case OwnerAutoBird:
		return PriorityAutoBird
	case OwnerAutoHospital:
		return PriorityHospital
	case OwnerAutoSceatRes:
		return PriorityAutoSceat
	case OwnerAutoRecruit:
		return PriorityRecruit
	case OwnerAutoTool:
		return PriorityAutoTool
	case OwnerAutoBeri:
		return PriorityAutoBeri
	default:
		return PriorityBackground
	}
}

type Request struct {
	Owner           string
	WorkID          string
	Priority        Priority
	Reason          string
	Claims          []Claim
	Deadline        time.Time
	MaxHold         time.Duration
	PreemptLower    bool
	PreemptEqual    bool
	Protected       bool
	SupersedeOwners []string
}

type Lane string

const (
	LaneCommand      Lane = "command"
	LaneAttackLaunch Lane = "attack-launch"
)

const (
	CommandSurfaceInternalApp = "internal_app"
	CommandSurfaceToolkit     = "toolkit"
	CommandSurfaceRuntime     = "runtime"
)

type Command struct {
	ID            uint64      `json:"id"`
	BrokerID      uint64      `json:"brokerId,omitempty"`
	HarnessID     uint64      `json:"harnessId,omitempty"`
	SubmissionID  uint64      `json:"submissionId,omitempty"`
	FrameIndex    int         `json:"frameIndex,omitempty"`
	WorkID        string      `json:"workId,omitempty"`
	Owner         string      `json:"owner"`
	Builder       string      `json:"builder,omitempty"`
	Intent        string      `json:"intent,omitempty"`
	Surface       string      `json:"surface,omitempty"`
	Effect        string      `json:"effect,omitempty"`
	Opcode        string      `json:"opcode,omitempty"`
	RequestShape  string      `json:"requestShape,omitempty"`
	RequestFields []string    `json:"requestFields,omitempty"`
	Priority      Priority    `json:"priority"`
	Lane          Lane        `json:"lane"`
	Payload       []byte      `json:"-"`
	QueuedAt      time.Time   `json:"queuedAt"`
	NotBefore     time.Time   `json:"notBefore,omitempty"`
	Deadline      time.Time   `json:"deadline,omitempty"`
	CoalesceKey   string      `json:"-"`
	Attempts      int         `json:"attempts,omitempty"`
	Guard         func() bool `json:"-"`
}

type CommandOptions struct {
	WorkID      string
	Owner       string
	Builder     string
	Intent      string
	Surface     string
	Effect      string
	Priority    Priority
	Lane        Lane
	NotBefore   time.Time
	Deadline    time.Time
	CoalesceKey string
	Guard       func() bool
}

func (o CommandOptions) command(payload []byte) Command {
	owner := strings.TrimSpace(o.Owner)
	if owner == "" {
		owner = OwnerManual
	}
	priority := o.Priority
	if priority == 0 {
		priority = DefaultPriority(owner)
	}
	lane := o.Lane
	if lane == "" {
		lane = LaneCommand
	}
	return Command{
		WorkID:      o.WorkID,
		Owner:       owner,
		Builder:     strings.TrimSpace(o.Builder),
		Intent:      strings.TrimSpace(o.Intent),
		Surface:     strings.TrimSpace(o.Surface),
		Effect:      strings.TrimSpace(o.Effect),
		Priority:    priority,
		Lane:        lane,
		Payload:     append([]byte(nil), payload...),
		NotBefore:   o.NotBefore,
		Deadline:    o.Deadline,
		CoalesceKey: o.CoalesceKey,
		Guard:       o.Guard,
	}
}
