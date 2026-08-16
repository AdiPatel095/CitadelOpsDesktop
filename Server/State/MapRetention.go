package State

import (
	"sort"
	"time"
)

const (
	accountPrivateMapRetentionLimit = 100_000
	sharedWorldMapRetentionLimit    = 250_000
)

type mapRetentionCandidate struct {
	kingdomID KingdomID
	key       string
	typeID    int
	observed  time.Time
}

// PruneMap removes expired and over-limit account-private observations in one
// ordinary map revision. Objective shared-world facts are pruned once by the
// process WorldMapStore instead of once per account.
func (store *Store) PruneMap(now time.Time) (int, error) {
	if store == nil || now.IsZero() {
		return 0, nil
	}
	removed := 0
	_, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		candidates := privateMapRetentionCandidates(*state)
		domains := []string{"retention"}
		for _, candidate := range mapRetentionRemovals(candidates, now, accountPrivateMapRetentionLimit) {
			if state.DeleteMapObservation(candidate.kingdomID, candidate.key) {
				removed++
				if domain, retained := MapDomainForType(candidate.typeID); retained {
					domains = append(domains, domain)
				}
			}
		}
		return normalizeDomains(domains), removed > 0, nil
	})
	return removed, err
}

func privateMapRetentionCandidates(state GameState) []mapRetentionCandidate {
	candidates := []mapRetentionCandidate{}
	state.privateMapKingdomIDs(func(kingdomID KingdomID) {
		state.rangePrivateMapObservations(kingdomID, func(key string, observation MapObservation) bool {
			candidates = append(candidates, mapRetentionCandidate{
				kingdomID: kingdomID, key: key, typeID: observation.TypeID, observed: observation.ObservedAt,
			})
			return true
		})
	})
	return candidates
}

func mapRetentionRemovals(
	candidates []mapRetentionCandidate,
	now time.Time,
	limit int,
) []mapRetentionCandidate {
	removals := make([]mapRetentionCandidate, 0)
	retained := make([]mapRetentionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		observation := MapObservation{TypeID: candidate.typeID, ObservedAt: candidate.observed}
		if mapObservationExpired(observation, now) {
			removals = append(removals, candidate)
			continue
		}
		retained = append(retained, candidate)
	}
	if limit <= 0 || len(retained) <= limit {
		return removals
	}
	sort.Slice(retained, func(left, right int) bool {
		if !retained[left].observed.Equal(retained[right].observed) {
			return retained[left].observed.After(retained[right].observed)
		}
		if retained[left].kingdomID != retained[right].kingdomID {
			return retained[left].kingdomID < retained[right].kingdomID
		}
		return retained[left].key < retained[right].key
	})
	return append(removals, retained[limit:]...)
}

// Prune bounds every shared world independently. Deletions flow through the
// ordinary coordinate event and grouped SQLite commit paths.
func (store *WorldMapStore) Prune(now time.Time) int {
	if store == nil || now.IsZero() {
		return 0
	}
	store.mu.RLock()
	snapshots := make(map[string]*worldMapGeneration, len(store.worlds))
	for worldID, generation := range store.worlds {
		snapshots[worldID] = generation
	}
	store.mu.RUnlock()
	removed := 0
	for worldID, generation := range snapshots {
		if generation == nil {
			continue
		}
		candidates := []mapRetentionCandidate{}
		for kingdomID := range generation.values {
			rangeWorldFacts(generation.values, kingdomID, func(key string, fact WorldMapFact) bool {
				candidates = append(candidates, mapRetentionCandidate{
					kingdomID: kingdomID, key: key, typeID: fact.TypeID, observed: fact.ObservedAt,
				})
				return true
			})
		}
		removals := mapRetentionRemovals(candidates, now, sharedWorldMapRetentionLimit)
		if len(removals) == 0 {
			continue
		}
		changes := make([]MapChange, 0, len(removals))
		for _, candidate := range removals {
			changes = append(changes, MapChange{
				KingdomID: candidate.kingdomID, Key: candidate.key, TypeID: candidate.typeID,
				Deleted: true, expectedObservedAt: candidate.observed,
			})
		}
		event := store.commit(worldID, nil, changes)
		removed += len(event.Changes)
	}
	return removed
}
