package olympiad

import (
	"context"
	"sync"
	"time"
)

// GameType is how a match's two competitors were drawn: from one class's
// registrations, or from every registration regardless of class.
type GameType int

const (
	// Classed matches draw both competitors from one class's registrations.
	Classed GameType = iota
	// NonClassed matches draw them from the registrations of every class.
	NonClassed
)

// Competitor is what a match keeps of a player drawn into it.
type Competitor struct {
	Name      string
	BaseClass int
}

// Roster finds the online player objectID to draw into a match; ok is false
// when that player is offline.
type Roster func(objectID int32) (c Competitor, ok bool)

// Participant is one of a match's two competitors.
type Participant struct {
	ObjectID int32
	Name     string
	// Side is 1 for the first competitor drawn, 2 for the second.
	Side      int
	BaseClass int
}

// Standing is a competitor's state when its match is judged. Dead, HP and
// CP are the competitor's own, also while it is offline.
type Standing struct {
	Online bool
	Dead   bool
	HP, CP float64
}

// ResultKind is one message the judging of a match sends.
type ResultKind int

const (
	// ResultWon tells the stadium the named competitor won.
	ResultWon ResultKind = iota
	// ResultTie tells the stadium the match ended in a tie.
	ResultTie
	// ResultPointsGained tells both competitors the named one gained
	// Points.
	ResultPointsGained
	// ResultPointsLost tells both competitors the named one lost Points.
	ResultPointsLost
)

// ResultMessage is one message of a match's judging.
type ResultMessage struct {
	Kind   ResultKind
	Name   string
	Points int
}

// Outcome is what the judging of a match tells and gives: Messages in
// order, then Reward to the competitor Winner when Winner is not 0.
type Outcome struct {
	Messages []ResultMessage
	Winner   int32
	Reward   []Reward
}

// Fight is one match's result as the olympiad_fights table stores it.
// Winner is the winning side, 0 for a tie; Start is in Unix milliseconds
// and Time is the match's length in milliseconds.
type Fight struct {
	CharOneID, CharTwoID       int32
	CharOneClass, CharTwoClass int
	Winner                     int
	Start, Time                int64
	Classed                    bool
}

// Game is one match between two competitors in a stadium. The stadium's
// match driver judges it, while the competitors' hits add to its damage,
// so its state is guarded by mu.
type Game struct {
	o       *Olympiad
	stadium int
	kind    GameType
	players [2]Participant

	mu           sync.Mutex
	damage       [2]int
	disconnected [2]bool
	defecting    [2]bool
	start        time.Time
}

// NonClassedGame draws two online competitors from queue into a
// non-classed match in stadium, or returns nil when queue no longer holds
// two. Each competitor drawn is taken off queue; a first draw found offline
// is dropped, and a second one found offline is dropped while the first
// goes back to the end of queue. rnd(n) picks an index below n.
func (o *Olympiad) NonClassedGame(stadium int, queue *[]int32, roster Roster, rnd func(n int) int) *Game {
	players, ok := drawPair(queue, roster, rnd)
	if !ok {
		return nil
	}
	return o.newGame(stadium, NonClassed, players)
}

// ClassedGame draws two online competitors from one of ready, the class
// queues holding enough registrations, into a classed match in stadium, or
// returns nil when none of them still holds two. It picks the queue at
// random and drops it from ready when no pair can be drawn from it; drawing
// takes the competitors off that queue as NonClassedGame does.
func (o *Olympiad) ClassedGame(stadium int, ready *[]*[]int32, roster Roster, rnd func(n int) int) *Game {
	for len(*ready) > 0 {
		i := rnd(len(*ready))
		queue := (*ready)[i]
		if queue != nil && len(*queue) >= 2 {
			if players, ok := drawPair(queue, roster, rnd); ok {
				return o.newGame(stadium, Classed, players)
			}
		}
		*ready = append((*ready)[:i], (*ready)[i+1:]...)
	}
	return nil
}

func (o *Olympiad) newGame(stadium int, kind GameType, players [2]Participant) *Game {
	return &Game{o: o, stadium: stadium, kind: kind, players: players}
}

