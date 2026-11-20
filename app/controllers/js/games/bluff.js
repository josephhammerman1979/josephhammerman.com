// bluff.js — renderers for the bluffing games: Mia, Perudo and Liar's Poker
// Dice (see games.js for the renderer API).  The server only sends the dice
// each player may see, so "hidden" dice here are just counts.
//
// state shapes: app/games/bluff/{mia,perudo,liarspoker}.go (miaView,
// perudoView, lpView).

(function () {
  // ── Shared pieces ──────────────────────────────────────────────────────────

  // n face-down dice: other players' concealed dice (only the count is sent).
  function hiddenDice(ui, n, opts) {
    let out = "";
    for (let i = 0; i < n; i++) out += ui.die(0, Object.assign({ hidden: true }, opts || {}));
    return `<span class="dice-row">${out}</span>`;
  }

  // The big central "current claim" box.
  function claimBox(label, bigHTML, sub, cls) {
    return `<div class="board-section bluff-claim ${cls || ""}">
        <div class="bluff-claim-label">${label}</div>
        <div class="bluff-claim-value">${bigHTML}</div>
        ${sub ? `<div class="bluff-claim-sub">${sub}</div>` : ""}
      </div>`;
  }

  function secretBox(title, inner) {
    return `<div class="board-section bluff-secret">
        <h3>🔒 ${title} <span class="pill gold">only you can see these</span></h3>
        <div class="board-row">${inner}</div>
      </div>`;
  }

  // Name plus a verb that agrees with it: "You roll" / "Sam rolls".
  function sv(ui, seat, you, other) {
    return `${ui.name(seat)} ${seat === ui.mySeat ? you : other}`;
  }

  function verdict(truthful) {
    return truthful
      ? `<span class="pill good">TRUE</span>`
      : `<span class="pill bad">BLUFF</span>`;
  }

  function pips(n, total, on, off) {
    let s = "";
    for (let i = 0; i < total; i++) s += i < n ? on : off;
    return `<span class="bluff-pips">${s}</span>`;
  }

  function statusCell(ui, s, seat, extra) {
    if (s.winner === seat) return `<span class="pill good">winner</span>`;
    if (ui.view.removed[seat]) return `<span class="pill bad">left</span>`;
    if (s.out[seat]) return `<span class="pill bad">out</span>`;
    return extra || "";
  }

  // ── Mia ────────────────────────────────────────────────────────────────────

  function miaDiceFromValue(v) {
    return [Math.floor(v / 10), v % 10];
  }

  function miaBoard(s, ui) {
    const parts = [];
    const nameOf = (v) => {
      const e = s.values.find((x) => x.value === v);
      return ui.esc(e ? e.name : String(v));
    };

    // The claim.
    if (s.winner >= 0) {
      // nothing: the reveal and scoreboard say it all
    } else if (s.claim && s.phase === "respond") {
      parts.push(claimBox(
        sv(ui, s.claimBy, "claim", "claims"),
        `${ui.esc(s.claimName)} <span class="bluff-claim-dice">${ui.dice(miaDiceFromValue(s.claim), { small: true })}</span>`,
        `${ui.name(s.responder)} must accept or challenge`));
    } else if (s.claim && s.phase === "declare") {
      parts.push(claimBox(
        `To beat — accepted from ${ui.name(s.claimBy)}`,
        ui.esc(s.claimName),
        s.claim === 21 ? `Mia can only be matched: ${ui.name(s.current)} must declare Mia` : `${ui.name(s.current)} must declare higher`));
    } else {
      parts.push(claimBox("Fresh round", "No claim yet", `${s.phase === "roll" ? sv(ui, s.current, "roll", "rolls") : sv(ui, s.current, "declare", "declares")} — anything goes`, "dim"));
    }

    // The cup.
    if (s.myDice) {
      parts.push(secretBox("Your roll",
        `${ui.dice(s.myDice, { big: true, fresh: ui.fresh })}<span class="big-number">${nameOf(s.myValue)}</span>`));
    } else if (s.cupDice > 0) {
      parts.push(`<div class="board-section">
          <h3>🥤 In ${ui.name(s.current)}’s cup</h3>
          <div class="board-row">${hiddenDice(ui, s.cupDice, { big: true })}</div>
        </div>`);
    }

    // The last reveal.
    if (s.reveal) {
      const r = s.reveal;
      parts.push(`<div class="board-section bluff-reveal ${r.truthful ? "true" : "bluff"}">
          <h3>Cup lifted: ${ui.name(r.challenger)} challenged ${ui.name(r.claimant)}</h3>
          <div class="board-row">${ui.dice(r.dice, { big: true })}
            <span class="bluff-reveal-text">${ui.name(r.claimant)} had <strong>${ui.esc(r.realName)}</strong>,
              claimed <strong>${ui.esc(r.claimName)}</strong> ${verdict(r.truthful)}</span></div>
          <p class="bluff-loser">💔 ${ui.name(r.loser)} lost a life${r.livesLeft > 0 ? ` (${r.livesLeft} left)` : ` — ${r.loser === ui.mySeat ? "you're" : "they're"} out`}</p>
        </div>`);
    }

    // Controls.
    if (ui.waitingOnMe) {
      if (s.phase === "roll") {
        parts.push(`<div class="board-actions">${ui.button("Roll the dice 🎲", { type: "roll" }, { primary: true, roll: true })}</div>`);
      } else if (s.phase === "declare" && s.legal) {
        const legal = new Set(s.legal);
        const def = legal.has(s.myValue) ? s.myValue : s.legal[0];
        const opts = s.values.slice().reverse().map((v) =>
          `<option value="${v.value}"${legal.has(v.value) ? "" : " disabled"}${v.value === def ? " selected" : ""}>` +
          `${ui.esc(v.name)}${v.value === s.myValue ? " ← your roll" : ""}</option>`).join("");
        parts.push(`<div class="board-section">
            <form class="board-actions" data-act-form='{"type":"declare"}'>
              <label>Declare <select name="index" data-num>${opts}</select></label>
              <button type="submit" class="btn primary">Declare</button>
            </form>
            <p class="note">${legal.has(s.myValue) ? "Your real roll is good enough — or bluff higher." : "Your real roll isn't high enough: you'll have to bluff."}</p>
          </div>`);
      } else if (s.phase === "respond") {
        parts.push(`<div class="board-actions">
            ${ui.button(`Accept ${ui.esc(s.claimName)} & roll — then beat it`, { type: "accept" }, { primary: true })}
            ${ui.button("Challenge — lift the cup!", { type: "challenge" }, { cls: "danger" })}
          </div>`);
      }
    }

    // Scoreboard.
    const rows = s.lives.map((l, seat) => ({
      seat,
      cells: [
        pips(l, s.startLives, "♥", "♡"),
        statusCell(ui, s, seat, seat === s.current && s.cupDice ? `<span class="pill gold">has the cup</span>` : ""),
      ],
    }));
    parts.push(ui.scoreTable(["Lives", ""], rows));

    parts.push(`<details class="bluff-help"><summary>Ranking, best first</summary>
        <p class="note">${s.values.slice().reverse().map((v) => ui.esc(v.name)).join(" › ")}</p></details>`);
    return parts.join("");
  }

  GameUI.register("mia", { board: miaBoard });

  // ── Perudo ─────────────────────────────────────────────────────────────────

  function perudoMatches(d, face, wild) {
    return d === face || (wild && d === 1);
  }

  function perudoBoard(s, ui) {
    const parts = [];
    const bidFace = s.bid ? s.bid.face : 0;

    parts.push(`<p class="note bluff-table-note">🎲 <strong>${s.totalDice}</strong> dice in play · round ${s.round}${s.wild ? " · <strong>ones are wild</strong>" : ""}</p>`);

    if (s.winner >= 0) {
      // final reveal below
    } else if (s.bid) {
      parts.push(claimBox(
        `${sv(ui, s.bid.by, "bid", "bids")} at least`,
        `<span class="bluff-count">${s.bid.count}</span> × ${ui.die(s.bid.face, { big: true })}`,
        `${ui.esc(s.bid.name)}${s.wild && s.bid.face !== 1 ? " (ones count too)" : ""} — ${ui.name(s.current)} to answer`));
    } else {
      parts.push(claimBox("New round", "No bid yet", sv(ui, s.current, "open", "opens") + " the bidding", "dim"));
    }

    if (s.myDice && s.myDice.length) {
      const dice = s.myDice.map((d) => ui.die(d, { big: true, fresh: ui.fresh, kept: bidFace && perudoMatches(d, bidFace, s.wild) })).join("");
      const mine = bidFace ? s.myDice.filter((d) => perudoMatches(d, bidFace, s.wild)).length : -1;
      parts.push(secretBox("Your dice",
        `<span class="dice-row">${dice}</span>` +
        (mine >= 0 ? `<span class="note">You have <strong>${mine}</strong> toward the bid</span>` : "")));
    }

    if (s.reveal) {
      const r = s.reveal;
      const good = r.kind === "challenge" ? r.actual >= r.bid.count : r.actual === r.bid.count;
      const head = r.kind === "challenge"
        ? `${ui.name(r.caller)} challenged the bid of ${ui.esc(r.bid.name)} by ${ui.name(r.bid.by)}`
        : `${ui.name(r.caller)} called spot on: ${ui.esc(r.bid.name)} by ${ui.name(r.bid.by)}`;
      const rows = r.dice.map((d, seat) => d.length
        ? `<div class="board-row bluff-reveal-row"><span class="bluff-who">${ui.name(seat)}</span>${
          d.map((f) => ui.die(f, { small: true, kept: perudoMatches(f, r.bid.face, r.wild) })).join("")}</div>`
        : "").join("");
      let result = "";
      if (r.loser >= 0) result = `💔 ${ui.name(r.loser)} lost a die`;
      else if (r.gainer >= 0) result = `🎉 ${ui.name(r.gainer)} got a die back`;
      else result = `🎯 Spot on — but ${ui.name(r.caller)} already had five dice`;
      const verdictText = r.kind === "challenge"
        ? (good ? `<span class="pill good">BID STANDS</span>` : `<span class="pill bad">BLUFF</span>`)
        : (good ? `<span class="pill good">SPOT ON</span>` : `<span class="pill bad">NOT EXACT</span>`);
      parts.push(`<div class="board-section bluff-reveal ${good ? "true" : "bluff"}">
          <h3>Cups lifted: ${head}</h3>
          <p class="bluff-reveal-text">There ${r.actual === 1 ? "was" : "were"} <strong class="big-number">${r.actual}</strong>
            × ${ui.die(r.bid.face, { small: true })}${r.wild && r.bid.face !== 1 ? " (with wild ones)" : ""} ${verdictText}</p>
          ${rows}
          <p class="bluff-loser">${result}</p>
        </div>`);
    }

    if (ui.waitingOnMe) {
      let count, face;
      if (s.bid && s.minRaise) {
        count = s.minRaise.count; face = s.minRaise.face;
      } else if (!s.bid) {
        // Open with the face I hold most of.
        face = s.minFace; count = 1;
        for (let f = s.minFace; f <= 6; f++) {
          const n = (s.myDice || []).filter((d) => perudoMatches(d, f, s.wild)).length;
          if (n >= count) { count = n; face = f; }
        }
        count = Math.max(1, count);
      }
      const canRaise = !!s.minRaise;
      let faceOpts = "";
      for (let f = s.minFace; f <= 6; f++) faceOpts += `<option value="${f}"${f === face ? " selected" : ""}>${f}s</option>`;
      const raiseForm = canRaise
        ? `<form data-act-form='{"type":"bid"}'>
             <input type="number" name="count" min="1" max="${s.totalDice}" value="${count}" aria-label="count">
             <span>×</span>
             <select name="face" data-num aria-label="face">${faceOpts}</select>
             <button type="submit" class="btn primary">${s.bid ? "Raise" : "Bid"}</button>
           </form>`
        : `<p class="note">Nothing beats ${ui.esc(s.bid.name)} — challenge or call spot on.</p>`;
      const hint = s.bid && s.minRaise
        ? `<p class="note">Smallest raise: ${ui.esc(s.minRaise.name)}. Raise = same count with a higher face, or a higher count.</p>`
        : "";
      const calls = s.bid
        ? `${ui.button(`Challenge — fewer than ${s.bid.count}`, { type: "challenge" }, { cls: "danger" })}
           ${ui.button(`Spot on — exactly ${s.bid.count}`, { type: "spoton" }, { cls: "warn" })}`
        : "";
      parts.push(`<div class="board-section"><div class="board-actions">${raiseForm}${calls}</div>${hint}</div>`);
    }

    const rows = s.diceCounts.map((n, seat) => ({
      seat,
      cells: [
        `<span class="big-number">${n}</span>`,
        s.out[seat] ? "" : (seat === ui.mySeat && s.myDice ? "" : hiddenDice(ui, n, { small: true })),
        statusCell(ui, s, seat, s.bid && s.bid.by === seat ? `<span class="pill gold">bidder</span>` : ""),
      ],
    }));
    parts.push(ui.scoreTable(["Dice", "Cup", ""], rows));
    return parts.join("");
  }

  GameUI.register("perudo", { board: perudoBoard });

  // ── Liar's Poker Dice ──────────────────────────────────────────────────────

  // lpEval mirrors games.EvalHand (app/games/poker.go) for five dice.
  function lpEval(d) {
    const c = [0, 0, 0, 0, 0, 0, 0];
    d.forEach((f) => { c[f]++; });
    const faces = [];
    for (let f = 6; f >= 1; f--) if (c[f]) faces.push(f);
    faces.sort((a, b) => c[b] - c[a] || b - a);
    const key = [];
    faces.forEach((f) => { for (let k = 0; k < c[f]; k++) key.push(f); });
    const top = c[faces[0]], second = faces.length > 1 ? c[faces[1]] : 0;
    const p = (f) => `${f}s`;
    if (top === 5) return { cat: 8, key, name: `Five ${p(faces[0])}` };
    if (top === 4) return { cat: 7, key, name: `Four ${p(faces[0])}` };
    if (top === 1 && !c[1]) return { cat: 6, key, name: "High straight" };
    if (top === 1 && !c[6]) return { cat: 5, key, name: "Low straight" };
    if (top === 3 && second === 2) return { cat: 4, key, name: `Full house, ${p(faces[0])} over ${p(faces[1])}` };
    if (top === 3) return { cat: 3, key, name: `Three ${p(faces[0])}` };
    if (top === 2 && second === 2) return { cat: 2, key, name: `Two pair, ${p(faces[0])} and ${p(faces[1])}` };
    if (top === 2) return { cat: 1, key, name: `Pair of ${p(faces[0])}` };
    return { cat: 0, key, name: `${key[0]} high` };
  }
  function lpCompare(a, b) {
    if (a.cat !== b.cat) return a.cat - b.cat;
    for (let i = 0; i < a.key.length && i < b.key.length; i++) if (a.key[i] !== b.key[i]) return a.key[i] - b.key[i];
    return 0;
  }

  // Live "what hand is this" readout on the bid form.
  function lpUpdateForm(form) {
    const out = form.querySelector("[data-lp-readout]");
    if (!out) return;
    const faces = [0, 1, 2, 3, 4].map((i) => parseInt(form.elements["dice" + i].value, 10));
    const h = lpEval(faces);
    let bid = null;
    try { bid = JSON.parse(form.dataset.lpBid || "null"); } catch (_) { /* no bid */ }
    const ok = !bid || lpCompare(h, lpEval(bid)) > 0;
    out.textContent = ok ? `${h.name} ✓` : `${h.name} — not higher than the bid`;
    out.className = "pill " + (ok ? "good" : "bad");
    const btn = form.querySelector("button[type=submit]");
    if (btn) btn.disabled = !ok;
  }
  document.addEventListener("change", (e) => {
    const form = e.target.closest && e.target.closest("form[data-lp-bid]");
    if (form) lpUpdateForm(form);
  });

  function lpBoard(s, ui) {
    const parts = [];
    const mine = ui.waitingOnMe;
    const holder = ui.name(s.holder);

    if (s.winner >= 0) {
      // final reveal below
    } else if (s.bid) {
      const sub = s.phase === "respond"
        ? `${ui.name(s.responder)} must accept or challenge${s.canAccept ? "" : " — nothing beats it, so challenge"}`
        : `${holder} accepted and must bid higher`;
      parts.push(claimBox(sv(ui, s.bidder, "bid", "bids"), ui.esc(s.bidName),
        `<span class="bluff-claim-dice">${ui.dice(s.bid, { small: true })}</span> ${sub}`));
    } else {
      parts.push(claimBox(`Round ${s.round}`, "No bid yet", `${s.phase === "roll" ? sv(ui, s.holder, "roll", "rolls") : sv(ui, s.holder, "bid", "bids")} first — any hand goes`, "dim"));
    }

    // The dice: plain view and cup.
    if (s.dice && s.winner < 0) {
      const selectable = mine && (s.phase === "arrange" || s.phase === "reroll");
      const die = (i, extra) => ui.die(s.dice[i], Object.assign({
        big: true, fresh: ui.fresh, selectable, index: i,
        hidden: !s.dice[i],
      }, extra || {}));
      const viewIdx = [], cupIdx = [];
      s.inView.forEach((v, i) => (v ? viewIdx : cupIdx).push(i));
      if (s.phase === "arrange" && mine) {
        const all = [0, 1, 2, 3, 4].map((i) => die(i, { kept: s.inView[i] })).join("");
        parts.push(`<div class="board-section bluff-secret">
            <h3>🔒 You took the cup <span class="pill gold">only you can see the cup dice</span></h3>
            <div class="board-row"><span class="dice-row">${all}</span></div>
            <p class="note">Tap the dice you want in <strong>plain view</strong> (everyone sees them). The rest go in the cup.
              ${viewIdx.length ? "Outlined dice are in view now." : ""}</p>
            <p class="note">Your hand: <strong>${ui.esc(s.myHand || "")}</strong></p>
          </div>`);
      } else {
        const viewRow = viewIdx.length
          ? `<span class="dice-row">${viewIdx.map((i) => die(i)).join("")}</span>`
          : `<span class="note">none</span>`;
        const cupRow = cupIdx.length
          ? `<span class="dice-row">${cupIdx.map((i) => die(i)).join("")}</span>`
          : `<span class="note">empty</span>`;
        parts.push(`<div class="board-section">
            <h3>👀 Plain view — everyone sees these</h3>
            <div class="board-row">${viewRow}</div>
          </div>
          <div class="board-section ${s.canSee ? "bluff-secret" : ""}">
            <h3>${s.canSee
              ? `🔒 In your cup <span class="pill gold">only you can see these</span>`
              : `🥤 In ${holder}’s cup`}</h3>
            <div class="board-row">${cupRow}</div>
            ${s.canSee && s.myHand ? `<p class="note">All five make: <strong>${ui.esc(s.myHand)}</strong></p>` : ""}
          </div>`);
      }
    }

    if (s.reveal) {
      const r = s.reveal;
      parts.push(`<div class="board-section bluff-reveal ${r.truthful ? "true" : "bluff"}">
          <h3>Cup lifted: ${ui.name(r.challenger)} challenged ${ui.name(r.bidder)}</h3>
          <div class="board-row">${ui.dice(r.dice, { big: true })}</div>
          <p class="bluff-reveal-text">${ui.name(r.bidder)} had <strong>${ui.esc(r.realName)}</strong>,
            bid <strong>${ui.esc(r.bidName)}</strong> ${verdict(r.truthful)}</p>
          <p class="bluff-loser">💔 ${ui.name(r.loser)} lost a token${r.tokensLeft > 0 ? ` (${r.tokensLeft} left)` : ` — ${r.loser === ui.mySeat ? "you're" : "they're"} out`}</p>
        </div>`);
    }

    if (mine) {
      if (s.phase === "roll") {
        parts.push(`<div class="board-actions">${ui.button("Roll five dice into the cup 🎲", { type: "roll" }, { primary: true, roll: true })}</div>`);
      } else if (s.phase === "respond") {
        parts.push(`<div class="board-actions">
            ${ui.button("Accept — take the cup, then bid higher", { type: "accept" }, { primary: true, disabled: !s.canAccept })}
            ${ui.button("Challenge — lift the cup!", { type: "challenge" }, { cls: "danger" })}
          </div>`);
      } else if (s.phase === "arrange") {
        const n = ui.selected.size;
        parts.push(`<div class="board-actions">
            ${ui.button(n ? `Show ${n} in view · ${5 - n} in the cup` : "Keep all five in the cup", { type: "arrange" }, { primary: true, withSelection: true })}
          </div>`);
      } else if (s.phase === "reroll") {
        const sel = Array.from(ui.selected);
        const sides = new Set(sel.map((i) => !!s.inView[i]));
        const mixed = sides.size > 1;
        const where = sides.has(true) ? "in view (everyone sees the result)" : "in the cup (stays secret)";
        parts.push(`<div class="board-section">
            <p class="note">Tap dice to re-roll — all from the cup or all from plain view, not both.</p>
            <div class="board-actions">
              ${ui.button(mixed ? "Pick dice from one side only" : sel.length ? `Re-roll ${sel.length} ${where}` : "Re-roll selected",
                { type: "reroll" }, { primary: true, roll: true, withSelection: true, disabled: mixed || !sel.length })}
              ${ui.button("Don't re-roll", { type: "reroll", dice: [] })}
            </div>
          </div>`);
      } else if (s.phase === "bid") {
        const sug = s.suggest || [6, 5, 4, 3, 1];
        const selects = sug.map((f, i) => {
          let o = "";
          for (let v = 6; v >= 1; v--) o += `<option value="${v}"${v === f ? " selected" : ""}>${v}</option>`;
          return `<select name="dice${i}" data-num aria-label="die ${i + 1}">${o}</select>`;
        }).join("");
        const h = lpEval(sug);
        const ok = !s.bid || lpCompare(h, lpEval(s.bid)) > 0;
        parts.push(`<div class="board-section">
            <form class="board-actions" data-act-form='{"type":"bid"}' data-lp-bid='${ui.esc(JSON.stringify(s.bid || null))}'>
              <span class="bluff-bid-selects">${selects}</span>
              <span data-lp-readout class="pill ${ok ? "good" : "bad"}">${ui.esc(h.name)}${ok ? " ✓" : ""}</span>
              <button type="submit" class="btn primary"${ok ? "" : " disabled"}>Bid this hand</button>
            </form>
            <p class="note">${s.bid ? `Must beat ${ui.esc(s.bidName)}. ` : ""}You may bid any hand — truthfully or not.</p>
          </div>`);
      }
    }

    const rows = s.tokens.map((t, seat) => ({
      seat,
      cells: [
        pips(t, s.startTokens, "●", "○"),
        statusCell(ui, s, seat, seat === s.holder && s.dice ? `<span class="pill gold">has the cup</span>` : ""),
      ],
    }));
    parts.push(ui.scoreTable(["Tokens", ""], rows));
    parts.push(`<details class="bluff-help"><summary>Hand ranking, best first</summary>
        <p class="note">${(s.rankings || []).map(ui.esc).join(" › ")}</p></details>`);
    return parts.join("");
  }

  GameUI.register("liarspoker", { board: lpBoard });
})();
