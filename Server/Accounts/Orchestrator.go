package Accounts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/History"
	"CitadelDesktop/Server/PrivateMetrics"
	"CitadelDesktop/Server/Reports"
	"CitadelDesktop/Server/Session"
	"CitadelDesktop/Server/State"
)

const (
	OrchestratorSchemaVersion     = 1
	maximumControlRequestBytes    = 1 << 20
	maximumConfigurationSyncBytes = 32 << 20
	maximumConfigurationSections  = 4096
	maximumPlacementLease         = 15 * time.Minute
	maximumDashboardGrant         = 15 * time.Minute
	maximumDashboardBootstrap     = 2 * time.Minute
	maximumPrivateMetricsGrant    = 15 * time.Minute
	controlStatusInterval         = 5 * time.Second
	controlHeartbeatInterval      = 30 * time.Second
	defaultRuntimeDrainTimeout    = 10 * time.Second
)

type OrchestratorConfig struct {
	CellID        string
	Token         string
	Supervisor    *Supervisor
	DashboardAuth *TenantAuthenticator
	DrainTimeout  time.Duration
	Now           func() time.Time
}

// RuntimeAssignment is one desired account runtime on this cell.
//
// A runtime lives for as long as it stays in the desired set: it is created by
// the first reconcile that names it and drained only by a reconcile that omits
// it (or by process shutdown). LeaseExpiresAt bounds the credential-bearing
// side channels of the placement — the private-metrics grant and, through their
// own expiry, dashboard grants — and marks how fresh the control plane's view
// of the placement is. A lapsed lease pauses those channels and is reported in
// status; it never stops the runtime or its game session.
type RuntimeAssignment struct {
	RuntimeID      string    `json:"runtimeId"`
	TenantID       string    `json:"tenantId"`
	PlacementEpoch uint64    `json:"placementEpoch"`
	LeaseExpiresAt time.Time `json:"leaseExpiresAt"`
	StartSession   bool      `json:"startSession"`
	// The exact account-owned control-plane configuration must be acknowledged
	// before this runtime may start a game session or execute automation.
	DesiredConfigurationRevision uint64 `json:"desiredConfigurationRevision,omitempty"`
	DesiredConfigurationDigest   string `json:"desiredConfigurationDigest,omitempty"`
	// OnDisconnect is retained for control-plane compatibility. Hosted runtimes
	// canonicalize both the omitted value and the legacy "release" value to
	// "hold" so their persisted dashboard data and configuration API remain
	// addressable while the game socket is disconnected.
	OnDisconnect string `json:"onDisconnect,omitempty"`
	// PrivateMetrics is a write-only control-plane grant. It is retained only
	// in memory and deliberately absent from RuntimeStatus and event streams.
	PrivateMetrics *PrivateMetrics.Grant `json:"privateMetrics,omitempty"`
}

type ReconcileRequest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Revision      uint64              `json:"revision"`
	Runtimes      []RuntimeAssignment `json:"runtimes"`
}