// drawPair takes two online competitors off queue, the first on side 1.
func drawPair(queue *[]int32, roster Roster, rnd func(n int) int) ([2]Participant, bool) {
	for len(*queue) > 1 {
		firstID := takeAt(queue, rnd(len(*queue)))
		first, ok := roster(firstID)
		if !ok {
			continue
		}
		secondID := takeAt(queue, rnd(len(*queue)))
		second, ok := roster(secondID)
		if !ok {
			*queue = append(*queue, firstID)
			continue
		}
		return [2]Participant{
			{ObjectID: firstID, Name: first.Name, Side: 1, BaseClass: first.BaseClass},
			{ObjectID: secondID, Name: second.Name, Side: 2, BaseClass: second.BaseClass},
		}, true
	}
	return [2]Participant{}, false
}

// takeAt removes and returns the element of queue at i.
func takeAt(queue *[]int32, i int) int32 {
	id := (*queue)[i]
	*queue = append((*queue)[:i], (*queue)[i+1:]...)
	return id
}

// Stadium is the stadium the match is fought in.
func (g *Game) Stadium() int { return g.stadium }

// Type is how the match's competitors were drawn.
func (g *Game) Type() GameType { return g.kind }

// Participants are the match's two competitors, side 1 first.
func (g *Game) Participants() [2]Participant { return g.players }

// Contains reports whether objectID competes in the match.
func (g *Game) Contains(objectID int32) bool { return g.sideOf(objectID) >= 0 }

// sideOf is the index of objectID's side, -1 when it does not compete.
func (g *Game) sideOf(objectID int32) int {
	for i, p := range g.players {
		if p.ObjectID == objectID {
			return i
		}
	}
	return -1
}

// AddDamage adds damage the competitor objectID dealt its opponent.
func (g *Game) AddDamage(objectID int32, damage int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if i := g.sideOf(objectID); i >= 0 {
		g.damage[i] += damage
	}
}

// ResetDamage forgets the damage dealt so far, as the fight begins.
func (g *Game) ResetDamage() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.damage = [2]int{}
}

// Disconnect records that the competitor objectID left the game.
func (g *Game) Disconnect(objectID int32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if i := g.sideOf(objectID); i >= 0 {
		g.disconnected[i] = true
	}
}

// Defect records that the competitor objectID could not be brought into
// the stadium.
func (g *Game) Defect(objectID int32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if i := g.sideOf(objectID); i >= 0 {
		g.defecting[i] = true
	}
}

// Start records the moment the fight begins.
func (g *Game) Start(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.start = now
}

// divider is what the lower of the two competitors' points is divided by
// into what the match moves.
func (g *Game) divider() int {
	if g.kind == Classed {
		return g.o.cfg.DividerClassed
	}
	return g.o.cfg.DividerNonClassed
}

// reward is what the winner of the match is given.
func (g *Game) reward() []Reward {
	if g.kind == Classed {
		return g.o.cfg.ClassedReward
	}
	return g.o.cfg.NonClassedReward
}

// Judge decides the match at now from each competitor's standing, looked
// up by object id: it moves points between the competitors' records,
// counts the match in them, queues the fight's row and returns what the
// stadium and the competitors are told and the winner is given.
//
// A competitor who defected before the fight loses a third of its points,
// at most MaxPoints, and nothing else happens. Otherwise the match moves
// the lower of the two totals divided by the type's divider, at least 1
// and at most MaxPoints: a competitor who disconnected loses them to one
// who did not, or both lose them when both did; no fight row is stored
// then. With neither disconnected, both gone offline is a draw; otherwise
// one wins when the other is offline, when it alone still stands (alive
// with at least half a point of HP and CP together), or when both stand
// and it dealt more damage; anything else is a tie, in which each loses its
// own total divided by the divider, at most MaxPoints. The winner of a
// decided match is rewarded.
func (g *Game) Judge(now time.Time, standing func(objectID int32) Standing) Outcome {
	g.mu.Lock()
	damage, disconnected, defecting, start := g.damage, g.disconnected, g.defecting, g.start
	g.mu.Unlock()

	j := judging{g: g}
	one, two := g.players[0], g.players[1]
	onePoints, twoPoints := g.points(one.ObjectID), g.points(two.ObjectID)
	maxPoints := g.o.cfg.MaxPoints

	if defecting[0] || defecting[1] {
		if defecting[0] {
			j.lose(one, min(onePoints/3, maxPoints))
		}
		if defecting[1] {
			j.lose(two, min(twoPoints/3, maxPoints))
		}
		return j.out
	}

	diff := min(max(min(onePoints, twoPoints)/g.divider(), 1), maxPoints)
	oneStanding, twoStanding := standing(one.ObjectID), standing(two.ObjectID)
	oneCrash, twoCrash := disconnected[0], disconnected[1]
	oneHP, twoHP := fightingHP(oneStanding), fightingHP(twoStanding)
	switch {
	case twoCrash && !oneCrash:
		j.decide(one, two, diff)
	case oneCrash && !twoCrash:
		j.decide(two, one, diff)
	case oneCrash && twoCrash:
		j.announce(ResultTie, "")
		j.count(one, compLost)
		j.lose(one, diff)
		j.count(two, compLost)
		j.lose(two, diff)
	case !oneStanding.Online && !twoStanding.Online:
		j.count(one, compDrawn)
		j.count(two, compDrawn)
		j.announce(ResultTie, "")
	case !twoStanding.Online || (twoHP == 0 && oneHP != 0) || (damage[0] > damage[1] && twoHP != 0 && oneHP != 0):
		g.saveFight(1, start, now)
		j.decide(one, two, diff)
	case !oneStanding.Online || (oneHP == 0 && twoHP != 0) || (damage[1] > damage[0] && oneHP != 0 && twoHP != 0):
		g.saveFight(2, start, now)
		j.decide(two, one, diff)
	default:
		g.saveFight(0, start, now)
		j.announce(ResultTie, "")
		j.lose(one, min(onePoints/g.divider(), maxPoints))
		j.lose(two, min(twoPoints/g.divider(), maxPoints))
	}
	j.count(one, compDone)
	j.count(two, compDone)
	return j.out
}

