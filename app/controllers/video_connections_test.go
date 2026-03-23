package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// ─── TopicManager unit tests ──────────────────────────────────────────────────

func TestTopicManagerSubscribePublish(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	ch := make(chan []byte, 10)
	tm.control <- topicOperation{opType: "sub", topic: "room:userA", ch: ch}

	// Allow the run() goroutine to process the subscription.
	time.Sleep(10 * time.Millisecond)

	msg := []byte(`{"hello":"world"}`)
	tm.control <- topicOperation{opType: "pub", topic: "room:userA", message: msg}

	select {
	case got := <-ch:
		if string(got) != string(msg) {
			t.Fatalf("got %s, want %s", got, msg)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for published message")
	}
}

func TestTopicManagerUnsubscribe(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	ch := make(chan []byte, 10)
	tm.control <- topicOperation{opType: "sub", topic: "room:userB", ch: ch}
	time.Sleep(10 * time.Millisecond)

	tm.control <- topicOperation{opType: "unsub", topic: "room:userB", ch: ch}
	time.Sleep(10 * time.Millisecond)

	// Publishing after unsubscribe should not deliver to the channel.
	tm.control <- topicOperation{opType: "pub", topic: "room:userB", message: []byte("nope")}

	// The unsub handler closes the channel; drain any close signal, then
	// verify no actual message bytes were delivered.
	select {
	case msg, ok := <-ch:
		if ok && len(msg) > 0 {
			t.Fatalf("unexpected message after unsubscribe: %s", msg)
		}
		// ok==false means the channel was closed (expected); ok==true with empty
		// payload is also fine — either way no real message was delivered.
	case <-time.After(200 * time.Millisecond):
		// also acceptable: nothing in the channel
	}
}

func TestPublishWithNoSubscribers(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	// Should not panic or block.
	tm.control <- topicOperation{opType: "pub", topic: "ghost:topic", message: []byte("x")}
	time.Sleep(50 * time.Millisecond)
}

// ─── Room membership tests ────────────────────────────────────────────────────

func TestAddRoomMember(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	ok, count := tm.addRoomMember("room1", "alice")
	if !ok || count != 1 {
		t.Fatalf("expected ok=true count=1, got ok=%v count=%d", ok, count)
	}

	ok, count = tm.addRoomMember("room1", "bob")
	if !ok || count != 2 {
		t.Fatalf("expected ok=true count=2, got ok=%v count=%d", ok, count)
	}
}

func TestRoomCapacityLimit(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	for i := 0; i < maxRoomParticipants; i++ {
		ok, _ := tm.addRoomMember("fullroom", string(rune('a'+i)))
		if !ok {
			t.Fatalf("unexpected failure at slot %d", i)
		}
	}

	ok, count := tm.addRoomMember("fullroom", "overflow")
	if ok {
		t.Fatalf("expected room full, got ok=true count=%d", count)
	}
}

func TestRemoveRoomMember(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	tm.addRoomMember("r", "u1")
	tm.addRoomMember("r", "u2")
	tm.removeRoomMember("r", "u1")

	members := tm.getRoomMembers("r", "")
	if len(members) != 1 || members[0] != "u2" {
		t.Fatalf("unexpected members after remove: %v", members)
	}
}

func TestGetRoomMembersExcludesRequester(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	tm.addRoomMember("r2", "alice")
	tm.addRoomMember("r2", "bob")
	tm.addRoomMember("r2", "carol")

	members := tm.getRoomMembers("r2", "bob")
	for _, m := range members {
		if m == "bob" {
			t.Fatal("getRoomMembers should exclude the requesting user")
		}
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d: %v", len(members), members)
	}
}

func TestGetRoomMembersEmptyRoom(t *testing.T) {
	tm := NewTopicManager()
	defer close(tm.shutdown)

	members := tm.getRoomMembers("nonexistent", "bob")
	if len(members) != 0 {
		t.Fatalf("expected empty slice, got %v", members)
	}
}

// ─── WebSocket integration tests ─────────────────────────────────────────────

// dialWS connects a test WebSocket client to the given test server URL with the
// provided room and user IDs.
func dialWS(t *testing.T, serverURL, roomID, userID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") +
		"/rooms/" + roomID + "/ws?userID=" + userID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return conn
}

// readJSON reads one text message from the WebSocket and unmarshals it.
func readJSON(t *testing.T, conn *websocket.Conn, deadline time.Duration) map[string]interface{} {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(deadline))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return msg
}

