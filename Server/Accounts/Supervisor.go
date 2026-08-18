// Package Accounts owns process-level account cardinality. Everything inside
// App remains a single-account runtime; Supervisor creates N of those runtimes
// while sharing only services whose data is objectively common.
package Accounts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/AppUpdate"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/PrivateMetrics"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/WorldIntel"
)

var accountIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

const (
	mapRetentionSweepInterval = time.Hour
	drainCheckpointTimeout    = 5 * time.Second
	playerRebindSweepInterval = 20 * time.Second
	playerRebindStopTimeout   = 60 * time.Second
)

type AccountID string

func ParseAccountID(raw string) (AccountID, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if !accountIDPattern.MatchString(normalized) {
		return "", fmt.Errorf("account id must be 1-64 lowercase letters, digits, underscores, or hyphens")
	}
	return AccountID(normalized), nil
}

type Config struct {
	DataRoot         string
	GameDataCacheDir string
	// MaxAccounts bounds one process group's GC and failure domain. Zero keeps
	// the desktop/test composition unbounded; tenant manifests always provide a
	// positive limit.
	MaxAccounts            int
	Offline                bool
	RuntimeContext         context.Context
	GameData               *GameData.Manager
	PrivateMetricsClient   *PrivateMetrics.Client
	UpdateEndpoint         string
	UpdateInstallSupported bool
}

type AccountConfig struct {
	ID string
	// DataDir is reserved for the desktop N=1 composition, which must continue
	// using its existing profile root. Hosted accounts omit it and are placed
	// under DataRoot/Accounts/{id}.
	DataDir   string
	Transport Session.Transport
	Chromium  *Session.ChromiumConfig
	// BackgroundOnly prevents a hosted account from ever constructing a browser
	// transport, even if an imported desktop profile selected Full mode.
	BackgroundOnly          bool
	StartSession            bool
	PrivateMetricsPlacement *PrivateMetrics.Placement
}

type accountRuntime struct {
	application *App.Application
	cancel      context.CancelFunc
	// config is retained so the supervisor can restart the runtime unchanged,
	// e.g. after rebinding its profile onto the player-keyed directory.
	config AccountConfig
}

type Supervisor struct {
	config         Config
	ctx            context.Context
	cancel         context.CancelFunc
	gameData       *GameData.Manager
	worldMaps      *State.WorldMapStore
	updates        *AppUpdate.Manager
	worldIntel     *WorldIntel.CloudClient
	privateMetrics *PrivateMetrics.Client
	reportsCloud   *Reports.CloudClient
	ingest         *Ingest.Registry
	ownsGameData   bool
	startupErr     error

	mu       sync.RWMutex
	accounts map[AccountID]accountRuntime
	stopping map[AccountID]accountRuntime
	pending  map[AccountID]struct{}
	dataDirs map[string]AccountID
	closed   bool
	addWG    sync.WaitGroup

	// playerBindings maps runtime IDs to the player-keyed profile directory
	// under Players/ (see PlayerDirs.go). Guarded by mu; persisted next to the
	// account root. identityOf is a test seam over State.AccountIdentity.
	playerBindings     map[string]string
	playerBindingsPath string
	identityOf         func(*App.Application) (string, int64, bool)
	rebindMu           sync.Mutex

	refreshMu sync.Mutex
	startOnce sync.Once
}

type Capacity struct {
	Max      int `json:"max"`
	Active   int `json:"active"`
	Starting int `json:"starting"`
	Stopping int `json:"stopping"`
}

