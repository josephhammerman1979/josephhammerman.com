let iceCandidatesQueue = [];
let ws;
let peerConnection;
let wsPromise;

async function initializePeerConnection() {
  try {
    // const response = await fetch("https://josephhammerman.metered.live/api/v1/turn/credentials?apiKey=9a6bf82f8a9f452e5a05748571f5dd8033c6");
    // const iceServers = await response.json();

    // const peerConfiguration = { iceServers: iceServers, iceCandidatePoolSize: 10 };
    peerConnection = new RTCPeerConnection({"iceServers": [{"urls": "stun:stun.l.google.com:19302", iceCandidatePoolSize: 10 }]})

    // Now that we have the ICE servers, create the peer connection
    // peerConnection = new RTCPeerConnection(peerConfiguration);

    console.log('Created RTCPeerConnection with configuration: ', peerConnection.getConfiguration());

    // Initialize WebSocket connection
    ws = new WebSocket((window.location.protocol === "https:" ? "wss://" : "ws://") + window.location.host + '/video/connections' + window.location.search);
    console.log('WebSocket connection established');

    wsPromise = new Promise((resolve, reject) => {
       if (ws.readyState === WebSocket.OPEN) {
         console.log('WebSocket is already open.');
         resolve();
      }
      ws.onopen = () => {
        console.log('WebSocket open event');
        resolve();
      };
      ws.onerror = (error) => {
        console.error('WebSocket error: ', error);
        reject(error);
      };
    });

    setupWebSocketEventHandlers();
    setupPeerConnectionEventHandlers();

  } catch (error) {
    console.error('Error initializing peer connection: ', error);
  }
}

function setupWebSocketEventHandlers() {
ws.onmessage = (evt) => {
  console.log('WebSocket message received: ', evt.data);
  const message = JSON.parse(evt.data);
  console.log('Signaling message type: ', message.type);
  console.log('Signaling message received: ', JSON.stringify(message));

  switch (message.type) {
    case 'offer': {
      console.log('Setting remote description with offer: ', JSON.stringify(message));
      peerConnection.setRemoteDescription(RTCSessionDescription(message))
        .then(() => { return peerConnection.createAnswer()})
        .then(answer => {
          console.log('Answer created');
          return peerConnection.setLocalDescription(answer);
         })
        wsPromise.then(() => {
          console.log('Local description set to answer');
          ws.send(JSON.stringify(peerConnection.localDescription));
          processIceCandidatesQueue()
        })
        .then(() => console.log('Sent local description to peer'))
        .catch(error => console.error('Error setting remote description or creating answer: ', error));
      break;
    }
    case 'answer': {
      console.log('Setting remote description with answer: ', JSON.stringify(message));
      peerConnection.setRemoteDescription(RTCSessionDescription(message))
        .then(() => console.log('Set remote description'))
        .then(() => processIceCandidatesQueue()) // Process the ICE candidate queue after setting remote description
        .then(() => console.log('Processed candidate queue in web handlers'))
        .catch(error => console.error('Error during answer handling: ', error));
      break;
    }
    case 'candidate': {
      console.log('ICE candidate received: ', JSON.stringify(message.ice));
      const iceCandidate = new RTCIceCandidate(message.ice);
      if (peerConnection.remoteDescription) {
        console.log('Attempting to add ICE candidate: ', JSON.stringify(message.ice));
        peerConnection.addIceCandidate(iceCandidate)
          .then(() => console.log('Added ICE candidate', message))
          .catch(error => console.error('Error adding ICE candidate: ', error));
      } else {
        console.warn('Remote description is not set yet, queueing ICE candidate');
        iceCandidatesQueue.push(iceCandidate); // Add ICE candidate to the queue
        console.log('ICE candidate enqueued: ', message.ice);
      }
      break;
    }
  }
};

ws.onclose = function(event) {
    console.log('WebSocket closed. Attempting to reconnect...');
    setTimeout(function() {
        // Attempt to reconnect
        ws = new WebSocket(ws.url);
        // Re-apply event listeners and re-initialize as necessary
    }, 1000); // Reconnect after 1 second
};
}

function setupPeerConnectionEventHandlers() {

navigator.mediaDevices.getUserMedia({video: true, audio: true}).then(stream => {
  let element = document.getElementById('local_video');
  element.srcObject = stream;
  element.play().then(() => {
    stream.getTracks().forEach(track => peerConnection.addTrack(track, stream));
    peerConnection.onnegotiationneeded = () => {
      console.log('Negotiation needed');
      peerConnection.createOffer().then(offer => {
        return peerConnection.setLocalDescription(offer);
      }).then(() => {
          wsPromise.then(() => {
              ws.send(JSON.stringify(peerConnection.localDescription));
          }).catch(error => {
              console.error('Error sending data over WebSocket: ', error);
          });
      }).then(() => {
        // After setting the local description and possibly receiving the remote description,
        // start processing the queued ICE candidates.
        processIceCandidatesQueue()
        console.log('Processed candidate queue from onnegotiationneededr handlers');
      });
    }
  });
});

peerConnection.ontrack = evt => {
  console.log('Remote track added, streams: ', evt.streams);
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
}

// Function to process the queued ICE candidates
function processIceCandidatesQueue() {
  console.log('Entered ICE candidate dequeue function');

  const maxWaitTime = 50000;
  const startTime = Date.now();

  function processQueue() {
    // Check if the queue has been populated or if the max wait time has been exceeded
    //console.log('Checking candidate queue for members');
    if (peerConnection.remoteDescription) {
    if (iceCandidatesQueue.length > 0 || Date.now() - startTime > maxWaitTime) {
      while (iceCandidatesQueue.length > 0) {
        console.log('ICE candidate pool populated');
        const iceCandidate = iceCandidatesQueue.shift();
        peerConnection.addIceCandidate(iceCandidate)
          .then(() => console.log('Added ICE candidate from within the candidate queue processing func', iceCandidate))
          .catch(error => console.error('Error adding queued ICE candidate: ', error));
      }
    } else {
      // If the queue is still empty and the max wait time has not been exceeded, check again after a delay
      console.log('Candidate queue empty, sleeping')
      setTimeout(processQueue, 200); // Check again after 2 seconds
    }
  }
  }

  processQueue(); // Start the queue processing
}

initializePeerConnection();
