// Package games holds the rules engines for the dice games played in video
// rooms.  The server owns all game state: clients send Actions and receive a
// per-player view, so dice a player shouldn't see (Mia, Perudo, ...) never
// leave the server.
//
// Each game implements Game and registers a Def from an init function.  A
// Table (table.go) wraps a running Game with the room-level concerns —
// which client sits in which seat, removals, and building views.
package games

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// Action is a player's move.  Which fields matter depends on Type; each game
// documents the actions it accepts.
type Action struct {
	Type   string `json:"type"`
	Dice   []int  `json:"dice,omitempty"`   // die indices (keep / reroll) or face values (a bid)
	Index  int    `json:"index,omitempty"`  // a square, area, option index
	Count  int    `json:"count,omitempty"`  // a bid quantity
	Face   int    `json:"face,omitempty"`   // a bid face
	Amount int    `json:"amount,omitempty"` // a wager
	Text   string `json:"text,omitempty"`   // free text (rule proposals)
	Yes    bool   `json:"yes,omitempty"`    // a vote
}

// Game is one running game.  Seats are 0..n-1 in roster order.  All methods
// are called with the table lock held, so implementations need no locking.
type Game interface {
	// Act applies seat's action, or returns an error (shown to that player)
	// without changing anything.
	Act(seat int, a Action) error

	// Legal lists actions seat could take right now.  For actions with a
	// large or free-form parameter space (bids, wagers, text) a
	// representative sample is enough.  Used by tests and bots to play
	// every game to completion, so every listed action must be accepted.
	Legal(seat int) []Action

	// Remove takes seat out of the game (they left or were idle).  The
	// game must carry on with the remaining players; if fewer than two
	// remain the Table ends the game itself.
	Remove(seat int)

	// View is what seat may see, JSON-encoded for that player's renderer.
	// seat is -1 for spectators.  Never include hidden information that
	// seat isn't entitled to.
	View(seat int) interface{}

	// Prompt is a one-line instruction for seat, e.g. "Roll or hold" or
	// "Waiting for {P2} to bid".  Use P(seat) for names.
	Prompt(seat int) string

	// Waiting lists the seats whose input the game is waiting for.
	Waiting() []int

	// Badges is a short status per seat ("45", "3 chips", "out") shown on
	// that player's video tile and chip.
	Badges() []string

	Over() bool
	Winners() []int
}

// Option is a setup choice offered before a game starts (target score,
// variant, ...).  The first choice is the default.
type Option struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Def describes a game for the catalog and constructs instances.
type Def struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Summary    string   `json:"summary"` // one line for the picker
	Rules      []string `json:"rules"`   // short how-to-play bullets
	MinPlayers int      `json:"min"`
	MaxPlayers int      `json:"max"`
	Options    []Option `json:"options,omitempty"`
	Order      int      `json:"-"` // position in the catalog

	// The game's entry from the rules sheet, for its rules page
	// (/games/rules/<id>): the header facts ("Number of dice: 1", ...), the
	// rules paragraphs as written, and board diagrams under /css/rules/.
	Facts  []string `json:"-"`
	Sheet  []string `json:"-"`
	Images []string `json:"-"`

	// RulesURL links to the game's tab in the shared rules document, if
	// known (see RulesLinks).
	RulesURL string `json:"rulesUrl,omitempty"`

	// New starts a game for players seats.  opts has a value for every
	// Option key (defaults filled in).
	New func(env *Env, players int, opts map[string]string) Game `json:"-"`
}

var registry = map[string]*Def{}

// Register adds a game to the catalog.  Call from an init function.
func Register(d *Def) {
	if d.ID == "" || d.New == nil {
		panic("games: Register needs an ID and a New func")
	}
	if _, dup := registry[d.ID]; dup {
		panic("games: duplicate game id " + d.ID)
	}
	if d.RulesURL == "" {
		d.RulesURL = RulesLinks[d.ID]
	}
	registry[d.ID] = d
}

// Lookup returns the Def for id, or nil.
func Lookup(id string) *Def { return registry[id] }

