// dice_game.js
// Handles WASM loading, player coordination, and game-event routing for
// the Pig Dice multiplayer mini-game embedded in a WebRTC room.
//
// Depends on globals set by video.js:  myID, roomID, peers, sendSignal,
//                                      mySlot, playerSlots

// "Running" means a game is in progress in this tab (WASM loaded, we're a
// participant). "Spectating" means a game is in progress in the room but we
// joined after it started — we get chips + status, no WASM, no Roll/Hold.
// "Ended" means the most recent game finished; a New Game click can start
// a fresh round without reloading the WASM.
let diceGameRunning = false;
let diceGameSpectating = false;
let gameEnded = false;

// The roster is captured at game_start time and frozen for the life of that
// game. The host's broadcast carries the authoritative list. New peers
// joining after that point do not get a slot in this game; they become
// spectators and wait for the next game_start.
let gameRoster = [];     // ordered list of clientIDs participating in the current game
let gameHostID = null;   // clientID of the host (first to send game_start)
let myGameSlot = -1;     // our index within gameRoster, or -1 if spectator
let currentVariant = "pig";    // active game's rules variant ("pig" | "bigpig")
const kickedSlots = new Set(); // game-roster indices that have been kicked

// kickedClientIDs persists ACROSS rounds for the lifetime of the page —
// once a player is booted for AFK, they stay out of subsequent automatic
// rosters even though their server slot is still active. (A page reload
// clears it, by design: it's session-scoped, not durable punishment.)
const kickedClientIDs = new Set();

// Build a clientID list ordered by server-assigned slot index. Used to
// snapshot the roster at game_start time and to render the chip list before
// a game has started.  Excludes clientIDs in kickedClientIDs so that
// next-round auto-rosters skip players the host already kicked.
function _playerListBySlot() {
  const out = [];
  Object.keys(playerSlots).forEach((id) => {
    out[playerSlots[id]] = id;
  });
  return out
    .filter((id) => typeof id === "string")
    .filter((id) => !kickedClientIDs.has(id));
}

// Read the variant picker; default to "pig" if the element is missing or
// the value isn't one of our known variants.
function _selectedVariant() {
  const sel = document.getElementById("game-variant");
  if (!sel) return "pig";
  return sel.value === "bigpig" ? "bigpig" : "pig";
}

// ── Entry point ──────────────────────────────────────────────────────────────

/**
 * Start a new game session.  Called when the local user clicks the Start button.
 * The caller becomes the host: their snapshot of the current playerSlots is
 * frozen as the game roster, and only they can kick AFK players.  Players
 * already kicked for AFK in a prior round are excluded automatically.
 */
function startDiceGame() {
  if (diceGameRunning || diceGameSpectating) return;
  if (gameEnded) _resetGameState();

  const roster  = _playerListBySlot();   // already filters kickedClientIDs
  const variant = _selectedVariant();
  if (roster.length === 0) return;

  // Broadcast roster + variant + host identity so every peer agrees on who
  // is playing and which ruleset is in effect.
  sendSignal({
    type:    "game_start",
    from:    myID,
    to:      "room",
    roomID:  roomID,
    roster:  roster,
    variant: variant,
  });

  _initDiceGame(roster, myID, variant);
}

/**
 * Initialise the game for a specific roster + variant, with a specific host.
 * If our clientID isn't in the roster (we joined after the host clicked
 * Start), we drop into spectator mode — chips render, no WASM, no buttons.
 * Safe to call multiple times — re-entrant calls are silently ignored while
 * the game is running.
 */
