package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var errBudgetExceeded = errors.New("one or more performance budgets were exceeded")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "UI performance harness:", err)
		os.Exit(1)
	}
}

func run() error {
	options := parseOptions()
	root, err := resolveRepoRoot(options.RepoRoot)
	if err != nil {
		return err
	}
	options.RepoRoot = root
	if err := validateOptions(options); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var preview *previewServer
	if options.BaseURL == "" {
		fixture := filepath.Join(root, "Logs", "RecvCommandsJSON", "gbd.json")
		if _, err := os.Stat(fixture); err != nil {
			return fmt.Errorf("mock fixture %s is required: %w", fixture, err)
		}
		if !options.SkipBuild {
			fmt.Println("Building the production client with deterministic mock data...")
			if err := buildClient(ctx, root); err != nil {
				return err
			}
		} else if _, err := os.Stat(filepath.Join(root, "Client", "dist", "index.html")); err != nil {
			return fmt.Errorf("-skip-build requires Client/dist/index.html: %w", err)
		}

		preview, err = startPreview(ctx, root)
		if err != nil {
			return err
		}
		defer preview.Stop()
		options.BaseURL = preview.URL
	} else {
		options.BaseURL = strings.TrimRight(options.BaseURL, "/")
	}

	fmt.Printf("Running at %s with %.1fx CPU throttle and %dx%d viewport.\n",
		options.BaseURL,
		options.CPUThrottle,
		options.ViewportW,
		options.ViewportH,
	)
	harness, err := NewBrowserHarness(ctx, options)
	if err != nil {
		return err
	}
	defer harness.Close()

	for warmup := 0; warmup < options.Warmups; warmup++ {
		fmt.Printf("Warmup %d/%d...\n", warmup+1, options.Warmups)
		if _, err := harness.Run(ctx, -(warmup + 1)); err != nil {
			return fmt.Errorf("warmup %d: %w", warmup+1, err)
		}
	}

	runs := make([]RunResult, 0, options.Runs)
	for index := 1; index <= options.Runs; index++ {
		fmt.Printf("Measured run %d/%d...\n", index, options.Runs)
		result, err := harness.Run(ctx, index)
		if err != nil {
			return fmt.Errorf("run %d: %w", index, err)
		}
		runs = append(runs, result)
	}

	commit, dirty := gitState(ctx, root)
	summary := summarizeRuns(runs)
	report := Report{
		CreatedAt: time.Now(),
		Environment: Environment{
			BaseURL:         options.BaseURL,
			Browser:         harness.Product(),
			CPUThrottle:     options.CPUThrottle,
			GitCommit:       commit,
			GitDirty:        dirty,
			Headless:        !options.Headed,
			OperatingSystem: runtime.GOOS + "/" + runtime.GOARCH,
			ViewportHeight:  options.ViewportH,
			ViewportWidth:   options.ViewportW,
		},
		Runs:          runs,
		SchemaVersion: 1,
		Summary:       summary,
	}
	report.Budgets = evaluateBudgets(summary)

	if options.BaselinePath != "" {
		baselinePath := options.BaselinePath
		if !filepath.IsAbs(baselinePath) {
			baselinePath = filepath.Join(root, baselinePath)
		}
		baseline, err := loadReport(baselinePath)
		if err != nil {
			return fmt.Errorf("read baseline report: %w", err)
		}
		report.BaselinePath = baselinePath
		report.Comparisons = compareReports(summary, baseline.Summary)
		warnIncompatibleBaseline(report.Environment, baseline.Environment)
	}
	report.Opportunities = rankOpportunities(report.Budgets, report.Comparisons)

	outputDir := options.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join(root, "PerformanceArtifacts", time.Now().Format("20060102-150405"))
	} else if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(root, outputDir)
	}
	reportPath, err := writeReport(outputDir, report)
	if err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	reportPath, _ = filepath.Abs(reportPath)
	printReport(report, reportPath)

	if options.FailOnBudget && !budgetsPassed(report.Budgets) {
		return errBudgetExceeded
	}
	return nil
}

func parseOptions() Options {
	options := Options{}
	flag.StringVar(&options.BaseURL, "base-url", "", "measure an already-running mock-enabled client instead of starting Vite preview")
	flag.StringVar(&options.BaselinePath, "baseline", "", "compare against a previous Report.json")
	flag.Float64Var(&options.CPUThrottle, "cpu-rate", 1, "Chrome CPU slowdown factor (use 4 to expose lower-end hardware bottlenecks)")
	flag.BoolVar(&options.FailOnBudget, "fail-on-budget", false, "exit non-zero after writing the report when a budget fails")
	flag.BoolVar(&options.Headed, "headed", false, "show Chrome while the harness runs")
	flag.StringVar(&options.OutputDir, "output", "", "report directory (default PerformanceArtifacts/<timestamp>)")
	flag.StringVar(&options.RepoRoot, "repo-root", "", "repository root; normally discovered automatically")
	flag.IntVar(&options.Runs, "runs", 3, "number of measured runs")
	flag.BoolVar(&options.SkipBuild, "skip-build", false, "reuse the existing Client/dist production bundle")
	flag.DurationVar(&options.Timeout, "timeout", 30*time.Second, "maximum wait for each UI state")
	flag.Int64Var(&options.ViewportH, "viewport-height", 1000, "browser viewport height")
	flag.Int64Var(&options.ViewportW, "viewport-width", 1440, "browser viewport width")
	flag.IntVar(&options.Warmups, "warmups", 1, "number of unreported warmup runs")
	flag.Parse()
	return options
}

