package controllers

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "nhooyr.io/websocket"
    "sort"
    "strings"
    "sync"
    "time"
)

// Simulating a local PubSub system
var (
    localPubSub sync.Map // A thread-safe map to store topics and their messages
)

// Message represents a message that can be published to a topic
type Message struct {
    Data       []byte
    Attributes map[string]string
}

// TopicMessages stores messages for a topic
type TopicMessages struct {
    messages []Message
    mux      sync.Mutex
}

// Add a constant for the keep-alive interval
const keepAliveInterval = 30 * time.Second

func VideoConnections(w http.ResponseWriter, r *http.Request) {
    ws, err := websocket.Accept(w, r, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer closeWS(ws)
    userID := strings.ToLower(r.URL.Query().Get("userID"))
    peerID := strings.ToLower(r.URL.Query().Get("peerID"))

    peers := []string{userID, peerID}
    sort.Strings(peers)
    topicName := fmt.Sprintf("video-%s-%s", peers[0], peers[1])
    
    ctx := context.Background()

    cctx, cancelFunc := context.WithCancel(ctx)

    go wsLoop(ctx, cancelFunc, ws, topicName, userID)
    pubSubLoop(cctx, ctx, ws, topicName, userID)
}

func wsLoop(ctx context.Context, cancelFunc context.CancelFunc, ws *websocket.Conn, topicName string, userID string) {
    log.Printf("Starting wsLoop for %s...", userID)

    defer func() {
        cancelFunc()
        log.Printf("Shutting down wsLoop for %s...", userID)
    }()

    keepAliveTicker := time.NewTicker(keepAliveInterval)
    defer keepAliveTicker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-keepAliveTicker.C:
            // Send a keep-alive message to prevent the WebSocket connection from timing out
            if err := ws.Write(ctx, websocket.MessageText, []byte("keep-alive")); err != nil {
                log.Printf("Error sending keep-alive message: %s", err)
                return
            }
       default:
           if _, message, err := ws.Read(ctx); err != nil {
                // could check for 'close' here and tell peer we have closed
                log.Printf("Error reading message %s", err)
                break
            } else {
                log.Printf("Received message to websocket.")
                msg := Message{
                    Data:       message,
                    Attributes: map[string]string{"sender": userID},
                }
                publishToLocalTopic(topicName, msg)
            }
        }
    }
    cancelFunc()
    log.Printf("Shutting down wsLoop for %s...", userID)
}

func pubSubLoop(cctx, ctx context.Context, ws *websocket.Conn, topicName string, userID string) {
    log.Printf("Starting pubSubLoop for %s...", userID)
    for {
        select {
        case <-cctx.Done():
            log.Printf("Shutting down pubSubLoop for %s...", userID)
            return
        default:
            messages := getMessagesFromLocalTopic(topicName)
            for _, msg := range messages {
                if msg.Attributes["sender"] == userID {
                    log.Println("skipping message from self")
                    continue
                }
                log.Printf("Received message to pubSub: ")
                if err := ws.Write(ctx, websocket.MessageText, msg.Data); err != nil {
                    log.Printf("Error writing message to %s: %s", userID, err)
                    return
                }
            }
        }
    }
}

func publishToLocalTopic(topicName string, msg Message) {
    value, _ := localPubSub.LoadOrStore(topicName, &TopicMessages{})
    topicMessages := value.(*TopicMessages)
    topicMessages.mux.Lock()
    defer topicMessages.mux.Unlock()
    topicMessages.messages = append(topicMessages.messages, msg)
}

func getMessagesFromLocalTopic(topicName string) []Message {
    value, ok := localPubSub.Load(topicName)
    if !ok {
        return nil
    }
    topicMessages := value.(*TopicMessages)
    topicMessages.mux.Lock()
    defer topicMessages.mux.Unlock()
    messages := topicMessages.messages
    topicMessages.messages = nil // Clear messages after reading
    return messages
}

func closeWS(ws *websocket.Conn) {
    // can check if already closed here
    if err := ws.Close(websocket.StatusNormalClosure, ""); err != nil {
        log.Printf("Error closing: %s", err)
    }
}
