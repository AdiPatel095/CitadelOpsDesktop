package main

import "time"

type Options struct {
	BaseURL      string
	BaselinePath string
	CPUThrottle  float64
	FailOnBudget bool
	Headed       bool
	OutputDir    string
	RepoRoot     string
	Runs         int
	SkipBuild    bool
	Timeout      time.Duration
	ViewportH    int64
	ViewportW    int64
	Warmups      int
}

type Environment struct {
	BaseURL         string  `json:"baseUrl"`
	Browser         string  `json:"browser"`
	CPUThrottle     float64 `json:"cpuThrottle"`
	GitCommit       string  `json:"gitCommit,omitempty"`
	GitDirty        bool    `json:"gitDirty"`
	Headless        bool    `json:"headless"`
	OperatingSystem string  `json:"operatingSystem"`
	ViewportHeight  int64   `json:"viewportHeight"`
	ViewportWidth   int64   `json:"viewportWidth"`
}

type StartupMetrics struct {
	AppReadyMs          float64 `json:"appReadyMs"`
	CLSTotal            float64 `json:"clsTotal"`
	DOMContentLoadedMs  float64 `json:"domContentLoadedMs"`
	DecodedBodyBytes    int64   `json:"decodedBodyBytes"`
	FirstContentfulMs   float64 `json:"firstContentfulPaintMs"`
	JSHeapBytes         int64   `json:"jsHeapBytes"`
	LargestContentfulMs float64 `json:"largestContentfulPaintMs"`
	LoadEventMs         float64 `json:"loadEventMs"`
	LongTaskCount       int     `json:"longTaskCount"`
	RequestCount        int     `json:"requestCount"`
	TotalBlockingMs     float64 `json:"totalBlockingTimeMs"`
	TransferBytes       int64   `json:"transferBytes"`
	TTFBMs              float64 `json:"ttfbMs"`
}

type ScenarioMetrics struct {
	EventDurationMs float64 `json:"eventDurationMs"`
	HeapDeltaBytes  int64   `json:"heapDeltaBytes"`
	LayoutShift     float64 `json:"layoutShift"`
	LongTaskCount   int     `json:"longTaskCount"`
	LongTaskMs      float64 `json:"longTaskMs"`
	Name            string  `json:"name"`
	PaintMs         float64 `json:"paintMs"`
	ReadyMs         float64 `json:"readyMs"`
}

type Diagnostics struct {
	ConsoleErrorCount       int      `json:"consoleErrorCount"`
	HTTPFailureCount        int      `json:"httpFailureCount"`
	NetworkFailureCount     int      `json:"networkFailureCount"`
	UnhandledExceptionCount int      `json:"unhandledExceptionCount"`
	Items                   []string `json:"items,omitempty"`
}

type RunResult struct {
	Diagnostics Diagnostics       `json:"diagnostics"`
	DurationMs  float64           `json:"durationMs"`
	Index       int               `json:"index"`
	Scenarios   []ScenarioMetrics `json:"scenarios"`
	Startup     StartupMetrics    `json:"startup"`
}

type Distribution struct {
	Maximum float64 `json:"maximum"`
	Median  float64 `json:"median"`
	Minimum float64 `json:"minimum"`
	P95     float64 `json:"p95"`
}

type StartupSummary struct {
	AppReadyMs          Distribution `json:"appReadyMs"`
	CLSTotal            Distribution `json:"clsTotal"`
	FirstContentfulMs   Distribution `json:"firstContentfulPaintMs"`
	LargestContentfulMs Distribution `json:"largestContentfulPaintMs"`
	TotalBlockingMs     Distribution `json:"totalBlockingTimeMs"`
	TransferBytes       Distribution `json:"transferBytes"`
}

type ScenarioSummary struct {
	EventDurationMs Distribution `json:"eventDurationMs"`
	LayoutShift     Distribution `json:"layoutShift"`
	LongTaskMs      Distribution `json:"longTaskMs"`
	Name            string       `json:"name"`
	PaintMs         Distribution `json:"paintMs"`
	ReadyMs         Distribution `json:"readyMs"`
}

type DiagnosticsSummary struct {
	ConsoleErrors       Distribution `json:"consoleErrors"`
	HTTPFailures        Distribution `json:"httpFailures"`
	NetworkFailures     Distribution `json:"networkFailures"`
	UnhandledExceptions Distribution `json:"unhandledExceptions"`
}

type Summary struct {
	Diagnostics DiagnosticsSummary `json:"diagnostics"`
	Scenarios   []ScenarioSummary  `json:"scenarios"`
	Startup     StartupSummary     `json:"startup"`
}

type BudgetResult struct {
	Budget float64 `json:"budget"`
	Metric string  `json:"metric"`
	Passed bool    `json:"passed"`
	Unit   string  `json:"unit"`
	Value  float64 `json:"value"`
}

type ComparisonItem struct {
	Baseline     float64 `json:"baseline"`
	Current      float64 `json:"current"`
	Delta        float64 `json:"delta"`
	DeltaPercent float64 `json:"deltaPercent"`
	Metric       string  `json:"metric"`
}

type Opportunity struct {
	BaselineDeltaPercent *float64 `json:"baselineDeltaPercent,omitempty"`
	Budget               float64  `json:"budget"`
	Metric               string   `json:"metric"`
	Priority             float64  `json:"priority"`
	Value                float64  `json:"value"`
}

type Report struct {
	BaselinePath  string           `json:"baselinePath,omitempty"`
	Budgets       []BudgetResult   `json:"budgets"`
	Comparisons   []ComparisonItem `json:"comparisons,omitempty"`
	CreatedAt     time.Time        `json:"createdAt"`
	Environment   Environment      `json:"environment"`
	Opportunities []Opportunity    `json:"opportunities"`
	Runs          []RunResult      `json:"runs"`
	SchemaVersion int              `json:"schemaVersion"`
	Summary       Summary          `json:"summary"`
}
