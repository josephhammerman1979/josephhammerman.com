package pushluck

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Threes: five dice, keep at least one per roll; lowest total wins the pot,
// with 3s counting as zero.  Five 6s ("Shooting the Moon") wins outright.
//
// Actions: {type:"roll"} starts a turn; {type:"keep", dice:[indices]} keeps
// those dice and rolls the rest.

const threesDice = 5

func init() {
	games.Register(&games.Def{
		ID: "threes", Name: "Threes", Order: 70,
		Summary:    "Lowest score wins the pot — and 3s count as zero. Five 6s shoots the moon.",
		MinPlayers: 2, MaxPlayers: 8,
		Options: []games.Option{chipsOption, {Key: "hands", Label: "Hands", Choices: []games.Choice{
			{Value: "5", Label: "5 hands"}, {Value: "3", Label: "3 hands"}, {Value: "8", Label: "8 hands"},
		}}},
		Rules: []string{
			"Everyone antes 1 chip into the pot each hand.",
			"On your turn roll five dice. After every roll keep at least one die (tap it), then roll the rest, until all five are kept.",
			"Add up your dice, but 3s count as zero. Lowest score wins the pot; 3-3-3-3-3 = 0 is the best.",
			"Finish with 6-6-6-6-6 (\"Shooting the Moon\") and you win the hand instantly.",
			"Tied players ante again and replay between themselves. The winner rolls first next hand.",
			"You can only lose what you have: short of the ante, you're all-in and can only win back your own stake from each player.",
			"Out of chips: buy back in or sit out the game. Best result after buy-ins wins after the last hand.",
		},
		Facts: []string{"Number of dice: 5", "Player count: 2 or more", "Object: win rounds"},
		Sheet: []string{
			"Each player antes one betting unit (usually a dollar, pound, euro, etc) into the pot and they then take it in turn throwing the dice.  The object of the game is to score the lowest total amount by adding up the spot values of all five set aside dice but counting 3s as zeroes.",
			"Each player has up to five rolls of the dice, but must set at least one aside after each throw.  Once a dice is set aside, it may not be rolled again.",
			"The best possible score after all the dice have been set aside is 0 (3-3-3-3-3), but should any player roll 6-6-6-6-6 (\"Shooting the Moon\"), they win instantly with no further play",
			"In the event of equally low scores, tied players ante an additional unit into the pot and play again.",
			"The winner takes the pot and rolls first in the next game.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newThrees(env, n, atoiDefault(opts["hands"], 5, 3, 5, 8), opts)
		},
	})
}

// threesScore adds the dice with 3s counting as zero.
func threesScore(d []int) int {
	t := 0
	for _, f := range d {
		if f != 3 {
			t += f
		}
	}
	return t
}

func isMoon(d []int) bool { return len(d) == threesDice && games.Counts(d)[6] == threesDice }

type threes struct {
	env     *games.Env
	seats   games.Seats
	hands   int
	hand    int
	starter int
	turnOrder
	turn   keepTurn
	kept   [][]int // this play, per seat; nil = not played yet
	scores []int   // this play, per seat; -1 = not played yet
	ct     chipTable
	replay int // 0 for a hand's first play, then 1, 2, ... for tie replays
	last   *roundResult
	winner int
}

func newThrees(env *games.Env, n, hands int, opts map[string]string) *threes {
	g := &threes{env: env, seats: games.NewSeats(n), hands: hands, ct: newChipTable(n, opts), winner: -1}
	g.kept, g.scores = make([][]int, n), intsOf(n, -1)
	g.startHand()
	return g
}

func (g *threes) startHand() {
	order := g.ct.playersFrom(g.starter, &g.seats)
	if len(order) < 2 {
		g.finish()
		return
	}
	g.hand++
	g.replay = 0
	g.starter = order[0]
	carried := g.ct.Pot.Total()
	g.ct.ante(order, 1)
	extra := ""
	if carried > 0 {
		extra = fmt.Sprintf(", including %d carried over", carried)
	}
	g.env.Log(games.LogInfo, "Hand %d of %d — everyone antes 1 (pot %d%s). %s rolls first", g.hand, g.hands, g.ct.Pot.Total(), extra, games.P(g.starter))
	g.play(order)
}

