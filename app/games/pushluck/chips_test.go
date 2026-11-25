package pushluck

import (
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestPotGamesFinishWithBustsAndRebuys(t *testing.T) {
	for _, id := range []string{"threes", "midnight"} {
		def := games.Lookup(id)
		for seed := int64(0); seed < 20; seed++ {
			// Tiny stacks force busts, all-ins and rebuys.
			err := games.Simulate(def, games.SimOptions{Players: 3, Seed: seed, Options: map[string]string{"chips": "20"}})
			if err != nil {
				t.Fatalf("%s seed %d: %v", id, seed, err)
			}
		}
	}
}

func TestThreesAllInTieReplayCapsWinnings(t *testing.T) {
	// Seats 0 and 1 tie on 0 (all 3s); seat 2 scores high.  Seat 0 has only
	// the 1 chip it antes, so it's all-in for the replay re-ante.
	env := games.NewTestEnv(
		3, 3, 3, 3, 3, // seat 0: 0
		3, 3, 3, 3, 3, // seat 1: 0
		6, 6, 6, 6, 5, // seat 2: 29
		3, 3, 3, 3, 3, // replay seat 0: 0
		6, 6, 6, 6, 6, // replay seat 1: moon!
	)
	g := newThrees(env, 3, 5, nil)
	g.ct.St.Chips[0] = 0 // had 1, anted it
	for _, s := range []int{0, 1, 2} {
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(5))
	}
	if g.replay != 1 || !g.ct.Pot.AllIn[0] {
		t.Fatalf("replay=%d allIn=%v, want seat 0 all-in for the replay", g.replay, g.ct.Pot.AllIn)
	}
	mustAct(t, g, 0, roll())
	mustAct(t, g, 0, keepAll(5))
	mustAct(t, g, 1, roll())
	mustAct(t, g, 1, keepAll(5))
	// Seat 1 shot the moon and isn't all-in: it takes the whole pot.
	for i, c := range g.ct.St.Chips {
		if c < 0 {
			t.Fatalf("seat %d went negative: %v", i, g.ct.St.Chips)
		}
	}
	// Pot: three antes, plus seat 1's replay ante (seat 0 had nothing left).
	if g.ct.St.Chips[1] != 20-1-1+4 || g.ct.St.Chips[0] != 0 || g.ct.Pot.Total() != 0 {
		t.Fatalf("chips=%v pot=%d, want seat 1 to have taken the pot of 4", g.ct.St.Chips, g.ct.Pot.Total())
	}
}

func TestThreesBustedPlayerRebuyOrLeave(t *testing.T) {
	env := games.NewTestEnv(
		6, 6, 6, 6, 5, // seat 0: 29
		3, 3, 3, 3, 3, // seat 1: 0, wins
	)
	g := newThrees(env, 2, 5, nil)
	g.ct.St.Chips[0] = 0 // seat 0 anted its last chip
	for _, s := range []int{0, 1} {
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(5))
	}
	if w := g.Waiting(); len(w) != 1 || w[0] != 0 {
		t.Fatalf("waiting %v, want the busted seat 0", w)
	}
	if err := g.Act(1, roll()); err == nil {
		t.Fatal("the next hand should wait for the rebuy decision")
	}
	mustAct(t, g, 0, games.Action{Type: "leave"})
	if !g.Over() || g.Winners()[0] != 1 {
		t.Fatalf("over=%v winners=%v: with one funded player left the game should end", g.Over(), g.Winners())
	}
}

func TestMidnightRebuyStartsNextRound(t *testing.T) {
	g := newMidnight(games.NewTestEnv(
		1, 4, 6, 6, 6, 6,
		2, 2, 2, 2, 2, 2,
		2, 2, 2, 2, 2, 2,
	), 3, 3, nil)
	order := g.Order
	g.ct.St.Chips[order[1]] = 0 // anted their last chip
	for _, s := range order {
		mustAct(t, g, s, roll())
		mustAct(t, g, s, keepAll(6))
	}
	if !g.ct.Rebuy[order[1]] || g.round != 1 {
		t.Fatalf("rebuy=%v round=%d, want %d deciding before round 2", g.ct.Rebuy, g.round, order[1])
	}
	mustAct(t, g, order[1], games.Action{Type: "rebuy"})
	if g.round != 2 || g.ct.St.BuyIns[order[1]] != 2 || g.ct.St.Chips[order[1]] != 19 {
		t.Fatalf("round=%d buyIns=%d chips=%d", g.round, g.ct.St.BuyIns[order[1]], g.ct.St.Chips[order[1]])
	}
}