func New(ctx context.Context, config Config) (*Supervisor, error) {
	if strings.TrimSpace(config.DataRoot) == "" {
		return nil, fmt.Errorf("account data root is required")
	}
	dataRoot, err := filepath.Abs(config.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve account data root: %w", err)
	}
	config.DataRoot = dataRoot
	cacheDir := strings.TrimSpace(config.GameDataCacheDir)
	if cacheDir == "" {
		cacheDir = filepath.Join(dataRoot, "Shared", "GameData", "Items")
	} else {
		cacheDir, err = filepath.Abs(cacheDir)
		if err != nil {
			return nil, fmt.Errorf("resolve shared game-data cache: %w", err)
		}
		if !pathWithin(dataRoot, cacheDir) {
			return nil, fmt.Errorf("shared game-data cache must stay inside the process data root")
		}
	}
	config.GameDataCacheDir = cacheDir
	if config.RuntimeContext == nil {
		config.RuntimeContext = ctx
	}
	if config.RuntimeContext == nil {
		config.RuntimeContext = context.Background()
	}
	runtimeContext, cancel := context.WithCancel(config.RuntimeContext)

	gameData := config.GameData
	ownsGameData := gameData == nil
	var startupErr error
	if ownsGameData {
		gameData = GameData.NewManager(GameData.UpdaterConfig{
			CacheDir: cacheDir,
		})
		if config.Offline {
			startupErr = gameData.LoadCache()
		} else {
			startupContext := ctx
			if startupContext == nil {
				startupContext = runtimeContext
			}
			startupErr = gameData.Initialize(startupContext)
		}
	}
	worldMaps, worldMapErr := State.OpenWorldMapStore(dataRoot)
	if worldMapErr != nil {
		startupErr = errors.Join(startupErr, worldMapErr)
		worldMaps = State.NewWorldMapStore()
	}
	updates := AppUpdate.NewManager(AppUpdate.Config{
		CurrentVersion: App.Version, Endpoint: config.UpdateEndpoint,
		InstallSupported: config.UpdateInstallSupported,
	})
	worldIntel := WorldIntel.NewCloudClient(WorldIntel.ClientConfig{ClientVersion: App.Version})
	reportsCloud := Reports.NewCloudClient(Reports.CloudConfig{})
	ingestRegistry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(ingestRegistry); err != nil {
		cancel()
		_ = worldMaps.Close(context.Background())
		return nil, fmt.Errorf("register shared protocol reducers: %w", err)
	}

	bindingsPath := filepath.Join(dataRoot, "Accounts", playerBindingsFileName)
	bindings, bindingsErr := loadPlayerBindings(bindingsPath)
	if bindingsErr != nil {
		// A corrupt registry must not stop the cell: runtimes fall back to
		// staging directories and rebind again from live identity.
		startupErr = errors.Join(startupErr, bindingsErr)
		bindings = map[string]string{}
	}

	return &Supervisor{
		config: config, ctx: runtimeContext, cancel: cancel,
		gameData: gameData, worldMaps: worldMaps, updates: updates, worldIntel: worldIntel,
		privateMetrics: config.PrivateMetricsClient,
		reportsCloud:   reportsCloud, ingest: ingestRegistry,
		ownsGameData: ownsGameData, startupErr: startupErr,
		accounts: map[AccountID]accountRuntime{}, stopping: map[AccountID]accountRuntime{},
		pending: map[AccountID]struct{}{}, dataDirs: map[string]AccountID{},
		playerBindings: bindings, playerBindingsPath: bindingsPath,
		identityOf: func(application *App.Application) (string, int64, bool) {
			if application == nil || application.State == nil {
				return "", 0, false
			}
			return application.State.AccountIdentity()
		},
	}, nil
}

func (supervisor *Supervisor) GameData() *GameData.Manager {
	if supervisor == nil {
		return nil
	}
	return supervisor.gameData
}

func (supervisor *Supervisor) WorldMaps() *State.WorldMapStore {
	if supervisor == nil {
		return nil
	}
	return supervisor.worldMaps
}

func (supervisor *Supervisor) StartupError() error {
	if supervisor == nil {
		return nil
	}
	return supervisor.startupErr
}

func (supervisor *Supervisor) Start() {
	if supervisor == nil {
		return
	}
	supervisor.startOnce.Do(func() {
		supervisor.worldMaps.StartPersistence()
		go supervisor.updates.Run(supervisor.ctx)
		ready := make(chan struct{})
		go supervisor.runWorldMapPropagation(ready)
		<-ready
		go supervisor.runMapRetention()
		go supervisor.runPlayerRebinds()
		if supervisor.ownsGameData {
			go supervisor.runCatalogRefresh()
		}
	})
}

func (supervisor *Supervisor) runMapRetention() {
	ticker := time.NewTicker(mapRetentionSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-supervisor.ctx.Done():
			return
		case now := <-ticker.C:
			supervisor.worldMaps.Prune(now.UTC())
			supervisor.mu.RLock()
			stores := make([]*State.Store, 0, len(supervisor.accounts))
			for _, runtime := range supervisor.accounts {
				if runtime.application != nil && runtime.application.State != nil {
					stores = append(stores, runtime.application.State)
				}
			}
			supervisor.mu.RUnlock()
			for _, store := range stores {
				_, _ = store.PruneMap(now.UTC())
			}
		}
	}
}

