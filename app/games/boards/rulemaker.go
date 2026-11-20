package boards

import (
	"fmt"
	"strings"

	"github.com/josephhammerman1979/josephhammerman.com/app/games"
)

// Rule Maker: roll one die a round, first to 50 — and each round's high
// roller proposes a house rule that the table votes on.  The software
// can't enforce invented rules, so adopted rules are honour-system.
//
// Actions: {type:"roll"}, {type:"propose", text}, {type:"pass"},
// {type:"vote", yes}.

func init() {
	games.Register(&games.Def{
		ID: "rulemaker", Name: "Meta", Order: 140,
		Summary:    "Race to 50 one die at a time — and each round's high roller gets to invent a new house rule.",
		MinPlayers: 2, MaxPlayers: 10,
		Rules: []string{
			"Each round, everyone rolls one die in turn and adds it to their score.",
			"The round's highest roller (ties roll off) proposes a new house rule — or passes.",
			"Everyone votes yes or no; a majority of yes votes adopts the rule.",
			"The app can't enforce house rules — play them on your honour (adjust scores out loud, etc.).",
			"First to 50 points wins — unless a house rule says otherwise!",
		},
		Facts: []string{"Number of dice: 1", "Player count: 2-∞", "Object: First to 50"},
		Sheet: []string{
			"Each player in turn throws one die. The high scorer proposes a new rule. A vote is conducted. The game continues with the new rule in place. The first player to reach 50 points wins (assuming this rule stands).",
		},
		New: func(env *games.Env, n int, _ map[string]string) games.Game { return newRuleMaker(env, n) },
	})
}

const (
	ruleGoal    = 50
	ruleMaxText = 140
)

const (
	rmRoll    = "roll"
	rmPropose = "propose"
	rmVote    = "vote"
	rmOver    = "over"
)

const (
	voteNone = 0
	voteYes  = 1
	voteNo   = 2
)

type houseRule struct {
	Text  string `json:"text"`
	By    int    `json:"by"`
	Round int    `json:"round"`
}

type voteResult struct {
	Text    string `json:"text"`
	By      int    `json:"by"`
	Round   int    `json:"round"`
	Yes     []int  `json:"yes"`
	No      []int  `json:"no"`
	Adopted bool   `json:"adopted"`
	Passed  bool   `json:"passed"` // the proposer passed (no vote)
}

type ruleMaker struct {
	env    *games.Env
	seats  games.Seats
	scores []int
	phase  string

	round   int
	starter int
	order   []int
	turn    int
	rolls   []int // this round's roll totals, 0 = not yet
	dice    []int // the latest roll
	roller  int   // who rolled it

	proposer int
	proposal string
	votes    []int

	rules    []houseRule
	lastVote *voteResult
	winner   int
}

func newRuleMaker(env *games.Env, n int) *ruleMaker {
	g := &ruleMaker{env: env, seats: games.NewSeats(n), scores: make([]int, n), starter: -1, roller: -1, proposer: -1, winner: -1}
	g.startRound()
	return g
}

func (g *ruleMaker) startRound() {
	g.round++
	if g.starter < 0 {
		g.starter = 0
	} else {
		g.starter = g.seats.Next(g.starter)
	}
	if !g.seats.In(g.starter) {
		g.starter = g.seats.Next(g.starter)
	}
	g.order = g.seats.OrderFrom(g.starter)
	g.turn = 0
	g.rolls = make([]int, g.seats.N)
	g.votes = make([]int, g.seats.N)
	g.proposer, g.proposal = -1, ""
	g.phase = rmRoll
	g.env.Log(games.LogInfo, "Round %d — %s rolls first.", g.round, games.P(g.starter))
}

func (g *ruleMaker) cur() int {
	if g.phase == rmRoll && g.turn < len(g.order) {
		return g.order[g.turn]
	}
	if g.phase == rmPropose {
		return g.proposer
	}
	return -1
}

// cleanRule trims and collapses whitespace, drops control characters and
// braces (which the client would read as name tokens), and caps the length.
func cleanRule(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '{':
			return '('
		case '}':
			return ')'
		}
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > ruleMaxText {
		s = strings.TrimSpace(string(r[:ruleMaxText]))
	}
	return s
}

