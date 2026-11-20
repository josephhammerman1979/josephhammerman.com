package pushluck

import (
	"fmt"
	"strings"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Sevens Out: roll six dice; pairs that add to seven are removed; score the
// dice that are left.  The round's first player may roll up to three times
// and sets the roll limit for everyone else that round.
//
// Actions: {type:"roll"} rolls the dice still in play; {type:"stop"} scores
// them (after at least one roll).

const (
	sevensGoal     = 50
	sevensMaxRolls = 3
)

func init() {
	games.Register(&games.Def{
		ID: "sevensout", Name: "Sevens", Order: 50,
		Summary:    "Roll six dice, lose every pair that adds to seven, and score what's left. Reroll if you dare.",
		MinPlayers: 3, MaxPlayers: 8,
		Rules: []string{
			"Roll six dice. Every pair adding to seven (1+6, 2+5, 3+4) is removed for the rest of your turn.",
			"Your score is the total of the dice still in play. Stop and score it, or roll the remaining dice again.",
			"The round's first player may roll up to 3 times; however many rolls they take is the limit for everyone else that round.",
			"Lose every die and you score 0.",
			"Scores add up. The game ends after the round in which someone reaches 50 — highest total wins.",
		},
		Facts: []string{"Number of dice: 6", "Player count: 3-∞", "Object: First to 50"},
		Sheet: []string{
			"The game is played in an agreed-upon number of rounds. In each round, all players take a turn rolling the dice.",
			"One player is chosen to begin and then play proceeds clockwise around the table.",
			"Each player in turn rolls six dice and removes any combination of pairs of numbers that add up to seven. The aim is to score the highest possible total by adding together the numbers shown on the remaining dice. ",
			"The first player has the option of rolling up to three times on their turn. After each roll, they remove sets of dice that add up to seven (one or more sets may have to be removed), and all these dice are set aside and are not used again during the player's turn. Then the numbers on the remaining dice are used to calculate their score. The first player can either accept the score or they can choose to roll again. If they choose to roll again, and subsequently roll any combination of dice that total seven, those dice must be removed as well. The player may then keep their score after two rolls, or risk rolling one more time to increase their score.",
			"All the following players take their turns in the same manner except that they are then limited to the same number of rolls, or fewer, that the first player took.",
			"The round ends once all players have had a turn. The player to the left of the previous first player begins each new round.",
		},
		New: func(env *games.Env, n int, _ map[string]string) games.Game { return newSevensOut(env, n) },
	})
}

// sevensRemove marks the dice removed as pairs adding to seven.
func sevensRemove(roll []int) []bool {
	c := games.Counts(roll)
	var drop [7]int
	for a := 1; a <= 3; a++ {
		m := c[a]
		if c[7-a] < m {
			m = c[7-a]
		}
		drop[a], drop[7-a] = m, m
	}
	gone := make([]bool, len(roll))
	for i, f := range roll {
		if drop[f] > 0 {
			drop[f]--
			gone[i] = true
		}
	}
	return gone
}

type sevensLast struct {
	Seat  int    `json:"seat"`
	Roll  []int  `json:"roll"`
	Gone  []bool `json:"gone"`
	Score int    `json:"score"`
	Rolls int    `json:"rolls"`
}

type sevensOut struct {
	env    *games.Env
	seats  games.Seats
	totals []int
	round  int
	first  int
	turnOrder
	limit  int    // rolls allowed this round; 0 until the first turn sets it
	roll   []int  // this turn's latest roll
	gone   []bool // which dice of roll were removed
	live   []int  // dice still in play
	rolls  int
	scores []int // this round, per seat; -1 = not yet
	last   *sevensLast
	winner int
}

func newSevensOut(env *games.Env, n int) *sevensOut {
	g := &sevensOut{env: env, seats: games.NewSeats(n), totals: make([]int, n), first: -1, winner: -1}
	g.startRound()
	return g
}

func (g *sevensOut) startRound() {
	g.round++
	if g.first < 0 {
		g.first = 0
	} else {
		g.first = g.seats.Next(g.first)
	}
	g.limit = 0
	g.scores = intsOf(g.seats.N, -1)
	g.start(g.seats.OrderFrom(g.first), &g.seats)
	g.resetTurn()
	g.env.Log(games.LogInfo, "Round %d — %s rolls first and sets the roll limit", g.round, games.P(g.first))
}

func (g *sevensOut) resetTurn() {
	g.roll, g.gone, g.live, g.rolls = nil, nil, nil, 0
}

// maxRolls is the current player's roll limit.
func (g *sevensOut) maxRolls() int {
	if g.limit == 0 {
		return sevensMaxRolls
	}
	return g.limit
}

func (g *sevensOut) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	if seat != g.cur() {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "roll":
		g.doRoll()
		return nil
	case "stop":
		if g.rolls == 0 {
			return games.Errorf("Roll at least once.")
		}
		g.endTurn()
		return nil
	}
	return games.ErrUnknown
}

