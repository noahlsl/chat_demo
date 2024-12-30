package internal

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// Maximum message size allowed from peer.
	maxMessageSize = 512

	// send buffer size
	bufSize = 256
)

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn
	// The user id
	uid int64
	// Last communication time
	lastCommTime time.Time
	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
//
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
	}()
	c.conn.SetReadLimit(maxMessageSize)

	for {
		_, message, err := c.conn.Read(context.Background())
		if err != nil {
			break
		}
		// update last communication time
		c.lastCommTime = time.Now()
		//  do some things
		log.Println(message)
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() {
	defer func() {
		_ = c.conn.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				continue
			}

			if err := c.conn.Write(context.Background(), websocket.MessageBinary, message); err != nil {
				logx.Debug(err)
				break
			}

			// update last communication time
			c.lastCommTime = time.Now()
		}
	}
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		logx.Error(err)
		return
	}

	uid := cast.ToInt64(r.FormValue("uid"))
	client := &Client{
		hub:  hub,
		uid:  uid,
		conn: conn,
		send: make(chan []byte, bufSize),
	}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
