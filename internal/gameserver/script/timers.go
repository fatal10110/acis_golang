package script

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// timers is the registry of every script's timers. A timer is keyed by its
// script, its name and the NPC and player it is bound to, by identity: a
// nil NPC or player is bound to none and matches only timers bound to
// none. An NPC is identified by its spawn slot, so a timer bound to an NPC
// keeps its key across the slot's respawns. Every field is guarded by mu,
// which is never held while a hook runs.
type timers struct {
	r *Registry
	// engine is the queue a timer bound to no NPC or player queue runs on.
	engine *sim.Queue

	mu       sync.Mutex
	byScript map[*Script]map[timerKey]*scriptTimer
	byNPC    map[any]map[*scriptTimer]struct{}
	byPlayer map[any]map[*scriptTimer]struct{}
}

type timerKey struct {
	name string
	// npc and player are the identities of the bound NPC and player, nil
	// for none.
	npc, player any
}

// scriptTimer is one registered timer. Its fields are guarded by
// timers.mu.
type scriptTimer struct {
	s   *Script
	key timerKey
	npc *NPC
	pl  *Player
	// period is the fixed rate; 0 for a one-shot.
	period time.Duration
	// due is the deadline of the armed firing.
	due time.Time
	// queue is the timer's home queue, where its hook runs.
	queue *sim.Queue
	armed *sim.Timer
	// decays reports that the timer stops when its NPC decays: its script
	// is a behavior bound to the NPC's template.
	decays bool
}

func newTimers(r *Registry, engine *sim.Queue) *timers {
	return &timers{
		r: r, engine: engine,
		byScript: map[*Script]map[timerKey]*scriptTimer{},
		byNPC:    map[any]map[*scriptTimer]struct{}{},
		byPlayer: map[any]map[*scriptTimer]struct{}{},
	}
}

// timerNPC is a live NPC as the timers see it.
type timerNPC interface {
	NpcID() int
	Queue() *sim.Queue
	Decayed() bool
	Scratch() *npc.Scratch
}

// timerPlayer is a live player as the timers see it.
type timerPlayer interface {
	Queue() *sim.Queue
	Detaching() bool
}

// npcIdentity is the identity of the NPC n handles: its spawn slot's
// script memory, the same for every NPC the slot spawns; nil for none.
func npcIdentity(n *NPC) any {
	self := n.combatant()
	if self == nil {
		return nil
	}
	if live, ok := self.(timerNPC); ok {
		return live.Scratch()
	}
	return self
}

// playerIdentity is the identity of the player p handles: its character,
// whichever value the handle holds; nil for none.
func playerIdentity(p *Player) any {
	self := p.combatant()
	if self == nil {
		return nil
	}
	if h, ok := self.(player.CharacterHolder); ok {
		return h.PlayerCharacter()
	}
	return self
}

// StartTimer starts the script's one-shot timer name bound to n and p,
// either of which may be nil, firing the timer hook once d has passed. It
// reports false, starting nothing, when the script already has a timer of
// that name bound to the same NPC and player.
func (s *Script) StartTimer(name string, n *NPC, p *Player, d time.Duration) bool {
	return s.timers.start(s, name, n, p, d, 0)
}

// StartTimerAtFixedRate is StartTimer for a timer that fires once initial
// has passed and then every period after that first firing, on a fixed
// grid, until it is cancelled. A period that is not positive starts a
// one-shot timer.
func (s *Script) StartTimerAtFixedRate(name string, n *NPC, p *Player, initial, period time.Duration) bool {
	return s.timers.start(s, name, n, p, initial, max(period, 0))
}

// HasTimer reports whether the script has a timer name bound to n and p
// pending. A one-shot timer is no longer pending once its hook runs.
func (s *Script) HasTimer(name string, n *NPC, p *Player) bool {
	ts := s.timers
	ts.mu.Lock()
	defer ts.mu.Unlock()
	_, ok := ts.byScript[s][timerKey{name, npcIdentity(n), playerIdentity(p)}]
	return ok
}

