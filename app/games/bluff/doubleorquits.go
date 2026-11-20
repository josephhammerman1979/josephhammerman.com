package bluff

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Double or Quits: a bluffing game for a growing pot.
//
// Everyone antes into the pot.  The thrower rolls five dice in secret and
// may make one five-card-draw style redraw: expose any number of dice, then
// re-roll either the exposed dice or the hidden ones.  They then claim a
// poker hand.  Going round the table, each player still in the round
// believes the claim or challenges it.
//
//   - Challenged and the hand is as good as claimed: the challenger doubles
//     the pot and the claimant takes it all.
//   - Challenged and it was a lie: the claimant doubles the pot, which
//     carries over to the next round.
//   - Believed by everyone: the claim is established, and the player to the
//     claimant's left throws and must claim something higher — or pass,
//     dropping out of the round (passed players can't challenge later
//     claims).  If everyone else passes, the established claimant takes the
//     pot.
//
// Actions:
//   thrower: {type:"draw", dice:[indices to expose], index: 0 keep / 1 re-roll
//            hidden / 2 re-roll exposed}, {type:"claim", dice:[5 faces]},
//            {type:"pass"} (only once a claim is established)
//   responder: {type:"believe"}, {type:"challenge"}

const (
	doqDice = 5
	doqAnte = 1
)

func init() {
	games.Register(&games.Def{
		ID: "doubleorquits", Name: "Double or Quits", Order: 107,
		Summary:    "Bluff a poker hand for the pot. Get caught and you double it; catch a liar wrong and you pay.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Everyone antes 1 chip. The thrower rolls five hidden dice and may redraw once: expose any dice, then re-roll either the exposed ones or the hidden ones.",
			"The thrower claims a poker hand — true or not. In turn, everyone still in the round believes it or challenges.",
			"Challenge and the hand was there: you double the pot and the thrower takes it all. Challenge a lie: the thrower doubles the pot and it carries over.",
			"If everyone believes, the next player throws and must claim higher — or pass and sit out the rest of the round (no more challenging).",
			"If everyone else passes, the standing claim takes the pot. Most chips after the last round wins.",
		},
		Options: []games.Option{
			{Key: "rounds", Label: "Rounds", Choices: []games.Choice{
				{Value: "8", Label: "8 rounds"}, {Value: "5", Label: "5 rounds"}, {Value: "12", Label: "12 rounds"},
			}},
			{Key: "chips", Label: "Starting chips", Choices: []games.Choice{
				{Value: "20", Label: "20 chips"}, {Value: "50", Label: "50 chips"},
			}},
		},
		Facts: []string{"Number of dice: 5", "Number of cups: 1", "Object: Most chips at the end"},
		Sheet: []string{
			"Everyone stakes an ante. A player rolls a hidden hand and announces a value. The claim proceeds around the circle until someone disbelieves them.",
			"If they are disbelieved the dice are revealed, and if they had the hand they claimed or better they win and the challenger doubles the pot. If they are revealed to have been lying they must double the pot.",
			"If the claim goes around the table, the player to their left throws and must make a higher claim than the one that succeeded earlier in the round.",
			"House rules: a five-card-draw style redraw — expose any number of dice, then re-roll the exposed or the hidden dice. Players can pass when they are establishing the result if they cannot beat the current established hand; players who pass are out of the round and cannot challenge any future results.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newDoq(env, n, optInt(opts, "rounds", 8), optInt(opts, "chips", 20))
		},
	})
}

// doqResult is the public outcome of the last challenge, shown until the
// next claim.
type doqResult struct {
	Claimant   int        `json:"claimant"`
	Challenger int        `json:"challenger"`
	Claim      games.Hand `json:"claim"`
	Dice       []int      `json:"dice"`
	Actual     games.Hand `json:"actual"`
	Honest     bool       `json:"honest"`
	Paid       int        `json:"paid"` // what the loser put in
	Won        int        `json:"won"`  // pot taken by the claimant (0 if it carried over)
}

type doq struct {
	env    *games.Env
	seats  games.Seats
	rounds int
	round  int
	chips  []int
	pot    int

	starter int
	passed  []bool

	// The current throw.
	thrower int
	dice    []int
	exposed []bool
	drawn   bool

	// The claim being answered, and the one established before it.
	claim       *games.Hand
	claimDice   []int // the claimed faces as entered
	responder   int
	established *games.Hand
	estBy       int

	last *doqResult
	over bool
}

