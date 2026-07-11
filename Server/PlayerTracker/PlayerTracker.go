package playertracker

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
	resources "CitadelDesktop/Server/Models/Resources"
	"CitadelDesktop/Server/Paths"
	"CitadelDesktop/Server/ResponseRegistry"
)

const (
	historyVersion   = 4
	historyFileName  = "PlayerTracker.json"
	sampleInterval   = time.Minute
	samplePoll       = time.Minute
	refreshTimeout   = 5 * time.Second
	rawRetention     = 24 * time.Hour
	hourlyRetention  = 7 * 24 * time.Hour
	historyRetention = 366 * 24 * time.Hour
)

// Sample is one durable observation of account-wide player state.
type Sample struct {
	TimestampUnix   int64                            `json:"timestampUnix"`
	PlayerID        int                              `json:"playerId"`
	Might           float64                          `json:"might"`
	Glory           float64                          `json:"glory"`
	Gallantry       float64                          `json:"gallantry"`
	TroopsTotal     int                              `json:"troopsTotal"`
	TroopsStationed int                              `json:"troopsStationed"`
	TroopsTraveling int                              `json:"troopsTraveling"`
	TroopsHospital  int                              `json:"troopsHospital"`
	TroopsByUnit    map[int]int                      `json:"troopsByUnit,omitempty"`
	Coins           float64                          `json:"coins"`
	Rubies          float64                          `json:"rubies"`
	Currencies      *resources.PlayerGlobalResources `json:"currencies,omitempty"`
}

// MetricPoint is one chart value with provenance so local observations remain authoritative.
type MetricPoint struct {
	TimestampUnix int64   `json:"timestampUnix"`
	Value         float64 `json:"value"`
	Source        string  `json:"source"`
}

type TrackerIdentity struct {
	PlayerID         int    `json:"playerId"`
	PlayerName       string `json:"playerName"`
	Server           string `json:"server"`
	ExternalPlayerID string `json:"externalPlayerId"`
}

type FallbackInfo struct {
	Provider      string `json:"provider"`
	Status        string `json:"status"`
	Server        string `json:"server,omitempty"`
	PlayerName    string `json:"playerName,omitempty"`
	FetchedAtUnix int64  `json:"fetchedAtUnix,omitempty"`
	PointsAdded   int    `json:"pointsAdded,omitempty"`
}

type historyFile struct {
	Version    int                     `json:"version"`
	Players    map[int][]Sample        `json:"players"`
	Identities map[int]TrackerIdentity `json:"identities,omitempty"`
}

type response struct {
	Current         *Sample                  `json:"current"`
	Samples         []Sample                 `json:"samples"`
	Series          map[string][]MetricPoint `json:"series"`
	IntervalSeconds int64                    `json:"intervalSeconds"`
	Fallback        FallbackInfo             `json:"fallback"`
	Coverage        struct {
		Loot        bool `json:"loot"`
		EventScores bool `json:"eventScores"`
	} `json:"coverage"`
}

var store = struct {
	sync.Mutex
	loaded     bool
	players    map[int][]Sample
	identities map[int]TrackerIdentity
}{players: make(map[int][]Sample), identities: make(map[int]TrackerIdentity)}

// Start begins durable sampling and actively refreshes queryable state before each observation.
func Start() {
	load()
	go func() {
		ticker := time.NewTicker(samplePoll)
		defer ticker.Stop()
		for {
			refreshAndRecordCurrent()
			<-ticker.C
		}
	}()
}

// RegisterHandlers exposes the self-player history to the embedded client.
func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/player-tracker", handlePlayerTracker)
}

