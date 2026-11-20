package boards

import (
	"fmt"
	"strings"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Hundreds: build a three-digit number one die at a time.
//
// Each round is three passes round the table; on each pass every player
// rolls one die and places it in an empty square of their own board
// (hundreds, tens or ones).  Before rolling, a player may declare "low"
// (once per round) and the lowest number wins the round instead.
//
//   Classic: everyone starts with 3 tokens; the best number discards one;
//            first to discard all their tokens wins.
//   Postman: tokens start in the centre (3 per player).  While any remain,
//            the worst number takes one.  Once the centre is empty, play as
//            classic; anyone on zero tokens at that point wins.
//
// Actions: {type:"roll"}, {type:"low"} (declare low and roll),
// {type:"place", index: 0 hundreds | 1 tens | 2 ones}.

func init() {
	games.Register(&games.Def{
		ID: "hundreds", Name: "House Numbers", Order: 120,
		Summary:    "Roll one die at a time and build the biggest three-digit number — unless someone calls low.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Your board has three squares: hundreds, tens and ones.",
			"On your turn, roll one die and put it in any empty square. After three passes round the table every board is full.",
			"The highest number wins the round — but before you roll you may declare LOW, and then the lowest number wins this round.",
			"Classic: everyone starts with 3 tokens; the round winner discards one. First to get rid of all their tokens wins.",
			"Mr. Postman: the tokens start in the middle and the worst number takes one each round. When the middle is empty, the best number discards one — anyone with no tokens left wins.",
			"Ties are settled with a roll-off.",
		},
		Options: []games.Option{
			{Key: "variant", Label: "Variant", Choices: []games.Choice{
				choice("classic", "Classic (3 tokens each)"), choice("postman", "Mr. Postman (tokens start in the middle)")}},
		},
		Facts: []string{
			"Number of dice: 3 per player", "Gameboard: 3 squares per player", "Number of tokens: 3 per player",
			"Player count: 2 to 6", "Object: be the first player to discard all tokens",
		},
		Sheet: []string{
			"Put a gameboard and 3 chips in front of each player. Choose a random player to go first. Each player rolls one dice. They place the dice in hundreds, tens or ones category. The highest score is the winner, and they discard a token. Before a roll a player can declare “low” and the game is played for the lowest possible hand.",
			"Alternative: Postman",
			"Alternatively, the game can start with all tokens in the center. While there are token in the center, the objective is not to have the worst hand. If you do have the worst hand, take a token. Once all tokens are depleted, the game is played as above, with the best hand discarding a token. The first player to discard all tokens is the winner.",
		},
		Images: []string{"hundreds-board.png"},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newHundreds(env, n, opts["variant"] == "postman", env.Rand.Intn(n))
		},
	})
}

const hundredsTokens = 3

var squareNames = []string{"hundreds", "tens", "ones"}

type hundreds struct {
	env     *games.Env
	seats   games.Seats
	postman bool
	tokens  []int
	pool    int
	boards  [][3]int // 0 = empty square

	round   int
	starter int
	order   []int // this round's turn order
	turn    int   // index into order
	pass    int   // 0..2
	die     int   // rolled, waiting to be placed (0 = not rolled yet)
	low     bool
	lowBy   int

	last    *hundredsResult
	winners []int
	over    bool
}

// hundredsResult is the outcome of the last finished round.
type hundredsResult struct {
	Round  int      `json:"round"`
	Boards [][3]int `json:"boards"`
	Values []int    `json:"values"` // -1 for players not in the round
	Low    bool     `json:"low"`
	Seat   int      `json:"seat"` // who discarded or took a token
	Kind   string   `json:"kind"` // "discard" or "take"
	Tied   []int    `json:"tied"` // seats in a roll-off, if any
}

func newHundreds(env *games.Env, n int, postman bool, first int) *hundreds {
	g := &hundreds{env: env, seats: games.NewSeats(n), postman: postman, tokens: make([]int, n), boards: make([][3]int, n), starter: first, lowBy: -1}
	if postman {
		g.pool = hundredsTokens * n
	} else {
		for i := range g.tokens {
			g.tokens[i] = hundredsTokens
		}
	}
	g.startRound()
	return g
}

