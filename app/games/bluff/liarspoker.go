package bluff

import (
	"fmt"
	"sort"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Liar's Poker Dice: one set of five dice passes around the table.  Each die
// is either under the cup (only the holder sees it) or in plain view.  The
// holder bids a poker hand; the next player accepts — takes the cup, may
// rearrange and re-roll, and must bid higher — or challenges.
//
// Phases and actions (all by the holder unless noted):
//   roll     {type:"roll"}                  start a round: all five into the cup
//   bid      {type:"bid", dice:[5 faces]}   bid a hand (higher than the last)
//   respond  {type:"accept"} / {type:"challenge"}   by the next player
//   arrange  {type:"arrange", dice:[indices to put in plain view]}
//   reroll   {type:"reroll", dice:[indices]}  all in the cup or all in view;
//                                            empty to keep everything

const lpDice = 5

func init() {
	games.Register(&games.Def{
		ID: "liarspoker", Name: "Liar's Dice", Order: 100,
		Summary:    "Pass one cup of five dice around, bidding ever-higher poker hands. Believe it and improve it — or call the bluff.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Roll five dice under the cup, look, and bid a poker hand (e.g. “full house, 3s over 6s”). You may lie.",
			"The next player challenges, or accepts and takes the cup.",
			"After accepting, move dice between the cup and plain view, then re-roll some dice — all from the cup or all from view — and bid a higher hand.",
			"Challenge: the cup is lifted. If the hand is at least as good as the bid, the challenger loses a token; otherwise the bidder does.",
			"Run out of tokens and you're out. Last player with tokens wins.",
		},
		Options: []games.Option{{
			Key: "tokens", Label: "Tokens",
			Choices: []games.Choice{{Value: "3", Label: "3 tokens"}, {Value: "5", Label: "5 tokens"}},
		}},
		Facts: []string{
			"Number of dice: 5", "Number of cups: 1",
			"Number of tokens: 3 or more per player, depending on desired game length",
			"Player count: 2 to 6", "Object: win the game",
		},
		Sheet: []string{
			"This game is similar to Perdudo.",
			"The first player receives five dice & a cup. The first player to bid declares a poker dice hand value. After the bid is declared, the player to the left of the bidder either accepts the bid, or challenges the bid.",
			"Accepting the bid: Upon accepting player A’s claim, player B may take any dice out of the cup and place them in plain view, and may take any dice in plain view and put them in the cup.",
			"They may then re-roll any number of dice in or out of the cup, but not both. They then must bid a higher poker dice hand to the player to their left (they may have to lie!).",
			"Challenging the bid: If the bid is challenged,the bidder shows their entire hand. If the revealed hand is as good as the declared hand or better, the bidder wins,",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newLiarsPoker(env, n, optInt(opts, "tokens", 3))
		},
	})
}

// allHands lists every distinct five-dice hand (faces sorted high first),
// weakest to strongest.
var allHands = func() [][]int {
	out := [][]int{}
	var rec func(prefix []int, top int)
	rec = func(prefix []int, top int) {
		if len(prefix) == lpDice {
			out = append(out, games.Ints(prefix))
			return
		}
		for f := top; f >= 1; f-- {
			rec(append(prefix, f), f)
		}
	}
	rec(nil, 6)
	sort.Slice(out, func(i, j int) bool {
		return games.CompareHands(games.EvalHand(out[i]), games.EvalHand(out[j])) < 0
	})
	return out
}()

// lpMinRaise is the weakest hand that beats bid, or nil if none does.
func lpMinRaise(bid []int) []int {
	b := games.EvalHand(bid)
	for _, h := range allHands {
		if games.CompareHands(games.EvalHand(h), b) > 0 {
			return games.Ints(h)
		}
	}
	return nil
}

// lpSorted puts a hand's faces in the order its name reads (groups first).
func lpSorted(d []int) []int { return games.Ints(games.EvalHand(d).Key) }

type lpReveal struct {
	Bidder     int    `json:"bidder"`
	Challenger int    `json:"challenger"`
	Dice       []int  `json:"dice"`
	Bid        []int  `json:"bid"`
	BidName    string `json:"bidName"`
	RealName   string `json:"realName"`
	Truthful   bool   `json:"truthful"`
	Loser      int    `json:"loser"`
	TokensLeft int    `json:"tokensLeft"`
}

type liarsPoker struct {
	env         *games.Env
	seats       games.Seats
	tokens      []int
	startTokens int
	round       int
	phase       string // "roll", "bid", "respond", "arrange", "reroll"
	holder      int    // who has the cup
	dice        []int  // nil before the round's roll
	inView      []bool
	bid         []int // nil = no bid this round
	bidder      int
	reveal      *lpReveal
	winner      int
}

