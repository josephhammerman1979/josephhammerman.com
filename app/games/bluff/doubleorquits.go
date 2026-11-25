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
//     the pot and the claimant takes it.
//   - Challenged and it was a lie: the claimant doubles the pot, which
//     carries over to the next round.
//   - Believed by everyone: the claim is established, and the player to the
//     claimant's left throws and must claim something higher — or pass,
//     dropping out of the round (passed players can't challenge later
//     claims).  If everyone else passes, the established claimant takes the
//     pot.
//
// Chips follow the shared rules in app/games/chips.go: nobody goes negative,
// a player who can't cover a payment is all-in and can only win their own
// stake from each player's share of the pot, and busted players sit out the
// rest of the round, then buy back in or sit out the game.
//
// Actions:
//   thrower: {type:"draw", dice:[indices to expose], index: 0 keep / 1 re-roll
//            hidden / 2 re-roll exposed}, {type:"claim", dice:[5 faces]},
//            {type:"pass"} (only once a claim is established)
//   responder: {type:"believe"}, {type:"challenge"}
//   between rounds, busted players: {type:"rebuy"}, {type:"leave"}

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
			"Challenge and the hand was there: you double the pot and the thrower takes it. Challenge a lie: the thrower doubles the pot and it carries over.",
			"If everyone believes, the next player throws and must claim higher — or pass and sit out the rest of the round (no more challenging).",
			"You can only lose what you have. Short of the full amount? You're all-in, and can only win back your own stake from each player.",
			"Out of chips: sit out the rest of the round, then buy back in or sit out the game. Best result after buy-ins wins.",
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
			"Players who go bust must wait until the round is complete, then buy in again. You can never go negative — you can only lose your stake — and if you can't match the stake, you can only win from the pot what you put in.",
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
	Paid       int        `json:"paid"`  // what the loser put in
	AllIn      bool       `json:"allIn"` // the loser couldn't pay in full
	Won        int        `json:"won"`   // taken by the claimant (0 if it carried over)
	Left       int        `json:"left"`  // left in the pot because the winner was all-in
}

type doq struct {
	env    *games.Env
	seats  games.Seats
	rounds int
	round  int
	st     games.Stacks
	pot    games.Pot

	starter int
	passed  []bool // out of this round: passed, or had no chips at the start

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

	rebuy []bool // between rounds: busted players still deciding
	last  *doqResult
	over  bool
}

func newDoq(env *games.Env, n, rounds, chips int) *doq {
	g := &doq{env: env, seats: games.NewSeats(n), rounds: rounds,
		st: games.NewStacks(n, chips), pot: games.NewPot(n), rebuy: make([]bool, n), starter: -1}
	g.startRound(0)
	return g
}

func (g *doq) canPlay(seat int) bool { return g.seats.In(seat) && g.st.Funded(seat) }

func (g *doq) playable() int {
	n := 0
	for s := 0; s < g.seats.N; s++ {
		if g.canPlay(s) {
			n++
		}
	}
	return n
}

