package bluff

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Perudo: everyone rolls five dice under their own cup and bids on how many
// of a face are on the whole table.  Raise, challenge, or call "spot on".
//
// Actions on your turn:
//   {type:"bid", count, face}   open or raise
//   {type:"challenge"}          the bid is too high
//   {type:"spoton"}             the bid is exactly right
//
// A raise is the same count with a higher face, or a higher count with any
// face.  With wild ones, ones count toward every face and can't be bid.

const perudoStartDice = 5

func init() {
	games.Register(&games.Def{
		ID: "perudo", Name: "Perudo", Order: 90,
		Summary:    "Everyone hides five dice. Bid on how many of a face are hidden under everyone's cups. Raise, challenge, or call it spot on.",
		MinPlayers: 2, MaxPlayers: 6,
		Rules: []string{
			"Everyone rolls five dice under their cup; you only see your own.",
			"Bids are about all the dice on the table, e.g. “three fours” means at least three 4s in total.",
			"On your turn raise the bid (same count with a higher face, or a higher count), challenge it, or call “spot on”.",
			"Challenge: all cups are lifted. If the bid was there, the challenger loses a die; if not, the bidder does.",
			"Spot on: if the count is exactly right you get a die back; if not, you lose one.",
			"Lose all your dice and you're out. Last player with dice wins.",
		},
		Options: []games.Option{{
			Key: "wild", Label: "Ones",
			Choices: []games.Choice{{Value: "no", Label: "Ones are normal"}, {Value: "yes", Label: "Ones are wild"}},
		}},
		Facts: []string{"Number of dice: 5 per player", "Number of cups: 1 per player", "Player count: 2 to 6", "Object: win the game"},
		Sheet: []string{
			"This game is similar to Liar’s Dice. It is known as “The King of Dice Games”. There is standard play and a number of variants.",
			"Each player receives five dice & a cup. All cups are rolled simultaneously. The first player to bid declares a dice face value and count of dice (e.g. “five fives”). The bid is on the cumulative total off all the dice when the cups are lifted. After the bid is declared, the player to the left of the bidder either accepts the bid, challenges the bid, or declares that the bid is “Spot On”.",
			"Accepting the bid: after accepting the accepting player must bid a higher dice face value (e.g. “five sixes”) or higher total number of cubes (e.g. “6 twos”) to the player to their left, who then has the same three options available to them.",
			"Challenging the bid: If the bid is challenged, all cups are lifted. If the declared number of faces and dice or greater are revealed, the challenger loses one of their dice. If less than the declared number of dice of the value are revealed, the bidder loses a die.",
			"If “Spot On” is declared, the “Spot On” caller gets a dice back if the declaration of the bidder is revealed to be exactly correct.",
			"A popular additional rule is to make ones wild.",
		},
		New: func(env *games.Env, n int, opts map[string]string) games.Game {
			return newPerudo(env, n, opts["wild"] == "yes")
		},
	})
}

type perudoBid struct {
	Count int    `json:"count"`
	Face  int    `json:"face"`
	By    int    `json:"by"`
	Name  string `json:"name"`
}

type perudoReveal struct {
	Kind   string    `json:"kind"` // "challenge" or "spoton"
	Caller int       `json:"caller"`
	Bid    perudoBid `json:"bid"`
	Actual int       `json:"actual"`
	Dice   [][]int   `json:"dice"`   // every player's dice, by seat (empty if out)
	Loser  int       `json:"loser"`  // -1 if nobody lost a die
	Gainer int       `json:"gainer"` // -1 if nobody gained a die
	Wild   bool      `json:"wild"`
}

type perudo struct {
	env    *games.Env
	seats  games.Seats
	wild   bool
	dice   [][]int
	round  int
	cur    int
	bid    *perudoBid
	reveal *perudoReveal
	winner int
}

func newPerudo(env *games.Env, n int, wild bool) *perudo {
	g := &perudo{env: env, seats: games.NewSeats(n), wild: wild, dice: make([][]int, n), winner: -1}
	for i := range g.dice {
		g.dice[i] = make([]int, perudoStartDice)
	}
	if wild {
		env.Log(games.LogInfo, "Ones are wild this game.")
	}
	g.startRound(0)
	return g
}

func (g *perudo) minFace() int {
	if g.wild {
		return 2
	}
	return 1
}

func (g *perudo) totalDice() int {
	t := 0
	for _, s := range g.seats.Active() {
		t += len(g.dice[s])
	}
	return t
}

// count is how many dice on the table show face (ones included when wild).
func (g *perudo) count(face int) int {
	n := 0
	for _, s := range g.seats.Active() {
		for _, d := range g.dice[s] {
			if d == face || (g.wild && d == 1) {
				n++
			}
		}
	}
	return n
}

