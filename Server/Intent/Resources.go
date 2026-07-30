package Intent

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"CitadelDesktop/Server/State"
)

type ResourceScope string

const (
	ResourceScopeApplication ResourceScope = "application"
	ResourceScopeSession     ResourceScope = "session"
	ResourceScopeAccount     ResourceScope = "account"
	ResourceScopeKingdom     ResourceScope = "kingdom"
	ResourceScopeCastle      ResourceScope = "castle"
)

// ResourceKey identifies effect authority independently of the legacy claim
// spelling. Empty identifiers are deliberate wildcards only after the key has
// been normalized.
type ResourceKey struct {
	Account      string          `json:"account,omitempty"`
	Scope        ResourceScope   `json:"scope"`
	KingdomID    State.KingdomID `json:"kingdomId,omitempty"`
	CastleID     State.CastleID  `json:"castleId,omitempty"`
	Capability   string          `json:"capability"`
	ResourceKind string          `json:"resourceKind"`
	ResourceID   string          `json:"resourceId,omitempty"`
}

func derivePlanResources(gameState State.GameState, plan Plan) []ResourceKey {
	resources := append([]ResourceKey(nil), plan.Resources...)
	resources = append(resources, legacyClaimsToResources(gameState, plan.Claims)...)
	return normalizeResourceKeys(resources)
}

func legacyClaimsToResources(gameState State.GameState, claims []string) []ResourceKey {
	claims = normalizeClaims(claims)
	account := resourceAccount(gameState)
	castleID, kingdomID := legacyClaimScope(gameState, claims)
	resources := make([]ResourceKey, 0, len(claims))
	for _, claim := range claims {
		resources = append(resources, legacyClaimResource(account, castleID, kingdomID, claim))
	}
	return normalizeResourceKeys(resources)
}

func resourceAccount(gameState State.GameState) string {
	worldID, playerID := State.BoundAccount(gameState)
	server := strings.ToLower(strings.TrimSpace(worldID))
	if server == "" && playerID == 0 {
		return ""
	}
	return fmt.Sprintf("%s#%d", server, playerID)
}

func legacyClaimScope(gameState State.GameState, claims []string) (State.CastleID, State.KingdomID) {
	var castleID State.CastleID
	for _, claim := range claims {
		parts := strings.Split(claim, ":")
		if len(parts) != 2 || parts[0] != "castle" {
			continue
		}
		value, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || value <= 0 {
			continue
		}
		candidate := State.CastleID(value)
		if castleID != 0 && castleID != candidate {
			return 0, 0
		}
		castleID = candidate
	}
	if castleID == 0 {
		return 0, 0
	}
	return castleID, gameState.Castles[castleID].KingdomID
}

