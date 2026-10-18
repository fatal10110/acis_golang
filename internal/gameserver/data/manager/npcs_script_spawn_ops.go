package manager

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// DoRespawn brings m back now through its spawn: it cancels the respawn its
// spawn has pending for it, clears the respawn time of a database-tracked
// spawn's row, and places it again when it has left the world. A dead NPC
// whose corpse has not decayed yet, or a live one, has only its pending
// respawn cancelled. A private comes back only while its master lives, and
// otherwise nothing is cancelled. An NPC whose spawn is gone (a standalone
// spawn after its decay, a private whose slot went with its decay) does
// nothing.
func (n *Npcs) DoRespawn(m npc.PartyMember) {
	slot, ok := n.slotOf(m.ObjectID())
	if !ok {
		return
	}
	if slot.master != nil {
		if master := npc.Current(slot.master); master == nil || master.Dead() {
			return
		}
	}
	n.respawn.Cancel(slot.key)
	if slot.dbName != "" {
		n.currentSpawns().CancelRespawn(slot.dbName)
	}
	n.respawnSlot(slot.key)
}

// DrawSpawnPoint draws a point m's spawn would place it at now, and the way it
// would face there: an npcmaker spawn draws as a respawn does (its one
// declared point, a weighted pick of its points with a random heading, or a
// new random point of its territory on every call); a standalone spawn
// gives its own point; a private gives the point it was created at, or a
// new point around its master's current position, facing the master's
// way. ok is false for an NPC no spawn placed, and for a territory with no
// point to draw.
func (n *Npcs) DrawSpawnPoint(m npc.PartyMember) (at location.Location, heading int, ok bool) {
	slot, found := n.slotOf(m.ObjectID())
	if !found {
		return location.Location{}, 0, false
	}
	switch {
	case slot.fixed, slot.placed:
		heading = slot.heading
		if heading < 0 {
			heading = rnd.Get(65536)
		}
		return slot.at, heading, true
	case slot.master != nil:
		master := npc.Current(slot.master)
		if master == nil || slot.tmpl == nil {
			return location.Location{}, 0, false
		}
		return n.privateSpawnLocation(master, slot.tmpl), master.Heading(), true
	}
	pos, ok := n.pickSpawnPosition(slot.maker, slot.entry)
	return pos.Location, pos.Heading, ok
}

// SetRespawnDelay sets the respawn delay of the spawn of m, a master's
// private, to d, 0 when d is negative. A private whose spawn declares no
// respawn delay leaves its master's privates when it decays and is not
// kept for a later respawn request (ScheduleRespawn). A private whose
// spawn is gone sets nothing; an NPC that is no private reports
// ErrNotPrivate.
func (n *Npcs) SetRespawnDelay(m npc.PartyMember, d time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	key, ok := n.live[m.ObjectID()]
	if !ok {
		key, ok = n.decayedSlotKeyLocked(m.ObjectID())
	}
	slot, found := n.slot[key]
	if !ok || !found {
		if m.IsPrivate() {
			return nil
		}
		return ErrNotPrivate
	}
	if slot.master == nil {
		return ErrNotPrivate
	}
	slot.entry.RespawnDelay = max(d, 0)
	n.slot[key] = slot
	return nil
}

// decayedSlotKeyLocked returns the key of the slot whose last placed NPC is
// id. The caller holds n.mu.
func (n *Npcs) decayedSlotKeyLocked(id int32) (string, bool) {
	for key, slot := range n.slot {
		if slot.liveID == id {
			return key, true
		}
	}
	return "", false
}
