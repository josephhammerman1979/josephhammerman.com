// video.js

const videoRoot = document.getElementById("video-root");
const roomID = videoRoot.dataset.roomId;

function randomID() {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}
const myID = randomID();
const peers = Object.create(null);
const pendingPeers = new Set();
const wsScheme = window.location.protocol === "https:" ? "wss://" : "ws://";
const ws = new WebSocket(
  wsScheme + window.location.host + "/rooms/" + encodeURIComponent(roomID) + "/ws?userID=" + encodeURIComponent(myID)
);
let localStream = null;

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

ws.onopen = () => {
  console.log("WS connected, myID =", myID, "roomID =", roomID);
};

ws.onmessage = (evt) => {
  const msg = JSON.parse(evt.data);
  if (!msg || msg.roomID !== roomID) return;

  // Server-initiated notification of existing room members.
  if (msg.type === "peers") {
    msg.peers.forEach((peerID) => {
      if (peerID === myID || peers[peerID]) return;
      if (localStream) {
        const pc = createPeerConnection(peerID);
        peers[peerID] = pc;
      } else {
        pendingPeers.add(peerID);
      }
    });
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
};

ws.onclose = () => {
  console.log("WS closed");
  Object.values(peers).forEach((pc) => pc.close());
};

function sendSignal(payload) {
  if (ws.readyState === WebSocket.OPEN) {
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
  .catch((err) => console.error("Error getting user media", err));
