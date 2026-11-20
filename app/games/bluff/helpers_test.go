package bluff

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func viewJSON(t *testing.T, g games.Game, seat int) string {
	t.Helper()
	b, err := json.Marshal(g.View(seat))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// assertHidden checks that none of the sentinel values appear in seat's view.
func assertHidden(t *testing.T, g games.Game, seat int, sentinels ...int) {
	t.Helper()
	js := viewJSON(t, g, seat)
	for _, s := range sentinels {
		if strings.Contains(js, itoa(s)) {
			t.Errorf("seat %d's view leaks hidden value %d: %s", seat, s, js)
		}
	}
}

func assertVisible(t *testing.T, g games.Game, seat int, sentinels ...int) {
	t.Helper()
	js := viewJSON(t, g, seat)
	for _, s := range sentinels {
		if !strings.Contains(js, itoa(s)) {
			t.Errorf("seat %d's view should show %d: %s", seat, s, js)
		}
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func mustAct(t *testing.T, g games.Game, seat int, a games.Action) {
	t.Helper()
	if err := g.Act(seat, a); err != nil {
		t.Fatalf("seat %d %+v: %v", seat, a, err)
	}
}

func lastLog(env *games.Env, n int) string {
	parts := []string{}
	for _, e := range env.Entries(n) {
		parts = append(parts, e.Text)
	}
	return strings.Join(parts, "\n")
}
