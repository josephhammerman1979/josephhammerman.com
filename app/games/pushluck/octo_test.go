package pushluck

import (
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestOctoPlaysToCompletion(t *testing.T) {
	if err := games.SimulateMany(games.Lookup("octo"), 5, nil); err != nil {
		t.Fatal(err)
	}
}

// newTestOcto builds a two-player Octo where seat 0 goes first (the
// opening roll-off uses the first two scripted faces), then rolls faces.
func newTestOcto(faces ...int) *octo {
	return newOcto(games.NewTestEnv(append([]int{6, 1}, faces...)...), 2)
}

func TestOctoSetAsideRules(t *testing.T) {
	g := newTestOcto(1, 2, 2, 3, 4, 5, 5, 5 /* reroll 5 dice */, 5, 5, 6, 6, 1)
	if g.cur() != 0 {
		t.Fatalf("opening roll-off: cur=%d, want 0", g.cur())
	}
	g.Act(0, games.Action{Type: "roll"})
	if err := g.Act(0, games.Action{Type: "keep_roll", Dice: []int{1, 3}}); err == nil {
		t.Fatal("setting aside two different numbers should be rejected")
	}
	if err := g.Act(0, games.Action{Type: "keep_roll", Dice: []int{5, 6, 7}}); err != nil {
		t.Fatal(err)
	}
	if g.turnTotal() != 15 || len(g.roll) != 5 {
		t.Fatalf("turn=%d live=%d, want 15 and 5", g.turnTotal(), len(g.roll))
	}
	// 5 5 6 6 1: the 5s are used, so the 6s can be taken but not the 5s.
	if err := g.Act(0, games.Action{Type: "keep_stop", Dice: []int{0, 1}}); err == nil {
		t.Fatal("setting aside an already-used number should be rejected")
	}
	g.Act(0, games.Action{Type: "keep_stop", Dice: []int{2, 3}})
	if g.totals[0] != 27 || g.cur() != 1 {
		t.Fatalf("total=%d cur=%d, want 27, 1", g.totals[0], g.cur())
	}
}

func TestOctoBustScoresZero(t *testing.T) {
	// Take the 4s, then roll only 4s again: bust.
	g := newTestOcto(4, 4, 4, 4, 4, 4, 4, 1 /* reroll 1 */, 4)
	g.Act(0, games.Action{Type: "roll"})
	g.Act(0, games.Action{Type: "keep_roll", Dice: []int{0, 1, 2, 3, 4, 5, 6}})
	if g.totals[0] != 0 || g.scores[0][0] != 0 || g.cur() != 1 {
		t.Fatalf("bust: total=%d score=%d cur=%d", g.totals[0], g.scores[0][0], g.cur())
	}
}

func TestOctoAllEightSetAsideEndsTurn(t *testing.T) {
	g := newTestOcto(3, 3, 3, 3, 3, 3, 3, 3)
	g.Act(0, games.Action{Type: "roll"})
	if err := g.Act(0, games.Action{Type: "keep_roll", Dice: []int{0, 1, 2, 3, 4, 5, 6, 7}}); err == nil {
		t.Fatal("rolling on with no dice left should be rejected")
	}
	g.Act(0, games.Action{Type: "keep_stop", Dice: []int{0, 1, 2, 3, 4, 5, 6, 7}})
	if g.totals[0] != 24 {
		t.Fatalf("total=%d, want 24", g.totals[0])
	}
}

func TestOctoRoundsEqualPlayers(t *testing.T) {
	g := newOcto(games.NewTestEnv(6, 1, 5), 3)
	turns := 0
	for !g.Over() && turns < 100 {
		seat := g.cur()
		g.Act(seat, games.Action{Type: "roll"})
		if g.roll != nil {
			g.Act(seat, g.Legal(seat)[0])
		}
		turns++
	}
	if !g.Over() || g.round != 3 || turns != 9 {
		t.Fatalf("over=%v round=%d turns=%d, want 3 rounds of 3 turns", g.Over(), g.round, turns)
	}
}