func legacyClaimResource(
	account string,
	defaultCastle State.CastleID,
	defaultKingdom State.KingdomID,
	claim string,
) ResourceKey {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(claim)), ":")
	prefix := parts[0]
	value := strings.Join(parts[1:], ":")
	accountKey := func(capability, kind, id string) ResourceKey {
		return ResourceKey{Account: account, Scope: ResourceScopeAccount, Capability: capability, ResourceKind: kind, ResourceID: id}
	}
	sessionKey := func(capability, kind, id string) ResourceKey {
		return ResourceKey{Account: account, Scope: ResourceScopeSession, Capability: capability, ResourceKind: kind, ResourceID: id}
	}
	castleKey := func(capability, kind, id string) ResourceKey {
		return ResourceKey{
			Account: account, Scope: ResourceScopeCastle, KingdomID: defaultKingdom, CastleID: defaultCastle,
			Capability: capability, ResourceKind: kind, ResourceID: id,
		}
	}
	kingdomKey := func(kingdomID State.KingdomID, capability, kind, id string) ResourceKey {
		return ResourceKey{
			Account: account, Scope: ResourceScopeKingdom, KingdomID: kingdomID,
			Capability: capability, ResourceKind: kind, ResourceID: id,
		}
	}

	switch prefix {
	case "game-data":
		return ResourceKey{Scope: ResourceScopeApplication, Capability: "catalog", ResourceKind: "official-game-data", ResourceID: "*"}
	case "application-update":
		return ResourceKey{Scope: ResourceScopeApplication, Capability: "application-update", ResourceKind: "release", ResourceID: "*"}
	case "event-difficulty":
		return accountKey("events", "difficulty", "*")
	case "event":
		return accountKey("events", "event", value)
	case "advisor":
		return accountKey("combat", "advisor", value)
	case "khan-protection":
		return accountKey("combat", "khan-protection", "*")
	case "khan-lane":
		// Each Auto Khan lane claims its own identifier so the lanes run
		// concurrently, while an unqualified "khan-lane" claim covers every lane
		// and lets the protection intents exclude all of them at once.
		return accountKey("combat", "khan-lane", value)
	case "session":
		return sessionKey("session", "lifecycle", "*")
	case "castle-focus":
		return sessionKey("session", "focus", "*")
	case "response":
		return sessionKey("protocol", "response", value)
	case "game-ui", "attack-context":
		return sessionKey("combat", "attack-context", "*")
	case "castle":
		id, _ := strconv.ParseInt(value, 10, 64)
		castle := State.CastleID(id)
		kingdom := defaultKingdom
		if castle != defaultCastle {
			kingdom = 0
		}
		return ResourceKey{
			Account: account, Scope: ResourceScopeCastle, KingdomID: kingdom, CastleID: castle,
			Capability: "*", ResourceKind: "*", ResourceID: "*",
		}
	case "kingdom":
		id, _ := strconv.ParseInt(value, 10, 64)
		return kingdomKey(State.KingdomID(id), "kingdom", "state", "*")
	case "map":
		id, _ := strconv.ParseInt(value, 10, 64)
		return kingdomKey(State.KingdomID(id), "world-map", "partition", "*")
	case "account-resources":
		return accountKey("economy", "spendable", "*")
	case "currency":
		return accountKey("economy", "spendable", value)
	case "hall-of-legends":
		return accountKey("economy", "spendable", "legend-skills")
	case "construction-inventory":
		return accountKey("inventory", "construction-item", "*")
	case "inventory":
		return accountKey("inventory", value, "*")
	case "storage":
		return accountKey("inventory", "storage", value)
	case "equipment", "gem":
		return accountKey("equipment", prefix, value)
	case "game":
		switch value {
		case "equipment":
			return accountKey("equipment", "state", "*")
		case "movements":
			return accountKey("movements", "movement", "*")
		case "crafting":
			return accountKey("crafting", "*", "*")
		}
		return accountKey("game", value, "*")
	case "commander":
		return accountKey("leaders", "commander", value)
	case "leader":
		if len(parts) >= 3 {
			return accountKey("leaders", parts[1], strings.Join(parts[2:], ":"))
		}
	case "building-layout":
		return castleKey("buildings", "layout", "*")
	case "building-construction":
		return castleKey("construction", "queue", "*")
	case "building":
		return castleKey("buildings", "instance", value)
	case "building-position":
		if len(parts) >= 4 {
			id, _ := strconv.ParseInt(parts[1], 10, 64)
			return ResourceKey{
				Account: account, Scope: ResourceScopeCastle, CastleID: State.CastleID(id),
				Capability: "buildings", ResourceKind: "position", ResourceID: strings.Join(parts[2:], ":"),
			}
		}
	case "decoration-layout":
		return castleKey("buildings", "decoration-layout", "*")
	case "defense":
		id, _ := strconv.ParseInt(value, 10, 64)
		return ResourceKey{
			Account: account, Scope: ResourceScopeCastle, CastleID: State.CastleID(id),
			Capability: "defense", ResourceKind: "setup", ResourceID: "*",
		}
	case "attack-inventory":
		id, _ := strconv.ParseInt(value, 10, 64)
		return ResourceKey{
			Account: account, Scope: ResourceScopeCastle, CastleID: State.CastleID(id),
			Capability: "garrison", ResourceKind: "attack-inventory", ResourceID: "*",
		}
	case "unit":
		if defaultCastle > 0 {
			return castleKey("garrison", "unit", value)
		}
		return accountKey("garrison", "unit", value)
	case "beri-capacity":
		id, _ := strconv.ParseInt(value, 10, 64)
		return ResourceKey{
			Account: account, Scope: ResourceScopeCastle, CastleID: State.CastleID(id),
			Capability: "garrison", ResourceKind: "beri-capacity", ResourceID: "*",
		}
	case "hospital":
		return castleKey("garrison", "hospital", "*")
	case "production-line":
		return castleKey("production", "line", value)
	case "crafting-building":
		return castleKey("crafting", "building", value)
	case "shop":
		if value == "" {
			value = "*"
		}
		return accountKey("shop", "dialog", value)
	case "construction-shop":
		return accountKey("shop", "dialog", "construction")
	case "reports":
		return accountKey("reports", "message", "*")
	case "report-message":
		return accountKey("reports", "message", value)
	case "battle-report":
		return accountKey("reports", "battle", value)
	case "alliance-directory":
		return accountKey("alliance", "directory", "*")
	case "alliance-help":
		return accountKey("alliance", "help", value)
	case "alliance-holding":
		return accountKey("alliance", "holding", value)
	case "configuration":
		return ResourceKey{
			Scope: ResourceScopeApplication, Capability: "configuration", ResourceKind: "section", ResourceID: value,
		}
	case "resource-transport", "troop-transport":
		return accountKey("transport", prefix, "*")
	case "movement":
		return accountKey("movements", "movement", value)
	case "scheduled-operation":
		return accountKey("scheduling", "operation", value)
	case "rift-launch":
		return accountKey("rift", "launch", value)
	case "beri-target":
		if len(parts) >= 2 {
			id, _ := strconv.ParseInt(parts[1], 10, 64)
			targetID := "*"
			if len(parts) >= 4 {
				targetID = strings.Join(parts[2:], ":")
			}
			return kingdomKey(State.KingdomID(id), "combat", "target", targetID)
		}
	case "tower-target", "nomad-target", "storm-target", "invasion-target", "spy-target", "khan-target", "player-target":
		if len(parts) >= 4 {
			id, _ := strconv.ParseInt(parts[1], 10, 64)
			return kingdomKey(State.KingdomID(id), "combat", "target", strings.Join(parts[2:], ":"))
		}
	}
	return accountKey("legacy", prefix, value)
}

