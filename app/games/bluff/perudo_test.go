package bluff

import (
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestPerudoPlaysToCompletion(t *testing.T) {
	for _, wild := range []string{"no", "yes"} {
		if err := games.SimulateMany(games.Lookup("perudo"), 5, map[string]string{"wild": wild}); err != nil {
			t.Fatal(err)
		}
	}
}

// newTestPerudo deals fixed dice to each seat.
func newTestPerudo(wild bool, dice ...[]int) *perudo {
	g := newPerudo(games.NewTestEnv(), len(dice), wild)
	for i, d := range dice {
		g.dice[i] = append([]int{}, d...)
	}
	return g
}

func TestPerudoRaises(t *testing.T) {
	g := newTestPerudo(false, []int{1, 2, 3, 4, 5}, []int{6, 6, 6, 6, 6})
	if err := g.Act(0, games.Action{Type: "challenge"}); err == nil {
		t.Fatal("can't challenge before a bid")
	}
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 3, Face: 4})
	for _, bad := range [][2]int{{3, 4}, {3, 2}, {2, 6}, {11, 6}, {4, 7}} {
		if err := g.Act(1, games.Action{Type: "bid", Count: bad[0], Face: bad[1]}); err == nil {
			t.Errorf("bid %v should be rejected after 3 fours", bad)
		}
	}
	mustAct(t, g, 1, games.Action{Type: "bid", Count: 3, Face: 5})
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 4, Face: 1})
	if mr := g.minRaise(); mr.Count != 4 || mr.Face != 2 {
		t.Errorf("min raise = %+v", mr)
	}
	g.bid = &perudoBid{Count: 10, Face: 6, By: 0}
	if g.minRaise() != nil {
		t.Error("10 sixes with 10 dice can't be raised")
	}
	for _, a := range g.Legal(1) {
		if a.Type == "bid" {
			t.Errorf("no raise should be listed: %+v", a)
		}
	}
}

func TestPerudoChallenge(t *testing.T) {
	g := newTestPerudo(false, []int{4, 4, 1, 2, 3}, []int{4, 5, 6, 6, 1})
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 4, Face: 4}) // only 3 fours
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if len(g.dice[0]) != 4 || len(g.dice[1]) != 5 {
		t.Fatalf("bidder should lose a die: %v", g.dice)
	}
	if g.cur != 0 || g.bid != nil || g.round != 2 {
		t.Fatalf("loser opens a new round: cur=%d bid=%v round=%d", g.cur, g.bid, g.round)
	}
	if g.reveal == nil || g.reveal.Actual != 3 || g.reveal.Loser != 0 {
		t.Fatalf("reveal = %+v", g.reveal)
	}
	if !strings.Contains(lastLog(g.env, 6), "only 3 — the bid of 4 fours was a bluff") {
		t.Errorf("log:\n%s", lastLog(g.env, 6))
	}
}

func TestPerudoWildOnes(t *testing.T) {
	g := newTestPerudo(true, []int{4, 4, 1, 2, 3}, []int{4, 5, 6, 6, 1})
	if err := g.Act(0, games.Action{Type: "bid", Count: 2, Face: 1}); err == nil {
		t.Fatal("bidding ones should be refused when they're wild")
	}
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 5, Face: 4}) // 3 fours + 2 wild ones
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if len(g.dice[1]) != 4 || len(g.dice[0]) != 5 {
		t.Fatalf("challenger should lose a die with wild ones counted: %v", g.dice)
	}
	if g.cur != 1 {
		t.Fatalf("challenger lost, so they open: cur=%d", g.cur)
	}
}

