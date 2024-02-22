let peerConnection = new RTCPeerConnection({"iceServers": [{"urls": "stun:stun.l.google.com:19302"}]}),
    ws = new WebSocket((window.location.protocol === "https:" ? "wss://" : "ws://") + window.location.host + '/video/connections' + window.location.search);
    console.log('WebSocket connection established');

ws.onmessage = (evt) => {
  const message = JSON.parse(evt.data);
  console.log('Message received: ', evt.data);
  switch (message.type) {
    case 'offer': {
      peerConnection.setRemoteDescription(message).then(() => {
        return peerConnection.createAnswer()
      }).then(answer => {
        return peerConnection.setLocalDescription(answer)
      }).then(() => {
        ws.send(JSON.stringify(peerConnection.localDescription));
      });
      break;
    }
    case 'answer': {
      peerConnection.setRemoteDescription(message);
      break;
    }
    case 'candidate': {
      peerConnection.addIceCandidate(new RTCIceCandidate(message.ice));
      break;
    }
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
        ws.send(JSON.stringify(peerConnection.localDescription));
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
  if (evt.candidate) {
    console.log('ICE candidate generated: ', evt.candidate);
    ws.send(JSON.stringify({type: 'candidate', ice: evt.candidate}));
  }
};

peerConnection.oniceconnectionstatechange = (evt) => {
  console.log('ICE connection state change: ', peerConnection.iceConnectionState);
  // Add additional error handling if needed
}
