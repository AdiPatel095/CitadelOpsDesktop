package Session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const (
	chromiumEndpointStartupTimeout = 20 * time.Second
	chromiumEndpointPollInterval   = 100 * time.Millisecond
	chromiumEndpointProbeTimeout   = 750 * time.Millisecond
)

type chromiumBrowserConnection struct {
	context context.Context
	cancel  context.CancelFunc
	reused  bool
}

func chromiumProfileDir(dataDir string, browserID string) string {
	return filepath.Join(dataDir, "Browser", browserID, "Profile")
}

func connectChromiumProfile(
	ctx context.Context,
	browser BrowserCandidate,
	profileDir string,
	dashboardURL string,
	headless bool,
) (chromiumBrowserConnection, error) {
	endpoint, endpointErr := waitForChromiumEndpoint(
		ctx, profileDir, chromiumEndpointProbeTimeout,
	)
	reused := endpointErr == nil
	if !reused {
		_ = os.Remove(filepath.Join(profileDir, "DevToolsActivePort"))
		if err := launchDetachedChromium(browser, profileDir, dashboardURL, headless); err != nil {
			return chromiumBrowserConnection{}, err
		}
		endpoint, endpointErr = waitForChromiumEndpoint(
			ctx, profileDir, chromiumEndpointStartupTimeout,
		)
		if endpointErr != nil {
			return chromiumBrowserConnection{}, fmt.Errorf(
				"wait for %s DevTools endpoint: %w", browser.Name, endpointErr,
			)
		}
	}

	allocatorContext, allocatorCancel := chromedp.NewRemoteAllocator(
		context.Background(), endpoint, chromedp.NoModifyURL,
	)
	browserContext, browserCancel := chromedp.NewContext(allocatorContext)
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			browserCancel()
			allocatorCancel()
		})
	}
	stopCallerCancellation := context.AfterFunc(ctx, cleanup)
	connectionTimeout := time.AfterFunc(chromiumEndpointStartupTimeout, cleanup)
	_, connectErr := chromedp.Targets(browserContext)
	connectionTimeout.Stop()
	stopCallerCancellation()
	if connectErr != nil {
		cleanup()
		return chromiumBrowserConnection{}, fmt.Errorf(
			"connect to %s profile: %w", browser.Name, connectErr,
		)
	}
	return chromiumBrowserConnection{
		context: browserContext,
		cancel:  cleanup,
		reused:  reused,
	}, nil
}

func waitForChromiumEndpoint(
	ctx context.Context,
	profileDir string,
	timeout time.Duration,
) (string, error) {
	deadlineContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(chromiumEndpointPollInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		endpoint, err := liveChromiumEndpoint(profileDir)
		if err == nil {
			return endpoint, nil
		}
		lastErr = err
		select {
		case <-deadlineContext.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", lastErr
		case <-ticker.C:
		}
	}
}

func liveChromiumEndpoint(profileDir string) (string, error) {
	activePortPath := filepath.Join(profileDir, "DevToolsActivePort")
	contents, err := os.ReadFile(activePortPath)
	if err != nil {
		return "", err
	}
	port, websocketPath, err := parseDevToolsActivePort(contents)
	if err != nil {
		return "", err
	}
	endpoint := fmt.Sprintf("ws://127.0.0.1:%d%s", port, websocketPath)
	versionURL := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	probeContext, cancel := context.WithTimeout(context.Background(), chromiumEndpointProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeContext, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout: chromiumEndpointProbeTimeout,
			}).DialContext,
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DevTools endpoint returned %s", response.Status)
	}
	var version struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(response.Body).Decode(&version); err != nil {
		return "", err
	}
	if err := validateChromiumDebuggerURL(version.WebSocketDebuggerURL, port); err != nil {
		return "", err
	}
	return endpoint, nil
}

func parseDevToolsActivePort(contents []byte) (int, string, error) {
	lines := strings.Split(strings.TrimSpace(string(contents)), "\n")
	if len(lines) < 2 {
		return 0, "", fmt.Errorf("DevToolsActivePort is incomplete")
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("DevToolsActivePort has an invalid port")
	}
	websocketPath := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(websocketPath, "/devtools/browser/") {
		return 0, "", fmt.Errorf("DevToolsActivePort has an invalid browser path")
	}
	return port, websocketPath, nil
}

