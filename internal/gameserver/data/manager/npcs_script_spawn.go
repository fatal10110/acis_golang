package manager

import (
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// scriptSpawnScatter is how far, on each axis, a script spawn with a random
// offset may land from the point it asks for.
const scriptSpawnScatter = 100

// scriptSpawnLift is how far above the ground a script spawn is dropped
// onto the ground from.
const scriptSpawnLift = 20

// AddSpawn places one NPC of template npcID as a standalone spawn that
// never respawns, as a script spawns one, and returns it. With scatter the
// NPC stands up to 100 from (x, y) on each axis, at the point geodata
// reaches from (x, y, z); without, at (x, y) on the ground below z. Either
// way it is then put on the ground below 20 above that point. A negative
// heading faces a random way. A positive despawn deletes the NPC once it
// has passed, unless it has left the world by then.
func (n *Npcs) AddSpawn(npcID int32, x, y, z, heading int, scatter bool, despawn time.Duration) (attackable.Combatant, error) {
	tmpl, ok := n.templates.Get(int(npcID))
	if !ok {
		return nil, fmt.Errorf("%w: %d", npc.ErrNoTemplate, npcID)
	}
	if scatter {
		tx := x + rnd.GetRange(-scriptSpawnScatter, scriptSpawnScatter)
		ty := y + rnd.GetRange(-scriptSpawnScatter, scriptSpawnScatter)
		at := n.geo.ValidLocation(x, y, z, tx, ty, z)
		x, y, z = at.X, at.Y, at.Z
	} else {
		z = int(n.geo.Height(x, y, z))
	}
	live, err := n.placeStandalone(tmpl, x, y, z+scriptSpawnLift, heading, nil)
	if err != nil {
		return nil, err
	}
	n.ScheduleDespawn(live, despawn)
	return live, nil
}

// SpawnSummoned is SpawnFixed for an NPC spawned for summoner: the NPC
// keeps summoner as the creature it was spawned for.
func (n *Npcs) SpawnSummoned(tmpl *npc.Template, x, y, z, heading int, summoner attackable.Combatant) error {
	_, err := n.placeStandalone(tmpl, x, y, z, heading, summoner)
	return err
}

// placeStandalone places one NPC of tmpl as a standalone spawn, as
// spawnFixed does, spawned for summoner, and returns it. A negative heading
// faces a random way.
func (n *Npcs) placeStandalone(tmpl *npc.Template, x, y, z, heading int, summoner attackable.Combatant) (attackable.Combatant, error) {
	if heading < 0 {
		heading = rnd.Get(65536)
	}
	return n.spawnStandalone(tmpl, x, y, z, heading, summoner)
}

// CreatePrivate places one NPC of template npcID as a private of master
// and returns it. The private stands around master, as a private of a
// spawn list entry does, or at *at facing heading when at is set, on the
// ground below it. Its script memory starts with params as its three spawn
// parameters. It never respawns on its own: a script brings it back
// (#3531). A positive despawn deletes it once it has passed, unless it has
// left the world by then.
//
// A private follows its master: it is deleted with it, and one that cannot
// respawn leaves master's privates when it decays. Only a hostile private
// can be placed yet (#3530); any other reports ErrNotPlaceable.
func (n *Npcs) CreatePrivate(master *npc.Hostile, npcID int32, at *location.Location, heading int, despawn time.Duration, params [3]int32) (*npc.Hostile, error) {
	tmpl, ok := n.templates.Get(int(npcID))
	if !ok {
		return nil, fmt.Errorf("%w: %d", npc.ErrNoTemplate, npcID)
	}
	return n.createPrivate(master, tmpl, at, heading, despawn, params, 0)
}

// CreatePrivates places master's privates: the ones its spawn entry
// declares, or, when it declares none, the ones its template lists. Each is
// placed as CreatePrivate places one, with the respawn delay and weight
// point its declaration gives. master first forgets the privates it had;
// they stay in the world. A declaration naming an NPC id no template has
// stops the placement with an error, the privates before it placed.
func (n *Npcs) CreatePrivates(master *npc.Hostile) error {
	privates := n.privatesOf(master)
	if len(privates) == 0 {
		return nil
	}
	master.ClearMinions()
	for _, p := range privates {
		tmpl, ok := n.templates.Get(int(p.NPCID))
		if !ok {
			return fmt.Errorf("%w: %d", npc.ErrNoTemplate, p.NPCID)
		}
		private, err := n.createPrivate(master, tmpl, nil, 0, 0, [3]int32{}, p.RespawnDelay)
		if err != nil {
			return err
		}
		private.Scratch().SetInt(npc.IntWeightPoint, int32(p.Weight))
	}
	return nil
}

// privatesOf returns the privates master's spawn entry declares, or its
// template's when the entry declares none.
func (n *Npcs) privatesOf(master *npc.Hostile) []spawn.Private {
	n.mu.Lock()
	key := n.live[master.ObjectID()]
	entry := n.slot[key].entry
	n.mu.Unlock()
	if len(entry.Privates) > 0 {
		return entry.Privates
	}
	declared := master.Instance.Template.Privates
	out := make([]spawn.Private, 0, len(declared))
	for _, p := range declared {
		out = append(out, spawn.Private{NPCID: int32(p.NpcID), Weight: p.Weight, RespawnDelay: p.RespawnDelay})
	}
	return out
}

// createPrivate places one private of tmpl for master under a slot of its
// own that respawns after respawn, when a script asks it to.
func (n *Npcs) createPrivate(master *npc.Hostile, tmpl *npc.Template, at *location.Location, heading int, despawn time.Duration, params [3]int32, respawn time.Duration) (*npc.Hostile, error) {
	if master == nil {
		return nil, fmt.Errorf("%w: npc %d has no master", ErrNotPlaceable, tmpl.ID)
	}
	if probe := (&npc.Instance{Template: tmpl}); npc.FolkKind(probe) || !npc.Attackable(probe) {
		return nil, fmt.Errorf("%w: private npc %d instance type %q", ErrNotPlaceable, tmpl.ID, tmpl.Type)
	}

	entry := spawn.Entry{NPCID: int32(tmpl.ID), RespawnDelay: respawn}
	slot := slotInfo{entry: entry, masterID: master.ObjectID(), tmpl: tmpl, scripted: true, memory: newSlotMemory(entry)}
	for i, p := range params {
		slot.memory.scratch.SetInt(npc.IntParam1+npc.IntSlot(i), p)
	}
	slot.key = fmt.Sprintf("private#%d", n.fixedSeq.Add(1))
	n.mu.Lock()
	n.slot[slot.key] = slot
	n.mu.Unlock()

	loc, face := n.privateSpawnLocation(master, tmpl), master.Heading()
	if at != nil {
		loc, face = location.Location{X: at.X, Y: at.Y, Z: int(n.geo.Height(at.X, at.Y, at.Z))}, heading
	}
	private, _ := n.instantiate(slot.key, entry, tmpl, loc, face, fullHP, fullMP, master).(*npc.Hostile)
	if private == nil {
		n.mu.Lock()
		delete(n.slot, slot.key)
		n.mu.Unlock()
		return nil, fmt.Errorf("%w: private npc %d", ErrNotPlaceable, tmpl.ID)
	}
	n.ScheduleDespawn(private, despawn)
	return private, nil
}

// ScheduleDespawn deletes the NPC live, on its own queue, once d has
// passed, unless it has left the world by then. A d that is not positive
// schedules nothing.
func (n *Npcs) ScheduleDespawn(live attackable.Combatant, d time.Duration) {
	if d <= 0 {
		return
	}
	switch o := live.(type) {
	case *npc.Hostile:
		o.Queue().After(d, func() {
			if !o.Decayed() {
				n.Remove(o)
			}
		})
	case *npc.Folk:
		if q := o.Queue(); q != nil {
			q.After(d, func() {
				if !o.Decayed() {
					n.RemoveFolk(o)
				}
			})
		}
	}
}

// RemoveFolk takes f out of the world at once, with no corpse, and answers
// its spawn slot as for a decayed corpse (npc.FolkRemover). It runs on the
// calling goroutine.
func (n *Npcs) RemoveFolk(f *npc.Folk) {
	n.decay.Cancel(f)
	f.Decay(n.state, n.RespawnHook(f.ObjectID()))
}
