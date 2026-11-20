// doubleorquits.js — Double or Quits.  See games.js for the renderer API.
//
// state (from app/games/bluff/doubleorquits.go): {round, rounds, pot, chips,
//   passed, thrower, phase ("throw"|"respond"), responder, drawn,
//   dice (0 = hidden from me), exposed, mine, claim, claimDice,
//   established, estBy, last}

(function () {
  function faceSelects(dice) {
    const sorted = dice.slice().sort((a, b) => b - a);
    return sorted.map((v, i) => `<select name="dice${i}" data-num aria-label="Die ${i + 1}">` +
      [6, 5, 4, 3, 2, 1].map((f) => `<option value="${f}"${f === v ? " selected" : ""}>${f}</option>`).join("") +
      `</select>`).join("");
  }

  function hand(s, ui) {
    // The thrower's five dice: exposed ones are public; hidden ones only
    // the thrower sees (everyone else gets 0 → a hidden die).
    const selectable = s.mine && s.phase === "throw" && !s.drawn && ui.waitingOnMe;
    return `<div class="board-row doq-hand">` + s.dice.map((v, i) => {
      const exposed = s.exposed[i];
      return `<span class="doq-die${exposed ? " exposed" : ""}">` +
        ui.die(v, { big: true, fresh: ui.fresh, hidden: v === 0, selectable, index: i, kept: exposed && !selectable }) +
        `<small>${exposed ? "exposed" : (s.mine ? "hidden" : "")}</small></span>`;
    }).join("") + `</div>`;
  }

  function board(s, ui) {
    const myTurn = ui.waitingOnMe;
    let html = `<div class="board-section">
      <div class="board-row">
        <span class="note">Pot</span><span class="big-number doq-pot">${s.pot}</span>
        <span class="pill">Round ${Math.min(s.round, s.rounds)} of ${s.rounds}</span>
        ${s.established ? `<span class="pill gold">To beat: ${ui.esc(s.established.name)} (${ui.name(s.estBy)})</span>` : ""}
      </div>`;

    if (s.phase === "respond" && s.claim) {
      html += `<div class="doq-claim"><span class="note">${ui.name(s.thrower)} claims</span>
        <span class="doq-claim-name">${ui.esc(s.claim.name)}</span></div>`;
    }

    html += `<p class="note">${s.mine ? "Your dice — only exposed dice are visible to others." : `${ui.name(s.thrower)}'s dice`}</p>`;
    html += hand(s, ui);

    if (myTurn && s.phase === "throw") {
      if (!s.drawn) {
        html += `<p class="note">Redraw (once): tap dice to expose them, then re-roll the exposed or the hidden ones — or keep them all.</p>
          <div class="board-actions">
            ${ui.button("Re-roll hidden", { type: "draw", index: 1 }, { primary: true, roll: true, withSelection: true })}
            ${ui.button("Re-roll exposed", { type: "draw", index: 2 }, { withSelection: true, disabled: ui.selected.size === 0 })}
            ${ui.button("Keep — no re-roll", { type: "draw", index: 0 }, { withSelection: true })}
          </div>`;
      }
      html += `<div class="board-actions"><form data-act-form='{"type":"claim"}'>
          <span class="note">Claim:</span> ${faceSelects(s.dice)}
          <button class="btn primary" type="submit">Claim this hand</button>
        </form>
        ${s.established ? ui.button(`Pass — sit out this round`, { type: "pass" }, { cls: "danger" }) : ""}
      </div>`;
      if (s.established) html += `<p class="note">You must claim better than ${ui.esc(s.established.name)}.</p>`;
    }

    if (myTurn && s.phase === "respond") {
      html += `<div class="board-actions">
        ${ui.button("Believe it", { type: "believe" }, { primary: true })}
        ${ui.button(`Challenge — if it's true you pay ${s.pot}`, { type: "challenge" }, { cls: "danger" })}
      </div>`;
    }
    html += `</div>`;

    if (s.last) {
      const l = s.last;
      html += `<div class="board-section doq-last"><h3>Last challenge</h3>
        <div class="board-row">${ui.name(l.challenger)} challenged ${ui.name(l.claimant)}'s ${ui.esc(l.claim.name)}:
          ${ui.dice(l.dice.slice().sort((a, b) => b - a), { small: true })} <strong>${ui.esc(l.actual.name)}</strong></div>
        <p class="note">${l.honest
          ? `It was there — ${ui.name(l.challenger)} paid ${l.paid} and ${ui.name(l.claimant)} took the pot of ${l.won}.`
          : `A bluff — ${ui.name(l.claimant)} paid ${l.paid} to double the pot, which carried over.`}</p></div>`;
    }

    const rows = s.chips.map((c, seat) => {
      let status = "";
      if (ui.view.removed[seat]) status = "";
      else if (s.passed[seat]) status = `<span class="pill bad">passed</span>`;
      else if (seat === s.thrower) status = s.phase === "respond" ? `<span class="pill gold">claiming</span>` : `<span class="pill gold">throwing</span>`;
      else if (seat === s.estBy) status = `<span class="pill good">claim stands</span>`;
      return { seat, cells: [`<span class="big-number">${c}</span>`, status] };
    });
    html += ui.scoreTable(["Chips", "This round"], rows);
    return html;
  }

  GameUI.register("doubleorquits", { board });
})();