// play starts a play of the hand among seats, in that order.
func (g *threes) play(seats []int) {
	g.kept = make([][]int, g.seats.N)
	g.scores = intsOf(g.seats.N, -1)
	g.turn = newKeepTurn(threesDice)
	g.start(seats, &g.seats)
}

func (g *threes) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	if g.ct.rebuying() {
		if err := g.ct.decide(g.env, seat, a); err != nil {
			return err
		}
		if !g.ct.rebuying() {
			g.startHand()
		}
		return nil
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
		g.env.Log(games.LogRoll, "%s rolled %s (roll 1 of %d)", who, games.DiceText(g.turn.Roll), threesDice)
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
		g.env.Log(games.LogInfo, "%s kept %s (score %d so far)", who, games.DiceText(faces), threesScore(g.turn.Kept))
		g.turn.roll(g.env)
		g.env.Log(games.LogRoll, "%s rolled %s (roll %d of %d)", who, games.DiceText(g.turn.Roll), g.turn.Rolls, threesDice)
		return nil
	}
	return games.ErrUnknown
}

func (g *threes) finishTurn(faces []int) {
	seat := g.cur()
	hand := g.turn.Kept
	g.kept[seat] = hand
	g.scores[seat] = threesScore(hand)
	if isMoon(hand) {
		g.env.Log(games.LogGood, "%s kept %s — 6-6-6-6-6: SHOOTING THE MOON!", games.P(seat), games.DiceText(faces))
		g.winHand(seat, true)
		return
	}
	g.env.Log(games.LogInfo, "%s kept %s — final hand %s scores %d", games.P(seat), games.DiceText(faces), games.DiceText(hand), g.scores[seat])
	g.advance()
}

func (g *threes) advance() {
	g.turn = newKeepTurn(threesDice)
	g.next(&g.seats)
	if g.done() {
		g.endPlay()
	}
}

func (g *threes) endPlay() {
	players := activeIn(g.Order, &g.seats)
	if len(players) == 0 {
		// Everyone in a tie replay left: the pot carries over.
		g.env.Log(games.LogInfo, "Nobody is left in the replay — the pot of %d carries over", g.ct.Pot.Total())
		g.last = g.result(-1)
		g.nextHand()
		return
	}
	best, low := []int{}, 0
	for _, s := range players {
		sc := g.scores[s]
		switch {
		case sc < 0:
		case len(best) == 0 || sc < low:
			best, low = []int{s}, sc
		case sc == low:
			best = append(best, s)
		}
	}
	if len(best) == 0 { // only possible if nobody left has played
		best = players
	}
	if len(best) == 1 {
		g.winHand(best[0], false)
		return
	}
	g.ct.ante(best, 1)
	g.replay++
	g.last = g.result(-1)
	g.env.Log(games.LogInfo, "%s tie on %d — they ante again (pot %d) and replay", nameList(best), low, g.ct.Pot.Total())
	g.play(best)
}

func (g *threes) result(w int) *roundResult {
	return &roundResult{Round: g.hand, Hands: copyHands(g.kept), Scores: games.Ints(g.scores), Winner: w}
}

func (g *threes) winHand(w int, moon bool) {
	won := g.ct.award(g.env, w)
	g.last = g.result(w)
	if moon {
		g.env.Log(games.LogGood, "%s wins hand %d instantly and takes the pot of %d", games.P(w), g.hand, won)
	} else if g.scores[w] >= 0 {
		g.env.Log(games.LogGood, "%s wins hand %d with %d and takes the pot of %d", games.P(w), g.hand, g.scores[w], won)
	} else {
		g.env.Log(games.LogGood, "%s is the last one left in the replay and takes the pot of %d", games.P(w), won)
	}
	g.starter = w
	g.nextHand()
}

func (g *threes) nextHand() {
	g.turn = newKeepTurn(threesDice)
	g.Order, g.Pos = nil, 0
	if g.hand >= g.hands {
		g.finish()
		return
	}
	if g.ct.markBusted(g.env, &g.seats) {
		return // wait for the rebuy decisions
	}
	g.startHand()
}

