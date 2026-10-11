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
	if key, ok := n.live[id]; ok {
		slot, ok := n.slot[key]
		return slot, ok
	}
	for _, slot := range n.slot {
		if slot.liveID == id {
			return slot, true
		}
	}
	return slotInfo{}, false
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
// around its master once d has passed. It comes back only when its corpse
// has left the world by then and its master is alive. Of two requests for
// one private, the first to come due brings it back. A d that is not
// positive asks nothing. A private whose spawn declares no respawn delay
// is gone once its corpse decays, and never comes back (#3531). An NPC
// that is no private reports ErrNotPrivate: an npcmaker's NPCs come back
// through their maker.
func (n *Npcs) ScheduleRespawn(h *npc.Hostile, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	slot, ok := n.slotOf(h.ObjectID())
	if !ok || slot.masterID == 0 {
		return ErrNotPrivate
	}
	n.respawn.AddEarliest(slot.key, n.now().Add(d))
	return nil
}

// DecayMe hides h, as the reference's decayMe hides an NPC: it leaves the
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
	delete(n.slot, key)
}