func (g *hundreds) cur() int {
	if g.over || g.turn >= len(g.order) {
		return -1
	}
	return g.order[g.turn]
}

func (g *hundreds) startRound() {
	g.round++
	if !g.seats.In(g.starter) {
		g.starter = g.seats.Next(g.starter)
	}
	g.boards = make([][3]int, g.seats.N)
	g.order = g.seats.OrderFrom(g.starter)
	g.turn, g.pass, g.die = 0, 0, 0
	g.low, g.lowBy = false, -1
	g.env.Log(games.LogInfo, "Round %d — %s goes first.", g.round, games.P(g.starter))
}

func value(b [3]int) int { return b[0]*100 + b[1]*10 + b[2] }

func (g *hundreds) Act(seat int, a games.Action) error {
	if g.over {
		return games.ErrGameOver
	}
	if seat != g.cur() {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "roll", "low":
		if g.die != 0 {
			return games.Errorf("Place your %d first.", g.die)
		}
		if a.Type == "low" {
			if g.low {
				return games.Errorf("This round is already low.")
			}
			g.low, g.lowBy = true, seat
			g.env.Log(games.LogInfo, "%s declared LOW — the lowest number wins this round!", games.P(seat))
		}
		g.die = g.env.Die()
		g.env.Log(games.LogRoll, "%s rolled a %d.", games.P(seat), g.die)
	case "place":
		if g.die == 0 {
			return games.Errorf("Roll first.")
		}
		if a.Index < 0 || a.Index > 2 {
			return games.Errorf("No such square.")
		}
		if g.boards[seat][a.Index] != 0 {
			return games.Errorf("That square is already filled.")
		}
		g.boards[seat][a.Index] = g.die
		g.env.Log(games.LogInfo, "%s put the %d in the %s.", games.P(seat), g.die, squareNames[a.Index])
		g.die = 0
		g.nextTurn()
	default:
		return games.ErrUnknown
	}
	return nil
}

func (g *hundreds) nextTurn() {
	for {
		g.turn++
		if g.turn >= len(g.order) {
			g.turn = 0
			g.pass++
			if g.pass >= 3 {
				g.score()
				return
			}
		}
		if g.seats.In(g.order[g.turn]) {
			return
		}
	}
}

func (g *hundreds) score() {
	n := g.seats.N
	vals := make([]int, n)
	parts := []string{}
	res := &hundredsResult{Round: g.round, Boards: append([][3]int{}, g.boards...), Values: make([]int, n), Low: g.low}
	for s := 0; s < n; s++ {
		res.Values[s] = -1
		if g.seats.In(s) {
			vals[s] = value(g.boards[s])
			res.Values[s] = vals[s]
			parts = append(parts, fmt.Sprintf("%s %d", games.P(s), vals[s]))
		}
	}
	mode := "high"
	if g.low {
		mode = "LOW"
	}
	g.env.Log(games.LogInfo, "Round %d (%s): %s.", g.round, mode, strings.Join(parts, ", "))

	pick := func(cands []int) int {
		if len(cands) == 1 {
			return cands[0]
		}
		res.Tied = games.Ints(cands)
		return games.RollOff(g.env, cands)
	}
	if g.postman && g.pool > 0 {
		worst := pick(games.BestSeats(&g.seats, vals, !g.low))
		g.tokens[worst]++
		g.pool--
		res.Seat, res.Kind = worst, "take"
		g.env.Log(games.LogBust, "%s has the worst number (%d) and takes a token — %d left in the middle.", games.P(worst), vals[worst], g.pool)
		if g.pool == 0 {
			g.env.Log(games.LogInfo, "The middle is empty — from now on the best number discards a token.")
		}
	} else {
		best := pick(games.BestSeats(&g.seats, vals, g.low))
		g.tokens[best]--
		res.Seat, res.Kind = best, "discard"
		g.env.Log(games.LogGood, "%s has the best number (%d) and discards a token — %s left.", games.P(best), vals[best], plural(g.tokens[best], "token", "tokens"))
	}
	g.last = res

	if !g.postman || g.pool == 0 {
		zero := []int{}
		for _, s := range g.seats.Active() {
			if g.tokens[s] == 0 {
				zero = append(zero, s)
			}
		}
		if len(zero) > 0 {
			g.over = true
			g.winners = zero
			if len(zero) == 1 {
				g.env.Log(games.LogWin, "%s has no tokens left and wins!", games.P(zero[0]))
			} else {
				g.env.Log(games.LogWin, "%s have no tokens and share the win!", seatList(zero))
			}
			return
		}
	}
	g.starter = g.seats.Next(g.starter)
	g.startRound()
}