func hasLegacyResource(resources []ResourceKey) bool {
	for _, resource := range resources {
		if resource.Capability == "legacy" {
			return true
		}
	}
	return false
}

func normalizeResourceKeys(resources []ResourceKey) []ResourceKey {
	byCanonical := make(map[string]ResourceKey, len(resources))
	for _, resource := range resources {
		resource.Account = strings.ToLower(strings.TrimSpace(resource.Account))
		resource.Scope = ResourceScope(strings.ToLower(strings.TrimSpace(string(resource.Scope))))
		resource.Capability = strings.ToLower(strings.TrimSpace(resource.Capability))
		resource.ResourceKind = strings.ToLower(strings.TrimSpace(resource.ResourceKind))
		resource.ResourceID = strings.ToLower(strings.TrimSpace(resource.ResourceID))
		if resource.Scope == "" || resource.Capability == "" || resource.ResourceKind == "" {
			continue
		}
		if resource.ResourceID == "" {
			resource.ResourceID = "*"
		}
		byCanonical[resource.Canonical()] = resource
	}
	keys := make([]string, 0, len(byCanonical))
	for key := range byCanonical {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ResourceKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, byCanonical[key])
	}
	return out
}

func (resource ResourceKey) Canonical() string {
	return fmt.Sprintf(
		"%s|%s|%d|%d|%s|%s|%s",
		resource.Account, resource.Scope, resource.KingdomID, resource.CastleID,
		resource.Capability, resource.ResourceKind, resource.ResourceID,
	)
}

func resourcesOverlap(left, right []ResourceKey) bool {
	for _, first := range left {
		for _, second := range right {
			if resourceKeysOverlap(first, second) {
				return true
			}
		}
	}
	return false
}

func resourceKeysOverlap(left, right ResourceKey) bool {
	if left.Canonical() == right.Canonical() {
		return true
	}
	if left.Account != "" && right.Account != "" && left.Account != right.Account {
		return false
	}
	if left.Scope != right.Scope {
		var parent, child ResourceKey
		if left.Scope == ResourceScopeAccount {
			parent, child = left, right
		} else if right.Scope == ResourceScopeAccount {
			parent, child = right, left
		} else {
			return false
		}
		if child.Scope != ResourceScopeCastle && child.Scope != ResourceScopeKingdom {
			return false
		}
		if child.Capability == "*" {
			return false
		}
		return dimensionsOverlap(parent.Capability, child.Capability) &&
			dimensionsOverlap(parent.ResourceKind, child.ResourceKind) &&
			dimensionsOverlap(parent.ResourceID, child.ResourceID)
	}
	switch left.Scope {
	case ResourceScopeCastle:
		if left.CastleID != right.CastleID {
			return false
		}
	case ResourceScopeKingdom:
		if left.KingdomID != right.KingdomID {
			return false
		}
	}
	return dimensionsOverlap(left.Capability, right.Capability) &&
		dimensionsOverlap(left.ResourceKind, right.ResourceKind) &&
		dimensionsOverlap(left.ResourceID, right.ResourceID)
}

func resourceContains(parent, child ResourceKey) bool {
	if parent.Account != "" && parent.Account != child.Account {
		return false
	}
	if parent.Scope != child.Scope || parent.KingdomID != child.KingdomID || parent.CastleID != child.CastleID {
		return false
	}
	return dimensionContains(parent.Capability, child.Capability) &&
		dimensionContains(parent.ResourceKind, child.ResourceKind) &&
		dimensionContains(parent.ResourceID, child.ResourceID)
}

func dimensionsOverlap(left, right string) bool {
	return left == "*" || right == "*" || left == right
}

func dimensionContains(parent, child string) bool {
	return parent == "*" || parent == child
}

func sameResourceKeys(left, right []ResourceKey) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Canonical() != right[index].Canonical() {
			return false
		}
	}
	return true
}