// judging collects a match's outcome while it changes the competitors'
// records.
type judging struct {
	g   *Game
	out Outcome
}

// decide gives the match to winner: diff points move from loser to winner,
// the match counts as won and lost, and winner is rewarded.
func (j *judging) decide(winner, loser Participant, diff int) {
	j.announce(ResultWon, winner.Name)
	j.count(winner, compWon)
	j.count(loser, compLost)
	j.gain(winner, diff)
	j.lose(loser, diff)
	j.out.Winner, j.out.Reward = winner.ObjectID, j.g.reward()
}

func (j *judging) announce(kind ResultKind, name string) {
	j.out.Messages = append(j.out.Messages, ResultMessage{Kind: kind, Name: name})
}

func (j *judging) gain(p Participant, n int) {
	j.g.o.updateNoble(p.ObjectID, func(r *Noble) { r.addPoints(n) })
	j.out.Messages = append(j.out.Messages, ResultMessage{Kind: ResultPointsGained, Name: p.Name, Points: n})
}

func (j *judging) lose(p Participant, n int) {
	j.g.o.updateNoble(p.ObjectID, func(r *Noble) { r.addPoints(-n) })
	j.out.Messages = append(j.out.Messages, ResultMessage{Kind: ResultPointsLost, Name: p.Name, Points: n})
}

// count adds one match to the counter of p's record field picks.
func (j *judging) count(p Participant, field func(*Noble) *int) {
	j.g.o.updateNoble(p.ObjectID, func(r *Noble) { *field(r)++ })
}

func compDone(r *Noble) *int  { return &r.CompDone }
func compWon(r *Noble) *int   { return &r.CompWon }
func compLost(r *Noble) *int  { return &r.CompLost }
func compDrawn(r *Noble) *int { return &r.CompDrawn }

// points is objectID's points, 0 without a record.
func (g *Game) points(objectID int32) int {
	n, _ := g.o.Noble(objectID)
	return n.Points
}

// fightingHP is what a competitor has left to fight with: HP and CP
// together while it is alive, 0 when dead or under half a point.
func fightingHP(s Standing) float64 {
	if s.Dead {
		return 0
	}
	if hp := s.HP + s.CP; hp >= 0.5 {
		return hp
	}
	return 0
}

// saveFight queues the fight's row: winner is the winning side, 0 for a
// tie.
func (g *Game) saveFight(winner int, start, now time.Time) {
	f := Fight{
		CharOneID: g.players[0].ObjectID, CharTwoID: g.players[1].ObjectID,
		CharOneClass: g.players[0].BaseClass, CharTwoClass: g.players[1].BaseClass,
		Winner: winner, Start: start.UnixMilli(), Time: now.Sub(start).Milliseconds(),
		Classed: g.kind == Classed,
	}
	g.o.write("save fight", func(ctx context.Context, st Store) error { return st.SaveFight(ctx, f) })
}
