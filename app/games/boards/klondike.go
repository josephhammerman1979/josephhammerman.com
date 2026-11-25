package boards

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Klondike (the sheet's "Banker"): bet that your five-dice poker hand beats
// the banker's.
//
//   Classic:  the banker rolls five dice to set the point; each player then
//             rolls five dice and wins their bet with a strictly better hand
//             (ties go to the banker).
//   Goldrush: each player rolls five dice and may re-roll up to three of
//             them once; the banker rolls last.  Seeing the banker's first
//             roll, each player simultaneously stays, doubles their bet, or
//             folds (paying half their bet, rounded up).  The banker then
//             may re-roll up to three dice "against the table", and the
//             remaining bets settle.
//
// The bank rotates from seat 0 to the left, once or twice round the table.
//
// Actions: {type:"wager", amount}, {type:"roll"}, {type:"stand"},
// {type:"reroll", dice:[indices]} (goldrush, 1-3 dice),
// {type:"stay"}, {type:"double"}, {type:"fold"} (goldrush decision).

func init() {
	games.Register(&games.Def{
		ID: "klondike", Name: "Klondike", Order: 110,
		Summary:    "Bet your chips that your five-dice poker hand beats the banker's.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Everyone takes a turn as banker. Each round the other players bet 1–5 chips.",
			"Classic: the banker rolls five dice to set the point. Then each player rolls five dice — beat the banker's poker hand to win your bet; a worse hand or a tie pays the banker.",
			"Gold Rush: each player rolls and may re-roll up to three dice once. The banker rolls last; seeing that roll, everyone chooses to stay, double their bet, or fold (lose half the bet, rounded up). Then the banker may re-roll up to three dice and the remaining bets settle.",
			"Hands, best first: five of a kind, four of a kind, high straight (2–6), low straight (1–5), full house, three of a kind, two pair, pair, highest die.",
			"Nobody goes below zero: you can only lose what you have, and a banker who runs dry pays winners in turn until the money's gone.",
			"Hit zero and you're busted: sit out the rest of the round, then buy back in or sit out the game. Best result after buy-ins wins.",
		},
		Options: []games.Option{
			{Key: "variant", Label: "Variant", Choices: []games.Choice{
				choice("classic", "Classic"), choice("goldrush", "Gold Rush (re-roll up to 3)")}},
			{Key: "chips", Label: "Starting chips", Choices: []games.Choice{
				choice("20", "20 chips"), choice("50", "50 chips")}},
			{Key: "banks", Label: "Turns as banker", Choices: []games.Choice{
				choice("1", "Once each"), choice("2", "Twice each")}},
		},
		Facts: []string{
			"Number of dice: 5 per player", "Number of cups: 0",
			"Number of tokens: Poker chips depending on buy-in amount",
			"Player count: 2 to 6", "Object: win money",
		},
		Sheet: []string{
			"One player is banker. Each player places a wager. The banker sets the point by rolling all five dice as a dice poker hand. Each player rolls their own dice and hopes to obtain a better result.",
			"Goldrush variant:",
			"Similar to five card draw. Each player places a stake. Each player roles with the banker rolling last. Each player chooses up to three to re-roll. The banker then chooses up to three to reroll against the table.",
			"Rotate the banker.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			chips, _ := strconv.Atoi(opts["chips"])
			banks, _ := strconv.Atoi(opts["banks"])
			return newKlondike(env, n, opts["variant"] == "goldrush", chips, banks)
		},
	})
}

const (
	bankerMaxBet    = 5
	bankerMaxReroll = 3
)

// Phases.
const (
	bkWager  = "wager"  // non-bankers choosing bets, simultaneously
	bkPoint  = "point"  // classic: banker rolls the point
	bkPlay   = "play"   // players roll in turn
	bkBank   = "bank"   // goldrush: banker rolls last, then (after bkDecide) may re-roll
	bkDecide = "decide" // goldrush: players stay / double / fold, simultaneously
	bkRebuy  = "rebuy"  // busted players choose to buy back in or leave
	bkOver   = "over"
)

