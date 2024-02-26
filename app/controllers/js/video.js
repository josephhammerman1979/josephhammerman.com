let iceCandidatesQueue = [];

let peerConnection = new RTCPeerConnection({
  iceServers: [
    {urls: [
        "stun.l.google.com:19302",
        "stun:stun1.l.google.com:19302",
        "stun:stun2.l.google.com:19302",
        "stun:stun3.l.google.com:19302",
        "stun:stun4.l.google.com:19302",
      ],
    },
    {urls: [
        "stun:global.stun.twilio.com:3478?transport=udp",
      ],
    },
    {urls: [
        "stun:stun.stunprotocol.org:3478",
      ],
    }
  ]
  iceCandidatePoolSize: 10,
}),
  ws = new WebSocket((window.location.protocol === "https:" ? "wss://" : "ws://") + window.location.host + '/video/connections' + window.location.search);
  console.log('WebSocket connection established');

ws.onmessage = (evt) => {
  const message = JSON.parse(evt.data);
  console.log('Message received: ', evt.data);

  switch (message.type) {
    case 'offer': {
      peerConnection.setRemoteDescription(message)
        .then(() => peerConnection.createAnswer())
        .then(answer => peerConnection.setLocalDescription(answer))
        .then(() => ws.send(JSON.stringify(peerConnection.localDescription)))
        .catch(error => console.error('Error setting remote description or creating answer: ', error));
      break;
    }
    case 'answer': {
      peerConnection.setRemoteDescription(message);
      break;
    }
    case 'candidate': {
      const iceCandidate = new RTCIceCandidate(message.ice);
      if (peerConnection.remoteDescription) {
        peerConnection.addIceCandidate(iceCandidate)
          .catch(error => console.error('Error adding ICE candidate: ', error));
      } else {
        console.warn('Remote description is not set yet, queueing ICE candidate');
        iceCandidatesQueue.push(iceCandidate); // Add ICE candidate to the queue
      }
      break;
    }
  }
};

// Handle queued ICE candidates once remote description is set
peerConnection.onnegotiationneeded = () => {
  while (iceCandidatesQueue.length > 0) {
    const iceCandidate = iceCandidatesQueue.shift();
    peerConnection.addIceCandidate(iceCandidate)
      .catch(error => console.error('Error adding queued ICE candidate: ', error));
  }
};

navigator.mediaDevices.getUserMedia({video: true, audio: true}).then(stream => {
  let element = document.getElementById('local_video');
  element.srcObject = stream;
  element.play().then(() => {
    stream.getTracks().forEach(track => peerConnection.addTrack(track, stream));
    peerConnection.onnegotiationneeded = () => {
      peerConnection.createOffer().then(offer => {
        return peerConnection.setLocalDescription(offer);
      }).then(() => {
          wsPromise.then(() => {
              ws.send(JSON.stringify(peerConnection.localDescription));
          }).catch(error => {
              console.error('Error sending data over WebSocket: ', error);
          });
      });
    }
  });
});

peerConnection.ontrack = evt => {
  console.log('Track added: ', evt.track);
  let element = document.getElementById('remote_video');
  if (element.srcObject === evt.streams[0]) return;
  element.srcObject = evt.streams[0];
  element.play();
};

peerConnection.onicecandidate = evt => {
  if (evt.candidate && ws.readyState === WebSocket.OPEN) {
    console.log('ICE candidate generated: ', evt.candidate);
    wsPromise.then(() => {
        ws.send(JSON.stringify({type: 'candidate', ice: evt.candidate}));
    }).catch(error => {
        console.error('Error sending data over WebSocket: ', error);
     });
  } else {
    console.log('WebSocket connection not open or candidate is null');
  }
};

ws.onmessage = (evt) => {
  const message = JSON.parse(evt.data);
  console.log('Signaling message received: ', message);
  // Add more detailed logging as needed
};

peerConnection.oniceconnectionstatechange = (evt) => {
  console.log('ICE connection state change: ', peerConnection.iceConnectionState);
  // Add additional error handling if needed
}

const wsPromise = new Promise((resolve, reject) => {
  if (ws.readyState === WebSocket.OPEN) {
    resolve();
  } else {
    ws.onopen = () => {
      resolve();
    };
  }
});