func validateOptions(options Options) error {
	if options.Runs < 1 {
		return errors.New("-runs must be at least 1")
	}
	if options.Warmups < 0 {
		return errors.New("-warmups cannot be negative")
	}
	if options.CPUThrottle < 1 {
		return errors.New("-cpu-rate must be at least 1")
	}
	if options.Timeout < time.Second {
		return errors.New("-timeout must be at least 1s")
	}
	if options.ViewportW < 800 || options.ViewportH < 600 {
		return errors.New("viewport must be at least 800x600 for the desktop client")
	}
	return nil
}

func resolveRepoRoot(explicit string) (string, error) {
	if explicit != "" {
		root, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if repoRootLooksValid(root) {
			return root, nil
		}
		return "", fmt.Errorf("%s is not a CitadelOpsDesktop repository root", root)
	}

	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if repoRootLooksValid(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", errors.New("could not find repository root containing go.mod and Client/package.json")
}

func repoRootLooksValid(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(root, "Client", "package.json")); err != nil {
		return false
	}
	return true
}

func buildClient(ctx context.Context, root string) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return errors.New("npm is required to build the client")
	}
	command := exec.CommandContext(ctx, npm, "run", "build")
	command.Dir = filepath.Join(root, "Client")
	command.Env = append(os.Environ(), "VITE_MOCK_WEBSOCKET=true")
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build client: %w", err)
	}
	return nil
}

type previewServer struct {
	Command *exec.Cmd
	Done    chan error
	URL     string
}

func startPreview(ctx context.Context, root string) (*previewServer, error) {
	node, err := exec.LookPath("node")
	if err != nil {
		return nil, errors.New("Node.js is required to serve the production client")
	}
	vite := filepath.Join(root, "Client", "node_modules", "vite", "bin", "vite.js")
	if _, err := os.Stat(vite); err != nil {
		return nil, fmt.Errorf("Vite runtime %s is unavailable: %w", vite, err)
	}
	port, err := availablePort()
	if err != nil {
		return nil, err
	}

	command := exec.CommandContext(ctx, node, vite, "preview", "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--strictPort")
	command.Dir = filepath.Join(root, "Client")
	command.Env = append(os.Environ(),
		"VITE_MOCK_WEBSOCKET=true",
		"CITADEL_PERFORMANCE_HARNESS=true",
		"CITADEL_LOCAL_BATTLE_REPORTS_ONLY=true",
	)
	var output bytes.Buffer
	command.Stdout = io.MultiWriter(os.Stdout, &output)
	command.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Vite preview: %w", err)
	}

	server := &previewServer{
		Command: command,
		Done:    make(chan error, 1),
		URL:     fmt.Sprintf("http://127.0.0.1:%d", port),
	}
	go func() {
		server.Done <- command.Wait()
	}()
	if err := waitForPreview(ctx, server, &output); err != nil {
		server.Stop()
		return nil, err
	}
	return server, nil
}

func (server *previewServer) Stop() {
	if server == nil || server.Command == nil || server.Command.Process == nil {
		return
	}
	_ = server.Command.Process.Kill()
	select {
	case <-server.Done:
	case <-time.After(5 * time.Second):
	}
}

func availablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve preview port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForPreview(ctx context.Context, server *previewServer, output *bytes.Buffer) error {
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-server.Done:
			return fmt.Errorf("Vite preview exited before it was ready: %v\n%s", err, output.String())
		case <-deadline.C:
			return fmt.Errorf("Vite preview did not become ready at %s\n%s", server.URL, output.String())
		case <-ticker.C:
			response, err := client.Get(server.URL)
			if err != nil {
				continue
			}
			_ = response.Body.Close()
			if response.StatusCode >= 200 && response.StatusCode < 400 {
				return nil
			}
		}
	}
}

func gitState(ctx context.Context, root string) (string, bool) {
	commitCommand := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
	commitCommand.Dir = root
	commitOutput, _ := commitCommand.Output()
	statusCommand := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCommand.Dir = root
	statusOutput, _ := statusCommand.Output()
	return strings.TrimSpace(string(commitOutput)), len(bytes.TrimSpace(statusOutput)) > 0
}

func warnIncompatibleBaseline(current Environment, baseline Environment) {
	var differences []string
	if current.CPUThrottle != baseline.CPUThrottle {
		differences = append(differences, "CPU throttle")
	}
	if current.ViewportWidth != baseline.ViewportWidth || current.ViewportHeight != baseline.ViewportHeight {
		differences = append(differences, "viewport")
	}
	if current.Browser != baseline.Browser {
		differences = append(differences, "browser")
	}
	if len(differences) > 0 {
		fmt.Printf("Warning: baseline differs in %s; percentage deltas may include environment noise.\n", strings.Join(differences, ", "))
	}
}
