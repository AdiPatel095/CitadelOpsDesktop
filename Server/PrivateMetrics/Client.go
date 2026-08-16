package PrivateMetrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	requestTimeout    = 15 * time.Second
	checkpointTimeout = 60 * time.Second
	maximumRetryAfter = time.Hour
	// maximumCheckpointBytes bounds one compressed dashboard checkpoint body.
	// Real profiles compress to well under 1 MB; anything larger is refused
	// locally instead of being retried against the backend.
	maximumCheckpointBytes = 8 << 20
)

type ClientConfig struct {
	Client        *http.Client
	Endpoint      string
	ClientVersion string
	// CheckpointEndpoint optionally receives dashboard checkpoints (the
	// projection the dashboard renders) under the same runtime placement grant.
	CheckpointEndpoint string
}

// Client is immutable and safe to share across account runtimes. Runtime
// identity and authorization remain per-request Placement values.
type Client struct {
	client             *http.Client
	endpoint           string
	checkpointEndpoint string
	clientVersion      string
}

// UploadOutcome classifies why a publication did not succeed so the publisher
// can decide between replaying the identical sample, building a fresh one, or
// waiting for a new placement.
type UploadOutcome int

const (
	// OutcomeTransient covers network failures, timeouts, throttling, and
	// backend unavailability. The identical sample may be replayed under the
	// same idempotency key.
	OutcomeTransient UploadOutcome = iota
	// OutcomeRejected means the backend durably refused this sample body or
	// its fencing labels. Replaying the identical request cannot succeed; the
	// publisher must build a fresh sample instead.
	OutcomeRejected
	// OutcomeUnauthorized means the grant itself was refused. The runtime
	// should stop spending the credential until orchestration rotates it.
	OutcomeUnauthorized
)

func (outcome UploadOutcome) String() string {
	switch outcome {
	case OutcomeTransient:
		return "transient"
	case OutcomeRejected:
		return "rejected"
	case OutcomeUnauthorized:
		return "unauthorized"
	default:
		return "unknown"
	}
}

// PublishError describes a failed publication without ever carrying the
// backend response body, which may contain private detail.
type PublishError struct {
	Outcome    UploadOutcome
	StatusCode int
	Code       string
	RetryAfter time.Duration
	Err        error
}

func (publishErr *PublishError) Error() string {
	if publishErr == nil {
		return "publish private metrics failed"
	}
	if publishErr.Err != nil {
		return fmt.Sprintf("publish private metrics: %v", publishErr.Err)
	}
	status := http.StatusText(publishErr.StatusCode)
	if status == "" {
		status = "unexpected status"
	}
	if publishErr.Code == "" {
		return fmt.Sprintf("publish private metrics: backend returned %d %s", publishErr.StatusCode, status)
	}
	return fmt.Sprintf("publish private metrics: backend returned %d %s (%s)", publishErr.StatusCode, status, publishErr.Code)
}

func (publishErr *PublishError) Unwrap() error {
	if publishErr == nil {
		return nil
	}
	return publishErr.Err
}

// OutcomeOf classifies any Upload error. Unknown errors are treated as
// transient so an unexpected local failure never discards a valid sample.
func OutcomeOf(err error) UploadOutcome {
	var publishErr *PublishError
	if errors.As(err, &publishErr) && publishErr != nil {
		return publishErr.Outcome
	}
	return OutcomeTransient
}

func NewClient(config ClientConfig) (*Client, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		return nil, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("private metrics endpoint must be an absolute HTTP(S) URL without a query or fragment")
	}
	checkpointEndpoint := strings.TrimSpace(config.CheckpointEndpoint)
	if checkpointEndpoint != "" {
		parsedCheckpoint, err := url.Parse(checkpointEndpoint)
		if err != nil || parsedCheckpoint.Host == "" || parsedCheckpoint.RawQuery != "" || parsedCheckpoint.Fragment != "" ||
			(parsedCheckpoint.Scheme != "https" && parsedCheckpoint.Scheme != "http") {
			return nil, fmt.Errorf("dashboard checkpoint endpoint must be an absolute HTTP(S) URL without a query or fragment")
		}
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: checkpointTimeout}
	}
	return &Client{
		client: client, endpoint: strings.TrimRight(endpoint, "/"),
		checkpointEndpoint: strings.TrimRight(checkpointEndpoint, "/"),
		clientVersion:      strings.TrimSpace(config.ClientVersion),
	}, nil
}

func (client *Client) Enabled() bool {
	return client != nil && client.client != nil && client.endpoint != ""
}

// CheckpointsEnabled reports whether dashboard checkpoints have a destination.
func (client *Client) CheckpointsEnabled() bool {
	return client.Enabled() && client.checkpointEndpoint != ""
}

