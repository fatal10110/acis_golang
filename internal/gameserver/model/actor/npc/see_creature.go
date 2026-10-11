package npc

import (
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// DefaultSeeRange is the shipped npcs.properties DefaultSeeRange: how far
// an NPC other than a raid or grand boss sees around itself.
const DefaultSeeRange = 450

// seeMaxZ is the largest height difference the raid tick scan, the
// look-neighbour scan and the teleport re-fire see across.
const seeMaxZ = 500

// lookNeighborInterval is the least time between two look-neighbour scans
// of one NPC.
const lookNeighborInterval = 30 * time.Second

// sight is what an NPC keeps of the creatures it saw: the playables its
// tick scan has already told its see-creature scripts about. An NPC lives
// once, so a new life starts with an empty set.
type sight struct {
	mu sync.Mutex
	// seen holds the playables told about, by the value the world tracks;
	// a playable that leaves the world stays in it, as a new object for
	// the same character is not the same playable.
	seen map[attackable.Combatant]struct{}
}

func (s *sight) hasSeen(c attackable.Combatant) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[c]
	return ok
}

func (s *sight) markSeen(c attackable.Combatant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen == nil {
		s.seen = map[attackable.Combatant]struct{}{}
	}
	s.seen[c] = struct{}{}
}

func (s *sight) forget(c attackable.Combatant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.seen, c)
}

// seer is an NPC whose surroundings its see-creature scripts are told
// about: a *Hostile or a *Folk.
type seer interface {
	attackable.Combatant
	world.Tracked
	npcID() int32
	raidBoss() bool
	canSeeThrough() bool
	seeRange() int
	sightState() *sight
	worldState() *world.State
	scriptHooks() ScriptHooks
	now() time.Time
	// raiseSeeCreature runs the NPC's see-creature hooks for c.
	raiseSeeCreature(c attackable.Combatant)
}

func (h *Hostile) npcID() int32             { return int32(h.NpcID()) }
func (h *Hostile) raidBoss() bool           { return h.RaidBoss() }
func (h *Hostile) canSeeThrough() bool      { return h.Instance.Template.CanSeeThrough }
func (h *Hostile) sightState() *sight       { return &h.sight }
func (h *Hostile) worldState() *world.State { return h.world }
func (h *Hostile) scriptHooks() ScriptHooks { return h.scripts }

// seeRange is how far h sees: its template aggro range for a raid or
// grand boss, the configured see range for any other NPC.
func (h *Hostile) seeRange() int {
	if h.RaidBoss() {
		return h.Instance.Template.AggroRange
	}
	if r := h.aiSettings().SeeRange; r > 0 {
		return r
	}
	return DefaultSeeRange
}

func (h *Hostile) now() time.Time {
	if q := h.Queue(); q != nil {
		return q.Now()
	}
	return time.Now()
}

func (h *Hostile) raiseSeeCreature(c attackable.Combatant) {
	h.scripts.HostileSeeCreature(h, c)
}

func (f *Folk) npcID() int32             { return int32(f.NpcID()) }
func (f *Folk) raidBoss() bool           { return false }
func (f *Folk) canSeeThrough() bool      { return f.Instance.Template.CanSeeThrough }
func (f *Folk) sightState() *sight       { return &f.sight }
func (f *Folk) worldState() *world.State { return f.world }
func (f *Folk) scriptHooks() ScriptHooks { return f.scripts }

// seeRange is how far f sees: the configured see range.
func (f *Folk) seeRange() int {
	if f.seeDistance > 0 {
		return f.seeDistance
	}
	return DefaultSeeRange
}

func (f *Folk) raiseSeeCreature(c attackable.Combatant) {
	f.scripts.FolkSeeCreature(f, c)
}

// flyer is a creature that can fly: a player on a wyvern.
type flyer interface {
	Flying() bool
}

// actingPlayer is c's acting player: c itself, or the owner of a summon.
// It is nil for an NPC.
func actingPlayer(c attackable.Combatant) attackable.Combatant {
	switch c.Kind() {
	case actor.KindPlayer:
		return c
	case actor.KindSummon:
		if owner, ok := c.Owner(); ok {
			return owner
		}
	}
	return nil
}

// unseenPlayer reports whether p, an acting player, is kept from every
// see-creature scan: spawn-protected or flying.
func unseenPlayer(p attackable.Combatant) bool {
	if p.SpawnProtected() {
		return true
	}
	f, ok := p.(flyer)
	return ok && f.Flying()
}

// hiddenFromSight reports whether c's acting player keeps c from an NPC's
// own scans: spawn-protected, flying or invisible.
func hiddenFromSight(c attackable.Combatant) bool {
	p := actingPlayer(c)
	return p != nil && (unseenPlayer(p) || attackable.HiddenActingPlayer(p))
}

// playable reports whether c is a player or a summon.
func playable(c attackable.Combatant) bool {
	k := c.Kind()
	return k == actor.KindPlayer || k == actor.KindSummon
}

// inSight reports whether a and b stand strictly within rng of each other
// in 3D, centre to centre.
func inSight(a, b attackable.Combatant, rng int) bool {
	ax, ay, az := a.Position()
	bx, by, bz := b.Position()
	dx := int64(ax) - int64(bx)
	dy := int64(ay) - int64(by)
	dz := int64(az) - int64(bz)
	return dx*dx+dy*dy+dz*dz < int64(rng)*int64(rng)
}