func newDoq(env *games.Env, n, rounds, chips int) *doq {
	g := &doq{env: env, seats: games.NewSeats(n), rounds: rounds, chips: make([]int, n)}
	for i := range g.chips {
		g.chips[i] = chips
	}
	g.startRound(0)
	return g
}

func (g *doq) startRound(starter int) {
	g.round++
	g.starter = starter
	g.passed = make([]bool, g.seats.N)
	g.established, g.estBy = nil, -1
	if g.pot == 0 {
		for _, s := range g.seats.Active() {
			g.chips[s] -= doqAnte
			g.pot += doqAnte
		}
		g.env.Log(games.LogInfo, "Round %d: everyone antes %d — the pot is %d.", g.round, doqAnte, g.pot)
	} else {
		g.env.Log(games.LogInfo, "Round %d: the pot of %d carries over.", g.round, g.pot)
	}
	g.throw(starter)
}

// throw gives seat a fresh hidden hand.
func (g *doq) throw(seat int) {
	g.thrower = seat
	g.dice = g.env.Roll(doqDice)
	g.exposed = make([]bool, doqDice)
	g.drawn = false
	g.claim, g.claimDice = nil, nil
	g.responder = -1
	g.env.Log(games.LogRoll, "%s threw five dice under the cup.", P(seat))
}

// inRound lists players who haven't passed, clockwise from seat.
func (g *doq) inRoundFrom(seat int) []int {
	out := []int{}
	for _, s := range g.seats.OrderFrom(seat) {
		if !g.passed[s] {
			out = append(out, s)
		}
	}
	return out
}

// nextIn returns the next player after seat who hasn't passed, or seat.
func (g *doq) nextIn(seat int) int {
	for i := 1; i <= g.seats.N; i++ {
		s := (seat + i) % g.seats.N
		if g.seats.In(s) && !g.passed[s] {
			return s
		}
	}
	return seat
}

func (g *doq) Act(seat int, a games.Action) error {
	if g.over {
		return games.ErrGameOver
	}
	if g.claim == nil {
		if seat != g.thrower {
			return games.ErrNotYourTurn
		}
		switch a.Type {
		case "draw":
			return g.draw(seat, a)
		case "claim":
			return g.makeClaim(seat, a.Dice)
		case "pass":
			if g.established == nil {
				return games.Errorf("You're opening the round — make a claim.")
			}
			g.pass(seat)
			return nil
		}
		return games.ErrUnknown
	}
	if seat != g.responder {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "believe":
		g.env.Log(games.LogInfo, "%s believed it.", P(seat))
		next := g.nextIn(seat)
		if next == g.thrower {
			g.established, g.estBy = g.claim, g.thrower
			g.env.Log(games.LogGood, "Everyone believed %s: %s stands.", P(g.thrower), g.claim.Name)
			g.nextThrower(g.thrower)
			return nil
		}
		g.responder = next
		return nil
	case "challenge":
		g.challenge(seat)
		return nil
	}
	return games.ErrUnknown
}

func (g *doq) draw(seat int, a games.Action) error {
	if g.drawn {
		return games.Errorf("You've already redrawn.")
	}
	idx, err := games.UniqueIndices(a.Dice, doqDice)
	if err != nil {
		return err
	}
	if a.Index < 0 || a.Index > 2 {
		return games.Errorf("Choose which dice to re-roll.")
	}
	if a.Index == 2 && len(idx) == 0 {
		return games.Errorf("Expose some dice to re-roll them.")
	}
	for _, i := range idx {
		g.exposed[i] = true
	}
	g.drawn = true
	reroll := []int{}
	for i := range g.dice {
		if (a.Index == 1 && !g.exposed[i]) || (a.Index == 2 && g.exposed[i]) {
			reroll = append(reroll, i)
		}
	}
	if len(idx) > 0 {
		shown := []int{}
		for i := range g.dice {
			if g.exposed[i] {
				shown = append(shown, g.dice[i])
			}
		}
		g.env.Log(games.LogInfo, "%s exposed %s.", P(seat), games.DiceText(shown))
	}
	if len(reroll) > 0 {
		fresh := g.env.Roll(len(reroll))
		for k, i := range reroll {
			g.dice[i] = fresh[k]
		}
		which := "hidden"
		if a.Index == 2 {
			which = "exposed"
		}
		g.env.Log(games.LogRoll, "%s re-rolled %d %s dice.", P(seat), len(reroll), which)
	} else {
		g.env.Log(games.LogInfo, "%s stood pat.", P(seat))
	}
	return nil
}

