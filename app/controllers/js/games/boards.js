// boards.js — renderers for Klondike, Hundreds, Three Hands and Rule Maker
// (app/games/boards/*.go).  See games.js for the renderer API.

(function () {
  function pill(text, cls) {
    return `<span class="pill${cls ? " " + cls : ""}">${text}</span>`;
  }
  function dots(n, cls) {
    let out = "";
    for (let i = 0; i < n; i++) out += `<i class="bd-token${cls ? " " + cls : ""}"></i>`;
    return `<span class="bd-tokens">${out || "—"}</span>`;
  }
  function emptyDice(n, opts) {
    const out = [];
    for (let i = 0; i < n; i++) out.push(ui0.die(0, Object.assign({ empty: true }, opts || {})));
    return `<span class="dice-row">${out.join("")}</span>`;
  }
  function plural(n, one, many) { return `${n} ${n === 1 ? one : many}`; }
  function nameList(ui, seats) {
    if (!seats || !seats.length) return "nobody";
    return seats.map((s) => ui.name(s)).join(", ");
  }
  // ui0 is the ui of the render in progress (helpers above need ui.die).
  let ui0 = null;

  // ── Klondike (the sheet's "Banker") ─────────────────────────────────────────────────────────────────

  function banker(s, ui) {
    ui0 = ui;
    const gold = s.variant === "goldrush";
    const me = ui.mySeat;
    const mine = me >= 0 ? s.seats[me] : null;
    const bank = s.banker >= 0 ? s.seats[s.banker] : null;
    const over = s.phase === "over";

    const head = `<div class="board-row">
        ${pill(`Round ${s.round} of ${s.rounds}`)}
        ${pill(gold ? "Gold Rush" : "Classic")}
        ${!over && s.banker >= 0 ? pill(`Banker: ${ui.name(s.banker)}`, "gold") : ""}
      </div>`;

    // The point: the banker's hand.
    let point = "";
    if (!over && bank) {
      let body;
      if (bank.rolled) {
        body = `${ui.dice(bank.dice, { big: true, fresh: ui.fresh && s.current === s.banker })}
          <div><div class="note">${gold ? "Banker's hand" : "The point to beat"}</div>
          <div class="big-number">${ui.esc(bank.hand)}</div></div>`;
      } else {
        body = `${emptyDice(5, { big: true })}
          <span class="note">${gold ? "The banker rolls last, after everyone else." : "The banker hasn't set the point yet."}</span>`;
      }
      point = `<div class="board-section bk-point">
          <h3>${s.banker === me ? "You are the banker" : `${ui.name(s.banker)} is the banker`}</h3>
          <div class="board-row">${body}</div>
        </div>`;
    }

    // The player whose turn it is (if not the banker).
    let turn = "";
    if (!over && s.phase === "play" && s.current >= 0) {
      const c = s.seats[s.current];
      const who = s.current === me ? "Your roll" : `${ui.name(s.current)}'s roll`;
      const dice = c.rolled
        ? ui.dice(c.dice, { big: true, fresh: ui.fresh, selectable: ui.waitingOnMe && gold && !c.rerolled })
        : emptyDice(5, { big: true });
      turn = `<div class="board-section">
          <h3>${who} — bet ${plural(c.wager, "chip", "chips")}</h3>
          <div class="board-row">${dice}
            ${c.rolled ? `<span class="big-number">${ui.esc(c.hand)}</span>` : ""}</div>
        </div>`;
    } else if (!over && s.phase === "bank" && bank && bank.rolled && ui.waitingOnMe) {
      // Banker re-roll choice: show selectable dice in place of the point.
      point = `<div class="board-section bk-point">
          <h3>Your hand as banker — pick up to ${s.maxReroll} dice to re-roll</h3>
          <div class="board-row">${ui.dice(bank.dice, { big: true, fresh: ui.fresh, selectable: true })}
            <span class="big-number">${ui.esc(bank.hand)}</span></div>
        </div>`;
    }

    // Controls.
    let actions = "";
    if (ui.waitingOnMe && mine) {
      if (s.phase === "rebuy") {
        actions = `<p class="note">You're out of chips. Buy back in (it counts against your final result), or sit out the rest of the game.</p>
          <div class="board-actions">
            ${ui.button(`Buy back in for ${s.start}`, { type: "rebuy" }, { primary: true })}
            ${ui.button("Sit out the rest of the game", { type: "leave" }, { cls: "danger" })}
          </div>`;
      } else if (s.phase === "decide") {
        const w = mine.wager;
        actions = `<p class="note">The banker has <strong>${ui.esc(bank.hand)}</strong> and may still re-roll up to ${s.maxReroll} dice.
            You have <strong>${ui.esc(mine.hand)}</strong>, betting ${plural(w, "chip", "chips")}.</p>
          <div class="board-actions">
            ${ui.button(`Stay at ${w}`, { type: "stay" }, { primary: true })}
            ${ui.button(`Double to ${2 * w}`, { type: "double" }, { cls: "warn", disabled: !mine.canDouble,
              title: mine.canDouble ? "Win or lose twice your bet" : `You need ${2 * w} chips to double` })}
            ${ui.button(`Fold — lose ${mine.foldCost}`, { type: "fold" }, { cls: "danger", title: "Pay half your bet now and sit out the rest of the round" })}
          </div>`;
      } else if (s.phase === "wager") {
        const btns = [];
        for (let a = 1; a <= Math.min(5, mine.maxBet); a++) {
          btns.push(ui.button(`Bet ${a}`, { type: "wager", amount: a }, { primary: a === 1 }));
        }
        actions = `<p class="note">You have ${plural(mine.chips, "chip", "chips")}. Win: banker pays you your bet. Lose or tie: you pay the banker.</p>
          <div class="board-actions">${btns.join("")}</div>`;
      } else if (s.phase === "point") {
        actions = `<div class="board-actions">${ui.button("Roll the point 🎲", { type: "roll" }, { primary: true, roll: true })}</div>`;
      } else if (!mine.rolled) {
        const label = s.phase === "bank" ? "Roll as banker 🎲"
          : gold ? "Roll five dice 🎲" : `Roll to beat ${ui.esc(bank.hand)} 🎲`;
        actions = `<div class="board-actions">${ui.button(label, { type: "roll" }, { primary: true, roll: true })}</div>`;
      } else {
        const n = ui.selected.size;
        const ok = n >= 1 && n <= s.maxReroll;
        actions = `<p class="note">Tap up to ${s.maxReroll} dice, then re-roll them (once only).</p>
          <div class="board-actions">
            ${ui.button(n ? `Re-roll ${plural(n, "die", "dice")}` : "Re-roll selected", { type: "reroll" }, { primary: true, withSelection: true, disabled: !ok })}
            ${ui.button(`Stand on ${ui.esc(mine.hand)}`, { type: "stand" }, { cls: "warn" })}
          </div>`;
      }
    }

    // Last round's result, while the next round's bets go down.
    let last = "";
    if (s.last && (s.phase === "wager" || s.phase === "rebuy" || over)) {
      const L = s.last;
      const net = L.net > 0 ? pill(`bank +${L.net}`, "good") : L.net < 0 ? pill(`bank −${-L.net}`, "bad") : pill("bank even");
      last = `<div class="board-section">
          <h3>Round ${L.round}: ${ui.name(L.banker)} banked with ${ui.esc(L.bankHand)} ${net}</h3>
          ${L.short ? `<p class="note">The bank ran out of chips — ${L.short} in winnings couldn't be paid.</p>` : ""}
          <div class="board-row">${ui.dice(L.bankDice, { small: true })}</div>
          ${L.rows.map((r) => `<div class="board-row">
              <span class="bk-who">${ui.name(r.seat)}</span>${ui.dice(r.dice, { small: true })}
              <span class="note">${ui.esc(r.hand)}</span>
              ${r.choice === "fold" ? pill(`folded, lost ${r.wager}`, "bad")
                : r.result > 0 ? pill(r.short ? `won ${r.paid} of ${r.wager} — the bank ran dry` : `won ${r.paid}`, r.short ? "gold" : "good")
                : pill(`lost ${r.paid}`, "bad")}
              ${r.choice === "double" ? pill("doubled", "gold") : ""}
            </div>`).join("")}
        </div>`;
    }

    const rows = s.seats.map((p, seat) => {
      let bet = "";
      if (seat === s.banker && !over) bet = pill("BANK", "gold");
      else if (p.wager > 0) bet = String(p.wager);
      else if (p.wagered) bet = pill("bet in");
      else if (!p.out && s.phase === "wager") bet = `<span class="note">…</span>`;
      let res = "";
      if (p.choice === "fold") res = pill(`folded −${p.wager}`, "bad");
      else if (p.result > 0) res = pill(`+${p.wager}`, "good");
      else if (p.result < 0) res = pill(`−${p.wager}`, "bad");
      else if (p.choice === "double") res = pill("doubled", "gold");
      else if (p.choice === "stay") res = pill("stays");
      else if (s.phase === "decide" && p.inRound) res = p.decided ? pill("decided ✓") : pill("deciding…");
      let chips = `<strong>${p.chips}</strong>`;
      if (p.out) chips = pill("out", "bad");
      else if (p.gone) chips = pill("sat out", "bad");
      else if (p.rebuying) chips = pill("busted — deciding", "bad");
      else if (p.busted) chips = pill("busted — sitting out", "bad");
      const net = p.net > 0 ? `+${p.net}` : String(p.net);
      return {
        seat,
        cells: [
          chips,
          `${p.buyIns > 1 ? `${p.buyIns}× ` : ""}<span class="note">net</span> ${net}`,
          bet,
          p.rolled ? `${ui.dice(p.dice, { small: true })}<div class="note">${ui.esc(p.hand)}${p.rerolled ? " (re-rolled)" : ""}</div>` : "",
          res,
        ],
      };
    });

    return `${head}${point}${turn}
      ${actions ? `<div class="board-section">${actions}</div>` : ""}
      ${ui.scoreTable(["Chips", "Buy-ins / net", "Bet", "Hand", ""], rows)}
      ${last}`;
  }

  // ── Hundreds ───────────────────────────────────────────────────────────────

  const SQUARES = ["Hundreds", "Tens", "Ones"];

  function hundredsBoard(s, ui, seat, board, opts) {
    const placing = opts.placing;
    const cells = board.map((face, i) => {
      if (face) return `<div class="hb-cell">${ui.die(face, {})}<small>${SQUARES[i]}</small></div>`;
      if (placing) {
        return `<div class="hb-cell">${ui.button(ui.die(s.die, { label: `Put the ${s.die} in the ${SQUARES[i].toLowerCase()}` }),
          { type: "place", index: i }, { cls: "hb-slot" })}<small>${SQUARES[i]}</small></div>`;
      }
      return `<div class="hb-cell">${ui.die(0, { empty: true })}<small>${SQUARES[i]}</small></div>`;
    }).join("");
    const number = board.map((f) => (f ? f : "_")).join("");
    const cls = ["hb-board"];
    if (opts.current) cls.push("current");
    if (opts.highlight) cls.push(opts.highlight);
    return `<div class="${cls.join(" ")}">
        <div class="hb-head"><span class="hb-name">${ui.name(seat)}</span>
          <span class="hb-number">${number}</span>
          ${opts.tokens !== undefined ? dots(opts.tokens) : ""}</div>
        <div class="hb-squares">${cells}</div>
      </div>`;
  }

  function hundreds(s, ui) {
    ui0 = ui;
    const me = ui.mySeat;
    const postman = s.variant === "postman";
    const over = ui.view.over;
    const head = `<div class="board-row">
        ${pill(`Round ${s.round}`)} ${pill(`Pass ${s.pass} of 3`)}
        <span class="hb-mode ${s.low ? "low" : "high"}">${s.low ? "LOW — lowest wins" : "HIGH — highest wins"}</span>
        ${s.low && s.lowBy >= 0 ? `<span class="note">called by ${ui.name(s.lowBy)}</span>` : ""}
      </div>
      ${postman ? `<div class="board-row"><span class="note">Tokens in the middle:</span>${dots(s.pool, "pool")}
        <span class="note">${s.pool > 0 ? "Worst number takes one." : "Middle empty — best number discards one."}</span></div>` : ""}`;

    let current = "";
    if (!over && s.current >= 0) {
      const who = s.current === me ? "You" : ui.name(s.current);
      const dice = s.die ? ui.dice([s.die], { big: true, fresh: ui.fresh }) : emptyDice(1, { big: true });
      let actions = "";
      if (ui.waitingOnMe && !s.die) {
        actions = `<div class="board-actions">
            ${ui.button("Roll 🎲", { type: "roll" }, { primary: true, roll: true })}
            ${ui.button(s.low ? "Already LOW" : "Declare LOW & roll", { type: "low" }, { cls: "warn", disabled: s.low, title: "Lowest number wins this round" })}
          </div>`;
      } else if (ui.waitingOnMe && s.die) {
        actions = `<p class="note">Tap an empty square on your board to place the ${s.die}.</p>`;
      }
      current = `<div class="board-section">
          <div class="board-row">${dice}<span class="note">${s.die ? `${who} rolled a <strong>${s.die}</strong>` : `${who} to roll`}</span></div>
          ${actions}
        </div>`;
    }

    // My board first, then the others in turn order.
    const seats = s.order.slice();
    for (let i = 0; i < s.boards.length; i++) if (!seats.includes(i)) seats.push(i);
    if (me >= 0 && seats.includes(me)) { seats.splice(seats.indexOf(me), 1); seats.unshift(me); }
    const boards = seats.filter((seat) => !s.out[seat]).map((seat) => hundredsBoard(s, ui, seat, s.boards[seat], {
      placing: ui.waitingOnMe && seat === me && s.die > 0,
      current: ui.isWaiting(seat),
      tokens: s.tokens[seat],
    })).join("");

    let last = "";
    if (s.last) {
      const L = s.last;
      const verb = L.kind === "take" ? "took a token (worst number)" : "discarded a token (best number)";
      const lastBoards = L.values.map((v, seat) => (v < 0 ? "" : hundredsBoard(s, ui, seat, L.boards[seat], {
        highlight: seat === L.seat ? (L.kind === "take" ? "bad" : "good") : "",
      }))).join("");
      last = `<details class="board-section" ${s.pass === 1 && !over ? "open" : ""}>
          <summary>Round ${L.round} (${L.low ? "LOW" : "high"}): ${ui.name(L.seat)} ${verb}
            ${L.tied && L.tied.length ? " after a roll-off" : ""}</summary>
          <div class="hb-boards">${lastBoards}</div>
        </details>`;
    }

    const rows = s.tokens.map((t, seat) => ({
      seat,
      cells: [s.out[seat] ? pill("out", "bad") : dots(t), s.out[seat] ? "" : s.boards[seat].map((f) => f || "_").join("")],
    }));
    return `${head}${current}<div class="hb-boards">${boards}</div>${last}
      ${ui.scoreTable([postman ? "Tokens (get to 0)" : "Tokens left", "Number"], rows)}`;
  }

  // ── Three Hands ────────────────────────────────────────────────────────────

  function threeHands(s, ui) {
    ui0 = ui;
    const over = ui.view.over;
    const me = ui.mySeat === 1 ? 1 : 0;   // my half on the left (seat 0 for spectators)
    const opp = 1 - me;
    const placing = ui.waitingOnMe && s.die > 0;
    const small = s.k === 5;

    function area(seat, a, mineSide) {
      const dice = s.areas[seat][a];
      const firstOpen = dice.indexOf(0);
      const canPlace = placing && mineSide && seat === ui.mySeat && firstOpen >= 0;
      const html = dice.map((f, i) => {
        if (f) return ui.die(f, { small });
        if (canPlace && i === firstOpen) return `<span class="th-ghost">${ui.die(s.die, { small })}</span>`;
        return ui.die(0, { empty: true, small });
      }).join("");
      const name = s.hands[seat][a] ? `<div class="th-hand">${ui.esc(s.hands[seat][a])}</div>` : `<div class="th-hand note">${dice.filter((f) => f).length}/${s.k}</div>`;
      if (canPlace) {
        return ui.button(`<span class="dice-row">${html}</span>${name}`, { type: "place", index: a },
          { cls: "th-area open", title: `Put the ${s.die} in hand ${a + 1}` });
      }
      return `<div class="th-area${firstOpen < 0 ? " full" : ""}"><span class="dice-row">${html}</span>${name}</div>`;
    }

    const head = `<div class="board-row">
        ${pill(`Round ${s.round}`)} ${pill(`First to ${s.goal} rounds`)}
        ${pill(`${ui.name(me)} ${s.wins[me]} – ${s.wins[opp]} ${ui.name(opp)}`, "gold")}
        ${pill(s.k === 5 ? "Five-dice hands" : "Three-dice hands")}
      </div>`;

    let current = "";
    if (!over && s.current >= 0) {
      const who = s.current === ui.mySeat ? "You" : ui.name(s.current);
      const dice = s.die ? ui.dice([s.die], { big: true, fresh: ui.fresh }) : emptyDice(1, { big: true });
      let actions = "";
      if (ui.waitingOnMe && !s.die) {
        actions = `<div class="board-actions">${ui.button("Roll 🎲", { type: "roll" }, { primary: true, roll: true })}</div>`;
      } else if (placing) {
        actions = `<p class="note">Tap one of your hands to put the ${s.die} there.</p>`;
      }
      current = `<div class="board-section">
          <div class="board-row">${dice}<span class="note">${s.die ? `${who} rolled a <strong>${s.die}</strong>` : `${who} to roll`}</span></div>
          ${actions}
        </div>`;
    }

    const rows = [0, 1, 2].map((a) => `<div class="th-row">
        ${area(me, a, true)}
        <div class="th-vs">Hand ${a + 1}</div>
        ${area(opp, a, false)}
      </div>`).join("");
    const board = `<div class="board-section th-board">
        <div class="th-row th-names"><div class="${ui.isWaiting(me) ? "turn" : ""}">${ui.name(me)}</div><div class="th-vs"></div>
          <div class="${ui.isWaiting(opp) ? "turn" : ""}">${ui.name(opp)}</div></div>
        ${rows}
      </div>`;

    let last = "";
    if (s.last) {
      const L = s.last;
      const res = (r) => (r < 0 ? pill("tie") : pill(`${ui.name(r)} wins`, r === ui.mySeat ? "good" : "bad"));
      last = `<details class="board-section" ${s.current === s.starter && !s.die && !over ? "open" : ""} ${over ? "open" : ""}>
          <summary>Round ${L.round}: ${L.winner < 0 ? "drawn — replayed" : `${ui.name(L.winner)} won`}</summary>
          ${[0, 1, 2].map((a) => `<div class="th-row">
              <div class="th-area full">${ui.dice(L.areas[me][a], { small: true })}<div class="th-hand">${ui.esc(L.hands[me][a])}</div></div>
              <div class="th-vs">${res(L.results[a])}</div>
              <div class="th-area full">${ui.dice(L.areas[opp][a], { small: true })}<div class="th-hand">${ui.esc(L.hands[opp][a])}</div></div>
            </div>`).join("")}
        </details>`;
    }

    const score = ui.scoreTable(["Rounds won"], [0, 1].map((seat) => ({ seat, cells: [`<strong>${s.wins[seat]}</strong> / ${s.goal}`] })));
    return `${head}${current}${board}${last}${score}`;
  }

  // ── Rule Maker ─────────────────────────────────────────────────────────────

  function ruleMaker(s, ui) {
    ui0 = ui;
    const me = ui.mySeat;
    const over = s.phase === "over";
    const head = `<div class="board-row">${pill(`Round ${s.round}`)} ${pill(`First to ${s.goal}`, "gold")}</div>`;

    let main = "";
    if (s.phase === "roll" || over) {
      const who = s.roller === me ? "You" : s.roller >= 0 ? ui.name(s.roller) : "";
      const dice = s.dice && s.dice.length ? ui.dice(s.dice, { big: true, fresh: ui.fresh }) : emptyDice(1, { big: true });
      const total = s.dice && s.dice.length ? `<span class="note">${who} rolled a</span><span class="big-number">${s.dice[0]}</span>` : "";
      const actions = ui.waitingOnMe
        ? `<div class="board-actions">${ui.button("Roll 🎲", { type: "roll" }, { primary: true, roll: true })}</div>` : "";
      main = `<div class="board-section">
          <div class="board-row">${dice}${total}</div>
          ${!over && s.current >= 0 ? `<p class="note">${s.current === me ? "Your roll" : `${ui.name(s.current)} rolls next`} — highest roll this round proposes a house rule.</p>` : ""}
          ${actions}
        </div>`;
    } else if (s.phase === "propose") {
      if (ui.waitingOnMe) {
        main = `<div class="board-section">
            <h3>You rolled highest — make a house rule!</h3>
            <form class="rm-form" data-act-form='{"type":"propose"}'>
              <input type="text" name="text" maxlength="${s.maxText}" required autocomplete="off"
                placeholder="e.g. Doubles score double">
              <button type="submit" class="btn primary">Propose</button>
            </form>
            <div class="board-actions">${ui.button("Pass — no new rule", { type: "pass" }, { cls: "warn" })}</div>
            <p class="note">Up to ${s.maxText} characters. Everyone votes; a majority adopts it. Rules are on the honour system.</p>
          </div>`;
      } else {
        main = `<div class="board-section"><h3>${ui.name(s.proposer)} rolled highest and is writing a house rule…</h3></div>`;
      }
    } else if (s.phase === "vote") {
      const voted = s.myVote !== 0;
      const controls = ui.waitingOnMe
        ? `<div class="board-actions rm-vote">
            ${ui.button("👍 Yes", { type: "vote", yes: true }, { primary: true })}
            ${ui.button("👎 No", { type: "vote", yes: false }, { cls: "danger" })}
          </div>`
        : voted ? `<p class="note">You voted <strong>${s.myVote === 1 ? "Yes" : "No"}</strong>.</p>` : "";
      main = `<div class="board-section">
          <h3>${s.proposer === me ? "You propose" : `${ui.name(s.proposer)} proposes`}:</h3>
          <blockquote class="rm-proposal">${ui.esc(s.proposal)}</blockquote>
          ${controls}
          <p class="note">${s.votesCast} of ${s.voters} votes in. Needs more than half to pass.</p>
        </div>`;
    }

    let lastVote = "";
    if (s.lastVote && s.phase !== "vote") {
      const L = s.lastVote;
      lastVote = L.passed
        ? `<p class="note">Round ${L.round}: ${ui.name(L.by)} passed on making a rule.</p>`
        : `<div class="board-section"><h3>Round ${L.round} vote: ${L.adopted ? pill(`adopted ${L.yes.length}–${L.no.length}`, "good") : pill(`rejected ${L.yes.length}–${L.no.length}`, "bad")}</h3>
            <blockquote class="rm-proposal small">${ui.esc(L.text)}</blockquote>
            <p class="note">Yes: ${nameList(ui, L.yes)} · No: ${nameList(ui, L.no)}</p></div>`;
    }

    const rules = `<div class="board-section">
        <h3>House rules (${s.rules.length})</h3>
        ${s.rules.length ? `<ol class="rm-rules">${s.rules.map((r) => `<li>${ui.esc(r.text)} <span class="note">— ${ui.name(r.by)}, round ${r.round}</span></li>`).join("")}</ol>`
          : `<p class="note">No house rules yet.</p>`}
        <p class="note">The app can't enforce house rules — play them on your honour.</p>
      </div>`;

    const rows = s.scores.map((score, seat) => {
      const pct = Math.min(100, (score / s.goal) * 100);
      let status = "";
      if (s.out[seat]) status = pill("out", "bad");
      else if (s.phase === "vote") status = s.voted[seat] ? pill("voted ✓", "good") : pill("voting…");
      else if (s.rolls[seat]) status = `+${s.rolls[seat]}`;
      return {
        seat,
        cells: [`<span class="big-number">${score}</span>`, status,
          `<div class="progress"><span class="banked" style="width:${pct}%"></span></div>`],
      };
    });
    return `${head}${main}${lastVote}${ui.scoreTable([`Score (to ${s.goal})`, "This round", ""], rows)}${rules}`;
  }

  // Wrap each board so this stylesheet's rules stay scoped to these games.
  function wrap(fn) {
    return { board: (s, ui) => `<div class="bd-game">${fn(s, ui)}</div>` };
  }
  GameUI.register("klondike", wrap(banker));
  GameUI.register("hundreds", wrap(hundreds));
  GameUI.register("threehands", wrap(threeHands));
  GameUI.register("rulemaker", wrap(ruleMaker));
})();
