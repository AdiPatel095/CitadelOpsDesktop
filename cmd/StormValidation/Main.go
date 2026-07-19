package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	stormKingdomID           = 4
	stormIslandTypeID        = 24
	stormFortTypeID          = 25
	minimumScanInterval      = 65 * time.Minute
	defaultScanInterval      = 6 * time.Hour
	defaultRetryInterval     = 5 * time.Minute
	defaultRequestTimeout    = 5 * time.Minute
	sessionReadyPollInterval = 10 * time.Second
	maximumResponseBytes     = 32 << 20
	maximumEvidenceExamples  = 20
)

type options struct {
	apiURL         string
	dataDir        string
	castleID       int64
	radius         int
	interval       time.Duration
	retryInterval  time.Duration
	requestTimeout time.Duration
	maxScans       int
	reschedule     bool
	once           bool
}

type apiClient struct {
	baseURL string
	http    *http.Client
}

type healthResponse struct {
	Status  string `json:"status"`
	Session struct {
		LoggedIn    bool `json:"loggedIn"`
		SocketReady bool `json:"socketReady"`
	} `json:"session"`
}

type gameState struct {
	Revision  uint64                            `json:"revision"`
	UpdatedAt time.Time                         `json:"updatedAt"`
	Castles   map[string]castle                 `json:"castles"`
	Map       map[string]map[string]observation `json:"map"`
	Storm     struct {
		LastScannedAt map[string]time.Time `json:"lastScannedAt"`
	} `json:"storm"`
}

