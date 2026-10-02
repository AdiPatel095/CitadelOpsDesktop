package State

import (
	"fmt"
	"testing"
	"time"
)

func TestSharedFortressCoverageAndPerAccountCeiling(t *testing.T) {
	for _, n := range []int{1, 2, 6, 8} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			s := NewWorldMapStore()
			start := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			scope := MapScanScope{Kind: "fortress", WorldID: "world-one", Zone: "EmpireEx", KingdomID: 1}
			interval := 30 * time.Minute
			baseline := len(FortressScanWindows(450, 450, nil))
			perAccount := map[string][]time.Time{}
			cycles := map[int]map[string]int{}
			for tick := 0; tick < 3*180; tick++ {
				now := start.Add(time.Duration(tick) * 10 * time.Second)
				for i := 0; i < n; i++ {
					s.RegisterFortressScanner(fmt.Sprint(i), scope, 450, 450, interval, now)
				}
				for i := 0; i < n; i++ {
					id := fmt.Sprint(i)
					lease := s.AcquireFortressScan(id, scope, now)
					for _, w := range lease.Windows {
						cycle := int(now.Sub(start) / interval)
						if cycles[cycle] == nil {
							cycles[cycle] = map[string]int{}
						}
						cycles[cycle][stormScanWindowKey(w)]++
						if cycles[cycle][stormScanWindowKey(w)] > 1 {
							t.Fatal("duplicate coverage in interval")
						}
						perAccount[id] = append(perAccount[id], now)
						count := 0
						for _, at := range perAccount[id] {
							if now.Before(at.Add(interval)) {
								count++
							}
						}
						if count > baseline {
							t.Fatalf("account %s sent %d requests in its interval; today's maximum is %d", id, count, baseline)
						}
						if err := s.CompleteFortressWindow(id, scope, lease.LeaseID, w, false, now); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			for cycle := 0; cycle < 3; cycle++ {
				if len(cycles[cycle]) != baseline {
					t.Fatalf("cycle %d covered %d/%d windows", cycle, len(cycles[cycle]), baseline)
				}
			}
		})
	}
}
func TestSharedFortressWeightedRatesStayWithinEachOwnInterval(t *testing.T) {
	s := NewWorldMapStore()
	start := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	scope := MapScanScope{Kind: "fortress", WorldID: "world-one", KingdomID: 1}
	intervals := map[string]time.Duration{"fast": 10 * time.Minute, "slow": 40 * time.Minute}
	calls := map[string][]time.Time{}
	baseline := len(FortressScanWindows(450, 450, nil))
	for tick := 0; tick < 240; tick++ {
		now := start.Add(time.Duration(tick) * 10 * time.Second)
		for id, interval := range intervals {
			s.RegisterFortressScanner(id, scope, 450, 450, interval, now)
		}
		for _, id := range []string{"fast", "slow"} {
			lease := s.AcquireFortressScan(id, scope, now)
			if lease.Interval != 10*time.Minute {
				t.Fatal("group interval is not the minimum")
			}
			for _, w := range lease.Windows {
				calls[id] = append(calls[id], now)
				count := 0
				for _, at := range calls[id] {
					if now.Before(at.Add(intervals[id])) {
						count++
					}
				}
				if count > baseline {
					t.Fatalf("%s exceeded own request rate", id)
				}
				if err := s.CompleteFortressWindow(id, scope, lease.LeaseID, w, false, now); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if len(calls["fast"]) <= len(calls["slow"]) {
		t.Fatalf("weighted scan rates: fast=%d slow=%d", len(calls["fast"]), len(calls["slow"]))
	}
}
func TestSharedFortressLostOwnerLeaseIsReassigned(t *testing.T) {
	s := NewWorldMapStore()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	scope := MapScanScope{Kind: "fortress", WorldID: "world-one", KingdomID: 1}
	for _, id := range []string{"lost", "survivor"} {
		s.RegisterFortressScanner(id, scope, 450, 450, time.Hour, now)
	}
	s.AcquireFortressScan("lost", scope, now)
	lease := s.AcquireFortressScan("lost", scope, now.Add(3*time.Second))
	if len(lease.Windows) == 0 {
		t.Fatal("no initial lease")
	}
	later := now.Add(SharedMapLeaseTTL + 4*time.Second)
	s.RegisterFortressScanner("survivor", scope, 450, 450, time.Hour, later)
	s.AcquireFortressScan("survivor", scope, later)
	got := s.AcquireFortressScan("survivor", scope, later.Add(3*time.Second))
	if len(got.Windows) == 0 || got.ParticipantCount != 1 {
		t.Fatal("lost owner blocked survivor")
	}
	if s.ValidateFortressLease("lost", scope, lease.LeaseID, lease.Windows, later) {
		t.Fatal("expired lease remained valid")
	}
}
func TestSharedFortressSeparatesMultiZoneHostsAndKingdoms(t *testing.T) {
	s := NewWorldMapStore()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	host := "wss://ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com"
	for _, zone := range []string{"EmpireEx", "EmpireEx_17", "EmpireEx_21"} {
		for _, kingdom := range []KingdomID{1, 2} {
			scope := MapScanScope{Kind: "fortress", WorldID: host, Zone: zone, KingdomID: kingdom}
			id := fmt.Sprintf("%s-%d", zone, kingdom)
			s.RegisterFortressScanner(id, scope, 450, 450, time.Hour, now)
			s.AcquireFortressScan(id, scope, now)
			got := s.AcquireFortressScan(id, scope, now.Add(3*time.Second))
			if got.ParticipantCount != 1 || len(got.Windows) == 0 {
				t.Fatal("zones or kingdoms mixed")
			}
			other := scope
			other.Zone = "foreign-zone"
			if s.ValidateFortressLease(id, other, got.LeaseID, got.Windows, now.Add(3*time.Second)) {
				t.Fatal("lease crossed zone")
			}
		}
	}
	a, b := NewGameState(), NewGameState()
	a.Account.WorldID = host
	b.Account.WorldID = host
	a.Session.Namespace = "EmpireEx"
	b.Session.Namespace = "EmpireEx_21"
	if SharedWorldID(&a) == SharedWorldID(&b) {
		t.Fatal("shared map scopes mixed zones")
	}
}

func TestSharedFortressRejoiningParticipantKeepsItsRequestBudget(t *testing.T) {
	s := NewWorldMapStore()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	scope := MapScanScope{Kind: "fortress", WorldID: "world-one", KingdomID: 1}
	s.RegisterFortressScanner("slow", scope, 450, 450, time.Hour, now)
	s.AcquireFortressScan("slow", scope, now)
	lease := s.AcquireFortressScan("slow", scope, now.Add(3*time.Second))
	for _, w := range lease.Windows {
		if err := s.CompleteFortressWindow("slow", scope, lease.LeaseID, w, false, now.Add(3*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	later := now.Add(3 * time.Minute)
	s.RegisterFortressScanner("fast", scope, 450, 450, time.Minute, later)
	s.AcquireFortressScan("fast", scope, later)
	s.RegisterFortressScanner("slow", scope, 450, 450, time.Hour, later.Add(time.Second))
	s.mu.RLock()
	used := len(s.mapScanGroups[scope].participants["slow"].requests)
	s.mu.RUnlock()
	if used != len(lease.Windows) {
		t.Fatal("roster expiry refunded request budget")
	}
}
