package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

type BrowserHarness struct {
	browserCancel context.CancelFunc
	browserCtx    context.Context
	options       Options
	product       string
	profileDir    string
}

type scenarioDefinition struct {
	Name  string
	Title string
}

var measuredScenarios = []scenarioDefinition{
	{Name: "Open Automation", Title: "Automation"},
	{Name: "Open Equipment", Title: "Equipment"},
	{Name: "Open Movement", Title: "Movement"},
	{Name: "Load Battle Stats", Title: "Battle Stats"},
	{Name: "Load My Stats", Title: "My Stats"},
	{Name: "Return to Castle", Title: "Castle"},
}

func NewBrowserHarness(ctx context.Context, options Options) (*BrowserHarness, error) {
	profileDir, err := os.MkdirTemp("", "CitadelUIPerformance-")
	if err != nil {
		return nil, fmt.Errorf("create isolated Chrome profile: %w", err)
	}

	allocatorOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !options.Headed),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("window-size", fmt.Sprintf("%d,%d", options.ViewportW, options.ViewportH)),
		chromedp.UserDataDir(profileDir),
	)
	allocatorCtx, allocatorCancel := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	browserCtx, browserCancel := chromedp.NewContext(allocatorCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocatorCancel()
		_ = os.RemoveAll(profileDir)
		return nil, fmt.Errorf("start Chrome: %w", err)
	}

	harness := &BrowserHarness{
		browserCtx: browserCtx,
		options:    options,
		product:    "Chrome",
		profileDir: profileDir,
	}
	harness.browserCancel = func() {
		browserCancel()
		allocatorCancel()
	}

	_ = chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		_, product, _, _, _, versionErr := cdpbrowser.GetVersion().Do(ctx)
		if versionErr == nil && product != "" {
			harness.product = product
		}
		return nil
	}))

	return harness, nil
}

func (h *BrowserHarness) Close() {
	if h.browserCancel != nil {
		h.browserCancel()
	}
	_ = os.RemoveAll(h.profileDir)
}

func (h *BrowserHarness) Product() string {
	return h.product
}

func (h *BrowserHarness) Run(ctx context.Context, index int) (RunResult, error) {
	started := time.Now()
	runCtx, cancel := chromedp.NewContext(h.browserCtx)
	defer cancel()
	runCtx, timeoutCancel := context.WithTimeout(runCtx, h.options.Timeout*time.Duration(len(measuredScenarios)+2))
	defer timeoutCancel()

	diagnostics := &diagnosticCollector{}
	chromedp.ListenTarget(runCtx, diagnostics.listen)

	if err := chromedp.Run(runCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := network.Enable().Do(ctx); err != nil {
				return err
			}
			if err := network.ClearBrowserCache().Do(ctx); err != nil {
				return err
			}
			if err := network.SetCacheDisabled(true).Do(ctx); err != nil {
				return err
			}
			if err := network.SetBypassServiceWorker(true).Do(ctx); err != nil {
				return err
			}
			if err := storage.ClearDataForOrigin(baseOrigin(h.options.BaseURL), "all").Do(ctx); err != nil {
				return err
			}
			if err := cdpruntime.Enable().Do(ctx); err != nil {
				return err
			}
			if err := emulation.SetCPUThrottlingRate(h.options.CPUThrottle).Do(ctx); err != nil {
				return err
			}
			if err := emulation.SetDeviceMetricsOverride(h.options.ViewportW, h.options.ViewportH, 1, false).Do(ctx); err != nil {
				return err
			}
			_, err := page.AddScriptToEvaluateOnNewDocument(performanceInstrumentation).Do(ctx)
			return err
		}),
		chromedp.Navigate(runURL(h.options.BaseURL, index)),
		chromedp.WaitVisible("main", chromedp.ByQuery),
	); err != nil {
		return RunResult{}, fmt.Errorf("open client: %w", err)
	}

	var appReadyMs float64
	if err := chromedp.Run(runCtx, chromedp.Evaluate(
		fmt.Sprintf("window.__citadelPerf.waitForAppReady(%d)", h.options.Timeout.Milliseconds()),
		&appReadyMs,
		awaitPromise,
	)); err != nil {
		return RunResult{}, fmt.Errorf("wait for Castle view readiness: %w", err)
	}

	var startup StartupMetrics
	if err := chromedp.Run(runCtx, chromedp.Evaluate(startupMetricsExpression, &startup)); err != nil {
		return RunResult{}, fmt.Errorf("collect startup metrics: %w", err)
	}
	startup.AppReadyMs = appReadyMs

	scenarios := make([]ScenarioMetrics, 0, len(measuredScenarios))
	for _, definition := range measuredScenarios {
		metrics, err := h.measureScenario(runCtx, definition)
		if err != nil {
			return RunResult{}, fmt.Errorf("%s: %w", definition.Name, err)
		}
		scenarios = append(scenarios, metrics)
	}

	return RunResult{
		Diagnostics: diagnostics.snapshot(),
		DurationMs:  float64(time.Since(started).Microseconds()) / 1000,
		Index:       index,
		Scenarios:   scenarios,
		Startup:     startup,
	}, nil
}

