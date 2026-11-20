// Package boards holds the board and table games: Banker, Hundreds, Three
// Hands and Rule Maker.  Each registers itself with package games from an
// init function.
package boards

import (
	"fmt"
	"strings"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

func choice(value, label string) games.Choice {
	return games.Choice{Value: value, Label: label}
}

// seatList formats seats as "{P0}, {P2} and {P3}".
func seatList(seats []int) string {
	names := make([]string, len(seats))
	for i, s := range seats {
		names[i] = games.P(s)
	}
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
