package controllers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

type channelWrapper struct {
	ch   chan []byte
	once sync.Once
}

type TopicManager struct {
	topics map[string][]*channelWrapper
	// roomID -> set of userIDs
	rooms map[string]map[string]struct{}
	// roomID -> ordered list of clientIDs; index = player slot.
	// Slot assignments are append-only for the lifetime of the room: a client
	// that disconnects and rejoins with the same clientID gets the same slot.
	roomSlots map[string][]string

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
	maxRoomParticipants  = 10
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func NewTopicManager() *TopicManager {
	tm := &TopicManager{
		topics:    make(map[string][]*channelWrapper),
		rooms:     make(map[string]map[string]struct{}),
		roomSlots: make(map[string][]string),
		control:   make(chan topicOperation, controlChannelBuffer),
		shutdown:  make(chan struct{}),
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
			break
		}
	}
	if len(tm.topics[op.topic]) == 0 {
		delete(tm.topics, op.topic)
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
	// rooms map is kept small by VideoConnections removing members on disconnect
}

// helpers for room membership

var idRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]{6,64}$`)

func validID(id string) bool {
	return idRegexp.MatchString(id)
}

func (tm *TopicManager) addRoomMember(roomID, userID string) (ok bool, count int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	members, exists := tm.rooms[roomID]
	if !exists {
		members = make(map[string]struct{})
		tm.rooms[roomID] = members
	}
	if len(members) >= maxRoomParticipants {
		return false, len(members)
	}
	members[userID] = struct{}{}
	return true, len(members)
}

func (tm *TopicManager) removeRoomMember(roomID, userID string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if members, exists := tm.rooms[roomID]; exists {
		delete(members, userID)
		if len(members) == 0 {
			delete(tm.rooms, roomID)
		}
	}
}

// assignSlot returns the slot index for clientID in the given room, allocating
// a new slot if this is the first time the clientID has been seen.  Slots are
// preserved across disconnect/reconnect so that a refresh restores the player
// to their original slot.  Also returns a snapshot of the full slot map for
// the room.
func (tm *TopicManager) assignSlot(roomID, clientID string) (slot int, slots map[string]int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	list := tm.roomSlots[roomID]
	slot = -1
	for i, id := range list {
		if id == clientID {
			slot = i
			break
		}
	}
	if slot == -1 {
		slot = len(list)
		list = append(list, clientID)
		tm.roomSlots[roomID] = list
	}
	slots = make(map[string]int, len(list))
	for i, id := range list {
		slots[id] = i
	}
	return slot, slots
}

// getRoomMembers returns all member IDs in a room excluding the given userID.
func (tm *TopicManager) getRoomMembers(roomID, excludeID string) []string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	members := tm.rooms[roomID]
	result := make([]string, 0, len(members))
	for id := range members {
		if id != excludeID {
			result = append(result, id)
		}
	}
	return result
}

// peersMessage is sent by the server to a newly-joined user to tell them
// which peers are already in the room (so they can initiate WebRTC offers)
// and the room's slot assignments (so the dice game can use authoritative
// player numbers instead of racing on a sorted client-ID list).
type peersMessage struct {
	Type   string         `json:"type"`
	RoomID string         `json:"roomID"`
	Peers  []string       `json:"peers"`
	Slots  map[string]int `json:"slots"`
	MySlot int            `json:"mySlot"`
}

// playerJoinedMessage is broadcast to existing room members when a new client
// connects, carrying the new client's server-assigned slot.
type playerJoinedMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomID"`
	PeerID string `json:"peerID"`
	Slot   int    `json:"slot"`
}

// playerLeftMessage is broadcast to remaining room members when a client
// disconnects.  The slot mapping itself is preserved server-side so the
// client can rejoin with the same player number.
type playerLeftMessage struct {
	Type   string `json:"type"`
	RoomID string `json:"roomID"`
	PeerID string `json:"peerID"`
}

// signaling message format

