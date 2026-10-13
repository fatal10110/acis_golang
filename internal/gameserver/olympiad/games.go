package olympiad

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

const (
	// drawDelay is when the first draw of a competition window comes, and
	// drawPeriod how often the draws come after it.
	drawDelay  = 30 * time.Second
	drawPeriod = 30 * time.Second
	// announcePeriod is how often the announcing game manager looks for a
	// match to announce; every announceDrawEvery-th look also draws.
	announcePeriod    = 500 * time.Millisecond
	announceDrawEvery = 60
)

// games is the game manager: the stadiums and the draws that fill them
// during a competition window. Its timers and counter are owned by the
// Olympiad's queue and guarded by o.run.
type games struct {
	o        *Olympiad
	arena    Arena
	stadiums []*Stadium
	rnd      func(n int) int

	timer   *sim.Timer
	ticker  *sim.Ticker
	counter int
}

// BindStadiums gives the Olympiad its n stadiums: their matches are carried
// out by arena, each runs on a queue newQueue makes, and rnd(n) draws an
// index below n for the draws (random when nil). It runs once, before
// Start.
func (o *Olympiad) BindStadiums(n int, arena Arena, newQueue func(name string) *sim.Queue, rnd func(n int) int) {
	if rnd == nil {
		rnd = rand.IntN
	}
	g := &games{o: o, arena: arena, rnd: rnd}
	for i := range n {
		g.stadiums = append(g.stadiums, &Stadium{id: i, games: g, queue: newQueue(fmt.Sprintf("olympiad-stadium-%d", i)), stage: stageIdle})
	}
	o.games = g
}

// Stadium returns stadium id, false when there is none.
func (o *Olympiad) Stadium(id int) (*Stadium, bool) {
	if o.games == nil || id < 0 || id >= len(o.games.stadiums) {
		return nil, false
	}
	return o.games.stadiums[id], true
}

// CompetitorDamage adds damage the competitor objectID dealt in the match of
// stadium gameID, while both of that match's competitors are online.
func (o *Olympiad) CompetitorDamage(objectID int32, gameID int, damage int) {
	s, ok := o.Stadium(gameID)
	if !ok {
		return
	}
	g := s.Game()
	if g == nil {
		return
	}
	for _, p := range g.players {
		if !o.games.arena.Online(p.ObjectID) {
			return
		}
	}
	g.AddDamage(objectID, damage)
}

// startGames starts the draws of a competition window, replacing any
// running ones. The caller holds o.run.
func (o *Olympiad) startGames() {
	g := o.games
	if g == nil {
		return
	}
	g.stopTimers()
	if !o.cfg.AnnounceGames {
		g.ticker = o.queue.Every(drawPeriod, g.drawTick)
		return
	}
	g.timer = o.queue.After(drawDelay, func() {
		o.run.Lock()
		defer o.run.Unlock()
		if o.stopped {
			return
		}
		g.timer = nil
		g.announceLocked()
		g.ticker = o.queue.Every(announcePeriod, g.announceTick)
	})
}

// stopGames stops the draws; running matches go on. The caller holds o.run.
func (o *Olympiad) stopGames() {
	if o.games != nil {
		o.games.stopTimers()
	}
}

func (g *games) stopTimers() {
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	if g.ticker != nil {
		g.ticker.Stop()
		g.ticker = nil
	}
}

// drawTick is one draw of a window that announces nothing.
func (g *games) drawTick() {
	g.o.run.Lock()
	defer g.o.run.Unlock()
	if !g.o.stopped {
		g.draw()
	}
}

// announceTick is one look of the announcing game manager.
func (g *games) announceTick() {
	g.o.run.Lock()
	defer g.o.run.Unlock()
	if !g.o.stopped {
		g.announceLocked()
	}
}

// announceLocked is one look of the announcing game manager: every
// announceDrawEvery-th one draws, then each match that just brought its
// competitors in is shouted by the Grand Olympiad Managers. The caller
// holds o.run.
func (g *games) announceLocked() {
	g.counter++
	if g.counter%announceDrawEvery == 0 {
		g.draw()
	}
	for _, s := range g.stadiums {
		game, ok := s.takeAnnounce()
		if !ok {
			continue
		}
		free := ""
		if game.kind == NonClassed {
			free = "-free"
		}
		g.arena.Shout(fmt.Sprintf("Olympiad class%s individual match is going to begin in Arena %d in a moment.", free, s.id+1))
	}
}

// draw fills the idle stadiums from the waiting lists during the
// competition window: the even stadiums take classed matches while a class
// queue that held enough registrations at the start of the draw gives one,
// and every stadium left takes non-classed matches while the non-classed
// list holds enough. A classed queue found unable to give a pair ends the
// draw. The waiting lists outlive the window: no draw runs outside it to
// clear them.
func (g *games) draw() {
	o := g.o
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.period != Competition {
		return
	}
	classes := make([]int, 0, len(o.classed))
	for class, list := range o.classed {
		if len(list) >= o.cfg.ClassedParticipants {
			classes = append(classes, class)
		}
	}
	slices.Sort(classes)
	lists := make([][]int32, len(classes))
	ready := make([]*[]int32, len(classes))
	for i, class := range classes {
		lists[i] = o.classed[class]
		ready[i] = &lists[i]
	}
	defer func() {
		for i, class := range classes {
			o.classed[class] = lists[i]
		}
	}()

	classedOpen := true
	nonClassedOpen := len(o.nonClassed) >= o.cfg.NonClassedParticipants
	for i, s := range g.stadiums {
		if !s.Running() {
			if classedOpen && i%2 == 0 {
				game, stop := o.ClassedGame(i, ready, g.arena.Competitor, g.rnd)
				if stop {
					return
				}
				if game != nil {
					g.start(s, game)
					continue
				}
				classedOpen = false
			}
			if nonClassedOpen {
				if game := o.NonClassedGame(i, &o.nonClassed, g.arena.Competitor, g.rnd); game != nil {
					g.start(s, game)
					continue
				}
				nonClassedOpen = false
			}
		}
		if !classedOpen && !nonClassedOpen {
			return
		}
	}
}

// start drafts game's competitors into stadium s and runs it there.
func (g *games) start(s *Stadium, game *Game) {
	for _, p := range game.players {
		g.arena.Draft(p.ObjectID, s.id)
	}
	s.attach(game)
}

// inMatch reports whether objectID competes in a stadium's match.
func (g *games) inMatch(objectID int32) bool {
	for _, s := range g.stadiums {
		if game := s.Game(); game != nil && game.Contains(objectID) {
			return true
		}
	}
	return false
}

// close stops every stadium's queue.
func (g *games) close() {
	for _, s := range g.stadiums {
		s.queue.Close()
	}
}