func (g *perudo) startRound(opener int) {
	g.round++
	for _, s := range g.seats.Active() {
		g.dice[s] = g.env.Roll(len(g.dice[s]))
	}
	g.cur = opener
	g.bid = nil
	g.env.Log(games.LogRoll, "Round %d: everyone rolled — %s in play. %s to open.", g.round, plural(g.totalDice(), "die", "dice"), P(opener))
}

// isRaise reports whether count×face is a legal bid now.
func (g *perudo) bidError(count, face int) error {
	if face < g.minFace() || face > 6 {
		if g.wild && face == 1 {
			return games.Errorf("Ones are wild, so you can't bid on them.")
		}
		return games.Errorf("Pick a face from %d to 6.", g.minFace())
	}
	if count < 1 || count > g.totalDice() {
		return games.Errorf("The count must be between 1 and %d (the dice in play).", g.totalDice())
	}
	if g.bid != nil && !(count > g.bid.Count || (count == g.bid.Count && face > g.bid.Face)) {
		return games.Errorf("Raise the count, or keep it at %d with a face higher than %d.", g.bid.Count, g.bid.Face)
	}
	return nil
}

// minRaise is the smallest legal bid, or nil if none is possible.
func (g *perudo) minRaise() *perudoBid {
	if g.bid == nil {
		return &perudoBid{Count: 1, Face: g.minFace(), By: -1, Name: faceCount(1, g.minFace())}
	}
	c, f := g.bid.Count, g.bid.Face+1
	if f > 6 {
		c, f = c+1, g.minFace()
	}
	if c > g.totalDice() {
		return nil
	}
	return &perudoBid{Count: c, Face: f, By: -1, Name: faceCount(c, f)}
}

func (g *perudo) Act(seat int, a games.Action) error {
	if g.winner >= 0 {
		return games.ErrGameOver
	}
	if seat != g.cur {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "bid":
		if err := g.bidError(a.Count, a.Face); err != nil {
			return err
		}
		g.bid = &perudoBid{Count: a.Count, Face: a.Face, By: seat, Name: faceCount(a.Count, a.Face)}
		g.reveal = nil
		g.env.Log(games.LogInfo, "%s bid %s.", P(seat), g.bid.Name)
		g.cur = g.seats.Next(seat)
	case "challenge", "spoton":
		if g.bid == nil {
			return games.Errorf("There's no bid yet — open the bidding.")
		}
		g.resolve(seat, a.Type)
	default:
		return games.ErrUnknown
	}
	return nil
}

func (g *perudo) resolve(caller int, kind string) {
	b := *g.bid
	actual := g.count(b.Face)
	rv := &perudoReveal{Kind: kind, Caller: caller, Bid: b, Actual: actual, Loser: -1, Gainer: -1, Wild: g.wild}
	rv.Dice = make([][]int, g.seats.N)
	for i := range rv.Dice {
		rv.Dice[i] = []int{}
		if g.seats.In(i) {
			rv.Dice[i] = games.Ints(g.dice[i])
		}
	}
	g.reveal = rv
	wildNote := ""
	if g.wild && b.Face != 1 {
		wildNote = " (counting wild ones)"
	}
	opener := caller
	if kind == "challenge" {
		g.env.Log(games.LogInfo, "%s challenged the bid of %s by %s. Cups up!", P(caller), b.Name, P(b.By))
		if actual >= b.Count {
			g.env.Log(games.LogGood, "There are %d%s — the bid of %s stands.", actual, wildNote, b.Name)
			rv.Loser = caller
		} else {
			g.env.Log(games.LogBust, "There are only %d%s — the bid of %s was a bluff.", actual, wildNote, b.Name)
			rv.Loser = b.By
		}
		opener = rv.Loser
	} else {
		g.env.Log(games.LogInfo, "%s called “spot on” on the bid of %s by %s. Cups up!", P(caller), b.Name, P(b.By))
		if actual == b.Count {
			if len(g.dice[caller]) < perudoStartDice {
				g.dice[caller] = append(g.dice[caller], 1)
				rv.Gainer = caller
				g.env.Log(games.LogGood, "Exactly %d%s — spot on! %s got a die back (%s).", actual, wildNote, P(caller), plural(len(g.dice[caller]), "die", "dice"))
			} else {
				g.env.Log(games.LogGood, "Exactly %d%s — spot on! %s already had five dice.", actual, wildNote, P(caller))
			}
		} else {
			g.env.Log(games.LogBust, "There are %d%s, not %d — not spot on.", actual, wildNote, b.Count)
			rv.Loser = caller
		}
	}
	if rv.Loser >= 0 {
		l := rv.Loser
		g.dice[l] = g.dice[l][:len(g.dice[l])-1]
		if len(g.dice[l]) == 0 {
			g.seats.Out[l] = true
			g.env.Log(games.LogBust, "%s lost their last die and is out of the game.", P(l))
			opener = g.seats.Next(l)
		} else {
			g.env.Log(games.LogBust, "%s lost a die (%s left).", P(l), plural(len(g.dice[l]), "die", "dice"))
		}
	}
	if g.checkWin() {
		return
	}
	g.startRound(opener)
}

