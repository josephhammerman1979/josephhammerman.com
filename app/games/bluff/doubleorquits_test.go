package bluff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestDoubleOrQuitsPlaysToCompletion(t *testing.T) {
	for _, rounds := range []string{"8", "5", "12"} {
		for _, chips := range []string{"20", "50"} {
			opts := map[string]string{"rounds": rounds, "chips": chips}
			if err := games.SimulateMany(games.Lookup("doubleorquits"), 5, opts); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func doqAct(t *testing.T, g *doq, seat int, a games.Action) {
	t.Helper()
	if err := g.Act(seat, a); err != nil {
		t.Fatalf("seat %d %+v: %v", seat, a, err)
	}
}

var fullHouse = []int{3, 3, 3, 6, 6}

func TestDoqHonestClaimChallengedChallengerDoubles(t *testing.T) {
	g := newDoq(games.NewTestEnv(3, 3, 3, 6, 6), 3, 8, 20) // pot 3
	doqAct(t, g, 0, games.Action{Type: "claim", Dice: fullHouse})
	doqAct(t, g, 1, games.Action{Type: "challenge"})
	// Seat 1 pays 3 (pot 6), seat 0 takes 6: 19-3=16 for seat 1, 19+6=25 for
	// seat 0.  Then round 2 starts with a fresh ante of 1 each.
	if g.chips[0] != 24 || g.chips[1] != 15 || g.chips[2] != 18 || g.pot != 3 {
		t.Fatalf("chips=%v pot=%d", g.chips, g.pot)
	}
	if g.round != 2 {
		t.Fatalf("round=%d, want 2", g.round)
	}
}

func TestDoqLieCaughtClaimantDoublesAndPotCarries(t *testing.T) {
	g := newDoq(games.NewTestEnv(1, 2, 4, 5, 6), 3, 8, 20) // pot 3, a 6-high bust hand
	doqAct(t, g, 0, games.Action{Type: "claim", Dice: fullHouse})
	doqAct(t, g, 1, games.Action{Type: "believe"})
	doqAct(t, g, 2, games.Action{Type: "challenge"})
	if g.chips[0] != 16 || g.pot != 6 || g.round != 2 {
		t.Fatalf("chips=%v pot=%d round=%d, want claimant 16, pot 6, round 2", g.chips, g.pot, g.round)
	}
	if g.chips[2] != 19 {
		t.Fatalf("no new ante on a carried pot: chips=%v", g.chips)
	}
}

func TestDoqBelievedClaimMustBeBeatenOrPassed(t *testing.T) {
	g := newDoq(games.NewTestEnv(3, 3, 3, 6, 6, 1, 1, 2, 3, 4), 3, 8, 20)
	doqAct(t, g, 0, games.Action{Type: "claim", Dice: fullHouse})
	doqAct(t, g, 1, games.Action{Type: "believe"})
	doqAct(t, g, 2, games.Action{Type: "believe"})
	if g.established == nil || g.thrower != 1 || g.claim != nil {
		t.Fatalf("after everyone believed: established=%v thrower=%d", g.established, g.thrower)
	}
	if err := g.Act(1, games.Action{Type: "claim", Dice: []int{6, 6, 5, 5, 1}}); err == nil {
		t.Fatal("a lower claim should be rejected")
	}
	doqAct(t, g, 1, games.Action{Type: "pass"})
	if !g.passed[1] || g.thrower != 2 {
		t.Fatalf("after pass: passed=%v thrower=%d", g.passed, g.thrower)
	}
	// Seat 2 passes too: seat 0's standing claim takes the pot.
	doqAct(t, g, 2, games.Action{Type: "pass"})
	if g.chips[0] != 19+3-1 || g.round != 2 { // took the pot of 3, then anted for round 2
		t.Fatalf("chips=%v round=%d", g.chips, g.round)
	}
}

func TestDoqPassedPlayersCantChallenge(t *testing.T) {
	g := newDoq(games.NewTestEnv(3, 3, 3, 6, 6, 1, 1, 2, 3, 4, 5, 5, 5, 5, 2), 3, 8, 20)
	doqAct(t, g, 0, games.Action{Type: "claim", Dice: fullHouse})
	doqAct(t, g, 1, games.Action{Type: "believe"})
	doqAct(t, g, 2, games.Action{Type: "believe"})
	doqAct(t, g, 1, games.Action{Type: "pass"}) // seat 1 is out of the round
	doqAct(t, g, 2, games.Action{Type: "claim", Dice: []int{5, 5, 5, 5, 2}})
	if g.responder != 0 {
		t.Fatalf("responder=%d, want 0 (seat 1 passed)", g.responder)
	}
}

func TestDoqFirstThrowerCantPass(t *testing.T) {
	g := newDoq(games.NewTestEnv(), 2, 8, 20)
	if err := g.Act(0, games.Action{Type: "pass"}); err == nil {
		t.Fatal("the opening thrower must claim, not pass")
	}
}

func TestDoqRedraw(t *testing.T) {
	g := newDoq(games.NewTestEnv(6, 6, 1, 2, 3, 4, 5, 2), 2, 8, 20)
	// Expose the two 6s and re-roll the three hidden dice.
	doqAct(t, g, 0, games.Action{Type: "draw", Dice: []int{0, 1}, Index: 1})
	if g.dice[0] != 6 || g.dice[2] != 4 || g.dice[4] != 2 || !g.exposed[0] || g.exposed[2] {
		t.Fatalf("dice=%v exposed=%v", g.dice, g.exposed)
	}
	if err := g.Act(0, games.Action{Type: "draw", Index: 1}); err == nil {
		t.Fatal("a second redraw should be rejected")
	}
}

func TestDoqHidesUnexposedDice(t *testing.T) {
	g := newDoq(games.NewTestEnv(), 3, 8, 20)
	g.dice = []int{81, 82, 83, 84, 85}
	g.exposed = []bool{true, false, false, false, false}
	for _, seat := range []int{1, 2, -1} {
		b, _ := json.Marshal(g.View(seat))
		for _, secret := range []string{"82", "83", "84", "85"} {
			if strings.Contains(string(b), secret) {
				t.Fatalf("seat %d sees a hidden die: %s", seat, b)
			}
		}
		if !strings.Contains(string(b), "81") {
			t.Fatalf("seat %d can't see the exposed die", seat)
		}
	}
	b, _ := json.Marshal(g.View(0))
	if !strings.Contains(string(b), "85") {
		t.Fatal("the thrower can't see their own dice")
	}
}
