package games

import (
	"fmt"
	"sort"
)

// Poker-dice hand categories for five dice, lowest to highest, in the order
// given on the rules sheet (note that straights beat a full house).
const (
	HighCard = iota
	OnePair
	TwoPair
	ThreeKind
	FullHouse
	LowStraight  // 1 2 3 4 5
	HighStraight // 2 3 4 5 6
	FourKind
	FiveKind
)

// Three-dice hand categories (four/five of a kind and full houses are
// impossible; a straight is three consecutive faces).
const (
	High3 = iota
	Pair3
	Straight3
	Trips3
)

// Hand is an evaluated poker-dice hand.  Hands of the same number of dice
// compare by Cat, then Key.
type Hand struct {
	Cat  int    `json:"cat"`
	Key  []int  `json:"key"`
	Name string `json:"name"`
}

func plural(face int) string { return fmt.Sprintf("%ds", face) }

// EvalHand ranks 3 or 5 dice.  Other sizes are ranked by sorted faces only.
func EvalHand(d []int) Hand {
	c := Counts(d)
	// Faces grouped by count (desc) then face (desc): the tie-break key.
	faces := []int{}
	for f := 6; f >= 1; f-- {
		if c[f] > 0 {
			faces = append(faces, f)
		}
	}
	sort.SliceStable(faces, func(i, j int) bool { return c[faces[i]] > c[faces[j]] })
	key := []int{}
	for _, f := range faces {
		for k := 0; k < c[f]; k++ {
			key = append(key, f)
		}
	}
	top := 0
	if len(faces) > 0 {
		top = c[faces[0]]
	}
	second := 0
	if len(faces) > 1 {
		second = c[faces[1]]
	}

	switch len(d) {
	case 3:
		sorted := Ints(d)
		sort.Ints(sorted)
		switch {
		case top == 3:
			return Hand{Trips3, key, "Three " + plural(faces[0])}
		case top == 1 && sorted[1] == sorted[0]+1 && sorted[2] == sorted[1]+1:
			return Hand{Straight3, []int{sorted[2]}, fmt.Sprintf("Straight to %d", sorted[2])}
		case top == 2:
			return Hand{Pair3, key, "Pair of " + plural(faces[0])}
		}
		return Hand{High3, key, fmt.Sprintf("%d high", key[0])}
	case 5:
		switch {
		case top == 5:
			return Hand{FiveKind, key, "Five " + plural(faces[0])}
		case top == 4:
			return Hand{FourKind, key, "Four " + plural(faces[0])}
		case top == 1 && c[1] == 0:
			return Hand{HighStraight, key, "High straight"}
		case top == 1 && c[6] == 0:
			return Hand{LowStraight, key, "Low straight"}
		case top == 3 && second == 2:
			return Hand{FullHouse, key, fmt.Sprintf("Full house, %s over %s", plural(faces[0]), plural(faces[1]))}
		case top == 3:
			return Hand{ThreeKind, key, "Three " + plural(faces[0])}
		case top == 2 && second == 2:
			return Hand{TwoPair, key, fmt.Sprintf("Two pair, %s and %s", plural(faces[0]), plural(faces[1]))}
		case top == 2:
			return Hand{OnePair, key, "Pair of " + plural(faces[0])}
		}
		return Hand{HighCard, key, fmt.Sprintf("%d high", key[0])}
	}
	sorted := Ints(d)
	sort.Sort(sort.Reverse(sort.IntSlice(sorted)))
	return Hand{0, sorted, DiceText(sorted)}
}

// CompareHands returns >0 if a beats b, <0 if b beats a, 0 for a tie.
func CompareHands(a, b Hand) int {
	if a.Cat != b.Cat {
		return a.Cat - b.Cat
	}
	for i := 0; i < len(a.Key) && i < len(b.Key); i++ {
		if a.Key[i] != b.Key[i] {
			return a.Key[i] - b.Key[i]
		}
	}
	return 0
}

// HandRankingNames lists five-dice categories from best to worst, for rules
// text and client help.
var HandRankingNames = []string{
	"Five of a kind", "Four of a kind", "High straight (2-6)", "Low straight (1-5)",
	"Full house", "Three of a kind", "Two pair", "One pair", "Highest single number",
}