// CancelTimer cancels the script's timer name bound to n and p.
func (s *Script) CancelTimer(name string, n *NPC, p *Player) {
	s.CancelTimers(TimerName(name), TimerNPC(n), TimerPlayer(p))
}

// CancelTimers cancels every timer of the script that matches all of
// filters; with no filter, every timer of the script.
func (s *Script) CancelTimers(filters ...TimerFilter) {
	ts := s.timers
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for key, t := range ts.byScript[s] {
		if matchAll(key, filters) {
			ts.remove(t)
		}
	}
}

// TimerFilter selects timers by their name, NPC or player.
type TimerFilter struct {
	field timerField
	name  string
	id    any
}

type timerField uint8

const (
	byName timerField = iota
	byNPC
	byPlayer
)

// TimerName selects the timers named name.
func TimerName(name string) TimerFilter { return TimerFilter{field: byName, name: name} }

// TimerNPC selects the timers bound to n; a nil n, the timers bound to no
// NPC.
func TimerNPC(n *NPC) TimerFilter { return TimerFilter{field: byNPC, id: npcIdentity(n)} }

// TimerPlayer selects the timers bound to p; a nil p, the timers bound to
// no player.
func TimerPlayer(p *Player) TimerFilter { return TimerFilter{field: byPlayer, id: playerIdentity(p)} }

func matchAll(key timerKey, filters []TimerFilter) bool {
	for _, f := range filters {
		switch f.field {
		case byName:
			if key.name != f.name {
				return false
			}
		case byNPC:
			if key.npc != f.id {
				return false
			}
		case byPlayer:
			if key.player != f.id {
				return false
			}
		}
	}
	return true
}

// start registers and arms one timer; see StartTimerAtFixedRate.
//
// The timer runs on its home queue: the NPC's when the script is a
// behavior bound to the NPC's template and the NPC is alive, else the
// player's when one is bound, else the engine queue.
func (ts *timers) start(s *Script, name string, n *NPC, p *Player, d, period time.Duration) bool {
	t := &scriptTimer{s: s, key: timerKey{name, npcIdentity(n), playerIdentity(p)}, npc: n, pl: p, period: period}
	live, _ := n.combatant().(timerNPC)
	if live != nil {
		t.decays = ts.r.behaviorOn(s, int32(live.NpcID()))
	}
	pl, _ := p.combatant().(timerPlayer)
	// The removals this timer is subject to, read before it registers:
	// the decay of its NPC when it decays with it, and the departure of its
	// player, whatever its home queue. A removal already under way here
	// came before the start, and does not take the timer.
	watchDecay := t.decays && !live.Decayed()
	watchDetach := pl != nil && !pl.Detaching()
	onNPCQueue := false
	switch {
	case t.decays && !n.combatant().Dead():
		t.queue, onNPCQueue = live.Queue(), true
	case pl != nil && !pl.Detaching():
		t.queue = pl.Queue()
	}
	if t.queue == nil {
		t.queue = ts.engine
	}
	if t.queue == nil {
		panic("script: a timer bound to no live NPC or player needs the engine queue")
	}

	ts.mu.Lock()
	set := ts.byScript[s]
	if _, dup := set[t.key]; dup {
		ts.mu.Unlock()
		return false
	}
	if set == nil {
		set = map[timerKey]*scriptTimer{}
		ts.byScript[s] = set
	}
	set[t.key] = t
	index(ts.byNPC, t.key.npc, t)
	index(ts.byPlayer, t.key.player, t)
	t.due = t.queue.Now().Add(max(d, 0))
	ts.arm(t)
	ts.mu.Unlock()

	// A start that raced a removal it is subject to may have registered
	// after that removal ran: it is taken as started just before it, and
	// removed, rather than left holding its key, on a closed queue or bound
	// to a departed player. A timer on the NPC's queue never outlives its
	// decay.
	if ((watchDecay || onNPCQueue) && live.Decayed()) || (watchDetach && pl.Detaching()) {
		ts.mu.Lock()
		if ts.byScript[s][t.key] == t {
			ts.remove(t)
		}
		ts.mu.Unlock()
	}
	return true
}