// zApart is the height difference between a and b.
func zApart(a, b attackable.Combatant) int {
	_, _, az := a.Position()
	_, _, bz := b.Position()
	if az > bz {
		return az - bz
	}
	return bz - az
}

// seesCreatures reports whether n has see-creature scripts to tell.
func seesCreatures(n seer) bool {
	hooks := n.scriptHooks()
	return hooks != nil && n.worldState() != nil && hooks.SeesCreatures(n.npcID())
}

// knownCombatants is every creature in n's surrounding regions, n
// excluded, taken before any hook runs.
func knownCombatants(n seer) []attackable.Combatant {
	var out []attackable.Combatant
	n.worldState().ForEachKnown(n, func(o world.Tracked) {
		if c, ok := o.(attackable.Combatant); ok {
			out = append(out, c)
		}
	})
	return out
}

// scanSight is the see-creature scan that opens n's periodic AI tick. It
// tells n's see-creature scripts, once, about each known playable that
// comes within its see range, and forgets one that leaves it so its return
// is told again. A raid or grand boss keeps no such memory: it tells about
// each playable in its see range and within 500 height on every tick that
// either of them moves, and, for one further apart in height, as any other
// NPC does. A spawn-protected, flying or invisible acting player is never
// told about, nor, unless n sees through it, a playable moving silently.
func scanSight(n seer) {
	if !seesCreatures(n) {
		return
	}
	raid := n.raidBoss()
	rng := n.seeRange()
	st := n.sightState()
	for _, c := range knownCombatants(n) {
		if !playable(c) || hiddenFromSight(c) {
			continue
		}
		inRange := inSight(n, c, rng)
		switch {
		case !raid && st.hasSeen(c):
			if !inRange {
				st.forget(c)
			}
		case raid && inRange && zApart(n, c) <= seeMaxZ:
			if c.IsMoving() || n.IsMoving() {
				n.raiseSeeCreature(c)
			}
		case inRange:
			if c.SilentMoving() && !n.canSeeThrough() {
				continue
			}
			st.markSeen(c)
			n.raiseSeeCreature(c)
		}
	}
}

// lookNeighbor tells n's see-creature scripts about every creature, NPCs
// included, within its see range (3D, collision radii included), at most
// once per 30 s: a call sooner after the last scan does nothing. A
// creature more than 500 apart in height is skipped, so is one whose acting
// player is spawn-protected, flying or invisible, and, unless n is a raid
// or grand boss or sees through it, one moving silently. The scan leaves
// the tick scan's memory alone.
func lookNeighbor(n seer, last *lookNeighborClock) {
	if !last.take(n.now()) || !seesCreatures(n) {
		return
	}
	var found []attackable.Combatant
	n.worldState().ForEachKnownInRadius(n, n.seeRange(), func(o world.Tracked) {
		if c, ok := o.(attackable.Combatant); ok {
			found = append(found, c)
		}
	})
	for _, c := range found {
		if hiddenFromSight(c) {
			continue
		}
		if !n.raidBoss() && c.SilentMoving() && !n.canSeeThrough() {
			continue
		}
		if zApart(n, c) > seeMaxZ {
			continue
		}
		n.raiseSeeCreature(c)
	}
}

// lookNeighborClock is when an NPC last looked around. The spawn slot
// keeps it, so it carries over the NPC's next lives.
type lookNeighborClock struct {
	mu   sync.Mutex
	last time.Time
}

// take reports whether a look-neighbour scan may run at now, and if so
// records now as the last one.
func (c *lookNeighborClock) take(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.last.IsZero() && now.Sub(c.last) < lookNeighborInterval {
		return false
	}
	c.last = now
	return true
}

// LookNeighbor tells h's see-creature scripts about the creatures around
// it, at most once per 30 s (see lookNeighbor).
func (h *Hostile) LookNeighbor() { lookNeighbor(h, &h.Scratch().lookNeighbor) }

// LookNeighbor tells f's see-creature scripts about the creatures around
// it, at most once per 30 s (see lookNeighbor).
func (f *Folk) LookNeighbor() { lookNeighbor(f, &f.Scratch().lookNeighbor) }

// ForceSeeCreature tells every NPC around c, a player or a summon that has
// just teleported or been summoned, about it: each NPC with c strictly
// within its see range, centre to centre, and at most 500 apart in height
// runs its see-creature hooks once. Nothing happens for any other creature,
// for one off the grid, or while c's acting player is spawn-protected or
// flying. Invisibility and silent moving do not hide c here, and no NPC's
// tick-scan memory changes. The hooks run on the caller's goroutine; the
// caller holds no lock.
func ForceSeeCreature(w *world.State, c attackable.Combatant) {
	t, ok := c.(world.Tracked)
	if w == nil || !ok || !playable(c) {
		return
	}
	if p := actingPlayer(c); p != nil && unseenPlayer(p) {
		return
	}
	var npcs []seer
	w.ForEachKnown(t, func(o world.Tracked) {
		if n, ok := o.(seer); ok {
			npcs = append(npcs, n)
		}
	})
	for _, n := range npcs {
		if n.scriptHooks() == nil {
			continue
		}
		if inSight(n, c, n.seeRange()) && zApart(n, c) <= seeMaxZ {
			n.raiseSeeCreature(c)
		}
	}
}
