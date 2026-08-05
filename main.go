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
	address := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	offline := flag.Bool("offline", false, "start without refreshing official game data")
	browser := flag.String("browser", os.Getenv("CITADEL_BROWSER"), "Chromium browser id, executable, or auto")
	browserPath := flag.String("browser-path", os.Getenv("CITADEL_BROWSER_PATH"), "explicit Chromium browser executable path")
	browserHeadless := flag.Bool("browser-headless", false, "run the game browser without visible windows")
	noAutoStart := flag.Bool("no-auto-start", false, "serve the dashboard without starting the game browser")
	replayLog := flag.String("replay-log", "", "stream a captured websocket log instead of launching a browser")
	replaySpeed := flag.Float64("replay-speed", 0, "capture replay speed multiplier; zero replays immediately")
	flag.Parse()

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if desktopBuild {
		if err := AppUpdate.CleanupOldExecutable(); err != nil {
			log.Printf("Could not remove the previous application binary: %v", err)
		}
	}
	listener, err := listenDashboard(*address)
	if err != nil {
		log.Fatal(err)
	}
	dashboardURL := localDashboardURL(listener.Addr().String())
	dataDir, err := Paths.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	var transport Session.Transport
	var chromium *Session.ChromiumConfig
	if *replayLog != "" {
		transport = Session.NewReplayTransport(Session.ReplayConfig{Path: *replayLog, Speed: *replaySpeed})
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
