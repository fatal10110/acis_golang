package olympiad

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// MessageKind is one message a stadium's match sends.
type MessageKind int

const (
	// MessageEnterStadiumIn tells the competitors they enter the stadium in
	// Number seconds.
	MessageEnterStadiumIn MessageKind = iota
	// MessageGameStartsIn tells the stadium the fight starts in Number
	// seconds.
	MessageGameStartsIn
	// MessageGameStarted tells the stadium the fight started.
	MessageGameStarted
	// MessageMovedToTownIn tells the competitors they return in Number
	// seconds.
	MessageMovedToTownIn
	// MessageOpponentLeft tells a competitor the match is cancelled because
	// its opponent left it.
	MessageOpponentLeft
	// MessageOpponentUnfit tells a competitor the match is cancelled because
	// its opponent no longer meets the requirements.
	MessageOpponentUnfit
	// MessageUnfitDead, MessageUnfitSubclass, MessageUnfitCursedWeapon (the
	// item Number) and MessageUnfitOverweight tell a competitor why it cannot
	// compete.
	MessageUnfitDead
	MessageUnfitSubclass
	MessageUnfitCursedWeapon
	MessageUnfitOverweight
	// MessageWon tells the stadium Name won the match.
	MessageWon
	// MessageTie tells the stadium the match ended in a tie.
	MessageTie
	// MessagePointsGained and MessagePointsLost tell the competitors Name
	// gained or lost Number points.
	MessagePointsGained
	MessagePointsLost
)

// Message is one message of a stadium's match.
type Message struct {
	Kind   MessageKind
	Number int
	Name   string
}

// Fitness is what the check before a competitor enters the stadium reads of
// it.
type Fitness struct {
	// Online reports the competitor is in the world; Detached that its
	// connection dropped while it lingers there.
	Online, Detached bool
	Observer         bool
	Dead             bool
	Subclass         bool
	// CursedWeaponID is the cursed weapon the competitor holds, 0 for none.
	CursedWeaponID int32
	Overweight     bool
}

// Arena carries out a stadium's match on its players and its zone. Every
// call names a player by object id and does nothing to one offline; what
// it does to a player, and what it sends one, reaches that player in the
// order of the calls.
type Arena interface {
	// Competitor finds the online player objectID to draw into a match.
	Competitor(objectID int32) (Competitor, bool)
	// Online reports whether objectID is in the world.
	Online(objectID int32) bool
	// Fitness reads objectID as the check before the stadium does.
	Fitness(objectID int32) Fitness
	// Standing reads objectID's state in its match; offline, it is the zero
	// Standing with GameID -1.
	Standing(objectID int32) Standing
	// Draft records that objectID competes in the match of stadium.
	Draft(objectID int32, stadium int)
	// Tell sends objectID m.
	Tell(objectID int32, m Message)
	// Announce sends m to every player inside stadium.
	Announce(stadium int, m Message)
	// Enter brings p into stadium, onto its side's spawn, as a competitor
	// not yet fighting; false when p is offline.
	Enter(stadium int, p Participant) bool
	// Strip readies objectID to compete: its effects, clan and hero skills,
	// summon's effects or pet, other players' cubics, party, barred items
	// and shots go, and it is healed.
	Strip(objectID int32)
	// Buff gives objectID the match's opening buffs.
	Buff(objectID int32)
	// Heal fills objectID's CP, HP and MP.
	Heal(objectID int32)
	// Fight lets objectID fight its opponent.
	Fight(objectID int32)
	// ShowCompetitors shows the stadium both competitors' gauges.
	ShowCompetitors(stadium int, one, two int32)
	// RefreshCombat sets everyone inside stadium in or out of combat as its
	// fight runs or not.
	RefreshCombat(stadium int)
	// Settle stops objectID's fight: actions, death and summon's actions
	// end and it is healed.
	Settle(objectID int32)
	// Release ends objectID's match standing and gives back what Strip
	// took that comes back.
	Release(objectID int32)
	// SendBack returns objectID to the town near where it entered.
	SendBack(objectID int32)
	// Reward gives objectID rewards.
	Reward(objectID int32, rewards []Reward)
	// Shout has every Grand Olympiad Manager shout text.
	Shout(text string)
}