// RecordCurrent persists the current player state when a sample is due.
func RecordCurrent() {
	sample, ok := currentSample(time.Now())
	if !ok {
		return
	}

	store.Lock()
	defer store.Unlock()
	ensureLoadedLocked()

	history := store.players[sample.PlayerID]
	if len(history) > 0 && sample.TimestampUnix-history[len(history)-1].TimestampUnix < int64(sampleInterval/time.Second) {
		return
	}

	cutoff := sample.TimestampUnix - int64(historyRetention/time.Second)
	firstKept := sort.Search(len(history), func(i int) bool {
		return history[i].TimestampUnix >= cutoff
	})
	next := append([]Sample(nil), history[firstKept:]...)
	next = append(next, sample)
	store.players[sample.PlayerID] = compactHistory(next, sample.TimestampUnix)
	writeLocked()
}

func compactHistory(history []Sample, nowUnix int64) []Sample {
	compacted := make([]Sample, 0, len(history))
	lastInterval := int64(0)
	lastBucket := int64(-1)
	for _, sample := range history {
		age := nowUnix - sample.TimestampUnix
		interval := int64(0)
		switch {
		case age > int64(hourlyRetention/time.Second):
			interval = int64((24 * time.Hour) / time.Second)
		case age > int64(rawRetention/time.Second):
			interval = int64(time.Hour / time.Second)
		}

		if interval == 0 {
			compacted = append(compacted, sample)
			lastInterval = 0
			lastBucket = -1
			continue
		}
		bucket := sample.TimestampUnix / interval
		if len(compacted) > 0 && lastInterval == interval && lastBucket == bucket {
			compacted[len(compacted)-1] = sample
			continue
		}
		compacted = append(compacted, sample)
		lastInterval = interval
		lastBucket = bucket
	}
	return compacted
}

// refreshAndRecordCurrent asks the game server for the safe, capture-verified snapshots used by the
// tracker. GDI refreshes self might/glory and DCL refreshes troop/resource state for every owned castle.
func refreshAndRecordCurrent() {
	if !ResponseRegistry.IsGameWebSocketReady() {
		RecordCurrent()
		return
	}
	gs := Models.GetGameState()
	if gs == nil || gs.PlayerID <= 0 {
		return
	}

	gdiWaiter := ResponseRegistry.Global.RegisterWaiter("gdi", refreshTimeout)
	dclWaiter := ResponseRegistry.Global.RegisterWaiter("dcl", refreshTimeout)
	defer gdiWaiter.Cleanup()
	defer dclWaiter.Cleanup()

	GameCommands.SendGDI(gs.PlayerID)
	GameCommands.SendDCLRefresh()
	_, _ = gdiWaiter.WaitWithTimeout()
	_, _ = dclWaiter.WaitWithTimeout()
	RecordCurrent()
}

func handlePlayerTracker(w http.ResponseWriter, r *http.Request) {
	current, hasCurrent := currentSample(time.Now())
	store.Lock()
	ensureLoadedLocked()
	var samples []Sample
	playerID := 0
	playerName := ""
	if hasCurrent {
		playerID = current.PlayerID
		playerName = Models.GetGameState().PlayerName
		samples = append(samples, store.players[current.PlayerID]...)
	} else {
		var newestPlayerID int
		var newestTimestamp int64
		for playerID, history := range store.players {
			if len(history) > 0 && history[len(history)-1].TimestampUnix > newestTimestamp {
				newestTimestamp = history[len(history)-1].TimestampUnix
				newestPlayerID = playerID
			}
		}
		samples = append(samples, store.players[newestPlayerID]...)
		playerID = newestPlayerID
		playerName = store.identities[newestPlayerID].PlayerName
	}
	store.Unlock()
	series := localMetricSeries(samples, func() *Sample {
		if hasCurrent {
			return &current
		}
		return nil
	}())
	rangeStart := requestedRangeStart(r, time.Now())
	fallback := augmentWithGGETracker(r.Context(), playerID, playerName, rangeStart, series)

	payload := response{
		Samples:         samples,
		Series:          series,
		IntervalSeconds: int64(sampleInterval / time.Second),
		Fallback:        fallback,
	}
	if hasCurrent {
		payload.Current = &current
	} else if len(samples) > 0 {
		last := samples[len(samples)-1]
		payload.Current = &last
	}
	payload.Coverage.Loot = len(series["loot"]) > 0

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[player-tracker] response encode: %v", err)
	}
}

