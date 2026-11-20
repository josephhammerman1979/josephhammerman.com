package bluff

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Mia: roll two dice under a cup, declare a value (truthfully or not), and
// pass it on.  The next player accepts — and must roll and declare higher —
// or challenges.  Lose a challenge, lose a life.
//
// Values are written high die first ("65").  From lowest to highest:
// 31 32 41 42 43 51 … 65, then pairs 11 … 66, then Mia (21).
//
// Phases and actions:
//   roll     — the current player starts a fresh sequence: {type:"roll"}
//   declare  — the current player has rolled: {type:"declare", index: value}
//   respond  — the next player: {type:"accept"} (and roll) or {type:"challenge"}

func init() {
	games.Register(&games.Def{
		ID: "mia", Name: "Mia", Order: 80,
		Summary:    "Roll two dice in secret, announce what you rolled — or bluff. Accept and beat it, or call the lie.",
		MinPlayers: 2, MaxPlayers: 8,
		Rules: []string{
			"Roll two dice that only you can see and announce a value: high die first, so 5 and 3 is 53.",
			"Pairs beat ordinary rolls (11 beats 65), and Mia — 2 and 1 — beats everything.",
			"You must announce something higher than the claim you accepted. You may lie.",
			"The next player either accepts (then rolls and must go higher) or challenges.",
			"On a challenge the dice are shown: a liar loses a life, otherwise the challenger does.",
			"Last player with lives left wins.",
		},
		Options: []games.Option{{
			Key: "lives", Label: "Lives",
			Choices: []games.Choice{{Value: "3", Label: "3 lives"}, {Value: "5", Label: "5 lives"}},
		}},
		Facts: []string{
			"Number of dice: 2",
			"Player count: 2-∞",
			"Dice Cups: 1. Get a drink coaster or something for a mat under the dice.",
			"Object: Still have lives after other players are knocked out.",
		},
		Sheet: []string{
			"Distribute three chips to each player. The objective of this game is to survive the longest by being the last player alive with chips.",
			"The hand values are read with the highest value first and the lower value second, as a ten digit number, e.g. “31” for a roll of 3 and 1. Pairs outrank non-paired hands. The exception to this is the hand “21” which is named Mia, and the game is named after.",
			"The first play to act rolls the dice and conceals them. They declare a value. The player to their left chooses to accept the value or they challenge the result. If challenged, the first player reveals the dice. If they have a hand equal to or better than the one they represented, the challenger loses a chip. If they were bluffing, they lose a chip. Play continues, with the winner of the action rolling a fresh hand and declaring a fresh result.",
			"If a player accepts another player bid, regardless of what they roll they must declare a higher hand than the one they accepted. A player can accept a roll of 1-2, but must declare a match!",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newMia(env, n, optInt(opts, "lives", 3))
		},
	})
}

// miaOrder lists every Mia value from lowest to highest.
var miaOrder = []int{31, 32, 41, 42, 43, 51, 52, 53, 54, 61, 62, 63, 64, 65, 11, 22, 33, 44, 55, 66, 21}

const miaMia = 21

func miaRank(v int) int {
	for i, o := range miaOrder {
		if o == v {
			return i
		}
	}
	return -1
}

// miaValue reads two dice as a value: high die first.
func miaValue(d []int) int {
	if len(d) != 2 {
		return 0
	}
	hi, lo := d[0], d[1]
	if lo > hi {
		hi, lo = lo, hi
	}
	return hi*10 + lo
}

func miaName(v int) string {
	switch {
	case v == miaMia:
		return "Mia (2-1)"
	case v/10 == v%10:
		return fmt.Sprintf("%d-%d pair", v/10, v%10)
	}
	return fmt.Sprintf("%d-%d", v/10, v%10)
}

type miaReveal struct {
	Claimant   int    `json:"claimant"`
	Challenger int    `json:"challenger"`
	Dice       []int  `json:"dice"`
	Claim      int    `json:"claim"`
	ClaimName  string `json:"claimName"`
	Real       int    `json:"real"`
	RealName   string `json:"realName"`
	Truthful   bool   `json:"truthful"`
	Loser      int    `json:"loser"`
	LivesLeft  int    `json:"livesLeft"`
}

type mia struct {
	env        *games.Env
	seats      games.Seats
	lives      []int
	startLives int
	phase      string // "roll", "declare", "respond"
	cur        int    // who holds the cup: the roller / declarer / claimant
	dice       []int  // cur's concealed roll (nil before rolling)
	claim      int    // the claim to beat (declare) or cur's claim (respond); 0 = none
	claimBy    int
	reveal     *miaReveal
	winner     int
}