function _initDiceGame(roster, hostID, variant) {
  if (diceGameRunning || diceGameSpectating) return;
  if (!Array.isArray(roster) || roster.length === 0) return;

  gameRoster      = roster.slice();
  gameHostID      = hostID;
  myGameSlot      = roster.indexOf(myID);
  currentVariant  = variant === "bigpig" ? "bigpig" : "pig";
  kickedSlots.clear();

  // Reflect the variant in the picker (visible to the user, disabled while
  // a game is in progress) so spectators see what's running.
  _syncVariantPicker();

  // Spectator path: joined after game_start, not in the roster.
  if (myGameSlot < 0) {
    diceGameSpectating = true;
    const startBtn = document.getElementById("start-game-btn");
    const statusEl = document.getElementById("game-status");
    if (startBtn) startBtn.style.display = "none";
    if (statusEl) statusEl.textContent = "Game in progress — waiting for next round.";
    _updateGamePlayerList(roster, -1);
    _ensureChipTick();
    _syncVariantPicker();
    _refreshFullscreen();
    return;
  }

  diceGameRunning = true;

  // Show the canvas panel + state panel + action buttons; hide the Start button.
  const container = document.getElementById("game-container");
  const statePanel = document.getElementById("game-state-panel");
  const startBtn  = document.getElementById("start-game-btn");
  const actionRow = document.getElementById("game-action-row");
  if (container)  container.style.display  = "block";
  if (statePanel) statePanel.style.display = "flex";
  if (startBtn)   startBtn.style.display   = "none";
  if (actionRow)  actionRow.classList.add("active");
  _renderStatePanelFromRoster();

  _updateGamePlayerList(roster, myGameSlot);
  _setupActionButtons();
  _ensureChipTick();
  _refreshFullscreen();

  // Configuration read by the Go WASM runtime at startup.
  window.diceGameConfig = {
    numPlayers:  roster.length,
    myPlayerIdx: myGameSlot,
    gameType:    currentVariant,
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

  // If a previous game already loaded the WASM module, reset it in-place
  // (the Ebiten run loop is still alive). Otherwise instantiate fresh.
  if (typeof window.diceGameReset === "function") {
    window.diceGameReset(roster.length, myGameSlot, currentVariant);
  } else {
    window.diceGameConfig = {
      numPlayers:  roster.length,
      myPlayerIdx: myGameSlot,
      gameType:    currentVariant,
    };
    _loadWasm();
  }
}

// Keep the variant picker visually in sync with the active game and disable
// it while a round is in flight (locking the variant prevents mid-game
// switching from desyncing peers).
function _syncVariantPicker() {
  const sel = document.getElementById("game-variant");
  if (!sel) return;
  if (currentVariant === "bigpig" || currentVariant === "pig") {
    sel.value = currentVariant;
  }
  sel.disabled = diceGameRunning || diceGameSpectating;
}

// Clear all per-game state in preparation for a fresh round.  Does not
// touch the WASM (a subsequent _initDiceGame will call diceGameReset).
// kickedClientIDs is deliberately NOT cleared — kicks for AFK persist across
// rounds for the page lifetime.
function _resetGameState() {
  diceGameRunning         = false;
  diceGameSpectating      = false;
  gameEnded               = false;
  gameRoster              = [];
  gameHostID              = null;
  myGameSlot              = -1;
  currentTurnIdx          = -1;
  turnStartedAt           = 0;
  userRequestedFullscreen = false;
  kickedSlots.clear();
  if (chipTickHandle) { clearInterval(chipTickHandle); chipTickHandle = null; }

  const newGameBtn = document.getElementById("new-game-btn");
  const statusEl   = document.getElementById("game-status");
  const actionRow  = document.getElementById("game-action-row");
  const statePanel = document.getElementById("game-state-panel");
  const enterFsBtn = document.getElementById("enter-fullscreen-btn");
  if (newGameBtn) newGameBtn.style.display = "none";
  if (statusEl)   statusEl.textContent = "";
  if (actionRow)  actionRow.classList.remove("active");
  if (statePanel) statePanel.style.display = "none";
  if (enterFsBtn) enterFsBtn.style.display = "none";
  _syncVariantPicker();
  _refreshFullscreen();
}

// Click handler for the "New Game" button shown after the game ends.
// Resets local state and broadcasts a fresh game_start; peers also reset
// when they see the broadcast (they're either gameEnded already, or were
// spectators of the finished round).
function startNewGame() {
  if (!gameEnded && !(diceGameRunning || diceGameSpectating)) return;
  _resetGameState();
  startDiceGame();
}

// Wire the Start / New Game buttons once on script load.  Start is the
// initial entry point; New Game is what appears after a finished round.
(function setupGameButtons() {
  const startBtn = document.getElementById("start-game-btn");
  if (startBtn) startBtn.addEventListener("click", startDiceGame);
  const newGameBtn = document.getElementById("new-game-btn");
  if (newGameBtn) newGameBtn.addEventListener("click", startNewGame);
})();

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
  const myTurn = diceGameRunning && currentTurnIdx === myGameSlot;
  if (rollBtn) rollBtn.disabled = !myTurn;
  if (holdBtn) holdBtn.disabled = !myTurn;
}