func (g *perudo) checkWin() bool {
	if g.winner >= 0 {
		return true
	}
	if act := g.seats.Active(); len(act) == 1 {
		g.winner = act[0]
		g.bid = nil
		g.env.Log(games.LogWin, "%s won — the last player with dice!", P(g.winner))
		return true
	}
	return false
}

func (g *perudo) Legal(seat int) []games.Action {
	if g.winner >= 0 || seat != g.cur {
		return nil
	}
	out := []games.Action{}
	total := g.totalDice()
	for f := g.minFace(); f <= 6; f++ {
		c := 1
		if g.bid != nil {
			c = g.bid.Count
			if f <= g.bid.Face {
				c++
			}
		}
		if c <= total {
			out = append(out, games.Action{Type: "bid", Count: c, Face: f})
		}
	}
	if g.bid != nil {
		out = append(out, games.Action{Type: "challenge"}, games.Action{Type: "spoton"})
	}
	return out
}

func (g *perudo) Remove(seat int) {
	if seat < 0 || seat >= g.seats.N || g.seats.Out[seat] {
		return
	}
	g.seats.Out[seat] = true
	g.dice[seat] = nil
	if g.checkWin() {
		return
	}
	if g.bid != nil || seat == g.cur {
		// Their dice leave the table, so the bidding so far no longer
		// makes sense: everyone rerolls.
		opener := g.cur
		if seat == g.cur {
			opener = g.seats.Next(seat)
		}
		g.startRound(opener)
	}
}

type perudoView struct {
	Round      int           `json:"round"`
	Current    int           `json:"current"`
	Wild       bool          `json:"wild"`
	MyDice     []int         `json:"myDice,omitempty"`
	DiceCounts []int         `json:"diceCounts"`
	Out        []bool        `json:"out"`
	TotalDice  int           `json:"totalDice"`
	MinFace    int           `json:"minFace"`
	Bid        *perudoBid    `json:"bid,omitempty"`
	MinRaise   *perudoBid    `json:"minRaise,omitempty"`
	Reveal     *perudoReveal `json:"reveal,omitempty"`
	StartDice  int           `json:"startDice"`
	Winner     int           `json:"winner"`
}

func (g *perudo) View(seat int) interface{} {
	v := perudoView{
		Round: g.round, Current: g.cur, Wild: g.wild, Out: append([]bool{}, g.seats.Out...),
		TotalDice: g.totalDice(), MinFace: g.minFace(), Bid: g.bid, Reveal: g.reveal,
		StartDice: perudoStartDice, Winner: g.winner,
	}
	v.DiceCounts = make([]int, g.seats.N)
	for i := range g.dice {
		v.DiceCounts[i] = len(g.dice[i])
	}
	if g.winner < 0 {
		if g.seats.In(seat) {
			v.MyDice = games.Ints(g.dice[seat])
		}
		v.MinRaise = g.minRaise()
	}
	return v
}

func (g *perudo) Prompt(seat int) string {
	if g.winner >= 0 {
		return winPrompt(seat, g.winner)
	}
	if seat == g.cur {
		if g.bid == nil {
			return "Open the bidding: how many of a face are on the whole table?"
		}
		if g.minRaise() == nil {
			return fmt.Sprintf("%s bid %s — the highest possible. Challenge or call spot on.", P(g.bid.By), g.bid.Name)
		}
		return fmt.Sprintf("%s bid %s. Raise, challenge, or call spot on.", P(g.bid.By), g.bid.Name)
	}
	if g.bid == nil {
		return fmt.Sprintf("Waiting for %s to open the bidding.", P(g.cur))
	}
	return fmt.Sprintf("Waiting for %s to answer the bid of %s by %s.", P(g.cur), g.bid.Name, P(g.bid.By))
}

func (g *perudo) Waiting() []int {
	if g.winner >= 0 {
		return nil
	}
	return []int{g.cur}
}

func (g *perudo) Badges() []string {
	out := make([]string, g.seats.N)
	for i := range out {
		switch {
		case g.seats.Out[i] && len(g.dice[i]) == 0 && i != g.winner:
			out[i] = "out"
		default:
			out[i] = plural(len(g.dice[i]), "die", "dice")
		}
	}
	return out
}

func (g *perudo) Over() bool { return g.winner >= 0 }

func (g *perudo) Winners() []int {
	if g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
