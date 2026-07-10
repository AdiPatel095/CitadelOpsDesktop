// Package autotcisched persists the next TCI (construction item) auto-upgrade fire times
// so the process can re-arm a wake (including reconnect) after disconnect, similar to autobird_sent.json.
package autotcisched

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Paths"
)

const fileName = "autotci_upgrade_schedule.json"

// Lead times for the AutoTCI schedule (see GameFeatures AutoTCI upgrade constants).
const (
	LoginLeadMillis int64 = 60 * 1000     // wake / prep session this long before expiry
	UbcWindowMillis int64 = 5 * 60 * 1000 // ubc is issued in the last 5m of RS (autoTCIUpgradeTimerMaxSec)
)

// SlotRecord is one equipped slot the user is tracking for auto-upgrade, with times derived
// from gca REMAINING (RS) at last observation. Game data on the next JAA is authoritative.
type SlotRecord struct {
	CastleID  int `json:"castleId"`
	OID       int `json:"oid"`
	SlotS     int `json:"slotS"`
	WireCID   int `json:"wireCid"`
	RSSeconds int `json:"rsSeconds"`
	// ExpiresAtMillis is the absolute wall time when the current TCI tier expires, from: observedAt + RS.
	ExpiresAtMillis int64 `json:"expiresAtMillis"`
	// LoginWakeAtMillis is when to wake the client to prep a session (LoginLeadMillis before Expires).
	// Optional on disk: if 0, derived as ExpiresAtMillis-LoginLeadMillis.
	LoginWakeAtMillis int64 `json:"loginWakeAtMillis,omitempty"`
	// FireAtMillis is when we should attempt ubc (UbcWindowMillis before expiry), or
	// a rebuy/buy+equip window when Rebuy is set (no further upgrade under user ceiling).
	FireAtMillis     int64 `json:"fireAtMillis"`
	ObservedAtMillis int64 `json:"observedAtMillis"`
	// Rebuy when at user tier ceiling: refresh via gbc/sbp and rpc (see GameFeatures).
	Rebuy bool `json:"rebuy,omitempty"`
}

// File is the full persisted schedule; replaced on every merge from live game data.
type File struct {
	Version  int          `json:"version"`
	PlayerID int          `json:"playerId"`
	Slots    []SlotRecord `json:"slots"`
}

var fileMu sync.Mutex

func filePath() string {
	return filepath.Join(Paths.DataDir(), fileName)
}

// Load returns the schedule (empty if missing or corrupt).
func Load() *File {
	fileMu.Lock()
	defer fileMu.Unlock()
	b, err := os.ReadFile(filePath())
	if err != nil {
		return &File{Version: 1, Slots: nil}
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		Logging.AutoTCILogf("schedule file", "parse %s: %v", filePath(), err)
		return &File{Version: 1, Slots: nil}
	}
	if f.Slots == nil {
		f.Slots = []SlotRecord{}
	}
	if f.Version == 0 {
		f.Version = 1
	}
	return &f
}

// Save overwrites the schedule file.
func Save(f *File) {
	if f == nil {
		return
	}
	_ = os.MkdirAll(Paths.DataDir(), 0755)
	if f.Slots == nil {
		f.Slots = []SlotRecord{}
	}
	if f.Version == 0 {
		f.Version = 1
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filePath(), b, 0644); err != nil {
		Logging.AutoTCILogf("schedule file", "write: %v", err)
	}
}

// EarliestFireMillis is deprecated: use NextScheduleWakeMillis. Kept for older callers; equivalent to
// only the ubc times, not login leads.
func EarliestFireMillis(f *File) int64 {
	if f == nil || len(f.Slots) == 0 {
		return 0
	}
	var min int64
	for i := range f.Slots {
		fm := f.Slots[i].FireAtMillis
		if fm == 0 {
			continue
		}
		if min == 0 || fm < min {
			min = fm
		}
	}
	return min
}

func deriveLUMillis(s *SlotRecord) (L, U int64, ok bool) {
	e := s.ExpiresAtMillis
	if e <= 0 {
		return 0, 0, false
	}
	L = s.LoginWakeAtMillis
	if L == 0 {
		L = e - LoginLeadMillis
	}
	if L < 0 {
		L = 0
	}
	U = s.FireAtMillis
	if U == 0 {
		U = e - UbcWindowMillis
	}
	if U < 0 {
		U = 0
	}
	return L, U, true
}

// nextSlotEventMillis is the next absolute wall time to schedule for this slot, or 0 with overdueUBC
// when we are at/past the ubc time; skip is true for unusable slot rows (e.g. missing Expires).
func nextSlotEventMillis(s *SlotRecord, now int64) (eventMillis int64, overdueUBC, skip bool) {
	L, U, ok := deriveLUMillis(s)
	if !ok {
		return 0, false, true
	}
	if now >= U {
		return 0, true, false
	}
	next := U
	if now < L && L < next {
		next = L
	}
	return next, false, false
}

// NextScheduleWakeMillis is the next absolute time to run the AutoTCI wake loop: either login prep
// (LoginLeadMillis before expiry) or the ubc window (UbcWindowMillis before expiry).
// Returns 0 if: no slots, any slot is at/past ubc time (need immediate), or there is no valid next event.
func NextScheduleWakeMillis(f *File, now int64) int64 {
	if f == nil || len(f.Slots) == 0 {
		return 0
	}
	var min int64
	have := false
	for i := range f.Slots {
		ev, odb, sk := nextSlotEventMillis(&f.Slots[i], now)
		if sk {
			continue
		}
		if odb {
			return 0
		}
		if ev > 0 {
			if !have || ev < min {
				have, min = true, ev
			}
		}
	}
	if !have {
		return 0
	}
	return min
}

// NextFireDuration returns time until the next schedule event (login prep or ubc); zero if already due or no data.
func NextFireDuration(f *File) time.Duration {
	ef := NextScheduleWakeMillis(f, time.Now().UnixMilli())
	if ef == 0 {
		return 0
	}
	t := time.UnixMilli(ef)
	return time.Until(t)
}

// Clear removes the on-disk file (best-effort) and in-memory is empty.
func Clear() {
	fileMu.Lock()
	defer fileMu.Unlock()
	_ = os.Remove(filePath())
}
