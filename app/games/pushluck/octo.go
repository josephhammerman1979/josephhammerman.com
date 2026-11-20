package pushluck

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Octo (Reiner Knizia): push your luck with eight dice.
//
// On your turn roll all eight dice and set aside one or more dice showing a
// single number.  Then stop, or roll the rest and set aside dice of a number
// you haven't set aside yet this turn.  If a roll has no new number to set
// aside, you bust and score nothing this round.  Stopping scores the total
// of the dice you set aside.  There are as many rounds as players; highest
// total wins.
//
// Actions: {type:"roll"} starts the turn; {type:"keep_roll", dice} and
// {type:"keep_stop", dice} set aside the selected dice (all one number) and
// roll on or stop.

const octoDice = 8

func init() {
	games.Register(&games.Def{
		ID: "octo", Name: "Octo", Order: 75,
		Summary:    "Eight dice. Set aside a new number each roll — repeat a number and you score nothing.",
		MinPlayers: 2, MaxPlayers: 8,
		Rules: []string{
			"Roll all eight dice and set aside one or more dice showing the same number.",
			"Then stop and score, or roll the rest and set aside dice of a number you haven't taken yet this turn.",
			"If a roll has no new number for you, you bust and score nothing this round.",
			"Stopping scores the total of the dice you set aside. Setting aside all eight ends your turn.",
			"There are as many rounds as players. Highest total wins.",
		},
		Facts: []string{"Number of dice: 8", "Player count: 2 or more", "Object: Highest score at the end of the game"},
		Sheet: []string{
			"Created by Reiner Knizia. Play a number of rounds equal to the number of players. Each player rolls a single die; the player with the highest roll goes first. Play continues clockwise around the table.",
			"To begin a player's turn, they roll all eight dice. They must set aside at least one of the dice: one number, and as many of that number as they like (from 1-2-2-3-4-5-5-5 they may set aside the 1, one or more 2s, the 3, the 4, or one or more 5s). You cannot set aside more than one number.",
			"They then decide whether to stop or continue rolling the remaining dice. They may keep rolling and setting aside dice as long as new numbers are available to set aside — once 5s have been set aside, no more 5s can be set aside.",
			"If a player rolls and is unable to set aside any dice, their turn is immediately over and they score zero points for the round. If a player stops their turn, they earn points equal to the total value of the dice they set aside.",
			"Play continues around the table until all of the rounds have been completed. At the end of the game, the player with the highest total score wins.",
		},
		New: func(env *games.Env, n int, _ map[string]string) games.Game { return newOcto(env, n) },
	})
}

type octoKeep struct {
	Face  int `json:"face"`
	Count int `json:"count"`
}

type octo struct {
	env    *games.Env
	seats  games.Seats
	rounds int
	round  int
	first  int   // first player every round (highest opening roll)
	order  []int // this round's turn order
	pos    int
	totals []int
	scores [][]int // per round, per seat (-1 = not yet played)

	roll []int // live dice; nil before the first roll of a turn
	kept []octoKeep
	over bool
}

func newOcto(env *games.Env, n int) *octo {
	g := &octo{env: env, seats: games.NewSeats(n), rounds: n, totals: make([]int, n)}
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	g.env.Log(games.LogInfo, "Everyone rolled one die to see who goes first.")
	g.first = games.RollOff(env, all)
	g.env.Log(games.LogInfo, "%s goes first.", games.P(g.first))
	g.startRound()
	return g
}

func (g *octo) startRound() {
	g.round++
	g.order = g.seats.OrderFrom(g.first)
	if !g.seats.In(g.first) {
		g.order = g.seats.OrderFrom(g.seats.Next(g.first))
	}
	g.pos = 0
	row := make([]int, g.seats.N)
	for i := range row {
		row[i] = -1
	}
	g.scores = append(g.scores, row)
	g.roll, g.kept = nil, nil
}

func (g *octo) cur() int { return g.order[g.pos] }

func (g *octo) used(face int) bool {
	for _, k := range g.kept {
		if k.Face == face {
			return true
		}
	}
	return false
}

func (g *octo) keptCount() int {
	n := 0
	for _, k := range g.kept {
		n += k.Count
	}
	return n
}

func (g *octo) turnTotal() int {
	t := 0
	for _, k := range g.kept {
		t += k.Face * k.Count
	}
	return t
}

// canKeep reports whether the live roll has any number not yet set aside.
func (g *octo) canKeep() bool {
	for _, v := range g.roll {
		if !g.used(v) {
			return true
		}
	}
	return false
}