type DashboardGrantRequest struct {
	PlacementEpoch uint64    `json:"placementEpoch"`
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// DashboardBootstrapRequest carries a short-lived, one-use browser login
// credential. Unlike DashboardGrantRequest, its token is never valid as an API
// bearer and is removed after the first successful tenant login.
type DashboardBootstrapRequest struct {
	PlacementEpoch uint64    `json:"placementEpoch"`
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// LoginCredentialRequest installs the game login and server selection the
// backend holds for a hosted account into one exact runtime placement. It is
// a separate no-store request so ordinary reconciles never carry a password.
// The body is decoded, written to the runtime's protected saved-login file,
// and never logged, echoed, or retained by the orchestrator.
type LoginCredentialRequest struct {
	PlacementEpoch uint64 `json:"placementEpoch"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	// Server is the game world code such as US1.
	Server string `json:"server"`
	// ServerURL and Zone optionally carry the world's resolved websocket
	// endpoint and SmartFox zone from the control plane's game server
	// directory. When present they override the runtime's own catalog, which
	// is how a world opened after this build ships still connects; when
	// absent the runtime resolves the code itself.
	ServerURL string `json:"serverUrl,omitempty"`
	Zone      string `json:"zone,omitempty"`
	Language  string `json:"language,omitempty"`
}

// ConfigurationSyncRequest installs the account-owned control-plane snapshot
// into one exact runtime placement. The control-plane revision is deliberately
// independent of the runtime store's local revision: it fences retries and
// stale writers while the runtime atomically adopts the exact portable state.
type ConfigurationSyncRequest struct {
	SchemaVersion  int                    `json:"schemaVersion"`
	PlacementEpoch uint64                 `json:"placementEpoch"`
	Digest         string                 `json:"digest"`
	Configuration  Configuration.Snapshot `json:"configuration"`
}

// BackgroundLoginSummary is the sanitized saved-login state of a runtime: it
// says whether a login is installed and for which server, never who or what.
type BackgroundLoginSummary struct {
	Configured bool       `json:"configured"`
	Server     string     `json:"server,omitempty"`
	Language   string     `json:"language,omitempty"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

const (
	PlacementLeaseActive = "active"
	PlacementLeaseLapsed = "lapsed"
)

type RuntimeStatus struct {
	RuntimeID      string    `json:"runtimeId"`
	TenantID       string    `json:"tenantId"`
	PlacementEpoch uint64    `json:"placementEpoch"`
	LeaseExpiresAt time.Time `json:"leaseExpiresAt"`
	// PlacementLease is "active" while the lease is current and "lapsed" once
	// it expired without renewal. The runtime keeps running either way.
	PlacementLease       string `json:"placementLease"`
	Lifecycle            string `json:"lifecycle"`
	DesiredSession       bool   `json:"desiredSession"`
	OnDisconnect         string `json:"onDisconnect"`
	GameDataReady        bool   `json:"gameDataReady"`
	SessionState         string `json:"sessionState"`
	LoggedIn             bool   `json:"loggedIn"`
	SocketReady          bool   `json:"socketReady"`
	Generation           uint64 `json:"generation"`
	BaselineGeneration   uint64 `json:"baselineGeneration"`
	ConnectionGeneration uint64 `json:"connectionGeneration"`
	// CooldownUntil, RetryAt, and LoginFailure describe the current login
	// obstacle so the control plane can wait out a cooldown, hand a suspended
	// or rejected account back to the user, or re-place a runtime that fails
	// for a reason it knows is neither.
	CooldownUntil                   *time.Time              `json:"cooldownUntil,omitempty"`
	RetryAt                         *time.Time              `json:"retryAt,omitempty"`
	LoginFailure                    *State.LoginFailure     `json:"loginFailure,omitempty"`
	BackgroundLogin                 *BackgroundLoginSummary `json:"backgroundLogin,omitempty"`
	PrivateMetricsState             string                  `json:"privateMetricsState,omitempty"`
	PrivateMetricsAt                *time.Time              `json:"privateMetricsAt,omitempty"`
	StatsMigrationState             string                  `json:"statsMigrationState,omitempty"`
	StatsMigrationSourceReports     int64                   `json:"statsMigrationSourceReports,omitempty"`
	StatsMigrationSourceBuckets     int64                   `json:"statsMigrationSourceBuckets,omitempty"`
	StatsMigrationPendingBuckets    int64                   `json:"statsMigrationPendingBuckets,omitempty"`
	StatsMigrationOldestAt          *time.Time              `json:"statsMigrationOldestAt,omitempty"`
	StatsMigrationNewestAt          *time.Time              `json:"statsMigrationNewestAt,omitempty"`
	StatsMigrationPendingFrom       *time.Time              `json:"statsMigrationPendingFrom,omitempty"`
	StatsMigrationPendingThrough    *time.Time              `json:"statsMigrationPendingThrough,omitempty"`
	CheckpointState                 string                  `json:"checkpointState,omitempty"`
	CheckpointAt                    *time.Time              `json:"checkpointAt,omitempty"`
	CheckpointRevision              uint64                  `json:"checkpointRevision,omitempty"`
	CheckpointConfigurationRevision uint64                  `json:"checkpointConfigurationRevision,omitempty"`
	DesiredConfigurationRevision    uint64                  `json:"desiredConfigurationRevision,omitempty"`
	DesiredConfigurationDigest      string                  `json:"desiredConfigurationDigest,omitempty"`
	AppliedConfigurationRevision    uint64                  `json:"appliedConfigurationRevision,omitempty"`
	AppliedConfigurationDigest      string                  `json:"appliedConfigurationDigest,omitempty"`
	ConfigurationState              string                  `json:"configurationState,omitempty"`
}

type CellStatus struct {
	SchemaVersion   int             `json:"schemaVersion"`
	Version         string          `json:"version"`
	BuildRevision   string          `json:"buildRevision"`
	BuildID         string          `json:"buildId"`
	CellID          string          `json:"cellId"`
	DesiredRevision uint64          `json:"desiredRevision"`
	GameDataReady   bool            `json:"gameDataReady"`
	Capacity        Capacity        `json:"capacity"`
	Runtimes        []RuntimeStatus `json:"runtimes"`
	ObservedAt      time.Time       `json:"observedAt"`
}

type Orchestrator struct {
	cellID        string
	tokenHash     [sha256.Size]byte
	supervisor    *Supervisor
	dashboardAuth *TenantAuthenticator
	drainTimeout  time.Duration
	now           func() time.Time

	reconcileMu        sync.Mutex
	mu                 sync.RWMutex
	revision           uint64
	runtimes           map[AccountID]RuntimeAssignment
	configurationSyncs map[AccountID]configurationSyncState

	subscriberMu sync.Mutex
	subscribers  map[chan CellStatus]struct{}
	startOnce    sync.Once
}

type configurationSyncState struct {
	placementEpoch uint64
	revision       uint64
	digest         string
	application    *App.Application
}

type orchestratorError struct {
	status int
	code   string
	err    error
}

func (controlErr *orchestratorError) Error() string {
	if controlErr == nil || controlErr.err == nil {
		return "orchestrator request failed"
	}
	return controlErr.err.Error()
}

func NewOrchestrator(config OrchestratorConfig) (*Orchestrator, error) {
	cellID, err := ParseAccountID(config.CellID)
	if err != nil || string(cellID) != config.CellID {
		return nil, fmt.Errorf("cell id must use the canonical runtime identifier format")
	}
	if len(config.Token) < minimumTenantTokenLength {
		return nil, fmt.Errorf("orchestrator token must contain at least %d characters", minimumTenantTokenLength)
	}
	if config.Supervisor == nil || config.DashboardAuth == nil {
		return nil, fmt.Errorf("orchestrator needs a supervisor and dashboard authenticator")
	}
	drainTimeout := config.DrainTimeout
	if drainTimeout <= 0 {
		drainTimeout = defaultRuntimeDrainTimeout
	}
	now := config.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Orchestrator{
		cellID: string(cellID), tokenHash: sha256.Sum256([]byte(config.Token)),
		supervisor: config.Supervisor, dashboardAuth: config.DashboardAuth,
		drainTimeout: drainTimeout, now: now,
		runtimes: map[AccountID]RuntimeAssignment{}, configurationSyncs: map[AccountID]configurationSyncState{},
		subscribers: map[chan CellStatus]struct{}{},
	}, nil
}

func (orchestrator *Orchestrator) Start(ctx context.Context) {
	if orchestrator == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	orchestrator.startOnce.Do(func() { go orchestrator.run(ctx) })
}

// run republishes cell status on a fixed cadence so lease lapses, session
// changes, and private-metrics state reach subscribers without a reconcile.
// Nothing here ever changes desired state: a lapsed lease is only observed.
func (orchestrator *Orchestrator) run(ctx context.Context) {
	ticker := time.NewTicker(controlStatusInterval)
	defer ticker.Stop()
	orchestrator.publishStatus()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			orchestrator.publishStatus()
		}
	}
}

func (orchestrator *Orchestrator) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /orchestrator/v1/status", orchestrator.handleStatus)
	mux.HandleFunc("GET /orchestrator/v1/events", orchestrator.handleEvents)
	mux.HandleFunc("POST /orchestrator/v1/reconcile", orchestrator.handleReconcile)
	mux.HandleFunc("PUT /orchestrator/v1/runtimes/{id}/dashboard-grant", orchestrator.handleDashboardGrant)
	mux.HandleFunc("PUT /orchestrator/v1/runtimes/{id}/dashboard-bootstrap", orchestrator.handleDashboardBootstrap)
	mux.HandleFunc("PUT /orchestrator/v1/runtimes/{id}/login", orchestrator.handleLoginCredential)
	mux.HandleFunc("PUT /orchestrator/v1/runtimes/{id}/configuration", orchestrator.handleConfigurationSync)
	mux.HandleFunc("DELETE /orchestrator/v1/runtimes/{id}/login", orchestrator.handleLoginRevocation)
	mux.HandleFunc("POST /orchestrator/v1/runtimes/{id}/reconnect", orchestrator.handleReconnect)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		setControlSecurityHeaders(writer)
		if !orchestrator.authenticate(request) {
			writeControlError(writer, http.StatusUnauthorized, "orchestrator_authentication_required")
			return
		}
		mux.ServeHTTP(writer, request)
	})
}

func (orchestrator *Orchestrator) authenticate(request *http.Request) bool {
	if orchestrator == nil || request == nil {
		return false
	}
	value := strings.TrimSpace(request.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return false
	}
	candidate := sha256.Sum256([]byte(value[len(prefix):]))
	return subtle.ConstantTimeCompare(candidate[:], orchestrator.tokenHash[:]) == 1
}

func (orchestrator *Orchestrator) handleStatus(writer http.ResponseWriter, _ *http.Request) {
	writeControlJSON(writer, http.StatusOK, orchestrator.Status())
}

func (orchestrator *Orchestrator) handleReconcile(writer http.ResponseWriter, request *http.Request) {
	var desired ReconcileRequest
	if err := decodeControlJSON(writer, request, &desired); err != nil {
		return
	}
	status, err := orchestrator.Reconcile(request.Context(), desired)
	if err != nil {
		writeOrchestratorError(writer, err)
		return
	}
	writeControlJSON(writer, http.StatusOK, status)
}

