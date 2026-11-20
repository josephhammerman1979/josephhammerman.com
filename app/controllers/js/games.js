// games.js
// Client shell for the server-run dice games.  The server (app/games) owns
// all game state; this file sends player actions and renders the views it
// receives: the game picker, player chips, turn banner, log, and the board,
// which is drawn by a per-game renderer.
//
// Depends on globals from video.js: myID, roomID, sendSignal, playerSlots,
// playerInfo, displayNameFor, setTileGameInfo.
//
// ── Writing a game renderer ──────────────────────────────────────────────────
//
// Each game registers a renderer (see js/games/pig.js for a full example):
//
//   GameUI.register("pig", {
//     board(state, ui) { return "<html for the game area>"; },
//   });
//
// `state` is the game's View(seat) from the server.  `ui` has:
//   ui.mySeat              my seat, or -1 when spectating
//   ui.view                the whole table view (roster, waiting, over, ...)
//   ui.waitingOnMe         the game is waiting for my input
//   ui.isWaiting(seat)     the game is waiting for that seat
//   ui.name(seat)          escaped display name ("You" for me)
//   ui.text(s)             escape s and replace {P<n>} tokens with names
//   ui.esc(s)              HTML-escape
//   ui.fresh               dice were rolled since the last render (animate them)
//   ui.selected            Set of die indices the player has tapped (see below)
//   ui.die(face, opts)     one die; opts: {fresh, selectable, index, selected,
//                          kept, faded, hidden, empty, small, big, label,
//                          cls (extra class, e.g. to mark scoring dice)}
//   ui.dice(faces, opts)   a row of dice; with opts.selectable each die gets
//                          its index so taps toggle ui.selected
//   ui.button(label, action, opts)
//                          a button that sends `action` (an object) when
//                          clicked; opts: {primary, roll, disabled,
//                          withSelection, cls, title}.  withSelection adds
//                          `dice: [selected indices]` to the action.  Mark the
//                          main roll button with roll:true so shake-to-roll
//                          can press it.
//   ui.scoreTable(headers, rows)
//                          a table; rows are [{seat, cells: [html...]}]; the
//                          first column is the player's name, highlighted
//                          when the game is waiting on them.
//
// A renderer may also define onSelect(index, selected, state), called after
// a die tap toggles `index` in the `selected` Set, to adjust the selection
// (e.g. Octo keeps it to one number).
//
// Forms: <form data-act-form='{"type":"bid"}'> sends that action merged with
// the form's named inputs on submit.  Inputs with type="number" or a
// data-num attribute become numbers; checkboxes become booleans; inputs named
// dice0, dice1, ... are collected into action.dice.  Pre-fill forms with a
// valid default so a player can usually just press the button.
//
// Renderers return HTML strings; escape anything from players (names, text)
// with ui.esc / ui.text.  Style with classes from css/games.css (.board-*,
// .die, .btn, .scoreboard, .pill, .note) or the renderer's own CSS file.

const GameUI = {
  renderers: Object.create(null),
  register(id, renderer) { this.renderers[id] = renderer; },
};

let gameCatalog = [];      // [{id, name, summary, rules, min, max, options}]
let gameView = null;       // latest game_state from the server, or null
let gameViewAt = 0;        // when it arrived (ms), to count away/idle time up
let lastRollSeq = -1;
let lastGameNo = -1;
let lastLogSeq = 0;
let wasWaitingOnMe = false;
const uiSelected = new Set();

const BASE_TITLE = document.title;
const NARROW_VIEWPORT_QUERY = "(max-width: 768px)";
let userMinimizedFullscreen = false;
let userRequestedFullscreen = false;

// ── Small helpers ────────────────────────────────────────────────────────────

function _esc(s) {
  return String(s == null ? "" : s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[c]);
}

function _rawName(seat) {
  if (!gameView || seat < 0 || seat >= gameView.roster.length) return "?";
  if (seat === gameView.mySeat) return "You";
  return displayNameFor(gameView.roster[seat]) || `Player ${seat + 1}`;
}

