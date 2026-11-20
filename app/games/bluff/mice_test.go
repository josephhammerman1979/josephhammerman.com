package bluff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestMicePlaysToCompletion(t *testing.T) {
	for _, limit := range []string{"20", "10", "30"} {
		if err := games.SimulateMany(games.Lookup("mice"), 5, map[string]string{"limit": limit}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMiceHandValue(t *testing.T) {
	if v := handValue([]int{1, 4, 6}); v != 641 {
		t.Fatalf("1 4 6 = %d, want 641", v)
	}
}

func act(t *testing.T, g *mice, seat int, a games.Action) {
	t.Helper()
	if err := g.Act(seat, a); err != nil {
		t.Fatalf("seat %d %+v: %v", seat, a, err)
	}
}

func TestMiceShowdownLowestPays(t *testing.T) {
	// Seat 0: 641, seat 1: 532, seat 2: 641 (ties don't matter at the top).
	g := newMice(games.NewTestEnv(1, 4, 6, 2, 3, 5, 6, 4, 1), 3, 20)
	act(t, g, 0, games.Action{Type: "open", Count: 2})
	act(t, g, 1, games.Action{Type: "join"})
	act(t, g, 2, games.Action{Type: "join"})
	if g.penalty[1] != 2 || g.penalty[0] != 0 || g.penalty[2] != 0 {
		t.Fatalf("penalties = %v, want [0 2 0]", g.penalty)
	}
	if g.round != 2 || g.starter != 1 {
		t.Fatalf("round=%d starter=%d, want 2, 1", g.round, g.starter)
	}
}

func TestMiceLeadMakesOthersAnswerAgain(t *testing.T) {
	g := newMice(games.NewTestEnv(6, 6, 6, 1, 1, 2, 3, 3, 3), 3, 20)
	act(t, g, 0, games.Action{Type: "open", Count: 1})
	act(t, g, 1, games.Action{Type: "join"})
	act(t, g, 2, games.Action{Type: "lead", Count: 4})
	if g.cur != 0 || g.round != 1 {
		t.Fatalf("after a lead, seat 0 must answer again; cur=%d round=%d", g.cur, g.round)
	}
	if err := g.Act(0, games.Action{Type: "lead", Count: 4}); err == nil {
		t.Fatal("leading without raising should be rejected")
	}
	act(t, g, 0, games.Action{Type: "join"})
	// Seat 1 runs; seats 0 (666) and 2 (333) have both agreed to 4, so it's
	// a showdown and seat 2 pays 4.
	act(t, g, 1, games.Action{Type: "run"})
	if g.penalty[1] != 3 || g.penalty[2] != 4 || g.penalty[0] != 0 || g.round != 2 {
		t.Fatalf("penalties=%v round=%d, want [0 3 4] and round 2", g.penalty, g.round)
	}
}

func TestMiceLastPlayerStandingTakesNothing(t *testing.T) {
	g := newMice(games.NewTestEnv(1, 1, 2, 6, 6, 5, 2, 2, 3), 3, 20)
	act(t, g, 0, games.Action{Type: "open", Count: 5})
	act(t, g, 1, games.Action{Type: "run"})
	act(t, g, 2, games.Action{Type: "run"})
	if g.penalty[0] != 0 || g.penalty[1] != 3 || g.penalty[2] != 3 {
		t.Fatalf("penalties = %v, want [0 3 3]", g.penalty)
	}
	if g.last == nil || g.last.LastStand != 0 || g.last.Shown[0] != nil {
		t.Fatalf("last standing player's dice must stay hidden: %+v", g.last)
	}
}

func TestMiceConcealsOtherPlayersDice(t *testing.T) {
	g := newMice(games.NewTestEnv(1, 1, 2, 6, 6, 5, 2, 2, 3), 3, 20)
	// Plant unmistakable values to search for.
	g.dice[0] = []int{71, 72, 73}
	for _, seat := range []int{1, 2, -1} {
		b, _ := json.Marshal(g.View(seat))
		for _, secret := range []string{"71", "72", "73", "737271"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("seat %d can see seat 0's dice: %s", seat, b)
			}
		}
	}
	b, _ := json.Marshal(g.View(0))
	if !strings.Contains(string(b), "71") {
		t.Fatal("owner can't see their own dice")
	}
}

func TestMiceEndsAtLimitFewestWins(t *testing.T) {
	g := newMice(games.NewTestEnv(1, 1, 2, 6, 6, 5), 2, 3)
	act(t, g, 0, games.Action{Type: "open", Count: 3})
	act(t, g, 1, games.Action{Type: "join"}) // seat 0 has 211, the lowest
	if !g.Over() || len(g.Winners()) != 1 || g.Winners()[0] != 1 {
		t.Fatalf("over=%v winners=%v penalties=%v", g.Over(), g.Winners(), g.penalty)
	}
}
