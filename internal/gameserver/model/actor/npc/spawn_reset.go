package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// SpawnHook runs the hook of the spawn that placed an NPC.
type SpawnHook interface {
	// SpawnedInPlace runs the hook of n's spawn for n shown again where it
	// stands as if newly spawned. n never left, so its spawn still counts
	// it once.
	SpawnedInPlace(n attackable.Combatant)
}

// reshowInPlace shows t again where it stands, as taking it out of the
// world and spawning it again there would: t exits its zones, the players
// around it forget it and then see it again as it now is, and it enters
// the zones at its position again. It reports whether t stands on the grid
// again; a t off the grid or despawned stays out of the world and its
// zones.
func reshowInPlace(w *world.State, t world.Tracked, zones *zoneMember) bool {
	if w == nil {
		return false
	}
	if placed, _ := w.RegionActivity(t); !placed {
		return false
	}
	zones.leave()
	w.Leave(t)
	w.Rejoin(t)
	if placed, _ := w.RegionActivity(t); !placed {
		return false
	}
	zones.enter()
	return true
}

// onQueue runs fn on q, at once when there is no queue.
func onQueue(q *sim.Queue, fn func()) {
	if q == nil {
		fn()
		return
	}
	q.Post(fn)
}

// resetForSpawn readies h, shown again where it stands, as a new spawn of
// it starts: its aggro and hate lists, its AI's desires and the creatures
// it saw are dropped, its escort points free, its shots refilled and its
// stance running; its AI sleep is decided again and its created hooks and
// its spawn's hook run again. A monster's over-hit, spoil, seed and soul
// crystal charges are dropped last, and a chest takes an unlock attempt
// again. It runs on h's queue; a despawn that landed first leaves h as it
// is.
func (h *Hostile) resetForSpawn() {
	if placed, _ := regionActivityOf(h.world, h); !placed {
		return
	}
	h.ClearAggroTables()
	h.brain.CleanupForSpawn()
	h.sight.clear()
	h.resetFollow()
	h.resetShots()
	h.ForceRunStance()
	h.aiSleep.settle(h.world, h)
	if h.scripts != nil {
		h.scripts.HostileCreated(h)
	}
	if h.spawns != nil {
		h.spawns.SpawnedInPlace(h)
	}
	if !h.MonsterKind() {
		return
	}
	h.overhit.clear()
	h.spoil.Reset()
	h.seed.clear()
	h.absorb.clear()
	if h.chestKind() {
		h.interacted.Store(false)
	}
}

// resetShots refills h's soulshots and spiritshots from its template, as
// it spawned with; the charged shots stay.
func (h *Hostile) resetShots() {
	soul, spirit, err := shotCounts(h.Instance.Template)
	if err != nil {
		// The constructor read the same parameters without error.
		return
	}
	h.shotsMu.Lock()
	defer h.shotsMu.Unlock()
	h.currentSoulshots, h.currentSpiritshots = soul, spirit
}

// resetForSpawn readies f, shown again where it stands, as a new spawn of
// it starts: its AI's desires and the creatures it saw are dropped, its
// escort points free, its shots refilled and its stance running; its AI
// sleep is decided again and its created hooks and its spawn's hook run
// again. It runs on f's queue; a despawn that landed first leaves f as it
// is.
func (f *Folk) resetForSpawn() {
	if placed, _ := regionActivityOf(f.world, f); !placed {
		return
	}
	f.cleanupAIForSpawn()
	f.resetFollow()
	f.resetShots()
	f.SetRunStance(true)
	f.aiSleep.settle(f.world, f)
	if f.scripts != nil {
		f.scripts.FolkCreated(f)
	}
	if f.spawns != nil {
		f.spawns.SpawnedInPlace(f)
	}
}

// cleanupAIForSpawn drops f's queued desires, its attack latch, its route
// walk, its social stamp and the creatures it saw.
func (f *Folk) cleanupAIForSpawn() {
	f.cast.brain.Lock()
	defer f.cast.brain.Unlock()
	f.cast.desires.Clear()
	f.fight.latched = nil
	f.leaveRoute()
	f.lastSocial.Store(0)
	f.sight.clear()
}

// resetShots refills f's soulshots and spiritshots from its template, as
// it spawned with; the charged shots stay.
func (f *Folk) resetShots() {
	soul, spirit, err := shotCounts(f.Instance.Template)
	if err != nil {
		// NewFolk read the same parameters without error.
		return
	}
	f.shots.mu.Lock()
	defer f.shots.mu.Unlock()
	f.shots.soul, f.shots.spirit = soul, spirit
}

// resetFollow frees every escort point and forgets the last escort step's
// master position. The party itself stays.
func (p *partyLinks) resetFollow() {
	p.partyMu.Lock()
	defer p.partyMu.Unlock()
	p.followSlots = [escortSlotCount]int32{}
	p.lastFollowingLoc, p.hasLastFollow = location.Location{}, false
}

// clear forgets every creature seen.
func (s *sight) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = nil
}

// clear disarms the over-hit check and forgets the stored strike.
func (o *overhitState) clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.enabled, o.damage, o.attacker = false, 0, nil
}

// clear forgets the sower, the seed and the harvest; the owner's level and
// crop multiplier stay.
func (s *SeedState) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sowerID, s.seed, s.harvested = 0, manor.Seed{}, false
}

// clear forgets every soul crystal charge.
func (a *absorbers) clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.byPlayer = nil
}