func (g *ruleMaker) Act(seat int, a games.Action) error {
	if g.phase == rmOver {
		return games.ErrGameOver
	}
	switch a.Type {
	case "roll":
		if g.phase != rmRoll {
			return games.ErrUnknown
		}
		if seat != g.cur() {
			return games.ErrNotYourTurn
		}
		g.dice = g.env.Roll(1)
		g.roller = seat
		t := games.Sum(g.dice)
		g.rolls[seat] = t
		g.scores[seat] += t
		if g.scores[seat] >= ruleGoal {
			g.phase = rmOver
			g.winner = seat
			g.env.Log(games.LogWin, "%s rolled a %d → %d and wins!", games.P(seat), t, g.scores[seat])
			return nil
		}
		g.env.Log(games.LogRoll, "%s rolled a %d → %d", games.P(seat), t, g.scores[seat])
		g.nextTurn()
	case "propose", "pass":
		if g.phase != rmPropose {
			return games.ErrUnknown
		}
		if seat != g.proposer {
			return games.ErrNotYourTurn
		}
		if a.Type == "pass" {
			g.lastVote = &voteResult{By: seat, Round: g.round, Passed: true, Yes: []int{}, No: []int{}}
			g.env.Log(games.LogInfo, "%s passed on making a rule.", games.P(seat))
			g.startRound()
			return nil
		}
		text := cleanRule(a.Text)
		if text == "" {
			return games.Errorf("Type a rule, or pass.")
		}
		g.proposal = text
		g.votes = make([]int, g.seats.N)
		g.phase = rmVote
		g.env.Log(games.LogInfo, "%s proposes: “%s”", games.P(seat), text)
	case "vote":
		if g.phase != rmVote {
			return games.ErrUnknown
		}
		if !g.seats.In(seat) {
			return games.ErrNotYourTurn
		}
		if g.votes[seat] != voteNone {
			return games.Errorf("You've already voted.")
		}
		if a.Yes {
			g.votes[seat] = voteYes
		} else {
			g.votes[seat] = voteNo
		}
		g.checkVotes()
	default:
		return games.ErrUnknown
	}
	return nil
}

func (g *ruleMaker) nextTurn() {
	g.turn++
	for g.turn < len(g.order) && !g.seats.In(g.order[g.turn]) {
		g.turn++
	}
	if g.turn < len(g.order) {
		return
	}
	best := games.BestSeats(&g.seats, g.rolls, false)
	p := best[0]
	if len(best) > 1 {
		g.env.Log(games.LogInfo, "%s tied for the high roll — roll-off!", seatList(best))
		p = games.RollOff(g.env, best)
	}
	g.proposer = p
	g.phase = rmPropose
	g.env.Log(games.LogGood, "%s rolled highest and may propose a house rule.", games.P(p))
}

func (g *ruleMaker) checkVotes() {
	res := &voteResult{Text: g.proposal, By: g.proposer, Round: g.round, Yes: []int{}, No: []int{}}
	for _, s := range g.seats.Active() {
		switch g.votes[s] {
		case voteNone:
			return
		case voteYes:
			res.Yes = append(res.Yes, s)
		default:
			res.No = append(res.No, s)
		}
	}
	total := len(res.Yes) + len(res.No)
	res.Adopted = len(res.Yes)*2 > total
	g.lastVote = res
	if res.Adopted {
		g.rules = append(g.rules, houseRule{Text: g.proposal, By: g.proposer, Round: g.round})
		g.env.Log(games.LogGood, "Rule adopted, %d–%d: “%s”", len(res.Yes), len(res.No), g.proposal)
	} else {
		g.env.Log(games.LogBust, "Rule rejected, %d–%d.", len(res.Yes), len(res.No))
	}
	g.startRound()
}

func (g *ruleMaker) Legal(seat int) []games.Action {
	switch g.phase {
	case rmRoll:
		if seat == g.cur() {
			return []games.Action{{Type: "roll"}}
		}
	case rmPropose:
		if seat == g.proposer {
			return []games.Action{{Type: "propose", Text: "  Doubles score double.  "}, {Type: "pass"}}
		}
	case rmVote:
		if g.seats.In(seat) && g.votes[seat] == voteNone {
			return []games.Action{{Type: "vote", Yes: true}, {Type: "vote", Yes: false}}
		}
	}
	return nil
}

