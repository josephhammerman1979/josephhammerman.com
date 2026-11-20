package pushluck

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Midnight (1-4-24): keep at least one die per roll until all six are kept.
// The hand qualifies only with a 1 and a 4; it scores the other four dice.
//
// Actions: {type:"roll"} starts a turn; {type:"keep", dice:[indices]} keeps
// those dice and rolls the rest.

func init() {
	games.Register(&games.Def{
		ID: "midnight", Name: "1-4-24", Order: 40,
		Summary:    "Ante a chip, keep a 1 and a 4 to qualify, and make the other four dice as high as you can to take the pot.",
		MinPlayers: 3, MaxPlayers: 8,
		Options: []games.Option{{Key: "rounds", Label: "Rounds", Choices: []games.Choice{
			{Value: "5", Label: "5 rounds"}, {Value: "3", Label: "3 rounds"}, {Value: "7", Label: "7 rounds"},
		}}},
		Rules: []string{
			"Each round everyone antes 1 chip into the pot.",
			"On your turn roll all six dice. After every roll keep at least one die (tap it), then roll the rest, until all six are kept.",
			"Your hand only counts if it includes a 1 and a 4.",
			"A qualifying hand scores the other four dice added up — 24 (four 6s) is the best.",
			"Highest qualifying hand takes the pot (ties roll off). If nobody qualifies, the pot carries over to the next round.",
			"Most chips after the last round wins the game.",
		},
		Facts: []string{"Number of dice: 6", "Player count: 3-8", "Object: Win each round, typically played for  money stakes"},
		Sheet: []string{
			"Starting with a random player and proceeding to their left, each player takes their complete turn. Begin by throwing all your dice. You must keep one die every round. A valid scoring hand must contain a one and a four.",
			"The value of a hand is declared without reference to the five points from the one and the four.",
			"The highest valid scoring hand every round wins the pot. Ties are decided through a roll off.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newMidnight(env, n, atoiDefault(opts["rounds"], 5, 3, 5, 7))
		},
	})
}

// midnightScore is the hand's score, or -1 if it lacks a 1 and a 4.
func midnightScore(d []int) int {
	c := games.Counts(d)
	if c[1] == 0 || c[4] == 0 {
		return -1
	}
	return games.Sum(d) - 5
}

// roundResult is a finished round, for the board.
type roundResult struct {
	Round  int     `json:"round"`
	Hands  [][]int `json:"hands"`  // per seat; null if they didn't play
	Scores []int   `json:"scores"` // per seat
	Bust   []bool  `json:"bust,omitempty"`
	Winner int     `json:"winner"`        // -1 if nobody won
	Pot    int     `json:"pot,omitempty"` // what the winner took
}

type midnight struct {
	env    *games.Env
	seats  games.Seats
	rounds int
	round  int
	first  int
	turnOrder
	turn   keepTurn
	hands  [][]int // kept dice this round, per seat; nil = not played yet
	scores []int   // this round, per seat
	wins   []int
	chips  []int // net chips, per seat
	pot    int
	carry  int // chips in this round's pot carried over from earlier rounds
	last   *roundResult
	winner int
}

func newMidnight(env *games.Env, n, rounds int) *midnight {
	g := &midnight{env: env, seats: games.NewSeats(n), rounds: rounds, wins: make([]int, n), chips: make([]int, n), winner: -1}
	g.startRound()
	return g
}

func (g *midnight) startRound() {
	g.round++
	if g.round == 1 {
		act := g.seats.Active()
		g.first = act[g.env.Rand.Intn(len(act))]
	} else {
		g.first = g.seats.Next(g.first)
	}
	g.hands = make([][]int, g.seats.N)
	g.scores = intsOf(g.seats.N, -1)
	g.turn = newKeepTurn(6)
	g.start(g.seats.OrderFrom(g.first), &g.seats)
	carried := g.pot
	g.carry = carried
	for _, s := range g.Order {
		g.chips[s]--
		g.pot++
	}
	ante := fmt.Sprintf("everyone antes 1 (pot %d)", g.pot)
	if carried > 0 {
		ante = fmt.Sprintf("everyone antes 1 — pot %d, including %d carried over", g.pot, carried)
	}
	g.env.Log(games.LogInfo, "Round %d of %d — %s. %s rolls first", g.round, g.rounds, ante, games.P(g.first))
}

func (g *midnight) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	if seat != g.cur() {
		return games.ErrNotYourTurn
	}
	who := games.P(seat)
	switch a.Type {
	case "roll":
		if g.turn.started() {
			return games.Errorf("Keep at least one die first.")
		}
		g.turn.roll(g.env)
		g.env.Log(games.LogRoll, "%s rolled %s", who, games.DiceText(g.turn.Roll))
		return nil
	case "keep":
		faces, err := g.turn.keep(a.Dice)
		if err != nil {
			return err
		}
		if g.turn.done() {
			g.finishTurn(faces)
			return nil
		}
		g.env.Log(games.LogInfo, "%s kept %s (%s)", who, games.DiceText(faces), g.keptStatus())
		g.turn.roll(g.env)
		g.env.Log(games.LogRoll, "%s rolled %s", who, games.DiceText(g.turn.Roll))
		return nil
	}
	return games.ErrUnknown
}

