// octo.js — Octo.  See games.js for the renderer API.
//
// state (from app/games/pushluck/octo.go): {round, rounds, current, roll,
//   kept: [{face, count}], turnTotal, totals, scores: [round][seat] (-1 = not played)}

(function () {
  function board(s, ui) {
    const used = new Set(s.kept.map((k) => k.face));
    const myTurn = ui.waitingOnMe;
    let html = `<div class="board-section">
      <div class="board-row"><span class="pill">Round ${s.round} of ${s.rounds}</span>
        ${s.turnTotal ? `<span class="note">This turn</span><span class="big-number">${s.turnTotal}</span>` : ""}</div>`;

    // Numbers already set aside this turn — they can't be taken again.
    if (s.kept.length) {
      html += `<div class="board-row octo-kept"><span class="note">Set aside:</span>` +
        s.kept.map((k) => `<span class="octo-group">${ui.dice(new Array(k.count).fill(k.face), { small: true, kept: true })}</span>`).join("") +
        `</div>`;
    }

    // The live roll.  Dice of an already-used number are faded.
    if (s.roll && s.roll.length) {
      html += `<div class="board-row">` + s.roll.map((v, i) => ui.die(v, {
        big: true, fresh: ui.fresh, faded: used.has(v),
        selectable: myTurn && !used.has(v), index: i,
      })).join("") + `</div>`;
    }

    if (myTurn) {
      if (!s.roll || !s.roll.length) {
        html += `<div class="board-actions">${ui.button("Roll all eight 🎲", { type: "roll" }, { primary: true, roll: true })}</div>`;
      } else {
        const sel = Array.from(ui.selected);
        const faces = new Set(sel.map((i) => s.roll[i]));
        const valid = sel.length > 0 && faces.size === 1;
        const face = valid ? s.roll[sel[0]] : 0;
        const left = s.roll.length - sel.length;
        const gain = valid ? face * sel.length : 0;
        html += `<p class="note">${valid
          ? `Setting aside ${sel.length}×${face} for +${gain}.`
          : sel.length ? "Pick dice of just one number." : "Tap dice of one new number to set them aside (tapping another number switches to it)."}</p>`;
        html += `<div class="board-actions">
          ${ui.button(valid && left ? `Set aside & roll ${left}` : "Set aside & roll", { type: "keep_roll" },
            { primary: true, roll: true, withSelection: true, disabled: !valid || left === 0 })}
          ${ui.button(valid ? `Set aside & stop for ${s.turnTotal + gain}` : "Set aside & stop", { type: "keep_stop" },
            { cls: "warn", withSelection: true, disabled: !valid })}
        </div>`;
      }
    }
    html += `</div>`;

    // Scoreboard: one column per round.
    const headers = [];
    for (let r = 0; r < s.rounds; r++) headers.push(`R${r + 1}`);
    headers.push("Total");
    const rows = s.totals.map((total, seat) => ({
      seat,
      cells: s.scores.map((row) => row[seat] < 0 ? "" : (row[seat] === 0 ? `<span class="pill bad">0</span>` : String(row[seat])))
        .concat(new Array(s.rounds - s.scores.length).fill(""))
        .concat([`<span class="big-number">${total}</span>`]),
    }));
    html += ui.scoreTable(headers, rows);
    return html;
  }

  // Only one number can be set aside at a time: tapping a die of another
  // number switches the selection to it.
  function onSelect(index, selected, s) {
    if (!selected.has(index) || !s.roll) return;
    const face = s.roll[index];
    Array.from(selected).forEach((i) => { if (s.roll[i] !== face) selected.delete(i); });
  }

  GameUI.register("octo", { board, onSelect });
})();
