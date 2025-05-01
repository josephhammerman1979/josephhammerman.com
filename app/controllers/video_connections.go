package controllers

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type channelWrapper struct {
	ch   chan []byte
	once sync.Once
}

type TopicManager struct {
	topics   map[string][]*channelWrapper
	control  chan topicOperation
	shutdown chan struct{}
	mu       sync.Mutex
}

type topicOperation struct {
	opType  string
	topic   string
	ch      chan []byte
	message []byte
}

const (
	keepAliveInterval    = 30 * time.Minute
	messageBufferSize    = 100
	controlChannelBuffer = 200
	writeTimeout         = 5 * time.Second
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewTopicManager() *TopicManager {
	tm := &TopicManager{
		topics:   make(map[string][]*channelWrapper),
		control:  make(chan topicOperation, controlChannelBuffer),
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
	for _, cw := range tm.topics[op.topic] {
		if cw.ch == op.ch {
			return
		}
	}

	tm.topics[op.topic] = append(tm.topics[op.topic], &channelWrapper{ch: op.ch})
	log.Printf("[TopicManager] Subscribed to %s (subscribers: %d)", op.topic, len(tm.topics[op.topic]))
}

func (tm *TopicManager) handleUnsubscribe(op topicOperation) {
	subs := tm.topics[op.topic]
	for i, cw := range subs {
		if cw.ch == op.ch {
			tm.topics[op.topic] = append(subs[:i], subs[i+1:]...)
			cw.once.Do(func() {
				close(cw.ch)
			})
			log.Printf("[TopicManager] Unsubscribed from %s (remaining: %d)", op.topic, len(tm.topics[op.topic]))
			return
		}
	}
}

func (tm *TopicManager) handlePublish(op topicOperation) {
	if subs, exists := tm.topics[op.topic]; exists {
		for _, cw := range subs {
			select {
			case cw.ch <- op.message:
			default:
				log.Printf("[TopicManager] Channel full for %s", op.topic)
			}
		}
	}
}

func (tm *TopicManager) cleanupTopics() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for topic, subs := range tm.topics {
		if len(subs) == 0 {
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

		userID := r.URL.Query().Get("userID")
		peerID := r.URL.Query().Get("peerID")
		log.Printf("[Connection] %s connecting to %s", userID, peerID)

		ctx, cancel := context.WithTimeout(r.Context(), keepAliveInterval)
		defer func() {
			conn.Close()
			cancel()
		}()

		msgChan := make(chan []byte, messageBufferSize)

		tm.control <- topicOperation{opType: "sub", topic: userID, ch: msgChan}
		defer func() {
			tm.control <- topicOperation{opType: "unsub", topic: userID, ch: msgChan}
		}()

		// Write pump
		go func() {
			defer cancel()
			for {
				select {
				case msg := <-msgChan:
					conn.SetWriteDeadline(time.Now().Add(writeTimeout))
					if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
						if !websocket.IsUnexpectedCloseError(err) {
							log.Printf("[Write] Closed: %v", err)
						}
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		// Read pump
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
}
