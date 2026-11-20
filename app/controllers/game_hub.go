package controllers

import (
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// GameHub runs the dice games for every room.  Clients send game messages
// addressed to "server" over the room WebSocket; the hub applies them to the
// room's Table and pushes each member a view built for them.
type GameHub struct {
	tm     *TopicManager
	mu     sync.Mutex
	rng    *rand.Rand
	tables map[string]*games.Table           // roomID -> current game
	gameNo map[string]int                    // roomID -> games started
	info   map[string]map[string]*memberInfo // roomID -> clientID -> info
}

type memberInfo struct {
	Dice   bool      // has the dice game loaded (announced via hello)
	LeftAt time.Time // when their last connection closed; zero if connected
}

func newGameHub(tm *TopicManager) *GameHub {
	return &GameHub{
		tm:     tm,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		tables: map[string]*games.Table{},
		gameNo: map[string]int{},
		info:   map[string]map[string]*memberInfo{},
	}
}

// catalogMessage lists the available games; sent to every client on join.
type catalogMessage struct {
	Type   string       `json:"type"` // "game_catalog"
	RoomID string       `json:"roomID"`
	Games  []*games.Def `json:"games"`
}

type gameErrorMessage struct {
	Type    string `json:"type"` // "game_error"
	RoomID  string `json:"roomID"`
	Message string `json:"message"`
}

type noGameMessage struct {
	Type   string `json:"type"` // "game_none"
	RoomID string `json:"roomID"`
}

func (h *GameHub) member(roomID, clientID string) *memberInfo {
	room := h.info[roomID]
	if room == nil {
		room = map[string]*memberInfo{}
		h.info[roomID] = room
	}
	m := room[clientID]
	if m == nil {
		m = &memberInfo{}
		room[clientID] = m
	}
	return m
}

func (h *GameHub) presence(roomID string) games.Presence {
	now := time.Now()
	return func(clientID string) (bool, time.Duration) {
		if h.tm.isConnected(roomID, clientID) {
			return true, 0
		}
		m := h.info[roomID][clientID]
		if m == nil || m.LeftAt.IsZero() {
			return false, games.RemoveGrace // never seen: treat as long gone
		}
		return false, now.Sub(m.LeftAt)
	}
}

func (h *GameHub) send(roomID, clientID string, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("[GameHub] marshal: %v", err)
		return
	}
	h.tm.control <- topicOperation{opType: "pub", topic: roomID + ":" + clientID, message: data}
}

// broadcast sends every connected member their view of the room's table.
// Caller holds h.mu.
func (h *GameHub) broadcast(roomID string) {
	t := h.tables[roomID]
	if t == nil {
		return
	}
	pres := h.presence(roomID)
	now := time.Now()
	for _, id := range h.tm.getRoomMembers(roomID, "") {
		h.send(roomID, id, t.ViewFor(roomID, id, pres, now))
	}
}

// joined is called when a client connects (including reconnects).
func (h *GameHub) joined(roomID, clientID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.member(roomID, clientID).LeftAt = time.Time{}
	h.send(roomID, clientID, catalogMessage{Type: "game_catalog", RoomID: roomID, Games: games.Catalog()})
	t := h.tables[roomID]
	if t == nil {
		h.send(roomID, clientID, noGameMessage{Type: "game_none", RoomID: roomID})
		return
	}
	if t.Seat(clientID) >= 0 {
		h.broadcast(roomID) // their seat is back online for everyone
		return
	}
	h.send(roomID, clientID, t.ViewFor(roomID, clientID, h.presence(roomID), time.Now()))
}

// left is called when a client's last connection closes.  roomEmpty means
// nobody is left, in which case the room's game is discarded.
func (h *GameHub) left(roomID, clientID string, roomEmpty bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if roomEmpty {
		delete(h.tables, roomID)
		delete(h.gameNo, roomID)
		delete(h.info, roomID)
		return
	}
	h.member(roomID, clientID).LeftAt = time.Now()
	if t := h.tables[roomID]; t != nil && t.Seat(clientID) >= 0 {
		h.broadcast(roomID)
	}
}

// hello records what a client announced about itself.
func (h *GameHub) hello(roomID, clientID string, dice bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.member(roomID, clientID).Dice = dice
}

// handle processes a game message a client addressed to "server".
func (h *GameHub) handle(roomID, clientID string, sig *signalMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	fail := func(msg string) {
		h.send(roomID, clientID, gameErrorMessage{Type: "game_error", RoomID: roomID, Message: msg})
	}
	now := time.Now()
	t := h.tables[roomID]

	switch sig.Type {
	case "game_new":
		def := games.Lookup(sig.GameID)
		if def == nil {
			fail("Unknown game.")
			return
		}
		if t != nil && !t.Over() {
			if seat := t.Seat(clientID); seat < 0 || t.Removed[seat] {
				fail("A game is in progress — only its players can restart it.")
				return
			}
		}
		h.member(roomID, clientID).Dice = true
		roster := []string{}
		for _, id := range h.tm.membersBySlot(roomID) {
			if m := h.info[roomID][id]; m != nil && m.Dice {
				roster = append(roster, id)
			}
		}
		h.gameNo[roomID]++
		nt, err := games.NewTable(def, roster, clientID, sig.Options, h.gameNo[roomID], h.rng, now)
		if err != nil {
			h.gameNo[roomID]--
			fail(err.Error())
			return
		}
		nt.Env.Log(games.LogInfo, "%s started %s.", games.P(nt.Seat(clientID)), def.Name)
		h.tables[roomID] = nt
		h.broadcast(roomID)

	case "game_action":
		if t == nil || sig.Action == nil {
			fail("No game is running.")
			return
		}
		if err := t.Act(clientID, *sig.Action, now); err != nil {
			fail(err.Error())
			return
		}
		h.broadcast(roomID)

	case "game_remove":
		if t == nil || sig.Seat == nil {
			fail("No game is running.")
			return
		}
		if err := t.CanRemove(clientID, *sig.Seat, h.presence(roomID), now); err != nil {
			fail(err.Error())
			return
		}
		t.Remove(*sig.Seat, now)
		h.broadcast(roomID)

	default:
		fail("Unknown game message.")
	}
}
