package boards

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Three Hands: two players each build three dice-poker hands, one die at a
// time, and compare them hand by hand.  Win two of the three hands to win
// the round; win two rounds to win the match.
//
//   three: three dice per hand (three-dice poker: trips > straight > pair).
//   five:  five dice per hand (full five-dice poker hands).
//
// Actions: {type:"roll"}, {type:"place", index: hand 0..2}.

func init() {
	games.Register(&games.Def{
		ID: "threehands", Name: "East Versus West", Order: 130,
		Summary:    "Two players, one die at a time: build three poker hands and win two of them.",
		MinPlayers: 2, MaxPlayers: 2,
		Rules: []string{
			"You each have three hands on your side of the board.",
			"Take turns: roll one die and put it in an open square of any of your hands.",
			"When both sides are full, hand 1 plays hand 1, 2 plays 2 and 3 plays 3. Win more hands than your opponent to win the round (a tied round is replayed).",
			"Three dice: three of a kind > straight > pair > highest die. Five dice: the usual poker-dice ranking.",
			"First to win two rounds wins the match.",
		},
		Options: []games.Option{
			{Key: "variant", Label: "Hand size", Choices: []games.Choice{
				choice("three", "Three dice per hand"), choice("five", "Five dice per hand")}},
		},
		Facts: []string{
			"Number of dice: 3 per player", "Gameboard: 3 squares per player",
			"Player count: 2", "Object: win two out of three rounds",
		},
		Sheet: []string{
			"The objective is to build the best three card Dice Poker hand that you can in each play area. Four of a kind, five of a kind, and full houses are not possible. Starting with a randomly chosen starting player, roll a dice. Place the dice on any of the open squares on your half of the board. The winner of two of the three play areas is the winner of the round.",
			"Variant:",
			"Play with full five dice poker hands.",
		},
		Images: []string{"three-hands-board.png", "three-hands-five-dice-board.png"},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			k := 3
			if opts["variant"] == "five" {
				k = 5
			}
			return newThreeHands(env, k, env.Rand.Intn(2))
		},
	})
}

const (
	thAreas    = 3
	thWinsGoal = 2
)

type threeHands struct {
	env     *games.Env
	k       int // dice per hand
	areas   [2][thAreas][]int
	cur     int
	starter int
	die     int
	round   int
	wins    [2]int
	last    *thResult
	winner  int
	over    bool
}

type thResult struct {
	Round   int                `json:"round"`
	Areas   [2][thAreas][]int  `json:"areas"`
	Hands   [2][thAreas]string `json:"hands"`
	Results [thAreas]int       `json:"results"` // winning seat per hand, -1 tie
	Winner  int                `json:"winner"`  // -1 drawn
}

func newThreeHands(env *games.Env, k, first int) *threeHands {
	g := &threeHands{env: env, k: k, starter: first, winner: -1}
	g.startRound()
	return g
}

func (g *threeHands) startRound() {
	g.round++
	for p := 0; p < 2; p++ {
		for a := 0; a < thAreas; a++ {
			g.areas[p][a] = make([]int, g.k)
		}
	}
	g.cur = g.starter
	g.die = 0
	g.env.Log(games.LogInfo, "Round %d — %s goes first.", g.round, games.P(g.starter))
}

func (g *threeHands) open(seat, area int) int {
	for i, v := range g.areas[seat][area] {
		if v == 0 {
			return i
		}
	}
	return -1
}

func (g *threeHands) full(seat int) bool {
	for a := 0; a < thAreas; a++ {
		if g.open(seat, a) >= 0 {
			return false
		}
	}
	return true
}

func (g *threeHands) Act(seat int, a games.Action) error {
	if g.over {
		return games.ErrGameOver
	}
	if seat != g.cur {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "roll":
		if g.die != 0 {
			return games.Errorf("Place your %d first.", g.die)
		}
		g.die = g.env.Die()
		g.env.Log(games.LogRoll, "%s rolled a %d.", games.P(seat), g.die)
	case "place":
		if g.die == 0 {
			return games.Errorf("Roll first.")
		}
		if a.Index < 0 || a.Index >= thAreas {
			return games.Errorf("No such hand.")
		}
		slot := g.open(seat, a.Index)
		if slot < 0 {
			return games.Errorf("That hand is full.")
		}
		g.areas[seat][a.Index][slot] = g.die
		g.env.Log(games.LogInfo, "%s put the %d in hand %d.", games.P(seat), g.die, a.Index+1)
		g.die = 0
		if g.full(0) && g.full(1) {
			g.score()
		} else if !g.full(1 - seat) {
			g.cur = 1 - seat
		}
	default:
		return games.ErrUnknown
	}
	return nil
}