type klondike struct {
	env      *games.Env
	seats    games.Seats
	gold     bool
	st       games.Stacks
	chips    []int // st.Chips (same slice)
	rotation []int // banker for each round, in order
	bankTurn int   // index into rotation
	banker   int
	phase    string

	order []int // this round's non-bankers, in turn order
	turn  int   // index into order (play phase)

	wager    []int // 0 = not yet chosen
	dice     [][]int
	hand     []games.Hand
	rolled   []bool
	rerolled []bool
	result   []int    // +1 won, -1 lost, 0 not settled
	choice   []string // goldrush decision: "", "stay", "double", "fold"

	last      *bankerSummary
	winners   []int
	rebuyWait []bool // busted players still choosing to rebuy or leave
}

// bankerSummary is the outcome of the last settled round, shown while the
// next round's bets go down.
type bankerSummary struct {
	Round    int             `json:"round"`
	Banker   int             `json:"banker"`
	BankDice []int           `json:"bankDice"`
	BankHand string          `json:"bankHand"`
	Rows     []bankerLastRow `json:"rows"`
	Net      int             `json:"net"`   // banker's gain (negative = loss)
	Short    int             `json:"short"` // winnings the banker couldn't pay
}

type bankerLastRow struct {
	Seat   int    `json:"seat"`
	Dice   []int  `json:"dice"`
	Hand   string `json:"hand"`
	Wager  int    `json:"wager"`
	Result int    `json:"result"`
	Paid   int    `json:"paid"`   // chips that actually changed hands
	Short  bool   `json:"short"`  // a winner the banker couldn't pay in full
	Choice string `json:"choice"` // goldrush: stay / double / fold
}

func newKlondike(env *games.Env, n int, gold bool, startChips, banks int) *klondike {
	if startChips <= 0 {
		startChips = 20
	}
	if banks <= 0 {
		banks = 1
	}
	g := &klondike{env: env, seats: games.NewSeats(n), gold: gold, st: games.NewStacks(n, startChips), banker: -1}
	g.chips = g.st.Chips
	g.rebuyWait = make([]bool, n)
	for b := 0; b < banks; b++ {
		for s := 0; s < n; s++ {
			g.rotation = append(g.rotation, s)
		}
	}
	g.wager, g.dice, g.hand = make([]int, n), make([][]int, n), make([]games.Hand, n)
	g.rolled, g.rerolled, g.result = make([]bool, n), make([]bool, n), make([]int, n)
	g.choice = make([]string, n)
	g.bankTurn = -1
	g.nextRound()
	return g
}

func (g *klondike) cur() int {
	if g.phase == bkPlay && g.turn < len(g.order) {
		return g.order[g.turn]
	}
	if g.phase == bkPoint || g.phase == bkBank {
		return g.banker
	}
	return -1
}

func (g *klondike) nextRound() {
	g.bankTurn++
	for g.bankTurn < len(g.rotation) && !g.canPlay(g.rotation[g.bankTurn]) {
		g.bankTurn++ // a busted or departed banker's turn is skipped
	}
	if g.bankTurn >= len(g.rotation) || g.playable() < 2 {
		g.finish()
		return
	}
	n := g.seats.N
	g.banker = g.rotation[g.bankTurn]
	g.phase = bkWager
	g.wager = make([]int, n)
	g.dice = make([][]int, n)
	g.hand = make([]games.Hand, n)
	g.rolled = make([]bool, n)
	g.rerolled = make([]bool, n)
	g.result = make([]int, n)
	g.choice = make([]string, n)
	g.order = nil
	for _, s := range g.seats.OrderFrom(g.banker) {
		if s != g.banker && g.canPlay(s) {
			g.order = append(g.order, s)
		}
	}
	g.turn = 0
	g.env.Log(games.LogInfo, "Klondike round %d of %d: %s is the banker. Place your bets!", g.bankTurn+1, len(g.rotation), games.P(g.banker))
}

