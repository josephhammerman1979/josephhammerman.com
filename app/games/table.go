package games

import (
	"math/rand"
	"time"
)

const (
	// RemoveGrace is how long a player must be disconnected before others
	// can remove them — long enough to ride out a page refresh.
	RemoveGrace = 15 * time.Second
	// IdleLimit is how long the game may wait on a connected player before
	// others can remove them.
	IdleLimit = 60 * time.Second

	viewLogEntries = 30
)

// Presence reports whether a client is connected and, if not, for how long
// they have been gone.
type Presence func(clientID string) (connected bool, awayFor time.Duration)

// Table is a game in progress in one room: the rules engine plus who sits
// where.
type Table struct {
	Def     *Def
	Options map[string]string
	Roster  []string // clientIDs by seat
	Host    string   // who started it
	GameNo  int
	Game    Game
	Env     *Env
	Removed []bool

	lastProgress time.Time
	forcedOver   bool
	forcedWinner []int
}

// NewTable starts def for roster.  Unknown or missing options get their
// default (first) choice.
func NewTable(def *Def, roster []string, host string, opts map[string]string, gameNo int, r *rand.Rand, now time.Time) (*Table, error) {
	if len(roster) < def.MinPlayers {
		return nil, Errorf("%s needs at least %d players.", def.Name, def.MinPlayers)
	}
	if def.MaxPlayers > 0 && len(roster) > def.MaxPlayers {
		return nil, Errorf("%s allows at most %d players.", def.Name, def.MaxPlayers)
	}
	clean := map[string]string{}
	for _, o := range def.Options {
		v := opts[o.Key]
		ok := false
		for _, c := range o.Choices {
			if c.Value == v {
				ok = true
			}
		}
		if !ok && len(o.Choices) > 0 {
			v = o.Choices[0].Value
		}
		clean[o.Key] = v
	}
	env := NewEnv(r)
	t := &Table{
		Def:          def,
		Options:      clean,
		Roster:       append([]string{}, roster...),
		Host:         host,
		GameNo:       gameNo,
		Env:          env,
		Removed:      make([]bool, len(roster)),
		lastProgress: now,
	}
	t.Game = def.New(env, len(roster), clean)
	return t, nil
}

// Seat returns clientID's seat, or -1.
func (t *Table) Seat(clientID string) int {
	for i, id := range t.Roster {
		if id == clientID {
			return i
		}
	}
	return -1
}

// Over reports whether the game has finished.
func (t *Table) Over() bool { return t.forcedOver || t.Game.Over() }

func (t *Table) winners() []int {
	if t.forcedOver {
		return t.forcedWinner
	}
	return t.Game.Winners()
}

func (t *Table) waiting() []int {
	if t.Over() {
		return []int{}
	}
	w := t.Game.Waiting()
	if w == nil {
		w = []int{}
	}
	return w
}

// Act applies clientID's action.
func (t *Table) Act(clientID string, a Action, now time.Time) error {
	seat := t.Seat(clientID)
	if seat < 0 {
		return Errorf("You're not playing in this game.")
	}
	if t.Removed[seat] {
		return Errorf("You were removed from this game.")
	}
	if t.Over() {
		return ErrGameOver
	}
	if err := t.Game.Act(seat, a); err != nil {
		return err
	}
	t.lastProgress = now
	return nil
}

// CanRemove reports whether byID may remove the player in target now.
func (t *Table) CanRemove(byID string, target int, presence Presence, now time.Time) error {
	by := t.Seat(byID)
	if by < 0 || t.Removed[by] {
		return Errorf("Only players in this game can remove someone.")
	}
	if t.Over() {
		return ErrGameOver
	}
	if target < 0 || target >= len(t.Roster) || t.Removed[target] {
		return Errorf("That player isn't in the game.")
	}
	if target == by {
		return Errorf("You can't remove yourself.")
	}
	connected, away := presence(t.Roster[target])
	if !connected {
		if away >= RemoveGrace {
			return nil
		}
		return Errorf("Give them %d seconds to come back.", int((RemoveGrace-away).Seconds())+1)
	}
	for _, w := range t.waiting() {
		if w == target && now.Sub(t.lastProgress) >= IdleLimit {
			return nil
		}
	}
	return Errorf("You can only remove a player who has left, or who has held up the game for %d seconds.", int(IdleLimit.Seconds()))
}

// Remove takes target out of the game, ending it if only one player is left.
func (t *Table) Remove(target int, now time.Time) {
	t.Removed[target] = true
	t.Env.Log(LogBust, "%s was removed from the game.", P(target))
	left := []int{}
	for i, r := range t.Removed {
		if !r {
			left = append(left, i)
		}
	}
	if len(left) <= 1 && !t.Game.Over() {
		t.forcedOver = true
		t.forcedWinner = left
		if len(left) == 1 {
			t.Env.Log(LogWin, "%s wins — everyone else has left.", P(left[0]))
		}
	} else {
		t.Game.Remove(target)
	}
	t.lastProgress = now
}

// View is the per-client message describing the table.
type View struct {
	Type        string            `json:"type"` // "game_state"
	RoomID      string            `json:"roomID"`
	GameNo      int               `json:"gameNo"`
	GameID      string            `json:"gameId"`
	GameName    string            `json:"gameName"`
	Options     map[string]string `json:"options"`
	Roster      []string          `json:"roster"`
	Host        string            `json:"host"`
	MySeat      int               `json:"mySeat"`
	Removed     []bool            `json:"removed"`
	Connected   []bool            `json:"connected"`
	AwaySeconds []int             `json:"awaySeconds"`
	IdleSeconds int               `json:"idleSeconds"`
	GraceSecs   int               `json:"graceSeconds"`
	IdleLimit   int               `json:"idleLimit"`
	Waiting     []int             `json:"waiting"`
	Badges      []string          `json:"badges"`
	Prompt      string            `json:"prompt"`
	Over        bool              `json:"over"`
	Winners     []int             `json:"winners"`
	Log         []LogEntry        `json:"log"`
	RollSeq     int               `json:"rollSeq"`
	State       interface{}       `json:"state"`
}

// ViewFor builds clientID's view (spectators get seat -1).
func (t *Table) ViewFor(roomID, clientID string, presence Presence, now time.Time) View {
	seat := t.Seat(clientID)
	n := len(t.Roster)
	v := View{
		Type:        "game_state",
		RoomID:      roomID,
		GameNo:      t.GameNo,
		GameID:      t.Def.ID,
		GameName:    t.Def.Name,
		Options:     t.Options,
		Roster:      t.Roster,
		Host:        t.Host,
		MySeat:      seat,
		Removed:     t.Removed,
		Connected:   make([]bool, n),
		AwaySeconds: make([]int, n),
		IdleSeconds: int(now.Sub(t.lastProgress).Seconds()),
		GraceSecs:   int(RemoveGrace.Seconds()),
		IdleLimit:   int(IdleLimit.Seconds()),
		Waiting:     t.waiting(),
		Badges:      t.Game.Badges(),
		Prompt:      t.Game.Prompt(seat),
		Over:        t.Over(),
		Winners:     t.winners(),
		Log:         t.Env.Entries(viewLogEntries),
		RollSeq:     t.Env.RollSeq,
		State:       t.Game.View(seat),
	}
	if v.Winners == nil {
		v.Winners = []int{}
	}
	if t.forcedOver {
		v.Prompt = "Game over."
	}
	for i, id := range t.Roster {
		c, away := presence(id)
		v.Connected[i] = c
		if !c {
			v.AwaySeconds[i] = int(away.Seconds())
		}
	}
	return v
}
