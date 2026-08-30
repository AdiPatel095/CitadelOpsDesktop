package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/AppUpdate"
	"CitadelDesktop/Server/Paths"
	"CitadelDesktop/Server/Session"
)

func main() {
	address := flag.String("addr", defaultListenAddress(), "HTTP listen address")
	offline := flag.Bool("offline", false, "start without refreshing official game data")
	browser := flag.String("browser", os.Getenv("CITADEL_BROWSER"), "Chromium browser id, executable, or auto")
	browserPath := flag.String("browser-path", os.Getenv("CITADEL_BROWSER_PATH"), "explicit Chromium browser executable path")
	browserHeadless := flag.Bool("browser-headless", false, "run the game browser without visible windows")
	noAutoStart := flag.Bool("no-auto-start", false, "serve the dashboard without starting the game browser")
	replayLog := flag.String("replay-log", "", "stream a captured websocket log instead of launching a browser")
	replaySpeed := flag.Float64("replay-speed", 0, "capture replay speed multiplier; zero replays immediately")
	replayReady := flag.Bool("replay-ready", false, "treat an offline replay as a ready game session")
	replayAcceptOutbound := flag.Bool("replay-accept-outbound", false, "accept replay commands into an offline sink")
	replayRecordAccepted := flag.Bool("replay-record-accepted", false, "include accepted replay command counts by opcode")
	hosted := flag.Bool("hosted", false, "run an orchestrator-managed multi-runtime tenant process")
	tenantConfig := flag.String("tenant-config", os.Getenv("CITADEL_TENANT_CONFIG"), "run the static hosted canary manifest at this path")
	tenantCellID := flag.String("tenant-cell-id", environmentDefault("CITADEL_TENANT_CELL_ID", "local-cell"), "opaque hosted worker cell id")
	tenantMaxRuntimes := flag.Int("tenant-max-runtimes", 8, "maximum account runtimes in one hosted process")
	tenantControlTokenEnv := flag.String("tenant-control-token-env", "CITADEL_TENANT_ORCHESTRATOR_TOKEN", "environment variable containing the hosted orchestrator token")
	tenantSessionKeyEnv := flag.String("tenant-session-key-env", "CITADEL_TENANT_SESSION_KEY", "environment variable containing the dashboard session signing key")
	tenantPrivateMetricsURL := flag.String("tenant-private-metrics-url", os.Getenv("CITADEL_PRIVATE_METRICS_URL"), "internal CitadelOpsBackend endpoint for runtime-scoped private metrics")
	tenantCheckpointURL := flag.String("tenant-dashboard-checkpoint-url", os.Getenv("CITADEL_DASHBOARD_CHECKPOINT_URL"), "internal CitadelOpsBackend endpoint for runtime dashboard checkpoints (requires the private metrics endpoint)")
	tenantDashboardOrigins := flag.String("tenant-dashboard-origins", os.Getenv("CITADEL_TENANT_DASHBOARD_ORIGINS"), "comma-separated browser origins (for example https://app.citadelops.app) allowed to reach account shards and the tenant login besides same-host requests")
	tenantInsecureHTTP := flag.Bool("tenant-insecure-http", false, "allow hosted dashboard cookies over plain HTTP for local canary use")
	flag.Parse()

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if desktopBuild {
		if err := AppUpdate.CleanupOldExecutable(); err != nil {
			log.Printf("Could not remove the previous application binary: %v", err)
		}
	}
	hostedMode := hostedModeEnabled(*hosted, *tenantConfig)
	listenAddress := resolvedListenAddress(*address, hostedMode, commandLineFlagSet("addr"), os.Getenv("PORT"))
	var listener net.Listener
	var err error
	if hostedMode {
		listener, err = net.Listen("tcp", listenAddress)
	} else {
		listener, err = listenDashboard(listenAddress)
	}
	if err != nil {
		log.Fatal(err)
	}
	dashboardURL := localDashboardURL(listener.Addr().String())
	dataDir, err := Paths.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	if hostedMode {
		if err := runHosted(rootContext, listener, HostedOptions{
			DataRoot: dataDir, Offline: *offline, StaticConfigPath: *tenantConfig,
			Dynamic: *hosted, CellID: *tenantCellID, MaxRuntimes: *tenantMaxRuntimes,
			ControlTokenEnvironment: *tenantControlTokenEnv,
			SessionKeyEnvironment:   *tenantSessionKeyEnv,
			PrivateMetricsURL:       *tenantPrivateMetricsURL,
			CheckpointURL:           *tenantCheckpointURL,
			DashboardOrigins:        *tenantDashboardOrigins,
			SecureCookies:           !*tenantInsecureHTTP,
		}); err != nil {
			log.Fatal(err)
		}
		return
	}
	var transport Session.Transport
	var chromium *Session.ChromiumConfig
	if *replayLog != "" {
		transport = Session.NewReplayTransport(Session.ReplayConfig{
			Path: *replayLog, Speed: *replaySpeed,
			Ready: *replayReady, AcceptOutbound: *replayAcceptOutbound,
			RecordAccepted: *replayRecordAccepted,
		})
	} else {
		chromium = &Session.ChromiumConfig{
			DataDir: dataDir, DashboardURL: dashboardURL,
			Browser: *browser, ExecutablePath: *browserPath, Headless: *browserHeadless,
		}
	}
	startupContext, cancelStartup := context.WithTimeout(rootContext, 90*time.Second)
	application, err := App.New(startupContext, App.Config{
		DataDir: dataDir, Offline: *offline, Transport: transport, Chromium: chromium, RuntimeContext: rootContext,
		UpdateEndpoint: os.Getenv("CITADEL_UPDATE_URL"), UpdateInstallSupported: desktopBuild,
	})
	cancelStartup()
	if err != nil {
		log.Fatal(err)
	}
	if application.StartupErr != nil {
		log.Printf("Startup completed with degraded services: %v", application.StartupErr)
	}
	application.Start(rootContext)

	mux := http.NewServeMux()
	mux.Handle("/api/", application.API.Handler())
	mux.Handle("/", frontendHandler("Client/dist"))
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	log.Printf("CitadelOps %s listening at %s", App.Version, dashboardURL)
	if !*noAutoStart {
		go func() {
			if err := application.Session.Start(rootContext); err != nil {
				log.Printf("Game session did not auto-start: %v", err)
			}
		}()
	}
	select {
	case <-rootContext.Done():
	case serveErr := <-serveErrors:
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("Dashboard server stopped: %v", serveErr)
		}
		stop()
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownContext)
	if err := application.Wait(shutdownContext); err != nil {
		log.Printf("Application shutdown: %v", err)
	}
}

func defaultListenAddress() string {
	return "127.0.0.1:8080"
}

func resolvedListenAddress(configured string, hosted bool, explicitlySet bool, cloudPort string) string {
	if hosted && !explicitlySet && cloudPort != "" {
		return net.JoinHostPort("0.0.0.0", cloudPort)
	}
	return configured
}

func commandLineFlagSet(name string) bool {
	found := false
	flag.Visit(func(item *flag.Flag) {
		if item.Name == name {
			found = true
		}
	})
	return found
}

func environmentDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func listenDashboard(address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err == nil {
		return listener, nil
	}
	host, port, splitErr := net.SplitHostPort(address)
	if splitErr != nil || port != "8080" {
		return nil, err
	}
	fallback, fallbackErr := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if fallbackErr != nil {
		return nil, err
	}
	return fallback, nil
}

func localDashboardURL(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "http://" + address
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
