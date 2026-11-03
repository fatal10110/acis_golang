package script

import (
	"slices"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// The operations the Rainbow Springs hall script adds (#3502): the war
// decrees its clans hand in, its notices to the clans it chose, the players
// on its battlefield, and the arena entry's effects strip and dismissal.

// MinClanLevel returns the clan level the hall sieges ask of a clan.
func (h *Hall) MinClanLevel() int32 { return int32(h.s.MinClanLevel()) }

// TellClans sends m to the members in the world of each of clanIDs, clan by
// clan; a clan no longer existing is skipped.
func (h *Hall) TellClans(clanIDs []int32, m SiegeMessage) { h.s.TellClans(clanIDs, m) }

// Decrees returns the Rainbow Springs war decrees clanID handed in, false
// when it handed in none.
func (h *Hall) Decrees(clanID int32) (int, bool) {
	n, ok := h.env.Halls.RainbowDecrees()[clanID]
	return n, ok
}

// SetDecrees records, and stores, the Rainbow Springs war decrees clanID
// handed in.
func (h *Hall) SetDecrees(clanID int32, count int) { h.env.Halls.AddRainbowDecrees(clanID, count) }

// RemoveDecrees drops, and stores, the Rainbow Springs war decrees clanID
// handed in, and returns them; false when it handed in none.
func (h *Hall) RemoveDecrees(clanID int32) (int, bool) {
	return h.env.Halls.RemoveRainbowDecrees(clanID)
}

// DropDecrees drops the Rainbow Springs war decrees clanID handed in,
// keeping their stored row until the decrees are cleared.
func (h *Hall) DropDecrees(clanID int32) { h.env.Halls.ForgetRainbowDecrees(clanID) }

// ClearDecrees drops, and stores, every clan's Rainbow Springs war decrees.
func (h *Hall) ClearDecrees() { h.env.Halls.ClearRainbowDecrees() }

// DecreeEntries returns each clan's Rainbow Springs war decrees, in the
// walk order of a hash map of clan ids holding them, put in it by clan id.
func (h *Hall) DecreeEntries() []IntMapEntry[int] {
	decrees := h.env.Halls.RainbowDecrees()
	ids := make([]int32, 0, len(decrees))
	for id := range decrees {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	walk := &IntMapCell[int]{}
	for _, id := range ids {
		walk.Set(id, decrees[id])
	}
	return walk.Entries()
}

// HallTask is a step a hall script armed to run later (Hall.Schedule).
type HallTask struct {
	t *sim.Timer
}

// Cancel stops the step unless it has run already. A nil task does
// nothing.
func (t *HallTask) Cancel() {
	if t != nil && t.t != nil {
		t.t.Stop()
	}
}

// Schedule runs fn once d has passed, on the halls' queue, as one
// invocation of the script the handle was made for: a panic aborts it and
// is logged, as a hook's is. Before the halls start nothing is armed and
// fn never runs. It returns the step, which Cancel stops.
func (h *Hall) Schedule(d time.Duration, fn func()) *HallTask {
	run := fn
	if sc := h.sc; sc != nil {
		run = func() { sc.runHall("task", fn) }
	}
	return &HallTask{t: h.s.After(d, run)}
}

// PlayersOnBattlefield returns the players standing on the hall siege's
// battlefield, dead or alive.
func (h *Hall) PlayersOnBattlefield() []*Player {
	f := h.s.Battlefield()
	if f == nil || h.env.World == nil {
		return nil
	}
	var out []*Player
	for _, a := range f.Occupants() {
		if _, ok := a.(zone.Player); !ok {
			continue
		}
		if o, ok := h.env.World.Player(a.ObjectID()); ok {
			if self, ok := o.(attackable.Combatant); ok {
				out = append(out, PlayerOf(self))
			}
		}
	}
	return out
}

// ClanLeader returns the clan's leader in the world, nil while offline.
func (c *ClanHandle) ClanLeader() *Player {
	leader := c.c.LeaderID()
	for _, p := range c.OnlineMembers() {
		if p.ObjectID() == leader {
			return p
		}
	}
	return nil
}

// StopAllEffects ends every effect on the player and shows it its effects
// once. From a hook on the player's own queue it runs inside the call; from
// any other it runs on the player's queue once the calling task is done.
func (p *Player) StopAllEffects() {
	c := p.character()
	if c.OnOwnQueue() {
		c.StopAllEffects()
		return
	}
	pl, ok := p.combatant().(timerPlayer)
	if !ok || pl.Detaching() {
		return
	}
	pl.Queue().Post(func() { c.RunOnOwnQueue(c.StopAllEffects) })
}

// unsummoner is a summon its owner can dismiss, on the summon's queue.
type unsummoner interface {
	Queue() *sim.Queue
	Unsummon()
}

// Unsummon dismisses the summon c, a pet or a servitor, as its owner's
// dismissal does, on the summon's queue: a living summon leaves the world,
// a dead one stays where it lies. A handle on anything but a summon panics,
// so the invocation aborts there.
func Unsummon(c Creature) {
	s, ok := combatantOf(c).(unsummoner)
	if !ok || !c.Summon() {
		panic("script: the dismissal of a handle on no summon")
	}
	if q := s.Queue(); q != nil {
		q.Post(s.Unsummon)
	}
}
