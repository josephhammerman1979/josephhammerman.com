// pushluck.js — Ten Thousand, Midnight, Sevens Out, Sixteen and Threes (see
// games.js for the renderer API; state shapes are the *View structs in
// app/games/pushluck/*.go).

(function () {
  // ── Shared pieces ────────────────────────────────────────────────────────

  function selection(ui, n) {
    return Array.from(ui.selected).filter((i) => i >= 0 && i < n).sort((a, b) => a - b);
  }

  function range(n) {
    const out = [];
    for (let i = 0; i < n; i++) out.push(i);
    return out;
  }

  function pill(text, cls) {
    return `<span class="pill${cls ? " " + cls : ""}">${text}</span>`;
  }

  function plural(n, one, many) { return `${n} ${n === 1 ? one : many}`; }

  function comma(n) { return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ","); }

  function signed(n) { return n > 0 ? `+${n}` : String(n); }

  // ── Chips (Threes, 1-4-24): stacks, all-in, busting and rebuys ──────────

  function chipCell(s, seat) {
    if (s.gone[seat]) return pill("sat out", "bad");
    if (s.rebuy[seat]) return pill("busted — deciding", "bad");
    let html = `<span class="big-number">${s.chips[seat]}</span>`;
    if (s.chips[seat] === 0 && !s.over) html = pill("busted — sitting out", "bad");
    if (s.allIn[seat] && !s.over) html += " " + pill("all-in", "gold");
    return html;
  }

  function netCell(s, seat) {
    return `${s.buyIns[seat] > 1 ? `${s.buyIns[seat]}× ` : ""}<span class="note">net</span> ${signed(s.net[seat])}`;
  }

  // Between hands, busted players buy back in or sit out.  Returns the
  // whole board for that pause, or "" when play is on.
  function rebuyBoard(s, ui, table) {
    if (!s.rebuying) return "";
    const busted = s.rebuy.map((r, seat) => (r ? seat : -1)).filter((x) => x >= 0);
    return `<div class="board-section">
        <h3>Out of chips: ${busted.map((x) => ui.name(x)).join(", ")}</h3>
        <p class="note">The next hand starts once they buy back in or sit out. Buying back in counts against your final result.</p>
        ${ui.waitingOnMe ? `<div class="board-actions">
          ${ui.button(`Buy back in for ${s.start}`, { type: "rebuy" }, { primary: true })}
          ${ui.button("Sit out the rest of the game", { type: "leave" }, { cls: "danger" })}
        </div>` : ""}
      </div>${table}`;
  }

  function smallDice(faces, opts) {
    return (faces || []).map((f) => ui_die(f, Object.assign({ small: true }, opts || {}))).join("");
  }
  let ui_die = null; // set per render so helpers can draw dice

  function emptyDice(ui, n) {
    return range(n).map(() => ui.die(0, { big: true, empty: true })).join("");
  }

  // The live roll: big, tappable when it's my turn.
  function liveRoll(s, ui, faces) {
    return ui.dice(faces, { big: true, fresh: ui.fresh, selectable: ui.waitingOnMe });
  }

  // Whose turn line for the top of the board.
  function turnLine(ui, seat, extra) {
    if (seat < 0) return "";
    const who = seat === ui.mySeat ? "Your turn" : `${ui.name(seat)}'s turn`;
    return `<div class="pl-turn">${who}${extra ? ` <span class="note">${extra}</span>` : ""}</div>`;
  }

  // ── Ten Thousand ────────────────────────────────────────────────────────

  function ttScore(d) {
    const c = [0, 0, 0, 0, 0, 0, 0];
    d.forEach((f) => { c[f]++; });
    let pts = 0;
    for (let f = 1; f <= 6; f++) {
      let n = c[f];
      if (n >= 3) {
        pts += Math.floor(n / 3) * (f === 1 ? 1000 : f * 100);
        n %= 3;
      }
      if (f === 1) pts += 100 * n;
      else if (f === 5) pts += 50 * n;
      else if (n > 0) return -1;
    }
    return d.length ? pts : -1;
  }

  function tenThousand(s, ui) {
    ui_die = ui.die;
    const rolled = s.roll && s.roll.length > 0;
    const myTurn = ui.waitingOnMe;

    // Status: turn total and what's been set aside.
    const aside = s.aside.map((g, i) =>
      `<span class="pl-group">${smallDice(g, { kept: true })}<span class="pill gold">+${s.asidePts[i]}</span></span>`).join("");
    const status = `
      <div class="board-row pl-status">
        <span class="pl-stat"><span class="pl-label">Turn total</span><span class="big-number">${comma(s.turnTotal)}</span></span>
        ${aside ? `<span class="pl-aside"><span class="pl-label">Set aside</span>${aside}</span>` : ""}
      </div>`;

    let diceHTML;
    if (rolled) {
      diceHTML = liveRoll(s, ui, s.roll);
    } else {
      diceHTML = emptyDice(ui, s.toRoll);
    }
    let hint = "";
    if (rolled) {
      hint = s.scoring.length
        ? `<p class="note">Scoring dice in this roll: ${s.scoring.map((i) => s.roll[i]).join(" ")} (1 = 100, 5 = 50, three of a kind = face × 100, three 1s = 1000)</p>`
        : "";
    } else if (s.toRoll === 6 && s.rolls > 0) {
      hint = `<p class="note">Hot dice — all six roll again!</p>`;
    }

    let actions = "";
    if (myTurn && !rolled) {
      actions = ui.button(`Roll ${s.toRoll} dice 🎲`, { type: "roll" }, { primary: true, roll: true });
    } else if (myTurn) {
      const sel = selection(ui, s.roll.length);
      const selPts = sel.length ? ttScore(sel.map((i) => s.roll[i])) : -1;
      const all = s.scoring;
      const allPts = ttScore(all.map((i) => s.roll[i]));
      const left = (n) => (s.roll.length - n === 0 ? 6 : s.roll.length - n);
      const selOK = selPts > 0;
      let selRow;
      if (!sel.length) {
        selRow = `<span class="note">Tap scoring dice to choose them yourself, or take them all:</span>`;
      } else if (!selOK) {
        selRow = `<span class="pill bad">Every die you set aside must score</span>`;
      } else {
        selRow =
          ui.button(`Set aside +${selPts} & roll ${left(sel.length)}${left(sel.length) === 6 && sel.length === s.roll.length ? " (hot dice!)" : ""}`,
            { type: "keep_roll" }, { primary: true, roll: true, withSelection: true }) +
          ui.button(`Set aside & bank ${comma(s.turnTotal + selPts)}`, { type: "keep_bank" }, { cls: "warn", withSelection: true });
      }
      const allRow =
        ui.button(`Take all scoring +${allPts} & roll ${left(all.length)}`, { type: "keep_roll", dice: all },
          { primary: !selOK, roll: !selOK }) +
        ui.button(`Take all & bank ${comma(s.turnTotal + allPts)}`, { type: "keep_bank", dice: all }, { cls: "warn" });
      actions = `<div class="board-actions">${selRow}</div><div class="board-actions">${allRow}</div>`;
    }

    let last = "";
    if (!rolled && s.last && !s.over) {
      last = `<div class="board-section pl-last${s.last.bad ? " bad" : ""}"><h3>Last turn</h3>
        <div class="board-row"><strong>${ui.name(s.last.seat)}</strong> ${smallDice(s.last.dice)}
        ${pill(ui.esc(s.last.text), s.last.bad ? "bad" : "good")}</div></div>`;
    }

    const rows = s.totals.map((total, seat) => {
      const risk = seat === s.current && !s.over ? s.turnTotal : 0;
      const banked = Math.min(100, (total / s.target) * 100);
      const riskPct = Math.min(100 - banked, (risk / s.target) * 100);
      return {
        seat,
        cells: [
          `<span class="big-number">${comma(total)}</span>`,
          risk ? pill(`+${comma(risk)}`, "gold") : "",
          `<div class="progress"><span class="banked" style="width:${banked}%"></span>` +
            `<span class="at-risk" style="width:${riskPct}%"></span></div>`,
        ],
      };
    });

    return `
      <div class="board-section">
        ${s.over ? "" : turnLine(ui, s.current, `first to ${comma(s.target)}`)}
        ${status}
        <div class="board-row pl-dice">${diceHTML}</div>
        ${hint}
        ${actions ? `<div class="pl-actions">${actions}</div>` : ""}
      </div>
      ${last}
      ${ui.scoreTable([`Banked (to ${comma(s.target)})`, "At risk", ""], rows)}`;
  }

  // ── Keep-the-dice games: Midnight, Sixteen, Threes ──────────────────────

  // keepBoard draws the shared layout: header pills, the kept dice, the live
  // roll, the keep buttons, a game-specific status line and the last round.
  function keepBoard(s, ui, o) {
    ui_die = ui.die;
    const myTurn = ui.waitingOnMe;
    const rolled = s.roll && s.roll.length > 0;
    const left = o.numDice - s.kept.length;

    const kept = s.kept.length
      ? `<span class="pl-kept">${s.kept.map((f) => ui.die(f, { kept: true })).join("")}</span>`
      : `<span class="note">Nothing kept yet</span>`;
    const live = rolled ? liveRoll(s, ui, s.roll) : emptyDice(ui, left);

    let actions = "";
    if (myTurn && !rolled) {
      actions = ui.button(`Roll ${plural(left, "die", "dice")} 🎲`, { type: "roll" }, { primary: true, roll: true });
    } else if (myTurn) {
      const sel = selection(ui, s.roll.length);
      const rest = s.roll.length - sel.length;
      const label = !sel.length ? "Tap dice to keep"
        : rest === 0 ? `Keep all ${sel.length} & finish`
        : `Keep ${sel.length} & roll ${rest}`;
      actions = ui.button(label, { type: "keep" }, { primary: true, roll: sel.length > 0, withSelection: true, disabled: !sel.length }) +
        (s.roll.length > 1
          ? ui.button(`Keep all ${s.roll.length} & finish`, { type: "keep", dice: range(s.roll.length) }, { cls: "warn" })
          : "");
      if (o.preview) {
        const p = o.preview(sel.map((i) => s.roll[i]));
        if (p) actions += `<span class="note">${p}</span>`;
      }
    }

    const lastRound = s.last && (!rolled || s.over) && s.kept.length === 0 ? o.lastHTML(s.last) : "";

    return `
      <div class="board-section">
        <div class="board-row">${o.pills}</div>
        ${s.over ? "" : turnLine(ui, s.current, o.turnNote || "")}
        <div class="board-row pl-status">${o.status}</div>
        <div class="pl-kept-row"><span class="pl-label">Kept</span>${kept}</div>
        <div class="board-row pl-dice">${live}</div>
        ${actions ? `<div class="board-actions">${actions}</div>` : ""}
      </div>
      ${lastRound}
      ${o.table}`;
  }

  function lastRoundHTML(ui, title, res, cell) {
    const rows = [];
    res.hands.forEach((h, seat) => {
      if (!h) return;
      const win = res.winner === seat;
      rows.push(`<div class="board-row pl-result${win ? " win" : ""}"><strong>${ui.name(seat)}</strong>
        ${h.map((f) => ui.die(f, { small: true })).join("")} ${cell(seat)}${win ? " " + pill("🏆 winner", "good") : ""}</div>`);
    });
    return `<div class="board-section pl-last"><h3>${title}</h3>${rows.join("")}
      ${res.winner < 0 ? `<p class="note">No winner.</p>` : ""}</div>`;
  }

  // ── Midnight ─────────────────────────────────────────────────────────────

  function midnightInfo(kept) {
    const has1 = kept.indexOf(1) >= 0;
    const has4 = kept.indexOf(4) >= 0;
    // The score excludes one 1 and one 4.
    let sum = kept.reduce((a, b) => a + b, 0);
    if (has1) sum -= 1;
    if (has4) sum -= 4;
    return { has1, has4, sum };
  }

  function midnight(s, ui) {
    const info = midnightInfo(s.kept);
    const status = s.over ? "" : `
      ${pill(info.has1 ? "1 ✓ secured" : "1 needed", info.has1 ? "good" : "bad")}
      ${pill(info.has4 ? "4 ✓ secured" : "4 needed", info.has4 ? "good" : "bad")}
      <span class="pl-stat"><span class="pl-label">Points (other dice)</span><span class="big-number">${info.sum}</span></span>
      <span class="note">max 24</span>`;
    const best = bestScore(s.scores, (v) => v >= 0, (a, b) => a > b);
    const rows = s.chips.map((c, seat) => ({
      seat,
      cells: [
        chipCell(s, seat),
        netCell(s, seat),
        s.wins[seat] ? plural(s.wins[seat], "win", "wins") : "",
        s.hands[seat] ? s.hands[seat].map((f) => ui.die(f, { small: true })).join("") : "",
        !s.hands[seat] ? "" : s.scores[seat] < 0 ? pill("no 1-4", "bad")
          : pill(String(s.scores[seat]), s.scores[seat] === best ? "good" : ""),
      ],
    }));
    const midTable = ui.scoreTable(["Chips", "Buy-ins / net", "Rounds", "This round", "Score"], rows);
    const paused = rebuyBoard(s, ui, midTable);
    if (paused) return paused;
    return keepBoard(s, ui, {
      numDice: 6,
      pills: pill(`Round ${s.round} of ${s.rounds}`, "gold") +
        (s.over ? "" : `<span class="pl-pot">Pot <strong>${s.pot}</strong></span>`) +
        (!s.over && s.carried ? pill(`${s.carried} carried over`, "gold") : "") +
        (best !== null && !s.over ? pill(`Best so far: ${best}`) : ""),
      status,
      preview: (faces) => {
        if (!faces.length) return "";
        const i = midnightInfo(s.kept.concat(faces));
        return `→ ${i.has1 && i.has4 ? `${i.sum} points` : "still needs " + [i.has1 ? "" : "a 1", i.has4 ? "" : "a 4"].filter(Boolean).join(" and ")}`;
      },
      lastHTML: (res) => lastRoundHTML(ui, res.winner < 0
          ? `Round ${res.round}: nobody qualified — pot carried over`
          : `Round ${res.round}: pot of ${res.pot} won`, res,
        (seat) => res.scores[seat] < 0 ? pill("no 1-4", "bad") : pill(String(res.scores[seat]))),
      table: midTable,
    });
  }

  function bestScore(scores, ok, better) {
    let best = null;
    scores.forEach((v) => { if (ok(v) && (best === null || better(v, best))) best = v; });
    return best;
  }

  // ── Sixteen ──────────────────────────────────────────────────────────────

  function sixteen(s, ui) {
    const total = s.kept.reduce((a, b) => a + b, 0);
    const toGo = s.goal - total;
    const status = s.over ? "" : `
      <span class="pl-stat"><span class="pl-label">Total</span><span class="big-number">${total}</span></span>
      ${pill(toGo >= 0 ? `${toGo} to 16` : `${-toGo} over`, toGo >= 0 ? "gold" : "bad")}
      <div class="progress pl-sixteen"><span class="banked" style="width:${Math.min(100, (total / s.goal) * 100)}%"></span></div>`;
    const okScores = s.scores.map((v, i) => (v >= 0 && !s.bust[i] ? v : -1));
    const best = bestScore(okScores, (v) => v >= 0, (a, b) => a > b);
    const need = Math.floor(s.rounds / 2) + 1;
    const rows = s.wins.map((w, seat) => ({
      seat,
      cells: [
        `<span class="big-number">${w}</span>`,
        s.hands[seat] ? s.hands[seat].map((f) => ui.die(f, { small: true })).join("") : "",
        !s.hands[seat] ? "" : s.bust[seat] ? pill(`${s.scores[seat]} — out`, "bad")
          : pill(String(s.scores[seat]), s.scores[seat] === best ? "good" : ""),
      ],
    }));
    return keepBoard(s, ui, {
      numDice: 3,
      pills: pill(`Round ${s.round} of ${s.rounds}`, "gold") + pill(`${need} wins takes the match`) +
        (best !== null && !s.over ? pill(`Best so far: ${best}`) : ""),
      status,
      preview: (faces) => {
        if (!faces.length) return "";
        const t = total + faces.reduce((a, b) => a + b, 0);
        const rest = 3 - s.kept.length - faces.length;
        return t + rest > s.goal ? `→ ${t}${rest ? ` + ${rest} more` : ""}: <strong>over 16!</strong>` : `→ ${t}${rest ? ` with ${plural(rest, "die", "dice")} to roll` : ""}`;
      },
      lastHTML: (res) => lastRoundHTML(ui, `Round ${res.round} result`, res,
        (seat) => res.bust && res.bust[seat] ? pill(`${res.scores[seat]} — out`, "bad") : pill(String(res.scores[seat]))),
      table: ui.scoreTable(["Round wins", "This round", "Total"], rows),
    });
  }

  // ── Threes ───────────────────────────────────────────────────────────────

  function threesScore(d) { return d.reduce((a, f) => a + (f === 3 ? 0 : f), 0); }

  function threes(s, ui) {
    const score = threesScore(s.kept);
    const status = s.over ? "" : `
      <span class="pl-stat"><span class="pl-label">Score (low wins)</span><span class="big-number">${score}</span></span>
      <span class="note">3s count as 0 · five 6s shoots the moon</span>`;
    const playing = new Set(s.playing);
    const best = bestScore(s.scores, (v) => v >= 0, (a, b) => a < b);
    const rows = s.chips.map((c, seat) => ({
      seat,
      cells: [
        chipCell(s, seat),
        netCell(s, seat),
        s.seatHands[seat] ? s.seatHands[seat].map((f) => ui.die(f, { small: true })).join("") : "",
        s.seatHands[seat] ? pill(String(s.scores[seat]), s.scores[seat] === best ? "good" : "")
          : (!s.over && !playing.has(seat) ? pill("sitting out", "") : ""),
      ],
    }));
    const thrTable = ui.scoreTable(["Chips", "Buy-ins / net", "This hand (kept)", "Score"], rows);
    const paused = rebuyBoard(s, ui, thrTable);
    if (paused) return paused;
    return keepBoard(s, ui, {
      numDice: 5,
      pills: pill(`Hand ${s.hand} of ${s.hands}`, "gold") + `<span class="pl-pot">Pot <strong>${s.pot}</strong></span>` +
        (s.replay ? pill(`Tie replay${s.replay > 1 ? " ×" + s.replay : ""}`, "bad") : "") +
        (s.rolls ? pill(`Roll ${s.rolls} of ${s.maxRolls}`) : "") +
        (best !== null && !s.over ? pill(`Low so far: ${best}`) : ""),
      status,
      preview: (faces) => faces.length ? `→ score ${score + threesScore(faces)}` : "",
      lastHTML: (res) => lastRoundHTML(ui, `Hand ${res.round}${res.winner < 0 ? " — tie" : " result"}`, res,
        (seat) => pill(String(res.scores[seat]))),
      table: thrTable,
    });
  }

  // ── Sevens Out ───────────────────────────────────────────────────────────

  function sevensDice(ui, roll, gone, opts) {
    return `<span class="dice-row">` + roll.map((f, i) => gone[i]
      ? `<span class="pl-gone" title="removed: pair adds to 7">${ui.die(f, opts)}</span>`
      : ui.die(f, opts)).join("") + `</span>`;
  }

  function sevensOut(s, ui) {
    ui_die = ui.die;
    const myTurn = ui.waitingOnMe;
    const rolled = s.rolls > 0;

    const slots = range(3).map((i) => {
      const cls = i >= s.maxRolls ? "off" : i < s.rolls ? "used" : "open";
      return `<span class="pl-slot ${cls}">${i + 1}</span>`;
    }).join("");
    const limitNote = s.limit === 0
      ? "First player: up to 3 rolls — however many you take is everyone's limit"
      : `Limit this round: ${plural(s.limit, "roll", "rolls")} (set by ${ui.name(s.first)})`;

    const dice = rolled
      ? sevensDice(ui, s.roll, s.gone, { big: true, fresh: ui.fresh })
      : emptyDice(ui, 6);

    let actions = "";
    if (myTurn) {
      const left = s.maxRolls - s.rolls;
      actions = rolled
        ? ui.button(`Roll ${plural(s.live.length, "die", "dice")} again (${plural(left, "roll", "rolls")} left)`, { type: "roll" }, { primary: true, roll: true }) +
          ui.button(`Stop & score ${s.showing}`, { type: "stop" }, { cls: "warn" })
        : ui.button("Roll 6 dice 🎲", { type: "roll" }, { primary: true, roll: true });
    }

    let last = "";
    if (!rolled && s.last && !s.over) {
      last = `<div class="board-section pl-last"><h3>Last turn</h3><div class="board-row">
        <strong>${ui.name(s.last.seat)}</strong> ${sevensDice(ui, s.last.roll, s.last.gone, { small: true })}
        ${pill(`scored ${s.last.score}`, s.last.score ? "good" : "bad")}
        <span class="note">after ${plural(s.last.rolls, "roll", "rolls")}</span></div></div>`;
    }

    const rows = s.totals.map((t, seat) => ({
      seat,
      cells: [
        `<span class="big-number">${t}</span>`,
        s.scores[seat] >= 0 ? pill(`+${s.scores[seat]}`, s.scores[seat] ? "good" : "bad")
          : seat === s.current && rolled ? pill(`+${s.showing}?`, "gold") : "",
        `<div class="progress"><span class="banked" style="width:${Math.min(100, (t / s.goal) * 100)}%"></span></div>`,
      ],
    }));

    return `
      <div class="board-section">
        <div class="board-row">${pill(`Round ${s.round}`, "gold")} ${pill(`Game ends after a round where someone has ${s.goal}`)}</div>
        ${s.over ? "" : turnLine(ui, s.current, "")}
        ${s.over ? "" : `<div class="board-row pl-limit"><span class="pl-label">Rolls</span>${slots}<span class="note">${limitNote}</span></div>`}
        <div class="board-row pl-dice">${dice}</div>
        ${rolled ? `<div class="board-row pl-status"><span class="pl-stat"><span class="pl-label">Showing</span><span class="big-number">${s.showing}</span></span>
          <span class="note">${plural(s.live.length, "die", "dice")} still in play · pairs adding to 7 are removed</span></div>` : ""}
        ${actions ? `<div class="board-actions">${actions}</div>` : ""}
      </div>
      ${last}
      ${ui.scoreTable([`Total (to ${s.goal})`, "This round", ""], rows)}`;
  }

  GameUI.register("tenthousand", { board: tenThousand });
  GameUI.register("midnight", { board: midnight });
  GameUI.register("sixteen", { board: sixteen });
  GameUI.register("threes", { board: threes });
  GameUI.register("sevensout", { board: sevensOut });
})();
