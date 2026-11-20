package pushluck

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Ten Thousand: six dice, set aside scoring dice, roll on or bank.
//
// Scoring (only these): a 1 = 100, a 5 = 50, three of a kind from one roll =
// face × 100 (three 1s = 1000).  Every die set aside must score.
//
// Actions: {type:"roll"} (first roll of a turn); {type:"keep_roll",
// dice:[indices]} sets those dice aside and rolls the rest (all six again if
// none are left: hot dice); {type:"keep_bank", dice:[indices]} sets them
// aside and banks the turn.

func init() {
	games.Register(&games.Def{
		ID: "tenthousand", Name: "5000 / 10000", Order: 30,
		Summary:    "Six dice: set aside 1s, 5s and triples, then push on or bank before you bust.",
		MinPlayers: 3, MaxPlayers: 8,
		Options: []games.Option{{Key: "target", Label: "Play to", Choices: []games.Choice{
			{Value: "5000", Label: "5,000"}, {Value: "10000", Label: "10,000"},
		}}},
		Rules: []string{
			"Roll six dice. Scoring: each 1 = 100, each 5 = 50, three of a kind = face × 100 (three 1s = 1000).",
			"After every roll set aside at least one scoring die (tap them). Every die you set aside must score, and triples must come from a single roll.",
			"Then roll the dice that are left, or bank your turn total.",
			"A roll with no scoring dice is a bust: your turn ends and you lose the turn's points.",
			"Hot dice: once all six are set aside, your next roll uses all six again.",
			"First to bank the target score wins.",
		},
		Facts: []string{"Number of dice: 6", "Player count: 3-8", "Object: First to 5000 or 10000"},
		Sheet: []string{
			"Starting with the first player and proceeding to their left, each player throws. A scoring die must be selected from every round. Dice, once scored, cannot be recombined to form different scoring combinations. A throw with no scoring dice causes a player’s turn to end. A player can stop throwing at any point and score their running total.",
			"Scoring",
			"Triples are scored as the number on the die (with three ones being worth 1000).",
			"Ones are worth 100",
			"Fives are worth 50",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newTenThousand(env, n, atoiDefault(opts["target"], 5000, 5000, 10000))
		},
	})
}

// ttScore scores dice set aside together from one roll.  ok is false if
// any die doesn't score (or there are none).
func ttScore(d []int) (pts int, ok bool) {
	c := games.Counts(d)
	for f := 1; f <= 6; f++ {
		n := c[f]
		if n >= 3 {
			trip := f * 100
			if f == 1 {
				trip = 1000
			}
			pts += (n / 3) * trip
			n %= 3
		}
		switch {
		case f == 1:
			pts += 100 * n
		case f == 5:
			pts += 50 * n
		case n > 0:
			return 0, false
		}
	}
	return pts, len(d) > 0
}

// ttScoring returns the indices of every die in roll that can score: all 1s
// and 5s, and whole triples of other faces.
func ttScoring(roll []int) []int {
	c := games.Counts(roll)
	take := [7]int{}
	for f := 1; f <= 6; f++ {
		take[f] = c[f] / 3 * 3
	}
	take[1], take[5] = c[1], c[5]
	out := []int{}
	for i, f := range roll {
		if take[f] > 0 {
			take[f]--
			out = append(out, i)
		}
	}
	return out
}

// lastTurn summarises the previous turn for the board.
type lastTurn struct {
	Seat int    `json:"seat"`
	Dice []int  `json:"dice"`
	Text string `json:"text"`
	Bad  bool   `json:"bad"`
}

type tenThousand struct {
	env      *games.Env
	seats    games.Seats
	target   int
	cur      int
	totals   []int
	turn     int     // points set aside this turn
	roll     []int   // the live roll; nil until the turn's first roll
	aside    [][]int // groups set aside this turn
	asidePts []int
	toRoll   int // dice the next roll uses
	rolls    int
	last     *lastTurn
	winner   int
}

func newTenThousand(env *games.Env, n, target int) *tenThousand {
	return &tenThousand{env: env, seats: games.NewSeats(n), target: target, totals: make([]int, n), toRoll: 6, winner: -1}
}

func (g *tenThousand) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	if seat != g.cur {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "roll":
		if g.roll != nil {
			return games.Errorf("Set aside at least one scoring die first.")
		}
		g.doRoll()
		return nil
	case "keep_roll", "keep_bank":
		if g.roll == nil {
			return games.Errorf("Roll first.")
		}
		sel, err := games.UniqueIndices(a.Dice, len(g.roll))
		if err != nil {
			return err
		}
		if len(sel) == 0 {
			return games.Errorf("Tap at least one scoring die to set aside.")
		}
		faces := make([]int, len(sel))
		for i, j := range sel {
			faces[i] = g.roll[j]
		}
		pts, ok := ttScore(faces)
		if !ok {
			return games.Errorf("Every die you set aside must score: 1s, 5s, or three of a kind.")
		}
		g.turn += pts
		g.aside = append(g.aside, faces)
		g.asidePts = append(g.asidePts, pts)
		g.toRoll = len(g.roll) - len(sel)
		g.roll = nil
		if a.Type == "keep_bank" {
			g.env.Log(games.LogInfo, "%s set aside %s for %d", games.P(g.cur), games.DiceText(faces), pts)
			g.bank()
			return nil
		}
		g.env.Log(games.LogInfo, "%s set aside %s for %d (turn %s)", games.P(g.cur), games.DiceText(faces), pts, comma(g.turn))
		if g.toRoll == 0 {
			g.toRoll = 6
			g.env.Log(games.LogGood, "Hot dice! %s rolls all six again with %s at risk", games.P(g.cur), comma(g.turn))
		}
		g.doRoll()
		return nil
	}
	return games.ErrUnknown
}

