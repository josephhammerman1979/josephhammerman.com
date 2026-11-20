// Package category holds category games: games where each roll is written
// into one of a fixed set of boxes on a score sheet.
package category

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Catego (Reiner Knizia, from Dice Games Properly Explained).
//
// Each player has a row of eleven columns, 2 to 12.  Over eleven rounds,
// each player in turn rolls two dice and writes the sum into any empty
// column of their own row — the sum has nothing to do with the column's
// number.  When every column is filled, each column goes to the player who
// wrote the highest value in it and is worth its number in points (column
// 12 is worth 12).  If the highest value in a column is tied, nobody wins
// it.  Highest total wins.
//
// Actions: {type:"roll"}, then {type:"place", index: column (2..12)}.

const (
	firstCol = 2
	lastCol  = 12
	numCols  = lastCol - firstCol + 1
)

func init() {
	games.Register(&games.Def{
		ID: "catego", Name: "Catego", Order: 150,
		Summary:    "Eleven rounds, eleven columns worth 2 to 12. Bid your rolls to win the valuable ones.",
		MinPlayers: 2, MaxPlayers: 4,
		Rules: []string{
			"Each round, roll two dice and write the total in any empty column of your row (2 to 12).",
			"The total doesn't have to match the column — put a big roll where you most want to win.",
			"After 11 rounds every column is full. Each column goes to whoever wrote the highest number in it and scores the column's number (column 12 scores 12).",
			"If the highest number in a column is tied, nobody wins that column.",
			"Most points wins. Should you lock up a cheap column with a so-so roll, or fight for 12?",
		},
		Facts: []string{"Number of dice: 2", "Player count: 2-4", "Object: Win the most valuable columns"},
		Sheet: []string{
			"From Reiner Knizia's Dice Games Properly Explained. Each player rolls two dice and records the sum against one of the eleven columns, 2 through 12, of their row. The value recorded has no relation to the column number, so any sum can be recorded against any column.",
			"Once 11 rounds are over and every player has recorded a value in each column, each column is won by the player who recorded the highest value in it. If the highest value in a column is shared by two or more players, nobody wins that column.",
			"Each column is worth its number: column 2 is worth the least and column 12 the most. A player's score is the total of the columns they win, and the highest score wins.",
		},
		New: func(env *games.Env, n int, _ map[string]string) games.Game { return newCatego(env, n) },
	})
}

type catego struct {
	env    *games.Env
	seats  games.Seats
	sheet  [][]int // sheet[seat][col-2]; 0 = empty
	cur    int
	round  int   // 1..11
	dice   []int // current roll, nil until rolled this turn
	over   bool
	scores []int // final scores, set when over
}

func newCatego(env *games.Env, n int) *catego {
	g := &catego{env: env, seats: games.NewSeats(n), sheet: make([][]int, n), round: 1}
	for i := range g.sheet {
		g.sheet[i] = make([]int, numCols)
	}
	return g
}

func (g *catego) Act(seat int, a games.Action) error {
	if g.over {
		return games.ErrGameOver
	}
	if seat != g.cur {
		return games.ErrNotYourTurn
	}
	switch a.Type {
	case "roll":
		if g.dice != nil {
			return games.Errorf("You've rolled — now pick a column for your %d.", games.Sum(g.dice))
		}
		g.dice = g.env.Roll(2)
		g.env.Log(games.LogRoll, "%s rolled %s = %d", games.P(seat), games.DiceText(g.dice), games.Sum(g.dice))
	case "place":
		if g.dice == nil {
			return games.Errorf("Roll first.")
		}
		col := a.Index
		if col < firstCol || col > lastCol {
			return games.Errorf("Pick a column from 2 to 12.")
		}
		if g.sheet[seat][col-firstCol] != 0 {
			return games.Errorf("You've already filled column %d.", col)
		}
		v := games.Sum(g.dice)
		g.sheet[seat][col-firstCol] = v
		g.env.Log(games.LogInfo, "%s wrote %d in column %d", games.P(seat), v, col)
		g.dice = nil
		g.advance()
	default:
		return games.ErrUnknown
	}
	return nil
}

// advance passes the turn, moving to the next round after the last active
// player, and scores the game after round 11.
func (g *catego) advance() {
	active := g.seats.Active()
	if g.cur == active[len(active)-1] {
		if g.round == numCols {
			g.finish()
			return
		}
		g.round++
		g.cur = active[0]
		return
	}
	g.cur = g.seats.Next(g.cur)
}