func newLiarsPoker(env *games.Env, n, tokens int) *liarsPoker {
	g := &liarsPoker{env: env, seats: games.NewSeats(n), tokens: make([]int, n), startTokens: tokens,
		inView: make([]bool, lpDice), bidder: -1, winner: -1}
	for i := range g.tokens {
		g.tokens[i] = tokens
	}
	g.newRound(0)
	return g
}

func (g *liarsPoker) newRound(holder int) {
	g.round++
	g.holder = holder
	g.phase = "roll"
	g.dice = nil
	g.inView = make([]bool, lpDice)
	g.bid, g.bidder = nil, -1
}

func (g *liarsPoker) responder() int {
	if g.phase != "respond" {
		return -1
	}
	return g.seats.Next(g.holder)
}

func (g *liarsPoker) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	if g.phase == "respond" {
		if seat != g.responder() {
			return games.ErrNotYourTurn
		}
		switch a.Type {
		case "accept":
			if lpMinRaise(g.bid) == nil {
				return games.Errorf("Nobody can beat five 6s — you have to challenge.")
			}
			g.env.Log(games.LogInfo, "%s accepted %s from %s and took the cup.", P(seat), games.EvalHand(g.bid).Name, P(g.holder))
			g.holder = seat
			g.phase = "arrange"
		case "challenge":
			g.challenge(seat)
		default:
			return games.ErrUnknown
		}
		return nil
	}
	if seat != g.holder {
		return games.ErrNotYourTurn
	}
	switch {
	case g.phase == "roll" && a.Type == "roll":
		g.dice = g.env.Roll(lpDice)
		g.inView = make([]bool, lpDice)
		g.reveal = nil
		g.phase = "bid"
		g.env.Log(games.LogRoll, "Round %d: %s rolled all five dice into the cup.", g.round, P(seat))
	case g.phase == "arrange" && a.Type == "arrange":
		idx, err := games.UniqueIndices(a.Dice, lpDice)
		if err != nil {
			return err
		}
		g.inView = make([]bool, lpDice)
		shown := []int{}
		for _, i := range idx {
			g.inView[i] = true
			shown = append(shown, g.dice[i])
		}
		g.phase = "reroll"
		if len(shown) == 0 {
			g.env.Log(games.LogInfo, "%s kept all five dice in the cup.", P(seat))
		} else {
			g.env.Log(games.LogInfo, "%s put %s in plain view: %s.", P(seat), plural(len(shown), "die", "dice"), games.DiceText(shown))
		}
	case g.phase == "reroll" && a.Type == "reroll":
		idx, err := games.UniqueIndices(a.Dice, lpDice)
		if err != nil {
			return err
		}
		if len(idx) > 0 {
			side := g.inView[idx[0]]
			for _, i := range idx {
				if g.inView[i] != side {
					return games.Errorf("Re-roll dice from the cup or from plain view, not both.")
				}
			}
			fresh := g.env.Roll(len(idx))
			for k, i := range idx {
				g.dice[i] = fresh[k]
			}
			if side {
				g.env.Log(games.LogRoll, "%s re-rolled %s in plain view: %s.", P(seat), plural(len(idx), "die", "dice"), games.DiceText(fresh))
			} else {
				g.env.Log(games.LogRoll, "%s re-rolled %s in the cup.", P(seat), plural(len(idx), "die", "dice"))
			}
		} else {
			g.env.Log(games.LogInfo, "%s didn't re-roll.", P(seat))
		}
		g.phase = "bid"
	case g.phase == "bid" && a.Type == "bid":
		if len(a.Dice) != lpDice {
			return games.Errorf("A bid is a hand of five dice.")
		}
		if !validFaces(a.Dice) {
			return games.Errorf("Each die in a bid must be 1 to 6.")
		}
		h := games.EvalHand(a.Dice)
		if g.bid != nil && games.CompareHands(h, games.EvalHand(g.bid)) <= 0 {
			return games.Errorf("You must bid higher than %s.", games.EvalHand(g.bid).Name)
		}
		g.bid = lpSorted(a.Dice)
		g.bidder = seat
		g.phase = "respond"
		g.env.Log(games.LogInfo, "%s bid %s (%s).", P(seat), h.Name, games.DiceText(g.bid))
	default:
		return games.ErrUnknown
	}
	return nil
}

