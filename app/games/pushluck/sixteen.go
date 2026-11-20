package pushluck

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Sixteen: keep at least one of three dice per roll until all are kept; get
// as close to 16 as possible without going over.
//
// Actions: {type:"roll"} starts a turn; {type:"keep", dice:[indices]} keeps
// those dice and rolls the rest.

const (
	sixteenDice = 3
	sixteenGoal = 16
)

func init() {
	games.Register(&games.Def{
		ID: "sixteen", Name: "Sixteen", Order: 60,
		Summary:    "Three dice, keep at least one per roll — get as close to 16 as you can without going over.",
		MinPlayers: 2, MaxPlayers: 8,
		Options: []games.Option{{Key: "rounds", Label: "Rounds", Choices: []games.Choice{
			{Value: "3", Label: "Best of 3"}, {Value: "5", Label: "Best of 5"},
		}}},
		Rules: []string{
			"On your turn roll three dice. After every roll keep at least one die (tap it), then roll the rest, until all three are kept.",
			"Your score is the total of the three dice. Over 16 and you're out of the round.",
			"Closest to 16 wins the round. Ties: with two players the one who rolled first wins; with more, the tied players roll off.",
			"The first player rotates each round. Win a majority of the rounds to win the match.",
		},
		Facts: []string{"Number of dice: 3", "Player count: 2-∞", "Object: Win Majority of Rounds Played"},
		Sheet: []string{
			"Each player in turn rolls three dice. Their objective is to score as close to sixteen as possible. At least one dice must be kept each round. Scoring over sixteen results in automatic disqualification. With only two players, ties favor the first to role. At higher player counts, conduct roll offs. Typically played in three rounds.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newSixteen(env, n, atoiDefault(opts["rounds"], 3, 3, 5))
		},
	})
}

type sixteen struct {
	env    *games.Env
	seats  games.Seats
	rounds int
	round  int
	first  int
	turnOrder
	turn   keepTurn
	hands  [][]int // kept dice this round, per seat; nil = not played yet
	scores []int   // this round's totals, per seat
	bust   []bool  // this round: went (or was sure to go) over 16
	wins   []int
	last   *roundResult
	winner int
}

func newSixteen(env *games.Env, n, rounds int) *sixteen {
	g := &sixteen{env: env, seats: games.NewSeats(n), rounds: rounds, wins: make([]int, n), first: -1, winner: -1}
	g.startRound()
	return g
}

func (g *sixteen) startRound() {
	g.round++
	if g.first < 0 {
		g.first = 0
	} else {
		g.first = g.seats.Next(g.first)
	}
	g.hands = make([][]int, g.seats.N)
	g.scores = intsOf(g.seats.N, -1)
	g.bust = make([]bool, g.seats.N)
	g.turn = newKeepTurn(sixteenDice)
	g.start(g.seats.OrderFrom(g.first), &g.seats)
	g.env.Log(games.LogInfo, "Round %d of %d — %s rolls first", g.round, g.rounds, games.P(g.first))
}

func (g *sixteen) Act(seat int, a games.Action) error {
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
		total := games.Sum(g.turn.Kept)
		// Every die left will add at least 1: stop as soon as a bust is certain.
		if g.turn.done() || total+g.turn.left() > sixteenGoal {
			g.finishTurn(faces)
			return nil
		}
		g.env.Log(games.LogInfo, "%s kept %s (total %d, %d to go)", who, games.DiceText(faces), total, sixteenGoal-total)
		g.turn.roll(g.env)
		g.env.Log(games.LogRoll, "%s rolled %s", who, games.DiceText(g.turn.Roll))
		return nil
	}
	return games.ErrUnknown
}

func (g *sixteen) finishTurn(faces []int) {
	seat := g.cur()
	g.hands[seat] = g.turn.Kept
	total := games.Sum(g.turn.Kept)
	g.scores[seat] = total
	switch {
	case total > sixteenGoal:
		g.bust[seat] = true
		g.env.Log(games.LogBust, "%s kept %s — %d is over 16: OUT this round", games.P(seat), games.DiceText(faces), total)
	case !g.turn.done():
		g.bust[seat] = true
		g.env.Log(games.LogBust, "%s kept %s — %d with %s still to roll is sure to go over 16: OUT this round",
			games.P(seat), games.DiceText(faces), total, plural(g.turn.left(), "die", "dice"))
	default:
		g.env.Log(games.LogGood, "%s kept %s — final total %d", games.P(seat), games.DiceText(faces), total)
	}
	g.advance()
}

func (g *sixteen) advance() {
	g.turn = newKeepTurn(sixteenDice)
	g.next(&g.seats)
	if g.done() {
		g.endRound()
	}
}