func (client *Client) CheckpointEndpoint() string {
	if client == nil {
		return ""
	}
	return client.checkpointEndpoint
}

// UploadCheckpoint publishes one dashboard checkpoint under the placement's
// grant. The body is gzip-compressed JSON; the idempotency key is the
// checkpoint ID so a retried upload never duplicates a checkpoint.
func (client *Client) UploadCheckpoint(ctx context.Context, placement Placement, checkpoint Checkpoint) error {
	if !client.CheckpointsEnabled() {
		return fmt.Errorf("dashboard checkpoint client is unavailable")
	}
	if strings.TrimSpace(placement.Grant.Token) == "" {
		return fmt.Errorf("private metrics grant is unavailable")
	}
	requestBody := CheckpointRequest{
		SchemaVersion: CheckpointSchemaVersion, CellID: placement.CellID,
		TenantID: placement.TenantID, RuntimeID: placement.RuntimeID,
		PlacementEpoch: placement.PlacementEpoch, DesiredRevision: placement.DesiredRevision,
		LeaseExpiresAt: placement.LeaseExpiresAt.UTC(), Checkpoint: checkpoint,
	}
	var compressed bytes.Buffer
	gzipWriter, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("compress dashboard checkpoint: %w", err)
	}
	if err := json.NewEncoder(gzipWriter).Encode(requestBody); err != nil {
		return fmt.Errorf("encode dashboard checkpoint: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("compress dashboard checkpoint: %w", err)
	}
	if compressed.Len() > maximumCheckpointBytes {
		return &PublishError{Outcome: OutcomeRejected, Code: "checkpoint_too_large"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.checkpointEndpoint, bytes.NewReader(compressed.Bytes()))
	if err != nil {
		return fmt.Errorf("create dashboard checkpoint request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+placement.Grant.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	request.Header.Set("Idempotency-Key", checkpoint.CheckpointID)
	request.Header.Set("X-Citadel-Checkpoint-Schema", strconv.Itoa(CheckpointSchemaVersion))
	if client.clientVersion != "" {
		request.Header.Set("User-Agent", "CitadelOpsDesktop/"+client.clientVersion)
	}
	return client.do(request)
}

func (client *Client) Endpoint() string {
	if client == nil {
		return ""
	}
	return client.endpoint
}

func (client *Client) Upload(ctx context.Context, placement Placement, sample Sample) error {
	if !client.Enabled() {
		return fmt.Errorf("private metrics client is unavailable")
	}
	if strings.TrimSpace(placement.Grant.Token) == "" {
		return fmt.Errorf("private metrics grant is unavailable")
	}
	requestBody := PublishRequest{
		SchemaVersion: SchemaVersion, CellID: placement.CellID,
		TenantID: placement.TenantID, RuntimeID: placement.RuntimeID,
		PlacementEpoch: placement.PlacementEpoch, DesiredRevision: placement.DesiredRevision,
		LeaseExpiresAt: placement.LeaseExpiresAt.UTC(), Sample: sample,
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("encode private metrics sample: %w", err)
	}
	requestContext, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, client.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create private metrics request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+placement.Grant.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	request.Header.Set("Idempotency-Key", sample.SampleID)
	request.Header.Set("X-Citadel-Metrics-Schema", strconv.Itoa(SchemaVersion))
	if client.clientVersion != "" {
		request.Header.Set("User-Agent", "CitadelOpsDesktop/"+client.clientVersion)
	}
	return client.do(request)
}

// do sends a publication request and classifies the answer without ever
// surfacing the backend response body.
func (client *Client) do(request *http.Request) error {
	response, err := client.client.Do(request)
	if err != nil {
		return &PublishError{Outcome: OutcomeTransient, Err: err}
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil
	}
	var structured struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	_ = json.Unmarshal(body, &structured)
	return &PublishError{
		Outcome: classifyStatus(response.StatusCode), StatusCode: response.StatusCode,
		Code:       sanitizeErrorCode(structured.Error.Code),
		RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
	}
}

func classifyStatus(status int) UploadOutcome {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return OutcomeUnauthorized
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return OutcomeTransient
	case status >= http.StatusBadRequest:
		return OutcomeRejected
	default:
		return OutcomeTransient
	}
}

// sanitizeErrorCode keeps only a short machine-readable code so a misbehaving
// backend cannot smuggle free text into runtime status or logs.
func sanitizeErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) > 64 {
		return ""
	}
	for _, character := range code {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return ""
		}
	}
	return code
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return min(time.Duration(seconds)*time.Second, maximumRetryAfter)
	}
	if at, err := http.ParseTime(value); err == nil {
		delay := at.Sub(now)
		if delay <= 0 {
			return 0
		}
		return min(delay, maximumRetryAfter)
	}
	return 0
}
