package controllers

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "nhooyr.io/websocket"
    "sort"
    "strings"
    //"time"
)

type rTCMessage struct {
	sender    string
        data      []byte
}

var topicMessage = make(map[string][]rTCMessage)

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

    _, ok := topicMessage[topicName]

    if !ok {
        log.Printf("Creating topic %s...", topicName)
        var messages []rTCMessage
        topicMessage[topicName] = messages
    }

    //cctx, cancelFunc := context.WithCancel(ctx)

    wsLoop(ctx, ws, topicMessage, userID, topicName)
    pubSubLoop(ctx, ws, topicMessage, userID, peerID, topicName)
}

func wsLoop(ctx context.Context, ws *websocket.Conn, topicMessage map[string][]rTCMessage, userID string, topicName string) {
    log.Printf("Starting wsLoop for %s...", userID)
    for {
        if _, message, err := ws.Read(ctx); err != nil {
            // could check for 'close' here and tell peer we have closed
            log.Printf("Error reading message %s", err)
            break
        } else {
            log.Printf("Received message to websocket.")
            newMessage := rTCMessage{
                sender: userID,
                data: message,
            }
            topicMessage[topicName] = append(topicMessage[topicName], newMessage)

            return
        }
    }
    // cancelFunc()
    log.Printf("Shutting down wsLoop for %s...", userID)
}

func pubSubLoop(ctx context.Context, ws *websocket.Conn, topicMessage map[string][]rTCMessage, userID string, peerID string, topicName string) {
    log.Printf("Starting pubSubLoop for %s, topic %s...", userID, topicName)

    _, ok := topicMessage[topicName]

    if !ok {
        log.Printf("Topic not exist in map %s, erroring", topicName)
        return
    }    

    for {
        for message := range topicMessage[topicName] {
            if topicMessage[topicName][message].sender == userID {
                log.Println("skipping message from self")
                return
            } else {
                if err := ws.Write(ctx, websocket.MessageText, topicMessage[topicName][message].data); err != nil {
                    log.Printf("Error writing message %s", err)
                    log.Printf("Shutting down pubSubLoop for %s...", userID)
                    break
                } // else {
                  //  log.Printf("Received message to publish")
                  //  continue
                // }
            } 
        }
    }
}

func closeWS(ws *websocket.Conn) {
    // can check if already closed here
    if err := ws.Close(websocket.StatusNormalClosure, ""); err != nil {
        log.Printf("Error closing: %s", err)
    }
}
