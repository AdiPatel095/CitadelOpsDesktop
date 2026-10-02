package State

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"
)

const SharedMapLeaseTTL = 2 * time.Minute
const FortressScanChunkSize = 90
const FortressScanMaximumChunk = 20

type MapScanScope struct {
	Kind, WorldID, Zone string
	KingdomID           KingdomID
}
type MapScanAssignment struct {
	LeaseID          string
	Windows          []StormMapBounds
	NextCheckAt      time.Time
	ParticipantCount int
	Interval         time.Duration
}
type mapScanParticipant struct {
	heartbeat time.Time
	interval  time.Duration
	x, y      int
	requests  []time.Time
}
type mapScanLease struct {
	account string
	windows map[string]StormMapBounds
	expires time.Time
}
type mapScanGroup struct {
	participants map[string]*mapScanParticipant
	content      map[string]bool
	completed    map[string]time.Time
	leases       map[string]*mapScanLease
	roster       string
	changed      time.Time
}

// sharedMapScans is process owned. Storm retains its existing geometry,
// persistence and scheduling; Fortress uses rate-bounded rendezvous leases.
// Account identities exist only in volatile roster/lease state.
type sharedMapScans struct {
	stormParticipants map[string]map[string]time.Time
	stormRosters      map[string]stormScanRoster
	stormLeases       map[string]stormScanLease
	nextStormLease    uint64
	mapScanGroups     map[MapScanScope]*mapScanGroup
	nextMapLease      uint64
}

func newSharedMapScans() sharedMapScans {
	return sharedMapScans{stormParticipants: map[string]map[string]time.Time{}, stormRosters: map[string]stormScanRoster{}, stormLeases: map[string]stormScanLease{}, mapScanGroups: map[MapScanScope]*mapScanGroup{}}
}

func normalizeMapScanScope(scope MapScanScope) MapScanScope {
	scope.WorldID = CanonicalWorldID(scope.WorldID)
	scope.Zone = strings.TrimSpace(scope.Zone)
	return scope
}
func (s *WorldMapStore) RegisterFortressScanner(account string, scope MapScanScope, x, y int, interval time.Duration, now time.Time) {
	if s == nil || strings.TrimSpace(account) == "" || interval <= 0 || now.IsZero() || x < 0 || y < 0 || x/90 > 20 || y/90 > 20 {
		return
	}
	scope = normalizeMapScanScope(scope)
	if scope.Kind != "fortress" || scope.WorldID == "" || scope.KingdomID < 1 || scope.KingdomID > 3 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.mapScanGroups[scope]
	if group == nil {
		group = &mapScanGroup{participants: map[string]*mapScanParticipant{}, content: map[string]bool{}, completed: map[string]time.Time{}, leases: map[string]*mapScanLease{}}
		s.mapScanGroups[scope] = group
	}
	p := group.participants[account]
	if p == nil {
		p = &mapScanParticipant{}
		group.participants[account] = p
	}
	p.heartbeat, p.interval, p.x, p.y = now, interval, x, y
}

// FortressScanWindows preserves the legacy adaptive discovery envelope: 90
// tiles per request, two chunks of padding, and a maximum chunk index of 20.
// It can grow only from observed content, rather than scanning an entire grid.
func FortressScanWindows(x, y int, content map[string]bool) []StormMapBounds {
	start := StormMapBounds{X1: (x / 90) * 90, Y1: (y / 90) * 90}
	start.X2 = start.X1 + 89
	start.Y2 = start.Y1 + 89
	queue := []StormMapBounds{start}
	seen := map[string]bool{stormScanWindowKey(start): true}
	minX, maxX, minY, maxY := x/90, x/90, y/90, y/90
	result := []StormMapBounds{}
	for len(queue) > 0 {
		w := queue[0]
		queue = queue[1:]
		result = append(result, w)
		cx, cy := w.X1/90, w.Y1/90
		has := content[stormScanWindowKey(w)]
		if has {
			minX = min(minX, cx)
			maxX = max(maxX, cx)
			minY = min(minY, cy)
			maxY = max(maxY, cy)
		}
		for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			nx, ny := cx+d[0], cy+d[1]
			if nx < 0 || ny < 0 || nx > 20 || ny > 20 {
				continue
			}
			if !has && (nx < minX-2 || nx > maxX+2 || ny < minY-2 || ny > maxY+2) {
				continue
			}
			next := StormMapBounds{X1: nx * 90, Y1: ny * 90, X2: nx*90 + 89, Y2: ny*90 + 89}
			key := stormScanWindowKey(next)
			if !seen[key] {
				seen[key] = true
				queue = append(queue, next)
			}
		}
	}
	return result
}

func rendezvousScore(window, account string, interval time.Duration) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(window + "\x00" + account))
	// Exponential races: weight is each account's own scan rate (1/interval).
	u := (float64(h.Sum64()>>11) + 1) / (float64(uint64(1)<<53) + 1)
	return -math.Log(u) * float64(interval)
}