func (supervisor *Supervisor) runWorldMapPropagation(ready chan<- struct{}) {
	events, unsubscribe := supervisor.worldMaps.Subscribe(1024)
	defer unsubscribe()
	close(ready)
	for {
		select {
		case <-supervisor.ctx.Done():
			return
		case event := <-events:
			supervisor.mu.RLock()
			stores := make([]*State.Store, 0, len(supervisor.accounts))
			for _, runtime := range supervisor.accounts {
				if runtime.application != nil && runtime.application.State != nil && runtime.application.State != event.Source {
					stores = append(stores, runtime.application.State)
				}
			}
			supervisor.mu.RUnlock()
			for _, store := range stores {
				store.AdoptWorldMap(event)
			}
		}
	}
}

func (supervisor *Supervisor) runCatalogRefresh() {
	timer := time.NewTimer(App.GameDataRefreshInterval)
	defer timer.Stop()
	for {
		select {
		case <-supervisor.ctx.Done():
			return
		case <-timer.C:
			refreshContext, cancel := context.WithTimeout(supervisor.ctx, 90*time.Second)
			_ = supervisor.RefreshGameData(refreshContext)
			cancel()
			timer.Reset(App.GameDataRefreshInterval)
		}
	}
}

func (supervisor *Supervisor) AddAccount(ctx context.Context, config AccountConfig) (*App.Application, error) {
	if supervisor == nil {
		return nil, fmt.Errorf("account supervisor is unavailable")
	}
	id, err := ParseAccountID(config.ID)
	if err != nil {
		return nil, err
	}
	dataDir, err := supervisor.accountDataDir(id, config.DataDir)
	if err != nil {
		return nil, err
	}
	supervisor.mu.Lock()
	if supervisor.closed {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("account supervisor is closed")
	}
	if _, exists := supervisor.accounts[id]; exists {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("account %q is already running", id)
	}
	if _, exists := supervisor.stopping[id]; exists {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("account %q is still stopping", id)
	}
	if _, exists := supervisor.pending[id]; exists {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("account %q is already starting", id)
	}
	if supervisor.config.MaxAccounts > 0 &&
		len(supervisor.accounts)+len(supervisor.pending)+len(supervisor.stopping) >= supervisor.config.MaxAccounts {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("account process limit of %d has been reached", supervisor.config.MaxAccounts)
	}
	if owner, exists := supervisor.dataDirs[dataDir]; exists {
		supervisor.mu.Unlock()
		return nil, fmt.Errorf("account data directory is already owned by %q", owner)
	}
	supervisor.pending[id] = struct{}{}
	supervisor.dataDirs[dataDir] = id
	supervisor.addWG.Add(1)
	supervisor.mu.Unlock()
	defer supervisor.addWG.Done()

	registered := false
	defer func() {
		if registered {
			return
		}
		supervisor.mu.Lock()
		delete(supervisor.pending, id)
		delete(supervisor.dataDirs, dataDir)
		supervisor.mu.Unlock()
	}()

	accountContext, cancel := context.WithCancel(supervisor.ctx)
	application, err := App.New(ctx, App.Config{
		DataDir: dataDir, AccountKey: string(id),
		Offline: supervisor.config.Offline, GameData: supervisor.gameData,
		WorldMaps:               supervisor.worldMaps,
		IngestRegistry:          supervisor.ingest,
		Updates:                 supervisor.updates,
		WorldIntelClient:        supervisor.worldIntel,
		PrivateMetricsClient:    supervisor.privateMetrics,
		PrivateMetricsPlacement: config.PrivateMetricsPlacement,
		ReportsCloudClient:      supervisor.reportsCloud,
		RefreshGameData:         supervisor.RefreshGameData,
		Transport:               config.Transport, Chromium: config.Chromium,
		BackgroundOnly: config.BackgroundOnly, RuntimeContext: accountContext,
		UpdateEndpoint:         supervisor.config.UpdateEndpoint,
		UpdateInstallSupported: supervisor.config.UpdateInstallSupported,
	})
	if err != nil {
		cancel()
		return nil, err
	}

	supervisor.mu.Lock()
	if supervisor.closed {
		delete(supervisor.pending, id)
		delete(supervisor.dataDirs, dataDir)
		supervisor.mu.Unlock()
		application.Start(accountContext)
		cancel()
		_ = application.Wait(context.Background())
		return nil, fmt.Errorf("account supervisor closed while account %q was starting", id)
	}
	delete(supervisor.pending, id)
	supervisor.accounts[id] = accountRuntime{application: application, cancel: cancel, config: config}
	supervisor.mu.Unlock()
	registered = true
	application.Start(accountContext)
	if config.StartSession {
		go func() { _ = application.Session.Start(accountContext) }()
	}
	return application, nil
}

