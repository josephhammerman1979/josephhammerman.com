package bluff

import (
	"fmt"
	"sort"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Of Mice and Men (Reiner Knizia, from Dice Games Properly Explained).
//
// Each round everyone rolls three dice under their cup and reads them as a
// three-digit number, highest die first (1 4 6 → 641).  The starting player
// opens with a penalty value.  Going clockwise, each player still in the
// round either runs with the mice (drops out, shows their dice, takes 3
// penalty points), joins the men (accepts the current value) or leads the
// men (raises it, so everyone else must answer again).  The round ends when
// one player is left — they take nothing and show nothing — or when
// everyone remaining has accepted the same value: then they all show, and
// the lowest number (every tied lowest) takes that many penalty points.
// When someone reaches the agreed limit, the fewest penalty points wins.
//
// Actions: {type:"open", count:v}, {type:"run"}, {type:"join"},
// {type:"lead", count:v}.

const (
	miceDice    = 3
	miceRunCost = 3
	miceMaxBid  = 10 // highest penalty value that can be named
)

func init() {
	games.Register(&games.Def{
		ID: "mice", Name: "Of Mice and Men", Order: 105,
		Summary:    "Three hidden dice make a number. Raise the penalty, stay in, or run — the lowest number pays.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Everyone rolls three hidden dice and reads them highest first: 1 4 6 makes 641.",
			"The starting player names a penalty (1 to 10). Then, in turn, each player still in either runs, joins or leads.",
			"Run with the mice: drop out, show your dice and take 3 penalty points.",
			"Join the men: accept the current penalty. Lead the men: raise it — everyone else has to answer again.",
			"If everyone else runs, the last player takes nothing. Otherwise, once all agree, everyone shows and the lowest number takes the penalty (ties all take it).",
			"When someone reaches the penalty limit, the fewest penalty points wins.",
		},
		Options: []games.Option{
			{Key: "limit", Label: "Play until", Choices: []games.Choice{
				{Value: "20", Label: "20 penalty points"},
				{Value: "10", Label: "10 penalty points"},
				{Value: "30", Label: "30 penalty points"},
			}},
		},
		Facts: []string{"Number of dice: 3 per player", "Number of cups: 1 per player", "Object: Fewest penalty points"},
		Sheet: []string{
			"Mice and Men (featured in Dice Games Properly Explained) is a bluffing and betting dice game where players try to avoid taking penalty points.",
			"Each player rolls their three dice under their cup and peeks privately. Arrange your three dice from highest to lowest to form a 3-digit number (highest = hundreds, middle = tens, lowest = units). For example, rolling 1, 4, and 6 makes 641. Higher numbers beat lower numbers.",
			"Opening Bid: A starting player picks a penalty point value greater than 0 (for example, 2 points).",
			"Clockwise Turns: Going clockwise, each active player chooses one of three actions. Run with the mice: You drop out of the round immediately, reveal your dice, and take 3 penalty points. Join the men: You stay in the round and accept the current penalty point value. Lead the men: You stay in and raise the stakes by announcing a higher penalty point value than the current one. Play continues around the table until everyone has either dropped out or agreed to the same penalty value.",
			"One player left: If everyone else runs with the mice, the last player standing wins the round automatically, takes no penalty points, and does not have to show their dice.",
			"Showdown: Once all remaining players have agreed to the same penalty value, they reveal their dice. The player (or tied players) with the lowest 3-digit number takes penalty points equal to that agreed-upon value.",
			"Players agree on a point limit before starting (or play a set number of rounds). When someone hits or passes the limit—or once the agreed rounds finish—the player with the fewest penalty points wins.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newMice(env, n, optInt(opts, "limit", 20))
		},
	})
}

// miceResult is the public outcome of the previous round, shown until the
// next round's opening bid.
type miceResult struct {
	Round     int     `json:"round"`
	Value     int     `json:"value"`
	Shown     [][]int `json:"shown"`     // per seat: dice shown (runners, showdown), nil if hidden
	Ran       []bool  `json:"ran"`       // per seat
	Losers    []int   `json:"losers"`    // took the round's penalty
	LastStand int     `json:"lastStand"` // seat that won by everyone running, or -1
}

type mice struct {
	env     *games.Env
	seats   games.Seats
	limit   int
	penalty []int

	round   int
	starter int
	cur     int
	dice    [][]int // this round's dice, per seat
	in      []bool  // still in this round
	ran     []bool
	agreed  []bool // has accepted the current value
	value   int    // 0 until opened
	leader  int    // who named the current value
	shown   [][]int
	last    *miceResult
	over    bool
}

func newMice(env *games.Env, n, limit int) *mice {
	g := &mice{env: env, seats: games.NewSeats(n), limit: limit, penalty: make([]int, n)}
	g.startRound(0)
	return g
}

