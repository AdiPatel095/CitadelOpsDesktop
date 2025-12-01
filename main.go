package main

import (
	"CitadelDesktop/Server/Core"
	"CitadelDesktop/Server/GameWebsocket" // Assuming this is the correct path
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

	// Create WebSocket hub
	Core.InitHub()

	// Startup frontend server
	go StartFrontendService()

	// Give servers a moment to start up
	time.Sleep(2 * time.Second)

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

	// Block forever
	select {}
}

func StartFrontendService() {
	// Create a sub-filesystem that starts from the 'Client/dist' directory
	subFS, err := fs.Sub(frontendAssets, "Client/dist")
	if err != nil {
		log.Fatal("Failed to create sub-filesystem for frontend assets:", err)
	}

	http.Handle("/", http.FileServer(http.FS(subFS)))
	http.HandleFunc("/ws", Core.ServeWs)

	log.Println("Dashboard available at : http://localhost:8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
