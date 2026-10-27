package manager

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// ErrNotPlaceable reports a template whose instance type has no live NPC
// model to spawn as, or an NPC the world could not take.
var ErrNotPlaceable = errors.New("npcs: npc cannot be placed")

// SpawnFixed places one NPC of tmpl as a standalone spawn of its own, not
// declared under any maker: at (x, y) on the ground height below z, facing
// heading, with respawn as its declared respawn delay. The spawn point is
// also where the NPC wanders around. Such a spawn never respawns, whatever
// its delay: once its NPC decays or is deleted, the spawn is gone.
//
// Only a template a live hostile or civilian NPC models can be placed; any
// other reports ErrNotPlaceable and places nothing.
func (n *Npcs) SpawnFixed(tmpl *npc.Template, x, y, z, heading int, respawn time.Duration) error {
	_, err := n.spawnStandalone(tmpl, x, y, z, heading, respawn, nil, 0, 0)
	return err
}

// SpawnFixedFolk is SpawnFixed with no respawn delay for a civilian NPC,
// returning it. A template of any other kind reports ErrNotPlaceable.
func (n *Npcs) SpawnFixedFolk(tmpl *npc.Template, x, y, z, heading int) (*npc.Folk, error) {
	if tmpl == nil || !npc.FolkKind(&npc.Instance{Template: tmpl}) {
		return nil, ErrNotPlaceable
	}
	live, err := n.spawnStandalone(tmpl, x, y, z, heading, 0, nil, 0, 0)
	if err != nil {
		return nil, err
	}
	f, _ := live.(*npc.Folk)
	return f, nil
}

// SpawnFixedHostile is SpawnFixed with no respawn delay for a hostile NPC,
// returning it. A template of any other kind reports ErrNotPlaceable.
func (n *Npcs) SpawnFixedHostile(tmpl *npc.Template, x, y, z, heading int) (*npc.Hostile, error) {
	if tmpl == nil || !npc.Attackable(&npc.Instance{Template: tmpl}) {
		return nil, ErrNotPlaceable
	}
	live, err := n.spawnStandalone(tmpl, x, y, z, heading, 0, nil, 0, 0)
	if err != nil {
		return nil, err
	}
	h, ok := live.(*npc.Hostile)
	if !ok {
		return nil, ErrNotPlaceable
	}
	return h, nil
}

// spawnFixed is SpawnFixed with no respawn delay, also returning the placed
// NPC's object id.
func (n *Npcs) spawnFixed(tmpl *npc.Template, x, y, z, heading int) (int32, error) {
	live, err := n.spawnStandalone(tmpl, x, y, z, heading, 0, nil, 0, 0)
	if err != nil {
		return 0, err
	}
	return live.ObjectID(), nil
}

// spawnStandalone is SpawnFixed for an NPC spawned for summoner, nil for
// none, and belonging to the castle castleID and the clan clanID, 0 for
// none, returning the NPC placed. Its created hooks have run by then; one may have deleted it
// again.
func (n *Npcs) spawnStandalone(tmpl *npc.Template, x, y, z, heading int, respawn time.Duration, summoner attackable.Combatant, castleID int, clanID int32) (attackable.Combatant, error) {
	return n.spawnStandaloneAt(tmpl, location.Location{X: x, Y: y, Z: int(n.geo.Height(x, y, z))}, heading, respawn, summoner, castleID, clanID)
}

// spawnStandaloneAt is spawnStandalone at the exact point at, with no
// snapping onto the ground.
func (n *Npcs) spawnStandaloneAt(tmpl *npc.Template, at location.Location, heading int, respawn time.Duration, summoner attackable.Combatant, castleID int, clanID int32) (attackable.Combatant, error) {
	if tmpl == nil {
		return nil, ErrNotPlaceable
	}
	if probe := (&npc.Instance{Template: tmpl}); !npc.FolkKind(probe) && !npc.Attackable(probe) {
		return nil, fmt.Errorf("%w: npc %d instance type %q", ErrNotPlaceable, tmpl.ID, tmpl.Type)
	}
	key := fmt.Sprintf("fixed#%d", n.fixedSeq.Add(1))
	entry := spawn.Entry{NPCID: int32(tmpl.ID), RespawnDelay: max(respawn, 0)}
	n.mu.Lock()
	n.slot[key] = slotInfo{key: key, entry: entry, tmpl: tmpl, fixed: true, at: at, heading: heading, summoner: summoner, castleID: castleID, clanID: clanID, memory: newSlotMemory(entry, nil)}
	n.mu.Unlock()

	live := n.instantiate(key, entry, tmpl, at, heading, fullHP, fullMP, nil)
	if live == nil {
		n.mu.Lock()
		n.dropSlotLocked(key)
		n.mu.Unlock()
		return nil, fmt.Errorf("%w: npc %d", ErrNotPlaceable, tmpl.ID)
	}
	return live, nil
}