func (orchestrator *Orchestrator) Reconcile(ctx context.Context, desired ReconcileRequest) (CellStatus, error) {
	if orchestrator == nil {
		return CellStatus{}, &orchestratorError{status: http.StatusServiceUnavailable, code: "orchestrator_unavailable", err: errors.New("orchestrator is unavailable")}
	}
	orchestrator.reconcileMu.Lock()
	defer orchestrator.reconcileMu.Unlock()
	now := orchestrator.now().UTC()
	normalized, err := orchestrator.validateReconcile(desired, now)
	if err != nil {
		return CellStatus{}, err
	}

	orchestrator.mu.RLock()
	currentRevision := orchestrator.revision
	current := cloneAssignments(orchestrator.runtimes)
	orchestrator.mu.RUnlock()
	if normalized.Revision < currentRevision {
		return CellStatus{}, &orchestratorError{status: http.StatusConflict, code: "stale_desired_revision", err: fmt.Errorf("desired revision %d is older than %d", normalized.Revision, currentRevision)}
	}
	desiredByID := assignmentsByID(normalized.Runtimes)
	if normalized.Revision == currentRevision && currentRevision != 0 {
		if !sameAssignments(current, desiredByID) {
			return CellStatus{}, &orchestratorError{status: http.StatusConflict, code: "desired_revision_conflict", err: errors.New("desired revision was reused with different assignments")}
		}
		return orchestrator.Status(), nil
	}
	for id, assignment := range desiredByID {
		if existing, exists := current[id]; exists {
			if existing.TenantID != assignment.TenantID {
				return CellStatus{}, &orchestratorError{status: http.StatusConflict, code: "runtime_owner_conflict", err: fmt.Errorf("runtime %q cannot change tenant ownership", id)}
			}
			if assignment.PlacementEpoch < existing.PlacementEpoch {
				return CellStatus{}, &orchestratorError{status: http.StatusConflict, code: "stale_placement_epoch", err: fmt.Errorf("runtime %q placement epoch is stale", id)}
			}
		}
	}

	added := make([]AccountID, 0)
	for id, assignment := range desiredByID {
		if _, exists := current[id]; exists {
			continue
		}
		if _, err := orchestrator.supervisor.AddAccount(ctx, AccountConfig{
			ID: string(id), BackgroundOnly: true, StartSession: false,
			ControlConfigurationRequired: assignment.DesiredConfigurationRevision > 0,
			ControlConfigurationReady:    false,
			PrivateMetricsPlacement:      orchestrator.privateMetricsPlacement(assignment, normalized.Revision),
		}); err != nil {
			orchestrator.rollbackAdded(added)
			return CellStatus{}, &orchestratorError{status: http.StatusServiceUnavailable, code: "runtime_start_failed", err: fmt.Errorf("start runtime %q: %w", id, err)}
		}
		if err := orchestrator.dashboardAuth.RegisterRuntime(id); err != nil {
			orchestrator.drainRuntime(id)
			orchestrator.rollbackAdded(added)
			return CellStatus{}, &orchestratorError{status: http.StatusInternalServerError, code: "runtime_auth_failed", err: err}
		}
		added = append(added, id)
		_ = assignment
	}

	removed := make([]AccountID, 0)
	for id := range current {
		if _, retained := desiredByID[id]; !retained {
			removed = append(removed, id)
		}
	}
	// Close the old-configuration execution window before publishing the new
	// desired tuple or performing any slower placement/drain bookkeeping.
	// Reconcile and sync share reconcileMu, so no acknowledgement can cross
	// this transition.
	for id, assignment := range desiredByID {
		if application, exists := orchestrator.supervisor.Application(id); exists && application != nil {
			application.SetControlConfigurationReady(
				assignment.DesiredConfigurationRevision > 0,
				orchestrator.configurationReady(id, assignment),
			)
		}
	}
	orchestrator.mu.Lock()
	orchestrator.revision = normalized.Revision
	orchestrator.runtimes = desiredByID
	for id, assignment := range desiredByID {
		if assignment.DesiredConfigurationRevision == 0 {
			delete(orchestrator.configurationSyncs, id)
		}
	}
	for _, id := range removed {
		delete(orchestrator.configurationSyncs, id)
	}
	orchestrator.mu.Unlock()
	for id, assignment := range desiredByID {
		if err := orchestrator.supervisor.SetPrivateMetricsPlacement(
			id, orchestrator.privateMetricsPlacement(assignment, normalized.Revision),
		); err != nil {
			return CellStatus{}, &orchestratorError{
				status: http.StatusInternalServerError, code: "private_metrics_configuration_failed",
				err: fmt.Errorf("configure runtime %q private metrics: %w", id, err),
			}
		}
	}
	for _, id := range removed {
		orchestrator.dashboardAuth.RevokeRuntime(id)
		orchestrator.drainRuntime(id)
	}
	for id, assignment := range desiredByID {
		application, exists := orchestrator.supervisor.Application(id)
		if !exists || application == nil {
			continue
		}
		configurationReady := orchestrator.configurationReady(id, assignment)
		application.SetControlConfigurationReady(assignment.DesiredConfigurationRevision > 0, configurationReady)
		if application.Session == nil {
			continue
		}
		application.SetSessionReconnectPolicy(Session.ReconnectPolicy(assignment.OnDisconnect))
		if !assignment.StartSession {
			stopContext, cancel := context.WithTimeout(context.Background(), orchestrator.drainTimeout)
			_ = application.Session.Stop(stopContext)
			cancel()
			continue
		}
		if configurationReady {
			orchestrator.startSessionIfDesired(id, assignment.PlacementEpoch)
			continue
		}
		// A newly placed runtime was created with StartSession=false above and
		// remains parked until its first configuration sync. An existing session
		// stays connected while its execution gate pauses every mutation; this
		// avoids disconnecting the account for an ordinary dashboard save.
	}
	orchestrator.publishStatus()
	return orchestrator.Status(), nil
}

