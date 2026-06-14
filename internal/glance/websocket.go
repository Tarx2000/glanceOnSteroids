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
	mu   sync.Mutex
}

// WriteMessage writes a message to the client connection in a thread-safe manner with a write deadline.
func (c *Client) WriteMessage(messageType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(messageType, data)
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 256),
		unregister: make(chan *Client, 256),
	}
}

// ActiveConnections returns the number of currently connected WebSocket clients.
func (h *Hub) ActiveConnections() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// ActivePages returns a map of page slugs currently being viewed by connected clients.
func (h *Hub) ActivePages() map[string]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	pages := make(map[string]bool)
	for client := range h.clients {
		if client.page != "" {
			pages[client.page] = true
		}
	}
	return pages
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Println("[WS] Client connected")

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
func (h *Hub) BroadcastMessage(msgType string, data interface{}) {
	payload := map[string]interface{}{
		"type": msgType,
		"data": data,
	}

	msgBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WS] Failed to marshal broadcast message: %v", err)
		return
	}

	h.mu.Lock()
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	h.mu.Unlock()

	var failed []*Client
	for _, client := range clients {
		err := client.WriteMessage(websocket.TextMessage, msgBytes)
		if err != nil {
			log.Printf("[WS] Error writing to socket: %v", err)
			client.conn.Close()
			failed = append(failed, client)
		}
	}

	if len(failed) > 0 {
		h.mu.Lock()
		for _, c := range failed {
			delete(h.clients, c)
		}
		h.mu.Unlock()
	}
}

func (a *Application) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}
	pageSlug := r.URL.Query().Get("page")
	client := &Client{conn: conn, page: pageSlug}
	a.Hub.register <- client

	// Trigger initial Spotify push via poller
	if a.SpotifyPoller != nil {
		go a.SpotifyPoller.TriggerInitialPush()
	}

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
				if err := client.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read loop to detect disconnects and handle client updates
	go func() {
		defer func() {
			a.Hub.unregister <- client
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
					a.Hub.mu.Lock()
					client.page = msg.Page
					a.Hub.mu.Unlock()
				}
			}
		}
	}()
}