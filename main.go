package main

import (
	"CitadelDesktop/Server/Core"
	"CitadelDesktop/Server/FrontendWebsocket"
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/License"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"sync"
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

	// Startup frontend server (always, so users can see registration status)
	go StartFrontendService()

	// Give servers a moment to start up
	time.Sleep(2 * time.Second)

	// Wait for registration (polls every 15 seconds)
	// This blocks until the hardware is registered
	go func() {
		if License.WaitForRegistration() {
			// Start credits sync goroutine
			go License.StartCreditsSync()

			// Now proceed with game connection
			startGameConnection()
		}
	}()

	// Block forever
	select {}
}

func startGameConnection() {
	var loginBytes [][]byte
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		loginBytes = Core.GetLoginBytes()
	}()

	wg.Wait()

	errInternalSocket := GameWebsocket.NewGameWebsocket()
	if errInternalSocket != nil {
		log.Fatal(errInternalSocket)
	}
	if loginBytes == nil {
		log.Fatal("No login bytes")
	}
	GameWebsocket.LoginToGame(loginBytes)
}

func StartFrontendService() {
	// Create a sub-filesystem that starts from the 'Client/dist' directory
	subFS, err := fs.Sub(frontendAssets, "Client/dist")
	if err != nil {
		log.Fatal("Failed to create sub-filesystem for frontend assets:", err)
	}

	http.Handle("/", http.FileServer(http.FS(subFS)))
	http.HandleFunc("/ws", FrontendWebsocket.ServeWs)

	log.Println("Dashboard available at : http://localhost:8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
