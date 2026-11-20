// dice_game.js
// Handles WASM loading, player coordination, and game-event routing for
// the Pig Dice multiplayer mini-game embedded in a WebRTC room.
//
// Depends on globals set by video.js:  myID, roomID, sendSignal, ws,
//                                      mySlot, playerSlots, playerInfo,
//                                      displayNameFor, setTileGameInfo

// "Running" means a game is in progress in this tab (WASM loaded, we're a
// participant). "Spectating" means a game is in progress in the room but we
// joined after it started — we get chips + status, no WASM, no Roll/Hold.
// "Ended" means the most recent game finished; a New Game click can start
// a fresh round without reloading the WASM.
let diceGameRunning = false;
let diceGameSpectating = false;
let gameEnded = false;

// The roster is captured at game_start time and frozen for the life of that
// game. The starter's broadcast carries the authoritative list. New peers
// joining after that point do not get a slot in this game; they become
// spectators and wait for the next game_start.
let gameRoster = [];     // ordered list of clientIDs participating in the current game
let gameHostID = null;   // clientID of the player who started the game
let gameStartedAt = 0;   // when we last started a game ourselves (start-race tie-break)
let myGameSlot = -1;     // our index within gameRoster, or -1 if spectator
let currentVariant = "pig";    // active game's rules variant ("pig" | "bigpig")
const kickedSlots = new Set(); // game-roster indices that have been removed

// kickedClientIDs persists ACROSS rounds for the lifetime of the page —
// once a player is booted for AFK, they stay out of subsequent automatic
// rosters even though they're still connected. (A page reload clears it, by
// design: it's session-scoped, not durable punishment.)  Players removed for
// being disconnected are not added: if they come back they play next round.
const kickedClientIDs = new Set();

// clientID -> time (ms) we noticed they disconnected, for roster members
// who are currently away.  Cleared when they rejoin.
const disconnectedAt = Object.create(null);

// Snapshot sent by a peer when we (re)join mid-game; applied once the WASM
// is ready to take it.
let pendingSnapshot = null;

// Latest state from the WASM, kept so names / connection changes can
// re-render without waiting for the next game event.
let lastState = null;
let lastLoggedSeq = 0;
const ACTION_LOG_LIMIT = 8;

// Two players clicking Start at nearly the same moment would each adopt the
// other's game; within this window the lower clientID's game wins instead.
const START_RACE_MS = 3000;

// Seconds the active player must hold their turn before the host can kick.
const AFK_THRESHOLD_SEC = 60;

// Seconds a player must be disconnected before others can remove them —
// long enough to ride out a page refresh.
const DISCONNECT_GRACE_SEC = 15;

const BASE_TITLE = document.title;

function _esc(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[c]);
}

function _isConnected(clientID) {
  return clientID === myID || typeof playerSlots[clientID] === "number";
}

function _isAway(clientID) {
  return disconnectedAt[clientID] !== undefined;
}

function _awaySeconds(clientID) {
  const t = disconnectedAt[clientID];
  return t === undefined ? 0 : Math.floor((Date.now() - t) / 1000);
}

// Name for a roster slot: "You" for us, otherwise the peer's display name
// or "Player N".
function _nameForSlot(idx) {
  if (idx === myGameSlot) return "You";
  const name = displayNameFor(gameRoster[idx]);
  return name || `Player ${idx + 1}`;
}

// Build the list of players for a new game: connected clients that have the
// dice game loaded, ordered by server-assigned slot.  Excludes clientIDs in
// kickedClientIDs so that next-round rosters skip players kicked for AFK.
function _playerListBySlot() {
  const out = [];
  Object.keys(playerSlots).forEach((id) => {
    out[playerSlots[id]] = id;
  });
  return out
    .filter((id) => typeof id === "string")
    .filter((id) => id === myID || (playerInfo[id] && playerInfo[id].dice))
    .filter((id) => !kickedClientIDs.has(id));
}