type signalMessage struct {
	Type   string          `json:"type"`   // "offer","answer","candidate","game_start","game_event"…
	From   string          `json:"from"`   // sender userID
	To     string          `json:"to"`     // target userID or "room" (broadcast)
	RoomID string          `json:"roomID"` // room scope
	SDP    json.RawMessage `json:"sdp,omitempty"`
	ICE    json.RawMessage `json:"ice,omitempty"`
	// keep RawMessage so we just relay; clients parse SDP/ICE/Event payloads
	Event json.RawMessage `json:"event,omitempty"`
}

func VideoConnections(tm *TopicManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		roomID := vars["roomID"]
		userID := r.URL.Query().Get("userID")

		if !validID(roomID) || !validID(userID) {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		if ok, count := tm.addRoomMember(roomID, userID); !ok {
			log.Printf("[Connection] room %s full (%d users)", roomID, count)
			http.Error(w, "room full", http.StatusConflict)
			return
		}
		defer func() {
			tm.removeRoomMember(roomID, userID)
			// Notify remaining peers so they can tear down stale WebRTC
			// connections and dice-game state without waiting for an ICE timeout.
			remaining := tm.getRoomMembers(roomID, userID)
			if len(remaining) == 0 {
				return
			}
			data, err := json.Marshal(playerLeftMessage{
				Type:   "player_left",
				RoomID: roomID,
				PeerID: userID,
			})
			if err != nil {
				return
			}
			for _, memberID := range remaining {
				tm.control <- topicOperation{
					opType:  "pub",
					topic:   roomID + ":" + memberID,
					message: data,
				}
			}
		}()

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed: %v", err)
			return
		}

		log.Printf("[Connection] %s joined room %s", userID, roomID)

		ctx, cancel := context.WithTimeout(r.Context(), keepAliveInterval)
		defer func() {
			conn.Close()
			cancel()
		}()

		msgChan := make(chan []byte, messageBufferSize)
		topicSelf := roomID + ":" + userID

		tm.control <- topicOperation{opType: "sub", topic: topicSelf, ch: msgChan}
		defer func() {
			tm.control <- topicOperation{opType: "unsub", topic: topicSelf, ch: msgChan}
		}()

		// Assign (or restore) this client's player slot and snapshot the room's slot map.
		mySlot, slots := tm.assignSlot(roomID, userID)
		log.Printf("[Connection] %s in room %s assigned slot %d", userID, roomID, mySlot)

		// Tell the new user about peers already in the room (for WebRTC offers)
		// and the full slot map (for dice-game player ordering).
		existing := tm.getRoomMembers(roomID, userID)
		if data, err := json.Marshal(peersMessage{
			Type:   "peers",
			RoomID: roomID,
			Peers:  existing,
			Slots:  slots,
			MySlot: mySlot,
		}); err == nil {
			msgChan <- data
		}

		// Tell existing peers that this client joined and what slot they got.
		if joined, err := json.Marshal(playerJoinedMessage{
			Type:   "player_joined",
			RoomID: roomID,
			PeerID: userID,
			Slot:   mySlot,
		}); err == nil {
			for _, memberID := range existing {
				tm.control <- topicOperation{
					opType:  "pub",
					topic:   roomID + ":" + memberID,
					message: joined,
				}
			}
		}

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

				var sig signalMessage
				if err := json.Unmarshal(message, &sig); err != nil {
					log.Printf("[Read] invalid JSON: %v", err)
					continue
				}

				// Basic validation: enforce room + sender ID.
				// sig.To may be a specific userID or "room" (broadcast to all members).
				if sig.RoomID != roomID || !validID(sig.From) {
					continue
				}
				if sig.To != "room" && !validID(sig.To) {
					continue
				}

				if sig.To == "room" {
					// Broadcast to every other member of this room.
					for _, memberID := range tm.getRoomMembers(roomID, sig.From) {
						tm.control <- topicOperation{
							opType:  "pub",
							topic:   roomID + ":" + memberID,
							message: message,
						}
					}
				} else {
					// Forward to a specific peer.
					tm.control <- topicOperation{
						opType:  "pub",
						topic:   roomID + ":" + sig.To,
						message: message,
					}
				}
			}
		}
	}
}