func index(m map[any]map[*scriptTimer]struct{}, id any, t *scriptTimer) {
	if id == nil {
		return
	}
	set := m[id]
	if set == nil {
		set = map[*scriptTimer]struct{}{}
		m[id] = set
	}
	set[t] = struct{}{}
}

func unindex(m map[any]map[*scriptTimer]struct{}, id any, t *scriptTimer) {
	if set := m[id]; set != nil {
		delete(set, t)
		if len(set) == 0 {
			delete(m, id)
		}
	}
}

// arm schedules t's next firing at t.due. ts.mu is held.
func (ts *timers) arm(t *scriptTimer) {
	t.armed = t.queue.After(max(t.due.Sub(t.queue.Now()), 0), func() { ts.fire(t) })
}

// remove takes t out of the registry and stops its armed firing. ts.mu is
// held.
func (ts *timers) remove(t *scriptTimer) {
	set := ts.byScript[t.s]
	delete(set, t.key)
	if len(set) == 0 {
		delete(ts.byScript, t.s)
	}
	unindex(ts.byNPC, t.key.npc, t)
	unindex(ts.byPlayer, t.key.player, t)
	if t.armed != nil {
		t.armed.Stop()
	}
}

// fire runs one firing of t on its home queue. A one-shot leaves the
// registry before its hook runs, so the hook may start the same timer
// again; a fixed-rate timer is armed for its next deadline after the hook,
// unless the hook cancelled it. Neither the NPC nor the player is checked
// for being alive.
func (ts *timers) fire(t *scriptTimer) {
	ts.mu.Lock()
	if ts.byScript[t.s][t.key] != t {
		ts.mu.Unlock()
		return
	}
	if t.period == 0 {
		ts.remove(t)
	}
	ts.mu.Unlock()

	s := t.s
	res := ts.r.answer(s, hookTimer, func() string { return s.Hooks.Timer(s, Timer{Name: t.key.name, NPC: t.npc, Player: t.pl}) })
	ts.show(t, res)

	if t.period == 0 {
		return
	}
	ts.mu.Lock()
	if ts.byScript[s][t.key] == t {
		t.due = t.due.Add(t.period)
		ts.arm(t)
	}
	ts.mu.Unlock()
}

// show answers the timer's player with res. Only a timer bound to a player
// shows anything.
//
// Showing an answer is the dialog path's (#130); until it lands a
// non-empty answer for a player is logged and dropped.
func (ts *timers) show(t *scriptTimer, res Result) {
	if t.key.player == nil || res.Kind == ResultNone || res.Kind == ResultAborted {
		return
	}
	ts.r.log.Warn().Str("script", t.s.path).Str("timer", t.key.name).Msg("script: timer answer not shown")
}

// npcDecayed removes the timers bound to the NPC with identity id whose
// script is a behavior bound to its template.
func (ts *timers) npcDecayed(id any) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for t := range ts.byNPC[id] {
		if t.decays {
			ts.remove(t)
		}
	}
}

// playerDetached removes every timer bound to the player with identity id,
// whatever its script.
func (ts *timers) playerDetached(id any) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for t := range ts.byPlayer[id] {
		ts.remove(t)
	}
}

// PlayerDetached runs as c leaves the world: every script timer bound to c
// stops.
func (r *Registry) PlayerDetached(c *player.Character) {
	r.timers.playerDetached(c)
}

var (
	_ timerNPC    = (*npc.Hostile)(nil)
	_ timerNPC    = (*npc.Folk)(nil)
	_ timerPlayer = (*player.Character)(nil)
)
