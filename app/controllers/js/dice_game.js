// dice_game.js
// Handles WASM loading, player coordination, and game-event routing for
// the Pig Dice multiplayer mini-game embedded in a WebRTC room.
//
// Depends on globals set by video.js:  myID, roomID, peers, sendSignal,
//                                      mySlot, playerSlots

let diceGameRunning = false;
let diceGamePlayers = [];  // list of clientIDs ordered by server-assigned slot

// Build a clientID list ordered by server-assigned slot index. Used both to
// pass numPlayers/myPlayerIdx to the WASM and to render the player chip list.
function _playerListBySlot() {
  const out = [];
  Object.keys(playerSlots).forEach((id) => {
    out[playerSlots[id]] = id;
  });
  return out.filter((id) => typeof id === "string");
}

// ── Entry point ──────────────────────────────────────────────────────────────

/**
 * Start a new game session.  Called when the local user clicks the Start button.
 * Broadcasts a game_start message so peers can initialise their own instances.
 * The server's slot map is authoritative — peers compute their own player list
 * from the cached playerSlots rather than trusting the message body.
 */
function startDiceGame() {
  if (diceGameRunning) return;

  // Broadcast to all peers so they can start their own game instance.
  sendSignal({
    type: "game_start",
    from: myID,
    to: "room",
    roomID: roomID,
  });

  // Initialise our own instance.
  _initDiceGame();
}

/**
 * Initialise (or re-initialise) the game using the server-assigned slot map.
 * Safe to call multiple times — subsequent calls while a game is running
 * are silently ignored.
 */