func (g *klondike) finish() {
	g.phase = bkOver
	best := []int{}
	pool := g.seats.Active()
	if len(pool) == 0 {
		for i := 0; i < g.seats.N; i++ {
			pool = append(pool, i)
		}
	}
	for _, s := range pool {
		switch {
		case len(best) == 0 || g.st.Net(s) > g.st.Net(best[0]):
			best = []int{s}
		case g.st.Net(s) == g.st.Net(best[0]):
			best = append(best, s)
		}
	}
	g.winners = best
	if len(best) == 1 {
		g.env.Log(games.LogWin, "%s wins, %s net!", games.P(best[0]), signed(g.st.Net(best[0])))
	} else {
		g.env.Log(games.LogWin, "%s share the win at %s net each!", seatList(best), signed(g.st.Net(best[0])))
	}
}

// canPlay reports whether seat can take part in a round: still at the
// table, with chips, and not sitting out after busting.
func (g *klondike) canPlay(seat int) bool { return g.seats.In(seat) && g.st.Funded(seat) }

func (g *klondike) playable() int {
	n := 0
	for s := 0; s < g.seats.N; s++ {
		if g.canPlay(s) {
			n++
		}
	}
	return n
}

func signed(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	return fmt.Sprint(n)
}

func (g *klondike) maxBet(seat int) int {
	m := bankerMaxBet
	if g.chips[seat] < m {
		m = g.chips[seat]
	}
	return m
}

func (g *klondike) needsWager(seat int) bool {
	return g.phase == bkWager && seat != g.banker && g.seats.In(seat) && g.wager[seat] == 0
}

