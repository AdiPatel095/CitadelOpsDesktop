package main

import (
	"CitadelDesktop/Server/FrontendWebsocket"
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/License"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed all:Client/dist
var frontendAssets embed.FS

func main() {
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
	GameWebsocket.SetInsufficientCreditsCallback(FrontendWebsocket.SendInsufficientCreditsMessage)
	GameWebsocket.SetGameLoginStatusCallback(FrontendWebsocket.SendGameLoginStatusMessage)

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

	log.Println("Dashboard available at : http://localhost:8080")
	// Allow CORS for development if needed, but since we are serving frontend from same origin, it's fine.
	// Actually, for local dev (vite on 5173), we might need CORS or proxy.
	// Assuming prod build for now or proxy in vite config.

	// Wrap mux with CORS middleware if necessary, or simple serve
	// For simplicity, just serve mux.
	err = http.ListenAndServe(":8080", mux)
	if err != nil {
		log.Fatal(err)
	}
}