func (g *liarsPoker) challenge(challenger int) {
	bidder := g.holder
	bidHand, realHand := games.EvalHand(g.bid), games.EvalHand(g.dice)
	truthful := games.CompareHands(realHand, bidHand) >= 0
	g.env.Log(games.LogInfo, "%s challenged %s on %s. The cup is lifted!", P(challenger), P(bidder), bidHand.Name)
	loser, winner := bidder, challenger
	if truthful {
		loser, winner = challenger, bidder
		g.env.Log(games.LogGood, "%s had %s (%s) — the bid of %s was good.", P(bidder), games.DiceText(g.dice), realHand.Name, bidHand.Name)
	} else {
		g.env.Log(games.LogBust, "%s had %s (%s) — the bid of %s was a bluff.", P(bidder), games.DiceText(g.dice), realHand.Name, bidHand.Name)
	}
	g.tokens[loser]--
	g.reveal = &lpReveal{
		Bidder: bidder, Challenger: challenger, Dice: games.Ints(g.dice), Bid: games.Ints(g.bid),
		BidName: bidHand.Name, RealName: realHand.Name, Truthful: truthful, Loser: loser, TokensLeft: g.tokens[loser],
	}
	if g.tokens[loser] <= 0 {
		g.tokens[loser] = 0
		g.seats.Out[loser] = true
		g.env.Log(games.LogBust, "%s lost their last token and is out of the game.", P(loser))
	} else {
		g.env.Log(games.LogBust, "%s lost a token (%s left).", P(loser), plural(g.tokens[loser], "token", "tokens"))
	}
	if g.checkWin() {
		return
	}
	g.newRound(winner)
}

func (g *liarsPoker) checkWin() bool {
	if g.winner >= 0 {
		return true
	}
	if act := g.seats.Active(); len(act) == 1 {
		g.winner = act[0]
		g.phase = "over"
		g.env.Log(games.LogWin, "%s won — the last player with tokens!", P(g.winner))
		return true
	}
	return false
}

// suggestion is a sensible default bid for the holder: their real hand if it
// is good enough, else the smallest raise.
func (g *liarsPoker) suggestion() []int {
	if g.dice == nil {
		return nil
	}
	if !validFaces(g.dice) { // can't happen in play; keeps views safe
		if g.bid == nil {
			return []int{6, 5, 4, 3, 1}
		}
		return lpMinRaise(g.bid)
	}
	if g.bid == nil || games.CompareHands(games.EvalHand(g.dice), games.EvalHand(g.bid)) > 0 {
		return lpSorted(g.dice)
	}
	return lpMinRaise(g.bid)
}

func validFaces(d []int) bool {
	for _, f := range d {
		if f < 1 || f > 6 {
			return false
		}
	}
	return true
}

func (g *liarsPoker) sideIndices(view bool) []int {
	out := []int{}
	for i, v := range g.inView {
		if v == view {
			out = append(out, i)
		}
	}
	return out
}

func (g *liarsPoker) Legal(seat int) []games.Action {
	if g.winner >= 0 {
		return nil
	}
	if g.phase == "respond" {
		if seat != g.responder() {
			return nil
		}
		out := []games.Action{{Type: "challenge"}}
		if lpMinRaise(g.bid) != nil {
			out = append(out, games.Action{Type: "accept"})
		}
		return out
	}
	if seat != g.holder {
		return nil
	}
	switch g.phase {
	case "roll":
		return []games.Action{{Type: "roll"}}
	case "arrange":
		return []games.Action{
			{Type: "arrange", Dice: g.sideIndices(true)},
			{Type: "arrange", Dice: []int{}},
			{Type: "arrange", Dice: []int{0, 1}},
		}
	case "reroll":
		out := []games.Action{{Type: "reroll", Dice: []int{}}}
		if c := g.sideIndices(false); len(c) > 0 {
			out = append(out, games.Action{Type: "reroll", Dice: c})
		}
		if v := g.sideIndices(true); len(v) > 0 {
			out = append(out, games.Action{Type: "reroll", Dice: v[:1]})
		}
		return out
	case "bid":
		out := []games.Action{{Type: "bid", Dice: g.suggestion()}}
		top := []int{6, 6, 6, 6, 6}
		if g.bid == nil || games.CompareHands(games.EvalHand(top), games.EvalHand(g.bid)) > 0 {
			out = append(out, games.Action{Type: "bid", Dice: top})
		}
		return out
	}
	return nil
}

func (g *liarsPoker) Remove(seat int) {
	if seat < 0 || seat >= g.seats.N || g.seats.Out[seat] {
		return
	}
	g.seats.Out[seat] = true
	if g.checkWin() {
		return
	}
	if seat == g.holder {
		next := g.seats.Next(seat)
		g.env.Log(games.LogInfo, "The cup passes to %s for a fresh round.", P(next))
		g.newRound(next)
	}
	// A removed responder is skipped automatically; a removed earlier
	// bidder's bid still has to be beaten.
}