func (g *ruleMaker) Remove(seat int) {
	g.seats.Out[seat] = true
	if g.phase == rmOver {
		return
	}
	if g.seats.ActiveCount() < 2 {
		g.phase = rmOver
		g.winner = g.seats.Active()[0]
		return
	}
	switch g.phase {
	case rmRoll:
		if g.turn < len(g.order) && g.order[g.turn] == seat {
			g.nextTurn()
		}
	case rmPropose:
		if seat == g.proposer {
			g.env.Log(games.LogInfo, "No rule this round.")
			g.startRound()
		}
	case rmVote:
		g.votes[seat] = voteNone
		g.checkVotes()
	}
}

type ruleMakerView struct {
	Scores    []int       `json:"scores"`
	Out       []bool      `json:"out"`
	Goal      int         `json:"goal"`
	Phase     string      `json:"phase"`
	Round     int         `json:"round"`
	Current   int         `json:"current"`
	Order     []int       `json:"order"`
	Rolls     []int       `json:"rolls"`
	Dice      []int       `json:"dice"`
	Roller    int         `json:"roller"`
	Proposer  int         `json:"proposer"`
	Proposal  string      `json:"proposal"`
	Voted     []bool      `json:"voted"` // who has voted (not how)
	VotesCast int         `json:"votesCast"`
	Voters    int         `json:"voters"`
	MyVote    int         `json:"myVote"` // 0 none, 1 yes, 2 no
	MaxText   int         `json:"maxText"`
	Rules     []houseRule `json:"rules"`
	LastVote  *voteResult `json:"lastVote"`
}

func (g *ruleMaker) View(seat int) interface{} {
	v := ruleMakerView{
		Scores: games.Ints(g.scores), Out: append([]bool{}, g.seats.Out...), Goal: ruleGoal, Phase: g.phase,
		Round: g.round, Current: g.cur(), Order: games.Ints(g.order), Rolls: games.Ints(g.rolls),
		Dice: games.Ints(g.dice), Roller: g.roller, Proposer: g.proposer, Proposal: g.proposal,
		Voted: make([]bool, g.seats.N), MaxText: ruleMaxText,
		Rules: append([]houseRule{}, g.rules...), LastVote: g.lastVote,
	}
	if g.phase == rmVote {
		for _, s := range g.seats.Active() {
			v.Voters++
			if g.votes[s] != voteNone {
				v.Voted[s] = true
				v.VotesCast++
			}
		}
		if seat >= 0 && seat < g.seats.N {
			v.MyVote = g.votes[seat]
		}
	}
	return v
}

func (g *ruleMaker) Prompt(seat int) string {
	switch g.phase {
	case rmOver:
		return fmt.Sprintf("%s reached %d and wins!", games.P(g.winner), g.scores[g.winner])
	case rmRoll:
		c := g.cur()
		if seat == c {
			return "Roll your die — it goes on your score."
		}
		return fmt.Sprintf("Waiting for %s to roll.", games.P(c))
	case rmPropose:
		if seat == g.proposer {
			return "You rolled highest! Propose a house rule, or pass."
		}
		return fmt.Sprintf("Waiting for %s to propose a house rule.", games.P(g.proposer))
	case rmVote:
		if g.seats.In(seat) && g.votes[seat] == voteNone {
			return fmt.Sprintf("Vote on %s's proposed rule: yes or no?", games.P(g.proposer))
		}
		return "Waiting for everyone to vote."
	}
	return ""
}

func (g *ruleMaker) Waiting() []int {
	switch g.phase {
	case rmRoll, rmPropose:
		return []int{g.cur()}
	case rmVote:
		out := []int{}
		for _, s := range g.seats.Active() {
			if g.votes[s] == voteNone {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func (g *ruleMaker) Badges() []string {
	out := make([]string, g.seats.N)
	for s := range out {
		if g.seats.Out[s] {
			out[s] = "out"
			continue
		}
		out[s] = fmt.Sprintf("%d/%d", g.scores[s], ruleGoal)
		if g.phase == rmVote && g.votes[s] != voteNone {
			out[s] += " · voted"
		}
	}
	return out
}

func (g *ruleMaker) Over() bool { return g.phase == rmOver }

func (g *ruleMaker) Winners() []int {
	if g.phase != rmOver || g.winner < 0 {
		return nil
	}
	return []int{g.winner}
}
