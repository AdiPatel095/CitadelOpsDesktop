package State

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

type ScopeKind string

const (
	ScopeApplication ScopeKind = "application"
	ScopeSession     ScopeKind = "session"
	ScopeAccount     ScopeKind = "account"
	ScopeKingdom     ScopeKind = "kingdom"
	ScopeCastle      ScopeKind = "castle"
)

const (
	CapabilityProtocol              = "protocol"
	CapabilitySession               = "session"
	CapabilitySessionContext        = "session-context"
	CapabilityAccountProfile        = "account-profile"
	CapabilityAccountWallet         = "account-wallet"
	CapabilityCastleDirectory       = "castle-directory"
	CapabilityBuildings             = "buildings"
	CapabilityBuildingQueue         = "building-queue"
	CapabilityConstruction          = "construction"
	CapabilityConstructionItems     = "construction-items"
	CapabilityConstructionInventory = "construction-inventory"
	CapabilityConstructionCommerce  = "construction-commerce"
	CapabilityInventory             = "inventory"
	CapabilityEconomy               = "economy"
	CapabilityGarrison              = "garrison"
	CapabilityDefense               = "defense"
	CapabilityProduction            = "production"
	CapabilityCrafting              = "crafting"
	CapabilityMovement              = "movement"
	CapabilityAttacks               = "attacks"
	CapabilityLogistics             = "logistics"
	CapabilityLeaders               = "leaders"
	CapabilityEquipment             = "equipment"
	CapabilityAlliance              = "alliance"
	CapabilityWorldMap              = "world-map"
	CapabilityEvents                = "events"
	CapabilityReports               = "reports"
	CapabilityAutomation            = "automation"
)

type ScopeKey struct {
	Kind                 ScopeKind `json:"kind"`
	World                string    `json:"world,omitempty"`
	PlayerID             PlayerID  `json:"playerId,omitempty"`
	KingdomID            KingdomID `json:"kingdomId,omitempty"`
	CastleID             CastleID  `json:"castleId,omitempty"`
	SessionGeneration    uint64    `json:"sessionGeneration,omitempty"`
	ConnectionGeneration uint64    `json:"connectionGeneration,omitempty"`
}

func ApplicationScope() ScopeKey {
	return ScopeKey{Kind: ScopeApplication}
}

func SessionScope(state GameState) ScopeKey {
	return ScopeKey{
		Kind: ScopeSession, World: strings.TrimSpace(state.Session.ServerURL), PlayerID: state.Player.ID,
		SessionGeneration: state.Session.Generation, ConnectionGeneration: state.Session.ConnectionGeneration,
	}
}

func AccountScope(state GameState) ScopeKey {
	worldID, playerID := BoundAccount(state)
	return ScopeKey{Kind: ScopeAccount, World: worldID, PlayerID: playerID}
}

func KingdomScope(state GameState, kingdomID KingdomID) ScopeKey {
	worldID, playerID := BoundAccount(state)
	return ScopeKey{
		Kind: ScopeKingdom, World: worldID, PlayerID: playerID,
		KingdomID: kingdomID,
	}
}

func CastleScope(state GameState, castleID CastleID) ScopeKey {
	worldID, playerID := BoundAccount(state)
	kingdomID := KingdomID(0)
	if castle, found := state.Castles[castleID]; found {
		kingdomID = castle.KingdomID
	}
	return ScopeKey{
		Kind: ScopeCastle, World: worldID, PlayerID: playerID,
		KingdomID: kingdomID, CastleID: castleID,
	}
}

func BoundAccount(state GameState) (string, PlayerID) {
	worldID := strings.TrimSpace(state.Account.WorldID)
	if worldID == "" {
		worldID = strings.TrimSpace(state.Session.ServerURL)
	}
	playerID := state.Account.PlayerID
	if playerID == 0 {
		playerID = state.Player.ID
	}
	return worldID, playerID
}

func (scope ScopeKey) Canonical() string {
	world := url.QueryEscape(strings.TrimSpace(scope.World))
	switch scope.Kind {
	case ScopeApplication:
		return string(ScopeApplication)
	case ScopeSession:
		return string(ScopeSession)
	case ScopeKingdom:
		return fmt.Sprintf("kingdom:%s:%d:%d", world, scope.PlayerID, scope.KingdomID)
	case ScopeCastle:
		return fmt.Sprintf("castle:%s:%d:%d:%d", world, scope.PlayerID, scope.KingdomID, scope.CastleID)
	default:
		return fmt.Sprintf("account:%s:%d", world, scope.PlayerID)
	}
}

type PartitionKey struct {
	Capability string   `json:"capability"`
	Scope      ScopeKey `json:"scope"`
}

func ApplicationPartition(capability string) PartitionKey {
	return PartitionKey{Capability: normalizeCapability(capability), Scope: ApplicationScope()}
}