// The player who acts for the game when coordination is needed (AFK kicks,
// catching up a rejoining player): the starter if they're connected,
// otherwise the first connected, un-kicked roster member.  Every client
// computes the same answer from the same membership view.  excludeID lets
// us ignore a peer who is only just rejoining.
function _effectiveHostID(excludeID) {
  if (gameHostID && gameHostID !== excludeID && _isConnected(gameHostID) &&
      !kickedSlots.has(gameRoster.indexOf(gameHostID))) {
    return gameHostID;
  }
  for (let i = 0; i < gameRoster.length; i++) {
    const id = gameRoster[i];
    if (id === excludeID || kickedSlots.has(i)) continue;
    if (_isConnected(id)) return id;
  }
  return null;
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
 * The caller becomes the host: their snapshot of the connected players is
 * frozen as the game roster.  Players already kicked for AFK in a prior round
 * are excluded automatically.
 */
function startDiceGame() {
  if (diceGameRunning || diceGameSpectating) return;
  if (gameEnded) _resetGameState();

  const roster  = _playerListBySlot();
  const variant = _selectedVariant();
  if (roster.length === 0) return;

  // Broadcast roster + variant so every peer agrees on who is playing and
  // which ruleset is in effect.
  sendSignal({
    type:    "game_start",
    from:    myID,
    to:      "room",
    roomID:  roomID,
    roster:  roster,
    variant: variant,
  });

  gameStartedAt = Date.now();
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
  roster.forEach((id) => {
    if (_isConnected(id)) delete disconnectedAt[id];
    else if (disconnectedAt[id] === undefined) disconnectedAt[id] = Date.now();
  });

  // Reflect the variant in the picker (visible to the user, disabled while
  // a game is in progress) so spectators see what's running.
  _syncVariantPicker();

  // Spectator path: joined after game_start, not in the roster.
  if (myGameSlot < 0) {
    diceGameSpectating = true;
    const startBtn = document.getElementById("start-game-btn");
    const statusEl = document.getElementById("game-status");
    if (startBtn) startBtn.style.display = "none";
    if (statusEl) statusEl.textContent = "Game in progress — you'll be in the next round.";
    _updateGamePlayerList(roster, -1);
    _ensureChipTick();
    _syncVariantPicker();
    _refreshFullscreen();
    return;
  }

  diceGameRunning = true;

  // Show the canvas panel + state panel + action buttons. Hide Start, but
  // surface the New Game button so the user has an obvious way to restart
  // mid-round.
  const container = document.getElementById("game-container");
  const statePanel = document.getElementById("game-state-panel");
  const startBtn  = document.getElementById("start-game-btn");
  const newGameBtn = document.getElementById("new-game-btn");
  const actionRow = document.getElementById("game-action-row");
  if (container)  container.style.display  = "block";
  if (statePanel) statePanel.style.display = "flex";
  if (startBtn)   startBtn.style.display   = "none";
  if (newGameBtn) newGameBtn.style.display = "inline-block";
  if (actionRow)  actionRow.classList.add("active");
  _renderStatePanelFromRoster();

  _updateGamePlayerList(roster, myGameSlot);
  _setupActionButtons();
  _ensureChipTick();
  _refreshFullscreen();

  // Called by the Go game whenever the local player takes an action.  Sent
  // to the whole room via the server, so it reaches every player whether or
  // not we have a working video connection to them.
  window.diceGameSendEvent = function(jsonStr) {
    sendSignal({
      type:   "game_event",
      from:   myID,
      to:     "room",
      roomID: roomID,
      event:  JSON.parse(jsonStr),
    });
  };

  // If a previous game already loaded the WASM module, reset it in-place
  // (the Ebiten run loop is still alive). Otherwise instantiate fresh with
  // the configuration the Go runtime reads at startup.
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
  sel.disabled = (diceGameRunning && !gameEnded) || diceGameSpectating;
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
  pendingSnapshot         = null;
  lastState               = null;
  lastLoggedSeq           = 0;
  kickedSlots.clear();
  if (chipTickHandle) { clearInterval(chipTickHandle); chipTickHandle = null; }

  const newGameBtn = document.getElementById("new-game-btn");
  const statusEl   = document.getElementById("game-status");
  const actionRow  = document.getElementById("game-action-row");
  const statePanel = document.getElementById("game-state-panel");
  const enterFsBtn = document.getElementById("enter-fullscreen-btn");
  const logEl      = document.getElementById("game-action-log");
  if (newGameBtn) newGameBtn.style.display = "none";
  if (statusEl)   statusEl.textContent = "";
  if (actionRow)  actionRow.classList.remove("active");
  if (statePanel) statePanel.style.display = "none";
  if (enterFsBtn) enterFsBtn.style.display = "none";
  if (logEl)      logEl.innerHTML = "";
  document.title = BASE_TITLE;
  setTileGameInfo({});
  _syncVariantPicker();
  _refreshFullscreen();
}

// Click handler for the New Game button. Works whether a game is in
// progress (treated as an immediate restart), already ended, or never
// started — clears local state and broadcasts a fresh game_start so peers
// reset too. The button is always visible after the first game starts so
// the user can find it; mid-game clicks abandon the current round.
function startNewGame() {
  if (diceGameRunning && !gameEnded) {
    if (!confirm("Restart the game now? This ends the current round for everyone.")) {
      return;
    }
  }
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
  const myTurn = diceGameRunning && !gameEnded && currentTurnIdx === myGameSlot &&
                 !kickedSlots.has(myGameSlot);
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
  _syncVariantPicker();
  _refreshActionButtons();
  document.title = BASE_TITLE;
  _refreshFullscreen();
};

// Called by the WASM whenever any scoring / turn / message state changes.
window.diceGameOnStateChange = function(jsonStr) {
  let state;
  try { state = JSON.parse(jsonStr); } catch (_) { return; }
  _renderStatePanel(state);
};

// ── State panel: turn banner, dice, scores, action log ───────────────────────

function _formatAction(a) {
  const who  = _nameForSlot(a.playerIdx);
  const dice = Array.isArray(a.dice) ? a.dice.join(" + ") : "";
  switch (a.kind) {
    case "roll":
      return a.note
        ? `${who} rolled ${dice} — ${a.note} +${a.points} (turn ${a.turnTotal})`
        : `${who} rolled ${dice}: +${a.points} (turn ${a.turnTotal})`;
    case "bust":
      return `${who} rolled ${dice} — BUST, lost ${a.points}`;
    case "hold":
      return `${who} held +${a.points} → ${a.total}`;
    case "win":
      return `${who} held +${a.points} → ${a.total} and won!`;
    case "kick":
      return `${who} ${a.playerIdx === myGameSlot ? "were" : "was"} removed from the game`;
  }
  return "";
}

function _renderBanner(state) {
  const banner = document.getElementById("game-state-message");
  const title  = document.getElementById("turn-banner-title");
  const detail = document.getElementById("turn-banner-detail");
  if (!banner || !title || !detail) return;

  const players = Array.isArray(state.players) ? state.players : [];
  const cur     = players[state.currentIndex] || { turnScore: 0 };
  const mine    = !state.gameOver && state.currentIndex === myGameSlot;
  banner.classList.toggle("mine", mine);
  banner.classList.toggle("game-over", !!state.gameOver);

  if (state.gameOver) {
    const winnerIdx = state.winnerID - 1;
    const winner = players[winnerIdx];
    if (winnerIdx < 0 || !winner) {
      title.textContent  = "Game over";
      detail.textContent = "Every player was removed.";
    } else {
      title.textContent  = winnerIdx === myGameSlot ? "🏆 You win!" : `🏆 ${_nameForSlot(winnerIdx)} wins!`;
      detail.textContent = `${winner.totalScore} points. Press New Game to play again.`;
    }
    return;
  }

  if (mine) {
    title.textContent = "YOUR TURN";
    if (state.rolling)           detail.textContent = "Rolling…";
    else if (cur.turnScore > 0)  detail.textContent = `${cur.turnScore} points at risk — Roll again or Hold`;
    else                         detail.textContent = "Roll to start your turn";
    return;
  }

  title.textContent = `${_nameForSlot(state.currentIndex)}'s turn`;
  if (_isAway(gameRoster[state.currentIndex])) {
    detail.textContent = "Disconnected — waiting for them to come back";
  } else if (state.rolling) {
    detail.textContent = "Rolling…";
  } else if (cur.turnScore > 0) {
    detail.textContent = `${cur.turnScore} points at risk`;
  } else {
    detail.textContent = "Waiting for them to roll";
  }
}

function _renderDice(state) {
  const facesEl  = document.getElementById("game-state-die-faces");
  const dieLabel = document.getElementById("game-state-die-label");
  if (!facesEl || !dieLabel) return;

  // Render one die face per dieValues entry.  For BigPig that's two faces;
  // for Pig it's one. While rolling, faces show the dice emoji.
  const rawVals = Array.isArray(state.dieValues) ? state.dieValues : [];
  const slots = Math.max(1, rawVals.length);
  const facesHTML = [];
  for (let i = 0; i < slots; i++) {
    const v = state.rolling ? 0 : Math.max(0, Math.min(6, rawVals[i] || 0));
    const cls = "die-face" + (state.rolling ? " rolling" : "");
    facesHTML.push(`<div class="${cls}" data-value="${v}"></div>`);
  }
  facesEl.innerHTML = facesHTML.join("");

  const a = state.lastAction || {};
  if (state.rolling) {
    dieLabel.textContent = "Rolling…";
  } else if (a.seq && (a.kind === "roll" || a.kind === "bust")) {
    dieLabel.textContent = `${_nameForSlot(a.playerIdx)}: ${(a.dice || []).join(" + ")}`;
  } else {
    dieLabel.textContent = "Waiting for a roll…";
  }
}

function _renderScores(state) {
  const scoresEl = document.getElementById("game-state-scores");
  if (!scoresEl) return;
  const goal = state.winningScore || 100;
  const players = Array.isArray(state.players) ? state.players : [];
  scoresEl.innerHTML = players.map((p, idx) => {
    const isMe      = idx === myGameSlot;
    const isCurrent = idx === state.currentIndex && !p.kicked && !state.gameOver;
    const away      = !p.kicked && _isAway(gameRoster[idx]);
    const classes = [];
    if (isMe)      classes.push("me");
    if (isCurrent) classes.push("current");
    if (p.kicked)  classes.push("kicked");
    if (away)      classes.push("away");
    const label = isMe
      ? (displayNameFor(myID) ? `You (${_esc(displayNameFor(myID))})` : "You")
      : _esc(_nameForSlot(idx));
    const tag = p.kicked ? " (removed)"
              : away     ? ` <span class="player-away">disconnected</span>`
              : "";
    const turn = (!p.kicked && p.turnScore > 0)
      ? `<span class="player-turn">+${p.turnScore} at risk</span>`
      : "";
    const banked = Math.min(100, (p.totalScore / goal) * 100);
    const risk   = Math.min(100 - banked, (p.turnScore / goal) * 100);
    return `<li class="${classes.join(" ")}">`
         +   `<span class="player-label">${label}${tag}</span>`
         +   turn
         +   `<span class="player-score">${p.totalScore} / ${goal}</span>`
         +   `<span class="score-bar"><span class="banked" style="width:${banked}%"></span>`
         +     `<span class="at-risk" style="width:${risk}%"></span></span>`
         + `</li>`;
  }).join("");
}

function _flashBust(a) {
  const el = document.getElementById("bust-flash");
  if (!el) return;
  el.textContent = a.playerIdx === myGameSlot
    ? (a.points > 0 ? `BUST! You lost ${a.points}` : "BUST!")
    : `${_nameForSlot(a.playerIdx)} BUST${a.points > 0 ? ` −${a.points}` : ""}`;
  el.classList.remove("show");
  void el.offsetWidth; // restart the animation
  el.classList.add("show");
}

function _logAction(state) {
  const a = state.lastAction;
  if (!a || !a.seq || a.seq <= lastLoggedSeq) return;
  const jumped = a.seq > lastLoggedSeq + 1 && lastLoggedSeq > 0;
  lastLoggedSeq = a.seq;

  const logEl = document.getElementById("game-action-log");
  if (logEl) {
    const li = document.createElement("li");
    li.textContent = _formatAction(a);
    if (a.kind === "bust") li.className = "bust";
    if (a.kind === "win")  li.className = "win";
    logEl.prepend(li);
    while (logEl.children.length > ACTION_LOG_LIMIT) logEl.lastChild.remove();
  }
  // Only flash live busts, not ones we skipped past when catching up.
  if (a.kind === "bust" && !jumped) _flashBust(a);
}

// Score badges and turn highlight on the video tiles.
function _renderTiles(state) {
  const info = {};
  const players = state && Array.isArray(state.players) ? state.players : [];
  players.forEach((p, idx) => {
    const id = gameRoster[idx];
    if (!id) return;
    const turnBit = !p.kicked && p.turnScore > 0 ? ` (+${p.turnScore})` : "";
    info[id] = {
      badge: p.kicked ? "out" : `${p.totalScore}${turnBit}`,
      turn:  !state.gameOver && idx === state.currentIndex && !p.kicked,
      out:   p.kicked,
    };
  });
  setTileGameInfo(info);
}

function _renderStatePanel(state) {
  const panel = document.getElementById("game-state-panel");
  if (!panel) return;
  if (panel.style.display === "none" || panel.style.display === "") {
    panel.style.display = "flex";
  }
  lastState = state;
  _renderBanner(state);
  _renderDice(state);
  _renderScores(state);
  _logAction(state);
  _renderTiles(state);
  // A snapshot loaded before the game-over callback was wired up.
  if (state.gameOver && !gameEnded) window.diceGameOnGameOver(state.winnerID - 1);
}

function _rerender() {
  if (lastState) _renderStatePanel(lastState);
  if (gameRoster.length) _updateGamePlayerList(gameRoster, myGameSlot);
  else _updateGamePlayerList(_playerListBySlot(), _playerListBySlot().indexOf(myID));
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
    gameOver:     false,
    winnerID:     0,
    winningScore: 100,
    dieValues:    new Array(numDice).fill(0),
    rolling:      false,
    lastAction:   { seq: 0 },
    players:      gameRoster.map((_, i) => ({
      id:         i + 1,
      totalScore: 0,
      turnScore:  0,
      kicked:     kickedSlots.has(i),
    })),
  });
}

