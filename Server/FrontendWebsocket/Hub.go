package FrontendWebsocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all connections
	},
}

// FrontendSocket is the global WebSocket hub
var FrontendSocket *Hub

type Client struct {
	Conn *websocket.Conn
	Send chan []byte
}

type Hub struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	Mutex      sync.Mutex
}

func InitHub() {
	FrontendSocket = &Hub{
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
	}
	go FrontendSocket.Run()
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Mutex.Lock()
			h.Clients[client] = true
			h.Mutex.Unlock()
		case client := <-h.Unregister:
			h.Mutex.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
			}
			h.Mutex.Unlock()
		case message := <-h.Broadcast:
			h.Mutex.Lock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.Mutex.Unlock()
		}
	}
}

func (c *Client) WritePump() {
	defer func() {
		c.Conn.Close()
	}()
	for {
		message, ok := <-c.Send
		if !ok {
			c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
		c.Conn.WriteMessage(websocket.TextMessage, message)
	}
}

func (c *Client) ReadPump() {
	defer func() {
		FrontendSocket.Unregister <- c
		c.Conn.Close()
	}()
	// You can configure read limits for security
	// c.Conn.SetReadLimit(maxMessageSize)
	// c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	// c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			log.Printf("error reading message: %v", err)
			break
		}
		ParseFrontendMessage(message)
		// Process the message
	}
}

func ServeWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{Conn: conn, Send: make(chan []byte, 256)}
	FrontendSocket.Register <- client

	go client.WritePump()
	go client.ReadPump()

	SendInitialData(client)
}

func (c *Client) SendToClient(messageType string, payload interface{}, optionalData string) {
	message := map[string]interface{}{
		"type":         messageType,
		"payload":      payload,
		"optionalData": optionalData,
	}
	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Println("Error marshaling message:", err)
		return
	}
	c.Send <- jsonData
}

func SendFrontendMessage(messageType string, payload interface{}, optionalData string) {
	message := map[string]interface{}{
		"type":         messageType,
		"payload":      payload,
		"optionalData": optionalData,
	}
	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Fatal(err)
	}
	FrontendSocket.Broadcast <- jsonData

}
