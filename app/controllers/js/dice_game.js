// dice_game.js
// Handles WASM loading, player coordination, and game-event routing for
// the Pig Dice multiplayer mini-game embedded in a WebRTC room.
//
// Depends on globals set by video.js:  myID, roomID, peers, sendSignal

let diceGameRunning = false;
let diceGamePlayers = [];  // sorted list of userIDs participating in this game session

// ── Entry point ──────────────────────────────────────────────────────────────

/**
 * Start a new game session.  Called when the local user clicks the Start button.
 * Broadcasts a game_start message so peers can initialise their own instances.
 */
function startDiceGame() {
  if (diceGameRunning) return;

  // Build a deterministic, sorted player list from everyone currently in the room.
  const allPlayers = [myID, ...Object.keys(peers)].sort();

  // Broadcast to all peers so they can start their own game instance.
  sendSignal({
    type: "game_start",
    from: myID,
    to: "room",
    roomID: roomID,
    players: allPlayers,
  });

  // Initialise our own instance.
  _initDiceGame(allPlayers);
}

/**
 * Initialise (or re-initialise) the game for a given player list.
 * Safe to call multiple times — subsequent calls while a game is running
 * are silently ignored.
 */
function _initDiceGame(players) {
  if (diceGameRunning) return;
  diceGameRunning = true;
  diceGamePlayers = players;

  const myPlayerIdx = players.indexOf(myID);

  // Show the canvas panel.
  const container = document.getElementById("game-container");
  const startBtn  = document.getElementById("start-game-btn");
  if (container) container.style.display = "block";
  if (startBtn)  startBtn.style.display  = "none";

  _updateGamePlayerList(players, myPlayerIdx);

  // Configuration read by the Go WASM runtime at startup.
  window.diceGameConfig = {
    numPlayers:  players.length,
    myPlayerIdx: myPlayerIdx,
  };

  // Called by the Go game whenever the local player takes an action.
  window.diceGameSendEvent = function(jsonStr) {
    const event = JSON.parse(jsonStr);
    // Fan-out: send to every peer individually (server routes by To).
    Object.keys(peers).forEach(peerID => {
      sendSignal({
        type:   "game_event",
        from:   myID,
        to:     peerID,
        roomID: roomID,
        event:  event,
      });
    });
  };

  _loadWasm();
}

// ── WASM loading ─────────────────────────────────────────────────────────────

async function _loadWasm() {
  const statusEl = document.getElementById("game-status");
  try {
    if (statusEl) statusEl.textContent = "Loading game…";

    // The Go constructor comes from /js/wasm_exec.js (loaded in <head>).
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(
      fetch("/wasm/pig.wasm"),
      go.importObject
    );
    if (statusEl) statusEl.textContent = "";
    go.run(result.instance);
  } catch (err) {
    console.error("[DiceGame] WASM load error:", err);
    if (statusEl) statusEl.textContent = "Failed to load game. Run ./build_wasm.sh first.";
  }
}

// ── Incoming WebSocket message handler ───────────────────────────────────────

/**
 * Called from video.js ws.onmessage for game-related message types.
 */
function handleDiceGameMessage(msg) {
  switch (msg.type) {
    case "game_start":
      if (!diceGameRunning && Array.isArray(msg.players)) {
        _initDiceGame(msg.players);
      }
      break;

    case "game_event":
      if (diceGameRunning && msg.event) {
        // Pass the event payload to the running Go WASM instance.
        const fn = window.diceGameReceiveEvent;
        if (typeof fn === "function") {
          fn(JSON.stringify(msg.event));
        }
      }
      break;
  }
}

// ── UI helpers ────────────────────────────────────────────────────────────────

function _updateGamePlayerList(players, myPlayerIdx) {
  const listEl = document.getElementById("game-player-list");
  if (!listEl) return;
  listEl.innerHTML = players.map((id, idx) => {
    const isMe = idx === myPlayerIdx;
    const label = isMe ? `<strong>You (P${idx + 1})</strong>` : `Player ${idx + 1}`;
    return `<span class="game-player-chip${isMe ? " me" : ""}">${label}</span>`;
  }).join("");
}
