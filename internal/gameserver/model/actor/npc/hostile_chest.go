package npc

import "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"

// The Chest npc ids in this range are treasure boxes an unlock skill can
// open; every other Chest is a mimic that fights the opener instead.
const (
	firstBoxChestID = 18265
	lastBoxChestID  = 18298
)

// Remover takes a live NPC out of the world outside its corpse decay,
// arming its spawn's respawn the way a decayed corpse does.
type Remover interface {
	Remove(h *Hostile)
}

// Box reports a treasure-box chest that an unlock skill can open.
func (h *Hostile) Box() bool {
	id := h.Instance.Template.ID
	return h.chestKind() && id >= firstBoxChestID && id <= lastBoxChestID
}

// Interacted reports whether an unlock attempt has already claimed this
// chest.
func (h *Hostile) Interacted() bool { return h.interacted.Load() }

// ClaimInteraction marks this chest as interacted and reports whether this
// call made the claim, so only one of several concurrent unlock attempts
// goes on to open or destroy it. A respawn is a new Hostile, so the claim
// never outlives this spawn.
func (h *Hostile) ClaimInteraction() bool { return h.interacted.CompareAndSwap(false, true) }

// Kill runs this NPC's death sequence with its own kill rewards, as a
// killing blow would, and reports whether the death was newly applied.
func (h *Hostile) Kill(killer attackable.Combatant) bool { return h.Die(killer, h.rewards) }

// DeleteMe removes this NPC from the world at once, leaving no corpse, and
// arms its spawn's respawn. The removal runs on this NPC's own queue, the
// same queue its corpse decay would run on.
func (h *Hostile) DeleteMe() {
	h.Queue().Post(func() {
		if h.remover != nil {
			h.remover.Remove(h)
			return
		}
		h.Decay(h.world, nil)
	})
}