// handValue reads dice highest first as a three-digit number.
func handValue(d []int) int {
	s := games.Ints(d)
	sort.Sort(sort.Reverse(sort.IntSlice(s)))
	v := 0
	for _, x := range s {
		v = v*10 + x
	}
	return v
}

func (g *mice) startRound(starter int) {
	g.round++
	g.starter = starter
	g.cur = starter
	g.value = 0
	g.leader = -1
	n := g.seats.N
	g.dice = make([][]int, n)
	g.in = make([]bool, n)
	g.ran = make([]bool, n)
	g.agreed = make([]bool, n)
	g.shown = make([][]int, n)
	for _, s := range g.seats.Active() {
		g.dice[s] = g.env.Roll(miceDice)
		g.in[s] = true
	}
	g.env.Log(games.LogInfo, "Round %d: everyone rolled under their cups. %s opens.", g.round, P(starter))
}

func (g *mice) inRound() []int {
	out := []int{}
	for _, s := range g.seats.OrderFrom(g.starter) {
		if g.in[s] {
			out = append(out, s)
		}
	}
	return out
}

func (g *mice) Act(seat int, a games.Action) error {
	if g.over {
		return games.ErrGameOver
	}
	if seat != g.cur {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "open":
		if g.value != 0 {
			return games.Errorf("The round is already open — run, join or lead.")
		}
		if a.Count < 1 || a.Count > miceMaxBid {
			return games.Errorf("Name a penalty from 1 to %d.", miceMaxBid)
		}
		g.value, g.leader = a.Count, seat
		g.agreed[seat] = true
		g.last = nil
		g.env.Log(games.LogInfo, "%s opened at %d penalty points.", P(seat), a.Count)
	case "join":
		if g.value == 0 {
			return games.Errorf("Open the round first.")
		}
		g.agreed[seat] = true
		g.env.Log(games.LogInfo, "%s joined the men at %d.", P(seat), g.value)
	case "lead":
		if g.value == 0 {
			return games.Errorf("Open the round first.")
		}
		if a.Count <= g.value || a.Count > miceMaxBid {
			return games.Errorf("Raise to between %d and %d.", g.value+1, miceMaxBid)
		}
		g.value, g.leader = a.Count, seat
		for i := range g.agreed {
			g.agreed[i] = false
		}
		g.agreed[seat] = true
		g.env.Log(games.LogInfo, "%s led the men: the penalty is now %d.", P(seat), a.Count)
	case "run":
		if g.value == 0 {
			return games.Errorf("Open the round first.")
		}
		g.in[seat] = false
		g.ran[seat] = true
		g.shown[seat] = games.Ints(g.dice[seat])
		g.penalty[seat] += miceRunCost
		g.env.Log(games.LogBust, "%s ran with the mice, showing %d, and took %d penalty points.",
			P(seat), handValue(g.dice[seat]), miceRunCost)
	default:
		return games.ErrUnknown
	}
	g.advance()
	return nil
}

// advance ends the round if it's settled, otherwise passes to the next
// player still in the round.
func (g *mice) advance() {
	left := g.inRound()
	if len(left) == 1 && g.value != 0 {
		g.env.Log(games.LogGood, "Everyone else ran — %s takes nothing and keeps their dice hidden.", P(left[0]))
		g.endRound(nil, left[0])
		return
	}
	if len(left) == 0 {
		g.endRound(nil, -1)
		return
	}
	all := g.value != 0
	for _, s := range left {
		if !g.agreed[s] {
			all = false
		}
	}
	if all {
		g.showdown(left)
		return
	}
	next := g.cur
	for i := 0; i < g.seats.N; i++ {
		next = g.seats.Next(next)
		if g.in[next] {
			break
		}
	}
	g.cur = next
}

func (g *mice) showdown(left []int) {
	low := -1
	for _, s := range left {
		g.shown[s] = games.Ints(g.dice[s])
		if v := handValue(g.dice[s]); low < 0 || v < low {
			low = v
		}
	}
	parts, losers := []string{}, []int{}
	for _, s := range left {
		v := handValue(g.dice[s])
		parts = append(parts, fmt.Sprintf("%s %d", P(s), v))
		if v == low {
			losers = append(losers, s)
		}
	}
	g.env.Log(games.LogRoll, "Showdown at %d: %s", g.value, joinParts(parts))
	for _, s := range losers {
		g.penalty[s] += g.value
		g.env.Log(games.LogBust, "%s had the lowest number and took %d penalty points.", P(s), g.value)
	}
	g.endRound(losers, -1)
}

