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
	// PartitionKey is fully comparable. Using it directly avoids formatting and
	// URL-escaping an identity string on every state revision and dependency
	// lookup. The normal account/session capabilities occupy fixed immutable
	// slots; only uncommon kingdom/castle or custom scopes use the fallback map.
	common [32]*PartitionVersion
	extra  map[PartitionKey]*PartitionVersion
	count  int
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
	key = normalizePartitionKey(key)
	if slot, standard := commonPartitionVersionSlot(key); standard {
		version := versions.snapshot.common[slot]
		if version == nil || version.Key != key {
			return 0
		}
		return version.Version
	}
	if version := versions.snapshot.extra[key]; version != nil {
		return version.Version
	}
	return 0
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
	out := make([]PartitionVersion, 0, versions.snapshot.count)
	for _, version := range versions.snapshot.common {
		if version != nil {
			out = append(out, *version)
		}
	}
	for _, version := range versions.snapshot.extra {
		out = append(out, *version)
	}
	sort.Slice(out, func(left, right int) bool { return partitionKeyLess(out[left].Key, out[right].Key) })
	return out
}

type FocusSubcontext string

const (
	FocusSubcontextUnknown FocusSubcontext = ""
	FocusSubcontextCastle  FocusSubcontext = "castle"
	FocusSubcontextMap     FocusSubcontext = "map"
)

type ProtocolContextState struct {
	SessionGeneration           uint64          `json:"sessionGeneration"`
	ConnectionGeneration        uint64          `json:"connectionGeneration"`
	FocusedCastleID             CastleID        `json:"focusedCastleId,omitempty"`
	FocusSubcontext             FocusSubcontext `json:"focusSubcontext,omitempty"`
	FocusEpoch                  uint64          `json:"focusEpoch"`
	RecruitmentBUPCastleID      CastleID        `json:"recruitmentBupCastleId,omitempty"`
	RecruitmentBUPFocusEpoch    uint64          `json:"recruitmentBupFocusEpoch,omitempty"`
	RecruitmentBUPSerial        uint64          `json:"recruitmentBupSerial,omitempty"`
	RecruitmentAHRCoveredSerial uint64          `json:"recruitmentAhrCoveredSerial,omitempty"`
	RecruitmentAHRFocusCovered  bool            `json:"recruitmentAhrFocusCovered,omitempty"`
	RecruitmentAHRPending       bool            `json:"recruitmentAhrPending,omitempty"`
	ObservedAt                  time.Time       `json:"observedAt,omitempty"`
}

type PlanningView struct {
	State           GameState
	Partitions      PartitionVersions
	ProtocolContext ProtocolContextState
}

