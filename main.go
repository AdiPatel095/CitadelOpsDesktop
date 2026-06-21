package main

import (
	"CitadelDesktop/Server/FrontendWebsocket"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	battlereport "CitadelDesktop/Server/Models/BattleReport"
	"CitadelDesktop/Server/Paths"
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

	log.Printf("Instance data directory: %s", Paths.DataDir())

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

	ResponseRegistry.BroadcastStaleSnapshot = FrontendWebsocket.BroadcastLastKnownGameStateSnapshot

	Models.StartPeriodicGameStateSnapshots()

	if err := gamedata.LoadConsumptionReductionBuildings(); err != nil {
		log.Printf("Warning: failed to load consumption reduction building index: %v", err)
	}
	if n, err := GameParser.ConstructionItemGroupBuildingsReady(); err != nil {
		log.Printf("Warning: construction item group→building map unavailable: %v", err)
	} else {
		log.Printf("[gamedata] construction item group→building wodIDs: %d groups loaded", n)
	}

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
	Logging.RegisterLogHandlers(mux)
	battlereport.RegisterStatsHandlers(mux)
	mux.HandleFunc("/ws", FrontendWebsocket.ServeWs)

	port := currentPort
	addr := fmt.Sprintf(":%d", port)
	dashboardURL := fmt.Sprintf("http://localhost:%d", port)
	ResponseRegistry.SetDashboardURL(dashboardURL)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Dashboard available at: %s", dashboardURL)

	// Listen before opening Chrome so the first websocket connect succeeds.
	go ResponseRegistry.StartGameBrowser(dashboardURL)

	if err := http.Serve(ln, mux); err != nil {
		log.Fatal(err)
	}
}