// startRound deals in every player with chips, starting from the first one
// at or after first.
func (g *doq) startRound(first int) {
	if g.playable() < 2 {
		g.finish()
		return
	}
	for !g.canPlay(first) {
		first = (first + 1) % g.seats.N
	}
	g.round++
	g.starter = first
	g.passed = make([]bool, g.seats.N)
	for s := range g.passed {
		g.passed[s] = !g.canPlay(s)
	}
	g.established, g.estBy = nil, -1
	if g.pot.Total() == 0 {
		for _, s := range g.seats.Active() {
			if !g.passed[s] {
				g.st.PayInto(&g.pot, s, doqAnte)
			}
		}
		g.env.Log(games.LogInfo, "Round %d: everyone antes %d — the pot is %d.", g.round, doqAnte, g.pot.Total())
	} else {
		g.env.Log(games.LogInfo, "Round %d: the pot of %d carries over.", g.round, g.pot.Total())
	}
	g.throw(first)
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

// nextIn returns the next player after seat still in this round, or seat.
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
	if g.rebuying() {
		if !g.rebuy[seat] {
			if a.Type == "rebuy" || a.Type == "leave" {
				return games.Errorf("You're not busted.")
			}
			return games.Errorf("Waiting for busted players to buy back in or sit out.")
		}
		switch a.Type {
		case "rebuy":
			g.st.Rebuy(seat)
			g.env.Log(games.LogInfo, "%s bought back in for %d.", P(seat), g.st.Start)
		case "leave":
			g.st.Gone[seat] = true
			g.env.Log(games.LogInfo, "%s is sitting out the rest of the game.", P(seat))
		default:
			return games.ErrUnknown
		}
		g.rebuy[seat] = false
		if !g.rebuying() {
			g.startRound(g.seats.Next(g.starter))
		}
		return nil
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

func (g *doq) rebuying() bool {
	for _, r := range g.rebuy {
		if r {
			return true
		}
	}
	return false
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

// award gives seat the pot, logging anything an all-in winner couldn't take.
func (g *doq) award(seat int) (won, left int) {
	won = g.pot.Award(&g.st, seat)
	left = g.pot.Total()
	if left > 0 {
		g.env.Log(games.LogInfo, "%s was all-in, so %d stays in the pot.", P(seat), left)
	}
	return won, left
}

func (g *doq) takePot(seat int, why string) {
	won, _ := g.award(seat)
	g.env.Log(games.LogGood, "%s takes %d from the pot — %s.", P(seat), won, why)
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
	loser := g.thrower
	if honest {
		loser = seat
	}
	owed := g.pot.Total()
	res.Paid = g.st.PayInto(&g.pot, loser, owed)
	res.AllIn = res.Paid < owed
	short := ""
	if res.AllIn {
		short = fmt.Sprintf(" — all they had, %d of %d", res.Paid, owed)
	}
	if honest {
		g.env.Log(games.LogBust, "The hand was there: %s doubled the pot%s.", P(seat), short)
		res.Won, res.Left = g.award(g.thrower)
		g.env.Log(games.LogGood, "%s takes %d.", P(g.thrower), res.Won)
	} else {
		g.env.Log(games.LogBust, "Caught bluffing: %s doubled the pot to %d%s. It carries over.", P(g.thrower), g.pot.Total(), short)
	}
	g.last = res
	g.endRound()
}

func (g *doq) endRound() {
	g.claim, g.responder = nil, -1
	if g.round >= g.rounds {
		g.finish()
		return
	}
	for _, s := range g.seats.Active() {
		if g.st.Busted(s) {
			g.rebuy[s] = true
			g.env.Log(games.LogBust, "%s is busted.", P(s))
		}
	}
	if !g.rebuying() {
		g.startRound(g.seats.Next(g.starter))
	}
}

func (g *doq) finish() {
	g.over = true
	g.claim, g.responder = nil, -1
	if g.pot.Total() > 0 {
		g.env.Log(games.LogInfo, "The last pot of %d is returned.", g.pot.Total())
		g.pot.Refund(&g.st)
	}
	w := g.Winners()
	names := []string{}
	for _, s := range w {
		names = append(names, P(s))
	}
	g.env.Log(games.LogWin, "Best result: %s at %s.", joinParts(names), signedInt(g.st.Net(w[0])))
}

func signedInt(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprint(n)
}

func (g *doq) Legal(seat int) []games.Action {
	if g.over {
		return nil
	}
	if g.rebuying() {
		if g.rebuy[seat] {
			return append([]games.Action{}, games.RebuyActions...)
		}
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
	if g.rebuying() {
		g.rebuy[seat] = false
		if !g.rebuying() {
			g.startRound(g.seats.Next(g.starter))
		}
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
	Start       int         `json:"start"`
	Chips       []int       `json:"chips"`
	BuyIns      []int       `json:"buyIns"`
	Net         []int       `json:"net"`
	AllIn       []bool      `json:"allIn"`
	Gone        []bool      `json:"gone"`
	Rebuy       []bool      `json:"rebuy"`
	Passed      []bool      `json:"passed"`
	Thrower     int         `json:"thrower"`
	Phase       string      `json:"phase"` // "throw" | "respond" | "rebuy"
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
		Round: g.round, Rounds: g.rounds, Pot: g.pot.Total(), Start: g.st.Start,
		Chips: games.Ints(g.st.Chips), BuyIns: games.Ints(g.st.BuyIns), Net: g.st.Nets(),
		AllIn: append([]bool{}, g.pot.AllIn...), Gone: append([]bool{}, g.st.Gone...),
		Rebuy: append([]bool{}, g.rebuy...), Passed: append([]bool{}, g.passed...),
		Thrower: g.thrower, Responder: g.responder, Drawn: g.drawn,
		Exposed: append([]bool{}, g.exposed...), Mine: seat == g.thrower && !g.over && !g.rebuying(),
		Claim: g.claim, ClaimDice: games.Ints(g.claimDice), Established: g.established, EstBy: g.estBy, Last: g.last,
	}
	switch {
	case g.rebuying():
		v.Phase = "rebuy"
	case g.claim != nil:
		v.Phase = "respond"
	default:
		v.Phase = "throw"
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
	if g.rebuying() {
		if seat >= 0 && g.rebuy[seat] {
			return fmt.Sprintf("You're busted. Buy back in for %d chips, or sit out the rest of the game?", g.st.Start)
		}
		return "Waiting for busted players to buy back in or sit out."
	}
	if g.claim == nil {
		if seat != g.thrower {
			return fmt.Sprintf("Pot %d — %s is throwing.", g.pot.Total(), P(g.thrower))
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
		return fmt.Sprintf("%s claims %s. Believe it or challenge? (Pot %d)", P(g.thrower), g.claim.Name, g.pot.Total())
	}
	return fmt.Sprintf("%s claims %s — waiting for %s.", P(g.thrower), g.claim.Name, P(g.responder))
}

func (g *doq) Waiting() []int {
	if g.over {
		return nil
	}
	if g.rebuying() {
		out := []int{}
		for s, r := range g.rebuy {
			if r {
				out = append(out, s)
			}
		}
		return out
	}
	if g.claim != nil {
		return []int{g.responder}
	}
	return []int{g.thrower}
}

func (g *doq) Badges() []string {
	out := make([]string, g.seats.N)
	for i := range out {
		switch {
		case g.st.Gone[i]:
			out[i] = "sat out"
		case g.st.Busted(i) && !g.over:
			out[i] = "busted"
		default:
			out[i] = fmt.Sprintf("%d chips", g.st.Chips[i])
			if !g.over && g.passed[i] && g.seats.In(i) {
				out[i] += " · passed"
			}
		}
	}
	return out
}

func (g *doq) Over() bool { return g.over }

func (g *doq) Winners() []int {
	if !g.over {
		return nil
	}
	return games.BestSeats(&g.seats, g.st.Nets(), false)
}