func SessionPartition(state GameState, capability string) PartitionKey {
	return PartitionKey{Capability: normalizeCapability(capability), Scope: SessionScope(state)}
}

func AccountPartition(state GameState, capability string) PartitionKey {
	return PartitionKey{Capability: normalizeCapability(capability), Scope: AccountScope(state)}
}

func KingdomPartition(state GameState, capability string, kingdomID KingdomID) PartitionKey {
	return PartitionKey{Capability: normalizeCapability(capability), Scope: KingdomScope(state, kingdomID)}
}

func CastlePartition(state GameState, capability string, castleID CastleID) PartitionKey {
	return PartitionKey{Capability: normalizeCapability(capability), Scope: CastleScope(state, castleID)}
}

func (key PartitionKey) Canonical() string {
	return normalizeCapability(key.Capability) + "@" + key.Scope.Canonical()
}

type PartitionVersion struct {
	Key       PartitionKey `json:"key"`
	Version   uint64       `json:"version"`
	Revision  uint64       `json:"revision"`
	UpdatedAt time.Time    `json:"updatedAt"`
}

type PartitionDependency struct {
	Key     PartitionKey `json:"key"`
	Version uint64       `json:"version"`
}

type partitionVersionSnapshot struct {
	values map[string]PartitionVersion
}

type PartitionVersions struct {
	snapshot *partitionVersionSnapshot
}

func (versions PartitionVersions) Available() bool {
	return versions.snapshot != nil
}

func (versions PartitionVersions) Version(key PartitionKey) uint64 {
	if versions.snapshot == nil {
		return 0
	}
	return versions.snapshot.values[key.Canonical()].Version
}

func (versions PartitionVersions) Dependencies(keys ...PartitionKey) []PartitionDependency {
	keys = normalizePartitionKeys(keys)
	dependencies := make([]PartitionDependency, 0, len(keys))
	for _, key := range keys {
		dependencies = append(dependencies, PartitionDependency{Key: key, Version: versions.Version(key)})
	}
	return dependencies
}

func (versions PartitionVersions) Current(dependencies []PartitionDependency) bool {
	for _, dependency := range dependencies {
		if versions.Version(dependency.Key) != dependency.Version {
			return false
		}
	}
	return true
}

func (versions PartitionVersions) List() []PartitionVersion {
	if versions.snapshot == nil {
		return []PartitionVersion{}
	}
	out := make([]PartitionVersion, 0, len(versions.snapshot.values))
	for _, version := range versions.snapshot.values {
		out = append(out, version)
	}
	sort.Slice(out, func(left, right int) bool {
		return out[left].Key.Canonical() < out[right].Key.Canonical()
	})
	return out
}

type ProtocolContextState struct {
	SessionGeneration    uint64    `json:"sessionGeneration"`
	ConnectionGeneration uint64    `json:"connectionGeneration"`
	FocusedCastleID      CastleID  `json:"focusedCastleId,omitempty"`
	FocusEpoch           uint64    `json:"focusEpoch"`
	ObservedAt           time.Time `json:"observedAt,omitempty"`
}

type PlanningView struct {
	State           GameState
	Partitions      PartitionVersions
	ProtocolContext ProtocolContextState
}

func CapabilityForDomain(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "protocol":
		return CapabilityProtocol
	case "session":
		return CapabilitySession
	case "session-context", "command-context":
		return CapabilitySessionContext
	case "attack_dialog", "attack-dialog":
		return CapabilityAttacks
	case "player", "achievements", "legend-skills", "subscriptions":
		return CapabilityAccountProfile
	case "currencies":
		return CapabilityAccountWallet
	case "castles":
		return CapabilityCastleDirectory
	case "buildings", "building-layout":
		return CapabilityBuildings
	case "building-queue", "building-construction":
		return CapabilityBuildingQueue
	case "construction-items":
		return CapabilityConstructionItems
	case "construction-offers":
		return CapabilityConstructionCommerce
	case "resources", "market":
		return CapabilityEconomy
	case "units", "beri":
		return CapabilityGarrison
	case "defense", "khan":
		return CapabilityDefense
	case "production":
		return CapabilityProduction
	case "crafting":
		return CapabilityCrafting
	case "movements", "stationing":
		return CapabilityMovement
	case "kingdom-transport":
		return CapabilityLogistics
	case "commanders", "castellans", "generals", "general-skills":
		return CapabilityLeaders
	case "equipment", "gems":
		return CapabilityEquipment
	case "inventory", "storage":
		return CapabilityInventory
	case "alliance", "alliances":
		return CapabilityAlliance
	case "map":
		return CapabilityWorldMap
	case "events", "event-scores", "tower-cooldowns", "tower-queue", "invasion", "storm", "nomad-camps", "rift":
		return CapabilityEvents
	case "reports":
		return CapabilityReports
	case "automations", "scheduled":
		return CapabilityAutomation
	default:
		return normalizeCapability(domain)
	}
}

