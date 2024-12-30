package internal

import (
	"context"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	sync.RWMutex
	// Registered clients.
	clients map[int64]*Client

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[int64]*Client),
	}
}

func (h *Hub) Run() {
	// Heartbeat maintenance
	threading.GoSafe(h.heartbeat)

	for {
		select {
		case client := <-h.register:
			h.Lock()
			h.clients[client.uid] = client
			h.Unlock()

		case client := <-h.unregister:
			if _, ok := h.clients[client.uid]; ok {
				h.Lock()
				delete(h.clients, client.uid)
				close(client.send)
				h.Unlock()
			}

		case message := <-h.broadcast:
			for uid, client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, uid)
				}
			}
		}
	}
}

func (h *Hub) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	ctx := context.Background()
	for {
		select {
		case <-ticker.C:
			currentTime := time.Now()
			for uid := range h.clients {
				if client, ok := h.clients[uid]; ok {
					if !(currentTime.Sub(client.lastCommTime) >= 30*time.Second) {
						continue
					}

					err := client.conn.Ping(ctx)
					if err != nil {
						h.unregister <- client
						return
					}

					client.lastCommTime = currentTime
				}
			}
		}
	}
}

func (h *Hub) SendMessage(uid int64, message []byte) {
	if client, ok := h.clients[uid]; ok {
		client.send <- message
	} else {
		logx.Error("not found client")
	}
}