func (g *doq) makeClaim(seat int, faces []int) error {
	if len(faces) != doqDice {
		return games.Errorf("A claim is five dice.")
	}
	for _, f := range faces {
		if f < 1 || f > 6 {
			return games.Errorf("Each die is 1 to 6.")
		}
	}
	h := games.EvalHand(faces)
	if g.established != nil && games.CompareHands(h, *g.established) <= 0 {
		return games.Errorf("You must claim better than %s.", g.established.Name)
	}
	g.claim, g.claimDice = &h, games.Ints(faces)
	g.last = nil
	g.env.Log(games.LogInfo, "%s claimed %s.", P(seat), h.Name)
	g.responder = g.nextIn(seat)
	if g.responder == seat {
		// Everyone else has passed: nobody can challenge.
		g.established, g.estBy = g.claim, seat
		g.takePot(seat, "nobody is left to challenge")
	}
	return nil
}

func (g *doq) pass(seat int) {
	g.passed[seat] = true
	g.env.Log(games.LogBust, "%s couldn't beat %s and passed.", P(seat), g.established.Name)
	next := g.nextIn(seat)
	if next == g.estBy || next == seat {
		g.takePot(g.estBy, "everyone else passed")
		return
	}
	g.throw(next)
}

// nextThrower passes the throw to the left of seat, or awards the pot if
// nobody but the established claimant is left.
func (g *doq) nextThrower(seat int) {
	next := g.nextIn(seat)
	if next == g.estBy {
		g.takePot(g.estBy, "everyone else passed")
		return
	}
	g.throw(next)
}

func (g *doq) takePot(seat int, why string) {
	g.env.Log(games.LogGood, "%s takes the pot of %d — %s.", P(seat), g.pot, why)
	g.chips[seat] += g.pot
	g.pot = 0
	g.endRound()
}

func (g *doq) challenge(seat int) {
	actual := games.EvalHand(g.dice)
	honest := games.CompareHands(actual, *g.claim) >= 0
	res := &doqResult{
		Claimant: g.thrower, Challenger: seat, Claim: *g.claim,
		Dice: games.Ints(g.dice), Actual: actual, Honest: honest,
	}
	g.env.Log(games.LogRoll, "%s challenged! %s showed %s (%s) against a claim of %s.",
		P(seat), P(g.thrower), games.DiceText(g.dice), actual.Name, g.claim.Name)
	if honest {
		res.Paid = g.pot
		g.chips[seat] -= g.pot
		g.pot *= 2
		res.Won = g.pot
		g.env.Log(games.LogBust, "The hand was there: %s doubled the pot to %d.", P(seat), g.pot)
		g.env.Log(games.LogGood, "%s takes the pot of %d.", P(g.thrower), g.pot)
		g.chips[g.thrower] += g.pot
		g.pot = 0
	} else {
		res.Paid = g.pot
		g.chips[g.thrower] -= g.pot
		g.pot *= 2
		g.env.Log(games.LogBust, "Caught bluffing: %s doubled the pot to %d. It carries over.", P(g.thrower), g.pot)
	}
	g.last = res
	g.endRound()
}

func (g *doq) endRound() {
	g.claim, g.responder = nil, -1
	if g.round >= g.rounds {
		g.over = true
		w := g.Winners()
		if g.pot > 0 {
			g.env.Log(games.LogInfo, "The last pot of %d goes unclaimed.", g.pot)
		}
		names := []string{}
		for _, s := range w {
			names = append(names, P(s))
		}
		g.env.Log(games.LogWin, "Most chips: %s with %d.", joinParts(names), g.chips[w[0]])
		return
	}
	g.startRound(g.seats.Next(g.starter))
}

func (g *doq) Legal(seat int) []games.Action {
	if g.over {
		return nil
	}
	if g.claim != nil {
		if seat != g.responder {
			return nil
		}
		return []games.Action{{Type: "believe"}, {Type: "challenge"}}
	}
	if seat != g.thrower {
		return nil
	}
	out := []games.Action{}
	if !g.drawn {
		out = append(out,
			games.Action{Type: "draw", Index: 0},
			games.Action{Type: "draw", Index: 1},
			games.Action{Type: "draw", Dice: []int{0, 1}, Index: 2},
			games.Action{Type: "draw", Dice: []int{2}, Index: 1})
	}
	for _, faces := range [][]int{g.dice, {6, 6, 6, 6, 6}} {
		h := games.EvalHand(faces)
		if g.established == nil || games.CompareHands(h, *g.established) > 0 {
			out = append(out, games.Action{Type: "claim", Dice: games.Ints(faces)})
		}
	}
	if g.established != nil {
		out = append(out, games.Action{Type: "pass"})
	}
	return out
}

