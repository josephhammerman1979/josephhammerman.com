package games

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"
)

// SimOptions configures Simulate.
type SimOptions struct {
	Players  int
	Seed     int64
	MaxSteps int               // give up (error) after this many actions; default 20000
	Options  map[string]string // game options; defaults if nil
	// RemoveAt, if > 0, removes a random remaining player after this many
	// actions, exercising Remove mid-game.
	RemoveAt int
}

// Simulate plays def to completion with random legal actions, checking the
// Game contract: whoever is waited on has a legal action, every listed
// action is accepted, views encode to JSON, and the game ends.  Game
// packages call it from their tests.
func Simulate(def *Def, o SimOptions) error {
	if o.MaxSteps == 0 {
		o.MaxSteps = 20000
	}
	r := rand.New(rand.NewSource(o.Seed))
	roster := make([]string, o.Players)
	for i := range roster {
		roster[i] = fmt.Sprintf("player%d", i)
	}
	now := time.Unix(0, 0)
	t, err := NewTable(def, roster, roster[0], o.Options, 1, rand.New(rand.NewSource(o.Seed+1)), now)
	if err != nil {
		return err
	}
	present := func(string) (bool, time.Duration) { return true, 0 }

	for step := 0; step < o.MaxSteps; step++ {
		// Every seat's view must encode, and must not panic.
		for seat := -1; seat < o.Players; seat++ {
			id := ""
			if seat >= 0 {
				id = roster[seat]
			}
			if _, err := json.Marshal(t.ViewFor("room", id, present, now)); err != nil {
				return fmt.Errorf("step %d: view for seat %d: %v", step, seat, err)
			}
		}
		if t.Over() {
			for _, w := range t.winners() {
				if w < 0 || w >= o.Players {
					return fmt.Errorf("invalid winner %d", w)
				}
			}
			return nil
		}
		if o.RemoveAt > 0 && step == o.RemoveAt {
			left := []int{}
			for i, rm := range t.Removed {
				if !rm {
					left = append(left, i)
				}
			}
			t.Remove(left[r.Intn(len(left))], now)
			continue
		}
		waiting := t.waiting()
		if len(waiting) == 0 {
			return fmt.Errorf("step %d: game not over but waiting on nobody", step)
		}
		seat := waiting[r.Intn(len(waiting))]
		if t.Removed[seat] {
			return fmt.Errorf("step %d: waiting on removed seat %d", step, seat)
		}
		legal := t.Game.Legal(seat)
		if len(legal) == 0 {
			return fmt.Errorf("step %d: waiting on seat %d but it has no legal actions", step, seat)
		}
		a := legal[r.Intn(len(legal))]
		if err := t.Act(roster[seat], a, now); err != nil {
			return fmt.Errorf("step %d: seat %d legal action %+v rejected: %v", step, seat, a, err)
		}
	}
	return fmt.Errorf("game did not finish within %d actions", o.MaxSteps)
}

// SimulateMany runs Simulate across player counts and seeds, with and
// without a mid-game removal.  seeds is the number of games per setup.
func SimulateMany(def *Def, seeds int, opts map[string]string) error {
	for n := def.MinPlayers; n <= def.MaxPlayers && n <= 6; n++ {
		for s := 0; s < seeds; s++ {
			for _, removeAt := range []int{0, 7} {
				if removeAt > 0 && n < 2 {
					continue
				}
				err := Simulate(def, SimOptions{Players: n, Seed: int64(s*1000 + n), Options: opts, RemoveAt: removeAt})
				if err != nil {
					return fmt.Errorf("%s with %d players, seed %d, removeAt %d: %v", def.ID, n, s, removeAt, err)
				}
			}
		}
	}
	return nil
}
