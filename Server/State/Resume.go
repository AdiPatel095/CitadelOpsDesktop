package State

import (
	"crypto/rand"
	"encoding/hex"
	"maps"
	"slices"
	"time"
)

const (
	resumeMaxEvents = 512
	resumeMaxBytes  = 1 << 20
	resumeMaxAge    = 15 * time.Minute
)

type resumeEntry struct {
	event Event
	at    time.Time
	bytes int
}

// resumeRing is protected by Store.writeMu. It retains metadata only, never
// patches, cached encodings, map values, or old immutable state generations.
type resumeRing struct {
	entries [resumeMaxEvents]resumeEntry
	start   int
	count   int
	bytes   int
	floor   uint64
}

func newStoreInstance() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

func (store *Store) Instance() string { return store.instance }

func (ring *resumeRing) evict() {
	entry := &ring.entries[ring.start]
	ring.floor = entry.event.Revision
	ring.bytes -= entry.bytes
	*entry = resumeEntry{}
	ring.start = (ring.start + 1) % resumeMaxEvents
	ring.count--
}

func (ring *resumeRing) expire(now time.Time) {
	for ring.count > 0 && now.Sub(ring.entries[ring.start].at) > resumeMaxAge {
		ring.evict()
	}
}

func (ring *resumeRing) append(event Event, now time.Time) {
	ring.expire(now)
	metadata := resumeMetadata(event)
	weight := resumeMetadataBytes(metadata)
	for ring.count > 0 && (ring.count == resumeMaxEvents || ring.bytes+weight > resumeMaxBytes) {
		ring.evict()
	}
	if weight > resumeMaxBytes {
		// One oversized event cannot be retained, and no cursor before it is safe.
		ring.floor = event.Revision
		return
	}
	if ring.count == 0 {
		ring.floor = event.BaseRevision
	}
	index := (ring.start + ring.count) % resumeMaxEvents
	ring.entries[index] = resumeEntry{event: metadata, at: now, bytes: weight}
	ring.count++
	ring.bytes += weight
}

func resumeMetadata(event Event) Event {
	event.generation = nil
	event.clientEncoding = nil
	event.Patch = nil
	event.Domains = slices.Clone(event.Domains)
	event.Components = slices.Clone(event.Components)
	event.Partitions = slices.Clone(event.Partitions)
	event.mapChanges = slices.Clone(event.mapChanges)
	for index := range event.mapChanges {
		event.mapChanges[index].Observation = nil
	}
	event.castleIDs = slices.Clone(event.castleIDs)
	event.castleParts = maps.Clone(event.castleParts)
	event.equipmentIDs = slices.Clone(event.equipmentIDs)
	event.gemIDs = slices.Clone(event.gemIDs)
	event.itemKeys = slices.Clone(event.itemKeys)
	event.stormTargetKeys = slices.Clone(event.stormTargetKeys)
	event.towerCooldownKeys = slices.Clone(event.towerCooldownKeys)
	event.towerQueueCastles = slices.Clone(event.towerQueueCastles)
	event.reportMessageIDs = slices.Clone(event.reportMessageIDs)
	event.eventScoreIDs = slices.Clone(event.eventScoreIDs)
	event.movementIDs = slices.Clone(event.movementIDs)
	return event
}

// Conservative estimated retained metadata: 1024 bytes per event (struct and
// allocation overhead), 8 per numeric key/component, 32 per castle-part map
// entry, 128 per map key, 256 per partition, and 24 per string plus its bytes.
// The fixed ring slots are bounded separately by the 512-event count.
func resumeMetadataBytes(event Event) int {
	weight := 1024 + 8*(len(event.Components)+len(event.castleIDs)+len(event.equipmentIDs)+
		len(event.gemIDs)+len(event.towerQueueCastles)+len(event.reportMessageIDs)+
		len(event.eventScoreIDs)+len(event.movementIDs)) + 32*len(event.castleParts)
	for _, change := range event.mapChanges {
		weight += 128 + len(change.Key)
	}
	for _, partition := range event.Partitions {
		weight += 256 + len(partition.Key.Capability) + len(partition.Key.Scope.World)
	}
	for _, keys := range [][]string{event.Domains, event.itemKeys, event.stormTargetKeys, event.towerCooldownKeys} {
		for _, key := range keys {
			weight += 24 + len(key)
		}
	}
	return weight
}

// Resume reads the head and ring together, after the caller subscribes. The
// returned state is an immutable view; Event is one complete patch from since
// to that head. Events queued by the subscription at or below the head are
// harmless duplicates, discarded by the client's revision check.
func (store *Store) Resume(instance string, since uint64) (state GameState, event *Event, resumed bool) {
	store.writeMu.Lock()
	defer store.writeMu.Unlock()
	generation := store.generation.Load()
	state = *generation.state
	store.resume.expire(time.Now())
	if instance == "" || instance != store.instance || since < store.resume.floor || since > state.Revision {
		return state, nil, false
	}
	if since == state.Revision {
		return state, nil, true
	}
	var merged Event
	found := false
	for offset := 0; offset < store.resume.count; offset++ {
		entry := store.resume.entries[(store.resume.start+offset)%resumeMaxEvents].event
		if entry.Revision <= since {
			continue
		}
		if !found {
			merged = entry
			found = true
		} else {
			merged = mergeEventMetadata(merged, entry)
		}
	}
	if !found {
		return state, nil, false
	}
	merged.BaseRevision = since
	merged.generation = generation
	// Map entries in the ring are keys only. Resolve their latest values from
	// the same current generation as every other part of the merged patch.
	merged.mapChanges = slices.Clone(merged.mapChanges)
	for index := range merged.mapChanges {
		change := &merged.mapChanges[index]
		if observation, exists := state.LookupMapObservation(change.KingdomID, change.Key); exists {
			change.Observation = &observation
			change.TypeID = observation.TypeID
			change.Deleted = false
		} else {
			change.Deleted = true
		}
	}
	patch := coalesceEventQueue([]Event{merged})
	return state, &patch, true
}
