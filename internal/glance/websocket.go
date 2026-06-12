package glance

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow any origin since local setups might map different hostname/IP/port
		return true
	},
}

type Client struct {
	conn *websocket.Conn
	page string
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
}

var globalHub = Hub{
	clients:    make(map[*Client]bool),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

// ActiveConnections returns the number of currently connected WebSocket clients.
func ActiveConnections() int {
	globalHub.mu.Lock()
	defer globalHub.mu.Unlock()
	return len(globalHub.clients)
}

// ActivePages returns a map of page slugs currently being viewed by connected clients.
func ActivePages() map[string]bool {
	globalHub.mu.Lock()
	defer globalHub.mu.Unlock()
	pages := make(map[string]bool)
	for client := range globalHub.clients {
		if client.page != "" {
			pages[client.page] = true
		}
	}
	return pages
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Println("[WS] Client connected")

			// Push initial state to the client immediately after connection
			go triggerInitialSpotifyPush()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				client.conn.Close()
			}
			h.mu.Unlock()
			log.Println("[WS] Client disconnected")
		}
	}
}

// BroadcastMessage sends a JSON-wrapped message to all active WebSocket clients.
func BroadcastMessage(msgType string, data interface{}) {
	payload := map[string]interface{}{
		"type": msgType,
		"data": data,
	}

	msgBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WS] Failed to marshal broadcast message: %v", err)
		return
	}

	globalHub.mu.Lock()
	var failed []*Client
	for client := range globalHub.clients {
		err := client.conn.WriteMessage(websocket.TextMessage, msgBytes)
		if err != nil {
			log.Printf("[WS] Error writing to socket: %v", err)
			client.conn.Close()
			failed = append(failed, client)
		}
	}
	for _, c := range failed {
		delete(globalHub.clients, c)
	}
	globalHub.mu.Unlock()
}

func initWebSocket() {
	go globalHub.run()
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}
	pageSlug := r.URL.Query().Get("page")
	client := &Client{conn: conn, page: pageSlug}
	globalHub.register <- client

	// Set reasonable read deadline and pong handler to detect stale clients
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Send periodic pings to keep connection alive and reset client read deadlines
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read loop to detect disconnects and handle client updates
	go func() {
		defer func() {
			globalHub.unregister <- client
		}()
		for {
			_, msgBytes, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var msg struct {
				Type string `json:"type"`
				Page string `json:"page"`
			}
			if err := json.Unmarshal(msgBytes, &msg); err == nil {
				if msg.Type == "active_page" && msg.Page != "" {
					globalHub.mu.Lock()
					client.page = msg.Page
					globalHub.mu.Unlock()
				}
			}
		}
	}()
}