func (g *sixteen) endRound() {
	players := activeIn(g.Order, &g.seats)
	res := &roundResult{Round: g.round, Hands: copyHands(g.hands), Scores: games.Ints(g.scores), Bust: append([]bool{}, g.bust...), Winner: -1}
	best, top := []int{}, -1
	for _, s := range players { // in turn order, so best[0] rolled first
		switch sc := g.scores[s]; {
		case sc < 0 || g.bust[s]:
		case sc > top:
			best, top = []int{s}, sc
		case sc == top:
			best = append(best, s)
		}
	}
	g.last = res
	if len(best) == 0 {
		g.env.Log(games.LogBust, "Round %d: everyone went over — no winner", g.round)
	} else {
		w := best[0]
		switch {
		case len(best) > 1 && len(players) == 2:
			g.env.Log(games.LogInfo, "Tied on %d — with two players the first to roll, %s, takes it", top, games.P(w))
		case len(best) > 1:
			w = pickWinner(g.env, best)
		}
		g.wins[w]++
		res.Winner = w
		g.env.Log(games.LogGood, "%s wins round %d with %d (%s)", games.P(w), g.round, top, plural(g.wins[w], "win", "wins"))
		if g.wins[w]*2 > g.rounds {
			g.winner = w
			g.env.Log(games.LogWin, "%s wins the match, %d of %d rounds!", games.P(w), g.wins[w], g.rounds)
			return
		}
	}
	if g.round >= g.rounds {
		w := pickWinner(g.env, games.BestSeats(&g.seats, g.wins, false))
		g.winner = w
		g.env.Log(games.LogWin, "%s wins the match with %s!", games.P(w), plural(g.wins[w], "round win", "round wins"))
		return
	}
	g.startRound()
}

func (g *sixteen) Legal(seat int) []games.Action {
	if g.winner >= 0 || seat != g.cur() {
		return nil
	}
	return g.turn.legal()
}

func (g *sixteen) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.winner >= 0 {
		return
	}
	if seat == g.cur() {
		g.advance()
	}
}

type sixteenView struct {
	Round   int          `json:"round"`
	Rounds  int          `json:"rounds"`
	First   int          `json:"first"`
	Current int          `json:"current"`
	Kept    []int        `json:"kept"`
	Roll    []int        `json:"roll"`
	Rolls   int          `json:"rolls"`
	Hands   [][]int      `json:"hands"`
	Scores  []int        `json:"scores"`
	Bust    []bool       `json:"bust"`
	Wins    []int        `json:"wins"`
	Goal    int          `json:"goal"`
	Last    *roundResult `json:"last"`
	Over    bool         `json:"over"`
}

func (g *sixteen) View(seat int) interface{} {
	return sixteenView{
		Round: g.round, Rounds: g.rounds, First: g.first, Current: g.cur(),
		Kept: games.Ints(g.turn.Kept), Roll: games.Ints(g.turn.Roll), Rolls: g.turn.Rolls,
		Hands: copyHands(g.hands), Scores: games.Ints(g.scores), Bust: append([]bool{}, g.bust...), Wins: games.Ints(g.wins),
		Goal: sixteenGoal, Last: g.last, Over: g.winner >= 0,
	}
}

func (g *sixteen) Prompt(seat int) string {
	if g.winner >= 0 {
		return fmt.Sprintf("%s wins the match with %s!", games.P(g.winner), plural(g.wins[g.winner], "round win", "round wins"))
	}
	cur := g.cur()
	total := games.Sum(g.turn.Kept)
	if seat != cur {
		if !g.turn.started() {
			return fmt.Sprintf("Waiting for %s to roll (round %d of %d).", games.P(cur), g.round, g.rounds)
		}
		return fmt.Sprintf("%s has kept %d so far.", games.P(cur), total)
	}
	if !g.turn.started() {
		return "Roll three dice to start your turn."
	}
	if len(g.turn.Kept) == 0 {
		return "Tap at least one die to keep. Aim for 16 without going over."
	}
	return fmt.Sprintf("You have %d (%d to go). Tap at least one die to keep.", total, sixteenGoal-total)
}

func (g *sixteen) Waiting() []int {
	if g.winner >= 0 || g.cur() < 0 {
		return nil
	}
	return []int{g.cur()}
}

func (g *sixteen) Badges() []string {
	out := make([]string, g.seats.N)
	for i := range out {
		out[i] = plural(g.wins[i], "win", "wins")
		if g.hands[i] != nil && g.winner < 0 {
			if g.bust[i] {
				out[i] += " · over"
			} else {
				out[i] += fmt.Sprintf(" · %d", g.scores[i])
			}
		}
	}
	return out
}

func (g *sixteen) Over() bool { return g.winner >= 0 }

func (g *sixteen) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
