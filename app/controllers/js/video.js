// video.js

const videoRoot = document.getElementById("video-root");
const roomID = videoRoot.dataset.roomId;

function randomID() {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

// Stable client ID, persisted across reloads so the server can restore the
// player's slot on refresh/rejoin.
const CLIENT_ID_STORAGE_KEY = "pig.clientID";
function getOrCreateClientID() {
  let id = null;
  try { id = window.localStorage.getItem(CLIENT_ID_STORAGE_KEY); } catch (_) {}
  if (!id || !/^[a-f0-9]{16}$/.test(id)) {
    id = randomID();
    try { window.localStorage.setItem(CLIENT_ID_STORAGE_KEY, id); } catch (_) {}
  }
  return id;
}
const myID = getOrCreateClientID();
const peers = Object.create(null);
const pendingPeers = new Set();

// Server-assigned slot info, populated from the "peers" and "player_joined"
// messages. Read by dice_game.js to determine player ordering.
let mySlot = -1;
const playerSlots = Object.create(null);  // clientID -> slot index
const wsScheme = window.location.protocol === "https:" ? "wss://" : "ws://";
const wsURL = wsScheme + window.location.host + "/rooms/" + encodeURIComponent(roomID) + "/ws?userID=" + encodeURIComponent(myID);

let ws = null;
let localStream = null;

// Auto-reconnect state. The server preserves slot assignments across
// disconnect, so a reconnect restores the same player number.
const RECONNECT_MIN_MS = 500;
const RECONNECT_MAX_MS = 10000;
let reconnectAttempts = 0;
let reconnectTimer = null;
let manualClose = false;

// Copy invite link to clipboard
document.getElementById("copy-link-btn").addEventListener("click", () => {
  navigator.clipboard.writeText(window.location.href).then(() => {
    const btn = document.getElementById("copy-link-btn");
    const original = btn.textContent;
    btn.textContent = "Copied!";
    setTimeout(() => { btn.textContent = original; }, 2000);
  });
});

// Resize all videos based on how many are present
function updateLayout() {
  const grid = document.getElementById("video-grid");
  const n = grid.querySelectorAll("video").length;
  const cols = Math.max(1, Math.ceil(Math.sqrt(n)));
  grid.style.gridTemplateColumns = `repeat(${cols}, 1fr)`;
}

function connectWS() {
  ws = new WebSocket(wsURL);
  ws.onopen = onWSOpen;
  ws.onmessage = onWSMessage;
  ws.onclose = onWSClose;
  ws.onerror = (err) => console.error("[WS] error:", err);
}

function scheduleReconnect() {
  if (manualClose || reconnectTimer) return;
  const delay = Math.min(RECONNECT_MAX_MS, RECONNECT_MIN_MS * Math.pow(2, reconnectAttempts));
  reconnectAttempts++;
  console.log(`[WS] reconnect in ${delay}ms (attempt ${reconnectAttempts})`);
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connectWS();
  }, delay);
}

function onWSOpen() {
  console.log("WS connected, myID =", myID, "roomID =", roomID);
  reconnectAttempts = 0;
}

function onWSMessage(evt) {
  const msg = JSON.parse(evt.data);
  if (!msg || msg.roomID !== roomID) return;

  // Server-initiated notification of existing room members + slot assignments.
  if (msg.type === "peers") {
    if (typeof msg.mySlot === "number") mySlot = msg.mySlot;
    if (msg.slots && typeof msg.slots === "object") {
      Object.keys(msg.slots).forEach((id) => { playerSlots[id] = msg.slots[id]; });
    }
    msg.peers.forEach((peerID) => {
      if (peerID === myID || peers[peerID]) return;
      if (localStream) {
        const pc = createPeerConnection(peerID);
        peers[peerID] = pc;
      } else {
        pendingPeers.add(peerID);
      }
    });
    if (typeof onSlotsUpdated === "function") onSlotsUpdated();
    return;
  }

  // Server tells us a new player joined and what slot they got. If we still
  // have a stale RTCPeerConnection for this peer (e.g. they refreshed before
  // our ICE timeout fired), tear it down so the fresh offer creates a new one.
  if (msg.type === "player_joined") {
    if (msg.peerID && typeof msg.slot === "number") {
      playerSlots[msg.peerID] = msg.slot;
      if (peers[msg.peerID]) {
        peers[msg.peerID].close();
        delete peers[msg.peerID];
        removePeerVideo(msg.peerID);
      }
      if (typeof onSlotsUpdated === "function") onSlotsUpdated();
      if (typeof onPlayerJoined === "function") onPlayerJoined(msg.peerID);
    }
    return;
  }

  // Server tells us a peer disconnected. Tear down the RTC connection and
  // notify the dice game so it can end any in-progress session cleanly.
  if (msg.type === "player_left") {
    if (msg.peerID) {
      if (peers[msg.peerID]) {
        peers[msg.peerID].close();
        delete peers[msg.peerID];
      }
      removePeerVideo(msg.peerID);
      if (typeof onPlayerLeft === "function") onPlayerLeft(msg.peerID);
    }
    return;
  }

  // Dice game coordination messages (broadcast or direct).
  if (msg.type === "game_start" || msg.type === "game_event" || msg.type === "player_kick") {
    if (typeof handleDiceGameMessage === "function") {
      handleDiceGameMessage(msg);
    }
    return;
  }

  if (msg.to !== myID) return;
  const from = msg.from;
  if (!from || from === myID) return;

  let pc = peers[from];
  if (!pc) {
    pc = createPeerConnection(from);
    peers[from] = pc;
  }

  switch (msg.type) {
    case "offer":
      pc.setRemoteDescription(new RTCSessionDescription({ type: "offer", sdp: msg.sdp }))
        .then(() => pc.createAnswer())
        .then((answer) => pc.setLocalDescription(answer))
        .then(() => {
          sendSignal({ type: "answer", from: myID, to: from, roomID, sdp: pc.localDescription.sdp });
        })
        .catch((err) => console.error("Error handling offer", err));
      break;
    case "answer":
      pc.setRemoteDescription(new RTCSessionDescription({ type: "answer", sdp: msg.sdp }))
        .catch((err) => console.error("Error setting remote answer", err));
      break;
    case "candidate":
      if (msg.ice) {
        pc.addIceCandidate(new RTCIceCandidate(msg.ice))
          .catch((err) => console.error("Error adding ICE candidate", err));
      }
      break;
  }
}