func (supervisor *Supervisor) PrivateMetricsEnabled() bool {
	return supervisor != nil && supervisor.privateMetrics != nil && supervisor.privateMetrics.Enabled()
}

func (supervisor *Supervisor) SetPrivateMetricsPlacement(id AccountID, placement *PrivateMetrics.Placement) error {
	if supervisor == nil {
		return fmt.Errorf("account supervisor is unavailable")
	}
	application, exists := supervisor.Application(id)
	if !exists || application == nil {
		return fmt.Errorf("account %q is not running", id)
	}
	return application.SetPrivateMetricsPlacement(placement)
}

// ClearSavedLogins removes the saved game logins of an account that is not
// running, for example one that was drained before its credential was revoked.
// It reports whether the account's data directory existed on this cell.
func (supervisor *Supervisor) ClearSavedLogins(id AccountID) (bool, error) {
	if supervisor == nil {
		return false, fmt.Errorf("account supervisor is unavailable")
	}
	dataDir, err := supervisor.accountDataDir(id, "")
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(dataDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, Session.ClearSavedLogins(dataDir)
}

func (supervisor *Supervisor) Application(id AccountID) (*App.Application, bool) {
	if supervisor == nil {
		return nil, false
	}
	supervisor.mu.RLock()
	runtime, exists := supervisor.accounts[id]
	supervisor.mu.RUnlock()
	return runtime.application, exists
}

func (supervisor *Supervisor) AccountIDs() []AccountID {
	if supervisor == nil {
		return []AccountID{}
	}
	supervisor.mu.RLock()
	ids := make([]AccountID, 0, len(supervisor.accounts))
	for id := range supervisor.accounts {
		ids = append(ids, id)
	}
	supervisor.mu.RUnlock()
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (supervisor *Supervisor) Capacity() Capacity {
	if supervisor == nil {
		return Capacity{}
	}
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	return Capacity{
		Max: supervisor.config.MaxAccounts, Active: len(supervisor.accounts),
		Starting: len(supervisor.pending), Stopping: len(supervisor.stopping),
	}
}

func (supervisor *Supervisor) RefreshGameData(ctx context.Context) error {
	if supervisor == nil || supervisor.gameData == nil {
		return fmt.Errorf("shared official game data is unavailable")
	}
	supervisor.refreshMu.Lock()
	defer supervisor.refreshMu.Unlock()
	if err := supervisor.gameData.Refresh(ctx); err != nil {
		return err
	}
	supervisor.mu.RLock()
	applications := make([]*App.Application, 0, len(supervisor.accounts))
	for _, runtime := range supervisor.accounts {
		applications = append(applications, runtime.application)
	}
	supervisor.mu.RUnlock()
	var synchronizationErr error
	for _, application := range applications {
		if err := application.SynchronizeGameData(); err != nil {
			synchronizationErr = errors.Join(synchronizationErr, err)
		}
	}
	return synchronizationErr
}

func (supervisor *Supervisor) RemoveAccount(ctx context.Context, id AccountID) error {
	if supervisor == nil {
		return nil
	}
	supervisor.mu.Lock()
	runtime, exists := supervisor.accounts[id]
	newlyStopping := exists
	if exists {
		delete(supervisor.accounts, id)
		supervisor.stopping[id] = runtime
	} else {
		runtime, exists = supervisor.stopping[id]
	}
	supervisor.mu.Unlock()
	if !exists {
		return fmt.Errorf("account %q is not running", id)
	}

	var stopErr error
	if newlyStopping {
		// The last thing a runtime does before leaving the cell is to publish
		// its dashboard read model, so the frontend keeps rendering the final
		// situation while no runtime exists. Bounded: draining never waits on
		// a slow backend.
		if runtime.application.Checkpoints != nil {
			checkpointContext, cancelCheckpoint := context.WithTimeout(ctx, drainCheckpointTimeout)
			_ = runtime.application.Checkpoint(checkpointContext, PrivateMetrics.CheckpointReasonDrain)
			cancelCheckpoint()
		}
		// Withdraw sensor membership before potentially slow account teardown so
		// its uncompleted public-map lease can be reassigned immediately.
		if supervisor.worldMaps != nil {
			supervisor.worldMaps.UnregisterStormScanner(string(id))
		}
		// Cancellation ensures every account-owned worker begins draining before
		// we wait for durable stores to close.
		stopErr = runtime.application.Session.Stop(ctx)
		runtime.cancel()
	}
	waitErr := runtime.application.Wait(ctx)
	if waitErr == nil {
		supervisor.releaseStoppedAccount(id, runtime)
	} else {
		// Keep the profile directory reserved if the caller's shutdown deadline
		// expires. Release it only when the application really finishes.
		go func() {
			_ = runtime.application.Wait(context.Background())
			supervisor.releaseStoppedAccount(id, runtime)
		}()
	}
	return errors.Join(stopErr, waitErr)
}

func (supervisor *Supervisor) releaseStoppedAccount(id AccountID, runtime accountRuntime) {
	if supervisor.worldMaps != nil {
		supervisor.worldMaps.UnregisterStormScanner(string(id))
	}
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	current, exists := supervisor.stopping[id]
	if !exists || current.application != runtime.application {
		return
	}
	delete(supervisor.stopping, id)
	if owner, reserved := supervisor.dataDirs[runtime.application.DataDir]; reserved && owner == id {
		delete(supervisor.dataDirs, runtime.application.DataDir)
	}
}

// runPlayerRebinds migrates staging profiles onto their player-keyed
// directories as soon as the game identity is known (see PlayerDirs.go).
func (supervisor *Supervisor) runPlayerRebinds() {
	ticker := time.NewTicker(playerRebindSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-supervisor.ctx.Done():
			return
		case <-ticker.C:
			supervisor.rebindSweep()
		}
	}
}

// rebindSweep rebinds at most one runtime per tick so restarts stay calm even
// when a whole cell of fresh accounts resolves identity at the same time.
func (supervisor *Supervisor) rebindSweep() {
	supervisor.rebindMu.Lock()
	defer supervisor.rebindMu.Unlock()

	type candidate struct {
		id      AccountID
		runtime accountRuntime
	}
	supervisor.mu.RLock()
	if supervisor.closed {
		supervisor.mu.RUnlock()
		return
	}
	candidates := make([]candidate, 0, len(supervisor.accounts))
	for id, runtime := range supervisor.accounts {
		if runtime.config.DataDir != "" {
			// Explicit profile roots (the desktop N=1 composition) are never
			// migrated.
			continue
		}
		if _, bound := supervisor.playerBindings[string(id)]; bound {
			continue
		}
		candidates = append(candidates, candidate{id: id, runtime: runtime})
	}
	supervisor.mu.RUnlock()
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].id < candidates[right].id })

	for _, entry := range candidates {
		worldID, playerID, ok := supervisor.identityOf(entry.runtime.application)
		if !ok {
			continue
		}
		if err := supervisor.rebindAccount(entry.id, entry.runtime, worldID, playerID); err != nil {
			log.Printf("[accounts] rebind %s onto player %s failed: %v", entry.id, playerKey(worldID, playerID), err)
		}
		return
	}
}

