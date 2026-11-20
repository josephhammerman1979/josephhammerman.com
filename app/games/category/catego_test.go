package category

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func TestCategoPlaysToCompletion(t *testing.T) {
	if err := games.SimulateMany(games.Lookup("catego"), 8, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCategoPlacementRules(t *testing.T) {
	g := newCatego(games.NewTestEnv(6, 6, 2, 3), 2)
	if err := g.Act(0, games.Action{Type: "place", Index: 12}); err == nil {
		t.Fatal("placing before rolling should be rejected")
	}
	g.Act(0, games.Action{Type: "roll"})
	if err := g.Act(0, games.Action{Type: "roll"}); err == nil {
		t.Fatal("rolling twice should be rejected")
	}
	if err := g.Act(0, games.Action{Type: "place", Index: 13}); err == nil {
		t.Fatal("column 13 should be rejected")
	}
	g.Act(0, games.Action{Type: "place", Index: 2}) // 12 in column 2
	if g.sheet[0][0] != 12 || g.cur != 1 {
		t.Fatalf("sheet=%v cur=%d", g.sheet[0], g.cur)
	}
	g.Act(1, games.Action{Type: "roll"})
	g.Act(1, games.Action{Type: "place", Index: 12}) // 5 in column 12
	if g.round != 2 || g.cur != 0 {
		t.Fatalf("round=%d cur=%d after both players placed", g.round, g.cur)
	}
	g.Act(0, games.Action{Type: "roll"})
	if err := g.Act(0, games.Action{Type: "place", Index: 2}); err == nil {
		t.Fatal("refilling a column should be rejected")
	}
}

func TestCategoScoringAndTies(t *testing.T) {
	g := newCatego(games.NewTestEnv(), 3)
	// Column 12: seat 1 has the unique high.  Column 11: seats 0 and 2 tie
	// at the top, so nobody wins it even though seat 1 wrote something.
	g.sheet[0][12-firstCol], g.sheet[1][12-firstCol], g.sheet[2][12-firstCol] = 7, 9, 3
	g.sheet[0][11-firstCol], g.sheet[1][11-firstCol], g.sheet[2][11-firstCol] = 8, 4, 8
	g.sheet[0][2-firstCol] = 2
	tot := g.totals()
	if tot[0] != 2 || tot[1] != 12 || tot[2] != 0 {
		t.Fatalf("totals = %v, want [2 12 0]", tot)
	}
}

func TestCategoEndsAfterElevenRounds(t *testing.T) {
	g := newCatego(games.NewTestEnv(), 2)
	for round := 0; round < numCols; round++ {
		for seat := 0; seat < 2; seat++ {
			g.Act(seat, games.Action{Type: "roll"})
			g.Act(seat, games.Action{Type: "place", Index: firstCol + round})
		}
	}
	if !g.Over() || len(g.Winners()) == 0 {
		t.Fatalf("over=%v winners=%v after 11 rounds", g.Over(), g.Winners())
	}
}

func TestCategoRemoveLastPlayerOfRoundAdvancesRound(t *testing.T) {
	g := newCatego(games.NewTestEnv(), 3)
	g.Act(0, games.Action{Type: "roll"})
	g.Act(0, games.Action{Type: "place", Index: 5})
	g.Act(1, games.Action{Type: "roll"})
	g.Act(1, games.Action{Type: "place", Index: 5})
	g.Remove(2) // seat 2 was up, and last in the round
	if g.round != 2 || g.cur != 0 {
		t.Fatalf("round=%d cur=%d", g.round, g.cur)
	}
	if strings.Contains(mustJSON(t, g.View(0)), `"over":true`) {
		t.Fatal("game ended early")
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