func (orchestrator *Orchestrator) validateReconcile(desired ReconcileRequest, now time.Time) (ReconcileRequest, error) {
	if desired.SchemaVersion != OrchestratorSchemaVersion {
		return desired, &orchestratorError{status: http.StatusBadRequest, code: "unsupported_schema_version", err: fmt.Errorf("schema version %d is unsupported", desired.SchemaVersion)}
	}
	if desired.Revision == 0 {
		return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_desired_revision", err: errors.New("desired revision must be positive")}
	}
	capacity := orchestrator.supervisor.Capacity()
	if capacity.Max > 0 && len(desired.Runtimes) > capacity.Max {
		return desired, &orchestratorError{status: http.StatusUnprocessableEntity, code: "runtime_limit_exceeded", err: fmt.Errorf("desired runtime count exceeds cell limit of %d", capacity.Max)}
	}
	seen := make(map[AccountID]struct{}, len(desired.Runtimes))
	seenMetricsGrants := make(map[[sha256.Size]byte]AccountID, len(desired.Runtimes))
	currentMetricsGrants := make(map[[sha256.Size]byte]AccountID)
	orchestrator.mu.RLock()
	for id, assignment := range orchestrator.runtimes {
		if assignment.PrivateMetrics != nil {
			currentMetricsGrants[sha256.Sum256([]byte(assignment.PrivateMetrics.Token))] = id
		}
	}
	orchestrator.mu.RUnlock()
	for index := range desired.Runtimes {
		assignment := &desired.Runtimes[index]
		id, err := ParseAccountID(assignment.RuntimeID)
		if err != nil || string(id) != assignment.RuntimeID {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_runtime_id", err: fmt.Errorf("runtimes[%d].runtimeId is invalid", index)}
		}
		tenantID, err := ParseAccountID(assignment.TenantID)
		if err != nil || string(tenantID) != assignment.TenantID {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_tenant_id", err: fmt.Errorf("runtimes[%d].tenantId is invalid", index)}
		}
		if _, duplicate := seen[id]; duplicate {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "duplicate_runtime", err: fmt.Errorf("runtime %q appears more than once", id)}
		}
		if assignment.PlacementEpoch == 0 {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_placement_epoch", err: fmt.Errorf("runtime %q placement epoch must be positive", id)}
		}
		if assignment.DesiredConfigurationRevision == 0 {
			if assignment.DesiredConfigurationDigest != "" {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_configuration_digest", err: fmt.Errorf("runtime %q has a configuration digest without a revision", id)}
			}
		} else if !validConfigurationDigest(assignment.DesiredConfigurationDigest) {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_configuration_digest", err: fmt.Errorf("runtime %q configuration digest is invalid", id)}
		}
		assignment.LeaseExpiresAt = assignment.LeaseExpiresAt.UTC()
		if !assignment.LeaseExpiresAt.After(now) || assignment.LeaseExpiresAt.After(now.Add(maximumPlacementLease)) {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_placement_lease", err: fmt.Errorf("runtime %q placement lease must expire within %s", id, maximumPlacementLease)}
		}
		if _, ok := Session.ParseReconnectPolicy(assignment.OnDisconnect); !ok {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_disconnect_policy", err: fmt.Errorf("runtime %q disconnect policy must be hold or release", id)}
		}
		// A tenant dashboard is a durable control surface, not a view whose
		// lifetime follows the game connection. Accept old release assignments so
		// an existing desired document upgrades cleanly, but migrate them to hold.
		assignment.OnDisconnect = string(Session.ReconnectPolicyHold)
		if orchestrator.supervisor.PrivateMetricsEnabled() {
			if assignment.PrivateMetrics == nil {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "private_metrics_grant_required", err: fmt.Errorf("runtime %q requires a private metrics grant", id)}
			}
			grant := assignment.PrivateMetrics
			grant.ExpiresAt = grant.ExpiresAt.UTC()
			if len(grant.Token) < minimumTenantTokenLength || strings.TrimSpace(grant.Token) != grant.Token ||
				!grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(maximumPrivateMetricsGrant)) ||
				grant.ExpiresAt.After(assignment.LeaseExpiresAt) {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_private_metrics_grant", err: fmt.Errorf("runtime %q private metrics grant is invalid", id)}
			}
			digest := sha256.Sum256([]byte(grant.Token))
			if subtle.ConstantTimeCompare(digest[:], orchestrator.tokenHash[:]) == 1 {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_private_metrics_grant", err: fmt.Errorf("runtime %q private metrics grant conflicts with the control credential", id)}
			}
			if orchestrator.dashboardAuth.hasDashboardGrantDigest(digest) {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "invalid_private_metrics_grant", err: fmt.Errorf("runtime %q private metrics grant conflicts with a dashboard credential", id)}
			}
			if owner, duplicate := seenMetricsGrants[digest]; duplicate {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "duplicate_private_metrics_grant", err: fmt.Errorf("runtimes %q and %q reuse a private metrics grant", owner, id)}
			}
			if owner, reserved := currentMetricsGrants[digest]; reserved && owner != id {
				return desired, &orchestratorError{status: http.StatusBadRequest, code: "private_metrics_grant_reassigned", err: fmt.Errorf("runtime %q private metrics grant remains reserved for %q", id, owner)}
			}
			seenMetricsGrants[digest] = id
		} else if assignment.PrivateMetrics != nil {
			return desired, &orchestratorError{status: http.StatusBadRequest, code: "private_metrics_unavailable", err: fmt.Errorf("runtime %q provided a private metrics grant while publishing is disabled", id)}
		}
		seen[id] = struct{}{}
	}
	return desired, nil
}

