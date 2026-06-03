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

// ── Shake-to-roll ────────────────────────────────────────────────────────────
//
// On phones we expose a "shake the device" gesture as an alternative to
// pressing SPACE. The Go wasm exposes window.diceGameRoll() once the game is
// running; we call that on each detected shake. iOS requires an explicit
// permission grant triggered from a user gesture, so we show an enable
// button when DeviceMotionEvent is supported.

(function setupShakeToRoll() {
  // No DeviceMotionEvent? (most desktops) → nothing to do.
  if (typeof window.DeviceMotionEvent === "undefined") return;

  const btn = document.getElementById("enable-shake-btn");
  if (!btn) return;
  btn.style.display = "inline-block";

  // Shake-detection state. Tunable: SHAKE_THRESHOLD measures the L1 norm of
  // the per-axis acceleration delta between samples (m/s²-ish). SHAKE_DEBOUNCE
  // throttles to roughly one roll per second.
  const SHAKE_THRESHOLD = 25;
  const SHAKE_DEBOUNCE_MS = 900;
  let last = { x: 0, y: 0, z: 0, t: 0 };
  let lastShakeAt = 0;
  let enabled = false;

  function onMotion(evt) {
    const a = evt.accelerationIncludingGravity;
    if (!a) return;
    const now = Date.now();
    if (last.t === 0) {
      last = { x: a.x || 0, y: a.y || 0, z: a.z || 0, t: now };
      return;
    }
    const dt = now - last.t;
    if (dt < 80) return; // ~12 samples/s is plenty
    const delta =
      Math.abs((a.x || 0) - last.x) +
      Math.abs((a.y || 0) - last.y) +
      Math.abs((a.z || 0) - last.z);
    last = { x: a.x || 0, y: a.y || 0, z: a.z || 0, t: now };

    if (delta > SHAKE_THRESHOLD && now - lastShakeAt > SHAKE_DEBOUNCE_MS) {
      lastShakeAt = now;
      if (typeof window.diceGameRoll === "function") {
        window.diceGameRoll();
      }
    }
  }

  function enable() {
    if (enabled) return;
    window.addEventListener("devicemotion", onMotion);
    enabled = true;
    btn.textContent = "Shake-to-roll: ON";
    btn.disabled = true;
  }

  btn.addEventListener("click", () => {
    // iOS 13+ gates DeviceMotion behind a permission prompt.
    const reqPerm = window.DeviceMotionEvent.requestPermission;
    if (typeof reqPerm === "function") {
      reqPerm()
        .then((state) => {
          if (state === "granted") enable();
          else btn.textContent = "Shake-to-roll: denied";
        })
        .catch((err) => {
          console.error("[DiceGame] motion permission error:", err);
          btn.textContent = "Shake-to-roll: error";
        });
    } else {
      enable();
    }
  });
})();