// Countdown steps: the seconds each countdown announces.
var (
	arenaCountdown  = []int{120, 60, 30, 15, 10, 5, 4, 3, 2, 1, 0}
	battleCountdown = []int{60, 50, 40, 30, 20, 10, 5, 4, 3, 2, 1, 0}
	townCountdown   = []int{40, 30, 20, 10, 5, 4, 3, 2, 1, 0}
)

// buffCountdown is the second of the battle countdown the competitors are
// buffed and healed at.
const buffCountdown = 20

// stage is where a stadium's match stands.
type stage int

const (
	stageBegin stage = iota
	stageTeleToArena
	stageGameStarted
	stageBattleCountdown
	stageBattleStarted
	stageBattleInProgress
	stageGameStopped
	stageTeleToTown
	stageCleanup
	stageIdle
)

// Stadium runs one match at a time, on its own queue. mu guards the fields
// below it; the match's own steps, which only its queue runs, change them,
// and the game manager attaches a match to an idle stadium.
type Stadium struct {
	id    int
	games *games
	queue *sim.Queue

	mu        sync.Mutex
	stage     stage
	game      *Game
	countdown int
	announce  bool
}

// ID is the stadium's index among the stadiums.
func (s *Stadium) ID() int { return s.id }

// Game is the match the stadium runs, nil when idle.
func (s *Stadium) Game() *Game {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.game
}

// Running reports whether the stadium runs a match.
func (s *Stadium) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stage != stageIdle
}

// GameStarted reports whether the stadium's match has brought its
// competitors in, from then until they are sent back.
func (s *Stadium) GameStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stage >= stageGameStarted && s.stage <= stageCleanup
}

// BattleStarted reports whether the stadium's competitors are fighting.
func (s *Stadium) BattleStarted() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stage == stageBattleInProgress
}

// attach starts g in the idle stadium; a running one keeps its match.
func (s *Stadium) attach(g *Game) bool {
	s.mu.Lock()
	if s.stage != stageIdle {
		s.mu.Unlock()
		return false
	}
	s.game, s.stage, s.announce = g, stageBegin, false
	s.mu.Unlock()
	s.queue.Post(s.run)
	return true
}

// takeAnnounce reports, once, that the stadium's match just brought its
// competitors in.
func (s *Stadium) takeAnnounce() (*Game, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.announce {
		return nil, false
	}
	s.announce = false
	return s.game, s.game != nil
}

// set moves the stadium to st with countdown cd.
func (s *Stadium) set(st stage, cd int) {
	s.mu.Lock()
	s.stage, s.countdown = st, cd
	s.mu.Unlock()
}

// run runs the stadium's next step and schedules the one after.
func (s *Stadium) run() {
	if delay, ok := s.step(); ok {
		s.queue.After(delay, s.run)
	}
}

