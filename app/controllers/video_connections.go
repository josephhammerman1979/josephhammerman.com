package controllers

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// TopicManager structure to handle channel-based pubsub
type TopicManager struct {
	topics   map[string][]chan []byte
	control  chan topicOperation
	shutdown chan struct{}
	mu       sync.Mutex // Mutex for thread-safe map access
}

type topicOperation struct {
	opType  string // "sub", "unsub", "pub"
	topic   string
	ch      chan []byte
	message []byte
}

// Add a constant for the keep-alive interval
const keepAliveInterval = 30 * time.Second

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Accepting all requests
	},
}

func NewTopicManager() *TopicManager {
	tm := &TopicManager{
		topics:   make(map[string][]chan []byte),
		control:  make(chan topicOperation, 100), // Buffered channel for backpressure
		shutdown: make(chan struct{}),
	}
	go tm.run()
	return tm
}

func (tm *TopicManager) run() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case op := <-tm.control:
			tm.mu.Lock()
			switch op.opType {
			case "sub":
				tm.topics[op.topic] = append(tm.topics[op.topic], op.ch)
				log.Printf("[TopicManager] Subscribed channel to topic: %s (total subscribers: %d)", op.topic, len(tm.topics[op.topic]))
			case "unsub":
				for i, ch := range tm.topics[op.topic] {
					if ch == op.ch {
						tm.topics[op.topic] = append(tm.topics[op.topic][:i], tm.topics[op.topic][i+1:]...)
						log.Printf("[TopicManager] Unsubscribed channel from topic: %s (remaining subscribers: %d)", op.topic, len(tm.topics[op.topic]))
						close(ch)
						break
					}
				}
			case "pub":
				for _, ch := range tm.topics[op.topic] {
					select {
					case ch <- op.message:
					default: // Avoid blocking on full channels
						log.Printf("Channel full for topic %s", op.topic)
					}
				}
			}
			tm.mu.Unlock()

		case <-ticker.C:
			tm.mu.Lock()
			// Cleanup empty topics
			for topic, chans := range tm.topics {
				if len(chans) == 0 {
					delete(tm.topics, topic)
				}
			}
			tm.mu.Unlock()

		case <-tm.shutdown:
			return
		}
	}
}

func VideoConnections(tm *TopicManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// Get URL parameters
		userID := r.URL.Query().Get("userID")
		peerID := r.URL.Query().Get("peerID")

		// Create context with timeout
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()

		// Initialize message channel
		msgChan := make(chan []byte, 10)
		defer close(msgChan)

		// Subscribe to user's topic
		tm.control <- topicOperation{
			opType: "sub",
			topic:  userID,
			ch:     msgChan,
		}
		defer func() {
			tm.control <- topicOperation{
				opType: "unsub",
				topic:  userID,
				ch:     msgChan,
			}
		}()

		// 5. Single goroutine for bidirectional communication
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-msgChan:
					if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
						log.Printf("Write error: %v", err)
						return
					}
				}
			}
		}()

		// Read loop
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_, message, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err) {
						log.Printf("Unexpected close: %v", err)
					}
					return
				}

				// Publish to peer's topic
				tm.control <- topicOperation{
					opType:  "pub",
					topic:   peerID,
					message: message,
				}
			}
		}
	}
}