function _nameList(seats) {
  const names = seats.map(_rawName);
  if (names.length <= 1) return names.join("");
  return names.slice(0, -1).join(", ") + " and " + names[names.length - 1];
}

// Replace {P<n>} tokens with player names.  The viewer is "You", so their
// possessive ("{P0}'s turn") becomes "Your"/"your".  Game text should use
// past tense ("{P1} rolled") so "You" reads naturally.
function _names(s, wrap) {
  return s.replace(/\{P(\d+)\}('s)?/g, (_, n, poss, offset) => {
    const seat = parseInt(n, 10);
    if (gameView && seat === gameView.mySeat) {
      if (poss) return offset === 0 ? "Your" : "your";
      return offset === 0 ? "You" : "you";
    }
    return wrap(_rawName(seat)) + (poss || "");
  });
}

// Escape text and replace {P<n>} tokens with player names.
function _text(s) { return _names(_esc(s).replace(/&#39;/g, "'"), _esc); }

function _plainText(s) { return _names(String(s == null ? "" : s), (x) => x); }

const PIPS = {
  1: ["mc"], 2: ["tl", "br"], 3: ["tl", "mc", "br"], 4: ["tl", "tr", "bl", "br"],
  5: ["tl", "tr", "mc", "bl", "br"], 6: ["tl", "tr", "ml", "mr", "bl", "br"],
};

function _die(face, opts) {
  opts = opts || {};
  const cls = ["die"];
  if (opts.small) cls.push("small");
  if (opts.big) cls.push("big");
  if (opts.kept) cls.push("kept");
  if (opts.fresh) cls.push("fresh");
  if (opts.faded) cls.push("faded");
  if (opts.cls) cls.push(opts.cls);
  let attrs = "";
  if (opts.selectable) {
    cls.push("selectable");
    const sel = opts.selected !== undefined ? opts.selected : uiSelected.has(opts.index);
    if (sel) cls.push("selected");
    attrs += ` data-sel="${opts.index}" role="button" tabindex="0" aria-pressed="${sel}"`;
  }
  if (opts.label) attrs += ` title="${_esc(opts.label)}"`;
  if (opts.empty) return `<span class="${cls.concat("empty").join(" ")}"${attrs}></span>`;
  if (opts.hidden || !(face >= 1 && face <= 6)) {
    return `<span class="${cls.concat("hidden").join(" ")}"${attrs} aria-label="hidden die">?</span>`;
  }
  const pips = PIPS[face].map((p) => `<i class="pip p-${p}"></i>`).join("");
  return `<span class="${cls.join(" ")}"${attrs} aria-label="${face}">${pips}</span>`;
}

function _dice(faces, opts) {
  opts = opts || {};
  return `<span class="dice-row">` + (faces || []).map((f, i) =>
    _die(f, Object.assign({}, opts, { index: i }))).join("") + `</span>`;
}

function _button(label, action, opts) {
  opts = opts || {};
  const cls = ["btn"];
  if (opts.primary) cls.push("primary");
  if (opts.cls) cls.push(opts.cls);
  let attrs = ` data-act='${_esc(JSON.stringify(action))}'`;
  if (opts.withSelection) attrs += " data-with-sel";
  if (opts.roll) attrs += " data-roll";
  if (opts.disabled) attrs += " disabled";
  if (opts.title) attrs += ` title="${_esc(opts.title)}"`;
  return `<button type="button" class="${cls.join(" ")}"${attrs}>${label}</button>`;
}

function _scoreTable(headers, rows) {
  const head = `<tr><th>Player</th>${headers.map((h) => `<th>${h}</th>`).join("")}</tr>`;
  const body = rows.map((r) => {
    const cls = [];
    if (r.seat === (gameView && gameView.mySeat)) cls.push("me");
    if (gameView && gameView.waiting.includes(r.seat) && !gameView.over) cls.push("current");
    if (gameView && gameView.removed[r.seat]) cls.push("removed");
    return `<tr class="${cls.join(" ")}"><td class="who">${_esc(_rawName(r.seat))}</td>` +
      r.cells.map((c) => `<td>${c}</td>`).join("") + `</tr>`;
  }).join("");
  return `<table class="scoreboard">${head}${body}</table>`;
}

function _uiFor(view) {
  return {
    mySeat: view.mySeat,
    view: view,
    waitingOnMe: view.mySeat >= 0 && !view.over && view.waiting.includes(view.mySeat),
    isWaiting: (seat) => !view.over && view.waiting.includes(seat),
    name: (seat) => _esc(_rawName(seat)),
    text: _text,
    esc: _esc,
    fresh: view.rollSeq !== lastRollSeq,
    selected: uiSelected,
    die: _die,
    dice: _dice,
    button: _button,
    scoreTable: _scoreTable,
  };
}

// ── Sending ──────────────────────────────────────────────────────────────────

function _sendToServer(msg) {
  sendSignal(Object.assign({ from: myID, to: "server", roomID: roomID }, msg));
}

function sendGameAction(action) {
  _sendToServer({ type: "game_action", action: action });
  uiSelected.clear();
}

let statusTimer = null;
function _setStatus(text, isError) {
  const el = document.getElementById("game-status");
  if (!el) return;
  el.textContent = text || "";
  el.classList.toggle("error", !!isError);
  if (statusTimer) clearTimeout(statusTimer);
  if (isError) statusTimer = setTimeout(() => { el.textContent = ""; el.classList.remove("error"); }, 5000);
}

// ── Game picker and options ──────────────────────────────────────────────────

function _selectedDef() {
  const sel = document.getElementById("game-picker");
  return gameCatalog.find((d) => d.id === (sel && sel.value)) || gameCatalog[0];
}

function _renderPicker() {
  const sel = document.getElementById("game-picker");
  if (!sel || !gameCatalog.length) return;
  if (!sel.options.length) {
    sel.innerHTML = gameCatalog.map((d) =>
      `<option value="${_esc(d.id)}">${_esc(d.name)} (${d.min}–${d.max})</option>`).join("");
  }
  _renderOptions();
  _renderRules();
}

function _renderOptions() {
  const box = document.getElementById("game-options");
  const def = _selectedDef();
  if (!box || !def) return;
  box.innerHTML = (def.options || []).map((o) =>
    `<label class="game-option">${_esc(o.label)} <select data-opt="${_esc(o.key)}">` +
    o.choices.map((c) => `<option value="${_esc(c.value)}">${_esc(c.label)}</option>`).join("") +
    `</select></label>`).join("");
}

function _renderRules() {
  // While a game runs, show its rules; otherwise the picker's selection.
  const def = (gameView && !gameView.over && gameCatalog.find((d) => d.id === gameView.gameId)) || _selectedDef();
  if (!def) return;
  const nameEl = document.getElementById("game-rules-name");
  const sumEl  = document.getElementById("game-rules-summary");
  const listEl = document.getElementById("game-rules-list");
  if (nameEl) nameEl.textContent = def.name;
  if (sumEl)  sumEl.textContent = def.summary || "";
  if (listEl) listEl.innerHTML = (def.rules || []).map((r) => `<li>${_esc(r)}</li>`).join("");
  const pageEl = document.getElementById("game-rules-page");
  if (pageEl) pageEl.href = "/games/rules/" + encodeURIComponent(def.id);
  const docEl = document.getElementById("game-rules-doc");
  if (docEl) {
    docEl.hidden = !def.rulesUrl;
    if (def.rulesUrl) docEl.href = def.rulesUrl;
  }
}

function _startGame() {
  const def = _selectedDef();
  if (!def) return;
  if (gameView && !gameView.over && gameView.mySeat >= 0 &&
      !confirm("Start a new game now? This ends the current game for everyone.")) {
    return;
  }
  const options = {};
  document.querySelectorAll("#game-options [data-opt]").forEach((el) => { options[el.dataset.opt] = el.value; });
  _sendToServer({ type: "game_new", gameId: def.id, options: options });
}

(function setupHeader() {
  const sel = document.getElementById("game-picker");
  if (sel) sel.addEventListener("change", () => { _renderOptions(); _renderRules(); });
  const startBtn = document.getElementById("start-game-btn");
  if (startBtn) startBtn.addEventListener("click", _startGame);
})();

// ── Rendering a game state ───────────────────────────────────────────────────

function _elapsed() { return Math.floor((Date.now() - gameViewAt) / 1000); }

function _canRemove(seat) {
  const v = gameView;
  if (!v || v.over || v.mySeat < 0 || v.removed[v.mySeat] || seat === v.mySeat || v.removed[seat]) return false;
  if (!v.connected[seat]) return v.awaySeconds[seat] + _elapsed() >= v.graceSeconds;
  return v.waiting.includes(seat) && v.idleSeconds + _elapsed() >= v.idleLimit;
}

function _renderChips() {
  const listEl = document.getElementById("game-player-list");
  if (!listEl) return;
  const v = gameView;

  if (!v || v.over) {
    // Who would play if a game started now.
    const ids = [];
    Object.keys(playerSlots).forEach((id) => { ids[playerSlots[id]] = id; });
    const players = ids.filter((id) => typeof id === "string" &&
      (id === myID || (playerInfo[id] && playerInfo[id].dice)));
    if (!v) {
      listEl.innerHTML = players.map((id) => {
        const name = id === myID ? "You" : (displayNameFor(id) || "Guest");
        return `<span class="game-player-chip${id === myID ? " me" : ""}">${_esc(name)}</span>`;
      }).join("");
      return;
    }
  }

  listEl.innerHTML = v.roster.map((id, seat) => {
    const classes = ["game-player-chip"];
    const away = !v.connected[seat] && !v.removed[seat];
    if (seat === v.mySeat) classes.push("me");
    if (v.removed[seat]) classes.push("kicked");
    if (!v.over && v.waiting.includes(seat)) classes.push("current");
    if (away) classes.push("away");
    if (v.over && v.winners.includes(seat)) classes.push("winner");

    let html = `<span class="chip-name">${_esc(_rawName(seat))}</span>`;
    const badge = v.badges && v.badges[seat];
    if (badge) html += `<span class="chip-badge">${_esc(badge)}</span>`;
    if (v.removed[seat]) html += " <em>(removed)</em>";
    if (away) html += " <em>disconnected</em>";

    if (!v.over && v.mySeat >= 0 && !v.removed[v.mySeat] && seat !== v.mySeat && !v.removed[seat]) {
      if (away) {
        const wait = v.graceSeconds - (v.awaySeconds[seat] + _elapsed());
        html += wait > 0
          ? `<button class="remove-btn" disabled title="They may be refreshing">Remove in ${wait}s</button>`
          : `<button class="remove-btn" data-remove="${seat}">Remove</button>`;
      } else if (_canRemove(seat)) {
        html += `<button class="remove-btn" data-remove="${seat}" title="They've held up the game for a while">Remove</button>`;
      }
    }
    return `<span class="${classes.join(" ")}">${html}</span>`;
  }).join("");
}

function _renderBanner() {
  const v = gameView;
  const banner = document.getElementById("game-state-message");
  const title  = document.getElementById("turn-banner-title");
  const detail = document.getElementById("turn-banner-detail");
  if (!banner || !title || !detail || !v) return;
  const mine = v.mySeat >= 0 && !v.over && v.waiting.includes(v.mySeat);
  banner.classList.toggle("mine", mine);
  banner.classList.toggle("game-over", v.over);

  if (v.over) {
    if (!v.winners.length) title.textContent = "Game over";
    else if (v.winners.includes(v.mySeat)) {
      title.textContent = v.winners.length > 1 ? "🏆 You share the win!" : "🏆 You win!";
    } else {
      title.textContent = `🏆 ${_nameList(v.winners)} ${v.winners.length > 1 ? "win" : "wins"}!`;
    }
    detail.textContent = "Pick a game and press New Game to play again.";
    return;
  }
  if (mine) {
    title.textContent = "YOUR TURN";
  } else if (v.waiting.length) {
    const others = v.waiting.filter((s) => s !== v.mySeat);
    title.textContent = others.length > 1 ? `Waiting for ${_nameList(others)}` : `${_nameList(others)}'s turn`;
  } else {
    title.textContent = v.gameName;
  }
  if (v.mySeat < 0) title.textContent += " · you're watching";
  detail.textContent = _plainText(v.prompt);
}

function _renderLog(flash) {
  const logEl = document.getElementById("game-action-log");
  if (!logEl || !gameView) return;
  const entries = gameView.log || [];
  logEl.innerHTML = entries.slice(-10).reverse().map((e) =>
    `<li class="log-${_esc(e.kind)}">${_text(e.text)}</li>`).join("");

  const newest = entries.length ? entries[entries.length - 1].seq : 0;
  if (flash) {
    const fresh = entries.filter((e) => e.seq > lastLogSeq && (e.kind === "bust" || e.kind === "win"));
    if (fresh.length) _flash(fresh[fresh.length - 1]);
  }
  lastLogSeq = newest;
}

function _flash(entry) {
  const el = document.getElementById("bust-flash");
  if (!el) return;
  el.textContent = _plainText(entry.text);
  el.className = entry.kind === "win" ? "win" : "";
  void el.offsetWidth; // restart the animation
  el.classList.add("show");
}

function _renderTiles() {
  const v = gameView;
  if (!v || v.over && !v.winners.length) { setTileGameInfo({}); return; }
  const info = {};
  v.roster.forEach((id, seat) => {
    info[id] = {
      badge: v.removed[seat] ? "out" : (v.badges && v.badges[seat]) || "",
      turn: !v.over && v.waiting.includes(seat),
      out: v.removed[seat],
    };
  });
  setTileGameInfo(info);
}

function _renderBoard() {
  const boardEl = document.getElementById("game-board");
  if (!boardEl || !gameView) return;
  const r = GameUI.renderers[gameView.gameId];
  if (!r) {
    boardEl.innerHTML = `<p class="note">This game can't be shown — try reloading the page.</p>`;
    return;
  }
  try {
    boardEl.innerHTML = r.board(gameView.state, _uiFor(gameView));
  } catch (err) {
    console.error("[games] render error", err);
    boardEl.innerHTML = `<p class="note">Something went wrong drawing the game.</p>`;
  }
}

function _renderHeaderButtons() {
  const startBtn = document.getElementById("start-game-btn");
  const statePanel = document.getElementById("game-state-panel");
  const running = gameView && !gameView.over;
  if (startBtn) {
    startBtn.textContent = gameView ? "New Game" : "Start Game";
    // Spectators can't restart a running game.
    startBtn.disabled = !!(running && gameView.mySeat < 0);
  }
  if (statePanel) statePanel.style.display = gameView ? "flex" : "none";
}

function _renderAll(flash) {
  _renderHeaderButtons();
  _renderChips();
  if (gameView) {
    _renderBanner();
    _renderBoard();
    _renderLog(flash);
  }
  _renderTiles();
  _renderRules();
  _refreshFullscreen();
}

function _onGameState(view) {
  const newGame = view.gameNo !== lastGameNo;
  const changedRoll = view.rollSeq !== lastRollSeq;
  if (newGame) {
    uiSelected.clear();
    lastLogSeq = 0;
    userRequestedFullscreen = false;
  }
  const flash = !newGame && gameView !== null;
  gameView = view;
  gameViewAt = Date.now();

  const waitingOnMe = view.mySeat >= 0 && !view.over && view.waiting.includes(view.mySeat);
  if (waitingOnMe && !wasWaitingOnMe) {
    userMinimizedFullscreen = false;
    if (navigator.vibrate) navigator.vibrate([120, 60, 120]);
  }
  if (!waitingOnMe || changedRoll) uiSelected.clear();
  wasWaitingOnMe = waitingOnMe;
  document.title = waitingOnMe ? "🎲 Your turn! — " + BASE_TITLE : BASE_TITLE;

  _renderAll(flash);
  lastRollSeq = view.rollSeq;
  lastGameNo = view.gameNo;
}

/**
 * Called from video.js for game messages from the server.
 */
function handleDiceGameMessage(msg) {
  switch (msg.type) {
    case "game_catalog":
      gameCatalog = Array.isArray(msg.games) ? msg.games : [];
      _renderPicker();
      break;
    case "game_state":
      _onGameState(msg);
      break;
    case "game_none":
      gameView = null;
      _renderAll(false);
      break;
    case "game_error":
      _setStatus(msg.message, true);
      break;
  }
}

// ── Board input: buttons, die selection, forms ───────────────────────────────

(function setupBoardInput() {
  const panel = document.getElementById("game-panel");
  if (!panel) return;

  panel.addEventListener("click", (e) => {
    const sel = e.target.closest("[data-sel]");
    if (sel && panel.contains(sel)) {
      const i = parseInt(sel.dataset.sel, 10);
      if (uiSelected.has(i)) uiSelected.delete(i); else uiSelected.add(i);
      const r = gameView && GameUI.renderers[gameView.gameId];
      if (r && r.onSelect) r.onSelect(i, uiSelected, gameView.state);
      _renderBoard();
      return;
    }
    const act = e.target.closest("[data-act]");
    if (act && !act.disabled) {
      let action;
      try { action = JSON.parse(act.dataset.act); } catch (_) { return; }
      if (act.hasAttribute("data-with-sel")) action.dice = Array.from(uiSelected).sort((a, b) => a - b);
      sendGameAction(action);
      return;
    }
    const rm = e.target.closest("[data-remove]");
    if (rm && !rm.disabled) {
      const seat = parseInt(rm.dataset.remove, 10);
      if (confirm(`Remove ${_rawName(seat)} from this game?`)) {
        _sendToServer({ type: "game_remove", seat: seat });
      }
    }
  });

  panel.addEventListener("keydown", (e) => {
    const sel = e.target.closest && e.target.closest("[data-sel]");
    if (sel && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); sel.click(); }
  });

  panel.addEventListener("submit", (e) => {
    const form = e.target.closest("form[data-act-form]");
    if (!form) return;
    e.preventDefault();
    let action;
    try { action = JSON.parse(form.dataset.actForm); } catch (_) { return; }
    const dice = [];
    form.querySelectorAll("[name]").forEach((el) => {
      let val = el.value;
      if (el.type === "checkbox") val = el.checked;
      else if (el.type === "number" || el.hasAttribute("data-num")) val = parseInt(val, 10);
      const m = /^dice(\d+)$/.exec(el.name);
      if (m) dice[parseInt(m[1], 10)] = val;
      else action[el.name] = val;
    });
    if (dice.length) action.dice = dice;
    sendGameAction(action);
  });
})();

// Re-render chips once a second so away/idle timers and Remove buttons stay
// current between server updates.
setInterval(() => {
  if (gameView && !gameView.over) _renderChips();
}, 1000);

// ── Hooks called by video.js ─────────────────────────────────────────────────

function onSlotsUpdated() {
  const el = document.getElementById("game-status");
  if (el && el.textContent === "Reconnecting…") _setStatus("");
  _renderChips();
}
function onPlayerInfoUpdated() { _renderAll(false); }
function onTilesChanged() { _renderTiles(); }
function onWSDisconnected() { _setStatus("Reconnecting…"); }

// ── Mobile fullscreen takeover ───────────────────────────────────────────────
//
// On a narrow screen, when the game is waiting on you, the game panel takes
// over the screen and the videos shrink to a corner.  The exit button opts
// out until your next turn.

function _refreshFullscreen() {
  const narrow = window.matchMedia(NARROW_VIEWPORT_QUERY).matches;
  const inGame = !!gameView && !gameView.over;
  const myTurn = inGame && gameView.mySeat >= 0 && gameView.waiting.includes(gameView.mySeat);
  const want = narrow && inGame && ((myTurn && !userMinimizedFullscreen) || userRequestedFullscreen);
  document.body.classList.toggle("dice-fullscreen", want);
  const enterBtn = document.getElementById("enter-fullscreen-btn");
  if (enterBtn) enterBtn.style.display = narrow && inGame && !want ? "inline-block" : "none";
}

(function setupFullscreenControls() {
  const exitBtn = document.getElementById("exit-fullscreen-btn");
  const enterBtn = document.getElementById("enter-fullscreen-btn");
  if (exitBtn) exitBtn.addEventListener("click", () => {
    userMinimizedFullscreen = true;
    userRequestedFullscreen = false;
    _refreshFullscreen();
  });
  if (enterBtn) enterBtn.addEventListener("click", () => {
    userMinimizedFullscreen = false;
    userRequestedFullscreen = true;
    _refreshFullscreen();
  });
  if (window.matchMedia) {
    const mq = window.matchMedia(NARROW_VIEWPORT_QUERY);
    if (mq.addEventListener) mq.addEventListener("change", _refreshFullscreen);
    else if (mq.addListener) mq.addListener(_refreshFullscreen);
  }
})();

// ── Shake-to-roll ────────────────────────────────────────────────────────────
//
// Shaking the phone presses the board's roll button (one marked data-roll).
// iOS 13+ needs a permission grant from a tap, so it gets an "Enable
// Shake-to-Roll" button.  Firefox on Android uses shakes for its own "shake
// to summarize" feature and a page can't take the gesture back, so Firefox
// users get a one-time note explaining how to turn that off.

(function setupShakeToRoll() {
  if (typeof window.DeviceMotionEvent === "undefined") return;

  const btn = document.getElementById("enable-shake-btn");
  const needsPermission = typeof window.DeviceMotionEvent.requestPermission === "function";

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

  const SHAKE_THRESHOLD = 25;
  const SHAKE_DEBOUNCE_MS = 900;
  let last = { x: 0, y: 0, z: 0, t: 0 };
  let lastShakeAt = 0;
  let attached = false;

  function onMotion(evt) {
    const a = evt.accelerationIncludingGravity;
    if (!a) return;
    const now = Date.now();
    if (last.t === 0) { last = { x: a.x || 0, y: a.y || 0, z: a.z || 0, t: now }; return; }
    if (now - last.t < 80) return;
    const delta = Math.abs((a.x || 0) - last.x) + Math.abs((a.y || 0) - last.y) + Math.abs((a.z || 0) - last.z);
    last = { x: a.x || 0, y: a.y || 0, z: a.z || 0, t: now };
    if (delta > SHAKE_THRESHOLD && now - lastShakeAt > SHAKE_DEBOUNCE_MS) {
      lastShakeAt = now;
      const rollBtn = document.querySelector("#game-board [data-roll]:not([disabled])");
      if (rollBtn) rollBtn.click();
    }
  }

  function attach() {
    if (attached) return;
    window.addEventListener("devicemotion", onMotion);
    attached = true;
    if (btn) { btn.textContent = "Shake-to-roll: ON"; btn.disabled = true; }
  }

  if (!needsPermission) { attach(); return; }
  if (!btn) return;
  btn.style.display = "inline-block";
  btn.addEventListener("click", () => {
    window.DeviceMotionEvent.requestPermission()
      .then((state) => {
        if (state === "granted") attach();
        else btn.textContent = "Shake denied — use the Roll button";
      })
      .catch(() => { btn.textContent = "Shake unavailable — use the Roll button"; });
  });
})();

_renderAll(false);
