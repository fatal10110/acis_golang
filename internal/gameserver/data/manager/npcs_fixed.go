package manager

import (
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// ErrNotPlaceable reports a template whose instance type has no live NPC
// model to spawn as, or an NPC the world could not take.
var ErrNotPlaceable = errors.New("npcs: npc cannot be placed")

// SpawnFixed places one NPC of tmpl as a standalone spawn of its own, not
// declared under any maker: at (x, y) on the ground height below z, facing
// heading. The spawn point is also where the NPC wanders around. Such a
// spawn never respawns: once its NPC decays or is deleted, the spawn is
// gone.
//
// Only a template a live hostile or civilian NPC models can be placed; any
// other reports ErrNotPlaceable and places nothing.
func (n *Npcs) SpawnFixed(tmpl *npc.Template, x, y, z, heading int) error {
	_, err := n.spawnFixed(tmpl, x, y, z, heading)
	return err
}

// spawnFixed is SpawnFixed, also returning the placed NPC's object id.
func (n *Npcs) spawnFixed(tmpl *npc.Template, x, y, z, heading int) (int32, error) {
	if tmpl == nil {
		return 0, ErrNotPlaceable
	}
	if probe := (&npc.Instance{Template: tmpl}); !npc.FolkKind(probe) && !npc.Attackable(probe) {
		return 0, fmt.Errorf("%w: npc %d instance type %q", ErrNotPlaceable, tmpl.ID, tmpl.Type)
	}
	at := location.Location{X: x, Y: y, Z: int(n.geo.Height(x, y, z))}
	key := fmt.Sprintf("fixed#%d", n.fixedSeq.Add(1))
	entry := spawn.Entry{NPCID: int32(tmpl.ID)}
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, entry: entry, fixed: true, at: at, heading: heading}
	n.mu.Unlock()

	n.instantiate(key, entry, tmpl, at, heading, fullHP, fullMP, nil)

	n.mu.Lock()
	defer n.mu.Unlock()
	// A slot already gone held an NPC that has decayed since; one still
	// without a live NPC never placed it.
	slot, ok := n.slot[key]
	if ok && slot.liveID == 0 {
		delete(n.slot, key)
		return 0, fmt.Errorf("%w: npc %d", ErrNotPlaceable, tmpl.ID)
	}
	return slot.liveID, nil
}

// DeleteFixed removes the live NPC id at once, with no corpse, when it was
// placed by SpawnFixed, and drops its spawn: it never comes back. It
// reports false, removing nothing, for any other NPC. The removal runs on
// the NPC's own queue.
func (n *Npcs) DeleteFixed(id int32) bool {
	n.mu.Lock()
	key, live := n.live[id]
	slot := n.slot[key]
	n.mu.Unlock()
	if !live || !slot.fixed {
		return false
	}
	obj, ok := n.state.Object(id)
	if !ok {
		return false
	}
	switch a := obj.(type) {
	case *npc.Hostile:
		a.DeleteMe()
	case *npc.Folk:
		remove := func() {
			if n.decay != nil {
				n.decay.Cancel(a)
			}
			a.Decay(n.state, n.RespawnHook(id))
		}
		if q := a.Queue(); q != nil {
			q.Post(remove)
		} else {
			remove()
		}
	default:
		return false
	}
	return true
}

// SpawnRecord describes the spawn a live NPC belongs to: a maker's, a
// master's private, or a standalone one.
type SpawnRecord struct {
	// Maker names the maker that declares the NPC; empty for any other
	// spawn.
	Maker string
	// MasterID is the object id of the master a private spawns around;
	// zero for any other spawn.
	MasterID int32
	// Fixed reports a standalone spawn SpawnFixed placed, at At facing
	// Heading.
	Fixed   bool
	At      location.Location
	Heading int
}

// SpawnOf returns the spawn of the live NPC id, false when id is no NPC
// this population spawned.
func (n *Npcs) SpawnOf(id int32) (SpawnRecord, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	key, live := n.live[id]
	if !live {
		return SpawnRecord{}, false
	}
	slot, ok := n.slot[key]
	if !ok {
		return SpawnRecord{}, false
	}
	rec := SpawnRecord{MasterID: slot.masterID, Fixed: slot.fixed, At: slot.at, Heading: slot.heading}
	if slot.maker != nil {
		rec.Maker = slot.maker.Name
	}
	return rec, true
}