func (g *threeHands) score() {
	res := &thResult{Round: g.round, Winner: -1}
	var won [2]int
	for a := 0; a < thAreas; a++ {
		var h [2]games.Hand
		for p := 0; p < 2; p++ {
			res.Areas[p][a] = games.Ints(g.areas[p][a])
			h[p] = games.EvalHand(g.areas[p][a])
			res.Hands[p][a] = h[p].Name
		}
		c := games.CompareHands(h[0], h[1])
		switch {
		case c > 0:
			res.Results[a] = 0
			won[0]++
			g.env.Log(games.LogInfo, "Hand %d: %s's %s beats %s.", a+1, games.P(0), h[0].Name, h[1].Name)
		case c < 0:
			res.Results[a] = 1
			won[1]++
			g.env.Log(games.LogInfo, "Hand %d: %s's %s beats %s.", a+1, games.P(1), h[1].Name, h[0].Name)
		default:
			res.Results[a] = -1
			g.env.Log(games.LogInfo, "Hand %d: %s ties.", a+1, h[0].Name)
		}
	}
	g.last = res
	if won[0] == won[1] {
		g.env.Log(games.LogBust, "Round %d is drawn (%d–%d) and will be replayed.", g.round, won[0], won[1])
	} else {
		w := 0
		if won[1] > won[0] {
			w = 1
		}
		res.Winner = w
		g.wins[w]++
		g.env.Log(games.LogGood, "%s wins round %d, %d hands to %d.", games.P(w), g.round, won[w], won[1-w])
		if g.wins[w] >= thWinsGoal {
			g.over, g.winner = true, w
			g.env.Log(games.LogWin, "%s wins the match %d–%d!", games.P(w), g.wins[w], g.wins[1-w])
			return
		}
	}
	g.starter = 1 - g.starter
	g.startRound()
}

func (g *threeHands) Legal(seat int) []games.Action {
	if g.over || seat != g.cur {
		return nil
	}
	if g.die == 0 {
		return []games.Action{{Type: "roll"}}
	}
	out := []games.Action{}
	for a := 0; a < thAreas; a++ {
		if g.open(seat, a) >= 0 {
			out = append(out, games.Action{Type: "place", Index: a})
		}
	}
	return out
}

// Remove: Three Hands is strictly two-player, so the Table ends the game
// itself when someone leaves.  Award the match to the other player in case
// it's ever called.
func (g *threeHands) Remove(seat int) {
	if g.over || seat < 0 || seat > 1 {
		return
	}
	g.over, g.winner = true, 1-seat
}

type threeHandsView struct {
	K       int                `json:"k"`
	Areas   [2][thAreas][]int  `json:"areas"`
	Hands   [2][thAreas]string `json:"hands"` // name once a hand is full
	Current int                `json:"current"`
	Starter int                `json:"starter"`
	Die     int                `json:"die"`
	Round   int                `json:"round"`
	Wins    [2]int             `json:"wins"`
	Goal    int                `json:"goal"`
	Last    *thResult          `json:"last"`
}

func (g *threeHands) View(seat int) interface{} {
	v := threeHandsView{K: g.k, Current: g.cur, Starter: g.starter, Die: g.die, Round: g.round, Wins: g.wins, Goal: thWinsGoal, Last: g.last}
	if g.over {
		v.Current = -1
	}
	for p := 0; p < 2; p++ {
		for a := 0; a < thAreas; a++ {
			v.Areas[p][a] = games.Ints(g.areas[p][a])
			if g.open(p, a) < 0 {
				v.Hands[p][a] = games.EvalHand(g.areas[p][a]).Name
			}
		}
	}
	return v
}

func (g *threeHands) Prompt(seat int) string {
	if g.over {
		return fmt.Sprintf("%s wins the match %d–%d!", games.P(g.winner), g.wins[g.winner], g.wins[1-g.winner])
	}
	if seat != g.cur {
		if g.die != 0 {
			return fmt.Sprintf("Waiting for %s to place their %d.", games.P(g.cur), g.die)
		}
		return fmt.Sprintf("Waiting for %s to roll.", games.P(g.cur))
	}
	if g.die != 0 {
		return fmt.Sprintf("You rolled a %d — tap one of your hands to put it there.", g.die)
	}
	return "Roll a die."
}

func (g *threeHands) Waiting() []int {
	if g.over {
		return nil
	}
	return []int{g.cur}
}

func (g *threeHands) Badges() []string {
	out := make([]string, 2)
	for p := 0; p < 2; p++ {
		out[p] = fmt.Sprintf("%d/%d rounds", g.wins[p], thWinsGoal)
	}
	return out
}

func (g *threeHands) Over() bool { return g.over }

func (g *threeHands) Winners() []int {
	if !g.over || g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