type castle struct {
	ID        int64  `json:"id"`
	KingdomID int    `json:"kingdomId"`
	Name      string `json:"name"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Focused   bool   `json:"focused"`
}

type observation struct {
	KingdomID              int       `json:"kingdomId"`
	X                      int       `json:"x"`
	Y                      int       `json:"y"`
	TypeID                 int       `json:"typeId"`
	Level                  int       `json:"level,omitempty"`
	OwnerID                int64     `json:"ownerId,omitempty"`
	ObjectID               int64     `json:"objectId,omitempty"`
	StormIsleID            int64     `json:"stormIsleId,omitempty"`
	StormKind              string    `json:"stormKind,omitempty"`
	StormResource          string    `json:"stormResource,omitempty"`
	StormSize              string    `json:"stormSize,omitempty"`
	StormFixedLoot         int64     `json:"stormFixedLoot,omitempty"`
	StormVictoryCount      int64     `json:"stormVictoryCount,omitempty"`
	StormCooldownRemaining int       `json:"stormCooldownRemaining,omitempty"`
	ObservedAt             time.Time `json:"observedAt"`
}

type intentReceipt struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type bounds struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

type scanSummary struct {
	Targets       int `json:"targets"`
	Forts         int `json:"forts"`
	FortsWithID   int `json:"fortsWithId"`
	CooldownForts int `json:"cooldownForts"`
	Islands       int `json:"islands"`
	IslandsWithID int `json:"islandsWithId"`
}

type scanRecord struct {
	ID             string        `json:"id"`
	StartedAt      time.Time     `json:"startedAt"`
	CompletedAt    time.Time     `json:"completedAt"`
	NextScanAt     time.Time     `json:"nextScanAt"`
	Source         castle        `json:"source"`
	Bounds         bounds        `json:"bounds"`
	Radius         int           `json:"radius"`
	StateRevision  uint64        `json:"stateRevision"`
	StateUpdatedAt time.Time     `json:"stateUpdatedAt"`
	Baseline       string        `json:"baseline"`
	Summary        scanSummary   `json:"summary"`
	Targets        []observation `json:"targets"`
}

type transition struct {
	ScanID                  string       `json:"scanId"`
	Kind                    string       `json:"kind"`
	X                       int          `json:"x"`
	Y                       int          `json:"y"`
	ObservedAt              time.Time    `json:"observedAt"`
	BeforeEffectiveCooldown int          `json:"beforeEffectiveCooldown,omitempty"`
	AfterEffectiveCooldown  int          `json:"afterEffectiveCooldown,omitempty"`
	Before                  *observation `json:"before,omitempty"`
	After                   *observation `json:"after,omitempty"`
}

type transitionBatch struct {
	ScanID      string       `json:"scanId"`
	ObservedAt  time.Time    `json:"observedAt"`
	Baseline    string       `json:"baseline"`
	Transitions []transition `json:"transitions"`
}

type evidenceSummary struct {
	UpdatedAt  time.Time               `json:"updatedAt"`
	ScanCount  int                     `json:"scanCount"`
	Counters   map[string]int          `json:"counters"`
	Examples   map[string][]transition `json:"examples"`
	Conclusion string                  `json:"conclusion"`
}

type runStatus struct {
	State             string          `json:"state"`
	Detail            string          `json:"detail,omitempty"`
	PID               int             `json:"pid"`
	APIURL            string          `json:"apiUrl"`
	CastleID          int64           `json:"castleId,omitempty"`
	Radius            int             `json:"radius"`
	Interval          string          `json:"interval"`
	LastAttemptAt     time.Time       `json:"lastAttemptAt,omitempty"`
	LastSuccessAt     time.Time       `json:"lastSuccessAt,omitempty"`
	NextScanAt        time.Time       `json:"nextScanAt,omitempty"`
	LastScanID        string          `json:"lastScanId,omitempty"`
	LastSummary       scanSummary     `json:"lastSummary"`
	LastError         string          `json:"lastError,omitempty"`
	ConsecutiveErrors int             `json:"consecutiveErrors"`
	Evidence          evidenceSummary `json:"evidence"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type collector struct {
	options options
	api     apiClient
	logger  *log.Logger
	status  runStatus
}

type scanDeferredError struct {
	until time.Time
}

func (deferred scanDeferredError) Error() string {
	return "another Storm scan already ran; next safe scan is " + deferred.until.Format(time.RFC3339)
}

func main() {
	configured := parseOptions()
	if err := os.MkdirAll(configured.dataDir, 0o700); err != nil {
		fatalf("create validation data directory: %v", err)
	}
	logFile, err := os.OpenFile(filepath.Join(configured.dataDir, "Run.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fatalf("open validation log: %v", err)
	}
	defer logFile.Close()
	logger := log.New(io.MultiWriter(os.Stdout, logFile), "", log.LstdFlags|log.Lmicroseconds|log.LUTC)
	release, err := acquirePIDFile(filepath.Join(configured.dataDir, "StormValidation.pid"))
	if err != nil {
		logger.Fatalf("acquire validator process lock: %v", err)
	}
	defer release()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	instance := collector{
		options: configured,
		api: apiClient{
			baseURL: strings.TrimRight(configured.apiURL, "/"),
			http:    &http.Client{Timeout: configured.requestTimeout},
		},
		logger: logger,
	}
	if err := instance.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("Storm validation stopped: %v", err)
	}
}

func parseOptions() options {
	configured := options{}
	flag.StringVar(&configured.apiURL, "api", "http://127.0.0.1:8080", "CitadelOps API base URL")
	flag.StringVar(&configured.dataDir, "data-dir", filepath.Join("Data", "StormValidation"), "validation artifact directory")
	flag.Int64Var(&configured.castleID, "castle-id", 0, "Storm castle id; zero auto-detects it")
	flag.IntVar(&configured.radius, "radius", 50, "scan radius around the Storm castle (1-50)")
	flag.DurationVar(&configured.interval, "interval", defaultScanInterval, "interval between successful scans")
	flag.DurationVar(&configured.retryInterval, "retry", defaultRetryInterval, "retry interval after a failed scan")
	flag.DurationVar(&configured.requestTimeout, "request-timeout", defaultRequestTimeout, "HTTP and scan request timeout")
	flag.IntVar(&configured.maxScans, "max-scans", 0, "stop after this many successful scans in the current run; zero runs continuously")
	flag.BoolVar(&configured.reschedule, "reschedule", false, "recompute the next scan from the last success and configured interval")
	flag.BoolVar(&configured.once, "once", false, "perform one due scan and exit")
	flag.Parse()
	if configured.radius < 1 || configured.radius > 50 {
		fatalf("radius must be between 1 and 50")
	}
	if configured.interval < minimumScanInterval {
		fatalf("interval must be at least %s", minimumScanInterval)
	}
	if configured.retryInterval <= 0 || configured.requestTimeout <= 0 {
		fatalf("retry and request-timeout must be positive")
	}
	if configured.maxScans < 0 {
		fatalf("max-scans cannot be negative")
	}
	absolute, err := filepath.Abs(configured.dataDir)
	if err != nil {
		fatalf("resolve data directory: %v", err)
	}
	configured.dataDir = absolute
	return configured
}

func (collector *collector) run(ctx context.Context) error {
	statusPath := filepath.Join(collector.options.dataDir, "Status.json")
	_ = readJSONFile(statusPath, &collector.status)
	collector.status.State = "starting"
	collector.status.PID = os.Getpid()
	collector.status.APIURL = collector.options.apiURL
	collector.status.Radius = collector.options.radius
	collector.status.Interval = collector.options.interval.String()
	collector.status.UpdatedAt = time.Now().UTC()
	collector.status.Evidence = collector.loadEvidence()
	if collector.options.reschedule && !collector.status.LastSuccessAt.IsZero() {
		collector.status.NextScanAt = collector.status.LastSuccessAt.Add(collector.options.interval)
		minimumNext := collector.status.LastSuccessAt.Add(minimumScanInterval)
		if collector.status.NextScanAt.Before(minimumNext) {
			collector.status.NextScanAt = minimumNext
		}
	}
	if err := writeJSONFile(statusPath, collector.status); err != nil {
		return err
	}
	collector.logger.Printf(
		"validator started api=%s radius=%d interval=%s maxScans=%d next=%s",
		collector.options.apiURL, collector.options.radius, collector.options.interval, collector.options.maxScans,
		collector.status.NextScanAt.Format(time.RFC3339),
	)

	successfulScans := 0
	for {
		now := time.Now().UTC()
		if !collector.status.NextScanAt.IsZero() && now.Before(collector.status.NextScanAt) {
			collector.status.State = "waiting"
			collector.status.Detail = ""
			collector.status.UpdatedAt = now
			if err := writeJSONFile(statusPath, collector.status); err != nil {
				return err
			}
			if collector.options.once {
				collector.logger.Printf("next scan is not due until %s", collector.status.NextScanAt.Format(time.RFC3339))
				return nil
			}
			if err := waitUntil(ctx, collector.status.NextScanAt); err != nil {
				return err
			}
		}

		if err := collector.waitForReadySession(ctx, statusPath); err != nil {
			return err
		}
		collector.status.State = "scanning"
		collector.status.Detail = ""
		collector.status.LastAttemptAt = time.Now().UTC()
		minimumNext := collector.status.LastAttemptAt.Add(minimumScanInterval)
		if collector.status.NextScanAt.Before(minimumNext) {
			collector.status.NextScanAt = minimumNext
		}
		collector.status.UpdatedAt = collector.status.LastAttemptAt
		collector.status.LastError = ""
		if err := writeJSONFile(statusPath, collector.status); err != nil {
			return err
		}
		record, err := collector.scan(ctx)
		if err != nil {
			var deferred scanDeferredError
			if errors.As(err, &deferred) {
				collector.status.State = "waiting"
				collector.status.Detail = deferred.Error()
				collector.status.LastError = ""
				collector.status.ConsecutiveErrors = 0
				if collector.status.NextScanAt.Before(deferred.until) {
					collector.status.NextScanAt = deferred.until
				}
				collector.status.UpdatedAt = time.Now().UTC()
				if writeErr := writeJSONFile(statusPath, collector.status); writeErr != nil {
					return writeErr
				}
				collector.logger.Printf("scan deferred until %s because another bounded Storm scan was observed", collector.status.NextScanAt.Format(time.RFC3339))
				if collector.options.once {
					return nil
				}
				continue
			}
			collector.status.State = "error"
			collector.status.Detail = err.Error()
			collector.status.LastError = err.Error()
			collector.status.ConsecutiveErrors++
			retryAt := time.Now().UTC().Add(collector.options.retryInterval)
			if retryAt.Before(collector.status.NextScanAt) {
				retryAt = collector.status.NextScanAt
			}
			collector.status.NextScanAt = retryAt
			if !collector.status.LastSuccessAt.IsZero() {
				minimumNext := collector.status.LastSuccessAt.Add(minimumScanInterval)
				if collector.status.NextScanAt.Before(minimumNext) {
					collector.status.NextScanAt = minimumNext
				}
			}
			collector.status.UpdatedAt = time.Now().UTC()
			_ = writeJSONFile(statusPath, collector.status)
			collector.logger.Printf("scan failed: %v; retry=%s", err, collector.status.NextScanAt.Format(time.RFC3339))
			if collector.options.once {
				return err
			}
			continue
		}
		successfulScans++
		runComplete := collector.options.maxScans > 0 && successfulScans >= collector.options.maxScans
		collector.status.State = "waiting"
		collector.status.Detail = ""
		collector.status.CastleID = record.Source.ID
		collector.status.LastSuccessAt = record.CompletedAt
		collector.status.NextScanAt = record.NextScanAt
		if runComplete {
			collector.status.State = "complete"
			collector.status.NextScanAt = time.Time{}
		}
		collector.status.LastScanID = record.ID
		collector.status.LastSummary = record.Summary
		collector.status.ConsecutiveErrors = 0
		collector.status.LastError = ""
		collector.status.Evidence = collector.loadEvidence()
		collector.status.UpdatedAt = time.Now().UTC()
		if err := writeJSONFile(statusPath, collector.status); err != nil {
			return err
		}
		collector.logger.Printf(
			"scan succeeded id=%s targets=%d forts=%d fortsWithId=%d cooldownForts=%d islands=%d next=%s conclusion=%q",
			record.ID, record.Summary.Targets, record.Summary.Forts, record.Summary.FortsWithID,
			record.Summary.CooldownForts, record.Summary.Islands, record.NextScanAt.Format(time.RFC3339),
			collector.status.Evidence.Conclusion,
		)
		if collector.options.once || runComplete {
			return nil
		}
	}
}

func (collector *collector) waitForReadySession(ctx context.Context, statusPath string) error {
	waiting := false
	for {
		detail := ""
		var health healthResponse
		if err := collector.api.get(ctx, "/api/v2/health", &health); err != nil {
			detail = "CitadelOps API is unavailable"
		} else if !health.Session.LoggedIn || !health.Session.SocketReady {
			detail = "waiting for the authenticated game socket"
		} else {
			var state gameState
			if err := collector.api.get(ctx, "/api/v2/state", &state); err != nil {
				detail = "waiting for current account state"
			} else if _, err := selectStormCastle(state, collector.options.castleID); err != nil {
				detail = "waiting for the configured Storm castle from the test account"
			} else {
				if waiting {
					collector.logger.Printf("authenticated test session is ready; continuing the due validation scan")
				}
				return nil
			}
		}

		collector.status.State = "waiting_for_session"
		collector.status.Detail = detail
		collector.status.LastError = ""
		collector.status.UpdatedAt = time.Now().UTC()
		if err := writeJSONFile(statusPath, collector.status); err != nil {
			return err
		}
		if !waiting {
			collector.logger.Printf("%s; polling every %s without consuming a scan attempt", detail, sessionReadyPollInterval)
			waiting = true
		}
		timer := time.NewTimer(sessionReadyPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (collector *collector) scan(ctx context.Context) (scanRecord, error) {
	var health healthResponse
	if err := collector.api.get(ctx, "/api/v2/health", &health); err != nil {
		return scanRecord{}, fmt.Errorf("read CitadelOps health: %w", err)
	}
	if !health.Session.LoggedIn || !health.Session.SocketReady {
		return scanRecord{}, fmt.Errorf("test account session is not logged in and socket-ready")
	}
	var beforeState gameState
	if err := collector.api.get(ctx, "/api/v2/state", &beforeState); err != nil {
		return scanRecord{}, fmt.Errorf("read pre-scan state: %w", err)
	}
	source, err := selectStormCastle(beforeState, collector.options.castleID)
	if err != nil {
		return scanRecord{}, err
	}
	if lastScan := beforeState.Storm.LastScannedAt[strconv.FormatInt(source.ID, 10)]; !lastScan.IsZero() {
		nextSafeScan := lastScan.Add(minimumScanInterval)
		if time.Now().UTC().Before(nextSafeScan) {
			return scanRecord{}, scanDeferredError{until: nextSafeScan}
		}
	}
	scanBounds := bounds{
		X1: source.X - collector.options.radius,
		Y1: source.Y - collector.options.radius,
		X2: source.X + collector.options.radius,
		Y2: source.Y + collector.options.radius,
	}
	previous, baseline := collector.baseline(beforeState, source, scanBounds)
	startedAt := time.Now().UTC()
	scanID := "storm-validation-" + startedAt.Format("20060102T150405.000000000Z")
	request := map[string]any{
		"id":    scanID,
		"actor": "validation:storm",
		"arguments": map[string]any{
			"sourceCastleId": source.ID,
			"radius":         collector.options.radius,
			"scanStartedAt":  startedAt,
		},
	}
	var receipt intentReceipt
	if err := collector.api.post(ctx, "/api/v2/intents/"+url.PathEscape("storm.map.scan"), request, &receipt); err != nil {
		return scanRecord{}, fmt.Errorf("submit bounded Storm scan: %w", err)
	}
	if receipt.Status != "succeeded" {
		if receipt.Error == "" {
			receipt.Error = "operation ended with status " + receipt.Status
		}
		return scanRecord{}, fmt.Errorf("bounded Storm scan %s: %s", receipt.ID, receipt.Error)
	}
	var afterState gameState
	if err := collector.api.get(ctx, "/api/v2/state", &afterState); err != nil {
		return scanRecord{}, fmt.Errorf("read post-scan state: %w", err)
	}
	targets := targetsInBounds(afterState, scanBounds, startedAt.Add(-time.Second))
	if len(targets) == 0 {
		return scanRecord{}, fmt.Errorf("bounded Storm scan returned no fresh forts or islands")
	}
	completedAt := time.Now().UTC()
	record := scanRecord{
		ID: scanID, StartedAt: startedAt, CompletedAt: completedAt,
		NextScanAt: completedAt.Add(collector.options.interval), Source: source, Bounds: scanBounds,
		Radius: collector.options.radius, StateRevision: afterState.Revision, StateUpdatedAt: afterState.UpdatedAt,
		Baseline: baseline, Summary: summarize(targets), Targets: targets,
	}
	transitions := compareTargets(scanID, completedAt, previous, targets)
	if err := collector.persist(record, transitionBatch{
		ScanID: scanID, ObservedAt: completedAt, Baseline: baseline, Transitions: transitions,
	}); err != nil {
		return scanRecord{}, err
	}
	return record, nil
}

func (collector *collector) baseline(state gameState, source castle, scanBounds bounds) ([]observation, string) {
	var latest scanRecord
	if readJSONFile(filepath.Join(collector.options.dataDir, "Latest.json"), &latest) == nil &&
		latest.Source.ID == source.ID && latest.Bounds == scanBounds {
		return latest.Targets, "previous_validation_scan"
	}
	return targetsInBounds(state, scanBounds, time.Time{}), "pre_scan_app_state"
}

func (collector *collector) persist(record scanRecord, batch transitionBatch) error {
	if err := appendJSONLine(filepath.Join(collector.options.dataDir, "Scans.jsonl"), record); err != nil {
		return fmt.Errorf("append scan catalog: %w", err)
	}
	if err := appendJSONLine(filepath.Join(collector.options.dataDir, "Transitions.jsonl"), batch); err != nil {
		return fmt.Errorf("append transition catalog: %w", err)
	}
	if err := writeJSONFile(filepath.Join(collector.options.dataDir, "Latest.json"), record); err != nil {
		return fmt.Errorf("write latest scan: %w", err)
	}
	evidence := collector.loadEvidence()
	evidence.ScanCount++
	evidence.UpdatedAt = record.CompletedAt
	for _, current := range batch.Transitions {
		evidence.Counters[current.Kind]++
		if len(evidence.Examples[current.Kind]) < maximumEvidenceExamples {
			evidence.Examples[current.Kind] = append(evidence.Examples[current.Kind], current)
		}
	}
	evidence.Conclusion = evidenceConclusion(evidence.Counters)
	if err := writeJSONFile(filepath.Join(collector.options.dataDir, "Evidence.json"), evidence); err != nil {
		return fmt.Errorf("write validation evidence: %w", err)
	}
	return nil
}

func (collector *collector) loadEvidence() evidenceSummary {
	evidence := evidenceSummary{}
	_ = readJSONFile(filepath.Join(collector.options.dataDir, "Evidence.json"), &evidence)
	if evidence.Counters == nil {
		evidence.Counters = map[string]int{}
	}
	if evidence.Examples == nil {
		evidence.Examples = map[string][]transition{}
	}
	if evidence.Conclusion == "" {
		evidence.Conclusion = evidenceConclusion(evidence.Counters)
	}
	return evidence
}

func selectStormCastle(state gameState, requestedID int64) (castle, error) {
	candidates := make([]castle, 0)
	for _, current := range state.Castles {
		if current.KingdomID != stormKingdomID {
			continue
		}
		if requestedID > 0 && current.ID != requestedID {
			continue
		}
		candidates = append(candidates, current)
	}
	if len(candidates) == 0 {
		if requestedID > 0 {
			return castle{}, fmt.Errorf("Storm castle %d is not available in current state", requestedID)
		}
		return castle{}, fmt.Errorf("no Storm castle is available in current state")
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].Focused != candidates[right].Focused {
			return candidates[left].Focused
		}
		return candidates[left].ID < candidates[right].ID
	})
	return candidates[0], nil
}

func targetsInBounds(state gameState, scanBounds bounds, observedAfter time.Time) []observation {
	result := make([]observation, 0)
	for _, current := range state.Map[strconv.Itoa(stormKingdomID)] {
		if current.TypeID != stormFortTypeID && current.TypeID != stormIslandTypeID {
			continue
		}
		if current.X < scanBounds.X1 || current.X > scanBounds.X2 || current.Y < scanBounds.Y1 || current.Y > scanBounds.Y2 {
			continue
		}
		if !observedAfter.IsZero() && current.ObservedAt.Before(observedAfter) {
			continue
		}
		result = append(result, current)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].X != result[right].X {
			return result[left].X < result[right].X
		}
		if result[left].Y != result[right].Y {
			return result[left].Y < result[right].Y
		}
		return result[left].TypeID < result[right].TypeID
	})
	return result
}