// Apply a snapshot from a peer if the WASM is ready for it.
function _applyPendingSnapshot() {
  if (!pendingSnapshot || typeof window.diceGameLoadState !== "function") return;
  const snap = pendingSnapshot;
  pendingSnapshot = null;
  window.diceGameLoadState(snap);
}

// Called by the WASM whenever the current player changes (also once at game
// start with the initial CurrentIndex).  Tracks the value, resets the AFK
// timer, updates the Roll/Hold enabled-state, and drives the turn alerts and
// the mobile fullscreen takeover.
//
// The first call also doubles as our "WASM is ready" signal: at that point
// window.diceGameLoadState and window.diceGameKick exist, so we can apply
// the live state a peer sent us (a refreshed mid-game rejoin).
let wasmReady = false;
window.diceGameOnTurnChange = function(currentIdx) {
  if (!wasmReady) {
    wasmReady = true;
    _applyPendingSnapshot();
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

  const myTurnNow = currentIdx === myGameSlot && !gameEnded;
  if (myTurnNow && previous !== myGameSlot) {
    // Reset the user-minimised opt-out — they probably want the fullscreen
    // takeover this turn even if they dismissed it last turn.
    userMinimizedFullscreen = false;
    if (navigator.vibrate) navigator.vibrate([120, 60, 120]);
  }
  document.title = myTurnNow ? "🎲 Your turn! — " + BASE_TITLE : BASE_TITLE;
  _refreshFullscreen();
};

// ── Mobile fullscreen takeover ───────────────────────────────────────────────
//
// When it's the local player's turn on a narrow viewport, take over the
// screen so the state panel + Roll/Hold buttons are big and thumb-reachable;
// the video grid collapses to a PIP overlay. The user can opt out for the
// rest of the current turn via the exit button.

const NARROW_VIEWPORT_QUERY = "(max-width: 768px)";
let userMinimizedFullscreen = false;
// Set true when the user explicitly clicks the Fullscreen button — overrides
// the "only auto-enter on my turn" gate so spectators / off-turn players can
// also use the bigger view.  Cleared on game end or when the user exits.
let userRequestedFullscreen = false;

function _refreshFullscreen() {
  const narrow  = window.matchMedia(NARROW_VIEWPORT_QUERY).matches;
  const inGame  = diceGameRunning || diceGameSpectating;
  const autoOn  = diceGameRunning && !gameEnded && currentTurnIdx === myGameSlot && !userMinimizedFullscreen;
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

    // Ebiten v2 in WASM mode creates its own <canvas> in init() and
    // appendChild's it to document.body — we don't get to pick the target.
    // It also overwrites body { background:#000; height:100%; margin:0 }
    // and html { height:100%; margin:0 } which clobbers the rest of the
    // page styling. Intercept those side effects:
    _captureEbitenCanvas();

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

// Watch document.body for the canvas Ebiten injects at WASM startup, move
// it into #game-container, and undo the document/body style clobbering
// Ebiten does as part of its native-window setup. Runs once per session
// (WASM module is loaded only on the first game start).
function _captureEbitenCanvas() {
  if (window._ebitenCanvasCaptured) return;
  window._ebitenCanvasCaptured = true;

  const container = document.getElementById("game-container");
  if (!container) return;

  // Note any canvases that exist before WASM runs so we can distinguish
  // Ebiten's new one from anything we already had on the page.
  const preExisting = new Set(document.querySelectorAll("canvas"));

  const reparent = (canvas) => {
    container.appendChild(canvas);
    // Strip the inline width/height Ebiten set so our CSS wins.
    canvas.style.width  = "";
    canvas.style.height = "";
    canvas.style.margin = "";
    canvas.style.padding = "";
    _restoreBodyStyles();
  };

  // Some canvases might already be appended in the same tick the WASM
  // module's init runs; check synchronously first.
  document.querySelectorAll("body > canvas").forEach((c) => {
    if (!preExisting.has(c)) reparent(c);
  });

  const obs = new MutationObserver((muts) => {
    for (const m of muts) {
      for (const node of m.addedNodes) {
        if (node.nodeType === 1 && node.tagName === "CANVAS" &&
            !preExisting.has(node) && node.parentNode === document.body) {
          reparent(node);
        }
      }
    }
    // Body styles can be set after the canvas append too — keep restoring
    // for a short while after the canvas appears.
    _restoreBodyStyles();
  });
  obs.observe(document.body, { childList: true });

  // Ebiten may set body styles in a few separate frames during init; keep
  // resetting for ~2s after WASM load so we win the last write.
  let resetCount = 0;
  const ticker = setInterval(() => {
    _restoreBodyStyles();
    if (++resetCount > 30) {
      clearInterval(ticker);
      // Stop observing after Ebiten is fully settled.
      obs.disconnect();
    }
  }, 67);
}

// Reset the body/html style fields Ebiten's UI init writes. We blank the
// values so the cascade re-applies the page's app.css rules instead.
function _restoreBodyStyles() {
  const b = document.body.style;
  if (b.backgroundColor === "rgb(0, 0, 0)" || b.backgroundColor === "#000" ||
      b.backgroundColor === "black") {
    b.backgroundColor = "";
  }
  if (b.height === "100%") b.height = "";
  if (b.margin === "0px" || b.margin === "0") b.margin = "";
  if (b.padding === "0px" || b.padding === "0") b.padding = "";
  const h = document.documentElement.style;
  if (h.height === "100%") h.height = "";
  if (h.margin === "0px" || h.margin === "0") h.margin = "";
  if (h.padding === "0px" || h.padding === "0") h.padding = "";
}

// ── Incoming WebSocket message handler ───────────────────────────────────────

/**
 * Called from video.js ws.onmessage for game-related message types.
 */
function handleDiceGameMessage(msg) {
  switch (msg.type) {
    case "game_start": {
      if (!Array.isArray(msg.roster)) break;
      // A unicast game_start is a catch-up for us after (re)joining; a room
      // broadcast is someone starting a new game.
      const catchUp = msg.to === myID;

      // Simultaneous Start clicks: keep the game of the lower clientID so
      // both sides converge on the same game and host.
      if (!catchUp && diceGameRunning && gameHostID === myID &&
          Date.now() - gameStartedAt < START_RACE_MS && msg.from > myID) {
        break;
      }

      // Accept any other game_start — it's a unilateral restart. If we were
      // already running we drop our local state first.
      if (diceGameRunning || diceGameSpectating || gameEnded) {
        _resetGameState();
      }
      _initDiceGame(msg.roster, catchUp && msg.host ? msg.host : msg.from, msg.variant);

      // Replay removals already applied so a rejoin comes back to the same
      // logical state, then adopt the live scores / turn.
      if (Array.isArray(msg.kicked)) {
        msg.kicked.forEach((slot) => _markKicked(slot, null));
      }
      if (catchUp && typeof msg.state === "string" && diceGameRunning) {
        pendingSnapshot = msg.state;
        if (wasmReady) _applyPendingSnapshot();
      }
      _ensureChipTick();
      break;
    }

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
        _applyKick(msg.slot, msg.from, msg.reason);
      }
      break;
  }
}

// Called by video.js whenever the slot map changes ("peers", "player_joined"
// or "player_left").  Keeps the pre-game chip list in sync and notices roster
// members who left while our own connection was down.
function onSlotsUpdated() {
  const statusEl = document.getElementById("game-status");
  if (statusEl && statusEl.textContent === "Reconnecting…") {
    statusEl.textContent = "";
  }
  const online = ws && ws.readyState === WebSocket.OPEN;
  gameRoster.forEach((id) => {
    if (_isConnected(id)) delete disconnectedAt[id];
    else if (online && disconnectedAt[id] === undefined) disconnectedAt[id] = Date.now();
  });
  _rerender();
}

// Called by video.js when a peer's display name / capabilities change.
function onPlayerInfoUpdated() {
  _rerender();
}

// Called by video.js when a video tile is added, so it picks up its badge.
function onTilesChanged() {
  if (lastState) _renderTiles(lastState);
}

// Called by video.js when a peer arrives (player_joined). If a game is
// running and we're the one coordinating it, send them a unicast game_start
// carrying the frozen roster, removals and live state, so a returning player
// resumes the actual game and a fresh joiner drops into spectator mode.
// Without this catch-up, late joiners would think no game was running and
// could trigger a competing game_start.
function onPlayerJoined(peerID) {
  if (!peerID) return;
  delete disconnectedAt[peerID];
  const statusEl = document.getElementById("game-status");
  if (statusEl && statusEl.dataset.awayFor === peerID) {
    statusEl.textContent = "";
    delete statusEl.dataset.awayFor;
  }
  if (!(diceGameRunning || diceGameSpectating) || !gameRoster.length) return;
  if (_effectiveHostID(peerID) !== myID) return;

  const msg = {
    type:    "game_start",
    from:    myID,
    to:      peerID,
    roomID:  roomID,
    roster:  gameRoster,
    host:    gameHostID,
    kicked:  Array.from(kickedSlots),
    variant: currentVariant,
  };
  if (diceGameRunning && typeof window.diceGameGetState === "function") {
    msg.state = window.diceGameGetState();
  }
  sendSignal(msg);
}

// Called by video.js when a peer disconnects. Their chip shows them as away;
// after DISCONNECT_GRACE_SEC any player can remove them from the game.
function onPlayerLeft(peerID) {
  const slot = gameRoster.indexOf(peerID);
  if (slot < 0 || kickedSlots.has(slot)) return;
  disconnectedAt[peerID] = Date.now();
  const statusEl = document.getElementById("game-status");
  if (statusEl && diceGameRunning && !gameEnded) {
    statusEl.textContent = `${_nameForSlot(slot)} disconnected — you can remove them ` +
                           `if they're not back in ${DISCONNECT_GRACE_SEC}s.`;
    statusEl.dataset.awayFor = peerID;
  }
}

// Called by video.js when the local WebSocket drops (network blip, server
// restart, etc.). The reconnect runs automatically in the background.
function onWSDisconnected() {
  const statusEl = document.getElementById("game-status");
  if (statusEl) statusEl.textContent = "Reconnecting…";
}

// ── Player chips ─────────────────────────────────────────────────────────────

// Set on every turn change so the chip render can show "0:XX on turn" and
// gate the host's Kick button on AFK_THRESHOLD_SEC.
let turnStartedAt = 0;

// Interval handle for the once-per-second chip re-render while a game is
// active (so the turn timer + remove buttons update visually).
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
  const live     = diceGameRunning && !gameEnded && !kickedSlots.has(myGameSlot);
  const amHost   = live && _effectiveHostID() === myID;
  const elapsed  = turnStartedAt ? Math.max(0, Math.floor((Date.now() - turnStartedAt) / 1000)) : 0;

  listEl.innerHTML = players.map((id, idx) => {
    const isMe      = idx === myPlayerIdx;
    const isKicked  = inGame && kickedSlots.has(idx);
    const isCurrent = inGame && idx === currentTurnIdx && !isKicked;
    const away      = inGame && !isKicked && _isAway(id);

    const classes = ["game-player-chip"];
    if (isMe)      classes.push("me");
    if (isKicked)  classes.push("kicked");
    if (isCurrent) classes.push("current");
    if (away)      classes.push("away");

    const name = displayNameFor(id);
    let label = isMe
      ? `<strong>You${name ? ` (${_esc(name)})` : ""}</strong>`
      : _esc(name || `Player ${idx + 1}`);
    if (isKicked) label += " <em>(removed)</em>";
    if (away)     label += " <em>disconnected</em>";

    let extras = "";
    if (isCurrent && !gameEnded) {
      extras += `<span class="turn-timer">${_formatElapsed(elapsed)}</span>`;
    }
    if (live && !isMe && !isKicked) {
      if (away) {
        // Anyone still playing can remove a player who has been gone a while.
        const wait = DISCONNECT_GRACE_SEC - _awaySeconds(id);
        extras += wait > 0
          ? `<button class="remove-btn" disabled title="They may be refreshing">Remove in ${wait}s</button>`
          : `<button class="remove-btn" data-slot="${idx}" data-reason="disconnected">Remove</button>`;
      } else if (amHost) {
        // AFK: only on the player holding the turn past the threshold.
        const canKick = isCurrent && elapsed >= AFK_THRESHOLD_SEC;
        const title = canKick
          ? "Remove AFK player"
          : `Available once it's their turn and they've been idle ${AFK_THRESHOLD_SEC}s`;
        extras += `<button class="kick-btn" data-slot="${idx}" data-reason="afk"`
                + `${canKick ? "" : " disabled"} title="${title}">&times;</button>`;
      }
    }

    return `<span class="${classes.join(" ")}">${label}${extras}</span>`;
  }).join("");

  // Re-wire remove buttons after each render.
  listEl.querySelectorAll("button[data-slot]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const slot = parseInt(btn.dataset.slot, 10);
      if (!isNaN(slot)) _broadcastKick(slot, btn.dataset.reason);
    });
  });
}

