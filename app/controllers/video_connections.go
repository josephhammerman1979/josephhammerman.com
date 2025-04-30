package controllers

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// TopicManager handles pubsub with message buffering and cleanup
type TopicManager struct {
	topics   map[string]*topicData
	control  chan topicOperation
	shutdown chan struct{}
	mu       sync.Mutex
}

type topicData struct {
	subscribers []chan []byte
	buffer      [][]byte
	bufferSize  int
}

type topicOperation struct {
	opType  string // "sub", "unsub", "pub"
	topic   string
	ch      chan []byte
	message []byte
}

const (
	keepAliveInterval   = 30 * time.Second
	messageBufferSize   = 100 // Increased from 10
	topicBufferCapacity = 20  // Messages to buffer per topic with no subscribers
	writeTimeout        = 5 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewTopicManager() *TopicManager {
	tm := &TopicManager{
		topics:   make(map[string]*topicData),
		control:  make(chan topicOperation, 200), // Larger buffer for backpressure
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
			tm.processOperation(op)
		case <-ticker.C:
			tm.cleanupTopics()
		case <-tm.shutdown:
			return
		}
	}
}

func (tm *TopicManager) processOperation(op topicOperation) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	switch op.opType {
	case "sub":
		tm.handleSubscribe(op)
	case "unsub":
		tm.handleUnsubscribe(op)
	case "pub":
		tm.handlePublish(op)
	}
}

func (tm *TopicManager) handleSubscribe(op topicOperation) {
	td := tm.getOrCreateTopic(op.topic)

	// Send buffered messages first
	for _, msg := range td.buffer {
		select {
		case op.ch <- msg:
		default: // Don't block on full channel
		}
	}
	td.buffer = nil

	td.subscribers = append(td.subscribers, op.ch)
	log.Printf("[TopicManager] Subscribed to %s (subscribers: %d)", op.topic, len(td.subscribers))
}

func (tm *TopicManager) handleUnsubscribe(op topicOperation) {
	if td, exists := tm.topics[op.topic]; exists {
		for i, ch := range td.subscribers {
			if ch == op.ch {
				td.subscribers = append(td.subscribers[:i], td.subscribers[i+1:]...)
				close(ch)
				log.Printf("[TopicManager] Unsubscribed from %s (remaining: %d)", op.topic, len(td.subscribers))
				return
			}
		}
	}
}

func (tm *TopicManager) handlePublish(op topicOperation) {
	td := tm.getOrCreateTopic(op.topic)

	if len(td.subscribers) == 0 {
		// Buffer message if within capacity
		if len(td.buffer) < td.bufferSize {
			td.buffer = append(td.buffer, op.message)
		}
		return
	}

	for _, ch := range td.subscribers {
		select {
		case ch <- op.message:
		case <-time.After(writeTimeout):
			log.Printf("[TopicManager] Timeout publishing to %s", op.topic)
		}
	}
}

func (tm *TopicManager) getOrCreateTopic(topic string) *topicData {
	if td, exists := tm.topics[topic]; exists {
		return td
	}
	td := &topicData{
		bufferSize: topicBufferCapacity,
	}
	tm.topics[topic] = td
	return td
}

func (tm *TopicManager) cleanupTopics() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for topic, td := range tm.topics {
		if len(td.subscribers) == 0 && len(td.buffer) == 0 {
			delete(tm.topics, topic)
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

		userID := r.URL.Query().Get("userID")
		peerID := r.URL.Query().Get("peerID")
		log.Printf("[Connection] New connection - User: %s, Peer: %s", userID, peerID)

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
		defer cancel()

		msgChan := make(chan []byte, messageBufferSize)
		defer close(msgChan)

		// Register connection
		tm.control <- topicOperation{opType: "sub", topic: userID, ch: msgChan}
		defer func() {
			tm.control <- topicOperation{opType: "unsub", topic: userID, ch: msgChan}
		}()

		// Use separate write pump
		writeDone := make(chan struct{})
		go writePump(conn, msgChan, writeDone, ctx)

		// Read pump
		readPump(conn, tm, peerID, ctx)

		// Wait for writes to complete
		<-writeDone
	}
}

func writePump(conn *websocket.Conn, msgChan <-chan []byte, done chan<- struct{}, ctx context.Context) {
	defer close(done)

	for {
		select {
		case msg := <-msgChan:
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				if !websocket.IsUnexpectedCloseError(err) {
					log.Printf("[Write] Graceful close: %v", err)
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func readPump(conn *websocket.Conn, tm *TopicManager, peerID string, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err) {
					log.Printf("[Read] Unexpected close: %v", err)
				}
				return
			}

			tm.control <- topicOperation{
				opType:  "pub",
				topic:   peerID,
				message: message,
			}
		}
	}
}
