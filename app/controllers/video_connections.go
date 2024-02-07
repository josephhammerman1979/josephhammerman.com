package controllers

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "nhooyr.io/websocket"
    "sort"
    "strings"
)

var peerToWSMap = make(map[string]map[string]interface{})

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
    
    //peerToWSMap[topicName] = make(map[string]interface{})
    peerToWSMap[topicName] = map[string]interface{}{
        userID: nil,
        peerID: nil,
    }

    ctx := context.Background()

    cctx, cancelFunc := context.WithCancel(ctx)

    go wsLoop(ctx, cancelFunc, ws, peerToWSMap, userID, topicName)
    pubSubLoop(cctx, ctx, ws, peerToWSMap, userID, peerID, topicName)
}

func wsLoop(ctx context.Context, cancelFunc context.CancelFunc, ws *websocket.Conn, peerToWSMap map[string]map[string]interface{}, userID string, topicName string) {
    log.Printf("Starting wsLoop for %s...", userID)
    for {
        if _, message, err := ws.Read(ctx); err != nil {
            // could check for 'close' here and tell peer we have closed
            log.Printf("Error reading message %s", err)
            break
        } else {
            log.Printf("Received message to websocket.")
            peerToWSMap[topicName][userID] = message
                return
        }
    }
    cancelFunc()
    log.Printf("Shutting down wsLoop for %s...", userID)
}

func pubSubLoop(cctx, ctx context.Context, ws *websocket.Conn, peerToWSMap map[string]map[string]interface{}, peerID string, userID string, topicName string) {
    log.Printf("Starting pubSubLoop for %s...", userID)
    // subscriptionName := fmt.Sprintf("%s-%s", userID, topic.ID())
    // sub := pubSub.Subscription(subscriptionName)
    // if exists, err := sub.Exists(ctx); err != nil {
    //     log.Printf("Error checking if sub exists: %s", err)
    //     return
    // } else if !exists {
    //    log.Printf("Creating subscription: %s", subscriptionName)
    //    if _, err = pubSub.CreateSubscription(
    //        context.Background(),
    //        subscriptionName,
    //        pubsub.SubscriptionConfig{
    //            Topic:                 topic,
    //            EnableMessageOrdering: true,
    //        },
    //    ); err != nil {
    //        log.Printf("Error creating subscription: %s", err)
    //        return
    //    }
    //}
    //if err := sub.Receive(cctx, func(c context.Context, m *pubsub.Message) {
    
    //for topic := range peerToWSMap {
    //if topicName == peerToWSMap[topicName] 
    for peer := range peerToWSMap[topicName] {
       if _, message, err := ws.Read(ctx); err != nil {
         if peer != peerID {
             log.Println("skipping message from self")
             return
         }
         log.Printf("Received message to publish")
         if err := ws.Write(ctx, websocket.MessageText, message); err != nil {
             log.Printf("Error writing message to %s: %s", userID, err)
             return
         }
         log.Printf("Shutting down pubSubLoop for %s...", userID)
        }
    }
    //}
}

func closeWS(ws *websocket.Conn) {
    // can check if already closed here
    if err := ws.Close(websocket.StatusNormalClosure, ""); err != nil {
        log.Printf("Error closing: %s", err)
    }
}