func (s *WorldMapStore) AcquireFortressScan(account string, scope MapScanScope, now time.Time) MapScanAssignment {
	out := MapScanAssignment{NextCheckAt: now.Add(time.Minute)}
	if s == nil {
		return out
	}
	scope = normalizeMapScanScope(scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.mapScanGroups[scope]
	if g == nil {
		return out
	}
	for id, p := range g.participants {
		if now.Sub(p.heartbeat) >= SharedMapLeaseTTL {
			recent := p.requests[:0]
			for _, at := range p.requests {
				if now.Before(at.Add(p.interval)) {
					recent = append(recent, at)
				}
			}
			p.requests = recent
			if len(recent) == 0 {
				delete(g.participants, id)
			}
		}
	}
	for id, l := range g.leases {
		if !l.expires.After(now) || g.participants[l.account] == nil || now.Sub(g.participants[l.account].heartbeat) >= SharedMapLeaseTTL {
			delete(g.leases, id)
		}
	}
	if len(g.participants) == 0 {
		delete(s.mapScanGroups, scope)
		return out
	}
	ids := make([]string, 0, len(g.participants))
	windows := map[string]StormMapBounds{}
	budgets := map[string]int{}
	interval := time.Duration(math.MaxInt64)
	for id, p := range g.participants {
		if now.Sub(p.heartbeat) >= SharedMapLeaseTTL {
			continue
		}
		ids = append(ids, id)
		interval = min(interval, p.interval)
		personal := FortressScanWindows(p.x, p.y, g.content)
		for _, w := range personal {
			windows[stormScanWindowKey(w)] = w
		}
		recent := p.requests[:0]
		for _, at := range p.requests {
			if now.Before(at.Add(p.interval)) {
				recent = append(recent, at)
			}
		}
		p.requests = recent
		budgets[id] = max(0, len(personal)-len(recent))
	}
	sort.Strings(ids)
	out.ParticipantCount = len(ids)
	out.Interval = interval
	signature := strings.Join(ids, "\x00")
	if g.roster != signature {
		g.roster = signature
		g.changed = now
	}
	if now.Before(g.changed.Add(2 * time.Second)) {
		out.NextCheckAt = g.changed.Add(2 * time.Second)
		return out
	}
	if g.participants[account] == nil {
		return out
	}
	leased := map[string]bool{}
	for _, l := range g.leases {
		for key := range l.windows {
			leased[key] = true
		}
	}
	keys := make([]string, 0, len(windows))
	for key := range windows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assigned := []StormMapBounds{}
	for _, key := range keys {
		if leased[key] || !g.completed[key].IsZero() && now.Before(g.completed[key].Add(interval)) {
			continue
		}
		ranking := append([]string(nil), ids...)
		sort.Slice(ranking, func(i, j int) bool {
			a, b := ranking[i], ranking[j]
			sa, sb := rendezvousScore(key, a, g.participants[a].interval), rendezvousScore(key, b, g.participants[b].interval)
			if sa == sb {
				return a < b
			}
			return sa < sb
		})
		for _, owner := range ranking {
			if budgets[owner] > 0 {
				budgets[owner]--
				if owner == account && len(assigned) < 8 {
					assigned = append(assigned, windows[key])
				}
				break
			}
		}
	}
	if len(assigned) == 0 {
		return out
	}
	s.nextMapLease++
	out.LeaseID = fmt.Sprintf("fortress-%016x", s.nextMapLease)
	out.Windows = assigned
	out.NextCheckAt = now.Add(time.Second)
	lease := &mapScanLease{account: account, expires: now.Add(SharedMapLeaseTTL), windows: map[string]StormMapBounds{}}
	for _, w := range assigned {
		lease.windows[stormScanWindowKey(w)] = w
		g.participants[account].requests = append(g.participants[account].requests, now)
	}
	g.leases[out.LeaseID] = lease
	return out
}

func (s *WorldMapStore) CompleteFortressWindow(account string, scope MapScanScope, leaseID string, window StormMapBounds, hasContent bool, now time.Time) error {
	if s == nil {
		return fmt.Errorf("shared scan is unavailable")
	}
	scope = normalizeMapScanScope(scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.mapScanGroups[scope]
	if g == nil {
		return fmt.Errorf("shared scan scope is unavailable")
	}
	l := g.leases[leaseID]
	key := stormScanWindowKey(window)
	if l == nil || l.account != account || !l.expires.After(now) {
		return fmt.Errorf("shared scan lease is unavailable")
	}
	if bound, ok := l.windows[key]; !ok || bound != window {
		return fmt.Errorf("window is not leased to this account")
	}
	g.content[key] = hasContent
	g.completed[key] = now
	delete(l.windows, key)
	if len(l.windows) == 0 {
		delete(g.leases, leaseID)
	}
	return nil
}
func (s *WorldMapStore) ReleaseFortressScan(account string, scope MapScanScope, leaseID string) {
	if s == nil {
		return
	}
	scope = normalizeMapScanScope(scope)
	s.mu.Lock()
	defer s.mu.Unlock()
	if g := s.mapScanGroups[scope]; g != nil {
		if l := g.leases[leaseID]; l != nil && l.account == account {
			delete(g.leases, leaseID)
		}
	}
}

func (s *WorldMapStore) ValidateFortressLease(account string, scope MapScanScope, leaseID string, windows []StormMapBounds, now time.Time) bool {
	if s == nil {
		return false
	}
	scope = normalizeMapScanScope(scope)
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.mapScanGroups[scope]
	if g == nil {
		return false
	}
	l := g.leases[leaseID]
	if l == nil || l.account != account || !l.expires.After(now) || len(windows) != len(l.windows) {
		return false
	}
	seen := map[string]bool{}
	for _, w := range windows {
		key := stormScanWindowKey(w)
		if bound, ok := l.windows[key]; !ok || bound != w || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}
