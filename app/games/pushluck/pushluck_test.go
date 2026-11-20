package pushluck

import (
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func mustAct(t *testing.T, g games.Game, seat int, a games.Action) {
	t.Helper()
	if err := g.Act(seat, a); err != nil {
		t.Fatalf("seat %d %+v: %v", seat, a, err)
	}
}

func idx(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

func roll() games.Action             { return games.Action{Type: "roll"} }
func keep(d ...int) games.Action     { return games.Action{Type: "keep", Dice: d} }
func keepAll(n int) games.Action     { return games.Action{Type: "keep", Dice: idx(n)} }
func keepRoll(d ...int) games.Action { return games.Action{Type: "keep_roll", Dice: d} }
func keepBank(d ...int) games.Action { return games.Action{Type: "keep_bank", Dice: d} }

func TestAllGamesPlayToCompletion(t *testing.T) {
	cases := []struct {
		id   string
		opts []map[string]string
	}{
		{"tenthousand", []map[string]string{{"target": "5000"}, {"target": "10000"}}},
		{"midnight", []map[string]string{{"rounds": "5"}, {"rounds": "3"}, {"rounds": "7"}}},
		{"sevensout", []map[string]string{nil}},
		{"sixteen", []map[string]string{{"rounds": "3"}, {"rounds": "5"}}},
		{"threes", []map[string]string{{"hands": "5"}, {"hands": "3"}, {"hands": "8"}}},
	}
	for _, c := range cases {
		def := games.Lookup(c.id)
		if def == nil {
			t.Fatalf("%s not registered", c.id)
		}
		if len(def.Rules) == 0 || len(def.Sheet) == 0 || len(def.Facts) == 0 || def.Summary == "" {
			t.Errorf("%s: incomplete Def", c.id)
		}
		for _, o := range c.opts {
			if err := games.SimulateMany(def, 5, o); err != nil {
				t.Fatalf("%v (opts %v)", err, o)
			}
		}
	}
}

// ── Ten Thousand ────────────────────────────────────────────────────────────

func TestTenThousandScoring(t *testing.T) {
	cases := []struct {
		dice []int
		pts  int
		ok   bool
	}{
		{[]int{1}, 100, true},
		{[]int{5}, 50, true},
		{[]int{1, 5}, 150, true},
		{[]int{1, 1, 1}, 1000, true},
		{[]int{2, 2, 2}, 200, true},
		{[]int{6, 6, 6}, 600, true},
		{[]int{5, 5, 5, 5}, 550, true},
		{[]int{1, 1, 1, 1}, 1100, true},
		{[]int{4, 4, 4, 4, 4, 4}, 800, true},
		{[]int{4, 4, 4, 4}, 0, false},
		{[]int{2}, 0, false},
		{[]int{1, 2}, 0, false},
		{nil, 0, false},
	}
	for _, c := range cases {
		pts, ok := ttScore(c.dice)
		if pts != c.pts || ok != c.ok {
			t.Errorf("ttScore(%v) = %d,%v want %d,%v", c.dice, pts, ok, c.pts, c.ok)
		}
	}
	if got := ttScoring([]int{2, 1, 2, 3, 2, 5}); len(got) != 5 {
		t.Errorf("ttScoring = %v, want five dice (1, 5 and the 2s)", got)
	}
	if got := ttScoring([]int{2, 3, 4, 6, 2, 3}); len(got) != 0 {
		t.Errorf("ttScoring found scoring dice in a dud: %v", got)
	}
}

func TestTenThousandTurn(t *testing.T) {
	g := newTenThousand(games.NewTestEnv(
		1, 5, 2, 3, 4, 6, // seat 0, roll 1
		2, 3, 4, 6, 2, // roll 2 (5 dice): bust
		1, 1, 1, 5, 5, 5, // seat 1: all six score → hot dice
		1, 2, 3, 4, 6, 6, // seat 1, six again
	), 3, 5000)
	mustAct(t, g, 0, roll())
	if err := g.Act(0, keepRoll(2)); err == nil {
		t.Fatal("setting aside a non-scoring 2 should be rejected")
	}
	if err := g.Act(0, keepRoll()); err == nil {
		t.Fatal("setting aside nothing should be rejected")
	}
	if err := g.Act(0, roll()); err == nil {
		t.Fatal("rolling without setting aside should be rejected")
	}
	mustAct(t, g, 0, keepRoll(0)) // keep the 1, roll five → bust
	if g.cur != 1 || g.totals[0] != 0 || g.turn != 0 {
		t.Fatalf("after bust: cur=%d totals=%v turn=%d", g.cur, g.totals, g.turn)
	}
	if g.last == nil || !g.last.Bad {
		t.Fatal("bust should be recorded as the last turn")
	}
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, keepRoll(idx(6)...))
	if g.turn != 1500 || len(g.roll) != 6 {
		t.Fatalf("hot dice: turn=%d roll=%v, want 1500 and six new dice", g.turn, g.roll)
	}
	mustAct(t, g, 1, keepBank(0))
	if g.totals[1] != 1600 || g.cur != 2 {
		t.Fatalf("bank: totals=%v cur=%d", g.totals, g.cur)
	}
}

func TestTenThousandWinAndRemove(t *testing.T) {
	g := newTenThousand(games.NewTestEnv(5, 2, 2, 3, 4, 6), 3, 5000)
	g.totals[0] = 4950
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keepBank(0))
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatalf("expected seat 0 to win: totals=%v", g.totals)
	}

	g = newTenThousand(games.NewTestEnv(1, 2, 2, 3, 4, 6), 3, 5000)
	mustAct(t, g, 0, roll())
	g.Remove(0)
	if g.cur != 1 || g.roll != nil || g.turn != 0 {
		t.Fatalf("remove current: cur=%d roll=%v", g.cur, g.roll)
	}
	g.Remove(2)
	g.cur = 1
	g.nextTurn()
	if g.cur != 1 {
		t.Fatalf("removed seats should be skipped; cur=%d", g.cur)
	}
}

