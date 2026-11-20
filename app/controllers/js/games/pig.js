// pig.js — Pig and Big Pig.  The reference renderer: see games.js for the
// renderer API.
//
// state (from app/games/pig.go): {numDice, dice, current, turnTotal, rolled,
//                                 totals, goal}

(function () {
  function board(s, ui) {
    const rows = s.totals.map((total, seat) => {
      const atRisk = seat === s.current ? s.turnTotal : 0;
      const banked = Math.min(100, (total / s.goal) * 100);
      const risk = Math.min(100 - banked, (atRisk / s.goal) * 100);
      return {
        seat,
        cells: [
          `<span class="big-number">${total}</span>`,
          atRisk ? `<span class="pill gold">+${atRisk}</span>` : "",
          `<div class="progress"><span class="banked" style="width:${banked}%"></span>` +
            `<span class="at-risk" style="width:${risk}%"></span></div>`,
        ],
      };
    });

    const faces = s.dice && s.dice.length ? s.dice : new Array(s.numDice).fill(0);
    const diceHTML = s.dice && s.dice.length
      ? ui.dice(faces, { big: true, fresh: ui.fresh })
      : faces.map(() => ui.die(0, { big: true, empty: true })).join("");

    const myTurn = ui.waitingOnMe;
    const actions = myTurn
      ? `<div class="board-actions">
           ${ui.button("Roll 🎲", { type: "roll" }, { primary: true, roll: true })}
           ${ui.button(s.rolled ? `Hold +${s.turnTotal}` : "Hold", { type: "hold" }, { cls: "warn", disabled: !s.rolled })}
         </div>`
      : "";

    return `
      <div class="board-section">
        <div class="board-row">${diceHTML}
          <span class="note">${s.turnTotal ? `Turn total <strong>${s.turnTotal}</strong>` : ""}</span>
        </div>
        ${actions}
      </div>
      ${ui.scoreTable([`Banked (to ${s.goal})`, "At risk", ""], rows)}`;
  }

  GameUI.register("pig", { board });
  GameUI.register("bigpig", { board });
})();