// step runs the stadium's match one step on and returns when the next step
// is due; ok is false once the match is over.
func (s *Stadium) step() (delay time.Duration, ok bool) {
	s.mu.Lock()
	st, g, cd := s.stage, s.game, s.countdown
	s.mu.Unlock()
	if g == nil || st == stageIdle {
		return 0, false
	}
	cfg, a := &s.games.o.cfg, s.games.arena
	seconds := 1
	switch st {
	case stageBegin:
		s.set(stageTeleToArena, cfg.WaitTime)
	case stageTeleToArena:
		s.tellCompetitors(g, Message{Kind: MessageEnterStadiumIn, Number: cd})
		next, d := countdownStep(arenaCountdown, cd)
		seconds = d
		if next <= 0 {
			s.set(stageGameStarted, next)
		} else {
			s.set(stageTeleToArena, next)
		}
	case stageGameStarted:
		if !s.startGame(g) {
			s.set(stageGameStopped, cd)
			break
		}
		next, d := countdownStep(battleCountdown, cfg.WaitBattle)
		seconds = d
		s.set(stageBattleCountdown, next)
	case stageBattleCountdown:
		a.Announce(s.id, Message{Kind: MessageGameStartsIn, Number: cd})
		if cd == buffCountdown {
			s.eachOnline(g, a.Buff)
			s.eachOnline(g, a.Heal)
		}
		next, d := countdownStep(battleCountdown, cd)
		seconds = d
		if next <= 0 {
			s.set(stageBattleStarted, next)
		} else {
			s.set(stageBattleCountdown, next)
		}
	case stageBattleStarted:
		s.eachOnline(g, a.Heal)
		g.ResetDamage()
		// The fight runs before it starts, so the stadium's combat state
		// it refreshes reads it running.
		s.set(stageBattleInProgress, 0)
		if !s.startBattle(g) {
			s.set(stageGameStopped, 0)
		}
	case stageBattleInProgress:
		cd += 1000
		if s.haveWinner(g) || int64(cd) > cfg.BattleMillis {
			s.set(stageGameStopped, cd)
		} else {
			s.set(stageBattleInProgress, cd)
		}
	case stageGameStopped:
		next, d := countdownStep(townCountdown, cfg.WaitEnd)
		seconds = d
		s.set(stageTeleToTown, next)
		s.stopGame(g)
	case stageTeleToTown:
		s.tellCompetitors(g, Message{Kind: MessageMovedToTownIn, Number: cd})
		next, d := countdownStep(townCountdown, cd)
		seconds = d
		if next <= 0 {
			s.set(stageCleanup, next)
		} else {
			s.set(stageTeleToTown, next)
		}
	case stageCleanup:
		s.cleanup(g)
		s.mu.Lock()
		s.stage, s.game = stageIdle, nil
		s.mu.Unlock()
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

// countdownStep moves a countdown standing at cd to the next of times below
// it, returning that second and how many seconds away it is. Past the last
// second but zero the countdown ends: -1, a second away.
func countdownStep(times []int, cd int) (next, seconds int) {
	for _, t := range times[:len(times)-1] {
		if t < cd {
			return t, cd - t
		}
	}
	return -1, 1
}

// tellCompetitors sends m to both competitors of g that are online.
func (s *Stadium) tellCompetitors(g *Game, m Message) {
	for _, p := range g.players {
		s.games.arena.Tell(p.ObjectID, m)
	}
}

// eachOnline runs fn for each competitor of g that is online.
func (s *Stadium) eachOnline(g *Game, fn func(objectID int32)) {
	for _, p := range g.players {
		if s.games.arena.Online(p.ObjectID) {
			fn(p.ObjectID)
		}
	}
}

// startGame checks both competitors may still compete, brings them in and
// readies them; false when one defected or could not be brought in.
func (s *Stadium) startGame(g *Game) bool {
	if s.checkDefection(g) {
		return false
	}
	a := s.games.arena
	ok := true
	for _, p := range g.players {
		ok = a.Enter(s.id, p) && ok
	}
	if !ok {
		return false
	}
	s.eachOnline(g, a.Strip)
	s.mu.Lock()
	s.announce = true
	s.mu.Unlock()
	return true
}

// checkDefection marks each competitor of g that can no longer compete a
// defector and tells its opponent why; it reports whether either did.
func (s *Stadium) checkDefection(g *Game) bool {
	one, two := g.players[0], g.players[1]
	if m, unfit := s.unfit(one.ObjectID); unfit {
		g.Defect(one.ObjectID)
		s.games.arena.Tell(two.ObjectID, m)
	}
	if m, unfit := s.unfit(two.ObjectID); unfit {
		g.Defect(two.ObjectID)
		s.games.arena.Tell(one.ObjectID, m)
	}
	return g.defected()
}

// unfit reports whether objectID can no longer compete, with the message
// its opponent gets; the competitor itself is told why first when it is
// still there to hear it.
func (s *Stadium) unfit(objectID int32) (Message, bool) {
	a := s.games.arena
	f := a.Fitness(objectID)
	var own Message
	switch {
	case !f.Online, f.Detached:
		return Message{Kind: MessageOpponentLeft}, true
	case f.Observer:
		return Message{Kind: MessageOpponentUnfit}, true
	case f.Dead:
		own = Message{Kind: MessageUnfitDead}
	case f.Subclass:
		own = Message{Kind: MessageUnfitSubclass}
	case f.CursedWeaponID != 0:
		own = Message{Kind: MessageUnfitCursedWeapon, Number: int(f.CursedWeaponID)}
	case f.Overweight:
		own = Message{Kind: MessageUnfitOverweight}
	default:
		return Message{}, false
	}
	a.Tell(objectID, own)
	return Message{Kind: MessageOpponentUnfit}, true
}

// fighting reports whether both competitors of g are online and neither
// left the match.
func (s *Stadium) fighting(g *Game) bool {
	for i, p := range g.players {
		if gone, _ := g.flags(i); gone || !s.games.arena.Online(p.ObjectID) {
			return false
		}
	}
	return true
}

// startBattle starts the fight: both competitors must still be there. The
// stadium is shown both competitors, told the fight started, and set in
// combat.
func (s *Stadium) startBattle(g *Game) bool {
	if !s.fighting(g) {
		return false
	}
	a := s.games.arena
	g.Start(s.queue.Now())
	a.Fight(g.players[0].ObjectID)
	a.Fight(g.players[1].ObjectID)
	a.ShowCompetitors(s.id, g.players[0].ObjectID, g.players[1].ObjectID)
	a.Announce(s.id, Message{Kind: MessageGameStarted})
	a.RefreshCombat(s.id)
	return true
}

// haveWinner reports whether the fight is decided: a competitor left, or
// one is dead or no longer competes in this stadium.
func (s *Stadium) haveWinner(g *Game) bool {
	if !s.fighting(g) {
		return true
	}
	for _, p := range g.players {
		if st := s.games.arena.Standing(p.ObjectID); st.GameID != s.id || st.Dead {
			return true
		}
	}
	return false
}

// stopGame judges the match, tells its result, rewards its winner, takes
// the stadium out of combat and stops the competitors still in it.
func (s *Stadium) stopGame(g *Game) {
	a := s.games.arena
	out := g.Judge(s.queue.Now(), a.Standing)
	for _, m := range out.Messages {
		switch m.Kind {
		case ResultWon:
			a.Announce(s.id, Message{Kind: MessageWon, Name: m.Name})
		case ResultTie:
			a.Announce(s.id, Message{Kind: MessageTie})
		case ResultPointsGained:
			s.tellCompetitors(g, Message{Kind: MessagePointsGained, Name: m.Name, Number: m.Points})
		case ResultPointsLost:
			s.tellCompetitors(g, Message{Kind: MessagePointsLost, Name: m.Name, Number: m.Points})
		}
	}
	if out.Winner != 0 && len(out.Reward) > 0 && a.Online(out.Winner) {
		a.Reward(out.Winner, out.Reward)
	}
	a.RefreshCombat(s.id)
	s.eachStaying(g, true, a.Settle)
}

// cleanup gives the competitors still in the match their standing back and
// returns them.
func (s *Stadium) cleanup(g *Game) {
	a := s.games.arena
	s.eachStaying(g, true, a.Release)
	s.eachStaying(g, false, a.SendBack)
}

// eachStaying runs fn for each competitor of g that is online and neither
// defected nor left; inStadium further asks that it still competes in this
// stadium.
func (s *Stadium) eachStaying(g *Game, inStadium bool, fn func(objectID int32)) {
	a := s.games.arena
	for i, p := range g.players {
		gone, defected := g.flags(i)
		if gone || defected || !a.Online(p.ObjectID) {
			continue
		}
		if inStadium && a.Standing(p.ObjectID).GameID != s.id {
			continue
		}
		fn(p.ObjectID)
	}
}