func TestPerudoSpotOn(t *testing.T) {
	g := newTestPerudo(false, []int{6, 6, 2}, []int{6, 1, 2, 3, 4})
	g.seats = games.NewSeats(2)
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 3, Face: 6})
	mustAct(t, g, 1, games.Action{Type: "spoton"}) // exactly 3 sixes, but seat 1 already has 5
	if len(g.dice[1]) != 5 || g.cur != 1 {
		t.Fatalf("spot on at five dice: dice=%v cur=%d", g.dice, g.cur)
	}

	g = newTestPerudo(false, []int{6, 6, 2, 2, 2}, []int{6, 1, 2})
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 3, Face: 6})
	mustAct(t, g, 1, games.Action{Type: "spoton"})
	if len(g.dice[1]) != 4 || g.cur != 1 || g.reveal.Gainer != 1 {
		t.Fatalf("successful spot on should gain a die and open: dice=%v cur=%d", g.dice, g.cur)
	}

	g = newTestPerudo(false, []int{6, 6, 2, 2, 2}, []int{6, 1, 2})
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 2, Face: 6})
	mustAct(t, g, 1, games.Action{Type: "spoton"})
	if len(g.dice[1]) != 2 || g.reveal.Loser != 1 {
		t.Fatalf("wrong spot on should cost the caller a die: dice=%v", g.dice)
	}
}

func TestPerudoEliminationAndWin(t *testing.T) {
	g := newTestPerudo(false, []int{2}, []int{3, 3}, []int{4, 4})
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 2, Face: 2})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	if !g.seats.Out[0] || g.cur != 1 || g.totalDice() != 4 {
		t.Fatalf("seat 0 should be out, seat 1 opens: out=%v cur=%d total=%d", g.seats.Out, g.cur, g.totalDice())
	}
	g.dice[1], g.dice[2] = []int{3}, []int{4}
	mustAct(t, g, 1, games.Action{Type: "bid", Count: 2, Face: 3})
	mustAct(t, g, 2, games.Action{Type: "challenge"})
	if !g.Over() || g.Winners()[0] != 2 {
		t.Fatalf("seat 2 should win: dice=%v", g.dice)
	}
}

func TestPerudoHiddenDice(t *testing.T) {
	g := newTestPerudo(false, []int{1, 2, 3, 4, 5}, []int{71, 72, 73, 74, 75}, []int{81, 82, 83})
	for _, seat := range []int{0, 2, -1} {
		assertHidden(t, g, seat, 71, 72, 73, 74, 75)
	}
	for _, seat := range []int{0, 1, -1} {
		assertHidden(t, g, seat, 81, 82, 83)
	}
	assertVisible(t, g, 1, 71, 75)
	v := g.View(-1).(perudoView)
	if v.MyDice != nil || v.DiceCounts[2] != 3 || v.TotalDice != 13 {
		t.Errorf("spectator view: %+v", v)
	}
	// After a challenge every cup is lifted, for everyone.
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 1, Face: 2})
	mustAct(t, g, 1, games.Action{Type: "challenge"})
	assertVisible(t, g, -1, 71, 72, 73, 74, 75, 81, 82, 83)
	assertVisible(t, g, 0, 71, 81)
	// ...but the new round's dice are hidden again.
	g.dice[2] = []int{91, 92, 93}
	assertHidden(t, g, 0, 91, 92, 93)
	assertHidden(t, g, -1, 91, 92, 93)
}

func TestPerudoRemoveRestartsRound(t *testing.T) {
	g := newTestPerudo(false, []int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5}, []int{1, 2, 3, 4, 5})
	mustAct(t, g, 0, games.Action{Type: "bid", Count: 3, Face: 2})
	g.Remove(0) // the bidder leaves while seat 1 is deciding
	if g.bid != nil || g.cur != 1 || g.round != 2 || g.totalDice() != 10 {
		t.Fatalf("new round expected: bid=%v cur=%d round=%d total=%d", g.bid, g.cur, g.round, g.totalDice())
	}
	g.Remove(1) // the opener leaves: seat 2 is the last one
	if !g.Over() || g.Winners()[0] != 2 {
		t.Fatal("seat 2 should win")
	}
}
