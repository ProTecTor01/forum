package ws

import (
	"encoding/json"
	"log"
	"sync"
)

//--------------------------------------------------------------------------------------|

type Hub struct {
	mu      sync.RWMutex
	clients map[int]map[*Client]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[int]map[*Client]bool)}
}

func (h *Hub) Run() {
	// no-op; kept for symmetry and future extensions
}

//--------------------------------------------------------------------------------------|

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = make(map[*Client]bool)
	}
	h.clients[c.userID][c] = true
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.clients[c.userID]; ok {
		delete(m, c)
		if len(m) == 0 {
			delete(h.clients, c.userID)
		}
	}
}

func (h *Hub) OnlineUserIDs() []int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ids := make([]int, 0, len(h.clients))
	for id := range h.clients {
		ids = append(ids, id)
	}
	return ids
}

func (h *Hub) Broadcast(event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ws] marshal broadcast: %v", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, conns := range h.clients {
		for c := range conns {
			select {
			case c.send <- payload:
			default:
			}
		}
	}
}

func (h *Hub) SendToUser(userID int, event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ws] marshal send: %v", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients[userID] {
		select {
		case c.send <- payload:
		default:
		}
	}
}