func (g *octo) Act(seat int, a games.Action) error {
	if g.over {
		return games.ErrGameOver
	}
	if seat != g.cur() {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "roll":
		if g.roll != nil {
			return games.Errorf("Set aside some dice first.")
		}
		if len(g.kept) > 0 {
			return games.Errorf("Use \"set aside & roll\" to roll on.")
		}
		g.doRoll()
		return nil
	case "keep_roll", "keep_stop":
		if g.roll == nil {
			return games.Errorf("Roll first.")
		}
		sel, err := games.UniqueIndices(a.Dice, len(g.roll))
		if err != nil {
			return err
		}
		if len(sel) == 0 {
			return games.Errorf("Tap the dice you want to set aside.")
		}
		face := g.roll[sel[0]]
		for _, i := range sel {
			if g.roll[i] != face {
				return games.Errorf("You can only set aside one number at a time.")
			}
		}
		if g.used(face) {
			return games.Errorf("You've already set aside %ss this turn.", fmt.Sprint(face))
		}
		left := len(g.roll) - len(sel)
		if a.Type == "keep_roll" && left == 0 {
			return games.Errorf("There are no dice left to roll — stop instead.")
		}
		g.kept = append(g.kept, octoKeep{Face: face, Count: len(sel)})
		g.env.Log(games.LogRoll, "%s set aside %d×%d (turn %d)", games.P(seat), len(sel), face, g.turnTotal())
		if a.Type == "keep_stop" || left == 0 {
			g.endTurn(g.turnTotal())
			return nil
		}
		g.doRoll()
		return nil
	}
	return games.ErrUnknown
}

func (g *octo) doRoll() {
	g.roll = g.env.Roll(octoDice - g.keptCount())
	seat := g.cur()
	if !g.canKeep() {
		g.env.Log(games.LogBust, "%s rolled %s — no new number. BUST, scores 0 this round.", games.P(seat), games.DiceText(g.roll))
		g.endTurn(0)
		return
	}
	g.env.Log(games.LogRoll, "%s rolled %s", games.P(seat), games.DiceText(g.roll))
}

func (g *octo) endTurn(score int) {
	seat := g.cur()
	g.scores[g.round-1][seat] = score
	g.totals[seat] += score
	if score > 0 {
		g.env.Log(games.LogGood, "%s scored %d → %d", games.P(seat), score, g.totals[seat])
	}
	g.roll, g.kept = nil, nil
	g.nextTurn()
}

func (g *octo) nextTurn() {
	for {
		g.pos++
		if g.pos >= len(g.order) {
			if g.round >= g.rounds {
				g.finish()
				return
			}
			g.startRound()
			return
		}
		if g.seats.In(g.cur()) {
			return
		}
	}
}

func (g *octo) finish() {
	g.over = true
	w := g.Winners()
	if len(w) == 1 {
		g.env.Log(games.LogWin, "%s wins with %d!", games.P(w[0]), g.totals[w[0]])
	} else {
		g.env.Log(games.LogWin, "A tie at %d!", g.totals[w[0]])
	}
}

func (g *octo) Legal(seat int) []games.Action {
	if g.over || seat != g.cur() {
		return nil
	}
	if g.roll == nil {
		return []games.Action{{Type: "roll"}}
	}
	out := []games.Action{}
	for face := 1; face <= 6; face++ {
		if g.used(face) {
			continue
		}
		idx := []int{}
		for i, v := range g.roll {
			if v == face {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		out = append(out, games.Action{Type: "keep_stop", Dice: idx})
		if len(idx) < len(g.roll) {
			out = append(out, games.Action{Type: "keep_roll", Dice: idx})
			out = append(out, games.Action{Type: "keep_roll", Dice: idx[:1]})
		}
	}
	return out
}

func (g *octo) Remove(seat int) {
	wasCur := !g.over && seat == g.cur()
	g.seats.Out[seat] = true
	if wasCur && g.seats.ActiveCount() >= 2 {
		g.roll, g.kept = nil, nil
		g.nextTurn()
	}
}

type octoView struct {
	Round     int        `json:"round"`
	Rounds    int        `json:"rounds"`
	Current   int        `json:"current"`
	Roll      []int      `json:"roll"`
	Kept      []octoKeep `json:"kept"`
	TurnTotal int        `json:"turnTotal"`
	Totals    []int      `json:"totals"`
	Scores    [][]int    `json:"scores"`
}

func (g *octo) View(seat int) interface{} {
	scores := make([][]int, len(g.scores))
	for i, r := range g.scores {
		scores[i] = games.Ints(r)
	}
	kept := append([]octoKeep{}, g.kept...)
	cur := -1
	if !g.over {
		cur = g.cur()
	}
	return octoView{
		Round: g.round, Rounds: g.rounds, Current: cur, Roll: games.Ints(g.roll), Kept: kept,
		TurnTotal: g.turnTotal(), Totals: games.Ints(g.totals), Scores: scores,
	}
}

func (g *octo) Prompt(seat int) string {
	if g.over {
		return "All rounds are done."
	}
	if seat != g.cur() {
		return fmt.Sprintf("Round %d of %d — waiting for %s.", g.round, g.rounds, games.P(g.cur()))
	}
	if g.roll == nil {
		return fmt.Sprintf("Round %d of %d — roll all eight dice.", g.round, g.rounds)
	}
	return "Tap dice of one new number, then set them aside and roll on, or stop."
}

func (g *octo) Waiting() []int {
	if g.over {
		return nil
	}
	return []int{g.cur()}
}

func (g *octo) Badges() []string {
	out := make([]string, len(g.totals))
	for i, t := range g.totals {
		out[i] = fmt.Sprint(t)
		if !g.over && i == g.cur() && g.turnTotal() > 0 {
			out[i] += fmt.Sprintf(" (+%d)", g.turnTotal())
		}
	}
	return out
}

func (g *octo) Over() bool { return g.over }

func (g *octo) Winners() []int {
	if !g.over {
		return nil
	}
	return games.BestSeats(&g.seats, g.totals, false)
}