func (g *klondike) Act(seat int, a games.Action) error {
	if g.phase == bkOver {
		return games.ErrGameOver
	}
	switch a.Type {
	case "rebuy", "leave":
		if !g.needsRebuy(seat) {
			return games.ErrUnknown
		}
		g.rebuyWait[seat] = false
		if a.Type == "rebuy" {
			g.st.Rebuy(seat)
			g.env.Log(games.LogInfo, "%s bought back in for %d.", games.P(seat), g.st.Start)
		} else {
			g.st.Gone[seat] = true
			g.env.Log(games.LogInfo, "%s is sitting out the rest of the game.", games.P(seat))
		}
		g.afterRebuys()
		return nil
	case "wager":
		if g.phase != bkWager {
			return games.Errorf("Bets are already down.")
		}
		if seat == g.banker {
			return games.Errorf("The banker doesn't bet.")
		}
		if !g.needsWager(seat) {
			return games.Errorf("You've already placed your bet.")
		}
		if a.Amount < 1 || a.Amount > g.maxBet(seat) {
			return games.Errorf("Bet between 1 and %d chips.", g.maxBet(seat))
		}
		g.wager[seat] = a.Amount
		g.env.Log(games.LogInfo, "%s placed a bet.", games.P(seat))
		g.afterWager()
		return nil
	case "roll":
		if seat != g.cur() {
			return games.ErrNotYourTurn
		}
		switch g.phase {
		case bkPoint:
			g.dice[g.banker] = g.env.Roll(5)
			g.hand[g.banker] = games.EvalHand(g.dice[g.banker])
			g.rolled[g.banker] = true
			g.env.Log(games.LogRoll, "%s (banker) set the point: %s — %s", games.P(seat), games.DiceText(g.dice[seat]), g.hand[seat].Name)
			g.phase = bkPlay
			g.turn = 0
			g.skipOut()
		case bkPlay, bkBank:
			if g.rolled[seat] {
				return games.Errorf("You've already rolled — stand or re-roll.")
			}
			g.dice[seat] = g.env.Roll(5)
			g.hand[seat] = games.EvalHand(g.dice[seat])
			g.rolled[seat] = true
			if g.phase == bkBank {
				g.env.Log(games.LogRoll, "%s (banker) rolled %s — %s. Stay, double or fold?", games.P(seat), games.DiceText(g.dice[seat]), g.hand[seat].Name)
				g.phase = bkDecide
				return nil
			}
			if g.gold {
				g.env.Log(games.LogRoll, "%s rolled %s — %s", games.P(seat), games.DiceText(g.dice[seat]), g.hand[seat].Name)
				return nil
			}
			bank := g.hand[g.banker]
			if games.CompareHands(g.hand[seat], bank) > 0 {
				g.result[seat] = 1
				g.env.Log(games.LogGood, "%s rolled %s — %s beats %s: wins %s",
					games.P(seat), games.DiceText(g.dice[seat]), g.hand[seat].Name, bank.Name, plural(g.wager[seat], "chip", "chips"))
			} else {
				g.result[seat] = -1
				g.env.Log(games.LogBust, "%s rolled %s — %s doesn't beat %s: loses %s",
					games.P(seat), games.DiceText(g.dice[seat]), g.hand[seat].Name, bank.Name, plural(g.wager[seat], "chip", "chips"))
			}
			g.nextTurn()
		default:
			return games.ErrUnknown
		}
		return nil
	case "stand", "reroll":
		if !g.gold || (g.phase != bkPlay && g.phase != bkBank) {
			if g.phase == bkDecide {
				return games.Errorf("Waiting for everyone to stay, double or fold.")
			}
			return games.ErrUnknown
		}
		if seat != g.cur() {
			return games.ErrNotYourTurn
		}
		if !g.rolled[seat] {
			return games.Errorf("Roll first.")
		}
		who := games.P(seat)
		if seat == g.banker {
			who += " (banker)"
		}
		if a.Type == "stand" {
			g.env.Log(games.LogInfo, "%s stands on %s.", who, g.hand[seat].Name)
		} else {
			idx, err := games.UniqueIndices(a.Dice, 5)
			if err != nil {
				return err
			}
			if len(idx) < 1 || len(idx) > bankerMaxReroll {
				return games.Errorf("Pick 1 to %d dice to re-roll.", bankerMaxReroll)
			}
			fresh := g.env.Roll(len(idx))
			d := games.Ints(g.dice[seat])
			for i, k := range idx {
				d[k] = fresh[i]
			}
			g.dice[seat] = d
			g.hand[seat] = games.EvalHand(d)
			g.rerolled[seat] = true
			g.env.Log(games.LogRoll, "%s re-rolled %s: %s — %s", who, plural(len(idx), "die", "dice"), games.DiceText(d), g.hand[seat].Name)
		}
		if g.phase == bkBank {
			g.settle()
		} else {
			g.nextTurn()
		}
		return nil
	case "stay", "double", "fold":
		if g.phase != bkDecide {
			return games.ErrUnknown
		}
		if !g.needsDecision(seat) {
			if seat == g.banker {
				return games.Errorf("The banker doesn't choose.")
			}
			return games.Errorf("You've already decided.")
		}
		if a.Type == "double" && !g.canDouble(seat) {
			return games.Errorf("You need %d chips to double.", 2*g.wager[seat])
		}
		g.choice[seat] = a.Type
		g.env.Log(games.LogInfo, "%s has decided.", games.P(seat))
		g.afterDecisions()
		return nil
	}
	return games.ErrUnknown
}

func (g *klondike) inRound(seat int) bool {
	return seat != g.banker && g.seats.In(seat) && containsInt(g.order, seat) && g.choice[seat] != "fold"
}

func (g *klondike) needsDecision(seat int) bool {
	return g.phase == bkDecide && g.inRound(seat) && g.choice[seat] == ""
}

func (g *klondike) canDouble(seat int) bool { return g.chips[seat] >= 2*g.wager[seat] }

func foldCost(w int) int { return (w + 1) / 2 }

