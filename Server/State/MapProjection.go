package State

import "time"

// Official map object identifiers used by the current product projection.
// The tenant store remains deliberately smaller than the game's full map: add
// a policy here when a feature gains a real reader for another official type.
const (
	MapTypePlayerCastle  = 1
	MapTypeKingdomTower  = 2
	MapTypeBerimondTower = 17
	MapTypeForeignLord   = 21
	MapTypeStormIsland   = 24
	MapTypeStormFort     = 25
	MapTypeNomadCamp     = 27
	MapTypeSamuraiCamp   = 29
	MapTypeBloodcrow     = 34
	MapTypeKhanCamp      = 35
	MapTypeRift          = 43
)

type MapProjectionKind uint8

const (
	MapProjectionNone MapProjectionKind = iota
	MapProjectionPlayerCastle
	MapProjectionTower
	MapProjectionBerimond
	MapProjectionInvasion
	MapProjectionEventCamp
	MapProjectionStorm
	MapProjectionRift
	mapProjectionKindCount
)

// MapProjectionKindForType resolves an official object type to the compact
// feature partition that stores and serves it. Callers that know the feature
// they need should use RangeMapObservationsByKind instead of traversing the
// complete retained map and filtering every row themselves.
func MapProjectionKindForType(typeID int) (MapProjectionKind, bool) {
	policy, retained := MapProjectionFor(typeID)
	if !retained || policy.Kind <= MapProjectionNone || policy.Kind >= mapProjectionKindCount {
		return MapProjectionNone, false
	}
	return policy.Kind, true
}

// MapDomainForType is the exact policy wake domain for a retained official map
// type. The generic "map" domain remains a compatibility/reset signal; normal
// coordinate updates use these feature domains so unrelated policies stay idle.
func MapDomainForType(typeID int) (string, bool) {
	kind, retained := MapProjectionKindForType(typeID)
	if !retained {
		return "", false
	}
	return MapDomainForKind(kind)
}

func MapDomainForKind(kind MapProjectionKind) (string, bool) {
	switch kind {
	case MapProjectionPlayerCastle:
		return "map-player-castle", true
	case MapProjectionTower:
		return "map-tower", true
	case MapProjectionBerimond:
		return "map-berimond", true
	case MapProjectionInvasion:
		return "map-invasion", true
	case MapProjectionEventCamp:
		return "map-event-camp", true
	case MapProjectionStorm:
		return "map-storm", true
	case MapProjectionRift:
		return "map-rift", true
	default:
		return "", false
	}
}

type MapProjectionPolicy struct {
	Kind           MapProjectionKind
	ShareWhenOwned bool
	// ShareForWorld marks observations whose projected fields are public,
	// objective facts for every account that can access the same kingdom in the
	// same game world. Contributor identity is never part of the shared value.
	ShareForWorld    bool
	RetainObjectName bool
	// DashboardMap is true only when the React client directly reads this map
	// object kind. Backend automation retains the broader official projection.
	DashboardMap bool
	// MaxAge bounds stale event/map knowledge without changing the logical
	// official record. A zero observation timestamp is retained fail-closed.
	MaxAge time.Duration
}

// MapProjectionFor is the single extension point between official map object
// IDs and tenant state. Unknown types are decoded transiently but not retained.
func MapProjectionFor(typeID int) (MapProjectionPolicy, bool) {
	var policy MapProjectionPolicy
	switch typeID {
	case MapTypePlayerCastle:
		policy = MapProjectionPolicy{
			Kind: MapProjectionPlayerCastle, ShareWhenOwned: true, RetainObjectName: true,
			MaxAge: 30 * 24 * time.Hour,
		}
	case MapTypeKingdomTower:
		policy = MapProjectionPolicy{Kind: MapProjectionTower, MaxAge: 90 * 24 * time.Hour}
	case MapTypeBerimondTower:
		policy = MapProjectionPolicy{Kind: MapProjectionBerimond, MaxAge: 14 * 24 * time.Hour}
	case MapTypeForeignLord, MapTypeBloodcrow:
		policy = MapProjectionPolicy{Kind: MapProjectionInvasion, MaxAge: 3 * 24 * time.Hour}
	case MapTypeNomadCamp, MapTypeSamuraiCamp, MapTypeKhanCamp:
		policy = MapProjectionPolicy{Kind: MapProjectionEventCamp, MaxAge: 14 * 24 * time.Hour}
	case MapTypeStormIsland, MapTypeStormFort:
		// Storm map rows are the product of an ordinary public GAA map query.
		// Sharing them lets a process scan each window once. Every write/attack is
		// still guarded by the acting account's private castle, inventory,
		// commander, settings, movement, and targeted pre-dispatch verification.
		policy = MapProjectionPolicy{Kind: MapProjectionStorm, ShareForWorld: true, MaxAge: 3 * 24 * time.Hour}
	case MapTypeRift:
		policy = MapProjectionPolicy{Kind: MapProjectionRift, DashboardMap: true, MaxAge: 14 * 24 * time.Hour}
	default:
		return MapProjectionPolicy{}, false
	}
	return policy, true
}

func mapObservationExpired(observation MapObservation, now time.Time) bool {
	if now.IsZero() || observation.ObservedAt.IsZero() {
		return false
	}
	policy, retained := MapProjectionFor(observation.TypeID)
	return !retained || policy.MaxAge > 0 && !observation.ObservedAt.Add(policy.MaxAge).After(now)
}

// projectMapObservation strips fields that are irrelevant to the official
// object kind. This keeps the compatibility view easy to extend while making
// the stored tenant record an explicit, consumer-backed projection.
func projectMapObservation(source MapObservation) (MapObservation, bool) {
	policy, retained := MapProjectionFor(source.TypeID)
	if !retained {
		return MapObservation{}, false
	}
	projected := MapObservation{
		KingdomID: source.KingdomID,
		X:         source.X, Y: source.Y, TypeID: source.TypeID,
		ObservedAt: source.ObservedAt,
	}
	switch policy.Kind {
	case MapProjectionPlayerCastle:
		projected.Name = source.Name
		projected.OwnerID = source.OwnerID
		projected.ObjectID = source.ObjectID
	case MapProjectionTower:
		projected.Level = source.Level
		projected.ObjectID = source.ObjectID
		projected.TowerVictoryCount = source.TowerVictoryCount
		projected.TowerCooldownRemaining = source.TowerCooldownRemaining
	case MapProjectionBerimond:
		projected.Level = source.Level
		projected.ObjectID = source.ObjectID
	case MapProjectionInvasion:
		projected.Level = source.Level
		projected.ObjectID = source.ObjectID
	case MapProjectionEventCamp:
		projected.Level = source.Level
		projected.ObjectID = source.ObjectID
		projected.EventCampID = source.EventCampID
		projected.EventCampVictoryCount = source.EventCampVictoryCount
		projected.EventCampCooldownRemaining = source.EventCampCooldownRemaining
		projected.EventCampBaseWallBonus = source.EventCampBaseWallBonus
		projected.EventCampBaseGateBonus = source.EventCampBaseGateBonus
		projected.EventCampBaseMoatBonus = source.EventCampBaseMoatBonus
	case MapProjectionStorm:
		projected.Level = source.Level
		projected.OwnerID = source.OwnerID
		projected.ObjectID = source.ObjectID
		projected.StormIsleID = source.StormIsleID
		projected.StormVictoryCount = source.StormVictoryCount
		projected.StormCooldownRemaining = source.StormCooldownRemaining
	case MapProjectionRift:
		projected.ObjectID = source.ObjectID
	}
	return projected, true
}
