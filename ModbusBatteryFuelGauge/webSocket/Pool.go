package websocket

import (
	"github.com/gorilla/websocket"
	"log"
)

type Pool struct {
	Register   chan *Client
	Unregister chan *Client
	Clients    map[*Client]bool
	Broadcast  chan []byte
}

func NewPool() *Pool {
	return &Pool{
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Clients:    make(map[*Client]bool),
		Broadcast:  make(chan []byte),
	}
}

func (pool *Pool) Start() {
	for {
		select {
		case client := <-pool.Register:
			pool.Clients[client] = true
			break
		case client := <-pool.Unregister:
			delete(pool.Clients, client)
			break
		case message := <-pool.Broadcast:
			for client := range pool.Clients {
				if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
					log.Printf("Broadcast update error - %s\n", err)
					delete(pool.Clients, client)
				}
			}
		}
	}
}
