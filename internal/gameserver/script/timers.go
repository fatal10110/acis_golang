package script

import (
	"slices"
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
// keeps its key across the slot's respawns.
//
// A timer has one home, whose lock guards it: the list kept on its NPC's
// spawn slot (npcTimers) when it is bound to one, else its script's table
// (scriptTimers). No registry-wide lock is held to start, fire or remove a
// timer bound to an NPC. Locks nest in this order, and none is held while a
// hook runs: an NPC's list, a script's table, the player index (mu), then
// the timer's sim queue.
type timers struct {
	r *Registry
	// engine is the queue a timer bound to no NPC or player queue runs on.
	engine *sim.Queue

	// mu guards byPlayer, the timers bound to each player, by its identity.
	mu       sync.Mutex
	byPlayer map[any][]*scriptTimer
}

// scriptTimers is one script's table: its timers bound to no spawn slot,
// and the NPC lists that hold its other timers. Its fields are guarded by
// mu.
type scriptTimers struct {
	mu    sync.Mutex
	loose map[timerKey]*scriptTimer
	homes map[*npcTimers]struct{}
}

// npcTimers are the timers bound to one spawn slot's NPC, whatever their
// script, kept on the slot's Scratch for as long as the slot lives. Its
// fields are guarded by mu.
type npcTimers struct {
	mu   sync.Mutex
	list []*scriptTimer
}

// npcTimersOf returns the timer list kept on sc, made at its first timer.
func npcTimersOf(sc *npc.Scratch) *npcTimers {
	if h, _ := sc.ScriptTimers().(*npcTimers); h != nil {
		return h
	}
	h, _ := sc.KeepScriptTimers(&npcTimers{}).(*npcTimers)
	return h
}

// keptTimers returns the timer list kept on sc, nil before its first
// timer.
func keptTimers(sc *npc.Scratch) *npcTimers {
	h, _ := sc.ScriptTimers().(*npcTimers)
	return h
}

// find returns the timer of s with key in h. h.mu is held.
func (h *npcTimers) find(s *Script, key timerKey) *scriptTimer {
	for _, t := range h.list {
		if t.s == s && t.key == key {
			return t
		}
	}
	return nil
}

// holds reports whether h has a timer of s. h.mu is held.
func (h *npcTimers) holds(s *Script) bool {
	for _, t := range h.list {
		if t.s == s {
			return true
		}
	}
	return false
}

type timerKey struct {
	name string
	// npc and player are the identities of the bound NPC and player, nil
	// for none.
	npc, player any
}

// scriptTimer is one registered timer. Its fields are guarded by the lock
// of its home (scriptTimer.lock).
type scriptTimer struct {
	s   *Script
	key timerKey
	npc *NPC
	pl  *Player
	// home is the list of the spawn slot t is bound to, nil when t is in
	// its script's table. Set before the timer registers, it never changes.
	home *npcTimers
	// live reports that t is registered.
	live bool
	// period is the fixed rate; 0 for a one-shot.
	period time.Duration
	// due is the deadline of the armed firing.
	due time.Time
	// queue is the timer's home queue, where its hook runs.
	queue *sim.Queue
	// owner is the bound player when queue is that player's own queue, nil
	// otherwise. Set before the timer registers, it never changes.
	owner *player.Character
	// armed is the timer of t's firings, made at the start and re-armed
	// for each deadline of a fixed-rate timer.
	armed *sim.Timer
	// decays reports that the timer stops when its NPC decays: its script
	// is a behavior bound to the NPC's template.
	decays bool
}

// lock is the lock of t's home.
func (t *scriptTimer) lock() *sync.Mutex {
	if t.home != nil {
		return &t.home.mu
	}
	return &t.s.ownTimers.mu
}

func newTimers(r *Registry, engine *sim.Queue) *timers {
	return &timers{r: r, engine: engine, byPlayer: map[any][]*scriptTimer{}}
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
	key := timerKey{name, npcIdentity(n), playerIdentity(p)}
	if sc, ok := key.npc.(*npc.Scratch); ok {
		h := keptTimers(sc)
		if h == nil {
			return false
		}
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.find(s, key) != nil
	}
	own := s.ownTimers
	own.mu.Lock()
	defer own.mu.Unlock()
	_, ok := own.loose[key]
	return ok
}

// CancelTimer cancels the script's timer name bound to n and p.
func (s *Script) CancelTimer(name string, n *NPC, p *Player) {
	s.CancelTimers(TimerName(name), TimerNPC(n), TimerPlayer(p))
}

// CancelTimers cancels every timer of the script that matches all of
// filters; with no filter, every timer of the script.
func (s *Script) CancelTimers(filters ...TimerFilter) {
	ts, own := s.timers, s.ownTimers
	for _, f := range filters {
		if f.field != byNPC {
			continue
		}
		// Only the timers bound to that NPC can match.
		if sc, ok := f.id.(*npc.Scratch); ok {
			if h := keptTimers(sc); h != nil {
				h.mu.Lock()
				ts.cancelIn(h, s, filters)
				h.mu.Unlock()
			}
			return
		}
		own.mu.Lock()
		ts.cancelLoose(s, filters)
		own.mu.Unlock()
		return
	}
	own.mu.Lock()
	ts.cancelLoose(s, filters)
	homes := make([]*npcTimers, 0, len(own.homes))
	for h := range own.homes {
		homes = append(homes, h)
	}
	own.mu.Unlock()
	// A timer an NPC gains meanwhile started after the cancel.
	for _, h := range homes {
		h.mu.Lock()
		ts.cancelIn(h, s, filters)
		h.mu.Unlock()
	}
}

// cancelLoose removes the timers of s's table that match all of filters.
// s.ownTimers.mu is held.
func (ts *timers) cancelLoose(s *Script, filters []TimerFilter) {
	for key, t := range s.ownTimers.loose {
		if matchAll(key, filters) {
			ts.remove(t)
		}
	}
}

// cancelIn removes the timers of s in h that match all of filters. h.mu is
// held.
func (ts *timers) cancelIn(h *npcTimers, s *Script, filters []TimerFilter) {
	for i := len(h.list) - 1; i >= 0; i-- {
		if t := h.list[i]; t.s == s && matchAll(t.key, filters) {
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
		if h, ok := pl.(player.CharacterHolder); ok {
			t.owner = h.PlayerCharacter()
		}
	}
	if t.queue == nil {
		t.queue = ts.engine
	}
	if t.queue == nil {
		panic("script: a timer bound to no live NPC or player needs the engine queue")
	}
	if sc, ok := t.key.npc.(*npc.Scratch); ok {
		t.home = npcTimersOf(sc)
	}

	mu := t.lock()
	mu.Lock()
	if ts.registered(t) {
		mu.Unlock()
		return false
	}
	t.due = t.queue.Now().Add(max(d, 0))
	t.armed = t.queue.After(max(d, 0), func() { ts.fire(t) })
	ts.add(t)
	mu.Unlock()

	// A start that raced a removal it is subject to may have registered
	// after that removal ran: it is taken as started just before it, and
	// removed, rather than left holding its key, on a closed queue or bound
	// to a departed player. A timer on the NPC's queue never outlives its
	// decay.
	if ((watchDecay || onNPCQueue) && live.Decayed()) || (watchDetach && pl.Detaching()) {
		mu.Lock()
		if t.live {
			ts.remove(t)
		}
		mu.Unlock()
	}
	return true
}

// registered reports whether t's home has a timer of t's script and key.
// The lock of that home is held.
func (ts *timers) registered(t *scriptTimer) bool {
	if t.home != nil {
		return t.home.find(t.s, t.key) != nil
	}
	_, ok := t.s.ownTimers.loose[t.key]
	return ok
}

// add registers t in its home and in the player index. The lock of t's
// home is held.
func (ts *timers) add(t *scriptTimer) {
	t.live = true
	own := t.s.ownTimers
	if h := t.home; h != nil {
		first := !h.holds(t.s)
		h.list = append(h.list, t)
		if first {
			own.mu.Lock()
			if own.homes == nil {
				own.homes = map[*npcTimers]struct{}{}
			}
			own.homes[h] = struct{}{}
			own.mu.Unlock()
		}
	} else {
		if own.loose == nil {
			own.loose = map[timerKey]*scriptTimer{}
		}
		own.loose[t.key] = t
	}
	if id := t.key.player; id != nil {
		ts.mu.Lock()
		ts.byPlayer[id] = append(ts.byPlayer[id], t)
		ts.mu.Unlock()
	}
}

// remove takes t out of the registry and stops its armed firing. The lock
// of t's home is held.
func (ts *timers) remove(t *scriptTimer) {
	t.live = false
	own := t.s.ownTimers
	if h := t.home; h != nil {
		h.list = without(h.list, t)
		if !h.holds(t.s) {
			own.mu.Lock()
			delete(own.homes, h)
			own.mu.Unlock()
		}
	} else {
		delete(own.loose, t.key)
	}
	if id := t.key.player; id != nil {
		ts.mu.Lock()
		if list := without(ts.byPlayer[id], t); len(list) > 0 {
			ts.byPlayer[id] = list
		} else {
			delete(ts.byPlayer, id)
		}
		ts.mu.Unlock()
	}
	if t.armed != nil {
		t.armed.Stop()
	}
}

// without returns list less t, keeping the order of the others.
func without(list []*scriptTimer, t *scriptTimer) []*scriptTimer {
	if i := slices.Index(list, t); i >= 0 {
		return slices.Delete(list, i, i+1)
	}
	return list
}

// rearm schedules the next firing of the fixed-rate timer t, one period
// after the last deadline, or at once when that is already past. The lock
// of t's home is held. On a closed home queue nothing is armed, as a start
// there arms nothing.
func (ts *timers) rearm(t *scriptTimer) {
	t.due = t.due.Add(t.period)
	t.armed.Rearm(max(t.due.Sub(t.queue.Now()), 0))
}

// fire runs one firing of t on its home queue. A one-shot leaves the
// registry before its hook runs, so the hook may start the same timer
// again; a fixed-rate timer is armed for its next deadline after the hook,
// unless the hook cancelled it. Neither the NPC nor the player is checked
// for being alive. On the bound player's own queue the hook runs as a hook
// on that queue (TeleportTo).
func (ts *timers) fire(t *scriptTimer) {
	mu := t.lock()
	mu.Lock()
	if !t.live {
		mu.Unlock()
		return
	}
	if t.period == 0 {
		ts.remove(t)
	}
	mu.Unlock()

	s := t.s
	hook := func() {
		res := ts.r.answer(s, hookTimer, func() string { return s.Hooks.Timer(s, Timer{Name: t.key.name, NPC: t.npc, Player: t.pl}) })
		ts.show(t, res)
	}
	if t.owner != nil {
		t.owner.RunOnOwnQueue(hook)
	} else {
		hook()
	}

	if t.period == 0 {
		return
	}
	mu.Lock()
	if t.live {
		ts.rearm(t)
	}
	mu.Unlock()
}

// show sends the timer's player what its hook answered, through the
// timer's NPC when it has one: a page then ActionFailed, or a chat line.
// A timer bound to no player shows nothing.
func (ts *timers) show(t *scriptTimer, res Result) {
	if c, ok := t.key.player.(*player.Character); ok {
		t.s.show(c, t.npc, res)
	}
}

// npcDecayed removes the timers bound to the NPC of spawn slot sc whose
// script is a behavior bound to its template.
func (ts *timers) npcDecayed(sc *npc.Scratch) {
	h := keptTimers(sc)
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := len(h.list) - 1; i >= 0; i-- {
		if t := h.list[i]; t.decays {
			ts.remove(t)
		}
	}
}

// playerDetached removes every timer bound to the player with identity id,
// whatever its script. A timer that starts meanwhile sees the player
// detaching and removes itself (start).
func (ts *timers) playerDetached(id any) {
	ts.mu.Lock()
	list := slices.Clone(ts.byPlayer[id])
	ts.mu.Unlock()
	for _, t := range list {
		mu := t.lock()
		mu.Lock()
		if t.live {
			ts.remove(t)
		}
		mu.Unlock()
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