func (h *BrowserHarness) measureScenario(ctx context.Context, definition scenarioDefinition) (ScenarioMetrics, error) {
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		fmt.Sprintf("window.__citadelPerf.beginScenario(%q)", definition.Name),
		nil,
	)); err != nil {
		return ScenarioMetrics{}, fmt.Errorf("start measurement: %w", err)
	}

	selector := fmt.Sprintf("[title=%q]", definition.Title)
	if err := chromedp.Run(ctx, chromedp.Click(selector, chromedp.ByQuery)); err != nil {
		return ScenarioMetrics{}, fmt.Errorf("click %q navigation: %w", definition.Title, err)
	}

	var metrics ScenarioMetrics
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		fmt.Sprintf("window.__citadelPerf.finishScenario(%q, %d)", definition.Title, h.options.Timeout.Milliseconds()),
		&metrics,
		awaitPromise,
	)); err != nil {
		return ScenarioMetrics{}, fmt.Errorf("wait for usable state: %w", err)
	}
	metrics.Name = definition.Name
	return metrics, nil
}

func awaitPromise(params *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams {
	return params.WithAwaitPromise(true)
}

func runURL(baseURL string, index int) string {
	separator := "?"
	if strings.Contains(baseURL, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%scitadelPerfRun=%d", strings.TrimRight(baseURL, "/"), separator, index)
}

func baseOrigin(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return baseURL
	}
	return parsed.Scheme + "://" + parsed.Host
}

type diagnosticCollector struct {
	mu          sync.Mutex
	diagnostics Diagnostics
}

func (c *diagnosticCollector) listen(event any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch event := event.(type) {
	case *network.EventResponseReceived:
		if event.Response != nil && event.Response.Status >= 400 {
			c.diagnostics.HTTPFailureCount++
			c.addItem(fmt.Sprintf("HTTP %d %s", event.Response.Status, event.Response.URL))
		}
	case *network.EventLoadingFailed:
		if !event.Canceled {
			c.diagnostics.NetworkFailureCount++
			c.addItem("Network failure: " + event.ErrorText)
		}
	case *cdpruntime.EventConsoleAPICalled:
		if event.Type == cdpruntime.APITypeError || event.Type == cdpruntime.APITypeAssert {
			c.diagnostics.ConsoleErrorCount++
			c.addItem("Console: " + consoleMessage(event.Args))
		}
	case *cdpruntime.EventExceptionThrown:
		c.diagnostics.UnhandledExceptionCount++
		message := "Unhandled browser exception"
		if event.ExceptionDetails != nil {
			message = event.ExceptionDetails.Text
			if event.ExceptionDetails.Exception != nil && event.ExceptionDetails.Exception.Description != "" {
				message = event.ExceptionDetails.Exception.Description
			}
		}
		c.addItem(message)
	}
}

func (c *diagnosticCollector) addItem(item string) {
	if len(c.diagnostics.Items) < 20 {
		c.diagnostics.Items = append(c.diagnostics.Items, item)
	}
}

func (c *diagnosticCollector) snapshot() Diagnostics {
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := c.diagnostics
	copy.Items = append([]string(nil), c.diagnostics.Items...)
	return copy
}

func consoleMessage(args []*cdpruntime.RemoteObject) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == nil {
			continue
		}
		value := arg.Description
		if value == "" && len(arg.Value) > 0 {
			value = string(arg.Value)
		}
		if value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return "console error"
	}
	return strings.Join(parts, " ")
}