func summarize(targets []observation) scanSummary {
	result := scanSummary{Targets: len(targets)}
	for _, current := range targets {
		switch current.TypeID {
		case stormFortTypeID:
			result.Forts++
			if current.StormIsleID > 0 {
				result.FortsWithID++
			}
			if current.StormCooldownRemaining > 0 {
				result.CooldownForts++
			}
		case stormIslandTypeID:
			result.Islands++
			if current.StormIsleID > 0 {
				result.IslandsWithID++
			}
		}
	}
	return result
}

func compareTargets(scanID string, observedAt time.Time, before []observation, after []observation) []transition {
	beforeByCoordinate := make(map[string]observation, len(before))
	afterByCoordinate := make(map[string]observation, len(after))
	keys := map[string]struct{}{}
	for _, current := range before {
		key := coordinateKey(current.X, current.Y)
		beforeByCoordinate[key] = current
		keys[key] = struct{}{}
	}
	for _, current := range after {
		key := coordinateKey(current.X, current.Y)
		afterByCoordinate[key] = current
		keys[key] = struct{}{}
	}
	orderedKeys := make([]string, 0, len(keys))
	for key := range keys {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Slice(orderedKeys, func(left, right int) bool {
		leftObservation, leftFound := afterByCoordinate[orderedKeys[left]]
		if !leftFound {
			leftObservation = beforeByCoordinate[orderedKeys[left]]
		}
		rightObservation, rightFound := afterByCoordinate[orderedKeys[right]]
		if !rightFound {
			rightObservation = beforeByCoordinate[orderedKeys[right]]
		}
		if leftObservation.X != rightObservation.X {
			return leftObservation.X < rightObservation.X
		}
		return leftObservation.Y < rightObservation.Y
	})
	result := make([]transition, 0)
	for _, key := range orderedKeys {
		previous, previousFound := beforeByCoordinate[key]
		current, currentFound := afterByCoordinate[key]
		result = append(result, classifyTarget(scanID, observedAt, previous, previousFound, current, currentFound)...)
	}
	return result
}

func classifyTarget(
	scanID string,
	observedAt time.Time,
	previous observation,
	previousFound bool,
	current observation,
	currentFound bool,
) []transition {
	x, y := current.X, current.Y
	if !currentFound {
		x, y = previous.X, previous.Y
	}
	base := transition{ScanID: scanID, X: x, Y: y, ObservedAt: observedAt}
	if previousFound {
		copy := previous
		base.Before = &copy
		base.BeforeEffectiveCooldown = effectiveCooldown(previous, observedAt)
	}
	if currentFound {
		copy := current
		base.After = &copy
		base.AfterEffectiveCooldown = effectiveCooldown(current, observedAt)
	}
	if !previousFound {
		base.Kind = "target_appeared"
		return []transition{base}
	}
	if !currentFound {
		base.Kind = "target_disappeared"
		return []transition{base}
	}
	if previous.TypeID != current.TypeID {
		base.Kind = "target_type_changed"
		return []transition{base}
	}
	result := make([]transition, 0, 3)
	if previous.StormIsleID == 0 && current.StormIsleID > 0 {
		learned := base
		learned.Kind = "target_identity_learned"
		result = append(result, learned)
	} else if previous.StormIsleID > 0 && current.StormIsleID > 0 && previous.StormIsleID != current.StormIsleID {
		changed := base
		switch {
		case base.BeforeEffectiveCooldown > 0:
			changed.Kind = "announced_isle_changed_during_cooldown"
		case current.TypeID == stormFortTypeID && base.AfterEffectiveCooldown > 0 && current.StormVictoryCount == 0:
			changed.Kind = "replacement_announced_during_cooldown"
		default:
			changed.Kind = "target_isle_changed"
		}
		result = append(result, changed)
	}
	if current.TypeID == stormFortTypeID && previous.StormIsleID > 0 && previous.StormIsleID == current.StormIsleID {
		if base.BeforeEffectiveCooldown > 0 && base.AfterEffectiveCooldown > 0 {
			stable := base
			stable.Kind = "announced_isle_stable_during_cooldown"
			result = append(result, stable)
		}
		if previous.StormCooldownRemaining > 0 && base.BeforeEffectiveCooldown == 0 && base.AfterEffectiveCooldown == 0 {
			ready := base
			ready.Kind = "announced_isle_became_ready"
			result = append(result, ready)
		}
		if previous.StormVictoryCount != current.StormVictoryCount {
			progressed := base
			progressed.Kind = "fort_victory_count_changed"
			result = append(result, progressed)
		}
	}
	if current.TypeID == stormIslandTypeID && previous.OwnerID != current.OwnerID {
		owner := base
		owner.Kind = "island_owner_changed"
		result = append(result, owner)
	}
	return result
}

func effectiveCooldown(current observation, at time.Time) int {
	remaining := current.StormCooldownRemaining
	if remaining <= 0 || current.ObservedAt.IsZero() || !at.After(current.ObservedAt) {
		return max(0, remaining)
	}
	elapsed := int(at.Sub(current.ObservedAt) / time.Second)
	return max(0, remaining-elapsed)
}

func evidenceConclusion(counters map[string]int) string {
	if counters["announced_isle_changed_during_cooldown"] > 0 {
		return "A counterexample was observed: an announced fort identity changed before its cooldown expired."
	}
	if counters["replacement_announced_during_cooldown"] > 0 && counters["announced_isle_stable_during_cooldown"] > 0 {
		return "Supported so far: replacement fort identities are visible during cooldown and remain stable across scans."
	}
	if counters["replacement_announced_during_cooldown"] > 0 {
		return "Replacement fort identities were observed during cooldown; a later scan is still needed to prove stability."
	}
	if counters["announced_isle_stable_during_cooldown"] > 0 {
		return "Fort identities remained stable during cooldown; a newly defeated fort is still needed to prove replacement announcement."
	}
	return "Baseline collected; rollover evidence requires at least one later scan or fort defeat."
}

func coordinateKey(x int, y int) string {
	return strconv.Itoa(x) + ":" + strconv.Itoa(y)
}

func (client apiClient) get(ctx context.Context, path string, output any) error {
	return client.request(ctx, http.MethodGet, path, nil, output)
}

func (client apiClient) post(ctx context.Context, path string, input any, output any) error {
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return client.request(ctx, http.MethodPost, path, encoded, output)
}

func (client apiClient) request(ctx context.Context, method string, path string, body []byte, output any) error {
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("API returned %s: %s", response.Status, strings.TrimSpace(string(contents)))
	}
	if err := json.Unmarshal(contents, output); err != nil {
		return fmt.Errorf("decode API response: %w", err)
	}
	return nil
}

func waitUntil(ctx context.Context, deadline time.Time) error {
	delay := time.Until(deadline)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func appendJSONLine(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(value)
}

func readJSONFile(path string, output any) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(contents, output)
}

func writeJSONFile(path string, value any) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".StormValidation-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func acquirePIDFile(path string) (func(), error) {
	if contents, err := os.ReadFile(path); err == nil {
		pid, conversionErr := strconv.Atoi(strings.TrimSpace(string(contents)))
		if conversionErr == nil && pid > 0 && processRunning(pid) {
			return nil, fmt.Errorf("validator is already running as pid %d", pid)
		}
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, removeErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		file.Close()
		os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

func processRunning(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func fatalf(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", arguments...)
	os.Exit(2)
}
