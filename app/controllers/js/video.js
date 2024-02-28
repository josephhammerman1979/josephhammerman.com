let iceCandidatesQueue = [];
let ws;
let peerConnection;

async function initializePeerConnection() {
  try {
    const response = await fetch("https://josephhammerman.metered.live/api/v1/turn/credentials?apiKey=9a6bf82f8a9f452e5a05748571f5dd8033c6");
    const iceServers = await response.json();
    const peerConfiguration = { iceServers: iceServers };

    // Now that we have the ICE servers, create the peer connection
    peerConnection = new RTCPeerConnection(peerConfiguration);

    // Initialize WebSocket connection
    ws = new WebSocket((window.location.protocol === "https:" ? "wss://" : "ws://") + window.location.host + '/video/connections' + window.location.search);
    console.log('WebSocket connection established');

    setupWebSocketEventHandlers();

  } catch (error) {
    console.error('Error initializing peer connection: ', error);
  }
}

function setupWebSocketEventHandlers() {
const wsPromise = new Promise((resolve, reject) => {
  if (ws.readyState === WebSocket.OPEN) {
    resolve();
  } else {
      ws.onopen = () => {
        console.log('WebSocket open event');
        resolve();
      };
       ws.onerror = (error) => {
            reject(error);
       };
  }
})

ws.onmessage = (evt) => {
  const message = JSON.parse(evt.data);
  console.log('Signaling message type: ', message.type);
  console.log('Signaling message received: ', JSON.stringify(message));

  switch (message.type) {
    case 'offer': {
      console.log('Offer received: ', JSON.stringify(message));
      peerConnection.setRemoteDescription(message)
        .then(() => peerConnection.createAnswer())
        .then(answer => {
          console.log('Answer created');
          return peerConnection.setLocalDescription(answer);
         })
        wsPromise.then(() => {
          console.log('Local description set to answer');
          ws.send(JSON.stringify({ type: 'answer', sdp: peerConnection.localDescription }));
        })
        .then(() => console.log('Sent local description to peer'))
        .catch(error => console.error('Error setting remote description or creating answer: ', error));
      break;
    }
    case 'answer': {
      console.log('Answer received: ', JSON.stringify(message));
      peerConnection.setRemoteDescription(message);
      break;
    }
    case 'candidate': {
      console.log('ICE candidate received: ', JSON.stringify(message.ice));
      const iceCandidate = new RTCIceCandidate(message.ice);
      if (peerConnection.remoteDescription) {
        peerConnection.addIceCandidate(iceCandidate)
          .then(() => console.log('Setting remote description: ', message))
          .catch(error => console.error('Error adding ICE candidate: ', error));
      } else {
        console.warn('Remote description is not set yet, queueing ICE candidate');
        iceCandidatesQueue.push(iceCandidate); // Add ICE candidate to the queue
        console.log('ICE candidate received: ', message.ice);
      }
      break;
    }
  }
};
}

initializePeerConnection();

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
    console.log('ICE candidate generated: ', JSON.stringify(evt.candidate));
    wsPromise.then(() => {
        ws.send(JSON.stringify({type: 'candidate', ice: evt.candidate}));
        console.log('ICE candidate sent: ', JSON.stringify(evt.candidate));
    }).catch(error => {
        console.error('Error sending data over WebSocket: ', error);
     });
  } else {
    console.log('WebSocket connection not open or candidate is null');
  }
};

peerConnection.oniceconnectionstatechange = (evt) => {
  console.log('ICE connection state change: ', peerConnection.iceConnectionState);
  // Add additional error handling if needed
}