func CapabilityForDomain(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	if strings.HasPrefix(normalized, "map-") || normalized == "storm-scan-progress" || normalized == "storm-scan" {
		return CapabilityWorldMap
	}
	switch normalized {
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
	return keys
}

func normalizePartitionKeys(keys []PartitionKey) []PartitionKey {
	out := make([]PartitionKey, 0, len(keys))
	for _, key := range keys {
		key = normalizePartitionKey(key)
		if key.Capability == "" || key.Scope.Kind == "" {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if existing == key {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, key)
		}
	}
	sort.Slice(out, func(left, right int) bool { return partitionKeyLess(out[left], out[right]) })
	return out
}

func normalizePartitionKey(key PartitionKey) PartitionKey {
	key.Capability = normalizeCapability(key.Capability)
	key.Scope.World = strings.TrimSpace(key.Scope.World)
	// Preserve the identity semantics of ScopeKey.Canonical: application and
	// session partitions are process/session capabilities, while increasingly
	// specific account scopes include only their identifying coordinates.
	switch key.Scope.Kind {
	case ScopeApplication:
		key.Scope = ScopeKey{Kind: ScopeApplication}
	case ScopeSession:
		key.Scope = ScopeKey{Kind: ScopeSession}
	case ScopeKingdom:
		key.Scope.CastleID = 0
		key.Scope.SessionGeneration = 0
		key.Scope.ConnectionGeneration = 0
	case ScopeCastle:
		key.Scope.SessionGeneration = 0
		key.Scope.ConnectionGeneration = 0
	default:
		key.Scope.Kind = ScopeAccount
		key.Scope.KingdomID = 0
		key.Scope.CastleID = 0
		key.Scope.SessionGeneration = 0
		key.Scope.ConnectionGeneration = 0
	}
	return key
}

func partitionKeyLess(left PartitionKey, right PartitionKey) bool {
	left = normalizePartitionKey(left)
	right = normalizePartitionKey(right)
	if left.Capability != right.Capability {
		return left.Capability < right.Capability
	}
	if left.Scope.Kind != right.Scope.Kind {
		return left.Scope.Kind < right.Scope.Kind
	}
	if left.Scope.World != right.Scope.World {
		return left.Scope.World < right.Scope.World
	}
	if left.Scope.PlayerID != right.Scope.PlayerID {
		return left.Scope.PlayerID < right.Scope.PlayerID
	}
	if left.Scope.KingdomID != right.Scope.KingdomID {
		return left.Scope.KingdomID < right.Scope.KingdomID
	}
	if left.Scope.CastleID != right.Scope.CastleID {
		return left.Scope.CastleID < right.Scope.CastleID
	}
	if left.Scope.SessionGeneration != right.Scope.SessionGeneration {
		return left.Scope.SessionGeneration < right.Scope.SessionGeneration
	}
	return left.Scope.ConnectionGeneration < right.Scope.ConnectionGeneration
}

func commonPartitionVersionSlot(key PartitionKey) (int, bool) {
	expectedScope := ScopeAccount
	switch key.Capability {
	case CapabilityProtocol:
		expectedScope = ScopeSession
		if key.Scope.Kind == expectedScope {
			return 0, true
		}
	case CapabilitySession:
		expectedScope = ScopeSession
		if key.Scope.Kind == expectedScope {
			return 1, true
		}
	case CapabilitySessionContext:
		expectedScope = ScopeSession
		if key.Scope.Kind == expectedScope {
			return 2, true
		}
	case CapabilityAccountProfile:
		return 3, key.Scope.Kind == expectedScope
	case CapabilityAccountWallet:
		return 4, key.Scope.Kind == expectedScope
	case CapabilityCastleDirectory:
		return 5, key.Scope.Kind == expectedScope
	case CapabilityBuildings:
		return 6, key.Scope.Kind == expectedScope
	case CapabilityBuildingQueue:
		return 7, key.Scope.Kind == expectedScope
	case CapabilityConstruction:
		return 8, key.Scope.Kind == expectedScope
	case CapabilityConstructionItems:
		return 9, key.Scope.Kind == expectedScope
	case CapabilityConstructionInventory:
		return 10, key.Scope.Kind == expectedScope
	case CapabilityConstructionCommerce:
		return 11, key.Scope.Kind == expectedScope
	case CapabilityInventory:
		return 12, key.Scope.Kind == expectedScope
	case CapabilityEconomy:
		return 13, key.Scope.Kind == expectedScope
	case CapabilityGarrison:
		return 14, key.Scope.Kind == expectedScope
	case CapabilityDefense:
		return 15, key.Scope.Kind == expectedScope
	case CapabilityProduction:
		return 16, key.Scope.Kind == expectedScope
	case CapabilityCrafting:
		return 17, key.Scope.Kind == expectedScope
	case CapabilityMovement:
		return 18, key.Scope.Kind == expectedScope
	case CapabilityAttacks:
		expectedScope = ScopeSession
		if key.Scope.Kind == expectedScope {
			return 19, true
		}
	case CapabilityLogistics:
		return 20, key.Scope.Kind == expectedScope
	case CapabilityLeaders:
		return 21, key.Scope.Kind == expectedScope
	case CapabilityEquipment:
		return 22, key.Scope.Kind == expectedScope
	case CapabilityAlliance:
		return 23, key.Scope.Kind == expectedScope
	case CapabilityWorldMap:
		return 24, key.Scope.Kind == expectedScope
	case CapabilityEvents:
		return 25, key.Scope.Kind == expectedScope
	case CapabilityReports:
		return 26, key.Scope.Kind == expectedScope
	case CapabilityAutomation:
		return 27, key.Scope.Kind == expectedScope
	}
	return 0, false
}

func cloneExtraPartitionVersions(source map[PartitionKey]*PartitionVersion, extra int) map[PartitionKey]*PartitionVersion {
	clone := make(map[PartitionKey]*PartitionVersion, len(source)+extra)
	for key, version := range source {
		clone[key] = version
	}
	return clone
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
	next := partitionVersionSnapshot{}
	if current != nil {
		next = *current
	}
	normalized := normalizePartitionKeys(keys)
	changed := make([]PartitionVersion, 0, len(normalized))
	extraMutable := false
	for _, key := range normalized {
		var previous *PartitionVersion
		slot, standard := commonPartitionVersionSlot(key)
		if standard {
			previous = next.common[slot]
			if previous != nil && previous.Key != key {
				previous = nil
			}
		} else {
			if !extraMutable {
				next.extra = cloneExtraPartitionVersions(next.extra, 1)
				extraMutable = true
			}
			previous = next.extra[key]
		}
		version := PartitionVersion{Key: key}
		if previous != nil {
			version = *previous
		}
		version.Key = key
		version.Version++
		version.Revision = revision
		version.UpdatedAt = updatedAt
		if standard {
			next.common[slot] = &version
		} else {
			next.extra[key] = &version
		}
		if previous == nil {
			if standard && next.common[slot] != nil && current != nil && current.common[slot] != nil {
				// Rebinding replaces a standard slot instead of retaining an
				// unreachable identity from the previous account.
			} else {
				next.count++
			}
		}
		changed = append(changed, version)
	}
	return &next, changed
}

func initialProtocolContext(state GameState) ProtocolContextState {
	context := ProtocolContextState{
		SessionGeneration: state.Session.Generation, ConnectionGeneration: state.Session.ConnectionGeneration,
	}
	for castleID, castle := range state.Castles {
		if castle.Focused {
			context.FocusedCastleID = castleID
			context.FocusSubcontext = FocusSubcontextCastle
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
	focusSubcontext FocusSubcontext,
	observedAt time.Time,
) ProtocolContextState {
	next := current
	if next.SessionGeneration != state.Session.Generation ||
		next.ConnectionGeneration != state.Session.ConnectionGeneration {
		next.SessionGeneration = state.Session.Generation
		next.ConnectionGeneration = state.Session.ConnectionGeneration
		next.FocusedCastleID = 0
		next.FocusSubcontext = FocusSubcontextUnknown
		next.FocusEpoch++
		clearRecruitmentBUPBatch(&next)
		next.ObservedAt = observedAt
	}
	if focusSubcontext == FocusSubcontextUnknown &&
		!containsDomain(domains, "session-context") &&
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
	nextSubcontext := next.FocusSubcontext
	if focusedCastleID == 0 {
		nextSubcontext = FocusSubcontextUnknown
	} else if focusSubcontext != FocusSubcontextUnknown {
		nextSubcontext = focusSubcontext
	} else if next.FocusedCastleID != focusedCastleID {
		// A newly selected castle comes from a castle snapshot such as JAA.
		nextSubcontext = FocusSubcontextCastle
	}
	if next.FocusedCastleID != focusedCastleID || next.FocusSubcontext != nextSubcontext {
		next.FocusedCastleID = focusedCastleID
		next.FocusSubcontext = nextSubcontext
		next.FocusEpoch++
		clearRecruitmentBUPBatch(&next)
	}
	next.SessionGeneration = state.Session.Generation
	next.ConnectionGeneration = state.Session.ConnectionGeneration
	next.ObservedAt = observedAt
	return next
}

func clearRecruitmentBUPBatch(context *ProtocolContextState) {
	if context == nil {
		return
	}
	context.RecruitmentBUPCastleID = 0
	context.RecruitmentBUPFocusEpoch = 0
	context.RecruitmentBUPSerial = 0
	context.RecruitmentAHRCoveredSerial = 0
	context.RecruitmentAHRFocusCovered = false
	context.RecruitmentAHRPending = false
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