func (g *hundreds) Legal(seat int) []games.Action {
	if g.over || seat != g.cur() {
		return nil
	}
	if g.die == 0 {
		out := []games.Action{{Type: "roll"}}
		if !g.low {
			out = append(out, games.Action{Type: "low"})
		}
		return out
	}
	out := []games.Action{}
	for i := 0; i < 3; i++ {
		if g.boards[seat][i] == 0 {
			out = append(out, games.Action{Type: "place", Index: i})
		}
	}
	return out
}

func (g *hundreds) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.over {
		return
	}
	if g.seats.ActiveCount() < 2 {
		g.over = true
		g.winners = g.seats.Active()
		return
	}
	if seat == g.cur() {
		g.die = 0
		g.nextTurn()
	}
}

type hundredsView struct {
	Variant string          `json:"variant"`
	Boards  [][3]int        `json:"boards"`
	Tokens  []int           `json:"tokens"`
	Pool    int             `json:"pool"`
	Out     []bool          `json:"out"`
	Round   int             `json:"round"`
	Pass    int             `json:"pass"` // 1..3
	Current int             `json:"current"`
	Starter int             `json:"starter"`
	Order   []int           `json:"order"`
	Die     int             `json:"die"`
	Low     bool            `json:"low"`
	LowBy   int             `json:"lowBy"`
	Last    *hundredsResult `json:"last"`
}

func (g *hundreds) View(seat int) interface{} {
	v := hundredsView{
		Variant: "classic", Boards: append([][3]int{}, g.boards...), Tokens: games.Ints(g.tokens), Pool: g.pool,
		Out: append([]bool{}, g.seats.Out...), Round: g.round, Pass: g.pass + 1, Current: g.cur(),
		Starter: g.starter, Order: games.Ints(g.order), Die: g.die, Low: g.low, LowBy: g.lowBy, Last: g.last,
	}
	if g.postman {
		v.Variant = "postman"
	}
	if v.Pass > 3 {
		v.Pass = 3
	}
	return v
}

func (g *hundreds) Prompt(seat int) string {
	if g.over {
		if len(g.winners) == 1 {
			return fmt.Sprintf("%s wins!", games.P(g.winners[0]))
		}
		return fmt.Sprintf("%s share the win!", seatList(g.winners))
	}
	c := g.cur()
	if seat != c {
		if g.die != 0 {
			return fmt.Sprintf("Waiting for %s to place their %d.", games.P(c), g.die)
		}
		return fmt.Sprintf("Waiting for %s to roll.", games.P(c))
	}
	if g.die != 0 {
		return fmt.Sprintf("You rolled a %d — tap the square to put it in.", g.die)
	}
	if g.low {
		return "Roll your die. This round is LOW — lowest number wins."
	}
	return "Roll your die — or declare LOW first so the lowest number wins."
}

func (g *hundreds) Waiting() []int {
	if g.over {
		return nil
	}
	return []int{g.cur()}
}

func (g *hundreds) Badges() []string {
	out := make([]string, g.seats.N)
	for s := range out {
		if g.seats.Out[s] {
			out[s] = "out"
			continue
		}
		out[s] = plural(g.tokens[s], "token", "tokens")
	}
	return out
}

func (g *hundreds) Over() bool { return g.over }

func (g *hundreds) Winners() []int {
	if !g.over {
		return nil
	}
	return games.Ints(g.winners)
}
