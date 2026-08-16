package PrivateMetrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	defaultPublishInterval = time.Minute
	defaultStateDebounce   = 250 * time.Millisecond
	defaultAttemptTimeout  = 45 * time.Second
	// maximumBackoffFactor caps retry spacing at this multiple of the publish
	// interval. A refused grant also waits this long before it is spent again.
	maximumBackoffFactor = 8
	minimumGrantLength   = 32
)

const (
	StateWaitingForPlacement = "waiting-for-placement"
	StateWaitingForRuntime   = "waiting-for-runtime"
	StatePublishing          = "publishing"
	StatePublished           = "published"
	StateRetrying            = "retrying"
	StateRejected            = "rejected"
	StateGrantRejected       = "grant-rejected"
	StateError               = "error"
)

type PublisherConfig struct {
	RuntimeID string
	State     *State.Store
	GameData  *GameData.Manager
	Reports   ReportReader
	Client    *Client
	Placement *Placement
	// Interval is the steady-state publish cadence and the base retry delay.
	Interval time.Duration
	// Debounce delays the first publication after a burst of state changes.
	Debounce time.Duration
	// Timeout bounds one sample build plus upload attempt.
	Timeout time.Duration
	Now     func() time.Time
	// Jitter optionally overrides the retry jitter source. It must return a
	// value in [0, 1). Tests use it to make retry timing deterministic.
	Jitter func() float64
}

type PublisherStatus struct {
	Enabled             bool      `json:"enabled"`
	State               string    `json:"state"`
	LastAttemptAt       time.Time `json:"lastAttemptAt,omitempty"`
	LastPublishedAt     time.Time `json:"lastPublishedAt,omitempty"`
	NextAttemptAt       time.Time `json:"nextAttemptAt,omitempty"`
	ConsecutiveFailures int       `json:"consecutiveFailures,omitempty"`
	LastError           string    `json:"lastError,omitempty"`
}

type Publisher struct {
	runtimeID string
	state     *State.Store
	builder   *SampleBuilder
	client    *Client
	interval  time.Duration
	debounce  time.Duration
	timeout   time.Duration
	now       func() time.Time
	jitter    func() float64

	placementMu      sync.RWMutex
	placement        *Placement
	placementVersion uint64
	wake             chan struct{}
	started          atomic.Bool

	statusMu sync.RWMutex
	status   PublisherStatus
}

type pendingSample struct {
	placementVersion uint64
	placement        Placement
	sample           Sample
}

func NewPublisher(config PublisherConfig) (*Publisher, error) {
	runtimeID := strings.TrimSpace(config.RuntimeID)
	if runtimeID == "" || config.State == nil || config.Client == nil || !config.Client.Enabled() {
		return nil, fmt.Errorf("private metrics publisher needs a runtime id, state store, and enabled client")
	}
	interval := config.Interval
	if interval <= 0 {
		interval = defaultPublishInterval
	}
	debounce := config.Debounce
	if debounce <= 0 {
		debounce = defaultStateDebounce
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultAttemptTimeout
	}
	now := config.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	jitter := config.Jitter
	if jitter == nil {
		jitter = rand.Float64
	}
	publisher := &Publisher{
		runtimeID: runtimeID, state: config.State,
		builder: NewSampleBuilder(config.State, config.GameData, config.Reports),
		client:  config.Client, interval: interval, debounce: debounce, timeout: timeout,
		now: now, jitter: jitter, wake: make(chan struct{}, 1),
		status: PublisherStatus{Enabled: true, State: StateWaitingForPlacement},
	}
	if config.Placement != nil {
		if err := publisher.SetPlacement(config.Placement); err != nil {
			return nil, err
		}
	}
	return publisher, nil
}

