// mice.js — Of Mice and Men.  See games.js for the renderer API.
//
// state (from app/games/bluff/mice.go): {round, starter, current, value,
//   leader, maxBid, runCost, limit, penalty[], in[], ran[], agreed[],
//   shown[][] (runners' dice), myDice, myNumber, last}

(function () {
  function number(d) {
    return d.slice().sort((a, b) => b - a).join("");
  }

  function valueOptions(from, to, selected) {
    let out = "";
    for (let v = from; v <= to; v++) {
      out += `<option value="${v}"${v === selected ? " selected" : ""}>${v}</option>`;
    }
    return out;
  }

  function lastRound(s, ui) {
    const l = s.last;
    if (!l) return "";
    let html = `<div class="board-section mice-last"><h3>Round ${l.round} result — penalty ${l.value}</h3>`;
    if (l.lastStand >= 0) {
      html += `<p class="note">Everyone else ran. ${ui.name(l.lastStand)} took nothing and kept their dice hidden.</p>`;
    }
    html += `<div class="mice-shown">`;
    l.shown.forEach((d, seat) => {
      if (!d || !d.length) return;
      const lost = l.losers.includes(seat);
      html += `<div class="mice-hand${lost ? " lost" : ""}"><span class="who">${ui.name(seat)}</span>
        ${ui.dice(d.slice().sort((a, b) => b - a), { small: true })}
        <span class="mice-num">${number(d)}</span>
        ${l.ran[seat] ? `<span class="pill bad">ran +3</span>` : ""}
        ${lost ? `<span class="pill bad">lowest +${l.value}</span>` : ""}</div>`;
    });
    html += `</div></div>`;
    return html;
  }

  function board(s, ui) {
    const myTurn = ui.waitingOnMe;
    let html = "";

    // The stake.
    html += `<div class="board-section"><div class="board-row">
      <span class="note">Penalty on the table</span>
      <span class="big-number mice-stake">${s.value || "—"}</span>
      ${s.leader >= 0 && s.value ? `<span class="note">named by ${ui.name(s.leader)}</span>` : ""}
      <span class="pill">Round ${s.round}</span>
      <span class="pill">Limit ${s.limit}</span>
    </div>`;

    // My hidden hand.
    if (s.myDice && s.myDice.length) {
      const inRound = ui.mySeat >= 0 && s.in[ui.mySeat];
      html += `<div class="board-row">
        ${ui.dice(s.myDice.slice().sort((a, b) => b - a), { big: true, fresh: ui.fresh })}
        <span class="big-number">${s.myNumber}</span>
        <span class="note">${inRound ? "Only you can see these." : "You're out of this round."}</span>
      </div>`;
    }

    // Controls.
    if (myTurn) {
      if (!s.value) {
        html += `<div class="board-actions"><form data-act-form='{"type":"open"}'>
          <label>Open at <select name="count" data-num>${valueOptions(1, s.maxBid, 2)}</select> penalty points</label>
          <button class="btn primary" type="submit">Open the round</button></form></div>`;
      } else {
        html += `<div class="board-actions">
          ${ui.button(`Join the men — accept ${s.value}`, { type: "join" }, { primary: true })}
          ${ui.button(`Run with the mice — take ${s.runCost}`, { type: "run" }, { cls: "danger" })}`;
        if (s.value < s.maxBid) {
          html += `<form data-act-form='{"type":"lead"}'>
            <select name="count" data-num>${valueOptions(s.value + 1, s.maxBid, s.value + 1)}</select>
            <button class="btn warn" type="submit">Lead the men — raise</button></form>`;
        }
        html += `</div>`;
      }
    }
    html += `</div>`;

    // Everyone's status this round.
    const rows = s.penalty.map((p, seat) => {
      let status;
      if (ui.view.removed[seat]) status = "";
      else if (s.ran[seat]) {
        status = `<span class="pill bad">ran</span> ${ui.dice(s.shown[seat].slice().sort((a, b) => b - a), { small: true })}`;
      } else if (!s.in[seat]) status = "";
      else if (!s.value) status = seat === s.current ? "opening…" : `<span class="note">in</span>`;
      else if (s.agreed[seat]) status = `<span class="pill good">in at ${s.value}</span>`;
      else status = seat === s.current ? "deciding…" : `<span class="note">to answer</span>`;
      const pct = Math.min(100, (p / s.limit) * 100);
      return {
        seat,
        cells: [
          status,
          `<span class="big-number">${p}</span>`,
          `<div class="progress"><span class="banked mice-bar" style="width:${pct}%"></span></div>`,
        ],
      };
    });
    html += ui.scoreTable(["This round", "Penalty", `toward ${s.limit}`], rows);
    html += lastRound(s, ui);
    return html;
  }

  GameUI.register("mice", { board });
})();