func newTestServer(t *testing.T) (*httptest.Server, *TopicManager) {
	t.Helper()
	tm := NewTopicManager()
	r := mux.NewRouter()
	r.Handle("/rooms/{roomID}/ws", VideoConnections(tm))
	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		close(tm.shutdown)
	})
	return srv, tm
}

func TestFirstUserReceivesNoPeersMessage(t *testing.T) {
	srv, _ := newTestServer(t)

	conn := dialWS(t, srv.URL, "roomAAA1", "useraaa1")
	defer conn.Close()

	// The first user in a room should not receive any message immediately.
	conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("first user should not receive a message on join")
	}
}

func TestSecondUserReceivesPeersMessage(t *testing.T) {
	srv, _ := newTestServer(t)

	connA := dialWS(t, srv.URL, "roomBBB2", "userbbb2")
	defer connA.Close()

	// Give the server time to register user A before B joins.
	time.Sleep(20 * time.Millisecond)

	connB := dialWS(t, srv.URL, "roomBBB2", "userbbb3")
	defer connB.Close()

	msg := readJSON(t, connB, 500*time.Millisecond)

	if msg["type"] != "peers" {
		t.Fatalf("expected type=peers, got %v", msg["type"])
	}
	peers, ok := msg["peers"].([]interface{})
	if !ok || len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %v", msg["peers"])
	}
	if peers[0].(string) != "userbbb2" {
		t.Fatalf("expected peer userbbb2, got %v", peers[0])
	}
}

func TestSignalingForwardsBetweenPeers(t *testing.T) {
	srv, _ := newTestServer(t)

	connA := dialWS(t, srv.URL, "roomCCC3", "userccc3")
	defer connA.Close()
	time.Sleep(20 * time.Millisecond)

	connB := dialWS(t, srv.URL, "roomCCC3", "userccc4")
	defer connB.Close()

	// Drain the "peers" message B receives.
	_ = readJSON(t, connB, 500*time.Millisecond)

	// B sends an offer to A.
	offer := map[string]interface{}{
		"type":   "offer",
		"from":   "userccc4",
		"to":     "userccc3",
		"roomID": "roomCCC3",
		"sdp":    "v=0\r\no=fake 0 0 IN IP4 127.0.0.1\r\n",
	}
	raw, _ := json.Marshal(offer)
	if err := connB.WriteMessage(websocket.TextMessage, raw); err != nil {
		t.Fatalf("B write: %v", err)
	}

	// A should receive the offer.
	msg := readJSON(t, connA, 500*time.Millisecond)
	if msg["type"] != "offer" {
		t.Fatalf("expected offer, got %v", msg["type"])
	}
	if msg["from"] != "userccc4" {
		t.Fatalf("expected from=userccc4, got %v", msg["from"])
	}
}

func TestInvalidIDsRejected(t *testing.T) {
	srv, _ := newTestServer(t)

	// Too short userID
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/rooms/validroom1/ws?userID=ab"
	_, resp, _ := websocket.DefaultDialer.Dial(url, nil)
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for short userID, got %v", resp)
	}
}

func TestRoomFullRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	roomID := "fullroomX1"

	conns := make([]*websocket.Conn, maxRoomParticipants)
	for i := 0; i < maxRoomParticipants; i++ {
		userID := strings.Repeat(string(rune('a'+i)), 8)
		conns[i] = dialWS(t, srv.URL, roomID, userID)
		time.Sleep(5 * time.Millisecond)
	}
	defer func() {
		for _, c := range conns {
			if c != nil {
				c.Close()
			}
		}
	}()

	// One more should be rejected.
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/rooms/" + roomID + "/ws?userID=overflowXX"
	_, resp, _ := websocket.DefaultDialer.Dial(url, nil)
	if resp == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for full room, got %v", resp)
	}
}