// SetPlacement atomically rotates the runtime's placement and credential. Any
// pending sample from the previous placement is discarded rather than being
// relabeled with a newer fencing epoch. Rotation never bursts an extra
// publication: the next sample follows the regular cadence under the new grant.
func (publisher *Publisher) SetPlacement(placement *Placement) error {
	if publisher == nil {
		return fmt.Errorf("private metrics publisher is unavailable")
	}
	if placement == nil {
		publisher.placementMu.Lock()
		publisher.placement = nil
		publisher.placementVersion++
		publisher.placementMu.Unlock()
		publisher.updateStatus(func(status *PublisherStatus) {
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
	publisher.updateStatus(func(status *PublisherStatus) {
		status.State = StateWaitingForRuntime
		status.ConsecutiveFailures = 0
		status.LastError = ""
	})
	publisher.signal()
	return nil
}

func (publisher *Publisher) Status() PublisherStatus {
	if publisher == nil {
		return PublisherStatus{State: "disabled"}
	}
	publisher.statusMu.RLock()
	defer publisher.statusMu.RUnlock()
	return publisher.status
}

// Run drives publication until ctx ends. Exactly one goroutine owns the
// pending sample, the retry schedule, and the outbound credential copy.
func (publisher *Publisher) Run(ctx context.Context) {
	if publisher == nil || !publisher.started.CompareAndSwap(false, true) {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	events, unsubscribe := publisher.state.Subscribe(32)
	defer unsubscribe()
	timer := newAttemptTimer()
	defer timer.stop()

	var pending *pendingSample
	var lastPublished time.Time
	failures := 0

	schedule := func(at time.Time) {
		if timer.schedule(publisher.now().UTC(), at) {
			publisher.updateStatus(func(status *PublisherStatus) { status.NextAttemptAt = timer.at })
		}
	}
	nextCadence := func(now time.Time) time.Time {
		if lastPublished.IsZero() {
			return now
		}
		if next := lastPublished.Add(publisher.interval); next.After(now) {
			return next
		}
		return now
	}

	attempt := func() {
		timer.consume()
		placement, version, available := publisher.currentPlacement()
		now := publisher.now().UTC()
		if !available || !placement.LeaseExpiresAt.After(now) || !placement.Grant.ExpiresAt.After(now) {
			pending = nil
			publisher.updateStatus(func(status *PublisherStatus) {
				status.State = StateWaitingForPlacement
				status.LastAttemptAt = now
				status.NextAttemptAt = time.Time{}
			})
			return
		}
		attemptContext, cancel := context.WithTimeout(ctx, publisher.timeout)
		defer cancel()
		if pending == nil || pending.placementVersion != version {
			sample, err := publisher.builder.Build(attemptContext, now)
			if err != nil {
				if errors.Is(err, ErrRuntimeNotReady) {
					publisher.updateStatus(func(status *PublisherStatus) {
						status.State = StateWaitingForRuntime
						status.LastAttemptAt = now
						status.LastError = ""
					})
					// State changes wake the publisher sooner; this only keeps a
					// bounded heartbeat when the runtime stays quiet.
					schedule(now.Add(publisher.interval))
					return
				}
				failures++
				publisher.updateStatus(func(status *PublisherStatus) {
					status.State = StateError
					status.LastAttemptAt = now
					status.ConsecutiveFailures = failures
					status.LastError = err.Error()
				})
				schedule(now.Add(publisher.backoff(failures)))
				return
			}
			sample.SampleID = sampleID(placement, sample)
			pending = &pendingSample{placementVersion: version, placement: placement, sample: sample}
		}
		publisher.updateStatus(func(status *PublisherStatus) {
			status.State = StatePublishing
			status.LastAttemptAt = now
		})
		err := publisher.client.Upload(attemptContext, pending.placement, pending.sample)
		if err == nil {
			lastPublished = now
			pending = nil
			failures = 0
			publisher.updateStatus(func(status *PublisherStatus) {
				status.State = StatePublished
				status.LastPublishedAt = now
				status.ConsecutiveFailures = 0
				status.LastError = ""
			})
			schedule(now.Add(publisher.interval))
			return
		}
		if ctx.Err() != nil {
			return
		}
		failures++
		delay := publisher.backoff(failures)
		state := StateRetrying
		switch OutcomeOf(err) {
		case OutcomeUnauthorized:
			// The grant was refused. Do not keep spending it: wait for the
			// longest backoff so a stale credential is not hammered, and let a
			// placement rotation reset the schedule immediately.
			pending = nil
			state = StateGrantRejected
			delay = publisher.backoff(maximumBackoffFactor)
		case OutcomeRejected:
			// The backend durably refused this exact sample. Replaying it can
			// never succeed, so drop it and let the next attempt build a fresh
			// sample with a fresh idempotency key.
			pending = nil
			state = StateRejected
		default:
			var publishErr *PublishError
			if errors.As(err, &publishErr) && publishErr != nil && publishErr.RetryAfter > delay {
				delay = publishErr.RetryAfter
			}
		}
		message := err.Error()
		publisher.updateStatus(func(status *PublisherStatus) {
			status.State = state
			status.ConsecutiveFailures = failures
			status.LastError = message
		})
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
			// A state change only pulls the first publication forward (or the
			// first one after the runtime recovers). Retry backoff and the
			// steady cadence are never bypassed by a busy state stream.
			now := publisher.now().UTC()
			if pending == nil && failures == 0 && !nextCadence(now).After(now) {
				schedule(now.Add(publisher.debounce))
			}
		case <-publisher.wake:
			pending = nil
			failures = 0
			if _, _, available := publisher.currentPlacement(); available {
				schedule(nextCadence(publisher.now().UTC()))
			}
		case <-timer.channel():
			attempt()
		}
	}
}

// backoff returns the retry delay after the given number of consecutive
// failures: the publish interval doubled per failure, capped, and jittered by
// up to 20% so a cell full of runtimes does not retry in lockstep.
func (publisher *Publisher) backoff(failures int) time.Duration {
	return backoffDelay(publisher.interval, failures, publisher.jitter)
}

func backoffDelay(interval time.Duration, failures int, jitterSource func() float64) time.Duration {
	factor := 1
	for index := 1; index < failures && factor < maximumBackoffFactor; index++ {
		factor *= 2
	}
	if factor > maximumBackoffFactor {
		factor = maximumBackoffFactor
	}
	base := interval * time.Duration(factor)
	jitter := 0.0
	if jitterSource != nil {
		jitter = jitterSource()
	}
	if jitter < 0 || jitter >= 1 {
		jitter = 0
	}
	return base + time.Duration(float64(base)*0.2*jitter)
}

func (publisher *Publisher) currentPlacement() (Placement, uint64, bool) {
	publisher.placementMu.RLock()
	defer publisher.placementMu.RUnlock()
	if publisher.placement == nil {
		return Placement{}, publisher.placementVersion, false
	}
	return *publisher.placement, publisher.placementVersion, true
}

func (publisher *Publisher) signal() {
	select {
	case publisher.wake <- struct{}{}:
	default:
	}
}

func (publisher *Publisher) updateStatus(update func(*PublisherStatus)) {
	publisher.statusMu.Lock()
	defer publisher.statusMu.Unlock()
	publisher.status.Enabled = true
	update(&publisher.status)
}

func validatePlacement(placement Placement, runtimeID string, now time.Time) error {
	if placement.CellID == "" || placement.TenantID == "" || placement.RuntimeID != runtimeID ||
		placement.PlacementEpoch == 0 || placement.DesiredRevision == 0 {
		return fmt.Errorf("private metrics placement identity is incomplete")
	}
	if !placement.LeaseExpiresAt.After(now) || !placement.Grant.ExpiresAt.After(now) ||
		placement.Grant.ExpiresAt.After(placement.LeaseExpiresAt) {
		return fmt.Errorf("private metrics placement or grant is expired")
	}
	if len(placement.Grant.Token) < minimumGrantLength || strings.TrimSpace(placement.Grant.Token) != placement.Grant.Token {
		return fmt.Errorf("private metrics grant is invalid")
	}
	return nil
}

func sampleID(placement Placement, sample Sample) string {
	digest := sha256.New()
	_, _ = fmt.Fprintf(
		digest, "%d\x00%s\x00%s\x00%d\x00%d\x00%s\x00%d\x00%d\x00%d",
		SchemaVersion, placement.TenantID, placement.RuntimeID, placement.PlacementEpoch,
		sample.Account.AccountUID, sample.Account.WorldID, sample.Account.PlayerID,
		sample.StateRevision, sample.ObservedAt.UnixNano(),
	)
	return hex.EncodeToString(digest.Sum(nil))
}

// attemptTimer owns the single outstanding publish deadline. Scheduling an
// earlier deadline replaces a later one; a later deadline never postpones an
// earlier one that is already armed.
type attemptTimer struct {
	timer *time.Timer
	at    time.Time
	armed bool
}

func newAttemptTimer() *attemptTimer {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	return &attemptTimer{timer: timer}
}

func (timer *attemptTimer) channel() <-chan time.Time {
	return timer.timer.C
}

func (timer *attemptTimer) schedule(now time.Time, at time.Time) bool {
	if timer.armed && !at.Before(timer.at) {
		return false
	}
	if timer.armed && !timer.timer.Stop() {
		select {
		case <-timer.timer.C:
		default:
		}
	}
	delay := at.Sub(now)
	if delay < 0 {
		delay = 0
	}
	timer.timer.Reset(delay)
	timer.at = at
	timer.armed = true
	return true
}

func (timer *attemptTimer) consume() {
	timer.armed = false
	timer.at = time.Time{}
}

func (timer *attemptTimer) stop() {
	if !timer.timer.Stop() {
		select {
		case <-timer.timer.C:
		default:
		}
	}
	timer.armed = false
}
