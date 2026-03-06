// video.js

// Read roomID from data attribute set in video.gohtml
const roomID = document.body.dataset.roomId;

// Generate a per-tab userID (no persistence needed)
const myID = crypto.randomUUID();

// Map of peerID -> RTCPeerConnection
const peers = Object.create(null);

// Single outbound WebSocket for signaling
const wsScheme = window.location.protocol === "https:" ? "wss://" : "ws://";
const ws = new WebSocket(
  wsScheme + window.location.host + "/rooms/" + encodeURIComponent(roomID) + "/ws?userID=" + encodeURIComponent(myID)
);

// Local media stream
let localStream = null;

ws.onopen = () => {
  console.log("WS connected, myID =", myID, "roomID =", roomID);
};

ws.onmessage = (evt) => {
  const msg = JSON.parse(evt.data);

  // Basic guards
  if (!msg || msg.roomID !== roomID || msg.to !== myID) {
    return;
  }

  const from = msg.from;
  if (!from || from === myID) {
    return;
  }

  let pc = peers[from];
  if (!pc) {
    pc = createPeerConnection(from);
    peers[from] = pc;
  }

  switch (msg.type) {
    case "offer": {
      pc.setRemoteDescription(new RTCSessionDescription(msg))
        .then(() => pc.createAnswer())
        .then((answer) => pc.setLocalDescription(answer))
        .then(() => {
          sendSignal({
            type: "answer",
            from: myID,
            to: from,
            roomID: roomID,
            sdp: pc.localDescription.sdp,
          });
        })
        .catch((err) => console.error("Error handling offer", err));
      break;
    }

    case "answer": {
      pc.setRemoteDescription(new RTCSessionDescription(msg))
        .catch((err) => console.error("Error setting remote answer", err));
      break;
    }

    case "candidate": {
      if (msg.ice) {
        pc.addIceCandidate(new RTCIceCandidate(msg.ice))
          .catch((err) => console.error("Error adding ICE", err));
      }
      break;
    }

    default:
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

function createPeerConnection(peerID) {
  const pc = new RTCPeerConnection({
    iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
  });

  // Attach local tracks
  if (localStream) {
    localStream.getTracks().forEach((track) => pc.addTrack(track, localStream));
  }

  pc.onicecandidate = (evt) => {
    if (evt.candidate) {
      sendSignal({
        type: "candidate",
        from: myID,
        to: peerID,
        roomID: roomID,
        ice: evt.candidate,
      });
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
      document.getElementById("remote_container").appendChild(video);
    }
    if (video.srcObject !== stream) {
      video.srcObject = stream;
    }
  };

  // When negotiation is needed, this side is offering to peerID
  pc.onnegotiationneeded = () => {
    pc.createOffer()
      .then((offer) => pc.setLocalDescription(offer))
      .then(() => {
        sendSignal({
          type: "offer",
          from: myID,
          to: peerID,
          roomID: roomID,
          sdp: pc.localDescription.sdp,
        });
      })
      .catch((err) => console.error("Error creating offer", err));
  };

  return pc;
}

// Get local media once and attach to local video
navigator.mediaDevices
  .getUserMedia({ video: true, audio: true })
  .then((stream) => {
    localStream = stream;
    const localVideo = document.getElementById("local_video");
    localVideo.srcObject = stream;
    return localVideo.play();
  })
  .catch((err) => {
    console.error("Error getting user media", err);
  });