func currentSample(now time.Time) (Sample, bool) {
	connection := ResponseRegistry.GetGameConnectionStatus()
	if !connection.LoggedIn || !connection.SocketConnected {
		return Sample{}, false
	}
	gs := Models.GetGameState()
	if gs == nil || gs.PlayerID <= 0 {
		return Sample{}, false
	}
	if gs.GlobalResources.MightPt <= 0 && gs.Castle.MainCastle.Aid <= 0 {
		return Sample{}, false
	}

	stationed, traveling, hospital, troopsByUnit := troopSnapshot(&gs.Castle)
	currencies := gs.GlobalResources
	return Sample{
		TimestampUnix:   now.Unix(),
		PlayerID:        gs.PlayerID,
		Might:           gs.GlobalResources.MightPt,
		Glory:           gs.GlobalResources.GloryPt,
		Gallantry:       gs.GlobalResources.GallanPt,
		TroopsTotal:     stationed + traveling + hospital,
		TroopsStationed: stationed,
		TroopsTraveling: traveling,
		TroopsHospital:  hospital,
		TroopsByUnit:    troopsByUnit,
		Coins:           gs.GlobalResources.Coins,
		Rubies:          gs.GlobalResources.Rubies,
		Currencies:      &currencies,
	}, true
}

func troopSnapshot(castles *castle.PlayerCastles) (stationed, traveling, hospital int, byUnit map[int]int) {
	byUnit = make(map[int]int)
	if castles == nil {
		return 0, 0, 0, byUnit
	}
	all := []*castle.PlayerCastleInfo{
		&castles.MainCastle,
		&castles.Outpost1,
		&castles.Outpost2,
		&castles.Outpost3,
		&castles.IceCastle,
		&castles.DesertCastle,
		&castles.DungeonCastle,
		&castles.StormCastle,
		&castles.BeriWorldCastle,
		&castles.Metropolis,
		&castles.Capital,
	}
	for _, playerCastle := range all {
		stationed += addPositiveTroops(byUnit, playerCastle.Troops.TroopsI)
		traveling += addPositiveTroops(byUnit, playerCastle.Troops.TroopsTU)
		hospital += addPositiveTroops(byUnit, playerCastle.Troops.TroopsHI)
		hospital += addPositiveTroops(byUnit, playerCastle.Troops.TroopsSHI)
	}
	return stationed, traveling, hospital, byUnit
}

func addPositiveTroops(byUnit map[int]int, values map[int]int) int {
	total := 0
	for unitID, value := range values {
		if value > 0 {
			total += value
			byUnit[unitID] += value
		}
	}
	return total
}

func historyPath() string {
	return filepath.Join(Paths.DataDir(), historyFileName)
}

func load() {
	store.Lock()
	defer store.Unlock()
	ensureLoadedLocked()
}

func ensureLoadedLocked() {
	if store.loaded {
		return
	}
	store.loaded = true
	data, err := os.ReadFile(historyPath())
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[player-tracker] read history: %v", err)
		}
		return
	}
	var persisted historyFile
	if err := json.Unmarshal(data, &persisted); err != nil {
		log.Printf("[player-tracker] parse history: %v", err)
		return
	}
	if persisted.Players != nil {
		store.players = persisted.Players
	}
	if persisted.Identities != nil {
		store.identities = persisted.Identities
	}
}

func writeLocked() {
	data, err := json.MarshalIndent(historyFile{
		Version:    historyVersion,
		Players:    store.players,
		Identities: store.identities,
	}, "", "  ")
	if err != nil {
		log.Printf("[player-tracker] marshal history: %v", err)
		return
	}
	if err := os.MkdirAll(Paths.DataDir(), 0755); err != nil {
		log.Printf("[player-tracker] create data directory: %v", err)
		return
	}
	path := historyPath()
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Printf("[player-tracker] write history: %v", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[player-tracker] replace history: %v", err)
		_ = os.Remove(tmp)
	}
}
