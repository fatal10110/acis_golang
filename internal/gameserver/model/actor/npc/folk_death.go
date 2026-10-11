package npc

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Dead reports whether the NPC has died. A corpse stays dead once it
// decays: a respawn is a new NPC.
func (f *Folk) Dead() bool {
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return f.dead
}

// Decayed reports whether the NPC has left the world for good: its corpse
// decayed or it was deleted.
func (f *Folk) Decayed() bool {
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return f.decayed
}

// die runs the NPC's death once and reports whether this call ran it: its
// HP drops to zero, observers see the empty health bar, its walk and cast
// stop and its cast desires drop, every effect that does not last through
// death ends, observers see it die and leave its attack stance, and its
// corpse is registered for decay after the template corpse time. A
// civilian NPC pays no reward. Its dying script hooks are scheduled, then
// its clan members in range are told.
func (f *Folk) die(killer attackable.Combatant) bool {
	f.vitalsMu.Lock()
	if f.dead {
		f.vitalsMu.Unlock()
		return false
	}
	f.hp = 0
	f.dead = true
	f.vitalsMu.Unlock()

	f.BroadcastStatus()
	f.stopMoving()
	f.AbortCast()
	f.effects.StopAllExceptThoseThatLastThroughDeath()
	f.BroadcastStatus()
	f.emit(event.Died{})
	if f.motion != nil && f.motion.cfg.Control != nil {
		// The route walker releases a dead NPC from its route.
		f.motion.cfg.Control.Emit(event.Died{})
	}
	f.scheduleDecay()
	f.raiseDying(killer)
	raiseClanDied(f.scripts, f.world, f, killer)
	return true
}

// scheduleDecay registers the NPC's corpse with the decay task for the
// template corpse time.
func (f *Folk) scheduleDecay() {
	if f.decay == nil {
		return
	}
	deadline := f.decay.Add(f, f.CorpseTime())
	f.vitalsMu.Lock()
	if !f.decayed {
		f.corpseDeadline = deadline
	}
	f.vitalsMu.Unlock()
}

// HasCorpse reports whether the NPC is a corpse awaiting its decay.
func (f *Folk) HasCorpse() bool {
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return f.dead && !f.decayed && !f.corpseDeadline.IsZero()
}

// CorpseDeadline returns when the NPC's corpse decays, if one is
// registered.
func (f *Folk) CorpseDeadline() (time.Time, bool) {
	f.vitalsMu.Lock()
	defer f.vitalsMu.Unlock()
	return f.corpseDeadline, !f.corpseDeadline.IsZero()
}

// CorpseTime returns how long the template's corpse stays.
func (f *Folk) CorpseTime() time.Duration {
	return time.Duration(f.Instance.Template.CorpseTime) * time.Second
}

// Decay removes the NPC's corpse from the world, ends every effect it still
// holds and runs respawn, if any, then closes its queue. Its decayed script
// hooks run first, while it is still in the world. It runs once; a
// repeat call reports false. worldState may be nil when the NPC was never
// placed.
func (f *Folk) Decay(worldState *world.State, respawn func()) bool {
	return f.DecayWithRespawn(worldState, func(int32) func() { return respawn })
}

// DecayWithRespawn is Decay with the respawn hook resolved by respawnFor,
// called with this NPC's object id only once this call has won the decay
// and before its decayed hooks run. Of several concurrent decays, only the
// winner claims the spawn's respawn, so it is armed exactly once.
// respawnFor may be nil.
func (f *Folk) DecayWithRespawn(worldState *world.State, respawnFor func(id int32) func()) bool {
	f.vitalsMu.Lock()
	if f.decayed {
		f.vitalsMu.Unlock()
		return false
	}
	f.decayed, f.dead = true, true
	f.corpseDeadline = time.Time{}
	f.vitalsMu.Unlock()
	var respawn func()
	if respawnFor != nil {
		respawn = respawnFor(f.ObjectID())
	}
	f.raiseDecayed()

	// The NPC leaves its zones while its observers still know it.
	x, y, z := f.Position()
	f.zones.leave(location.Location{X: x, Y: y, Z: z})
	if worldState != nil {
		worldState.Despawn(f)
	}
	// End what outlived the death strip after the world removal, so nobody
	// sees it go; Untrack keeps a straggler tick from tracking the list
	// again.
	f.effects.StopAll()
	f.effects.Untrack()
	if respawn != nil {
		respawn()
	}
	// A respawn is a new NPC on a new queue; this one takes no more work.
	if f.queue != nil {
		f.queue.Close()
	}
	return true
}

// DeleteNow takes the NPC out of the world at once, leaving no corpse, on
// the calling goroutine, and answers its spawn as for a decayed corpse: it
// is out of the world when it returns.
func (f *Folk) DeleteNow() {
	if f.remover != nil {
		f.remover.RemoveFolk(f)
		return
	}
	f.Decay(f.world, nil)
}