func (g *threes) finish() {
	g.Order, g.Pos = nil, 0
	g.winner = g.ct.finish(g.env, &g.seats)
}

func signed(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprint(n)
}

func (g *threes) Legal(seat int) []games.Action {
	if g.winner >= 0 {
		return nil
	}
	if g.ct.rebuying() {
		return g.ct.legal(seat)
	}
	if seat != g.cur() {
		return nil
	}
	return g.turn.legal()
}

func (g *threes) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.winner >= 0 {
		return
	}
	if g.ct.rebuying() {
		g.ct.Rebuy[seat] = false
		if !g.ct.rebuying() {
			g.startHand()
		}
		return
	}
	if g.replay > 0 {
		if left := activeIn(g.Order, &g.seats); len(left) <= 1 {
			if len(left) == 1 {
				g.winHand(left[0], false)
			} else {
				g.endPlay()
			}
			return
		}
	}
	if seat == g.cur() {
		g.advance()
	}
}

type threesView struct {
	Hand      int     `json:"hand"`
	Hands     int     `json:"hands"`
	Replay    int     `json:"replay"`
	Playing   []int   `json:"playing"` // seats in this play, in turn order
	Starter   int     `json:"starter"`
	Current   int     `json:"current"`
	Kept      []int   `json:"kept"`
	Roll      []int   `json:"roll"`
	Rolls     int     `json:"rolls"`
	MaxRoll   int     `json:"maxRolls"`
	SeatHands [][]int `json:"seatHands"`
	Scores    []int   `json:"scores"`
	chipView
	Pot      int          `json:"pot"`
	Rebuying bool         `json:"rebuying"`
	Last     *roundResult `json:"last"`
	Over     bool         `json:"over"`
}

func (g *threes) View(seat int) interface{} {
	return threesView{
		Hand: g.hand, Hands: g.hands, Replay: g.replay, Playing: activeIn(g.Order, &g.seats), Starter: g.starter,
		Current: g.cur(), Kept: games.Ints(g.turn.Kept), Roll: games.Ints(g.turn.Roll), Rolls: g.turn.Rolls,
		MaxRoll: threesDice, SeatHands: copyHands(g.kept), Scores: games.Ints(g.scores), chipView: g.ct.view(),
		Pot: g.ct.Pot.Total(), Rebuying: g.ct.rebuying(), Last: g.last, Over: g.winner >= 0,
	}
}

func (g *threes) Prompt(seat int) string {
	if g.winner >= 0 {
		return fmt.Sprintf("%s wins, %s net!", games.P(g.winner), signed(g.ct.St.Net(g.winner)))
	}
	if g.ct.rebuying() {
		return g.ct.rebuyPrompt(seat)
	}
	cur := g.cur()
	if seat != cur {
		if !g.turn.started() {
			return fmt.Sprintf("Waiting for %s to roll (pot %d).", games.P(cur), g.ct.Pot.Total())
		}
		return fmt.Sprintf("%s has %d so far (low wins).", games.P(cur), threesScore(g.turn.Kept))
	}
	if !g.turn.started() {
		if g.replay > 0 {
			return "Tie replay — roll five dice. Lowest score takes the pot."
		}
		return "Roll five dice to start your turn. Lowest score wins."
	}
	return fmt.Sprintf("Score %d so far (3s count 0). Tap at least one die to keep.", threesScore(g.turn.Kept))
}

func (g *threes) Waiting() []int {
	if g.winner >= 0 {
		return nil
	}
	if g.ct.rebuying() {
		return g.ct.waiting()
	}
	if g.cur() < 0 {
		return nil
	}
	return []int{g.cur()}
}

func (g *threes) Badges() []string {
	out := make([]string, g.seats.N)
	for i := range out {
		out[i] = g.ct.badge(i)
		if g.kept[i] != nil && g.winner < 0 {
			out[i] += fmt.Sprintf(" · %d", g.scores[i])
		}
	}
	return out
}

func (g *threes) Over() bool { return g.winner >= 0 }

func (g *threes) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
