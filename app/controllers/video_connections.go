package controllers

import (
    "context"
    "fmt"
    "encoding/json"
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

var keepAliveMessage = map[string]interface{}{
    "type":      "keep-alive",
    "timestamp": time.Now().Unix(), // Add any additional data if needed
}

func VideoConnections(w http.ResponseWriter, r *http.Request) {
    ws, err := websocket.Accept(w, r, nil)
    if err != nil {
        log.Fatal(err)
    }

    userID := strings.ToLower(r.URL.Query().Get("userID"))
    peerID := strings.ToLower(r.URL.Query().Get("peerID"))

    peers := []string{userID, peerID}
    sort.Strings(peers)
    topicName := fmt.Sprintf("video-%s-%s", peers[0], peers[1])
    defer closeWS(ws, topicName)

    ctx := context.Background()

    cctx, cancelFunc := context.WithCancel(ctx)

    go wsLoop(ctx, cancelFunc, ws, topicName, userID)
    go pubSubLoop(cctx, ctx, ws, topicName, userID)
}

func wsLoop(ctx context.Context, cancelFunc context.CancelFunc, ws *websocket.Conn, topicName string, userID string) {
    log.Printf("Starting wsLoop for %s...", userID)

    defer func() {
        cancelFunc()
        log.Printf("Shutting down wsLoop for %s...", userID)
    }()

    keepAliveTicker := time.NewTicker(keepAliveInterval)
    jsonKeepAliveMessage, err := json.Marshal(keepAliveMessage)
    if err != nil {
        log.Printf("Error marshaling JSON: %v", err)
        return
    }
    defer keepAliveTicker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-keepAliveTicker.C:
            // Send a keep-alive message to prevent the WebSocket connection from timing out
            if err := ws.Write(ctx, websocket.MessageText, []byte(jsonKeepAliveMessage)); err != nil {
                log.Printf("Error sending keep-alive message: %v", err)
                if websocket.CloseStatus(err) == websocket.StatusAbnormalClosure {
                    log.Printf("WebSocket connection closed: %s", err)
                    return
                }
                return
            }
        default:
            if _, message, err := ws.Read(ctx); err != nil {
                if websocket.CloseStatus(err) == websocket.StatusAbnormalClosure {
                    log.Printf("WebSocket connection closed: %s", err)
                    //removeTopicFromLocalPubSub(topicName)
                    return
                } else if websocket.CloseStatus(err) == websocket.StatusNoStatusRcvd {
                    log.Printf("WebSocket connection closed: %s", err)
                    //removeTopicFromLocalPubSub(topicName)
                    return
                }
                log.Printf("Error reading message: %s", err)
                //removeTopicFromLocalPubSub(topicName)
                return
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
    log.Printf("Exiting reading loop for %s", userID)
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
    log.Printf("Exiting publishing loop for %s", userID)
}

func publishToLocalTopic(topicName string, msg Message) {
    log.Printf("Publishing to %s", topicName)
    value, _ := localPubSub.LoadOrStore(topicName, &TopicMessages{})
    topicMessages := value.(*TopicMessages)
    topicMessages.mux.Lock()
    defer topicMessages.mux.Unlock()
    topicMessages.messages = append(topicMessages.messages, msg)
}

func getMessagesFromLocalTopic(topicName string) []Message {
    value, ok := localPubSub.Load(topicName)
    if !ok {
        //log.Printf("No topic for %s", topicName)
        return nil
    }
    topicMessages := value.(*TopicMessages)
    topicMessages.mux.Lock()
    defer topicMessages.mux.Unlock()
    messages := topicMessages.messages
    if len(messages) > 0 {
        log.Printf("Dequeued messages in %s", topicName)
    }
    topicMessages.messages = nil // Clear messages after reading
    return messages
}

func removeTopicFromLocalPubSub(topicName string) {
    log.Printf("Deleted topic %s", topicName)
    localPubSub.Delete(topicName)
}

func closeWS(ws *websocket.Conn, topicName string) {
    // can check if already closed here
    if err := ws.Close(websocket.StatusNormalClosure, ""); err != nil {
        log.Printf("Error closing: %s", err)
        //removeTopicFromLocalPubSub(topicName)
    }
}