const performanceInstrumentation = `(() => {
  const perf = {
    appReady: 0,
    cls: 0,
    eventTimings: [],
    layoutShifts: [],
    longTasks: [],
    lcp: 0,
    pendingScenario: null,
  };

  const observe = (type, callback, options) => {
    try {
      if (!PerformanceObserver.supportedEntryTypes.includes(type)) return;
      const observer = new PerformanceObserver((list) => callback(list.getEntries()));
      observer.observe(options || { type, buffered: true });
    } catch (_) {}
  };

  observe('largest-contentful-paint', (entries) => {
    for (const entry of entries) perf.lcp = Math.max(perf.lcp, entry.startTime || 0);
  });
  observe('layout-shift', (entries) => {
    for (const entry of entries) {
      perf.layoutShifts.push({
        startTime: entry.startTime || 0,
        value: entry.value || 0,
        hadRecentInput: entry.hadRecentInput === true,
      });
      if (!entry.hadRecentInput) perf.cls += entry.value || 0;
    }
  });
  observe('longtask', (entries) => {
    for (const entry of entries) {
      perf.longTasks.push({ startTime: entry.startTime || 0, duration: entry.duration || 0 });
    }
  });
  observe('event', (entries) => {
    for (const entry of entries) {
      if ((entry.interactionId || 0) > 0) {
        perf.eventTimings.push({
          startTime: entry.startTime || 0,
          duration: entry.duration || 0,
          interactionId: entry.interactionId || 0,
          name: entry.name || '',
        });
      }
    }
  }, { type: 'event', buffered: true, durationThreshold: 16 });

  const heapSize = () => performance.memory && Number.isFinite(performance.memory.usedJSHeapSize)
    ? performance.memory.usedJSHeapSize
    : 0;
  const afterPaint = () => new Promise((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(resolve));
  });
  const waitFor = async (predicate, timeoutMs, description) => {
    const deadline = performance.now() + timeoutMs;
    while (performance.now() < deadline) {
      if (predicate()) return;
      await new Promise((resolve) => requestAnimationFrame(resolve));
    }
    throw new Error('Timed out waiting for ' + description);
  };
  const mainText = () => ((document.querySelector('main') || {}).textContent || '').toUpperCase();
  const navActive = (title) => Array.from(document.querySelectorAll('[title]')).some((element) =>
    element.getAttribute('title') === title && element.classList.contains('liquid-nav-item-active')
  );
  const painted = (title) => {
    const text = mainText();
    switch (title) {
      case 'Automation': return text.includes('AUTOMATION CONTROL');
      case 'Equipment': return text.includes('RECONFIGURE COMMANDER');
      case 'Movement': return text.includes('COMMANDERS');
      case 'Battle Stats': return text.includes('BATTLE STATS') && text.includes('REPORTS COUNTED');
      case 'My Stats': return text.includes('MY STATS') && text.includes('MIGHT POINTS TREND');
      case 'Castle': return text.includes('RESOURCES') && text.includes('UNITS');
      default: return text.includes(title.toUpperCase());
    }
  };
  const settled = (title) => {
    const text = mainText();
    if (!painted(title)) return false;
    if (title === 'Battle Stats') return text.includes('READY') && !text.includes('NOT LOADED') && !text.includes('LOADING');
    if (title === 'My Stats') return !text.includes('LOADING HISTORY');
    return true;
  };

  document.addEventListener('click', (event) => {
    if (!perf.pendingScenario) return;
    const target = event.target instanceof Element ? event.target.closest('.liquid-nav-item') : null;
    if (!target) return;
    perf.pendingScenario.clickStart = event.timeStamp;
    perf.pendingScenario.startHeap = heapSize();
  }, true);

  perf.waitForAppReady = async (timeoutMs) => {
    await waitFor(() => {
      const header = ((document.querySelector('header') || {}).textContent || '').toUpperCase();
      return header.includes('GAME CONNECTED') && painted('Castle');
    }, timeoutMs, 'mocked Castle data');
    if (document.fonts && document.fonts.ready) await document.fonts.ready;
    await afterPaint();
    perf.appReady = performance.now();
    return perf.appReady;
  };

  perf.beginScenario = (name) => {
    perf.pendingScenario = {
      name,
      fallbackStart: performance.now(),
      clickStart: 0,
      startHeap: heapSize(),
    };
  };

  perf.finishScenario = async (title, timeoutMs) => {
    const scenario = perf.pendingScenario;
    if (!scenario) throw new Error('No active performance scenario');
    const start = scenario.clickStart || scenario.fallbackStart;
    await waitFor(() => navActive(title) && painted(title), timeoutMs, title + ' first paint');
    await afterPaint();
    const paintAt = performance.now();
    await waitFor(() => navActive(title) && settled(title), timeoutMs, title + ' usable state');
    await afterPaint();
    const readyAt = performance.now();
    await Promise.resolve();

    const longTasks = perf.longTasks.filter((entry) => entry.startTime >= start && entry.startTime <= readyAt);
    const eventTimings = perf.eventTimings.filter((entry) => entry.startTime >= start && entry.startTime <= readyAt);
    const layoutShift = perf.layoutShifts
      .filter((entry) => entry.startTime >= start && entry.startTime <= readyAt)
      .reduce((total, entry) => total + entry.value, 0);
    const eventDuration = eventTimings.reduce((maximum, entry) => Math.max(maximum, entry.duration), 0);
    const result = {
      eventDurationMs: eventDuration,
      heapDeltaBytes: heapSize() - scenario.startHeap,
      layoutShift,
      longTaskCount: longTasks.length,
      longTaskMs: longTasks.reduce((total, entry) => total + entry.duration, 0),
      paintMs: paintAt - start,
      readyMs: readyAt - start,
    };
    perf.pendingScenario = null;
    return result;
  };

  window.__citadelPerf = perf;
})();`