// Re-render once a second while a game is active so the turn timer ticks
// and the remove buttons enable on time.
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

function _broadcastKick(slot, reason) {
  if (slot === myGameSlot || myGameSlot < 0) return;
  const name = _nameForSlot(slot);
  if (!confirm(`Remove ${name} from this game?`)) return;
  sendSignal({
    type:   "player_kick",
    from:   myID,
    to:     "room",
    roomID: roomID,
    slot:   slot,
    reason: reason === "afk" ? "afk" : "disconnected",
  });
  _applyKick(slot, myID, reason);
}

// Applies a removal requested by a player in this game.  Who may request
// one is enforced in the UI; every client accepts a request from any roster
// member so that all clients apply the same removals and stay in sync.
function _applyKick(slot, fromID, reason) {
  if (!gameRoster.length || !gameRoster.includes(fromID)) return;
  _markKicked(slot, reason);
}

function _markKicked(slot, reason) {
  if (slot < 0 || slot >= gameRoster.length) return;
  if (kickedSlots.has(slot)) return;          // already applied
  kickedSlots.add(slot);
  // AFK kicks persist across rounds so the player stays out of the next New
  // Game; disconnect removals don't — if they come back they play again.
  const clientID = gameRoster[slot];
  if (reason === "afk" && clientID && clientID !== myID) kickedClientIDs.add(clientID);
  if (typeof window.diceGameKick === "function") {
    window.diceGameKick(slot);
  }
  _refreshActionButtons();
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

  // Firefox Android uses shake gestures for its own "shake to summarize"
  // feature, and a page has no way to ask it to give the gesture up.  We
  // still listen (so shake-to-roll works once the user turns that feature
  // off) and show a one-time, dismissible note explaining how.
  const isFirefox = /Firefox|FxiOS/i.test(navigator.userAgent || "");
  const touch = window.matchMedia && window.matchMedia("(pointer: coarse)").matches;
  const HINT_DISMISSED_KEY = "pig.shakeHintDismissed";
  if (isFirefox && touch) {
    const hint = document.getElementById("shake-hint");
    let dismissed = false;
    try { dismissed = window.localStorage.getItem(HINT_DISMISSED_KEY) === "1"; } catch (_) {}
    if (hint && !dismissed) {
      hint.hidden = false;
      const close = document.getElementById("shake-hint-dismiss");
      if (close) close.addEventListener("click", () => {
        hint.hidden = true;
        try { window.localStorage.setItem(HINT_DISMISSED_KEY, "1"); } catch (_) {}
      });
    }
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

// Paint the pre-game chip list once on load.
_rerender();
