package PrivateMetrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	CheckpointSchemaVersion = 1

	defaultCheckpointInterval = 5 * time.Minute
	defaultCheckpointDebounce = 2 * time.Second
	defaultCheckpointTimeout  = 90 * time.Second
	checkpointOperationLimit  = 100
)

// CheckpointReason records why a checkpoint was taken.
type CheckpointReason string

const (
	// CheckpointReasonCadence is the periodic checkpoint taken while the state
	// or configuration revision keeps changing.
	CheckpointReasonCadence CheckpointReason = "cadence"
	// CheckpointReasonSession follows a game-session transition (connected,
	// released, cooldown, error, stopped) so the stale dashboard shows the
	// last true situation.
	CheckpointReasonSession CheckpointReason = "session"
	// CheckpointReasonConfiguration follows a durable settings update so an
	// offline tenant dashboard never falls behind configuration-only changes.
	CheckpointReasonConfiguration CheckpointReason = "configuration"
	// CheckpointReasonDrain is the final checkpoint taken before a runtime is
	// removed from the cell.
	CheckpointReasonDrain CheckpointReason = "drain"
)

// CheckpointSession is the sanitized session situation at checkpoint time.
type CheckpointSession struct {
	State                string              `json:"state"`
	LoggedIn             bool                `json:"loggedIn"`
	SocketReady          bool                `json:"socketReady"`
	Generation           uint64              `json:"generation"`
	BaselineGeneration   uint64              `json:"baselineGeneration"`
	ConnectionGeneration uint64              `json:"connectionGeneration"`
	CooldownUntil        *time.Time          `json:"cooldownUntil,omitempty"`
	RetryAt              *time.Time          `json:"retryAt,omitempty"`
	LoginFailure         *State.LoginFailure `json:"loginFailure,omitempty"`
	ChangedAt            time.Time           `json:"changedAt"`
}

// Checkpoint is the complete dashboard read model of one runtime at one
// instant: the same client state document the dashboard renders live, the
// configuration snapshot it edits, and the recent operation receipts. The
// backend keeps the latest checkpoint per hosted account so the frontend can
// render the dashboard — stale but complete — while no runtime exists.
type Checkpoint struct {
	CheckpointID          string            `json:"checkpointId"`
	ObservedAt            time.Time         `json:"observedAt"`
	Reason                CheckpointReason  `json:"reason"`
	StateRevision         uint64            `json:"stateRevision"`
	ConfigurationRevision uint64            `json:"configurationRevision,omitempty"`
	Session               CheckpointSession `json:"session"`
	// Account is present once the runtime has bound an authoritative game
	// identity; the backend attributes the checkpoint by placement regardless.
	Account *AccountBinding `json:"account,omitempty"`
	// State is the client state snapshot exactly as `state.snapshot` sends it
	// to the dashboard (official domains plus the dashboard map/Storm edge
	// projections); large backend-only collections are already excluded.
	State json.RawMessage `json:"state"`
	// Configuration is the dashboard-safe configuration snapshot (`config.changed`).
	Configuration json.RawMessage `json:"configuration,omitempty"`
	// Operations are the most recent intent receipts (`operations.snapshot`).
	Operations json.RawMessage `json:"operations,omitempty"`
}

// CheckpointRequest carries no credential material; the grant travels only in
// the Authorization header.
type CheckpointRequest struct {
	SchemaVersion   int        `json:"schemaVersion"`
	CellID          string     `json:"cellId"`
	TenantID        string     `json:"tenantId"`
	RuntimeID       string     `json:"runtimeId"`
	PlacementEpoch  uint64     `json:"placementEpoch"`
	DesiredRevision uint64     `json:"desiredRevision"`
	LeaseExpiresAt  time.Time  `json:"leaseExpiresAt"`
	Checkpoint      Checkpoint `json:"checkpoint"`
}