func (g *tenThousand) doRoll() {
	g.roll = g.env.Roll(g.toRoll)
	g.rolls++
	who := games.P(g.cur)
	if len(ttScoring(g.roll)) == 0 {
		lost := ""
		if g.turn > 0 {
			lost = fmt.Sprintf(", lost %s", comma(g.turn))
		}
		g.env.Log(games.LogBust, "%s rolled %s — no scoring dice, BUST%s", who, games.DiceText(g.roll), lost)
		g.last = &lastTurn{Seat: g.cur, Dice: games.Ints(g.roll), Text: "Bust" + lost, Bad: true}
		g.nextTurn()
		return
	}
	g.env.Log(games.LogRoll, "%s rolled %s", who, games.DiceText(g.roll))
}

func (g *tenThousand) bank() {
	g.totals[g.cur] += g.turn
	who := games.P(g.cur)
	g.last = &lastTurn{Seat: g.cur, Text: fmt.Sprintf("Banked %s", comma(g.turn))}
	for _, d := range g.aside {
		g.last.Dice = append(g.last.Dice, d...)
	}
	if g.totals[g.cur] >= g.target {
		g.winner = g.cur
		g.env.Log(games.LogWin, "%s banked %s → %s and wins!", who, comma(g.turn), comma(g.totals[g.cur]))
		return
	}
	g.env.Log(games.LogGood, "%s banked %s → %s", who, comma(g.turn), comma(g.totals[g.cur]))
	g.nextTurn()
}

func (g *tenThousand) nextTurn() {
	g.cur = g.seats.Next(g.cur)
	g.turn, g.roll, g.aside, g.asidePts, g.toRoll, g.rolls = 0, nil, nil, nil, 6, 0
}

func (g *tenThousand) Legal(seat int) []games.Action {
	if g.winner >= 0 || seat != g.cur {
		return nil
	}
	if g.roll == nil {
		return []games.Action{{Type: "roll"}}
	}
	sels := [][]int{ttScoring(g.roll)}
	c := games.Counts(g.roll)
	for f := 1; f <= 6; f++ {
		var idx []int
		for i, v := range g.roll {
			if v == f {
				idx = append(idx, i)
			}
		}
		if (f == 1 || f == 5) && len(idx) > 0 && len(idx) < len(sels[0]) {
			sels = append(sels, idx[:1])
		}
		if c[f] >= 3 && len(idx[:3]) < len(sels[0]) {
			sels = append(sels, idx[:3])
		}
	}
	out := []games.Action{}
	for _, s := range sels {
		out = append(out, games.Action{Type: "keep_roll", Dice: s}, games.Action{Type: "keep_bank", Dice: s})
	}
	return out
}

func (g *tenThousand) Remove(seat int) {
	g.seats.Out[seat] = true
	if seat == g.cur && g.winner < 0 {
		g.nextTurn()
	}
}

type ttView struct {
	Target    int       `json:"target"`
	Current   int       `json:"current"`
	Totals    []int     `json:"totals"`
	TurnTotal int       `json:"turnTotal"`
	Roll      []int     `json:"roll"`    // live dice; empty before the turn's first roll
	Scoring   []int     `json:"scoring"` // indices of Roll that can score
	Aside     [][]int   `json:"aside"`   // groups set aside this turn
	AsidePts  []int     `json:"asidePts"`
	ToRoll    int       `json:"toRoll"`
	Rolls     int       `json:"rolls"`
	Last      *lastTurn `json:"last"`
	Over      bool      `json:"over"`
}

func (g *tenThousand) View(seat int) interface{} {
	v := ttView{
		Target: g.target, Current: g.cur, Totals: games.Ints(g.totals), TurnTotal: g.turn,
		Roll: games.Ints(g.roll), Scoring: ttScoring(g.roll), Aside: copyHands(g.aside),
		AsidePts: games.Ints(g.asidePts), ToRoll: g.toRoll, Rolls: g.rolls, Last: g.last, Over: g.winner >= 0,
	}
	if v.Aside == nil {
		v.Aside = [][]int{}
	}
	return v
}

func (g *tenThousand) Prompt(seat int) string {
	if g.winner >= 0 {
		return fmt.Sprintf("%s wins with %s!", games.P(g.winner), comma(g.totals[g.winner]))
	}
	if seat != g.cur {
		if g.turn > 0 {
			return fmt.Sprintf("%s has %s at risk.", games.P(g.cur), comma(g.turn))
		}
		return fmt.Sprintf("Waiting for %s to roll.", games.P(g.cur))
	}
	if g.roll == nil {
		return "Roll six dice to start your turn."
	}
	if g.turn > 0 {
		return fmt.Sprintf("Tap scoring dice to set aside, then roll on or bank (%s at risk).", comma(g.turn))
	}
	return "Tap scoring dice to set aside, then roll on or bank."
}

func (g *tenThousand) Waiting() []int {
	if g.winner >= 0 {
		return nil
	}
	return []int{g.cur}
}

func (g *tenThousand) Badges() []string {
	out := make([]string, len(g.totals))
	for i, t := range g.totals {
		out[i] = comma(t)
		if i == g.cur && g.turn > 0 && g.winner < 0 {
			out[i] += " (+" + comma(g.turn) + ")"
		}
	}
	return out
}

func (g *tenThousand) Over() bool { return g.winner >= 0 }

func (g *tenThousand) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
