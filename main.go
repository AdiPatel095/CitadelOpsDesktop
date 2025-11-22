package main

import (
	"CitadelDesktop/Server/Router"
	"CitadelDesktop/Server/Websocket"
	"context"
	"log"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file, please ensure it's in the project root.")
		return
	}

	// Create WebSocket hub
	hub := Websocket.NewHub()
	go hub.Run()

	// Startup frontend server
	go StartFrontendService()

	// Startup HTTP BackendServer
	go StartHTTPService(hub)

	// Give servers a moment to start up
	time.Sleep(2 * time.Second)

	// Create a new browser context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("start-maximized", false),
		chromedp.NoSandbox,
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	// Create context for the first tab
	ctx, cancelFirstTab := chromedp.NewContext(allocCtx)
	defer cancelFirstTab()
	var title1 string
	var title2 string

	err1 := chromedp.Run(ctx,
		chromedp.Navigate("http://localhost:8080"),
		chromedp.Title(&title1))
	if err1 != nil {
		log.Fatal(err1)
	}

	// Create context for the second tab
	ctx2, cancelSecondTab := chromedp.NewContext(ctx)
	defer cancelSecondTab()

	err2 := chromedp.Run(ctx2,
		chromedp.Navigate("https://empire.goodgamestudios.com/"),
		chromedp.Title(&title2))
	if err2 != nil {
		log.Fatal(err2)
	}

	Websocket.SetupWebSocketListener(ctx2, "wss://ep-live-us1-game.goodgamestudios.com/")
	Websocket.StartMessageProcessor(ctx2, hub)

	// Block forever
	select {}
}

func StartFrontendService() {
	fs := http.FileServer(http.Dir("./Client/dist"))
	http.Handle("/", fs)

	log.Println("Frontend service started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func StartHTTPService(hub *Websocket.Hub) {
	mux := Router.NewRouter(hub)
	log.Println("Backend service started on :8081")
	err := http.ListenAndServe(":8081", mux)
	if err != nil {
		log.Fatal(err)
	}
}
