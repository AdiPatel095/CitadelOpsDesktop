package main

import (
	"CitadelDesktop/Server/FrontendWebsocket"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Version"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
)

//go:embed all:Client/dist
var frontendAssets embed.FS

var currentPort int

func main() {
	// Initialize custom file logger (pipes to both stdout and file)
	if err := Logging.InitLogger(); err != nil {
		log.Printf("Warning: Failed to initialize file logger: %v", err)
	}
	if err := Logging.InitChannelLogs(); err != nil {
		log.Printf("Warning: Failed to initialize channel logs: %v", err)
	}
	defer Logging.CloseLogger()
	defer Logging.CloseChannelLogs()

	// Clean up old binary from previous update (if exists)
	Version.CleanupOldBinary()

	// Initialize the port for the frontend service
	port, err := findAvailablePort(8080)
	if err != nil {
		log.Printf("Warning: Failed to initialize port: %v", err)
	}
	currentPort = port
	log.Printf("Frontend port initialized: %d", currentPort)

	// Create WebSocket hub
	FrontendWebsocket.InitHub()

	// Set up callbacks for ResponseRegistry to notify frontend
	ResponseRegistry.SetGameLoginStatusCallback(FrontendWebsocket.SendGameLoginStatusMessage)
	ResponseRegistry.SetAutoBirdStatusCallback(FrontendWebsocket.SendAutoBirdStatus)
	ResponseRegistry.SetMemoryStatsCallback(FrontendWebsocket.SendMemoryStatsMessage)

	// Set up callbacks for Version package
	Version.SetVersionUpdateCallback(FrontendWebsocket.SendVersionUpdateMessage)
	Version.SetUpdateProgressCallback(FrontendWebsocket.SendUpdateProgressMessage)
	Version.SetUpdateCompleteCallback(FrontendWebsocket.SendUpdateCompleteMessage)
	Version.SetUpdateErrorCallback(FrontendWebsocket.SendUpdateErrorMessage)

	// Startup frontend server
	go StartFrontendService()

	// Start version check service (runs in background)
	Version.StartVersionCheck()

	// Block forever
	select {}
}

func findAvailablePort(preferredPort int) (int, error) {
	// Try a preferred port first, if available
	if preferredPort > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", preferredPort))
		if err == nil {
			_ = ln.Close()
			return preferredPort, nil
		}
	}

	// Otherwise, let the OS choose
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func StartFrontendService() {
	// Create a sub-filesystem that starts from the 'Client/dist' directory
	subFS, err := fs.Sub(frontendAssets, "Client/dist")
	if err != nil {
		log.Fatal("Failed to create sub-filesystem for frontend assets:", err)
	}

	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.FS(subFS)))
	mux.Handle("/api/game-data/", http.StripPrefix("/api/game-data/", http.FileServer(http.Dir("Server/Data"))))
	Logging.RegisterLogHandlers(mux)
	mux.HandleFunc("/ws", FrontendWebsocket.ServeWs)

	port := currentPort
	log.Printf("Dashboard available at: http://localhost:%d", port)

	// Allow CORS for development if needed, but since we are serving frontend from same origin, it's fine.
	// Actually, for local dev (vite on 5173), we might need CORS or proxy.
	// Assuming prod build for now or proxy in vite config.

	// Wrap mux with CORS middleware if necessary, or simple serve
	// For simplicity, just serve mux.
	addr := fmt.Sprintf(":%d", port)

	// Start ChromeDP with the dashboard URL right before starting the HTTP server
	dashboardURL := fmt.Sprintf("http://localhost:%d", port)
	ResponseRegistry.SetDashboardURL(dashboardURL)
	go ResponseRegistry.StartGameBrowser(dashboardURL)

	err = http.ListenAndServe(addr, mux)
	if err != nil {
		log.Fatal(err)
	}
}