// ── Midnight ────────────────────────────────────────────────────────────────

func TestMidnightScoring(t *testing.T) {
	for _, c := range []struct {
		d    []int
		want int
	}{
		{[]int{1, 4, 6, 6, 6, 6}, 24},
		{[]int{1, 1, 4, 4, 6, 6}, 17},
		{[]int{1, 2, 6, 6, 6, 6}, -1},
		{[]int{4, 4, 6, 6, 6, 6}, -1},
	} {
		if got := midnightScore(c.d); got != c.want {
			t.Errorf("midnightScore(%v) = %d, want %d", c.d, got, c.want)
		}
	}
}

func TestMidnightRound(t *testing.T) {
	g := newMidnight(games.NewTestEnv(
		1, 4, 6, 6, 6, 6, // first player: 24
		6, 6, 2, 2, 2, 2, // second: keeps 6 6, then...
		6, 6, 6, 6, // ...no 1 and 4: doesn't qualify
		1, 4, 6, 6, 6, 6, // third: 24, tie
		2, 5, // roll-off: first player 2, third 5
	), 3, 1)
	order := g.Order
	mustAct(t, g, order[0], roll())
	mustAct(t, g, order[0], keepAll(6))
	mustAct(t, g, order[1], roll())
	if err := g.Act(order[1], keep()); err == nil {
		t.Fatal("keeping nothing should be rejected")
	}
	mustAct(t, g, order[1], keep(0, 1))
	if len(g.turn.Roll) != 4 {
		t.Fatalf("after keeping two, %d dice rolled, want 4", len(g.turn.Roll))
	}
	mustAct(t, g, order[1], keepAll(4))
	mustAct(t, g, order[2], roll())
	mustAct(t, g, order[2], keepAll(6))
	if !g.Over() || g.Winners()[0] != order[2] || g.wins[order[2]] != 1 {
		t.Fatalf("expected %d to win the roll-off: wins=%v", order[2], g.wins)
	}
	if g.chips[order[2]] != 2 || g.chips[order[0]] != -1 || g.chips[order[1]] != -1 || g.pot != 0 || g.last.Pot != 3 {
		t.Fatalf("winner should take the pot of 3: chips=%v pot=%d", g.chips, g.pot)
	}
	if g.last.Scores[order[1]] != -1 || g.last.Scores[order[0]] != 24 {
		t.Fatalf("round scores %v", g.last.Scores)
	}
}

