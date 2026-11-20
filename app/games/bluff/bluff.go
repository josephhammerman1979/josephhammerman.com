// Package bluff holds the bluffing dice games: Mia, Perudo and Liar's Poker
// Dice.  In all of them some dice are concealed, so each View only carries
// the dice that seat is entitled to see; reveals after a challenge are
// public.
package bluff

import (
	"fmt"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// P is the name token for seat.
func P(seat int) string { return games.P(seat) }

// plural returns "1 life" / "3 lives" style counts.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

var faceWords = [7][2]string{
	{}, {"one", "ones"}, {"two", "twos"}, {"three", "threes"},
	{"four", "fours"}, {"five", "fives"}, {"six", "sixes"},
}

// faceCount names a Perudo-style bid: "3 fours", "1 six".
func faceCount(count, face int) string {
	if face < 1 || face > 6 {
		return fmt.Sprintf("%d × %d", count, face)
	}
	if count == 1 {
		return "1 " + faceWords[face][0]
	}
	return fmt.Sprintf("%d %s", count, faceWords[face][1])
}

// optInt reads a numeric option, falling back to def.
func optInt(opts map[string]string, key string, def int) int {
	var v int
	if _, err := fmt.Sscan(opts[key], &v); err != nil || v <= 0 {
		return def
	}
	return v
}

// hearts draws n lives as hearts (or "out").
func hearts(n int) string {
	if n <= 0 {
		return "out"
	}
	s := ""
	for i := 0; i < n; i++ {
		s += "♥"
	}
	return s
}

func winPrompt(seat, winner int) string {
	if seat == winner {
		return "You win!"
	}
	return fmt.Sprintf("%s wins!", P(winner))
}
