package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/Paths"
	"CitadelDesktop/Server/Session"
)

func main() {
	address := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	offline := flag.Bool("offline", false, "start without refreshing official game data")
	browser := flag.String("browser", os.Getenv("CITADEL_BROWSER"), "Chromium browser id, executable, or auto")
	browserPath := flag.String("browser-path", os.Getenv("CITADEL_BROWSER_PATH"), "explicit Chromium browser executable path")
	browserHeadless := flag.Bool("browser-headless", false, "run the game browser without visible windows")
	replayLog := flag.String("replay-log", "", "stream a captured websocket log instead of launching a browser")
	replaySpeed := flag.Float64("replay-speed", 0, "capture replay speed multiplier; zero replays immediately")
	flag.Parse()

	rootContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dataDir, err := Paths.DataDir()
	if err != nil {
		log.Fatal(err)
	}
	var transport Session.Transport
	if *replayLog != "" {
		transport = Session.NewReplayTransport(Session.ReplayConfig{Path: *replayLog, Speed: *replaySpeed})
	} else {
		transport = Session.NewChromiumTransport(Session.ChromiumConfig{
			DataDir: dataDir, DashboardURL: localDashboardURL(*address),
			Browser: *browser, ExecutablePath: *browserPath, Headless: *browserHeadless,
		})
	}
	startupContext, cancelStartup := context.WithTimeout(rootContext, 90*time.Second)
	application, err := App.New(startupContext, App.Config{
		DataDir: dataDir, Offline: *offline, Transport: transport, RuntimeContext: rootContext,
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
		Addr:              *address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		<-rootContext.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	log.Printf("CitadelOps %s listening at http://%s", App.Version, *address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
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

func frontendHandler(directory string) http.Handler {
	indexPath := filepath.Join(directory, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		files := http.FileServer(http.Dir(directory))
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			path := filepath.Join(directory, filepath.Clean(request.URL.Path))
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				files.ServeHTTP(writer, request)
				return
			}
			http.ServeFile(writer, request, indexPath)
		})
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"name": "CitadelOps", "version": App.Version,
			"detail": "Build Client/ or run the Vite development server for the desktop UI.",
		})
	})
}