func TestMidnightNobodyQualifiesAndRotation(t *testing.T) {
	g := newMidnight(games.NewTestEnv(2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2), 3, 3)
	first := g.first
	for i := 0; i < 3; i++ {
		s := g.cur()
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(6))
	}
	if g.round != 2 || g.last.Winner != -1 || g.wins[0]+g.wins[1]+g.wins[2] != 0 {
		t.Fatalf("round=%d winner=%d wins=%v", g.round, g.last.Winner, g.wins)
	}
	if g.pot != 6 || g.chips[0] != -2 {
		t.Fatalf("pot should carry over: pot=%d chips=%v", g.pot, g.chips)
	}
	if g.first != (first+1)%3 {
		t.Fatalf("round 2 first = %d, want %d", g.first, (first+1)%3)
	}
	// Removing the current player passes the turn.
	cur := g.cur()
	mustAct(t, g, cur, roll())
	g.Remove(cur)
	if g.cur() == cur || g.turn.started() {
		t.Fatalf("remove current: cur=%d started=%v", g.cur(), g.turn.started())
	}
}

// ── Sevens Out ──────────────────────────────────────────────────────────────

func TestSevensRemove(t *testing.T) {
	left := func(roll []int) []int {
		out := []int{}
		for i, g := range sevensRemove(roll) {
			if !g {
				out = append(out, roll[i])
			}
		}
		return out
	}
	if got := left([]int{1, 6, 6, 2, 5, 3}); games.DiceText(got) != "6 3" {
		t.Errorf("left = %v, want 6 3", got)
	}
	if got := left([]int{3, 4, 3, 4, 3, 4}); len(got) != 0 {
		t.Errorf("left = %v, want nothing", got)
	}
	if got := left([]int{6, 6, 6, 1, 2, 2}); games.DiceText(got) != "6 6 2 2" {
		t.Errorf("left = %v, want 6 6 2 2", got)
	}
}

func TestSevensOutRollLimit(t *testing.T) {
	g := newSevensOut(games.NewTestEnv(
		1, 6, 2, 2, 2, 2, // seat 0: 8 showing
		5, 5, 1, 1, // roll 2: 12 showing; stops → limit 2
		6, 6, 6, 6, 6, 6, // seat 1: 36
		6, 6, 6, 6, 6, 6, // roll 2 → limit reached, scored automatically
		3, 4, 3, 4, 3, 4, // seat 2: everything removed → 0
	), 3)
	if err := g.Act(0, games.Action{Type: "stop"}); err == nil {
		t.Fatal("stopping before rolling should be rejected")
	}
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, games.Action{Type: "stop"})
	if g.limit != 2 || g.totals[0] != 12 {
		t.Fatalf("limit=%d totals=%v, want 2 and 12", g.limit, g.totals)
	}
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, roll())
	if g.cur() != 2 || g.totals[1] != 36 {
		t.Fatalf("roll limit not enforced: cur=%d totals=%v", g.cur(), g.totals)
	}
	g.totals[1] = 50
	mustAct(t, g, 2, roll())
	if !g.Over() || g.Winners()[0] != 1 || g.totals[2] != 0 {
		t.Fatalf("expected seat 1 to win at the end of the round: totals=%v over=%v", g.totals, g.Over())
	}
}

func TestSevensOutRoundRotation(t *testing.T) {
	g := newSevensOut(games.NewTestEnv(), 3)
	for i := 0; i < 3; i++ {
		s := g.cur()
		mustAct(t, g, s, roll())
		if g.cur() == s {
			mustAct(t, g, s, games.Action{Type: "stop"})
		}
	}
	if g.round != 2 || g.first != 1 || g.cur() != 1 || g.limit != 0 {
		t.Fatalf("round=%d first=%d cur=%d limit=%d", g.round, g.first, g.cur(), g.limit)
	}
}

// ── Sixteen ─────────────────────────────────────────────────────────────────

func TestSixteenRounds(t *testing.T) {
	g := newSixteen(games.NewTestEnv(
		6, 6, 2, // seat 0 keeps 6 6 (12)...
		5,       // ...then a 5: 17, out
		4, 6, 2, // seat 1 keeps 4 6 (10)...
		6, // ...then a 6: 16
	), 2, 3)
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keep(0, 1))
	mustAct(t, g, 0, keep(0))
	if g.cur() != 1 || !g.bust[0] || g.scores[0] != 17 {
		t.Fatalf("seat 0 should be out: cur=%d bust=%v scores=%v", g.cur(), g.bust, g.scores)
	}
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, keep(0, 1))
	if games.Sum(g.turn.Kept) != 10 || len(g.turn.Roll) != 1 {
		t.Fatalf("kept=%v roll=%v", g.turn.Kept, g.turn.Roll)
	}
	mustAct(t, g, 1, keep(0))
	if g.wins[1] != 1 || g.round != 2 || g.first != 1 {
		t.Fatalf("wins=%v round=%d first=%d", g.wins, g.round, g.first)
	}
}