const startupMetricsExpression = `(() => {
  const perf = window.__citadelPerf;
  const navigation = performance.getEntriesByType('navigation')[0] || {};
  const resources = performance.getEntriesByType('resource');
  const fcp = performance.getEntriesByName('first-contentful-paint')[0];
  const cutoff = perf.appReady || performance.now();
  const longTasks = perf.longTasks.filter((entry) => entry.startTime <= cutoff);
  const transferBytes = resources.reduce((total, entry) => total + (entry.transferSize || 0), navigation.transferSize || 0);
  const decodedBodyBytes = resources.reduce((total, entry) => total + (entry.decodedBodySize || 0), navigation.decodedBodySize || 0);
  return {
    appReadyMs: cutoff,
    clsTotal: perf.cls || 0,
    domContentLoadedMs: navigation.domContentLoadedEventEnd || 0,
    decodedBodyBytes,
    firstContentfulPaintMs: fcp ? fcp.startTime : 0,
    jsHeapBytes: performance.memory ? performance.memory.usedJSHeapSize : 0,
    largestContentfulPaintMs: perf.lcp || 0,
    loadEventMs: navigation.loadEventEnd || 0,
    longTaskCount: longTasks.length,
    requestCount: resources.length + 1,
    totalBlockingTimeMs: longTasks.reduce((total, entry) => total + Math.max(0, entry.duration - 50), 0),
    transferBytes,
    ttfbMs: navigation.responseStart || 0,
  };
})()`