func (g *sevensOut) doRoll() {
	n := 6
	if g.rolls > 0 {
		n = len(g.live)
	}
	g.roll = g.env.Roll(n)
	g.gone = sevensRemove(g.roll)
	g.rolls++
	g.live = nil
	pairs := []string{}
	var seen [7]int
	for i, f := range g.roll {
		if g.gone[i] {
			if f <= 3 {
				seen[f]++
			}
		} else {
			g.live = append(g.live, f)
		}
	}
	for a := 1; a <= 3; a++ {
		for k := 0; k < seen[a]; k++ {
			pairs = append(pairs, fmt.Sprintf("%d+%d", a, 7-a))
		}
	}
	who := games.P(g.cur())
	msg := fmt.Sprintf("%s rolled %s (roll %d of %d)", who, games.DiceText(g.roll), g.rolls, g.maxRolls())
	if len(pairs) > 0 {
		msg += " — removed " + strings.Join(pairs, ", ")
	}
	switch {
	case len(g.live) == 0:
		g.env.Log(games.LogBust, "%s: no dice left, scores 0", msg)
		g.endTurn()
	case g.rolls >= g.maxRolls():
		g.env.Log(games.LogRoll, "%s", msg)
		g.endTurn()
	default:
		g.env.Log(games.LogRoll, "%s: %d showing", msg, games.Sum(g.live))
	}
}

func (g *sevensOut) endTurn() {
	seat := g.cur()
	score := games.Sum(g.live)
	g.totals[seat] += score
	g.scores[seat] = score
	if score > 0 {
		g.env.Log(games.LogGood, "%s scores %d → %d", games.P(seat), score, g.totals[seat])
	}
	if g.limit == 0 {
		g.limit = g.rolls
		g.env.Log(games.LogInfo, "%s took %s — that's the limit for everyone this round", games.P(seat), plural(g.rolls, "roll", "rolls"))
	}
	g.last = &sevensLast{Seat: seat, Roll: games.Ints(g.roll), Gone: append([]bool{}, g.gone...), Score: score, Rolls: g.rolls}
	g.advance()
}

func (g *sevensOut) advance() {
	g.resetTurn()
	g.next(&g.seats)
	if !g.done() {
		return
	}
	reached := false
	for _, s := range g.seats.Active() {
		if g.totals[s] >= sevensGoal {
			reached = true
		}
	}
	if !reached {
		g.startRound()
		return
	}
	w := pickWinner(g.env, games.BestSeats(&g.seats, g.totals, false))
	g.winner = w
	g.env.Log(games.LogWin, "%s wins with %d!", games.P(w), g.totals[w])
}

func (g *sevensOut) Legal(seat int) []games.Action {
	if g.winner >= 0 || seat != g.cur() {
		return nil
	}
	if g.rolls == 0 {
		return []games.Action{{Type: "roll"}}
	}
	return []games.Action{{Type: "roll"}, {Type: "stop"}}
}

func (g *sevensOut) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.winner >= 0 {
		return
	}
	if seat == g.cur() {
		g.advance()
	}
}

type sevensView struct {
	Round    int         `json:"round"`
	First    int         `json:"first"`
	Current  int         `json:"current"`
	Limit    int         `json:"limit"`    // 0 = not set yet (first player's turn)
	MaxRolls int         `json:"maxRolls"` // the current player's limit
	Rolls    int         `json:"rolls"`
	Roll     []int       `json:"roll"`
	Gone     []bool      `json:"gone"`
	Live     []int       `json:"live"`
	Showing  int         `json:"showing"`
	Totals   []int       `json:"totals"`
	Scores   []int       `json:"scores"`
	Goal     int         `json:"goal"`
	Last     *sevensLast `json:"last"`
	Over     bool        `json:"over"`
}

func (g *sevensOut) View(seat int) interface{} {
	return sevensView{
		Round: g.round, First: g.first, Current: g.cur(), Limit: g.limit, MaxRolls: g.maxRolls(),
		Rolls: g.rolls, Roll: games.Ints(g.roll), Gone: append([]bool{}, g.gone...), Live: games.Ints(g.live),
		Showing: games.Sum(g.live), Totals: games.Ints(g.totals), Scores: games.Ints(g.scores),
		Goal: sevensGoal, Last: g.last, Over: g.winner >= 0,
	}
}

func (g *sevensOut) Prompt(seat int) string {
	if g.winner >= 0 {
		return fmt.Sprintf("%s wins with %d!", games.P(g.winner), g.totals[g.winner])
	}
	cur := g.cur()
	if seat != cur {
		if g.rolls > 0 {
			return fmt.Sprintf("%s has %d showing after roll %d of %d.", games.P(cur), games.Sum(g.live), g.rolls, g.maxRolls())
		}
		return fmt.Sprintf("Waiting for %s to roll (up to %s).", games.P(cur), plural(g.maxRolls(), "roll", "rolls"))
	}
	if g.rolls == 0 {
		if g.limit == 0 {
			return "You're first this round: roll up to 3 times. Your roll count becomes everyone's limit."
		}
		return fmt.Sprintf("Roll six dice — you get at most %s.", plural(g.limit, "roll", "rolls"))
	}
	return fmt.Sprintf("Score %d now, or roll %s again (%s left).", games.Sum(g.live), plural(len(g.live), "die", "dice"),
		plural(g.maxRolls()-g.rolls, "roll", "rolls"))
}

func (g *sevensOut) Waiting() []int {
	if g.winner >= 0 || g.cur() < 0 {
		return nil
	}
	return []int{g.cur()}
}

func (g *sevensOut) Badges() []string {
	out := make([]string, g.seats.N)
	for i := range out {
		out[i] = fmt.Sprint(g.totals[i])
		if i == g.cur() && g.rolls > 0 && g.winner < 0 {
			out[i] += fmt.Sprintf(" (+%d)", games.Sum(g.live))
		}
	}
	return out
}

func (g *sevensOut) Over() bool { return g.winner >= 0 }

func (g *sevensOut) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
