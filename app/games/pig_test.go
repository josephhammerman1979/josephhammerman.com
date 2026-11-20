package games

import "testing"

func TestBigPigRules(t *testing.T) {
	// Seat 0: 3+4 (7), then 1+1 (+25 → 32, keeps rolling), then 1+5 bust.
	g := newPig(NewTestEnv(3, 4, 1, 1, 1, 5), 2, 2)
	g.Act(0, Action{Type: "roll"})
	g.Act(0, Action{Type: "roll"})
	if g.turnTotal != 32 || g.cur != 0 {
		t.Fatalf("after double ones: turn=%d cur=%d, want 32, 0", g.turnTotal, g.cur)
	}
	g.Act(0, Action{Type: "roll"})
	if g.cur != 1 || g.totals[0] != 0 {
		t.Fatalf("after bust: cur=%d total=%d, want 1, 0", g.cur, g.totals[0])
	}
}

func TestPigHoldAndWin(t *testing.T) {
	g := newPig(NewTestEnv(6), 2, 1)
	if err := g.Act(0, Action{Type: "hold"}); err == nil {
		t.Fatal("hold before rolling should be rejected")
	}
	g.totals[0] = 95
	g.Act(0, Action{Type: "roll"})
	g.Act(0, Action{Type: "hold"})
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatalf("expected seat 0 to win, totals=%v", g.totals)
	}
	if err := g.Act(1, Action{Type: "roll"}); err == nil {
		t.Fatal("moves after the game ended should be rejected")
	}
}

func TestPigRemoveCurrentPlayerPassesTurn(t *testing.T) {
	g := newPig(NewTestEnv(4), 3, 1)
	g.Act(0, Action{Type: "roll"})
	g.Remove(0)
	if g.cur != 1 || g.turnTotal != 0 {
		t.Fatalf("cur=%d turn=%d after removing current player", g.cur, g.turnTotal)
	}
	g.cur = 2
	g.nextTurn()
	if g.cur != 1 {
		t.Fatalf("removed seat 0 was not skipped; cur=%d", g.cur)
	}
}

func TestPigGamesPlayToCompletion(t *testing.T) {
	for _, id := range []string{"pig", "bigpig"} {
		if err := SimulateMany(Lookup(id), 5, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPokerHandRanking(t *testing.T) {
	order := [][]int{
		{6, 5, 3, 2, 1}, // 6 high
		{2, 2, 6, 4, 3}, // pair
		{3, 3, 2, 2, 6}, // two pair
		{4, 4, 4, 1, 2}, // three of a kind
		{2, 2, 2, 5, 5}, // full house
		{1, 2, 3, 4, 5}, // low straight
		{2, 3, 4, 5, 6}, // high straight
		{1, 1, 1, 1, 6}, // four of a kind
		{2, 2, 2, 2, 2}, // five of a kind
	}
	for i := 1; i < len(order); i++ {
		a, b := EvalHand(order[i]), EvalHand(order[i-1])
		if CompareHands(a, b) <= 0 {
			t.Errorf("%s (%v) should beat %s (%v)", a.Name, order[i], b.Name, order[i-1])
		}
	}
	if CompareHands(EvalHand([]int{5, 5, 1, 2, 3}), EvalHand([]int{5, 5, 1, 2, 4})) >= 0 {
		t.Error("kicker should break a pair tie")
	}
	if got := EvalHand([]int{3, 3, 3, 6, 6}).Name; got != "Full house, 3s over 6s" {
		t.Errorf("name = %q", got)
	}
	three := [][]int{{6, 4, 1}, {2, 2, 5}, {3, 4, 5}, {1, 1, 1}}
	for i := 1; i < len(three); i++ {
		if CompareHands(EvalHand(three[i]), EvalHand(three[i-1])) <= 0 {
			t.Errorf("3-dice: %v should beat %v", three[i], three[i-1])
		}
	}
}