// Called by the WASM when the game ends — either a winning hold or a Kick
// that left only one un-kicked player.  Surfaces the New Game button so the
// round can be replayed without reloading the page.
window.diceGameOnGameOver = function(_winnerIdx) {
  gameEnded = true;
  const newGameBtn = document.getElementById("new-game-btn");
  if (newGameBtn) newGameBtn.style.display = "inline-block";
  // Let the picker re-enable so the user can choose a different variant for
  // the next round.  The new game won't start until they click New Game.
  const sel = document.getElementById("game-variant");
  if (sel) sel.disabled = false;
  _refreshFullscreen();
};

// Called by the WASM whenever any scoring / turn / message state changes.
// Paints the DOM state panel — which is the source of truth for mobile
// fullscreen mode (where the canvas is hidden) and a redundancy on desktop
// so the player never has to squint at the canvas to read their score.
window.diceGameOnStateChange = function(jsonStr) {
  let state;
  try { state = JSON.parse(jsonStr); } catch (_) { return; }
  _renderStatePanel(state);
};

function _renderStatePanel(state) {
  const panel    = document.getElementById("game-state-panel");
  const msgEl    = document.getElementById("game-state-message");
  const facesEl  = document.getElementById("game-state-die-faces");
  const dieLabel = document.getElementById("game-state-die-label");
  const scoresEl = document.getElementById("game-state-scores");
  if (!panel || !msgEl || !facesEl || !dieLabel || !scoresEl) return;

  if (panel.style.display === "none" || panel.style.display === "") {
    panel.style.display = "flex";
  }

  msgEl.textContent = state.message || "";
  msgEl.classList.remove("theirs", "game-over");
  if (state.gameOver) {
    msgEl.classList.add("game-over");
  } else if (typeof state.currentIndex === "number" &&
             typeof state.myPlayerIdx === "number" &&
             state.currentIndex !== state.myPlayerIdx) {
    msgEl.classList.add("theirs");
  }

  // Render one die face per dieValues entry.  For BigPig that's two faces;
  // for Pig it's one. While rolling, faces show the dice emoji.
  const rawVals = Array.isArray(state.dieValues) ? state.dieValues : [];
  // Ensure at least 1 face when we know the variant but the array is empty
  // (initial render before WASM fires its first state event).
  const slots = Math.max(1, rawVals.length);
  const facesHTML = [];
  for (let i = 0; i < slots; i++) {
    const v = state.rolling ? 0 : Math.max(0, Math.min(6, rawVals[i] || 0));
    const cls = "die-face" + (state.rolling ? " rolling" : "");
    facesHTML.push(`<div class="${cls}" data-value="${v}"></div>`);
  }
  facesEl.innerHTML = facesHTML.join("");

  if (state.rolling) {
    dieLabel.textContent = "Rolling…";
  } else if (rawVals.length === 2 && rawVals[0] > 0 && rawVals[1] > 0) {
    dieLabel.textContent =
      `Last roll: ${rawVals[0]} + ${rawVals[1]} = ${rawVals[0] + rawVals[1]}`;
  } else if (rawVals.length >= 1 && rawVals[0] > 0) {
    dieLabel.textContent = "Last roll: " + rawVals[0];
  } else {
    dieLabel.textContent = "Waiting for a roll…";
  }

  const players = Array.isArray(state.players) ? state.players : [];
  scoresEl.innerHTML = players.map((p, idx) => {
    const isMe      = idx === state.myPlayerIdx;
    const isCurrent = idx === state.currentIndex && !p.kicked && !state.gameOver;
    const classes = [];
    if (isMe)      classes.push("me");
    if (isCurrent) classes.push("current");
    if (p.kicked)  classes.push("kicked");
    const label = isMe ? `You (P${p.id})` : `Player ${p.id}`;
    const kickedMark = p.kicked ? " (kicked)" : "";
    const turn = (!p.kicked && p.turnScore > 0)
      ? `<span class="player-turn">turn +${p.turnScore}</span>`
      : "";
    return `<li class="${classes.join(" ")}">`
         +   `<span class="player-label">${label}${kickedMark}</span>`
         +   turn
         +   `<span class="player-score">${p.totalScore} pts</span>`
         + `</li>`;
  }).join("");
}