func validateChromiumDebuggerURL(value string, expectedPort int) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	if parsed.Scheme != "ws" {
		return fmt.Errorf("DevTools debugger URL uses %q", parsed.Scheme)
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("DevTools debugger URL is not loopback")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port != expectedPort {
		return fmt.Errorf("DevTools debugger URL port does not match the profile endpoint")
	}
	if !strings.HasPrefix(parsed.Path, "/devtools/browser/") {
		return fmt.Errorf("DevTools debugger URL is not a browser endpoint")
	}
	return nil
}

func launchDetachedChromium(
	browser BrowserCandidate,
	profileDir string,
	dashboardURL string,
	headless bool,
) error {
	launchURL := strings.TrimSpace(dashboardURL)
	if launchURL == "" {
		launchURL = "about:blank"
	}
	arguments := chromiumLaunchArguments(profileDir, launchURL, headless)
	command := exec.Command(browser.ExecutablePath, arguments...)
	configureDetachedChromiumProcess(command)
	if err := command.Start(); err != nil {
		return fmt.Errorf("start %s: %w", browser.Name, err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("detach %s process: %w", browser.Name, err)
	}
	return nil
}

func chromiumLaunchArguments(
	profileDir string,
	launchURL string,
	headless bool,
) []string {
	arguments := []string{
		"--disable-background-networking",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-breakpad",
		"--disable-client-side-phishing-detection",
		"--disable-default-apps",
		"--disable-dev-shm-usage",
		"--disable-extensions",
		"--disable-features=IsolateOrigins,site-per-process",
		"--disable-hang-monitor",
		"--disable-ipc-flooding-protection",
		"--disable-popup-blocking",
		"--disable-prompt-on-repost",
		"--disable-renderer-backgrounding",
		"--disable-site-isolation-trials",
		"--disable-sync",
		"--enable-automation",
		"--enable-features=NetworkService,NetworkServiceInProcess",
		"--force-color-profile=srgb",
		"--metrics-recording-only",
		"--no-default-browser-check",
		"--no-first-run",
		"--password-store=basic",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
		"--safebrowsing-disable-auto-update",
		"--use-mock-keychain",
		"--user-data-dir=" + profileDir,
	}
	if headless {
		arguments = append(arguments, "--headless", "--hide-scrollbars", "--mute-audio")
	}
	return append(arguments, launchURL)
}

func createChromiumTarget(ctx context.Context, targetURL string) (target.ID, error) {
	chromiumContext := chromedp.FromContext(ctx)
	if chromiumContext == nil || chromiumContext.Browser == nil {
		return "", fmt.Errorf("Chromium browser connection is unavailable")
	}
	return target.CreateTarget(targetURL).Do(cdp.WithExecutor(ctx, chromiumContext.Browser))
}

func ensureChromiumTarget(ctx context.Context, targetURL string) error {
	if strings.TrimSpace(targetURL) == "" {
		return nil
	}
	targets, err := chromedp.Targets(ctx)
	if err != nil {
		return err
	}
	for _, info := range targets {
		if info != nil && info.Type == "page" && sameChromiumPageURL(info.URL, targetURL) {
			return nil
		}
	}
	_, err = createChromiumTarget(ctx, targetURL)
	return err
}

func sameChromiumPageURL(left string, right string) bool {
	leftURL, leftErr := url.Parse(strings.TrimSpace(left))
	rightURL, rightErr := url.Parse(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil {
		return strings.TrimRight(left, "/") == strings.TrimRight(right, "/")
	}
	return strings.EqualFold(leftURL.Scheme, rightURL.Scheme) &&
		strings.EqualFold(leftURL.Host, rightURL.Host) &&
		strings.TrimRight(leftURL.EscapedPath(), "/") ==
			strings.TrimRight(rightURL.EscapedPath(), "/")
}

func detachChromedpTarget(ctx context.Context, cancel context.CancelFunc) {
	if cancel == nil {
		return
	}
	if chromiumContext := chromedp.FromContext(ctx); chromiumContext != nil {
		chromiumContext.Target = nil
	}
	cancel()
}