// afterDecisions applies everyone's stay / double / fold once all are in,
// then hands the dice back to the banker (or settles if everyone folded).
func (g *klondike) afterDecisions() {
	if g.phase != bkDecide {
		return
	}
	for _, s := range g.order {
		if g.needsDecision(s) {
			return
		}
	}
	b := g.banker
	staying := 0
	for _, s := range g.order {
		if !g.seats.In(s) {
			continue
		}
		w := g.wager[s]
		switch g.choice[s] {
		case "double":
			g.wager[s] = 2 * w
			staying++
			g.env.Log(games.LogInfo, "%s doubles to %d.", games.P(s), g.wager[s])
		case "fold":
			p := foldCost(w)
			g.wager[s] = p // what they lost
			g.chips[s] -= p
			g.chips[b] += p
			g.result[s] = -1
			g.env.Log(games.LogBust, "%s folds and pays the banker %d.", games.P(s), p)
		default:
			staying++
			g.env.Log(games.LogInfo, "%s stays at %d.", games.P(s), w)
		}
	}
	if staying == 0 {
		g.settle()
		return
	}
	g.phase = bkBank
}

func (g *klondike) afterWager() {
	if g.phase != bkWager {
		return
	}
	parts := []string{}
	for _, s := range g.order {
		if !g.seats.In(s) {
			continue
		}
		if g.wager[s] == 0 {
			return
		}
		parts = append(parts, fmt.Sprintf("%s %d", games.P(s), g.wager[s]))
	}
	g.env.Log(games.LogInfo, "Bets are down: %s.", strings.Join(parts, ", "))
	if g.gold {
		g.phase = bkPlay
		g.turn = 0
		g.skipOut()
	} else {
		g.phase = bkPoint
	}
}

// skipOut moves the play turn past removed players, ending the players'
// turns when everyone has gone.
func (g *klondike) skipOut() {
	for g.turn < len(g.order) && !g.seats.In(g.order[g.turn]) {
		g.turn++
	}
	if g.turn >= len(g.order) {
		if g.gold {
			g.phase = bkBank
		} else {
			g.settle()
		}
	}
}

func (g *klondike) nextTurn() {
	g.turn++
	g.skipOut()
}

// settle pays out the round.  Losers pay the banker first; then the banker
// pays winners in turn order from their left, only from what they have —
// nobody goes negative, so a winner late in the order may be paid short.
func (g *klondike) settle() {
	b := g.banker
	bank := g.hand[b]
	sum := &bankerSummary{
		Round: g.bankTurn + 1, Banker: b, BankDice: games.Ints(g.dice[b]), BankHand: bank.Name,
		Rows: []bankerLastRow{},
	}
	rows := map[int]*bankerLastRow{}
	played := []int{}
	for _, s := range g.order {
		if !g.seats.In(s) || !g.rolled[s] {
			continue
		}
		played = append(played, s)
		row := &bankerLastRow{Seat: s, Dice: games.Ints(g.dice[s]), Hand: g.hand[s].Name, Wager: g.wager[s], Choice: g.choice[s]}
		rows[s] = row
		w := g.wager[s]
		switch {
		case g.choice[s] == "fold":
			row.Result, row.Paid = -1, w // already paid
			sum.Net += w
		case games.CompareHands(g.hand[s], bank) <= 0:
			paid, _ := g.st.Pay(s, w)
			g.chips[b] += paid
			g.result[s], row.Result, row.Paid = -1, -1, paid
			sum.Net += paid
			if g.gold {
				g.env.Log(games.LogBust, "%s's %s doesn't beat the banker's %s: −%d", games.P(s), g.hand[s].Name, bank.Name, paid)
			}
		}
	}
	for _, s := range played {
		row := rows[s]
		if row.Result < 0 {
			continue
		}
		w := g.wager[s]
		paid, short := g.st.Pay(b, w)
		g.chips[s] += paid
		g.result[s], row.Result, row.Paid = 1, 1, paid
		sum.Net -= paid
		if short {
			row.Short = true
			sum.Short += w - paid
			g.env.Log(games.LogBust, "%s's %s beats the banker's %s, but the banker could only pay %d of %d.", games.P(s), g.hand[s].Name, bank.Name, paid, w)
		} else if g.gold {
			g.env.Log(games.LogGood, "%s's %s beats the banker's %s: +%d", games.P(s), g.hand[s].Name, bank.Name, paid)
		}
	}
	for _, s := range played {
		sum.Rows = append(sum.Rows, *rows[s])
	}
	switch {
	case sum.Short > 0:
		g.env.Log(games.LogBust, "The bank (%s) ran dry — %d in winnings went unpaid.", games.P(b), sum.Short)
	case sum.Net > 0:
		g.env.Log(games.LogGood, "The bank (%s) won %d this round.", games.P(b), sum.Net)
	case sum.Net < 0:
		g.env.Log(games.LogBust, "The bank (%s) paid out %d this round.", games.P(b), -sum.Net)
	default:
		g.env.Log(games.LogInfo, "The bank (%s) broke even this round.", games.P(b))
	}
	g.last = sum
	g.endRound()
}