func (g *doq) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.over || g.seats.ActiveCount() < 2 {
		return
	}
	g.passed[seat] = true
	switch {
	case g.claim == nil && seat == g.thrower:
		if g.established != nil && g.estBy != seat {
			g.nextThrower(seat)
		} else {
			// The opener or the established claimant left: start the throw
			// over with the next player and no claim to beat.
			g.established, g.estBy = nil, -1
			g.throw(g.nextIn(seat))
		}
	case g.claim != nil && seat == g.thrower:
		// The claimant left mid-claim: their claim lapses.
		g.established, g.estBy = nil, -1
		g.throw(g.nextIn(seat))
	case g.claim != nil && seat == g.responder:
		next := g.nextIn(seat)
		if next == g.thrower {
			g.established, g.estBy = g.claim, g.thrower
			g.nextThrower(g.thrower)
		} else {
			g.responder = next
		}
	case seat == g.estBy && g.claim == nil:
		g.established, g.estBy = nil, -1
	}
}

type doqView struct {
	Round       int         `json:"round"`
	Rounds      int         `json:"rounds"`
	Pot         int         `json:"pot"`
	Chips       []int       `json:"chips"`
	Passed      []bool      `json:"passed"`
	Thrower     int         `json:"thrower"`
	Phase       string      `json:"phase"` // "throw" | "respond"
	Responder   int         `json:"responder"`
	Drawn       bool        `json:"drawn"`
	Dice        []int       `json:"dice"` // the thrower's dice: hidden ones are 0 for everyone else
	Exposed     []bool      `json:"exposed"`
	Mine        bool        `json:"mine"` // the viewer is the thrower and sees every die
	Claim       *games.Hand `json:"claim,omitempty"`
	ClaimDice   []int       `json:"claimDice,omitempty"`
	Established *games.Hand `json:"established,omitempty"`
	EstBy       int         `json:"estBy"`
	Last        *doqResult  `json:"last,omitempty"`
}

func (g *doq) View(seat int) interface{} {
	v := doqView{
		Round: g.round, Rounds: g.rounds, Pot: g.pot, Chips: games.Ints(g.chips),
		Passed: append([]bool{}, g.passed...), Thrower: g.thrower, Responder: g.responder,
		Drawn: g.drawn, Exposed: append([]bool{}, g.exposed...), Mine: seat == g.thrower && !g.over,
		Claim: g.claim, ClaimDice: games.Ints(g.claimDice), Established: g.established, EstBy: g.estBy, Last: g.last,
	}
	v.Phase = "throw"
	if g.claim != nil {
		v.Phase = "respond"
	}
	v.Dice = make([]int, len(g.dice))
	for i, d := range g.dice {
		if v.Mine || g.exposed[i] {
			v.Dice[i] = d
		}
	}
	return v
}

func (g *doq) Prompt(seat int) string {
	if g.over {
		return "The last round is over."
	}
	if g.claim == nil {
		if seat != g.thrower {
			return fmt.Sprintf("Pot %d — %s is throwing.", g.pot, P(g.thrower))
		}
		switch {
		case !g.drawn && g.established != nil:
			return fmt.Sprintf("Beat %s: redraw if you like, then claim higher — or pass.", g.established.Name)
		case !g.drawn:
			return "Redraw if you like (expose dice, re-roll exposed or hidden), then claim a hand."
		case g.established != nil:
			return fmt.Sprintf("Claim better than %s, or pass.", g.established.Name)
		}
		return "Claim a hand — true or not."
	}
	if seat == g.responder {
		return fmt.Sprintf("%s claims %s. Believe it or challenge? (Pot %d)", P(g.thrower), g.claim.Name, g.pot)
	}
	return fmt.Sprintf("%s claims %s — waiting for %s.", P(g.thrower), g.claim.Name, P(g.responder))
}

func (g *doq) Waiting() []int {
	if g.over {
		return nil
	}
	if g.claim != nil {
		return []int{g.responder}
	}
	return []int{g.thrower}
}

func (g *doq) Badges() []string {
	out := make([]string, len(g.chips))
	for i, c := range g.chips {
		out[i] = fmt.Sprintf("%d chips", c)
		if !g.over && g.passed[i] && g.seats.In(i) {
			out[i] += " · passed"
		}
	}
	return out
}

func (g *doq) Over() bool { return g.over }

func (g *doq) Winners() []int {
	if !g.over {
		return nil
	}
	return games.BestSeats(&g.seats, g.chips, false)
}
