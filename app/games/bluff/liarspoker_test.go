package bluff

import (
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestLiarsPokerPlaysToCompletion(t *testing.T) {
	for _, tokens := range []string{"3", "5"} {
		if err := games.SimulateMany(games.Lookup("liarspoker"), 5, map[string]string{"tokens": tokens}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLiarsPokerMinRaise(t *testing.T) {
	if len(allHands) != 252 {
		t.Fatalf("%d hands, want 252", len(allHands))
	}
	if got := lpMinRaise([]int{1, 1, 1, 1, 1}); games.EvalHand(got).Name != "Five 2s" {
		t.Errorf("min raise over five 1s = %v", got)
	}
	if lpMinRaise([]int{6, 6, 6, 6, 6}) != nil {
		t.Error("nothing beats five 6s")
	}
	got := lpMinRaise([]int{6, 5, 4, 3, 1}) // best high card
	if games.EvalHand(got).Cat != games.OnePair {
		t.Errorf("min raise over 6-high = %v (%s)", got, games.EvalHand(got).Name)
	}
}

func newTestLP(faces ...int) *liarsPoker {
	return newLiarsPoker(games.NewTestEnv(faces...), 3, 3)
}

func TestLiarsPokerChallengeBluff(t *testing.T) {
	g := newTestLP(2, 2, 3, 5, 6)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{3, 3, 3, 6, 6}})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if g.tokens[0] != 2 || g.tokens[1] != 3 {
		t.Fatalf("bluffer should lose a token: %v", g.tokens)
	}
	if g.phase != "roll" || g.holder != 1 || g.bid != nil {
		t.Fatalf("challenger starts the next round: phase=%s holder=%d", g.phase, g.holder)
	}
	if !strings.Contains(lastLog(g.env, 6), "had 2 2 3 5 6 (Pair of 2s) — the bid of Full house, 3s over 6s was a bluff") {
		t.Errorf("log:\n%s", lastLog(g.env, 6))
	}
}

func TestLiarsPokerChallengeTruth(t *testing.T) {
	g := newTestLP(4, 4, 4, 2, 2)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{4, 4, 4, 2, 1}})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if g.tokens[1] != 2 || g.holder != 0 || !g.reveal.Truthful {
		t.Fatalf("challenger should lose: tokens=%v holder=%d", g.tokens, g.holder)
	}
}

func TestLiarsPokerAcceptArrangeReroll(t *testing.T) {
	g := newTestLP(6, 6, 2, 3, 4 /* reroll in view: */, 6 /* reroll in cup: */, 1)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{6, 6, 1, 2, 3}})
	if err := g.Act(2, games.Action{Type: "accept"}); err == nil {
		t.Fatal("only the next player may respond")
	}
	mustAct(t, g, 1, games.Action{Type: "accept"})
	if g.holder != 1 || g.phase != "arrange" {
		t.Fatalf("accepter takes the cup: holder=%d phase=%s", g.holder, g.phase)
	}
	mustAct(t, g, 1, games.Action{Type: "arrange", Dice: []int{0, 1, 2}})
	if err := g.Act(1, games.Action{Type: "reroll", Dice: []int{2, 3}}); err == nil {
		t.Fatal("re-rolling from both the cup and view must fail")
	}
	mustAct(t, g, 1, games.Action{Type: "reroll", Dice: []int{2}})
	if g.dice[2] != 6 || !strings.Contains(lastLog(g.env, 1), "in plain view: 6") {
		t.Fatalf("dice=%v log=%s", g.dice, lastLog(g.env, 1))
	}
	if err := g.Act(1, games.Action{Type: "bid", Dice: []int{6, 6, 1, 2, 3}}); err == nil {
		t.Fatal("an equal bid must be rejected")
	}
	if err := g.Act(1, games.Action{Type: "bid", Dice: []int{6, 6, 1, 2}}); err == nil {
		t.Fatal("a four-die bid must be rejected")
	}
	if s := g.suggestion(); games.EvalHand(s).Name != "Three 6s" {
		t.Errorf("suggestion should be the real hand: %v", s)
	}
	mustAct(t, g, 1, games.Action{Type: "bid", Dice: []int{6, 6, 6, 4, 3}})
	mustAct(t, g, 2, games.Action{Type: "accept"})
	mustAct(t, g, 2, games.Action{Type: "arrange", Dice: []int{0, 1, 2}})
	mustAct(t, g, 2, games.Action{Type: "reroll", Dice: []int{3, 4}})
	if !strings.Contains(lastLog(g.env, 1), "re-rolled 2 dice in the cup.") {
		t.Errorf("cup re-roll log: %s", lastLog(g.env, 1))
	}
}

