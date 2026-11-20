// Package pushluck holds the push-your-luck and keep-the-dice games: Ten
// Thousand, Midnight, Sevens Out, Sixteen and Threes.
package pushluck

import (
	"fmt"
	"strings"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// keepTurn is a turn of "roll the dice you haven't kept, keep at least one,
// repeat until every die is kept" (Midnight, Sixteen, Threes).
//
// Actions: {type:"roll"} starts the turn; {type:"keep", dice:[indices into
// Roll]} keeps those dice and, if any are left, rolls the rest at once.
type keepTurn struct {
	N     int
	Kept  []int // faces kept so far, in the order they were kept
	Roll  []int // the live dice; nil before the turn's first roll
	Rolls int
}

func newKeepTurn(n int) keepTurn { return keepTurn{N: n} }

func (k *keepTurn) started() bool { return k.Rolls > 0 }
func (k *keepTurn) done() bool    { return len(k.Kept) >= k.N }
func (k *keepTurn) left() int     { return k.N - len(k.Kept) }

func (k *keepTurn) roll(env *games.Env) {
	k.Roll = env.Roll(k.left())
	k.Rolls++
}

// keep validates idx and moves those dice from Roll to Kept, returning the
// kept faces.  Roll is cleared; the caller rolls again if dice are left.
func (k *keepTurn) keep(idx []int) ([]int, error) {
	if !k.started() || len(k.Roll) == 0 {
		return nil, games.Errorf("Roll first.")
	}
	sel, err := games.UniqueIndices(idx, len(k.Roll))
	if err != nil {
		return nil, err
	}
	if len(sel) == 0 {
		return nil, games.Errorf("Tap at least one die to keep.")
	}
	faces := make([]int, len(sel))
	for i, j := range sel {
		faces[i] = k.Roll[j]
	}
	k.Kept = append(k.Kept, faces...)
	k.Roll = nil
	return faces, nil
}

// legal samples the turn's actions: roll, or keep one die of each face, or
// keep them all.
func (k *keepTurn) legal() []games.Action {
	if !k.started() {
		return []games.Action{{Type: "roll"}}
	}
	out := []games.Action{}
	seen := map[int]bool{}
	all := []int{}
	for i, f := range k.Roll {
		all = append(all, i)
		if !seen[f] {
			seen[f] = true
			out = append(out, games.Action{Type: "keep", Dice: []int{i}})
		}
	}
	if len(all) > 1 {
		out = append(out, games.Action{Type: "keep", Dice: all})
	}
	return out
}

// turnOrder walks a fixed list of seats, skipping any that leave.
type turnOrder struct {
	Order []int
	Pos   int
}

func (t *turnOrder) start(order []int, s *games.Seats) {
	t.Order = order
	t.Pos = 0
	t.skip(s)
}

func (t *turnOrder) skip(s *games.Seats) {
	for t.Pos < len(t.Order) && !s.In(t.Order[t.Pos]) {
		t.Pos++
	}
}

func (t *turnOrder) next(s *games.Seats) {
	t.Pos++
	t.skip(s)
}

func (t *turnOrder) done() bool { return t.Pos >= len(t.Order) }

// cur is the seat whose turn it is, or -1 once everyone has played.
func (t *turnOrder) cur() int {
	if t.done() {
		return -1
	}
	return t.Order[t.Pos]
}

// activeIn lists the seats of order still in the game.
func activeIn(order []int, s *games.Seats) []int {
	out := []int{}
	for _, seat := range order {
		if s.In(seat) {
			out = append(out, seat)
		}
	}
	return out
}

// nameList joins seat name tokens: "{P0}, {P1} and {P2}".
func nameList(seats []int) string {
	parts := make([]string, len(seats))
	for i, s := range seats {
		parts[i] = games.P(s)
	}
	if len(parts) <= 1 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// comma formats 10000 as "10,000".
func comma(n int) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

func atoiDefault(s string, def int, allowed ...int) int {
	for _, a := range allowed {
		if fmt.Sprint(a) == s {
			return a
		}
	}
	return def
}

func intsOf(n, v int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = v
	}
	return out
}

// copyHands deep-copies per-seat dice for a view.
func copyHands(h [][]int) [][]int {
	out := make([][]int, len(h))
	for i, d := range h {
		if d != nil {
			out[i] = games.Ints(d)
		}
	}
	return out
}

// pickWinner returns the single seat among best, rolling off if tied.
func pickWinner(env *games.Env, best []int) int {
	if len(best) == 1 {
		return best[0]
	}
	env.Log(games.LogInfo, "%s are tied — roll-off!", nameList(best))
	return games.RollOff(env, best)
}