func newMia(env *games.Env, n, lives int) *mia {
	g := &mia{env: env, seats: games.NewSeats(n), lives: make([]int, n), startLives: lives, claimBy: -1, winner: -1}
	for i := range g.lives {
		g.lives[i] = lives
	}
	g.cur = env.Rand.Intn(n)
	g.phase = "roll"
	env.Log(games.LogInfo, "%s was picked at random to roll first.", P(g.cur))
	return g
}

func (g *mia) responder() int {
	if g.phase != "respond" {
		return -1
	}
	return g.seats.Next(g.cur)
}

// legalClaims lists the values cur may declare now, lowest first.
func (g *mia) legalClaims() []int {
	if g.claim == 0 {
		return append([]int{}, miaOrder...)
	}
	if g.claim == miaMia {
		return []int{miaMia}
	}
	return append([]int{}, miaOrder[miaRank(g.claim)+1:]...)
}

func (g *mia) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	switch g.phase {
	case "roll":
		if seat != g.cur {
			return games.ErrNotYourTurn
		}
		if a.Type != "roll" {
			return games.ErrUnknown
		}
		g.dice = g.env.Roll(2)
		g.claim, g.claimBy = 0, -1
		g.phase = "declare"
		g.env.Log(games.LogRoll, "%s rolled under the cup.", P(seat))
	case "declare":
		if seat != g.cur {
			return games.ErrNotYourTurn
		}
		if a.Type != "declare" {
			return games.ErrUnknown
		}
		v := a.Index
		r := miaRank(v)
		if r < 0 {
			return games.Errorf("That isn't a Mia value.")
		}
		if g.claim == miaMia && v != miaMia {
			return games.Errorf("You accepted Mia, so you must declare Mia.")
		}
		if g.claim != 0 && g.claim != miaMia && r <= miaRank(g.claim) {
			return games.Errorf("You must declare higher than %s.", miaName(g.claim))
		}
		g.claim, g.claimBy = v, seat
		g.phase = "respond"
		g.reveal = nil
		g.env.Log(games.LogInfo, "%s claimed %s.", P(seat), miaName(v))
	case "respond":
		if seat != g.responder() {
			return games.ErrNotYourTurn
		}
		switch a.Type {
		case "accept":
			g.env.Log(games.LogInfo, "%s accepted %s from %s and rolled.", P(seat), miaName(g.claim), P(g.cur))
			g.cur = seat
			g.dice = g.env.Roll(2)
			g.phase = "declare"
		case "challenge":
			g.challenge(seat)
		default:
			return games.ErrUnknown
		}
	default:
		return games.ErrUnknown
	}
	return nil
}

func (g *mia) challenge(challenger int) {
	claimant := g.cur
	real := miaValue(g.dice)
	truthful := miaRank(real) >= miaRank(g.claim)
	g.env.Log(games.LogInfo, "%s challenged %s on %s.", P(challenger), P(claimant), miaName(g.claim))
	loser, winner := claimant, challenger
	if truthful {
		loser, winner = challenger, claimant
		g.env.Log(games.LogGood, "%s showed %s — the claim of %s was true.", P(claimant), games.DiceText(g.dice), miaName(g.claim))
	} else {
		g.env.Log(games.LogBust, "%s showed %s — the claim of %s was a bluff.", P(claimant), games.DiceText(g.dice), miaName(g.claim))
	}
	g.lives[loser]--
	g.reveal = &miaReveal{
		Claimant: claimant, Challenger: challenger, Dice: games.Ints(g.dice),
		Claim: g.claim, ClaimName: miaName(g.claim), Real: real, RealName: miaName(real),
		Truthful: truthful, Loser: loser, LivesLeft: g.lives[loser],
	}
	if g.lives[loser] <= 0 {
		g.lives[loser] = 0
		g.seats.Out[loser] = true
		g.env.Log(games.LogBust, "%s lost their last life and is out of the game.", P(loser))
	} else {
		g.env.Log(games.LogBust, "%s lost a life (%s left).", P(loser), plural(g.lives[loser], "life", "lives"))
	}
	if g.checkWin() {
		return
	}
	g.newSequence(winner)
}

func (g *mia) newSequence(roller int) {
	g.cur = roller
	g.phase = "roll"
	g.dice = nil
	g.claim, g.claimBy = 0, -1
}

func (g *mia) checkWin() bool {
	if g.winner >= 0 {
		return true
	}
	if act := g.seats.Active(); len(act) == 1 {
		g.winner = act[0]
		g.phase = "over"
		g.env.Log(games.LogWin, "%s won — the last player with lives!", P(g.winner))
		return true
	}
	return false
}

