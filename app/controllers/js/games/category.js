// category.js — Catego.  See games.js for the renderer API.
//
// state (from app/games/category/catego.go): {round, rounds, current, dice,
//   sheet: [seat][col-firstCol], leaders: [col-firstCol] (seat or -1),
//   points: [seat], over, firstCol}

(function () {
  function board(s, ui) {
    const cols = s.sheet.length ? s.sheet[0].length : 11;
    const placing = ui.waitingOnMe && s.dice && s.dice.length;
    const sum = s.dice && s.dice.length ? s.dice[0] + s.dice[1] : 0;

    // Dice and the decision.
    let top = `<div class="board-row">`;
    if (s.dice && s.dice.length) {
      top += ui.dice(s.dice, { big: true, fresh: ui.fresh }) +
        `<span class="big-number">= ${sum}</span>`;
    } else {
      top += ui.die(0, { big: true, empty: true }) + ui.die(0, { big: true, empty: true });
    }
    top += `<span class="pill">Round ${Math.min(s.round, s.rounds)} of ${s.rounds}</span></div>`;
    if (ui.waitingOnMe && !placing) {
      top += `<div class="board-actions">${ui.button("Roll 🎲", { type: "roll" }, { primary: true, roll: true })}</div>`;
    } else if (placing) {
      top += `<p class="note">Tap one of your empty columns below to write <strong>${sum}</strong> there.</p>`;
    }

    // The score sheet: one row per player, columns 2..12.
    let head = `<tr><th>Player</th>`;
    for (let c = 0; c < cols; c++) head += `<th class="catego-col">${s.firstCol + c}</th>`;
    head += `<th>Points</th></tr>`;

    const rows = s.sheet.map((row, seat) => {
      const removed = ui.view.removed[seat];
      const cls = [];
      if (seat === ui.mySeat) cls.push("me");
      if (ui.isWaiting(seat)) cls.push("current");
      if (removed) cls.push("removed");
      let tr = `<tr class="${cls.join(" ")}"><td class="who">${ui.name(seat)}</td>`;
      row.forEach((v, c) => {
        const leads = s.leaders[c] === seat;
        const col = s.firstCol + c;
        if (v === 0 && placing && seat === ui.mySeat) {
          tr += `<td class="catego-cell open">${ui.button(String(col), { type: "place", index: col },
            { cls: "catego-place", title: `Write ${sum} in column ${col}` })}</td>`;
        } else {
          tr += `<td class="catego-cell${leads ? " leads" : ""}${v === 0 ? " empty" : ""}"` +
            `${leads ? ` title="${s.over ? "Won" : "Leading"}"` : ""}>${v || ""}</td>`;
        }
      });
      tr += `<td class="catego-points">${s.points[seat]}</td></tr>`;
      return tr;
    }).join("");

    const legend = `<p class="note">Highlighted numbers ${s.over ? "won" : "currently lead"} their column.
      A column is worth its number; a tie at the top means nobody wins it.</p>`;

    return `<div class="board-section">${top}</div>
      <div class="catego-wrap"><table class="scoreboard catego-sheet">${head}${rows}</table></div>
      ${legend}`;
  }

  GameUI.register("catego", { board });
})();