function _initDiceGame() {
  if (diceGameRunning) return;

  const players = _playerListBySlot();
  const myPlayerIdx = mySlot;
  if (myPlayerIdx < 0 || myPlayerIdx >= players.length) {
    console.error("[DiceGame] no server-assigned slot yet; ignoring game_start");
    return;
  }
  diceGameRunning = true;
  diceGamePlayers = players;

  // Show the canvas panel + action buttons; hide the Start button.
  const container = document.getElementById("game-container");
  const startBtn  = document.getElementById("start-game-btn");
  const actionRow = document.getElementById("game-action-row");
  if (container) container.style.display = "block";
  if (startBtn)  startBtn.style.display  = "none";
  if (actionRow) actionRow.classList.add("active");

  _updateGamePlayerList(players, myPlayerIdx);
  _setupActionButtons();

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

// ── On-screen Roll / Hold buttons ────────────────────────────────────────────

// Tracks the current player as reported by the WASM via diceGameOnTurnChange.
// -1 until the WASM finishes loading and registers its callback.
let currentTurnIdx = -1;

function _setupActionButtons() {
  const rollBtn = document.getElementById("roll-btn");
  const holdBtn = document.getElementById("hold-btn");
  if (rollBtn && !rollBtn.dataset.wired) {
    rollBtn.addEventListener("click", () => {
      if (typeof window.diceGameRoll === "function") window.diceGameRoll();
    });
    rollBtn.dataset.wired = "1";
  }
  if (holdBtn && !holdBtn.dataset.wired) {
    holdBtn.addEventListener("click", () => {
      if (typeof window.diceGameHold === "function") window.diceGameHold();
    });
    holdBtn.dataset.wired = "1";
  }
  _refreshActionButtons();
}

function _refreshActionButtons() {
  const rollBtn = document.getElementById("roll-btn");
  const holdBtn = document.getElementById("hold-btn");
  const myTurn = diceGameRunning && currentTurnIdx === mySlot;
  if (rollBtn) rollBtn.disabled = !myTurn;
  if (holdBtn) holdBtn.disabled = !myTurn;
}

// Called by the WASM whenever the current player changes (also once at game
// start with the initial CurrentIndex).  Tracks the value, updates the
// Roll/Hold enabled-state, and drives the mobile fullscreen takeover.
window.diceGameOnTurnChange = function(currentIdx) {
  const previous = currentTurnIdx;
  currentTurnIdx = currentIdx;
  _refreshActionButtons();
  // Reset the user-minimised opt-out when the turn comes back to us — they
  // probably want the fullscreen takeover next time even if they dismissed
  // it last turn.
  if (currentIdx === mySlot && previous !== mySlot) {
    userMinimizedFullscreen = false;
  }
  _refreshFullscreen();
};

// ── Mobile fullscreen takeover ───────────────────────────────────────────────
//
// When it's the local player's turn on a narrow viewport, take over the
// screen so the canvas + Roll/Hold buttons are big and thumb-reachable; the
// video grid collapses to a PIP overlay. The user can opt out for the rest
// of the current turn via the exit button.

const NARROW_VIEWPORT_QUERY = "(max-width: 768px)";
let userMinimizedFullscreen = false;

function _refreshFullscreen() {
  const wantFullscreen =
    diceGameRunning &&
    currentTurnIdx === mySlot &&
    !userMinimizedFullscreen &&
    window.matchMedia(NARROW_VIEWPORT_QUERY).matches;
  document.body.classList.toggle("dice-fullscreen", wantFullscreen);
}

// Wire the exit button + viewport-change listener once on script load.
(function setupFullscreenControls() {
  const exitBtn = document.getElementById("exit-fullscreen-btn");
  if (exitBtn) {
    exitBtn.addEventListener("click", () => {
      userMinimizedFullscreen = true;
      _refreshFullscreen();
    });
  }
  if (window.matchMedia) {
    const mq = window.matchMedia(NARROW_VIEWPORT_QUERY);
    const onChange = () => _refreshFullscreen();
    if (mq.addEventListener) mq.addEventListener("change", onChange);
    else if (mq.addListener) mq.addListener(onChange);
  }
})();

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
      if (!diceGameRunning) {
        _initDiceGame();
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

// Called by video.js whenever the server-assigned slot map changes (either
// the initial "peers" message or a subsequent "player_joined").  Keeps the
// displayed player chip list in sync and clears any "reconnecting…" status.
function onSlotsUpdated() {
  const statusEl = document.getElementById("game-status");
  if (statusEl && statusEl.textContent === "Reconnecting…") {
    statusEl.textContent = "";
  }
  if (!diceGameRunning) {
    _updateGamePlayerList(_playerListBySlot(), mySlot);
  }
}

// Called by video.js when a peer disconnects. The server preserves their
// slot for rejoin; we surface a status so the local user knows the game is
// effectively paused.
function onPlayerLeft(peerID) {
  const statusEl = document.getElementById("game-status");
  if (!statusEl) return;
  const slot = playerSlots[peerID];
  const label = typeof slot === "number" ? `Player ${slot + 1}` : "A player";
  statusEl.textContent = `${label} disconnected — waiting for rejoin…`;
}

// Called by video.js when the local WebSocket drops (network blip, server
// restart, etc.). The reconnect runs automatically in the background.
function onWSDisconnected() {
  const statusEl = document.getElementById("game-status");
  if (statusEl) statusEl.textContent = "Reconnecting…";
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
// Optional input mode: shake the phone instead of pressing the Roll button.
// iOS 13+ requires an explicit permission grant inside a user gesture, so
// on those devices we show an "Enable Shake-to-Roll" button.  Every other
// platform with DeviceMotion support (Android Chrome, older iOS) attaches
// the listener immediately — no extra tap required.  The on-screen Roll
// button is always available as a fallback.

(function setupShakeToRoll() {
  if (typeof window.DeviceMotionEvent === "undefined") return; // desktop / no sensor

  const btn = document.getElementById("enable-shake-btn");
  const needsPermission =
    typeof window.DeviceMotionEvent.requestPermission === "function";

  // Shake-detection state. SHAKE_THRESHOLD is the L1 norm of the per-axis
  // acceleration delta between samples (m/s²-ish); SHAKE_DEBOUNCE caps to
  // roughly one roll per second.
  const SHAKE_THRESHOLD = 25;
  const SHAKE_DEBOUNCE_MS = 900;
  let last = { x: 0, y: 0, z: 0, t: 0 };
  let lastShakeAt = 0;
  let attached = false;

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

  function attach() {
    if (attached) return;
    window.addEventListener("devicemotion", onMotion);
    attached = true;
    if (btn) {
      btn.textContent = "Shake-to-roll: ON";
      btn.disabled = true;
    }
  }

  if (!needsPermission) {
    // Android / non-iOS: attach immediately. The button is unnecessary here.
    attach();
    return;
  }

  // iOS 13+: surface the permission button and wire it to requestPermission.
  if (!btn) return;
  btn.style.display = "inline-block";
  btn.addEventListener("click", () => {
    window.DeviceMotionEvent.requestPermission()
      .then((state) => {
        if (state === "granted") attach();
        else btn.textContent = "Shake denied — use Roll button";
      })
      .catch((err) => {
        console.error("[DiceGame] motion permission error:", err);
        btn.textContent = "Shake unavailable — use Roll button";
      });
  });
})();
