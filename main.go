package main

import (
	"CitadelDesktop/Server/FrontendWebsocket"
	"CitadelDesktop/Server/GameFunctions"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/License"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Version"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed all:Client/dist
var frontendAssets embed.FS

func main() {
	// Initialize custom file logger (pipes to both stdout and file)
	if err := Logging.InitLogger(); err != nil {
		log.Printf("Warning: Failed to initialize file logger: %v", err)
	}
	defer Logging.CloseLogger()

	// Clean up old binary from previous update (if exists)
	Version.CleanupOldBinary()

	// Initialize the hardware ID (creates file if needed)
	if err := License.InitRegistration(); err != nil {
		log.Printf("Warning: Failed to initialize registration: %v", err)
	}

	// Create WebSocket hub
	FrontendWebsocket.InitHub()

	// Set up callbacks for License package to send messages to frontend
	License.SetSendStatusCallback(FrontendWebsocket.SendRegistrationStatusMessage)
	License.SetSendCreditsCallback(FrontendWebsocket.SendCreditsUpdateMessage)
	// Set up callback for GameWebsocket to notify frontend of insufficient credits
	ResponseRegistry.SetInsufficientCreditsCallback(FrontendWebsocket.SendInsufficientCreditsMessage)
	ResponseRegistry.SetGameLoginStatusCallback(FrontendWebsocket.SendGameLoginStatusMessage)
	ResponseRegistry.SetAutoBirdStatusCallback(FrontendWebsocket.SendAutoBirdStatus)
	ResponseRegistry.SetRequestCredentialsCallback(FrontendWebsocket.SendRequestCredentialsMessage)
	ResponseRegistry.SetMemoryStatsCallback(FrontendWebsocket.SendMemoryStatsMessage)

	// Wire GAM parser to auto-clean returned birds in real-time
	GameParser.OnGAMParsed = GameFunctions.CleanupReturnedBirds

	// Set up callbacks for Version package
	Version.SetVersionUpdateCallback(FrontendWebsocket.SendVersionUpdateMessage)
	Version.SetUpdateProgressCallback(FrontendWebsocket.SendUpdateProgressMessage)
	Version.SetUpdateCompleteCallback(FrontendWebsocket.SendUpdateCompleteMessage)
	Version.SetUpdateErrorCallback(FrontendWebsocket.SendUpdateErrorMessage)

	// Startup frontend server (always, so users can see registration status)
	go StartFrontendService()

	// Give servers a moment to start up
	time.Sleep(2 * time.Second)

	// Wait for registration (polls every 15 seconds)
	// This blocks until the hardware is registered
	// Wait for registration (polls every 15 seconds)
	// This blocks until the hardware is registered
	// Wait for registration (polls every 15 seconds)
	// This blocks until the hardware is registered
	go func() {
		if License.WaitForRegistration() {
			// Start credits sync goroutine
			go License.StartCreditsSync()
		}
	}()

	// Start version check service (runs in background)
	Version.StartVersionCheck()

	// Block forever
	select {}
}

func StartFrontendService() {
	// Create a sub-filesystem that starts from the 'Client/dist' directory
	subFS, err := fs.Sub(frontendAssets, "Client/dist")
	if err != nil {
		log.Fatal("Failed to create sub-filesystem for frontend assets:", err)
	}

	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.FS(subFS)))
	mux.HandleFunc("/ws", FrontendWebsocket.ServeWs)

	port := License.CurrentPort
	log.Printf("Dashboard available at: http://localhost:%d", port)

	// Allow CORS for development if needed, but since we are serving frontend from same origin, it's fine.
	// Actually, for local dev (vite on 5173), we might need CORS or proxy.
	// Assuming prod build for now or proxy in vite config.

	// Wrap mux with CORS middleware if necessary, or simple serve
	// For simplicity, just serve mux.
	addr := fmt.Sprintf(":%d", port)

	// Start ChromeDP with the dashboard URL right before starting the HTTP server
	dashboardURL := fmt.Sprintf("http://localhost:%d", port)
	go ResponseRegistry.StartGameBrowser(dashboardURL)

	err = http.ListenAndServe(addr, mux)
	if err != nil {
		log.Fatal(err)
	}
}