func (g *mia) Legal(seat int) []games.Action {
	if g.winner >= 0 {
		return nil
	}
	switch g.phase {
	case "roll":
		if seat == g.cur {
			return []games.Action{{Type: "roll"}}
		}
	case "declare":
		if seat == g.cur {
			legal := g.legalClaims()
			out := []games.Action{{Type: "declare", Index: legal[0]}}
			if real := miaValue(g.dice); real != legal[0] && containsInt(legal, real) {
				out = append(out, games.Action{Type: "declare", Index: real})
			}
			if legal[0] != miaMia {
				out = append(out, games.Action{Type: "declare", Index: miaMia})
			}
			return out
		}
	case "respond":
		if seat == g.responder() {
			return []games.Action{{Type: "accept"}, {Type: "challenge"}}
		}
	}
	return nil
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func (g *mia) Remove(seat int) {
	if seat < 0 || seat >= g.seats.N || g.seats.Out[seat] {
		return
	}
	g.seats.Out[seat] = true
	if g.checkWin() {
		return
	}
	if seat == g.cur {
		// The cup leaves with them: start a fresh sequence.
		next := g.seats.Next(seat)
		g.env.Log(games.LogInfo, "The cup passes to %s for a fresh round.", P(next))
		g.newSequence(next)
	}
	// A removed responder is skipped automatically (responder is computed);
	// a removed earlier claimant's claim still has to be beaten.
}

type miaValueInfo struct {
	Value int    `json:"value"`
	Name  string `json:"name"`
}

type miaView struct {
	Phase      string         `json:"phase"`
	Current    int            `json:"current"`
	Responder  int            `json:"responder"`
	Claim      int            `json:"claim"`
	ClaimName  string         `json:"claimName,omitempty"`
	ClaimBy    int            `json:"claimBy"`
	CupDice    int            `json:"cupDice"` // concealed dice under the current player's cup
	MyDice     []int          `json:"myDice,omitempty"`
	MyValue    int            `json:"myValue,omitempty"`
	Legal      []int          `json:"legal,omitempty"`
	Values     []miaValueInfo `json:"values"`
	Lives      []int          `json:"lives"`
	StartLives int            `json:"startLives"`
	Out        []bool         `json:"out"`
	Reveal     *miaReveal     `json:"reveal,omitempty"`
	Winner     int            `json:"winner"`
}

func (g *mia) View(seat int) interface{} {
	v := miaView{
		Phase: g.phase, Current: g.cur, Responder: g.responder(), Claim: g.claim, ClaimBy: g.claimBy,
		Lives: games.Ints(g.lives), StartLives: g.startLives,
		Out: append([]bool{}, g.seats.Out...), Reveal: g.reveal, Winner: g.winner,
	}
	if g.claim != 0 {
		v.ClaimName = miaName(g.claim)
	}
	for _, o := range miaOrder {
		v.Values = append(v.Values, miaValueInfo{o, miaName(o)})
	}
	if g.dice != nil && g.winner < 0 {
		v.CupDice = len(g.dice)
		if seat == g.cur {
			v.MyDice = games.Ints(g.dice)
			v.MyValue = miaValue(g.dice)
		}
	}
	if g.phase == "declare" && seat == g.cur && g.winner < 0 {
		v.Legal = g.legalClaims()
	}
	return v
}

func (g *mia) Prompt(seat int) string {
	if g.winner >= 0 {
		return winPrompt(seat, g.winner)
	}
	switch g.phase {
	case "roll":
		if seat == g.cur {
			return "Roll the dice to start a fresh round — any claim goes."
		}
		return fmt.Sprintf("Waiting for %s to roll.", P(g.cur))
	case "declare":
		if seat == g.cur {
			switch g.claim {
			case 0:
				return "Declare your roll — truthfully or not."
			case miaMia:
				return "You accepted Mia: you must declare Mia."
			}
			return fmt.Sprintf("Declare higher than %s — truthfully or not.", miaName(g.claim))
		}
		return fmt.Sprintf("%s is peeking at their dice…", P(g.cur))
	case "respond":
		r := g.responder()
		if seat == r {
			return fmt.Sprintf("%s claims %s. Accept (and roll) or challenge?", P(g.cur), miaName(g.claim))
		}
		return fmt.Sprintf("Waiting for %s to accept or challenge %s (claimed by %s).", P(r), miaName(g.claim), P(g.cur))
	}
	return ""
}

func (g *mia) Waiting() []int {
	if g.winner >= 0 {
		return nil
	}
	if g.phase == "respond" {
		return []int{g.responder()}
	}
	return []int{g.cur}
}

func (g *mia) Badges() []string {
	out := make([]string, len(g.lives))
	for i, l := range g.lives {
		if g.seats.Out[i] && l > 0 {
			out[i] = "left"
		} else {
			out[i] = hearts(l)
		}
	}
	return out
}

func (g *mia) Over() bool { return g.winner >= 0 }

func (g *mia) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