// Catalog lists all registered games in display order.
func Catalog() []*Def {
	out := make([]*Def, 0, len(registry))
	for _, d := range registry {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// ── Env: randomness and the game log ─────────────────────────────────────────

// LogEntry is one line of the game log.  Text may contain {P<n>} tokens
// that clients replace with player names.
type LogEntry struct {
	Seq  int    `json:"seq"`
	Kind string `json:"kind"` // see the Log* constants
	Text string `json:"text"`
}

// Log entry kinds.  Clients style them, and flash LogBust / LogWin entries.
const (
	LogInfo = "info"
	LogRoll = "roll"
	LogGood = "good"
	LogBust = "bust" // a setback: bust, lost a die/chip, caught bluffing
	LogWin  = "win"
)

const maxLog = 60

// Env is the game's interface to the outside world: dice and the log.
type Env struct {
	Rand    *rand.Rand
	RollSeq int // bumped on every roll so clients can animate fresh dice
	log     []LogEntry
	seq     int
	script  []int // tests: faces to roll before falling back to Rand
}

func NewEnv(r *rand.Rand) *Env { return &Env{Rand: r} }

// NewTestEnv returns an Env whose dice come up as faces, in order, and then
// fall back to a fixed-seed random source.  For tests.
func NewTestEnv(faces ...int) *Env {
	return &Env{Rand: rand.New(rand.NewSource(1)), script: append([]int{}, faces...)}
}

func (e *Env) face() int {
	if len(e.script) > 0 {
		v := e.script[0]
		e.script = e.script[1:]
		return v
	}
	return e.Rand.Intn(6) + 1
}

// Die rolls one die.
func (e *Env) Die() int {
	e.RollSeq++
	return e.face()
}

// Roll rolls n dice.
func (e *Env) Roll(n int) []int {
	e.RollSeq++
	out := make([]int, n)
	for i := range out {
		out[i] = e.face()
	}
	return out
}

// Log appends a log entry; format and args as for fmt.Sprintf.
func (e *Env) Log(kind, format string, args ...interface{}) {
	e.seq++
	e.log = append(e.log, LogEntry{Seq: e.seq, Kind: kind, Text: fmt.Sprintf(format, args...)})
	if len(e.log) > maxLog {
		e.log = e.log[len(e.log)-maxLog:]
	}
}

// Entries returns the most recent n log entries, oldest first.
func (e *Env) Entries(n int) []LogEntry {
	if len(e.log) <= n {
		return append([]LogEntry(nil), e.log...)
	}
	return append([]LogEntry(nil), e.log[len(e.log)-n:]...)
}

// ── Helpers shared by game implementations ───────────────────────────────────

// P is the name token for seat, e.g. "{P2}".
func P(seat int) string { return fmt.Sprintf("{P%d}", seat) }

// DiceText formats dice as "3 4 4 6".
func DiceText(d []int) string {
	parts := make([]string, len(d))
	for i, v := range d {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " ")
}

// Sum adds dice.
func Sum(d []int) int {
	t := 0
	for _, v := range d {
		t += v
	}
	return t
}

// Counts returns how many of each face (index 1..6) appear.
func Counts(d []int) [7]int {
	var c [7]int
	for _, v := range d {
		if v >= 1 && v <= 6 {
			c[v]++
		}
	}
	return c
}

// Errorf builds an error shown to the player who tried the action.
func Errorf(format string, args ...interface{}) error { return fmt.Errorf(format, args...) }

var (
	ErrNotYourTurn = errors.New("It isn't your turn.")
	ErrGameOver    = errors.New("The game is over.")
	ErrUnknown     = errors.New("That move isn't available right now.")
)

// UniqueIndices validates a set of die indices into a slice of length n:
// all in range, no repeats.  Returns them sorted.
func UniqueIndices(idx []int, n int) ([]int, error) {
	seen := map[int]bool{}
	out := make([]int, 0, len(idx))
	for _, i := range idx {
		if i < 0 || i >= n {
			return nil, Errorf("No such die.")
		}
		if seen[i] {
			return nil, Errorf("Each die can only be picked once.")
		}
		seen[i] = true
		out = append(out, i)
	}
	sort.Ints(out)
	return out, nil
}

// Seats tracks who is still in the game and whose turn is next.
type Seats struct {
	N   int
	Out []bool
}

func NewSeats(n int) Seats { return Seats{N: n, Out: make([]bool, n)} }

// In reports whether seat is still playing.
func (s *Seats) In(seat int) bool { return seat >= 0 && seat < s.N && !s.Out[seat] }

// Next returns the next seat after from (wrapping) that is still in, or
// from itself if nobody else is.
func (s *Seats) Next(from int) int {
	for i := 1; i <= s.N; i++ {
		n := (from + i) % s.N
		if !s.Out[n] {
			return n
		}
	}
	return from
}

// Active lists seats still in, in seat order.
func (s *Seats) Active() []int {
	out := []int{}
	for i := 0; i < s.N; i++ {
		if !s.Out[i] {
			out = append(out, i)
		}
	}
	return out
}

func (s *Seats) ActiveCount() int { return len(s.Active()) }

// OrderFrom lists active seats starting at first and going around.
func (s *Seats) OrderFrom(first int) []int {
	out := []int{}
	for i := 0; i < s.N; i++ {
		n := (first + i) % s.N
		if !s.Out[n] {
			out = append(out, n)
		}
	}
	return out
}

// Ints returns a copy of the given ints (handy for views).
func Ints(v []int) []int { return append([]int{}, v...) }

// BestSeats returns the active seats with the highest (or lowest, if low)
// score.
func BestSeats(s *Seats, score []int, low bool) []int {
	best := []int{}
	for _, seat := range s.Active() {
		if len(best) == 0 {
			best = []int{seat}
			continue
		}
		cur, b := score[seat], score[best[0]]
		switch {
		case cur == b:
			best = append(best, seat)
		case (cur > b) != low:
			best = []int{seat}
		}
	}
	return best
}

// RollOff breaks a tie among seats by rolling one die each until a single
// highest roll emerges, logging each round.  Returns the winning seat.
func RollOff(env *Env, seats []int) int {
	tied := append([]int{}, seats...)
	for len(tied) > 1 {
		best, bestSeats := 0, []int{}
		parts := []string{}
		for _, s := range tied {
			v := env.Die()
			parts = append(parts, fmt.Sprintf("%s %d", P(s), v))
			switch {
			case v > best:
				best, bestSeats = v, []int{s}
			case v == best:
				bestSeats = append(bestSeats, s)
			}
		}
		env.Log(LogRoll, "Roll-off: %s", strings.Join(parts, ", "))
		tied = bestSeats
	}
	return tied[0]
}