func TestSixteenTwoPlayerTieAndMajority(t *testing.T) {
	g := newSixteen(games.NewTestEnv(
		5, 5, 5, // round 1, seat 0: 15
		6, 4, 5, // seat 1: 15 → tie, seat 0 rolled first
		6, 6, 4, // round 2, seat 1: 16
		4, 4, 4, // seat 0: 12
	), 2, 3)
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keepAll(3))
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, keepAll(3))
	if g.wins[0] != 1 {
		t.Fatalf("two-player tie should go to the first to roll: wins=%v", g.wins)
	}
	g.wins[1] = 1 // pretend seat 1 has a win, so a second wins the match
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, keepAll(3))
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keepAll(3))
	if !g.Over() || g.Winners()[0] != 1 {
		t.Fatalf("majority should end the match: wins=%v over=%v", g.wins, g.Over())
	}
}

func TestSixteenThreePlayerTieRollsOff(t *testing.T) {
	g := newSixteen(games.NewTestEnv(
		5, 5, 5, // seat 0: 15
		6, 4, 5, // seat 1: 15
		1, 1, 1, // seat 2: 3
		2, 6, // roll-off: seat 1 wins
	), 3, 3)
	for s := 0; s < 3; s++ {
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(3))
	}
	if g.wins[1] != 1 || g.wins[0] != 0 {
		t.Fatalf("roll-off should decide a 3+ player tie: wins=%v", g.wins)
	}
}

// ── Threes ──────────────────────────────────────────────────────────────────

func TestThreesScoringAndTieReplay(t *testing.T) {
	if threesScore([]int{3, 3, 1, 2, 6}) != 9 || threesScore([]int{3, 3, 3, 3, 3}) != 0 {
		t.Fatal("3s should count as zero")
	}
	g := newThrees(games.NewTestEnv(
		3, 3, 1, 2, 6, // seat 0: 9
		3, 3, 3, 4, 5, // seat 1: 9 (tie)
		6, 6, 6, 6, 5, // seat 2: 29
		1, 1, 1, 1, 1, // replay, seat 0: 5
		3, 3, 3, 3, 1, // replay, seat 1: 1 → wins
	), 3, 5)
	if g.pot != 3 || g.chips[0] != -1 {
		t.Fatalf("ante: pot=%d chips=%v", g.pot, g.chips)
	}
	for s := 0; s < 3; s++ {
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(5))
	}
	if g.replay != 1 || g.pot != 5 || g.cur() != 0 || len(activeIn(g.Order, &g.seats)) != 2 {
		t.Fatalf("tie replay: replay=%d pot=%d cur=%d order=%v", g.replay, g.pot, g.cur(), g.Order)
	}
	if err := g.Act(2, roll()); err == nil {
		t.Fatal("seat 2 isn't in the replay")
	}
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keepAll(5))
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, keepAll(5))
	// Seat 1 paid 2 and won 5; then everyone antes for hand 2.
	if g.chips[1] != 2 || g.chips[0] != -3 || g.chips[2] != -2 || g.hand != 2 || g.cur() != 1 || g.pot != 3 {
		t.Fatalf("after replay: chips=%v hand=%d cur=%d pot=%d", g.chips, g.hand, g.cur(), g.pot)
	}
}

func TestThreesShootTheMoon(t *testing.T) {
	g := newThrees(games.NewTestEnv(6, 6, 6, 1, 1, 6, 6), 3, 1)
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keep(0, 1, 2))
	mustAct(t, g, 0, keep(0, 1))
	if !g.Over() || g.Winners()[0] != 0 || g.chips[0] != 2 {
		t.Fatalf("moon should win instantly: over=%v chips=%v", g.Over(), g.chips)
	}
}

func TestThreesRemoveDuringReplay(t *testing.T) {
	g := newThrees(games.NewTestEnv(1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 6, 6, 6, 6, 5), 3, 5)
	for s := 0; s < 3; s++ {
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(5))
	}
	if g.replay != 1 {
		t.Fatalf("expected a replay between seats 0 and 1")
	}
	g.Remove(0)
	if g.replay != 0 || g.hand != 2 || g.starter != 1 || g.chips[1] != 2 {
		t.Fatalf("last one left in the replay should win: hand=%d starter=%d chips=%v", g.hand, g.starter, g.chips)
	}
}
