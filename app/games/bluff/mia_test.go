package bluff

import (
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestMiaPlaysToCompletion(t *testing.T) {
	for _, lives := range []string{"3", "5"} {
		if err := games.SimulateMany(games.Lookup("mia"), 5, map[string]string{"lives": lives}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMiaRanking(t *testing.T) {
	order := [][]int{{1, 3}, {3, 2}, {4, 1}, {6, 5}, {1, 1}, {3, 3}, {6, 6}, {1, 2}}
	for i := 1; i < len(order); i++ {
		a, b := miaValue(order[i]), miaValue(order[i-1])
		if miaRank(a) <= miaRank(b) {
			t.Errorf("%v (%d) should beat %v (%d)", order[i], a, order[i-1], b)
		}
	}
	if miaValue([]int{2, 1}) != miaMia || miaValue([]int{3, 5}) != 53 {
		t.Error("miaValue reads high die first")
	}
}

func newTestMia(faces ...int) *mia {
	g := newMia(games.NewTestEnv(faces...), 3, 3)
	g.cur = 0
	return g
}

func TestMiaBluffCaught(t *testing.T) {
	g := newTestMia(4, 2)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	if err := g.Act(1, games.Action{Type: "challenge"}); err == nil {
		t.Fatal("nothing to challenge yet")
	}
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 55})
	if g.Waiting()[0] != 1 {
		t.Fatalf("waiting on %v, want 1", g.Waiting())
	}
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if g.lives[0] != 2 || g.lives[1] != 3 {
		t.Fatalf("lives = %v, the bluffer should lose one", g.lives)
	}
	if g.phase != "roll" || g.cur != 1 || g.claim != 0 {
		t.Fatalf("challenge winner should start fresh: phase=%s cur=%d claim=%d", g.phase, g.cur, g.claim)
	}
	if g.reveal == nil || g.reveal.Truthful || g.reveal.Real != 42 {
		t.Fatalf("reveal = %+v", g.reveal)
	}
	if !strings.Contains(lastLog(g.env, 5), "showed 4 2 — the claim of 5-5 pair was a bluff") {
		t.Errorf("log: %s", lastLog(g.env, 5))
	}
}

func TestMiaTruthWinsAndTiesFavourClaimant(t *testing.T) {
	g := newTestMia(6, 5)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 65})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if g.lives[1] != 2 || g.lives[0] != 3 || g.cur != 0 {
		t.Fatalf("an exact claim should cost the challenger: lives=%v cur=%d", g.lives, g.cur)
	}
}

func TestMiaAcceptMustGoHigherAndMatchMia(t *testing.T) {
	g := newTestMia(3, 1, 5, 2, 4, 4)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 64})
	mustAct(t, g, 1, games.Action{Type: "accept"})
	if g.cur != 1 || g.phase != "declare" || miaValue(g.dice) != 52 {
		t.Fatalf("accept should roll for seat 1: cur=%d phase=%s dice=%v", g.cur, g.phase, g.dice)
	}
	if err := g.Act(1, games.Action{Type: "declare", Index: 63}); err == nil {
		t.Fatal("declaring lower than the accepted claim must fail")
	}
	if err := g.Act(1, games.Action{Type: "declare", Index: 64}); err == nil {
		t.Fatal("declaring the same as the accepted claim must fail")
	}
	mustAct(t, g, 1, games.Action{Type: "declare", Index: miaMia})
	mustAct(t, g, 2, games.Action{Type: "accept"})
	if err := g.Act(2, games.Action{Type: "declare", Index: 66}); err == nil {
		t.Fatal("after accepting Mia you must declare Mia")
	}
	if legal := g.Legal(2); len(legal) != 1 || legal[0].Index != miaMia {
		t.Fatalf("legal after accepting Mia = %+v", legal)
	}
	mustAct(t, g, 2, games.Action{Type: "declare", Index: miaMia})
	if g.Waiting()[0] != 0 {
		t.Fatalf("responder should wrap to seat 0, got %v", g.Waiting())
	}
}

func TestMiaElimination(t *testing.T) {
	g := newTestMia(3, 1)
	g.lives[1] = 1
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 31})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if !g.seats.Out[1] || g.Over() {
		t.Fatalf("seat 1 should be out but the game goes on: out=%v over=%v", g.seats.Out, g.Over())
	}
	// Seat 0 rolls; seat 2 (skipping seat 1) responds.
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 65})
	if g.Waiting()[0] != 2 {
		t.Fatalf("waiting = %v, want 2", g.Waiting())
	}
	g.lives[2] = 1
	g.dice = []int{1, 3}
	mustAct(t, g, 2, games.Action{Type: "challenge"})
	if g.lives[0] != 2 {
		t.Fatalf("lives=%v", g.lives)
	}
	g.lives[0] = 3
	g.cur, g.phase = 2, "roll"
	mustAct(t, g, 2, games.Action{Type: "roll"})
	g.dice = []int{1, 3}
	mustAct(t, g, 2, games.Action{Type: "declare", Index: 66})
	mustAct(t, g, 0, games.Action{Type: "challenge"})
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatalf("seat 0 should win: lives=%v", g.lives)
	}
}

func TestMiaHiddenDice(t *testing.T) {
	g := newTestMia()
	mustAct(t, g, 0, games.Action{Type: "roll"})
	g.dice = []int{87, 93} // sentinels that can't appear anywhere else
	for _, seat := range []int{1, 2, -1} {
		assertHidden(t, g, seat, 87, 93)
	}
	assertVisible(t, g, 0, 87, 93)
	if v := g.View(1).(miaView); v.CupDice != 2 {
		t.Errorf("others should see that two dice are under the cup")
	}
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 43})
	for _, seat := range []int{1, 2, -1} {
		assertHidden(t, g, seat, 87, 93)
	}
	// Accepting rolls fresh dice for seat 1; seat 0 can't see them.
	mustAct(t, g, 1, games.Action{Type: "accept"})
	g.dice = []int{77, 78}
	assertHidden(t, g, 0, 77, 78, 87, 93)
	assertHidden(t, g, -1, 77, 78)
	assertVisible(t, g, 1, 77, 78)
	// A challenge reveals the dice to everyone.
	g.dice = []int{6, 6}
	mustAct(t, g, 1, games.Action{Type: "declare", Index: 54})
	mustAct(t, g, 2, games.Action{Type: "challenge"})
	for _, seat := range []int{0, 2, -1} {
		if v := g.View(seat).(miaView); v.Reveal == nil || v.Reveal.Real != 66 {
			t.Errorf("seat %d should see the reveal: %+v", seat, v.Reveal)
		}
	}
}

func TestMiaRemove(t *testing.T) {
	g := newTestMia()
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "declare", Index: 43})
	g.Remove(1) // the responder leaves: seat 2 responds instead
	if g.Waiting()[0] != 2 {
		t.Fatalf("waiting = %v, want 2", g.Waiting())
	}
	mustAct(t, g, 2, games.Action{Type: "accept"})
	g.Remove(2) // the holder leaves: seat 0 starts fresh and wins nothing yet
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatalf("seat 0 is the last one left: over=%v", g.Over())
	}

	g = newTestMia()
	g.seats = games.NewSeats(4)
	g.lives = []int{3, 3, 3, 3}
	mustAct(t, g, 0, games.Action{Type: "roll"})
	g.Remove(0)
	if g.cur != 1 || g.phase != "roll" || g.dice != nil {
		t.Fatalf("removing the roller should pass a fresh round to seat 1: cur=%d phase=%s", g.cur, g.phase)
	}
}