func (orchestrator *Orchestrator) handleDashboardGrant(writer http.ResponseWriter, request *http.Request) {
	rawID := request.PathValue("id")
	id, err := ParseAccountID(rawID)
	if err != nil || string(id) != rawID {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	var grant DashboardGrantRequest
	if err := decodeControlJSON(writer, request, &grant); err != nil {
		return
	}
	now := orchestrator.now().UTC()
	grant.ExpiresAt = grant.ExpiresAt.UTC()
	if grant.PlacementEpoch == 0 || !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(maximumDashboardGrant)) {
		writeControlError(writer, http.StatusBadRequest, "invalid_dashboard_grant")
		return
	}
	digest := sha256.Sum256([]byte(grant.Token))
	if orchestrator.reservedCredentialDigest(digest) {
		writeControlError(writer, http.StatusBadRequest, "invalid_dashboard_grant")
		return
	}
	orchestrator.mu.RLock()
	assignment, exists := orchestrator.runtimes[id]
	orchestrator.mu.RUnlock()
	if !exists {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	if assignment.PlacementEpoch != grant.PlacementEpoch {
		writeControlError(writer, http.StatusConflict, "stale_placement_epoch")
		return
	}
	if err := orchestrator.dashboardAuth.SetDashboardGrant(id, grant.Token, grant.ExpiresAt); err != nil {
		writeControlError(writer, http.StatusBadRequest, "invalid_dashboard_grant")
		return
	}
	orchestrator.publishStatus()
	writeControlJSON(writer, http.StatusOK, map[string]any{
		"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch,
		"expiresAt": grant.ExpiresAt,
	})
}

func (orchestrator *Orchestrator) handleDashboardBootstrap(writer http.ResponseWriter, request *http.Request) {
	rawID := request.PathValue("id")
	id, err := ParseAccountID(rawID)
	if err != nil || string(id) != rawID {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	var bootstrap DashboardBootstrapRequest
	if err := decodeControlJSON(writer, request, &bootstrap); err != nil {
		return
	}
	now := orchestrator.now().UTC()
	bootstrap.ExpiresAt = bootstrap.ExpiresAt.UTC()
	if bootstrap.PlacementEpoch == 0 || len(bootstrap.Token) < minimumTenantTokenLength ||
		strings.TrimSpace(bootstrap.Token) != bootstrap.Token || !bootstrap.ExpiresAt.After(now) ||
		bootstrap.ExpiresAt.After(now.Add(maximumDashboardBootstrap)) {
		writeControlError(writer, http.StatusBadRequest, "invalid_dashboard_bootstrap")
		return
	}
	digest := sha256.Sum256([]byte(bootstrap.Token))
	if orchestrator.reservedCredentialDigest(digest) || orchestrator.dashboardAuth.hasDashboardGrantDigest(digest) {
		writeControlError(writer, http.StatusBadRequest, "invalid_dashboard_bootstrap")
		return
	}
	orchestrator.mu.RLock()
	assignment, exists := orchestrator.runtimes[id]
	orchestrator.mu.RUnlock()
	if !exists {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	if assignment.PlacementEpoch != bootstrap.PlacementEpoch {
		writeControlError(writer, http.StatusConflict, "stale_placement_epoch")
		return
	}
	if !assignment.LeaseExpiresAt.After(now) || bootstrap.ExpiresAt.After(assignment.LeaseExpiresAt) {
		writeControlError(writer, http.StatusConflict, "placement_lease_lapsed")
		return
	}
	if err := orchestrator.dashboardAuth.SetDashboardBootstrap(id, bootstrap.Token, bootstrap.ExpiresAt); err != nil {
		writeControlError(writer, http.StatusBadRequest, "invalid_dashboard_bootstrap")
		return
	}
	writeControlJSON(writer, http.StatusCreated, map[string]any{
		"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch,
		"expiresAt": bootstrap.ExpiresAt,
	})
}

// handleLoginCredential installs or rotates a runtime's saved game login and
// server selection. The runtime keeps its own protected copy; when the
// assignment desires a session, the session is restarted so the new
// credential is used immediately (a stale credential's fatal login error is
// cleared by the restart).
func (orchestrator *Orchestrator) handleLoginCredential(writer http.ResponseWriter, request *http.Request) {
	rawID := request.PathValue("id")
	id, err := ParseAccountID(rawID)
	if err != nil || string(id) != rawID {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	var credential LoginCredentialRequest
	if err := decodeControlJSON(writer, request, &credential); err != nil {
		return
	}
	orchestrator.reconcileMu.Lock()
	defer orchestrator.reconcileMu.Unlock()
	orchestrator.mu.RLock()
	assignment, exists := orchestrator.runtimes[id]
	orchestrator.mu.RUnlock()
	if !exists {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	if credential.PlacementEpoch == 0 || assignment.PlacementEpoch != credential.PlacementEpoch {
		writeControlError(writer, http.StatusConflict, "stale_placement_epoch")
		return
	}
	if !orchestrator.configurationReady(id, assignment) {
		writeControlError(writer, http.StatusConflict, "configuration_not_ready")
		return
	}
	application, running := orchestrator.supervisor.Application(id)
	if !running || application == nil || application.BackgroundLogin == nil {
		writeControlError(writer, http.StatusServiceUnavailable, "runtime_not_running")
		return
	}
	login, err := application.BackgroundLogin.Configure(Session.BackgroundLoginInput{
		Username: credential.Username, Password: credential.Password,
		Server: credential.Server, ServerURL: credential.ServerURL, Zone: credential.Zone,
		Language: credential.Language,
	})
	if err != nil {
		writeControlError(writer, http.StatusBadRequest, "invalid_login_credential")
		return
	}
	if assignment.StartSession && application.Session != nil {
		restartContext, cancel := context.WithTimeout(request.Context(), orchestrator.drainTimeout)
		_ = application.Session.Stop(restartContext)
		cancel()
		orchestrator.startSessionIfDesired(id, assignment.PlacementEpoch)
	}
	orchestrator.publishStatus()
	writeControlJSON(writer, http.StatusOK, map[string]any{
		"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch,
		"backgroundLogin": BackgroundLoginSummary{
			Configured: login.Configured, Server: login.Server, Language: login.Language, UpdatedAt: login.UpdatedAt,
		},
	})
}

// handleConfigurationSync installs the backend's canonical account settings
// before a hosted game session is allowed to run. Placement epoch plus the
// control-plane revision/digest make retries idempotent and reject both an old
// cell placement and an old desired document.
func (orchestrator *Orchestrator) handleConfigurationSync(writer http.ResponseWriter, request *http.Request) {
	rawID := request.PathValue("id")
	id, err := ParseAccountID(rawID)
	if err != nil || string(id) != rawID {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	var input ConfigurationSyncRequest
	if err := decodeControlJSONLimit(writer, request, &input, maximumConfigurationSyncBytes); err != nil {
		return
	}
	if input.SchemaVersion != OrchestratorSchemaVersion ||
		input.Configuration.SchemaVersion != Configuration.SchemaVersion ||
		input.Configuration.Revision == 0 || !validConfigurationDigest(input.Digest) ||
		len(input.Configuration.Sections) > maximumConfigurationSections {
		writeControlError(writer, http.StatusBadRequest, "invalid_configuration")
		return
	}
	canonical, digest, err := canonicalConfigurationDigest(input.Configuration)
	if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(input.Digest)) != 1 {
		writeControlError(writer, http.StatusBadRequest, "invalid_configuration")
		return
	}
	for _, section := range []string{
		History.PlayerSamplesConfigurationSection,
		Reports.BattleResearchConfigurationSection,
	} {
		if _, installationScoped := canonical[section]; installationScoped {
			writeControlError(writer, http.StatusUnprocessableEntity, "installation_scoped_configuration")
			return
		}
	}

	// Reconcile and configuration acknowledgement are one authority state
	// machine. Sharing the lock prevents a pending reconcile from overwriting a
	// successful sync's ready gate (or the inverse).
	orchestrator.reconcileMu.Lock()
	defer orchestrator.reconcileMu.Unlock()
	orchestrator.mu.RLock()
	assignment, exists := orchestrator.runtimes[id]
	previous := orchestrator.configurationSyncs[id]
	orchestrator.mu.RUnlock()
	if !exists {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	if input.PlacementEpoch == 0 || assignment.PlacementEpoch != input.PlacementEpoch {
		writeControlError(writer, http.StatusConflict, "stale_placement_epoch")
		return
	}
	if assignment.DesiredConfigurationRevision != input.Configuration.Revision ||
		assignment.DesiredConfigurationDigest != input.Digest {
		writeControlError(writer, http.StatusConflict, "configuration_not_desired")
		return
	}
	application, running := orchestrator.supervisor.Application(id)
	if !running || application == nil || application.Configuration == nil {
		writeControlError(writer, http.StatusServiceUnavailable, "runtime_not_running")
		return
	}
	if previous.placementEpoch == input.PlacementEpoch {
		switch {
		case input.Configuration.Revision < previous.revision:
			writeControlError(writer, http.StatusConflict, "stale_configuration_revision")
			return
		case input.Configuration.Revision == previous.revision && input.Digest != previous.digest:
			writeControlError(writer, http.StatusConflict, "configuration_revision_conflict")
			return
		case input.Configuration.Revision == previous.revision && previous.application == application:
			application.SetControlConfigurationReady(true, true)
			if assignment.StartSession && application.Session != nil {
				orchestrator.startSessionIfDesired(id, assignment.PlacementEpoch)
			}
			writeControlJSON(writer, http.StatusOK, map[string]any{
				"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch,
				"appliedConfigurationRevision": previous.revision,
				"appliedConfigurationDigest":   previous.digest, "configurationState": "ready",
			})
			return
		}
	}
	replacement := App.DefaultConfigurationSections()
	for section, value := range canonical {
		replacement[section] = value
	}
	snapshot, changed, err := application.Configuration.ReplaceAllAuthoritative(
		replacement,
		History.PlayerSamplesConfigurationSection,
		Reports.BattleResearchConfigurationSection,
	)
	if err != nil {
		writeControlError(writer, http.StatusUnprocessableEntity, "configuration_apply_failed")
		return
	}
	orchestrator.mu.Lock()
	orchestrator.configurationSyncs[id] = configurationSyncState{
		placementEpoch: input.PlacementEpoch, revision: input.Configuration.Revision,
		digest: input.Digest, application: application,
	}
	orchestrator.mu.Unlock()
	application.SetControlConfigurationReady(true, true)
	// Reconcile normally starts the session on its next pass. This extra guard
	// supports an idempotent controller retry where StartSession was already
	// desired but the first sync attempt failed.
	if assignment.StartSession && application.Session != nil {
		orchestrator.startSessionIfDesired(id, assignment.PlacementEpoch)
	}
	orchestrator.publishStatus()
	writeControlJSON(writer, http.StatusOK, map[string]any{
		"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch,
		"appliedConfigurationRevision": input.Configuration.Revision,
		"appliedConfigurationDigest":   input.Digest, "configurationState": "ready",
		"runtimeConfigurationRevision": snapshot.Revision, "changedSections": changed,
	})
}

// handleReconnect forces a fresh game connection for a running runtime now —
// the Account Center "Reconnect" action — bypassing a scheduled retry, cooldown
// wait, or login park. The runtime keeps its own decision about the outcome
// and reports it through status as usual.
func (orchestrator *Orchestrator) handleReconnect(writer http.ResponseWriter, request *http.Request) {
	rawID := request.PathValue("id")
	id, err := ParseAccountID(rawID)
	if err != nil || string(id) != rawID {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	orchestrator.reconcileMu.Lock()
	defer orchestrator.reconcileMu.Unlock()
	orchestrator.mu.RLock()
	assignment, exists := orchestrator.runtimes[id]
	orchestrator.mu.RUnlock()
	if !exists {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	if !orchestrator.configurationReady(id, assignment) {
		writeControlError(writer, http.StatusConflict, "configuration_not_ready")
		return
	}
	application, running := orchestrator.supervisor.Application(id)
	if !running || application == nil || application.Session == nil {
		writeControlError(writer, http.StatusServiceUnavailable, "runtime_not_running")
		return
	}
	reconnectContext, cancel := context.WithTimeout(request.Context(), orchestrator.drainTimeout)
	err = application.Session.Reconnect(reconnectContext)
	cancel()
	if err != nil {
		writeControlJSON(writer, http.StatusConflict, map[string]any{
			"error":     map[string]string{"code": "reconnect_refused"},
			"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch,
			"detail": sanitizeReconnectDetail(err),
		})
		return
	}
	orchestrator.publishStatus()
	writeControlJSON(writer, http.StatusAccepted, map[string]any{
		"runtimeId": string(id), "placementEpoch": assignment.PlacementEpoch, "reconnecting": true,
	})
}

// sanitizeReconnectDetail keeps only the stable, credential-free reasons a
// reconnect can be refused.
func sanitizeReconnectDetail(err error) string {
	switch {
	case errors.Is(err, Session.ErrLoginParked):
		return "login_parked"
	case errors.Is(err, Session.ErrReconnectScheduled):
		return "reconnect_scheduled"
	case errors.Is(err, Session.ErrTransportUnavailable):
		return "transport_unavailable"
	default:
		return "session_start_failed"
	}
}

// handleLoginRevocation removes every saved game login for a runtime from this
// cell and parks its session. It also works for a runtime that has already
// been drained, so orchestration can scrub credentials before relocating or
// deleting an account.
func (orchestrator *Orchestrator) handleLoginRevocation(writer http.ResponseWriter, request *http.Request) {
	rawID := request.PathValue("id")
	id, err := ParseAccountID(rawID)
	if err != nil || string(id) != rawID {
		writeControlError(writer, http.StatusNotFound, "runtime_not_found")
		return
	}
	if application, running := orchestrator.supervisor.Application(id); running && application != nil {
		if application.Session != nil {
			stopContext, cancel := context.WithTimeout(request.Context(), orchestrator.drainTimeout)
			_ = application.Session.Stop(stopContext)
			cancel()
		}
		if application.BackgroundLogin != nil {
			if err := application.BackgroundLogin.Clear(); err != nil {
				writeControlError(writer, http.StatusInternalServerError, "login_revocation_failed")
				return
			}
		}
	} else {
		cleared, err := orchestrator.supervisor.ClearSavedLogins(id)
		if err != nil {
			writeControlError(writer, http.StatusInternalServerError, "login_revocation_failed")
			return
		}
		if !cleared {
			writeControlError(writer, http.StatusNotFound, "runtime_not_found")
			return
		}
	}
	orchestrator.publishStatus()
	writeControlJSON(writer, http.StatusOK, map[string]any{
		"runtimeId": string(id), "backgroundLogin": BackgroundLoginSummary{Configured: false},
	})
}

func (orchestrator *Orchestrator) Status() CellStatus {
	if orchestrator == nil {
		return CellStatus{
			SchemaVersion: OrchestratorSchemaVersion,
			Version:       App.Version, BuildRevision: App.BuildRevision, BuildID: App.BuildID,
			Runtimes: []RuntimeStatus{},
		}
	}
	orchestrator.mu.RLock()
	revision := orchestrator.revision
	assignments := cloneAssignments(orchestrator.runtimes)
	configurationSyncs := make(map[AccountID]configurationSyncState, len(orchestrator.configurationSyncs))
	for id, state := range orchestrator.configurationSyncs {
		configurationSyncs[id] = state
	}
	orchestrator.mu.RUnlock()
	_, gameDataReady := orchestrator.supervisor.GameData().Current()
	now := orchestrator.now().UTC()
	runtimes := make([]RuntimeStatus, 0, len(assignments))
	for id, assignment := range assignments {
		application, applicationExists := orchestrator.supervisor.Application(id)
		status := RuntimeStatus{
			RuntimeID: string(id), TenantID: assignment.TenantID,
			PlacementEpoch: assignment.PlacementEpoch, LeaseExpiresAt: assignment.LeaseExpiresAt,
			PlacementLease: placementLeaseState(assignment.LeaseExpiresAt, now),
			Lifecycle:      "starting", DesiredSession: assignment.StartSession, GameDataReady: gameDataReady,
			OnDisconnect:                 assignment.OnDisconnect,
			DesiredConfigurationRevision: assignment.DesiredConfigurationRevision,
			DesiredConfigurationDigest:   assignment.DesiredConfigurationDigest,
		}
		applied := configurationSyncs[id]
		if applicationExists && application != nil && applied.application == application &&
			applied.placementEpoch == assignment.PlacementEpoch {
			status.AppliedConfigurationRevision = applied.revision
			status.AppliedConfigurationDigest = applied.digest
		}
		switch {
		case assignment.DesiredConfigurationRevision == 0:
			status.ConfigurationState = "unmanaged"
		case status.AppliedConfigurationRevision == assignment.DesiredConfigurationRevision &&
			status.AppliedConfigurationDigest == assignment.DesiredConfigurationDigest:
			status.ConfigurationState = "ready"
		default:
			status.ConfigurationState = "pending"
		}
		if status.OnDisconnect == "" {
			status.OnDisconnect = string(Session.ReconnectPolicyHold)
		}
		if applicationExists && application != nil {
			status.Lifecycle = "running"
			if application.State != nil {
				session := application.State.Session()
				status.SessionState = session.Status
				status.LoggedIn = session.LoggedIn
				status.SocketReady = session.SocketReady
				status.Generation = session.Generation
				status.BaselineGeneration = session.BaselineGeneration
				status.ConnectionGeneration = session.ConnectionGeneration
				status.CooldownUntil = session.CooldownUntil
				status.RetryAt = session.RetryAt
				status.LoginFailure = session.LoginFailure
			}
			if application.BackgroundLogin != nil {
				if login, err := application.BackgroundLogin.Status(); err == nil {
					status.BackgroundLogin = &BackgroundLoginSummary{
						Configured: login.Configured, Server: login.Server,
						Language: login.Language, UpdatedAt: login.UpdatedAt,
					}
				}
			}
			if application.PrivateMetrics != nil {
				metricsStatus := application.PrivateMetrics.Status()
				status.PrivateMetricsState = metricsStatus.State
				status.StatsMigrationState = metricsStatus.StatsMigrationState
				status.StatsMigrationSourceReports = metricsStatus.StatsMigrationSourceReports
				status.StatsMigrationSourceBuckets = metricsStatus.StatsMigrationSourceBuckets
				status.StatsMigrationPendingBuckets = metricsStatus.StatsMigrationPendingBuckets
				if !metricsStatus.LastPublishedAt.IsZero() {
					publishedAt := metricsStatus.LastPublishedAt.UTC()
					status.PrivateMetricsAt = &publishedAt
				}
				assignTime := func(value time.Time, destination **time.Time) {
					if !value.IsZero() {
						at := value.UTC()
						*destination = &at
					}
				}
				assignTime(metricsStatus.StatsMigrationOldestAt, &status.StatsMigrationOldestAt)
				assignTime(metricsStatus.StatsMigrationNewestAt, &status.StatsMigrationNewestAt)
				assignTime(metricsStatus.StatsMigrationPendingFrom, &status.StatsMigrationPendingFrom)
				assignTime(metricsStatus.StatsMigrationPendingThrough, &status.StatsMigrationPendingThrough)
			}
			if application.Checkpoints != nil {
				checkpointStatus := application.Checkpoints.Status()
				status.CheckpointState = checkpointStatus.State
				status.CheckpointRevision = checkpointStatus.LastRevision
				status.CheckpointConfigurationRevision = checkpointStatus.LastConfigurationRevision
				if !checkpointStatus.LastCheckpointAt.IsZero() {
					checkpointAt := checkpointStatus.LastCheckpointAt.UTC()
					status.CheckpointAt = &checkpointAt
				}
			}
		}
		runtimes = append(runtimes, status)
	}
	sort.Slice(runtimes, func(left, right int) bool { return runtimes[left].RuntimeID < runtimes[right].RuntimeID })
	return CellStatus{
		SchemaVersion: OrchestratorSchemaVersion,
		Version:       App.Version, BuildRevision: App.BuildRevision, BuildID: App.BuildID,
		CellID:          orchestrator.cellID,
		DesiredRevision: revision, GameDataReady: gameDataReady, Capacity: orchestrator.supervisor.Capacity(),
		Runtimes: runtimes, ObservedAt: now,
	}
}

// placementLeaseState reports whether a placement lease is still current. A
// lapsed lease is an observation for the control plane, not a lifecycle event:
// the private-metrics publisher stops spending the lapsed grant on its own,
// dashboard grants carry their own expiry, and the runtime keeps running until
// a reconcile omits it.
func placementLeaseState(leaseExpiresAt time.Time, now time.Time) string {
	if leaseExpiresAt.After(now) {
		return PlacementLeaseActive
	}
	return PlacementLeaseLapsed
}

func (orchestrator *Orchestrator) rollbackAdded(ids []AccountID) {
	for _, id := range ids {
		orchestrator.dashboardAuth.RevokeRuntime(id)
		orchestrator.drainRuntime(id)
	}
}

func (orchestrator *Orchestrator) drainRuntime(id AccountID) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), orchestrator.drainTimeout)
		defer cancel()
		// The runtime's last act is its final dashboard checkpoint under the
		// placement it still holds; then outbound publishing is revoked and
		// the runtime is removed, while siblings proceed untouched.
		if application, exists := orchestrator.supervisor.Application(id); exists && application != nil {
			checkpointContext, cancelCheckpoint := context.WithTimeout(ctx, drainCheckpointTimeout)
			_ = application.Checkpoint(checkpointContext, PrivateMetrics.CheckpointReasonDrain)
			cancelCheckpoint()
		}
		_ = orchestrator.supervisor.SetPrivateMetricsPlacement(id, nil)
		_ = orchestrator.supervisor.RemoveAccount(ctx, id)
	}()
}

func (orchestrator *Orchestrator) handleEvents(writer http.ResponseWriter, request *http.Request) {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		writeControlError(writer, http.StatusInternalServerError, "stream_unsupported")
		return
	}
	updates, release := orchestrator.subscribe()
	defer release()
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("X-Accel-Buffering", "no")
	if err := writeControlEvent(writer, "orchestrator.ready", orchestrator.Status()); err != nil {
		return
	}
	flusher.Flush()
	heartbeat := time.NewTicker(controlHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case status, open := <-updates:
			if !open || writeControlEvent(writer, "orchestrator.status", status) != nil {
				return
			}
			flusher.Flush()
		case timestamp := <-heartbeat.C:
			if _, err := fmt.Fprintf(writer, ": heartbeat %s\n\n", timestamp.UTC().Format(time.RFC3339)); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (orchestrator *Orchestrator) subscribe() (<-chan CellStatus, func()) {
	updates := make(chan CellStatus, 1)
	orchestrator.subscriberMu.Lock()
	orchestrator.subscribers[updates] = struct{}{}
	orchestrator.subscriberMu.Unlock()
	var once sync.Once
	return updates, func() {
		once.Do(func() {
			orchestrator.subscriberMu.Lock()
			delete(orchestrator.subscribers, updates)
			close(updates)
			orchestrator.subscriberMu.Unlock()
		})
	}
}

func (orchestrator *Orchestrator) publishStatus() {
	if orchestrator == nil {
		return
	}
	status := orchestrator.Status()
	orchestrator.subscriberMu.Lock()
	defer orchestrator.subscriberMu.Unlock()
	for subscriber := range orchestrator.subscribers {
		select {
		case subscriber <- status:
		default:
			select {
			case <-subscriber:
			default:
			}
			subscriber <- status
		}
	}
}

func (orchestrator *Orchestrator) privateMetricsPlacement(
	assignment RuntimeAssignment,
	desiredRevision uint64,
) *PrivateMetrics.Placement {
	if orchestrator == nil || !orchestrator.supervisor.PrivateMetricsEnabled() || assignment.PrivateMetrics == nil {
		return nil
	}
	return &PrivateMetrics.Placement{
		CellID: orchestrator.cellID, TenantID: assignment.TenantID, RuntimeID: assignment.RuntimeID,
		PlacementEpoch: assignment.PlacementEpoch, DesiredRevision: desiredRevision,
		LeaseExpiresAt: assignment.LeaseExpiresAt.UTC(), Grant: *assignment.PrivateMetrics,
	}
}

func (orchestrator *Orchestrator) reservedCredentialDigest(candidate [sha256.Size]byte) bool {
	if orchestrator == nil {
		return false
	}
	if subtle.ConstantTimeCompare(candidate[:], orchestrator.tokenHash[:]) == 1 {
		return true
	}
	orchestrator.mu.RLock()
	defer orchestrator.mu.RUnlock()
	for _, assignment := range orchestrator.runtimes {
		if assignment.PrivateMetrics == nil {
			continue
		}
		digest := sha256.Sum256([]byte(assignment.PrivateMetrics.Token))
		if subtle.ConstantTimeCompare(candidate[:], digest[:]) == 1 {
			return true
		}
	}
	return false
}

func assignmentsByID(assignments []RuntimeAssignment) map[AccountID]RuntimeAssignment {
	indexed := make(map[AccountID]RuntimeAssignment, len(assignments))
	for _, assignment := range assignments {
		indexed[AccountID(assignment.RuntimeID)] = cloneRuntimeAssignment(assignment)
	}
	return indexed
}

func cloneAssignments(assignments map[AccountID]RuntimeAssignment) map[AccountID]RuntimeAssignment {
	cloned := make(map[AccountID]RuntimeAssignment, len(assignments))
	for id, assignment := range assignments {
		cloned[id] = cloneRuntimeAssignment(assignment)
	}
	return cloned
}

func cloneRuntimeAssignment(assignment RuntimeAssignment) RuntimeAssignment {
	if assignment.PrivateMetrics != nil {
		grant := *assignment.PrivateMetrics
		assignment.PrivateMetrics = &grant
	}
	return assignment
}

func sameAssignments(left, right map[AccountID]RuntimeAssignment) bool {
	if len(left) != len(right) {
		return false
	}
	for id, leftValue := range left {
		rightValue, exists := right[id]
		if !exists || leftValue.RuntimeID != rightValue.RuntimeID || leftValue.TenantID != rightValue.TenantID ||
			leftValue.PlacementEpoch != rightValue.PlacementEpoch || leftValue.StartSession != rightValue.StartSession ||
			leftValue.DesiredConfigurationRevision != rightValue.DesiredConfigurationRevision ||
			leftValue.DesiredConfigurationDigest != rightValue.DesiredConfigurationDigest ||
			leftValue.OnDisconnect != rightValue.OnDisconnect ||
			!leftValue.LeaseExpiresAt.Equal(rightValue.LeaseExpiresAt) ||
			!samePrivateMetricsGrant(leftValue.PrivateMetrics, rightValue.PrivateMetrics) {
			return false
		}
	}
	return true
}

func (orchestrator *Orchestrator) configurationReady(id AccountID, assignment RuntimeAssignment) bool {
	if assignment.DesiredConfigurationRevision == 0 {
		return true // Backward compatibility for an older control plane.
	}
	application, exists := orchestrator.supervisor.Application(id)
	if !exists || application == nil {
		return false
	}
	orchestrator.mu.RLock()
	applied := orchestrator.configurationSyncs[id]
	orchestrator.mu.RUnlock()
	return applied.placementEpoch == assignment.PlacementEpoch &&
		applied.revision == assignment.DesiredConfigurationRevision &&
		applied.digest == assignment.DesiredConfigurationDigest &&
		applied.application == application
}

func (orchestrator *Orchestrator) startSessionIfDesired(id AccountID, placementEpoch uint64) {
	if orchestrator == nil {
		return
	}
	go func() {
		orchestrator.reconcileMu.Lock()
		defer orchestrator.reconcileMu.Unlock()
		orchestrator.mu.RLock()
		assignment, exists := orchestrator.runtimes[id]
		orchestrator.mu.RUnlock()
		if !exists || assignment.PlacementEpoch != placementEpoch || !assignment.StartSession ||
			!orchestrator.configurationReady(id, assignment) {
			return
		}
		application, running := orchestrator.supervisor.Application(id)
		if !running || application == nil || application.Session == nil {
			return
		}
		_ = application.Session.Start(context.Background())
	}()
}

func validConfigurationDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// canonicalConfigurationDigest uses the shared backend/cell wire contract:
// SHA-256 over JSON {schemaVersion,revision,sections}, with sorted map keys and
// compact section JSON. updatedAt is observation metadata and is excluded.
func canonicalConfigurationDigest(snapshot Configuration.Snapshot) (map[string]json.RawMessage, string, error) {
	sections := make(map[string]json.RawMessage, len(snapshot.Sections))
	for section, value := range snapshot.Sections {
		if err := Configuration.Validate(section, value); err != nil {
			return nil, "", err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, value); err != nil {
			return nil, "", err
		}
		sections[section] = append(json.RawMessage(nil), compact.Bytes()...)
	}
	document := struct {
		SchemaVersion int                        `json:"schemaVersion"`
		Revision      uint64                     `json:"revision"`
		Sections      map[string]json.RawMessage `json:"sections"`
	}{snapshot.SchemaVersion, snapshot.Revision, sections}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return sections, hex.EncodeToString(digest[:]), nil
}

func samePrivateMetricsGrant(left, right *PrivateMetrics.Grant) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftDigest := sha256.Sum256([]byte(left.Token))
	rightDigest := sha256.Sum256([]byte(right.Token))
	return subtle.ConstantTimeCompare(leftDigest[:], rightDigest[:]) == 1 && left.ExpiresAt.Equal(right.ExpiresAt)
}

func decodeControlJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	return decodeControlJSONLimit(writer, request, target, maximumControlRequestBytes)
}

func decodeControlJSONLimit(writer http.ResponseWriter, request *http.Request, target any, maximum int64) error {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Type"))), "application/json") {
		writeControlError(writer, http.StatusUnsupportedMediaType, "content_type_not_supported")
		return errors.New("content type is not application/json")
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maximum)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeControlError(writer, http.StatusBadRequest, "invalid_request")
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		writeControlError(writer, http.StatusBadRequest, "invalid_request")
		return err
	}
	return nil
}

func writeControlEvent(writer io.Writer, event string, status CellStatus) error {
	payload, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "id: %d\nevent: %s\nretry: 5000\ndata: %s\n\n", status.DesiredRevision, event, payload)
	return err
}

func writeOrchestratorError(writer http.ResponseWriter, err error) {
	var controlErr *orchestratorError
	if errors.As(err, &controlErr) {
		writeControlError(writer, controlErr.status, controlErr.code)
		return
	}
	writeControlError(writer, http.StatusInternalServerError, "orchestrator_failed")
}

func writeControlError(writer http.ResponseWriter, status int, code string) {
	writeControlJSON(writer, status, map[string]any{"error": map[string]string{"code": code}})
}

func writeControlJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func setControlSecurityHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
}