func (g *midnight) keptStatus() string {
	c := games.Counts(g.turn.Kept)
	switch {
	case c[1] > 0 && c[4] > 0:
		return "has the 1 and 4"
	case c[1] > 0:
		return "still needs a 4"
	case c[4] > 0:
		return "still needs a 1"
	}
	return "needs a 1 and a 4"
}

func (g *midnight) finishTurn(faces []int) {
	seat := g.cur()
	hand := g.turn.Kept
	g.hands[seat] = hand
	g.scores[seat] = midnightScore(hand)
	if g.scores[seat] < 0 {
		g.env.Log(games.LogBust, "%s kept %s — final hand %s has no 1 and 4: doesn't qualify", games.P(seat), games.DiceText(faces), games.DiceText(hand))
	} else {
		g.env.Log(games.LogGood, "%s kept %s — final hand %s scores %d", games.P(seat), games.DiceText(faces), games.DiceText(hand), g.scores[seat])
	}
	g.advance()
}

// advance moves to the next player, ending the round when all have played.
func (g *midnight) advance() {
	g.turn = newKeepTurn(6)
	g.next(&g.seats)
	if g.done() {
		g.endRound()
	}
}

func (g *midnight) endRound() {
	players := activeIn(g.Order, &g.seats)
	res := &roundResult{Round: g.round, Hands: copyHands(g.hands), Scores: games.Ints(g.scores), Winner: -1}
	best, top := []int{}, -1
	for _, s := range players {
		switch sc := g.scores[s]; {
		case sc < 0:
		case sc > top:
			best, top = []int{s}, sc
		case sc == top:
			best = append(best, s)
		}
	}
	if len(best) == 0 {
		g.env.Log(games.LogBust, "Round %d: nobody qualified — the pot of %d carries over", g.round, g.pot)
	} else {
		w := pickWinner(g.env, best)
		g.wins[w]++
		res.Winner = w
		res.Pot = g.pot
		g.chips[w] += g.pot
		g.env.Log(games.LogGood, "%s wins round %d with %d and takes the pot of %d (chips %s)", games.P(w), g.round, top, g.pot, signed(g.chips[w]))
		g.pot = 0
	}
	g.last = res
	if g.round >= g.rounds {
		if g.pot > 0 {
			g.env.Log(games.LogInfo, "The last round had no winner — the pot of %d goes unclaimed", g.pot)
		}
		w := pickWinner(g.env, games.BestSeats(&g.seats, g.chips, false))
		g.winner = w
		g.env.Log(games.LogWin, "%s wins the game with %s chips!", games.P(w), signed(g.chips[w]))
		return
	}
	g.startRound()
}

func (g *midnight) Legal(seat int) []games.Action {
	if g.winner >= 0 || seat != g.cur() {
		return nil
	}
	return g.turn.legal()
}

func (g *midnight) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.winner >= 0 {
		return
	}
	if seat == g.cur() {
		g.advance()
	}
}

type midnightView struct {
	Round   int          `json:"round"`
	Rounds  int          `json:"rounds"`
	First   int          `json:"first"`
	Current int          `json:"current"`
	Kept    []int        `json:"kept"`
	Roll    []int        `json:"roll"`
	Rolls   int          `json:"rolls"`
	Hands   [][]int      `json:"hands"`
	Scores  []int        `json:"scores"`
	Wins    []int        `json:"wins"`
	Chips   []int        `json:"chips"`
	Pot     int          `json:"pot"`
	Carried int          `json:"carried"`
	Last    *roundResult `json:"last"`
	Over    bool         `json:"over"`
}

func (g *midnight) View(seat int) interface{} {
	return midnightView{
		Round: g.round, Rounds: g.rounds, First: g.first, Current: g.cur(),
		Kept: games.Ints(g.turn.Kept), Roll: games.Ints(g.turn.Roll), Rolls: g.turn.Rolls,
		Hands: copyHands(g.hands), Scores: games.Ints(g.scores), Wins: games.Ints(g.wins), Chips: games.Ints(g.chips), Pot: g.pot, Carried: g.carry,
		Last: g.last, Over: g.winner >= 0,
	}
}

func (g *midnight) Prompt(seat int) string {
	if g.winner >= 0 {
		return fmt.Sprintf("%s wins with %s chips!", games.P(g.winner), signed(g.chips[g.winner]))
	}
	cur := g.cur()
	if seat != cur {
		if !g.turn.started() {
			return fmt.Sprintf("Waiting for %s to roll (round %d of %d, pot %d).", games.P(cur), g.round, g.rounds, g.pot)
		}
		return fmt.Sprintf("%s is choosing dice to keep — %s.", games.P(cur), g.keptStatus())
	}
	if !g.turn.started() {
		return "Roll all six dice to start your turn."
	}
	return fmt.Sprintf("Tap at least one die to keep (%s).", g.keptStatus())
}

func (g *midnight) Waiting() []int {
	if g.winner >= 0 || g.cur() < 0 {
		return nil
	}
	return []int{g.cur()}
}

func (g *midnight) Badges() []string {
	out := make([]string, g.seats.N)
	for i := range out {
		out[i] = signed(g.chips[i]) + " chips"
		if g.hands[i] != nil && g.winner < 0 {
			if g.scores[i] < 0 {
				out[i] += " · no 1-4"
			} else {
				out[i] += fmt.Sprintf(" · %d", g.scores[i])
			}
		}
	}
	return out
}

func (g *midnight) Over() bool { return g.winner >= 0 }

func (g *midnight) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
