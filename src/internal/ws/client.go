package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

//--------------------------------------------------------------------------------------|

type Client struct {
	userID int
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
}

func NewClient(userID int, hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		userID: userID,
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, 64),
	}
}

//--------------------------------------------------------------------------------------|

func (c *Client) Send(payload []byte) {
	select {
	case c.send <- payload:
	default:
	}
}

//--------------------------------------------------------------------------------------|

func (c *Client) ReadPump(handle func([]byte), onClose func()) {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
		if onClose != nil {
			onClose()
		}
	}()

	c.conn.SetReadLimit(64 * 1024)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[ws] read error: %v", err)
			}
			break
		}
		handle(message)
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(50 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

//--------------------------------------------------------------------------------------|

type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

func EncodeEvent(eventType string, data any) []byte {
	payload, _ := json.Marshal(Event{Type: eventType, Data: data})
	return payload
}