func joinParts(p []string) string {
	out := ""
	for i, s := range p {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

func (g *mice) endRound(losers []int, lastStand int) {
	if losers == nil {
		losers = []int{}
	}
	g.last = &miceResult{
		Round: g.round, Value: g.value, Shown: g.shown, Ran: g.ran,
		Losers: losers, LastStand: lastStand,
	}
	for _, s := range g.seats.Active() {
		if g.penalty[s] >= g.limit {
			g.over = true
		}
	}
	if g.over {
		w := g.Winners()
		names := []string{}
		for _, s := range w {
			names = append(names, P(s))
		}
		g.env.Log(games.LogWin, "The limit of %d was reached. Fewest penalty points: %s with %d.",
			g.limit, joinParts(names), g.penalty[w[0]])
		return
	}
	last := g.last
	g.startRound(g.seats.Next(g.starter))
	g.last = last
}

func (g *mice) Legal(seat int) []games.Action {
	if g.over || seat != g.cur {
		return nil
	}
	if g.value == 0 {
		return []games.Action{{Type: "open", Count: 1}, {Type: "open", Count: 3}, {Type: "open", Count: miceMaxBid}}
	}
	out := []games.Action{{Type: "run"}, {Type: "join"}}
	if g.value < miceMaxBid {
		out = append(out, games.Action{Type: "lead", Count: g.value + 1})
	}
	return out
}

func (g *mice) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.over {
		return
	}
	wasCur := seat == g.cur
	g.in[seat] = false
	g.agreed[seat] = false
	if g.seats.ActiveCount() < 2 {
		return // the Table ends the game
	}
	if g.value == 0 {
		if wasCur {
			// The opener left before opening: the next player opens instead.
			g.starter = g.seats.Next(seat)
			g.cur = g.starter
		}
		return
	}
	if wasCur {
		g.advance()
		return
	}
	// Someone else left: the round may now be settled.
	left := g.inRound()
	all := len(left) > 0
	for _, s := range left {
		if !g.agreed[s] {
			all = false
		}
	}
	if len(left) <= 1 || all {
		g.advance()
	}
}

type miceView struct {
	Round    int         `json:"round"`
	Starter  int         `json:"starter"`
	Current  int         `json:"current"`
	Value    int         `json:"value"`
	Leader   int         `json:"leader"`
	MaxBid   int         `json:"maxBid"`
	RunCost  int         `json:"runCost"`
	Limit    int         `json:"limit"`
	Penalty  []int       `json:"penalty"`
	In       []bool      `json:"in"`
	Ran      []bool      `json:"ran"`
	Agreed   []bool      `json:"agreed"`
	Shown    [][]int     `json:"shown"`  // dice shown this round (runners)
	MyDice   []int       `json:"myDice"` // only the viewer's own, while hidden
	MyNumber int         `json:"myNumber"`
	Last     *miceResult `json:"last,omitempty"`
}

func (g *mice) View(seat int) interface{} {
	v := miceView{
		Round: g.round, Starter: g.starter, Current: g.cur, Value: g.value, Leader: g.leader,
		MaxBid: miceMaxBid, RunCost: miceRunCost, Limit: g.limit, Penalty: games.Ints(g.penalty),
		In: append([]bool{}, g.in...), Ran: append([]bool{}, g.ran...),
		Agreed: append([]bool{}, g.agreed...), Last: g.last,
	}
	v.Shown = make([][]int, len(g.shown))
	for i, d := range g.shown {
		v.Shown[i] = games.Ints(d)
	}
	if seat >= 0 && seat < len(g.dice) && g.dice[seat] != nil && !g.over {
		v.MyDice = games.Ints(g.dice[seat])
		v.MyNumber = handValue(g.dice[seat])
	}
	return v
}

func (g *mice) Prompt(seat int) string {
	if g.over {
		return "The penalty limit has been reached."
	}
	if seat != g.cur {
		if g.value == 0 {
			return fmt.Sprintf("Waiting for %s to open.", P(g.cur))
		}
		return fmt.Sprintf("Penalty is %d — waiting for %s.", g.value, P(g.cur))
	}
	if g.value == 0 {
		return "Name the penalty to open the round."
	}
	return fmt.Sprintf("Penalty is %d. Run (take %d), join, or lead higher.", g.value, miceRunCost)
}

func (g *mice) Waiting() []int {
	if g.over {
		return nil
	}
	return []int{g.cur}
}

func (g *mice) Badges() []string {
	out := make([]string, len(g.penalty))
	for i, p := range g.penalty {
		out[i] = fmt.Sprintf("%d pen", p)
	}
	return out
}

func (g *mice) Over() bool { return g.over }

func (g *mice) Winners() []int {
	if !g.over {
		return nil
	}
	return games.BestSeats(&g.seats, g.penalty, true)
}