// columnWinner returns the seat holding the unique highest value in col, or
// -1 if the column is empty or the top value is tied.
func (g *catego) columnWinner(col int) int {
	best, winner := 0, -1
	for _, seat := range g.seats.Active() {
		v := g.sheet[seat][col-firstCol]
		switch {
		case v > best:
			best, winner = v, seat
		case v == best && v > 0:
			winner = -1
		}
	}
	return winner
}

func (g *catego) totals() []int {
	t := make([]int, g.seats.N)
	for col := firstCol; col <= lastCol; col++ {
		if w := g.columnWinner(col); w >= 0 {
			t[w] += col
		}
	}
	return t
}

func (g *catego) finish() {
	g.over = true
	g.scores = g.totals()
	for col := firstCol; col <= lastCol; col++ {
		if w := g.columnWinner(col); w >= 0 {
			g.env.Log(games.LogGood, "Column %d goes to %s", col, games.P(w))
		} else {
			g.env.Log(games.LogInfo, "Column %d is tied — nobody wins it", col)
		}
	}
	w := g.Winners()
	if len(w) == 1 {
		g.env.Log(games.LogWin, "%s wins with %d points!", games.P(w[0]), g.scores[w[0]])
	} else {
		g.env.Log(games.LogWin, "A tie at %d points!", g.scores[w[0]])
	}
}

func (g *catego) Legal(seat int) []games.Action {
	if g.over || seat != g.cur {
		return nil
	}
	if g.dice == nil {
		return []games.Action{{Type: "roll"}}
	}
	out := []games.Action{}
	for col := firstCol; col <= lastCol; col++ {
		if g.sheet[seat][col-firstCol] == 0 {
			out = append(out, games.Action{Type: "place", Index: col})
		}
	}
	return out
}

// Remove drops a player; their column entries no longer count.  Rounds are
// counted by the remaining players' turns, so the game still ends after
// every remaining player has filled all eleven columns.
func (g *catego) Remove(seat int) {
	wasCur := seat == g.cur
	wasLast := false
	if active := g.seats.Active(); len(active) > 0 {
		wasLast = seat == active[len(active)-1]
	}
	g.seats.Out[seat] = true
	if !wasCur {
		return
	}
	g.dice = nil
	active := g.seats.Active()
	if wasLast {
		if g.round == numCols {
			g.finish()
			return
		}
		g.round++
		g.cur = active[0]
		return
	}
	g.cur = g.seats.Next(seat)
}

type categoView struct {
	Round    int     `json:"round"`
	Rounds   int     `json:"rounds"`
	Current  int     `json:"current"`
	Dice     []int   `json:"dice"`
	Sheet    [][]int `json:"sheet"`   // [seat][col-2]
	Leaders  []int   `json:"leaders"` // per column: current unique leader or -1
	Points   []int   `json:"points"`  // per seat: points from columns currently led
	Over     bool    `json:"over"`
	FirstCol int     `json:"firstCol"`
}

func (g *catego) View(seat int) interface{} {
	sheet := make([][]int, len(g.sheet))
	for i := range g.sheet {
		sheet[i] = games.Ints(g.sheet[i])
	}
	leaders := make([]int, numCols)
	for col := firstCol; col <= lastCol; col++ {
		leaders[col-firstCol] = g.columnWinner(col)
	}
	return categoView{
		Round: g.round, Rounds: numCols, Current: g.cur, Dice: games.Ints(g.dice),
		Sheet: sheet, Leaders: leaders, Points: g.totals(), Over: g.over, FirstCol: firstCol,
	}
}

func (g *catego) Prompt(seat int) string {
	if g.over {
		return "All columns are filled."
	}
	if seat != g.cur {
		return fmt.Sprintf("Round %d of %d — waiting for %s.", g.round, numCols, games.P(g.cur))
	}
	if g.dice == nil {
		return fmt.Sprintf("Round %d of %d — roll two dice.", g.round, numCols)
	}
	return fmt.Sprintf("You rolled %d — tap an empty column to write it there.", games.Sum(g.dice))
}

func (g *catego) Waiting() []int {
	if g.over {
		return nil
	}
	return []int{g.cur}
}

func (g *catego) Badges() []string {
	t := g.totals()
	out := make([]string, len(t))
	for i, v := range t {
		out[i] = fmt.Sprintf("%d pts", v)
	}
	return out
}

func (g *catego) Over() bool { return g.over }

func (g *catego) Winners() []int {
	if !g.over {
		return nil
	}
	return games.BestSeats(&g.seats, g.scores, false)
}
