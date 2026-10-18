package manager

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
)

// ErrNotPrivate reports a script respawn request for an NPC that is no
// master's private: only a private's spawn serves one yet.
var ErrNotPrivate = errors.New("npcs: not a private")

// slotOf returns the spawn slot of the live or decayed NPC with object id,
// and whether it has one: a decayed NPC's slot is found by the NPC it last
// placed, while it has not placed another.
func (n *Npcs) slotOf(id int32) (slotInfo, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	key, ok := n.slotKeyLocked(id)
	if !ok {
		return slotInfo{}, false
	}
	slot, ok := n.slot[key]
	return slot, ok
}

// slotKeyLocked returns the key of the spawn slot of the live or decayed
// NPC with object id, as slotOf finds it. The caller holds n.mu.
func (n *Npcs) slotKeyLocked(id int32) (string, bool) {
	if key, ok := n.live[id]; ok {
		return key, true
	}
	for key, slot := range n.slot {
		if slot.liveID == id {
			return key, true
		}
	}
	return "", false
}

// RespawnDelay returns the respawn delay the spawn that placed h declares,
// without its random part; 0 for an NPC no spawn placed.
func (n *Npcs) RespawnDelay(h *npc.Hostile) time.Duration {
	slot, ok := n.slotOf(h.ObjectID())
	if !ok {
		return 0
	}
	return slot.entry.RespawnDelay
}

// ScheduleRespawn asks the spawn of h, a master's private, to bring it back
// once d has passed: around its master, or at its own point when it was
// created at one. It comes back only when its corpse has left the world by
// then and its master is alive. Of two requests for one private, the first
// to come due brings it back. A d that is not positive asks nothing. A
// private whose spawn declares no respawn delay comes back too, when the
// request was made before its corpse decayed, but no longer as its master's
// private: its decay let it go. A private whose spawn is gone (hidden, or
// decayed with nothing left to bring it back) asks nothing. An NPC that is
// no private reports ErrNotPrivate: an npcmaker's NPCs come back through
// their maker.
func (n *Npcs) ScheduleRespawn(h *npc.Hostile, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	slot, ok := n.slotOf(h.ObjectID())
	if !ok {
		if h.IsPrivate() {
			return nil
		}
		return ErrNotPrivate
	}
	if slot.master == nil {
		return ErrNotPrivate
	}
	n.respawn.AddEarliest(slot.key, n.now().Add(d))
	return nil
}

// dropOrphanScriptSlot drops the slot key of a private a script created,
// whose NPC is decaying, unless the private can still come back: a request
// for it is pending or under way, or its spawn declares a respawn delay and
// its master lives, for the master's script to ask it back. A dead
// master's private has nobody left to ask, and a request already pending
// would find that master dead anyway, so a kept slot would stay for good.
func (n *Npcs) dropOrphanScriptSlot(key string, slot slotInfo) {
	master := npc.Current(slot.master)
	alive := master != nil && !master.Dead()
	n.mu.Lock()
	defer n.mu.Unlock()
	cur, ok := n.slot[key]
	if !ok || cur.spawning || n.respawn.Tracked(key) || (slot.entry.RespawnDelay > 0 && alive) {
		return
	}
	n.dropSlotLocked(key)
}

// dropGoneScriptPrivates drops, as master decays, the slots of the privates
// a script created for it whose NPC has left the world for good: none is
// in the world and no request to bring it back is pending. Only master's
// party-died hooks ask a private back, at its death, so nothing can ask
// for them any more.
func (n *Npcs) dropGoneScriptPrivates(master npc.PartyMember) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for key := range n.privates[master.Scratch()] {
		if slot := n.slot[key]; slot.scripted && !slot.spawning && !n.liveLocked(key) && !n.respawn.Tracked(key) {
			n.dropSlotLocked(key)
		}
	}
}

// DecayMe hides h without a decay: it leaves the
// world with no decayed hook, and its spawn is not told, so it never
// respawns and an npcmaker keeps counting it. It takes no more AI turns
// and walks no route. The slot of a private a script created goes with it,
// as nothing can bring that private back.
func (n *Npcs) DecayMe(h *npc.Hostile) {
	if n.decay != nil {
		n.decay.Cancel(h)
	}
	if !h.Hide(n.state) {
		return
	}
	id := h.ObjectID()
	n.ai.Remove(h)
	n.walker.StopRouteByID(id)

	n.mu.Lock()
	defer n.mu.Unlock()
	key, ok := n.live[id]
	if !ok || !n.slot[key].scripted {
		return
	}
	delete(n.live, id)
	n.liveCount--
	n.dropSlotLocked(key)
}