// endRound offers busted players a rebuy, or starts the next round.
func (g *klondike) endRound() {
	g.rebuyWait = make([]bool, g.seats.N)
	if g.bankTurn+1 >= len(g.rotation) {
		g.nextRound() // that was the last round
		return
	}
	any := false
	for _, s := range g.seats.Active() {
		if g.st.Busted(s) {
			g.rebuyWait[s] = true
			any = true
			g.env.Log(games.LogBust, "%s is busted!", games.P(s))
		}
	}
	if any {
		g.phase = bkRebuy
		return
	}
	g.nextRound()
}

func (g *klondike) needsRebuy(seat int) bool {
	return g.phase == bkRebuy && g.seats.In(seat) && g.rebuyWait[seat]
}

func (g *klondike) afterRebuys() {
	if g.phase != bkRebuy {
		return
	}
	for s := range g.rebuyWait {
		if g.needsRebuy(s) {
			return
		}
	}
	g.nextRound()
}

func (g *klondike) Legal(seat int) []games.Action {
	if g.phase == bkOver {
		return nil
	}
	if g.needsRebuy(seat) {
		return append([]games.Action{}, games.RebuyActions...)
	}
	if g.needsWager(seat) {
		out := []games.Action{}
		for a := 1; a <= g.maxBet(seat); a++ {
			out = append(out, games.Action{Type: "wager", Amount: a})
		}
		return out
	}
	if g.needsDecision(seat) {
		out := []games.Action{{Type: "stay"}, {Type: "fold"}}
		if g.canDouble(seat) {
			out = append(out, games.Action{Type: "double"})
		}
		return out
	}
	if seat != g.cur() {
		return nil
	}
	if g.phase == bkPoint || !g.rolled[seat] {
		return []games.Action{{Type: "roll"}}
	}
	return []games.Action{
		{Type: "stand"},
		{Type: "reroll", Dice: []int{0}},
		{Type: "reroll", Dice: []int{1, 3}},
		{Type: "reroll", Dice: []int{2, 3, 4}},
	}
}

func (g *klondike) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.phase == bkOver {
		return
	}
	if g.seats.ActiveCount() < 2 {
		g.finish()
		return
	}
	if g.phase == bkRebuy {
		g.afterRebuys()
		return
	}
	if seat == g.banker {
		g.env.Log(games.LogInfo, "The banker left — this round is called off and bets are returned.")
		for _, s := range g.order {
			if g.choice[s] == "fold" && g.phase != bkWager {
				g.chips[s] += g.wager[s] // refund the fold
			}
		}
		g.nextRound()
		return
	}
	switch g.phase {
	case bkWager:
		g.afterWager()
	case bkDecide:
		g.afterDecisions()
	case bkPlay:
		if g.turn < len(g.order) && g.order[g.turn] == seat {
			g.nextTurn()
		}
	}
}