// rebindAccount performs the one-time migration of a staging profile onto the
// player-keyed directory: stop, adopt or move the corpus, bind, restart.
func (supervisor *Supervisor) rebindAccount(id AccountID, runtime accountRuntime, worldID string, playerID int64) error {
	key := playerKey(worldID, playerID)
	staging := filepath.Join(supervisor.config.DataRoot, "Accounts", string(id))
	playerDir := filepath.Join(supervisor.config.DataRoot, playerDirsName, key)
	log.Printf("[accounts] rebinding %s onto player profile %s", id, key)

	stopContext, cancel := context.WithTimeout(context.Background(), playerRebindStopTimeout)
	defer cancel()
	if err := supervisor.RemoveAccount(stopContext, id); err != nil {
		return fmt.Errorf("stop for rebind: %w", err)
	}
	if err := supervisor.waitForDataDirRelease(stopContext, staging); err != nil {
		return err
	}

	if _, err := os.Stat(playerDir); os.IsNotExist(err) {
		// First time this player is seen: the staging corpus becomes the
		// player directory wholesale.
		if err := os.MkdirAll(filepath.Dir(playerDir), 0o700); err != nil {
			return fmt.Errorf("create player root: %w", err)
		}
		if err := os.Rename(staging, playerDir); err != nil {
			return fmt.Errorf("promote staging profile: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect player directory: %w", err)
	} else {
		// The player already has a corpus (for example the hosted account was
		// deleted and recreated): adopt it and fold the staging minutes in.
		stamp := strconv.FormatInt(supervisorNow().Unix(), 10)
		if err := mergeStagingIntoPlayerDir(staging, playerDir, stamp); err != nil {
			return fmt.Errorf("adopt player profile: %w", err)
		}
	}

	supervisor.mu.Lock()
	supervisor.playerBindings[string(id)] = key
	bindings := make(map[string]string, len(supervisor.playerBindings))
	for runtimeID, boundKey := range supervisor.playerBindings {
		bindings[runtimeID] = boundKey
	}
	path := supervisor.playerBindingsPath
	supervisor.mu.Unlock()
	if err := savePlayerBindings(path, bindings); err != nil {
		return err
	}

	if _, err := supervisor.AddAccount(context.Background(), runtime.config); err != nil {
		return fmt.Errorf("restart on player profile: %w", err)
	}
	log.Printf("[accounts] %s now runs on player profile %s", id, key)
	return nil
}

func (supervisor *Supervisor) waitForDataDirRelease(ctx context.Context, dataDir string) error {
	for {
		supervisor.mu.RLock()
		_, reserved := supervisor.dataDirs[dataDir]
		supervisor.mu.RUnlock()
		if !reserved {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("profile directory %s was not released in time", dataDir)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// supervisorNow is a seam for tests; production uses the wall clock.
var supervisorNow = time.Now

func (supervisor *Supervisor) accountDataDir(id AccountID, requested string) (string, error) {
	dataDir := strings.TrimSpace(requested)
	if dataDir == "" {
		// Profiles are keyed by game identity once it is known: a bound
		// runtime lands on the shared player directory, an unbound one stages
		// under its runtime ID until the first login reveals the player.
		if key := supervisor.playerBindingFor(string(id)); key != "" {
			dataDir = filepath.Join(supervisor.config.DataRoot, playerDirsName, key)
		} else {
			dataDir = filepath.Join(supervisor.config.DataRoot, "Accounts", string(id))
		}
	}
	resolved, err := filepath.Abs(dataDir)
	if err != nil {
		return "", fmt.Errorf("resolve account data directory: %w", err)
	}
	if resolved != supervisor.config.DataRoot &&
		!pathWithin(filepath.Join(supervisor.config.DataRoot, "Accounts"), resolved) &&
		!pathWithin(filepath.Join(supervisor.config.DataRoot, playerDirsName), resolved) {
		return "", fmt.Errorf("account data directory must be the desktop root or remain inside the account or player roots")
	}
	return resolved, nil
}

func (supervisor *Supervisor) playerBindingFor(runtimeID string) string {
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	return supervisor.playerBindings[runtimeID]
}

func pathWithin(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == "." {
		return err == nil
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (supervisor *Supervisor) Close(ctx context.Context) error {
	if supervisor == nil {
		return nil
	}
	supervisor.mu.Lock()
	supervisor.closed = true
	supervisor.mu.Unlock()
	supervisor.cancel()
	// Account construction owns profile leases too. Wait for every in-flight
	// constructor to either register or tear down before taking the close set.
	supervisor.addWG.Wait()
	ids := supervisor.AccountIDs()
	var closeErr error
	for _, id := range ids {
		if err := supervisor.RemoveAccount(ctx, id); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	supervisor.mu.RLock()
	stopping := make(map[AccountID]accountRuntime, len(supervisor.stopping))
	for id, runtime := range supervisor.stopping {
		stopping[id] = runtime
	}
	supervisor.mu.RUnlock()
	for id, runtime := range stopping {
		if err := runtime.application.Wait(ctx); err != nil {
			closeErr = errors.Join(closeErr, err)
			continue
		}
		supervisor.releaseStoppedAccount(id, runtime)
	}
	return errors.Join(closeErr, supervisor.worldMaps.Close(ctx))
}