func defaultPartitionKeys(state GameState, domains []string) []PartitionKey {
	keys := make([]PartitionKey, 0, len(domains)+2)
	for _, domain := range domains {
		capability := CapabilityForDomain(domain)
		if capability == "" {
			continue
		}
		switch capability {
		case CapabilityProtocol, CapabilitySession, CapabilitySessionContext, CapabilityAttacks:
			keys = append(keys, SessionPartition(state, capability))
		default:
			keys = append(keys, AccountPartition(state, capability))
		}
		if capability == CapabilitySession {
			keys = append(keys, SessionPartition(state, CapabilitySessionContext))
		}
		if strings.EqualFold(strings.TrimSpace(domain), "resources") {
			keys = append(keys, AccountPartition(state, CapabilityAccountWallet))
		}
	}
	return normalizePartitionKeys(keys)
}

func normalizePartitionKeys(keys []PartitionKey) []PartitionKey {
	unique := make(map[string]PartitionKey, len(keys))
	for _, key := range keys {
		key.Capability = normalizeCapability(key.Capability)
		if key.Capability == "" || key.Scope.Kind == "" {
			continue
		}
		unique[key.Canonical()] = key
	}
	canonical := make([]string, 0, len(unique))
	for key := range unique {
		canonical = append(canonical, key)
	}
	sort.Strings(canonical)
	out := make([]PartitionKey, 0, len(canonical))
	for _, key := range canonical {
		out = append(out, unique[key])
	}
	return out
}

func normalizeCapability(capability string) string {
	return strings.ToLower(strings.TrimSpace(capability))
}

func advancePartitionVersions(
	current *partitionVersionSnapshot,
	keys []PartitionKey,
	revision uint64,
	updatedAt time.Time,
) (*partitionVersionSnapshot, []PartitionVersion) {
	values := map[string]PartitionVersion{}
	if current != nil {
		values = make(map[string]PartitionVersion, len(current.values)+len(keys))
		for key, version := range current.values {
			values[key] = version
		}
	}
	changed := make([]PartitionVersion, 0, len(keys))
	for _, key := range normalizePartitionKeys(keys) {
		canonical := key.Canonical()
		version := values[canonical]
		version.Key = key
		version.Version++
		version.Revision = revision
		version.UpdatedAt = updatedAt
		values[canonical] = version
		changed = append(changed, version)
	}
	return &partitionVersionSnapshot{values: values}, changed
}

func initialProtocolContext(state GameState) ProtocolContextState {
	context := ProtocolContextState{
		SessionGeneration: state.Session.Generation, ConnectionGeneration: state.Session.ConnectionGeneration,
	}
	for castleID, castle := range state.Castles {
		if castle.Focused {
			context.FocusedCastleID = castleID
			context.FocusEpoch = 1
			context.ObservedAt = state.UpdatedAt
			break
		}
	}
	return context
}

func nextProtocolContext(
	current ProtocolContextState,
	state GameState,
	domains []string,
	partitions []PartitionKey,
	observedAt time.Time,
) ProtocolContextState {
	next := current
	if next.SessionGeneration != state.Session.Generation ||
		next.ConnectionGeneration != state.Session.ConnectionGeneration {
		next.SessionGeneration = state.Session.Generation
		next.ConnectionGeneration = state.Session.ConnectionGeneration
		next.FocusedCastleID = 0
		next.FocusEpoch++
		next.ObservedAt = observedAt
	}
	if !containsDomain(domains, "session-context") &&
		!containsPartitionCapability(partitions, CapabilitySessionContext) {
		return next
	}
	focusedCastleID := CastleID(0)
	for castleID, castle := range state.Castles {
		if castle.Focused {
			focusedCastleID = castleID
			break
		}
	}
	if next.FocusedCastleID != focusedCastleID {
		next.FocusedCastleID = focusedCastleID
		next.FocusEpoch++
	}
	next.SessionGeneration = state.Session.Generation
	next.ConnectionGeneration = state.Session.ConnectionGeneration
	next.ObservedAt = observedAt
	return next
}

func containsPartitionCapability(partitions []PartitionKey, capability string) bool {
	capability = normalizeCapability(capability)
	for _, partition := range partitions {
		if normalizeCapability(partition.Capability) == capability {
			return true
		}
	}
	return false
}

func containsDomain(domains []string, wanted string) bool {
	wanted = strings.ToLower(strings.TrimSpace(wanted))
	for _, domain := range domains {
		if strings.ToLower(strings.TrimSpace(domain)) == wanted {
			return true
		}
	}
	return false
}