type bankerSeatView struct {
	Chips    int    `json:"chips"`
	BuyIns   int    `json:"buyIns"`
	Net      int    `json:"net"`
	Busted   bool   `json:"busted"` // on zero chips: sitting out this round
	Gone     bool   `json:"gone"`   // chose to sit out the rest of the game
	Rebuying bool   `json:"rebuying"`
	Out      bool   `json:"out"`
	Wager    int    `json:"wager"`   // -1 while hidden
	Wagered  bool   `json:"wagered"` // has chosen a bet this round
	Dice     []int  `json:"dice"`
	Hand     string `json:"hand"`
	Rolled   bool   `json:"rolled"`
	Rerolled bool   `json:"rerolled"`
	Result   int    `json:"result"`
	MaxBet   int    `json:"maxBet"`
	// Goldrush decision.  Others' choices stay hidden ("") until everyone
	// has decided; Decided shows who still has to.
	Choice    string `json:"choice"`
	Decided   bool   `json:"decided"`
	InRound   bool   `json:"inRound"`
	CanDouble bool   `json:"canDouble"`
	FoldCost  int    `json:"foldCost"`
}

type bankerView struct {
	Variant   string           `json:"variant"`
	Phase     string           `json:"phase"`
	Banker    int              `json:"banker"`
	Current   int              `json:"current"`
	Round     int              `json:"round"`
	Rounds    int              `json:"rounds"`
	Order     []int            `json:"order"`
	Seats     []bankerSeatView `json:"seats"`
	MaxReroll int              `json:"maxReroll"`
	Start     int              `json:"start"`
	Last      *bankerSummary   `json:"last"`
}

func (g *klondike) View(seat int) interface{} {
	v := bankerView{
		Variant: "classic", Phase: g.phase, Banker: g.banker, Current: g.cur(),
		Round: g.bankTurn + 1, Rounds: len(g.rotation), Order: games.Ints(g.order),
		MaxReroll: bankerMaxReroll, Start: g.st.Start, Last: g.last,
	}
	if g.gold {
		v.Variant = "goldrush"
	}
	if v.Round > v.Rounds {
		v.Round = v.Rounds
	}
	for s := 0; s < g.seats.N; s++ {
		sv := bankerSeatView{Chips: g.chips[s], BuyIns: g.st.BuyIns[s], Net: g.st.Net(s),
			Busted: g.st.Busted(s), Gone: g.st.Gone[s], Rebuying: g.needsRebuy(s),
			Out: g.seats.Out[s], Wager: g.wager[s], Wagered: g.wager[s] > 0, MaxBet: g.maxBet(s)}
		if g.phase == bkOver {
			sv.Wager = 0
		} else {
			// Others' bets stay hidden until everyone's bet is down.
			if g.phase == bkWager && s != seat {
				sv.Wager = -1
			}
			sv.Dice = games.Ints(g.dice[s])
			sv.Rolled = g.rolled[s]
			sv.Rerolled = g.rerolled[s]
			sv.Result = g.result[s]
			sv.Choice = g.choice[s]
			sv.Decided = g.choice[s] != ""
			sv.InRound = g.inRound(s)
			if g.phase == bkDecide && s != seat {
				sv.Choice = ""
			}
			if s != g.banker && g.wager[s] > 0 {
				sv.CanDouble = g.canDouble(s)
				sv.FoldCost = foldCost(g.wager[s])
			}
			if g.rolled[s] {
				sv.Hand = g.hand[s].Name
			}
		}
		v.Seats = append(v.Seats, sv)
	}
	return v
}