func TestLiarsPokerFiveSixesMustBeChallenged(t *testing.T) {
	g := newTestLP(1, 2, 3, 4, 6)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{6, 6, 6, 6, 6}})
	if err := g.Act(1, games.Action{Type: "accept"}); err == nil {
		t.Fatal("five 6s can't be accepted")
	}
	if legal := g.Legal(1); len(legal) != 1 || legal[0].Type != "challenge" {
		t.Fatalf("legal = %+v", legal)
	}
}

func TestLiarsPokerElimination(t *testing.T) {
	g := newLiarsPoker(games.NewTestEnv(1, 2, 3, 4, 6), 2, 3)
	g.tokens[0] = 1
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{5, 5, 1, 2, 3}})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if !g.Over() || g.Winners()[0] != 1 {
		t.Fatalf("seat 1 should win: tokens=%v", g.tokens)
	}
}

func TestLiarsPokerHiddenDice(t *testing.T) {
	g := newTestLP()
	mustAct(t, g, 0, games.Action{Type: "roll"})
	g.dice = []int{71, 72, 73, 74, 75}
	for _, seat := range []int{1, 2, -1} {
		assertHidden(t, g, seat, 71, 72, 73, 74, 75)
		v := g.View(seat).(lpView)
		if v.CanSee || len(v.Dice) != 5 {
			t.Errorf("seat %d: canSee=%v dice=%v", seat, v.CanSee, v.Dice)
		}
	}
	assertVisible(t, g, 0, 71, 75)
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{1, 1, 2, 3, 4}})
	mustAct(t, g, 1, games.Action{Type: "accept"})
	// Seat 1 now sees everything; seat 0 no longer sees the cup.
	assertVisible(t, g, 1, 71, 72, 73, 74, 75)
	assertHidden(t, g, 0, 71, 72, 73, 74, 75)
	mustAct(t, g, 1, games.Action{Type: "arrange", Dice: []int{1, 3}})
	// Dice in plain view are public; the cup stays hidden.
	for _, seat := range []int{0, 2, -1} {
		assertVisible(t, g, seat, 72, 74)
		assertHidden(t, g, seat, 71, 73, 75)
	}
	mustAct(t, g, 1, games.Action{Type: "reroll", Dice: []int{}})
	g.dice = []int{6, 6, 6, 6, 1} // keep the hand evaluable for the challenge
	g.inView = []bool{false, true, false, true, false}
	mustAct(t, g, 1, games.Action{Type: "bid", Dice: []int{6, 6, 6, 6, 2}})
	mustAct(t, g, 2, games.Action{Type: "challenge"})
	for _, seat := range []int{0, -1} {
		v := g.View(seat).(lpView)
		if v.Reveal == nil || games.DiceText(v.Reveal.Dice) != "6 6 6 6 1" {
			t.Errorf("seat %d should see the reveal: %+v", seat, v.Reveal)
		}
	}
}

func TestLiarsPokerRemove(t *testing.T) {
	g := newLiarsPoker(games.NewTestEnv(), 4, 3)
	mustAct(t, g, 0, games.Action{Type: "roll"})
	mustAct(t, g, 0, games.Action{Type: "bid", Dice: []int{2, 2, 3, 4, 5}})
	g.Remove(1) // responder leaves: seat 2 responds
	if g.Waiting()[0] != 2 {
		t.Fatalf("waiting = %v", g.Waiting())
	}
	mustAct(t, g, 2, games.Action{Type: "accept"})
	g.Remove(2) // holder leaves: fresh round for seat 3
	if g.holder != 3 || g.phase != "roll" || g.bid != nil {
		t.Fatalf("holder=%d phase=%s bid=%v", g.holder, g.phase, g.bid)
	}
	g.Remove(3)
	if !g.Over() || g.Winners()[0] != 0 {
		t.Fatal("seat 0 should win")
	}
}