function onWSClose() {
  console.log("WS closed");
  // Drop all peer state — a fresh "peers" message on reconnect will rebuild
  // the WebRTC mesh from scratch.
  Object.keys(peers).forEach((peerID) => {
    peers[peerID].close();
    delete peers[peerID];
    removePeerVideo(peerID);
  });
  pendingPeers.clear();
  mySlot = -1;
  Object.keys(playerSlots).forEach((id) => { delete playerSlots[id]; });
  if (typeof onWSDisconnected === "function") onWSDisconnected();
  scheduleReconnect();
}

window.addEventListener("beforeunload", () => {
  manualClose = true;
  if (ws) ws.close();
});

function sendSignal(payload) {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(payload));
  }
}

function removePeerVideo(peerID) {
  const video = document.querySelector(`video[data-peer-id="${peerID}"]`);
  if (video) {
    video.srcObject = null;
    video.remove();
    updateLayout();
  }
}

function createPeerConnection(peerID) {
  const pc = new RTCPeerConnection({
    iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
  });

  if (localStream) {
    localStream.getTracks().forEach((track) => pc.addTrack(track, localStream));
  }

  pc.onicecandidate = (evt) => {
    if (evt.candidate) {
      sendSignal({ type: "candidate", from: myID, to: peerID, roomID, ice: evt.candidate });
    }
  };

  pc.ontrack = (evt) => {
    const stream = evt.streams[0];
    if (!stream) return;
    let video = document.querySelector(`video[data-peer-id="${peerID}"]`);
    if (!video) {
      video = document.createElement("video");
      video.autoplay = true;
      video.playsInline = true;
      video.dataset.peerId = peerID;
      document.getElementById("video-grid").appendChild(video);
      updateLayout();
    }
    if (video.srcObject !== stream) {
      video.srcObject = stream;
    }
  };

  pc.onconnectionstatechange = () => {
    if (["disconnected", "failed", "closed"].includes(pc.connectionState)) {
      pc.close();
      delete peers[peerID];
      removePeerVideo(peerID);
    }
  };

  pc.onnegotiationneeded = () => {
    pc.createOffer()
      .then((offer) => pc.setLocalDescription(offer))
      .then(() => {
        sendSignal({ type: "offer", from: myID, to: peerID, roomID, sdp: pc.localDescription.sdp });
      })
      .catch((err) => console.error("Error creating offer", err));
  };

  return pc;
}

connectWS();

navigator.mediaDevices
  .getUserMedia({ video: true, audio: true })
  .then((stream) => {
    localStream = stream;
    const localVideo = document.getElementById("local_video");
    localVideo.srcObject = stream;
    updateLayout();
    // Connect to any peers that arrived before the local stream was ready.
    pendingPeers.forEach((peerID) => {
      if (!peers[peerID]) {
        peers[peerID] = createPeerConnection(peerID);
      }
    });
    pendingPeers.clear();
    return localVideo.play();
  })
  .catch((err) => console.error("Error getting user media", err))
  .finally(() => {
    // Dice rooms (created via "Create Dice Room") auto-start the game.
    const params = new URLSearchParams(window.location.search);
    if (params.get("game") === "dice" && typeof startDiceGame === "function") {
      startDiceGame();
    }
  });
