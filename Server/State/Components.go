package State

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Component is a top-level independently copy-on-write part of one account's
// state. A mutation declares its write components before it runs, so Store can
// copy only those parts for validation and atomically retain every untouched
// part from the previous generation.
type Component uint8

const (
	ComponentCatalog Component = iota
	ComponentSession
	ComponentAccount
	ComponentPlayer
	ComponentCastles
	ComponentCommanders
	ComponentGenerals
	ComponentCastellans
	ComponentMovements
	ComponentMovementSnapshot
	ComponentStationing
	ComponentScheduled
	ComponentRift
	ComponentInventory
	ComponentSubscriptions
	ComponentMarket
	ComponentKingdomTransport
	ComponentBeri
	ComponentAlliance
	ComponentAlliances
	ComponentAllianceHelp
	ComponentWorldMap
	ComponentTowerCooldowns
	ComponentTowerQueue
	ComponentInvasion
	ComponentStorm
	ComponentNomadCamps
	ComponentAdvisor
	ComponentKhan
	ComponentDailyAttacks
	ComponentAttackDialog
	ComponentAttackPresets
	ComponentAttackAnalytics
	ComponentEventScores
	ComponentCommandContext
	ComponentAutomations
	ComponentReports
	ComponentObservations
	ComponentCombatCooldown
	componentCount
)

var componentNames = [...]string{
	"catalog",
	"session",
	"account",
	"player",
	"castles",
	"commanders",
	"generals",
	"castellans",
	"movements",
	"movementSnapshot",
	"stationing",
	"scheduled",
	"rift",
	"inventory",
	"subscriptions",
	"market",
	"kingdomTransport",
	"beri",
	"alliance",
	"alliances",
	"allianceHelpRequests",
	"map",
	"towerCooldowns",
	"towerQueue",
	"invasion",
	"storm",
	"nomadCamps",
	"advisor",
	"khan",
	"dailyAttacks",
	"attackDialog",
	"attackPresets",
	"attackAnalytics",
	"eventScores",
	"commandContext",
	"automations",
	"reports",
	"observations",
	"combatCooldown",
}

func (component Component) String() string {
	if component >= componentCount {
		return "unknown"
	}
	return componentNames[component]
}

// MarshalJSON keeps component names stable and human-readable on the wire.
// The numeric representation is only an internal ComponentSet detail.
func (component Component) MarshalJSON() ([]byte, error) {
	if component >= componentCount {
		return nil, fmt.Errorf("unknown state component %d", component)
	}
	return json.Marshal(component.String())
}

func (component *Component) UnmarshalJSON(data []byte) error {
	if component == nil {
		return fmt.Errorf("state component destination is nil")
	}
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("decode state component: %w", err)
	}
	for candidate := Component(0); candidate < componentCount; candidate++ {
		if candidate.String() == name {
			*component = candidate
			return nil
		}
	}
	return fmt.Errorf("unknown state component %q", name)
}

// ComponentSet is a compact write/dirty mask. It is intentionally a value so
// recording it on every revision does not allocate.
type ComponentSet uint64

const (
	AllComponents        ComponentSet = (ComponentSet(1) << componentCount) - 1
	AllAccountComponents              = AllComponents &^ (ComponentSet(1) << ComponentWorldMap)
)

func Components(values ...Component) ComponentSet {
	var set ComponentSet
	for _, component := range values {
		if component < componentCount {
			set |= ComponentSet(1) << component
		}
	}
	return set
}

func (set ComponentSet) Has(component Component) bool {
	return component < componentCount && set&(ComponentSet(1)<<component) != 0
}

func (set ComponentSet) Union(other ComponentSet) ComponentSet {
	return set | other
}

func (set ComponentSet) List() []Component {
	components := make([]Component, 0, componentCount)
	for component := Component(0); component < componentCount; component++ {
		if set.Has(component) {
			components = append(components, component)
		}
	}
	return components
}

func normalizeComponents(components []Component) []Component {
	set := Components(components...)
	out := set.List()
	sort.Slice(out, func(left, right int) bool { return out[left].String() < out[right].String() })
	return out
}
