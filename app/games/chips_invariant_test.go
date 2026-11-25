package games_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/bluff"
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/boards"
	_ "github.com/josephhammerman1979/josephhammerman.com/app/games/pushluck"
)

// negativeChips walks a view looking for any "chips" value below zero.
func negativeChips(v interface{}, path string) string {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, c := range x {
			if k == "chips" {
				switch n := c.(type) {
				case float64:
					if n < 0 {
						return path + ".chips"
					}
				case []interface{}:
					for i, e := range n {
						if f, ok := e.(float64); ok && f < 0 {
							return fmt.Sprintf("%s.chips[%d]", path, i)
						}
					}
				}
			}
			if p := negativeChips(c, path+"."+k); p != "" {
				return p
			}
		}
	case []interface{}:
		for i, c := range x {
			if p := negativeChips(c, fmt.Sprintf("%s[%d]", path, i)); p != "" {
				return p
			}
		}
	}
	return ""
}

// TestChipsNeverGoNegative plays the betting games at random and checks
// every player's view after every move.
func TestChipsNeverGoNegative(t *testing.T) {
	for _, id := range []string{"klondike", "doubleorquits", "threes", "midnight"} {
		def := games.Lookup(id)
		for seed := int64(0); seed < 25; seed++ {
			r := rand.New(rand.NewSource(seed))
			n := def.MinPlayers + int(seed)%(def.MaxPlayers-def.MinPlayers+1)
			roster := make([]string, n)
			for i := range roster {
				roster[i] = fmt.Sprintf("p%d", i)
			}
			opts := map[string]string{"chips": "20", "variant": []string{"classic", "goldrush"}[seed%2], "rounds": "12", "hands": "8", "banks": "2"}
			tb, err := games.NewTable(def, roster, roster[0], opts, 1, rand.New(rand.NewSource(seed+7)), time.Unix(0, 0))
			if err != nil {
				t.Fatal(err)
			}
			for step := 0; step < 5000 && !tb.Over(); step++ {
				w := tb.Game.Waiting()
				seat := w[r.Intn(len(w))]
				legal := tb.Game.Legal(seat)
				if err := tb.Act(roster[seat], legal[r.Intn(len(legal))], time.Unix(0, 0)); err != nil {
					t.Fatalf("%s seed %d: %v", id, seed, err)
				}
				for s := 0; s < n; s++ {
					b, _ := json.Marshal(tb.Game.View(s))
					var v interface{}
					json.Unmarshal(b, &v)
					if p := negativeChips(v, "view"); p != "" {
						t.Fatalf("%s seed %d step %d: negative chips at %s: %s", id, seed, step, p, b)
					}
				}
			}
		}
	}
}