// BuildCheckpoint projects the runtime for the stale dashboard. Unlike a
// metrics sample it has no readiness gate: a released, cooling-down, or
// errored runtime is exactly what the stale dashboard must show.
func BuildCheckpoint(
	ctx context.Context,
	store *State.Store,
	configuration *Configuration.Store,
	intents *Intent.Engine,
	reason CheckpointReason,
	observedAt time.Time,
) (Checkpoint, error) {
	if store == nil {
		return Checkpoint{}, fmt.Errorf("state store is unavailable")
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	view := store.ReadOnlyView()
	stateDocument, err := json.Marshal(State.NewClientStateSnapshot(view))
	if err != nil {
		return Checkpoint{}, fmt.Errorf("encode dashboard state: %w", err)
	}
	checkpoint := Checkpoint{
		ObservedAt: observedAt, Reason: reason, StateRevision: view.Revision,
		Session: CheckpointSession{
			State: view.Session.Status, LoggedIn: view.Session.LoggedIn, SocketReady: view.Session.SocketReady,
			Generation: view.Session.Generation, BaselineGeneration: view.Session.BaselineGeneration,
			ConnectionGeneration: view.Session.ConnectionGeneration,
			CooldownUntil:        view.Session.CooldownUntil, RetryAt: view.Session.RetryAt,
			LoginFailure: view.Session.LoginFailure, ChangedAt: view.Session.ChangedAt.UTC(),
		},
		State: stateDocument,
	}
	if worldID := State.CanonicalWorldID(view.Account.WorldID); view.Account.UID > 0 && view.Player.ID > 0 && worldID != "" {
		checkpoint.Account = &AccountBinding{
			AccountUID: view.Account.UID, WorldID: worldID,
			PlayerID: int64(view.Player.ID), BoundAt: view.Account.BoundAt.UTC(),
		}
	}
	if configuration != nil {
		snapshot := configuration.Snapshot()
		checkpoint.ConfigurationRevision = snapshot.Revision
		if encoded, err := json.Marshal(snapshot); err == nil {
			checkpoint.Configuration = encoded
		}
	}
	if intents != nil {
		if receipts, err := intents.RecentOperations(ctx, checkpointOperationLimit); err == nil {
			if encoded, err := json.Marshal(receipts); err == nil {
				checkpoint.Operations = encoded
			}
		}
	}
	return checkpoint, nil
}

func checkpointID(placement Placement, checkpoint Checkpoint) string {
	digest := sha256.New()
	_, _ = fmt.Fprintf(
		digest, "checkpoint\x00%d\x00%s\x00%s\x00%d\x00%d\x00%d\x00%s\x00%d",
		CheckpointSchemaVersion, placement.TenantID, placement.RuntimeID, placement.PlacementEpoch,
		checkpoint.StateRevision, checkpoint.ConfigurationRevision, checkpoint.Reason, checkpoint.ObservedAt.UnixNano(),
	)
	return hex.EncodeToString(digest.Sum(nil))
}

type CheckpointPublisherConfig struct {
	RuntimeID     string
	State         *State.Store
	Configuration *Configuration.Store
	Intents       *Intent.Engine
	Client        *Client
	Placement     *Placement
	// Interval is the cadence at which a changed state or configuration
	// revision is checkpointed; it is also the base retry delay.
	Interval time.Duration
	// Debounce delays the checkpoint that follows a session transition so a
	// burst of status changes yields one checkpoint.
	Debounce time.Duration
	// Timeout bounds one build plus upload.
	Timeout time.Duration
	Now     func() time.Time
	Jitter  func() float64
}

type CheckpointStatus struct {
	Enabled                   bool             `json:"enabled"`
	State                     string           `json:"state"`
	LastAttemptAt             time.Time        `json:"lastAttemptAt,omitempty"`
	LastCheckpointAt          time.Time        `json:"lastCheckpointAt,omitempty"`
	LastRevision              uint64           `json:"lastRevision,omitempty"`
	LastConfigurationRevision uint64           `json:"lastConfigurationRevision,omitempty"`
	LastReason                CheckpointReason `json:"lastReason,omitempty"`
	NextAttemptAt             time.Time        `json:"nextAttemptAt,omitempty"`
	ConsecutiveFailures       int              `json:"consecutiveFailures,omitempty"`
	LastError                 string           `json:"lastError,omitempty"`
}

// CheckpointPublisher keeps the backend's copy of the dashboard read model
// current: on a cadence while state or configuration changes, promptly after
// a session transition or durable settings update, and once more when the
// runtime is drained. It shares the runtime's private-metrics placement and
// follows the same retry discipline.
type CheckpointPublisher struct {
	runtimeID     string
	state         *State.Store
	configuration *Configuration.Store
	intents       *Intent.Engine
	client        *Client
	interval      time.Duration
	debounce      time.Duration
	timeout       time.Duration
	now           func() time.Time
	jitter        func() float64

	placementMu      sync.RWMutex
	placement        *Placement
	placementVersion uint64
	wake             chan struct{}
	started          atomic.Bool

	uploadMu sync.Mutex

	statusMu sync.RWMutex
	status   CheckpointStatus
}

func NewCheckpointPublisher(config CheckpointPublisherConfig) (*CheckpointPublisher, error) {
	runtimeID := strings.TrimSpace(config.RuntimeID)
	if runtimeID == "" || config.State == nil || config.Client == nil || !config.Client.CheckpointsEnabled() {
		return nil, fmt.Errorf("dashboard checkpoint publisher needs a runtime id, state store, and checkpoint endpoint")
	}
	interval := config.Interval
	if interval <= 0 {
		interval = defaultCheckpointInterval
	}
	debounce := config.Debounce
	if debounce <= 0 {
		debounce = defaultCheckpointDebounce
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultCheckpointTimeout
	}
	now := config.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	jitter := config.Jitter
	if jitter == nil {
		jitter = rand.Float64
	}
	publisher := &CheckpointPublisher{
		runtimeID: runtimeID, state: config.State, configuration: config.Configuration, intents: config.Intents,
		client: config.Client, interval: interval, debounce: debounce, timeout: timeout,
		now: now, jitter: jitter, wake: make(chan struct{}, 1),
		status: CheckpointStatus{Enabled: true, State: StateWaitingForPlacement},
	}
	if config.Placement != nil {
		if err := publisher.SetPlacement(config.Placement); err != nil {
			return nil, err
		}
	}
	return publisher, nil
}

// SetPlacement rotates the placement under which checkpoints are published.
func (publisher *CheckpointPublisher) SetPlacement(placement *Placement) error {
	if publisher == nil {
		return fmt.Errorf("dashboard checkpoint publisher is unavailable")
	}
	if placement == nil {
		publisher.placementMu.Lock()
		publisher.placement = nil
		publisher.placementVersion++
		publisher.placementMu.Unlock()
		publisher.updateStatus(func(status *CheckpointStatus) {
			status.State = StateWaitingForPlacement
			status.NextAttemptAt = time.Time{}
			status.ConsecutiveFailures = 0
			status.LastError = ""
		})
		publisher.signal()
		return nil
	}
	normalized := *placement
	normalized.CellID = strings.TrimSpace(normalized.CellID)
	normalized.TenantID = strings.TrimSpace(normalized.TenantID)
	normalized.RuntimeID = strings.TrimSpace(normalized.RuntimeID)
	normalized.LeaseExpiresAt = normalized.LeaseExpiresAt.UTC()
	normalized.Grant.ExpiresAt = normalized.Grant.ExpiresAt.UTC()
	if err := validatePlacement(normalized, publisher.runtimeID, publisher.now().UTC()); err != nil {
		return err
	}
	publisher.placementMu.Lock()
	publisher.placement = &normalized
	publisher.placementVersion++
	publisher.placementMu.Unlock()
	publisher.updateStatus(func(status *CheckpointStatus) {
		status.State = StateWaitingForRuntime
		status.ConsecutiveFailures = 0
		status.LastError = ""
	})
	publisher.signal()
	return nil
}

func (publisher *CheckpointPublisher) Status() CheckpointStatus {
	if publisher == nil {
		return CheckpointStatus{State: "disabled"}
	}
	publisher.statusMu.RLock()
	defer publisher.statusMu.RUnlock()
	return publisher.status
}

// Checkpoint builds and uploads one checkpoint now under the current
// placement. It is used for the drain checkpoint and by tests; ctx bounds the
// attempt. It returns nil when no placement is installed, because a runtime
// without a grant has nothing to publish to.
func (publisher *CheckpointPublisher) Checkpoint(ctx context.Context, reason CheckpointReason) error {
	if publisher == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	placement, _, available := publisher.currentPlacement()
	now := publisher.now().UTC()
	if !available || !placement.LeaseExpiresAt.After(now) || !placement.Grant.ExpiresAt.After(now) {
		return nil
	}
	return publisher.publish(ctx, placement, reason, now)
}

func (publisher *CheckpointPublisher) publish(ctx context.Context, placement Placement, reason CheckpointReason, now time.Time) error {
	publisher.uploadMu.Lock()
	defer publisher.uploadMu.Unlock()
	attemptContext, cancel := context.WithTimeout(ctx, publisher.timeout)
	defer cancel()
	checkpoint, err := BuildCheckpoint(attemptContext, publisher.state, publisher.configuration, publisher.intents, reason, now)
	if err != nil {
		publisher.updateStatus(func(status *CheckpointStatus) {
			status.State = StateError
			status.LastAttemptAt = now
			status.LastError = err.Error()
		})
		return err
	}
	checkpoint.CheckpointID = checkpointID(placement, checkpoint)
	publisher.updateStatus(func(status *CheckpointStatus) {
		status.State = StatePublishing
		status.LastAttemptAt = now
	})
	if err := publisher.client.UploadCheckpoint(attemptContext, placement, checkpoint); err != nil {
		return err
	}
	publisher.updateStatus(func(status *CheckpointStatus) {
		status.State = StatePublished
		status.LastCheckpointAt = now
		status.LastRevision = checkpoint.StateRevision
		status.LastConfigurationRevision = checkpoint.ConfigurationRevision
		status.LastReason = reason
		status.ConsecutiveFailures = 0
		status.LastError = ""
	})
	return nil
}

// Run drives cadence and session-transition checkpoints until ctx ends.
func (publisher *CheckpointPublisher) Run(ctx context.Context) {
	if publisher == nil || !publisher.started.CompareAndSwap(false, true) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	events, unsubscribe := publisher.state.Subscribe(32)
	defer unsubscribe()
	var configurationEvents <-chan Configuration.Event
	if publisher.configuration != nil {
		var unsubscribeConfiguration func()
		configurationEvents, unsubscribeConfiguration = publisher.configuration.Subscribe(16)
		defer unsubscribeConfiguration()
	}
	timer := newAttemptTimer()
	defer timer.stop()

	failures := 0
	var lastSessionKey string
	pendingReason := CheckpointReasonCadence
	sessionKey := func() string {
		session := publisher.state.Session()
		return fmt.Sprintf("%s|%t|%t|%d", session.Status, session.LoggedIn, session.SocketReady, session.ConnectionGeneration)
	}
	lastSessionKey = sessionKey()

	schedule := func(at time.Time) {
		if timer.schedule(publisher.now().UTC(), at) {
			publisher.updateStatus(func(status *CheckpointStatus) { status.NextAttemptAt = timer.at })
		}
	}
	attempt := func() {
		timer.consume()
		placement, _, available := publisher.currentPlacement()
		now := publisher.now().UTC()
		if !available || !placement.LeaseExpiresAt.After(now) || !placement.Grant.ExpiresAt.After(now) {
			publisher.updateStatus(func(status *CheckpointStatus) {
				status.State = StateWaitingForPlacement
				status.LastAttemptAt = now
				status.NextAttemptAt = time.Time{}
			})
			return
		}
		reason := pendingReason
		pendingReason = CheckpointReasonCadence
		status := publisher.Status()
		if reason == CheckpointReasonCadence &&
			publisher.state.Revision() == status.LastRevision &&
			publisher.configurationRevision() == status.LastConfigurationRevision && failures == 0 {
			// Nothing changed since the last checkpoint; keep the cadence.
			schedule(now.Add(publisher.interval))
			return
		}
		err := publisher.publish(ctx, placement, reason, now)
		if err == nil {
			failures = 0
			schedule(now.Add(publisher.interval))
			return
		}
		if ctx.Err() != nil {
			return
		}
		failures++
		delay := backoffDelay(publisher.interval, failures, publisher.jitter)
		state := StateRetrying
		switch OutcomeOf(err) {
		case OutcomeUnauthorized:
			state = StateGrantRejected
			delay = backoffDelay(publisher.interval, maximumBackoffFactor, publisher.jitter)
		case OutcomeRejected:
			state = StateRejected
		default:
			var publishErr *PublishError
			if errors.As(err, &publishErr) && publishErr != nil && publishErr.RetryAfter > delay {
				delay = publishErr.RetryAfter
			}
		}
		message := err.Error()
		publisher.updateStatus(func(status *CheckpointStatus) {
			status.State = state
			status.ConsecutiveFailures = failures
			status.LastError = message
		})
		if reason != CheckpointReasonCadence {
			pendingReason = reason
		}
		schedule(now.Add(delay))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case _, open := <-events:
			if !open {
				events = nil
				continue
			}
			if key := sessionKey(); key != lastSessionKey {
				lastSessionKey = key
				if failures == 0 {
					pendingReason = CheckpointReasonSession
					schedule(publisher.now().UTC().Add(publisher.debounce))
				}
			}
		case _, open := <-configurationEvents:
			if !open {
				configurationEvents = nil
				continue
			}
			if pendingReason != CheckpointReasonSession {
				pendingReason = CheckpointReasonConfiguration
			}
			if failures == 0 {
				schedule(publisher.now().UTC().Add(publisher.debounce))
			}
		case <-publisher.wake:
			failures = 0
			if _, _, available := publisher.currentPlacement(); available {
				schedule(publisher.now().UTC().Add(publisher.debounce))
			}
		case <-timer.channel():
			attempt()
		}
	}
}

func (publisher *CheckpointPublisher) configurationRevision() uint64 {
	if publisher == nil || publisher.configuration == nil {
		return 0
	}
	return publisher.configuration.Revision()
}

func (publisher *CheckpointPublisher) currentPlacement() (Placement, uint64, bool) {
	publisher.placementMu.RLock()
	defer publisher.placementMu.RUnlock()
	if publisher.placement == nil {
		return Placement{}, publisher.placementVersion, false
	}
	return *publisher.placement, publisher.placementVersion, true
}

func (publisher *CheckpointPublisher) signal() {
	select {
	case publisher.wake <- struct{}{}:
	default:
	}
}

func (publisher *CheckpointPublisher) updateStatus(update func(*CheckpointStatus)) {
	publisher.statusMu.Lock()
	defer publisher.statusMu.Unlock()
	publisher.status.Enabled = true
	update(&publisher.status)
}