// DeleteFixed removes the live NPC id at once, with no corpse, when it was
// placed by SpawnFixed, and drops its spawn: it never comes back. A
// master's privates stay in the world. It reports false, removing nothing,
// for any other NPC. The removal runs on the NPC's own queue.
func (n *Npcs) DeleteFixed(id int32) bool {
	return n.deleteFixed(id, false)
}

// DeleteFixedWithPrivates is DeleteFixed for a spawn deleted as a whole: a
// master takes its privates with it first, as a maker deleting its entries
// does.
func (n *Npcs) DeleteFixedWithPrivates(id int32) bool {
	return n.deleteFixed(id, true)
}

func (n *Npcs) deleteFixed(id int32, withPrivates bool) bool {
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
		if withPrivates {
			n.despawnMinions(a)
		}
		a.DeleteMe()
	case *npc.Folk:
		if withPrivates {
			n.despawnMinions(a)
		}
		onQueue(a.Queue(), func() { n.RemoveFolk(a) })
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
	// MasterID and MasterName are the object id and name of the master a
	// private spawns around, as its current life; zero and empty for any
	// other spawn.
	MasterID   int32
	MasterName string
	// Fixed reports a standalone spawn SpawnFixed placed.
	Fixed bool
	// At and Heading are the point a standalone spawn, or a private a
	// script created at a point, places its NPC at; unset for any other
	// spawn (see SpawnPoint).
	At      location.Location
	Heading int
	// Entry is the spawn's declaration: the maker entry, or the entry a
	// private's or a standalone spawn was given. It holds the spawn's
	// respawn delays, positions, privates and AI parameters.
	Entry spawn.Entry
}

// SpawnOf returns the spawn of the live NPC id, false when id is no NPC
// this population spawned.
func (n *Npcs) SpawnOf(id int32) (SpawnRecord, bool) {
	n.mu.Lock()
	key, live := n.live[id]
	slot, ok := n.slot[key]
	n.mu.Unlock()
	if !live || !ok {
		return SpawnRecord{}, false
	}
	rec := SpawnRecord{Fixed: slot.fixed, Entry: slot.entry}
	if slot.fixed || slot.placed {
		rec.At, rec.Heading = slot.at, slot.heading
	}
	if slot.master != nil {
		master := npc.Current(slot.master)
		rec.MasterID, rec.MasterName = master.ObjectID(), master.CharacterName()
	}
	if slot.maker != nil {
		rec.Maker = slot.maker.Name
	}
	return rec, true
}

// SpawnPoint returns where the spawn of the live NPC id places its NPC: a
// standalone spawn's own point, a private's own point when a script gave
// it one, else a point drawn around the private's master, anew for each
// call, facing the way the master faces. ok is false for a maker's spawn,
// whose entry declares its points, and for an NPC this population did not
// spawn.
func (n *Npcs) SpawnPoint(id int32) (at location.Location, heading int, ok bool) {
	n.mu.Lock()
	key, live := n.live[id]
	slot, found := n.slot[key]
	n.mu.Unlock()
	switch {
	case !live || !found || slot.maker != nil:
		return location.Location{}, 0, false
	case slot.fixed || slot.placed:
		return slot.at, slot.heading, true
	case slot.master != nil:
		master := npc.Current(slot.master)
		return n.privateSpawnLocation(master, slot.tmpl), master.Heading(), true
	}
	return location.Location{}, 0, false
}