func (g *klondike) Prompt(seat int) string {
	if g.phase == bkOver {
		if len(g.winners) == 1 {
			return fmt.Sprintf("%s wins, %s net!", games.P(g.winners[0]), signed(g.st.Net(g.winners[0])))
		}
		return fmt.Sprintf("%s share the win!", seatList(g.winners))
	}
	b := games.P(g.banker)
	switch g.phase {
	case bkRebuy:
		if g.needsRebuy(seat) {
			return fmt.Sprintf("You're busted. Buy back in for %d chips, or sit out the rest of the game?", g.st.Start)
		}
		return fmt.Sprintf("Waiting for %s to buy back in or sit out.", seatList(g.Waiting()))
	case bkWager:
		if g.needsWager(seat) {
			return fmt.Sprintf("Bet 1–%d chips against the banker, %s.", g.maxBet(seat), b)
		}
		waiting := []int{}
		for _, s := range g.order {
			if g.needsWager(s) {
				waiting = append(waiting, s)
			}
		}
		if seat == g.banker {
			return fmt.Sprintf("You're the banker. Waiting for bets from %s.", seatList(waiting))
		}
		return fmt.Sprintf("Waiting for bets from %s.", seatList(waiting))
	case bkPoint:
		if seat == g.banker {
			return "You're the banker — roll five dice to set the point."
		}
		return fmt.Sprintf("Waiting for the banker, %s, to set the point.", b)
	case bkPlay:
		c := g.cur()
		if seat == c {
			if !g.rolled[c] {
				if g.gold {
					return "Roll your five dice."
				}
				return fmt.Sprintf("Roll five dice — beat %s to win your bet.", g.hand[g.banker].Name)
			}
			return fmt.Sprintf("You have %s. Stand, or pick up to %d dice to re-roll once.", g.hand[c].Name, bankerMaxReroll)
		}
		return fmt.Sprintf("Waiting for %s to roll.", games.P(c))
	case bkBank:
		if seat == g.banker {
			if !g.rolled[seat] {
				return "You're the banker — roll last. Then the table chooses to stay, double or fold."
			}
			return fmt.Sprintf("You have %s. Stand, or re-roll up to %d dice against the table.", g.hand[seat].Name, bankerMaxReroll)
		}
		return fmt.Sprintf("Waiting for the banker, %s.", b)
	case bkDecide:
		if g.needsDecision(seat) {
			return fmt.Sprintf("The banker has %s. Stay, double to %d, or fold and lose %d?", g.hand[g.banker].Name, 2*g.wager[seat], foldCost(g.wager[seat]))
		}
		return fmt.Sprintf("Waiting for %s to stay, double or fold.", seatList(g.Waiting()))
	}
	return ""
}

func (g *klondike) Waiting() []int {
	if g.phase == bkOver {
		return nil
	}
	if g.phase == bkWager {
		out := []int{}
		for _, s := range g.order {
			if g.needsWager(s) {
				out = append(out, s)
			}
		}
		return out
	}
	if g.phase == bkDecide {
		out := []int{}
		for _, s := range g.order {
			if g.needsDecision(s) {
				out = append(out, s)
			}
		}
		return out
	}
	if g.phase == bkRebuy {
		out := []int{}
		for s := 0; s < g.seats.N; s++ {
			if g.needsRebuy(s) {
				out = append(out, s)
			}
		}
		return out
	}
	return []int{g.cur()}
}

func (g *klondike) Badges() []string {
	out := make([]string, g.seats.N)
	for s := range out {
		if g.seats.Out[s] {
			out[s] = "out"
			continue
		}
		if g.st.Gone[s] {
			out[s] = "sat out"
			continue
		}
		if g.st.Busted(s) {
			out[s] = "busted"
			continue
		}
		out[s] = plural(g.chips[s], "chip", "chips")
		if s == g.banker && g.phase != bkOver {
			out[s] += " · bank"
		}
	}
	return out
}

func (g *klondike) Over() bool { return g.phase == bkOver }

func (g *klondike) Winners() []int {
	if g.phase != bkOver {
		return nil
	}
	return games.Ints(g.winners)
}