// Pre-WASM placeholder render so the panel shows player slots immediately
// when the game starts, before the WASM has emitted its first state change.
function _renderStatePanelFromRoster() {
  if (!gameRoster.length) return;
  const numDice = currentVariant === "bigpig" ? 2 : 1;
  _renderStatePanel({
    variant:      currentVariant,
    currentIndex: 0,
    myPlayerIdx:  myGameSlot,
    message:      myGameSlot === 0
      ? "Your turn — press Roll to begin"
      : "Waiting for Player 1 to roll…",
    gameOver:  false,
    winnerID:  0,
    dieValues: new Array(numDice).fill(0),
    rolling:   false,
    players:   gameRoster.map((_, i) => ({
      id:         i + 1,
      totalScore: 0,
      turnScore:  0,
      kicked:     kickedSlots.has(i),
    })),
  });
}

// Called by the WASM whenever the current player changes (also once at game
// start with the initial CurrentIndex).  Tracks the value, resets the AFK
// timer, updates the Roll/Hold enabled-state, and drives the mobile
// fullscreen takeover.
//
// The first call also doubles as our "WASM is ready" signal: at that point
// window.diceGameKick exists, so we can replay any kicks that arrived via
// the catch-up game_start (e.g. a refreshed mid-game rejoin).
let wasmReady = false;
window.diceGameOnTurnChange = function(currentIdx) {
  if (!wasmReady) {
    wasmReady = true;
    if (kickedSlots.size && typeof window.diceGameKick === "function") {
      kickedSlots.forEach((slot) => window.diceGameKick(slot));
    }
  }
  const previous = currentTurnIdx;
  currentTurnIdx = currentIdx;
  turnStartedAt  = Date.now();
  _refreshActionButtons();
  // Re-render chips so the previous timer disappears and the new current
  // chip's timer + (potential) kick button appear immediately.
  if (gameRoster.length) _updateGamePlayerList(gameRoster, myGameSlot);
  // Reset the user-minimised opt-out when the turn comes back to us — they
  // probably want the fullscreen takeover next time even if they dismissed
  // it last turn.
  if (currentIdx === myGameSlot && previous !== myGameSlot) {
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
// Set true when the user explicitly clicks the Fullscreen button — overrides
// the "only auto-enter on my turn" gate so spectators / off-turn players can
// also use the bigger view.  Cleared on game end or when the user exits.
let userRequestedFullscreen = false;

function _refreshFullscreen() {
  const narrow  = window.matchMedia(NARROW_VIEWPORT_QUERY).matches;
  const inGame  = diceGameRunning || diceGameSpectating;
  const autoOn  = diceGameRunning && currentTurnIdx === myGameSlot && !userMinimizedFullscreen;
  const wantFullscreen = inGame && narrow && (autoOn || userRequestedFullscreen);
  document.body.classList.toggle("dice-fullscreen", wantFullscreen);

  // The re-enter button shows whenever fullscreen would be useful (narrow +
  // game running) but isn't currently active — i.e. the user has exited, or
  // it's not their turn and they haven't asked for it yet.
  const enterBtn = document.getElementById("enter-fullscreen-btn");
  if (enterBtn) {
    const showEnter = inGame && narrow && !wantFullscreen;
    enterBtn.style.display = showEnter ? "inline-block" : "none";
  }
}

// Wire the exit + re-enter fullscreen controls and the viewport listener.
(function setupFullscreenControls() {
  const exitBtn  = document.getElementById("exit-fullscreen-btn");
  const enterBtn = document.getElementById("enter-fullscreen-btn");
  if (exitBtn) {
    exitBtn.addEventListener("click", () => {
      userMinimizedFullscreen = true;
      userRequestedFullscreen = false;
      _refreshFullscreen();
    });
  }
  if (enterBtn) {
    enterBtn.addEventListener("click", () => {
      userMinimizedFullscreen = false;
      userRequestedFullscreen = true;
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
      if (!Array.isArray(msg.roster)) break;
      // Ignore game_start while a game is actively running here.  But if the
      // last game ended, or we were spectating a finished round, accept the
      // fresh broadcast — _resetGameState drops the stale state first.
      if (diceGameRunning && !gameEnded) break;
      if (diceGameSpectating || gameEnded) _resetGameState();
      _initDiceGame(msg.roster, msg.from, msg.variant);
      // Replay any kicks the host already applied so a refreshed-mid-game
      // rejoin comes back to the same logical state.
      if (Array.isArray(msg.kicked)) {
        msg.kicked.forEach((slot) => _applyKick(slot, msg.from));
      }
      _ensureChipTick();
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

    case "player_kick":
      if (typeof msg.slot === "number") {
        _applyKick(msg.slot, msg.from);
      }
      break;
  }
}

// Called by video.js whenever the server-assigned slot map changes (either
// the initial "peers" message or a subsequent "player_joined").  Keeps the
// pre-game chip list in sync; once a game starts the roster is frozen so we
// leave the chips alone.
function onSlotsUpdated() {
  const statusEl = document.getElementById("game-status");
  if (statusEl && statusEl.textContent === "Reconnecting…") {
    statusEl.textContent = "";
  }
  if (!diceGameRunning && !diceGameSpectating) {
    _updateGamePlayerList(_playerListBySlot(), mySlot);
  }
}

// Called by video.js when a brand-new peer arrives (player_joined). If we
// are the host of an active game, send them a unicast game_start carrying
// both the frozen roster and the set of already-kicked slots, so a
// returning player rebuilds their WASM with the correct state and a fresh
// joiner drops into spectator mode. Without this catch-up, late joiners
// would think no game was running and could trigger a competing game_start.
function onPlayerJoined(peerID) {
  if (!peerID) return;
  if ((diceGameRunning || diceGameSpectating) && gameHostID === myID && gameRoster.length) {
    sendSignal({
      type:    "game_start",
      from:    myID,
      to:      peerID,
      roomID:  roomID,
      roster:  gameRoster,
      kicked:  Array.from(kickedSlots),
      variant: currentVariant,
    });
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

// Seconds the active player must hold their turn before the host can kick.
const AFK_THRESHOLD_SEC = 60;

// Set on every turn change so the chip render can show "0:XX on turn" and
// gate the host's Kick button on AFK_THRESHOLD_SEC.
let turnStartedAt = 0;

// Interval handle for the once-per-second chip re-render while a game is
// active (so the turn timer + kick enable update visually).
let chipTickHandle = null;

function _formatElapsed(secs) {
  const m = Math.floor(secs / 60);
  const s = secs % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function _updateGamePlayerList(players, myPlayerIdx) {
  const listEl = document.getElementById("game-player-list");
  if (!listEl) return;

  const inGame   = diceGameRunning || diceGameSpectating;
  const amHost   = inGame && gameHostID === myID;
  const elapsed  = turnStartedAt ? Math.max(0, Math.floor((Date.now() - turnStartedAt) / 1000)) : 0;

  listEl.innerHTML = players.map((id, idx) => {
    const isMe      = idx === myPlayerIdx;
    const isKicked  = kickedSlots.has(idx);
    const isCurrent = inGame && idx === currentTurnIdx && !isKicked;

    const classes = ["game-player-chip"];
    if (isMe)      classes.push("me");
    if (isKicked)  classes.push("kicked");
    if (isCurrent) classes.push("current");

    let label = isMe ? `<strong>You (P${idx + 1})</strong>` : `Player ${idx + 1}`;
    if (isKicked) label += " <em>(kicked)</em>";

    let extras = "";
    if (isCurrent) {
      extras += `<span class="turn-timer">${_formatElapsed(elapsed)}</span>`;
    }
    if (amHost && !isMe && !isKicked) {
      // Only enable kick on the player who is currently holding the turn
      // past the AFK threshold — host can't pre-emptively boot.
      const canKick = isCurrent && elapsed >= AFK_THRESHOLD_SEC;
      const title = canKick
        ? "Kick AFK player"
        : `Available after ${AFK_THRESHOLD_SEC}s of inactivity`;
      extras += `<button class="kick-btn" data-slot="${idx}"`
              + `${canKick ? "" : " disabled"} title="${title}">&times;</button>`;
    }

    return `<span class="${classes.join(" ")}">${label}${extras}</span>`;
  }).join("");

  // Re-wire kick buttons after each render.
  listEl.querySelectorAll(".kick-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      const slot = parseInt(btn.dataset.slot, 10);
      if (!isNaN(slot)) _broadcastKick(slot);
    });
  });
}

// Re-render the chip list once a second while a game is active so the turn
// timer ticks and the kick button flips to enabled at the AFK threshold.
function _ensureChipTick() {
  const want = diceGameRunning || diceGameSpectating;
  if (want && !chipTickHandle) {
    chipTickHandle = setInterval(() => {
      if (gameRoster.length) _updateGamePlayerList(gameRoster, myGameSlot);
    }, 1000);
  } else if (!want && chipTickHandle) {
    clearInterval(chipTickHandle);
    chipTickHandle = null;
  }
}

function _broadcastKick(slot) {
  if (gameHostID !== myID) return;     // only host can kick
  if (slot === myGameSlot) return;     // can't self-kick
  sendSignal({
    type:   "player_kick",
    from:   myID,
    to:     "room",
    roomID: roomID,
    slot:   slot,
  });
  _applyKick(slot, myID);
}

function _applyKick(slot, fromID) {
  if (!gameRoster.length) return;
  if (fromID !== gameHostID) return;          // ignore non-host kicks
  if (slot < 0 || slot >= gameRoster.length) return;
  if (kickedSlots.has(slot)) return;          // already applied
  kickedSlots.add(slot);
  // Persist the kick across rounds — the auto-roster builder consults
  // kickedClientIDs so an AFK player stays out of the next New Game.
  const clientID = gameRoster[slot];
  if (clientID && clientID !== myID) kickedClientIDs.add(clientID);
  if (typeof window.diceGameKick === "function") {
    window.diceGameKick(slot);
  }
  _updateGamePlayerList(gameRoster, myGameSlot);
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

  // Firefox Android intercepts shake gestures for its built-in AI page
  // summary feature, so our DeviceMotion listener never fires.  There's no
  // programmatic way for a page to opt out — the user has to disable the
  // gesture in Firefox settings ("Shake to summarize") or use a different
  // browser.  Surface a hint on the button so the user knows the on-screen
  // Roll button is the working alternative on Firefox.
  const isFirefox = /Firefox|FxiOS/i.test(navigator.userAgent || "");
  if (isFirefox && btn) {
    btn.textContent = "Shake-to-roll: blocked by Firefox — use Roll button";
    btn.disabled = true;
    btn.style.display = "inline-block";
    btn.title =
      "Firefox captures the shake gesture for its AI page-summary feature." +
      " Use the on-screen Roll button instead, or disable Firefox's shake-to-summarise.";
    return;
  }

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