type lpView struct {
	Phase       string    `json:"phase"`
	Round       int       `json:"round"`
	Holder      int       `json:"holder"`
	Responder   int       `json:"responder"`
	Dice        []int     `json:"dice"`   // 0 = in the cup, hidden from this seat
	InView      []bool    `json:"inView"` // which dice are in plain view
	CanSee      bool      `json:"canSee"` // this seat sees the dice in the cup
	MyHand      string    `json:"myHand,omitempty"`
	Bid         []int     `json:"bid,omitempty"`
	BidName     string    `json:"bidName,omitempty"`
	Bidder      int       `json:"bidder"`
	CanAccept   bool      `json:"canAccept"`
	Suggest     []int     `json:"suggest,omitempty"`
	Tokens      []int     `json:"tokens"`
	StartTokens int       `json:"startTokens"`
	Out         []bool    `json:"out"`
	Reveal      *lpReveal `json:"reveal,omitempty"`
	Winner      int       `json:"winner"`
	Rankings    []string  `json:"rankings"`
}

func (g *liarsPoker) View(seat int) interface{} {
	v := lpView{
		Phase: g.phase, Round: g.round, Holder: g.holder, Responder: g.responder(),
		InView: append([]bool{}, g.inView...), Bidder: g.bidder,
		Tokens: games.Ints(g.tokens), StartTokens: g.startTokens, Out: append([]bool{}, g.seats.Out...),
		Reveal: g.reveal, Winner: g.winner, Rankings: games.HandRankingNames,
	}
	if g.dice != nil && g.winner < 0 {
		v.CanSee = seat == g.holder
		v.Dice = make([]int, lpDice)
		for i, d := range g.dice {
			if v.CanSee || g.inView[i] {
				v.Dice[i] = d
			}
		}
		if v.CanSee && validFaces(g.dice) {
			v.MyHand = games.EvalHand(g.dice).Name
		}
	}
	if g.bid != nil {
		v.Bid = games.Ints(g.bid)
		v.BidName = games.EvalHand(g.bid).Name
		v.CanAccept = g.phase == "respond" && lpMinRaise(g.bid) != nil
	}
	if g.phase == "bid" && seat == g.holder && g.winner < 0 {
		v.Suggest = g.suggestion()
	}
	return v
}

func (g *liarsPoker) Prompt(seat int) string {
	if g.winner >= 0 {
		return winPrompt(seat, g.winner)
	}
	mine := seat == g.holder
	switch g.phase {
	case "roll":
		if mine {
			return "Roll all five dice into the cup to start the round."
		}
		return fmt.Sprintf("Waiting for %s to roll.", P(g.holder))
	case "bid":
		if mine {
			if g.bid == nil {
				return "Bid a poker hand — truthfully or not."
			}
			return fmt.Sprintf("Bid higher than %s — truthfully or not.", games.EvalHand(g.bid).Name)
		}
		return fmt.Sprintf("%s is deciding what to bid…", P(g.holder))
	case "respond":
		r := g.responder()
		name := games.EvalHand(g.bid).Name
		if seat == r {
			if lpMinRaise(g.bid) == nil {
				return fmt.Sprintf("%s bids %s — nothing beats it, so you must challenge.", P(g.holder), name)
			}
			return fmt.Sprintf("%s bids %s. Accept (take the cup) or challenge?", P(g.holder), name)
		}
		return fmt.Sprintf("Waiting for %s to accept or challenge %s (bid by %s).", P(r), name, P(g.holder))
	case "arrange":
		if mine {
			return "Tap the dice you want in plain view (the rest stay in the cup)."
		}
		return fmt.Sprintf("%s is arranging the dice…", P(g.holder))
	case "reroll":
		if mine {
			return "Tap dice to re-roll — all from the cup or all from view — or keep them all."
		}
		return fmt.Sprintf("%s may re-roll some dice…", P(g.holder))
	}
	return ""
}

func (g *liarsPoker) Waiting() []int {
	if g.winner >= 0 {
		return nil
	}
	if g.phase == "respond" {
		return []int{g.responder()}
	}
	return []int{g.holder}
}

func (g *liarsPoker) Badges() []string {
	out := make([]string, g.seats.N)
	for i, t := range g.tokens {
		switch {
		case g.seats.Out[i] && t == 0:
			out[i] = "out"
		case g.seats.Out[i]:
			out[i] = "left"
		default:
			out[i] = plural(t, "token", "tokens")
			if i == g.holder && g.dice != nil {
				out[i] += " · cup"
			}
		}
	}
	return out
}

func (g *liarsPoker) Over() bool { return g.winner >= 0 }

func (g *liarsPoker) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
