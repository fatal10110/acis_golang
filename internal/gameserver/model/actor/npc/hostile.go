package npc

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const defaultDriftRange = 200

var hostileInstanceKinds = map[InstanceKind]struct{}{
	"Chest":           {},
	"FeedableBeast":   {},
	"FestivalMonster": {},
	"FriendlyMonster": {},
	"GrandBoss":       {},
	"Guard":           {},
	"HalishaChest":    {},
	"Monster":         {},
	"RaidBoss":        {},
	"SiegeGuard":      {},
}

// Hostile is a live attackable NPC with world presence and an AI loop.
type Hostile struct {
	world.Presence

	Instance *Instance

	brain *ai.Attackable
	move  ai.MoveController
	world *world.State

	deathMu sync.Mutex
	dead    bool
	decayed bool

	// hpMu guards hp, kept separate from deathMu: hp bookkeeping (and a
	// future regen tick) shouldn't need the death latch, and the death
	// latch shouldn't need to know how HP got to zero.
	hpMu sync.Mutex
	hp   int

	// roll draws a uniform integer in [0, n) for MakeAttackHit's hit/crit/
	// damage-spread rolls. It defaults to math/rand's global source; tests
	// substitute a fixed function for deterministic combat outcomes.
	roll func(n int) int
}

// NewHostile creates a live attackable NPC wrapper for inst.
func NewHostile(inst *Instance, movement ai.MoveController, attack ai.AttackController) (*Hostile, error) {
	if inst == nil {
		return nil, errors.New("npc: nil hostile instance")
	}
	if inst.Template == nil {
		return nil, errors.New("npc: hostile instance has nil template")
	}
	kind := hostileKind(inst)
	if _, ok := hostileInstanceKinds[kind]; !ok {
		return nil, fmt.Errorf("npc %d: instance type %q is not attackable", inst.Template.ID, kind)
	}
	if movement == nil {
		return nil, errors.New("npc: nil hostile movement")
	}
	if attack == nil {
		return nil, errors.New("npc: nil hostile attack")
	}

	h := &Hostile{
		Instance: inst,
		move:     movement,
		hp:       int(inst.Template.HPMax),
		roll:     rand.Intn,
	}
	h.brain = ai.NewAttackable(h, movement, attack)
	return h, nil
}

// SetWorld records the world registry BroadcastAttack reaches nearby
// observers through. Call it once, after placing this NPC on the grid —
// BroadcastAttack is a no-op until then. This mirrors Decay's worldState
// parameter, which BroadcastAttack has no room for since attack.CreatureActor
// fixes its signature to the snapshot alone.
func (h *Hostile) SetWorld(state *world.State) {
	h.world = state
}

// SetRollSource overrides the random source MakeAttackHit uses for its
// hit/crit/damage-spread rolls, for deterministic tests.
func (h *Hostile) SetRollSource(f func(n int) int) {
	h.roll = f
}

// ObjectID returns the world object id assigned to this NPC.
func (h *Hostile) ObjectID() int32 {
	return h.Instance.ObjectID
}

// AI returns the hostile NPC brain.
func (h *Hostile) AI() *ai.Attackable {
	return h.brain
}

// AddDamageHate records physical threat against this NPC.
func (h *Hostile) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	h.brain.AddDamageHate(attacker, damage, hate)
}

// AddHate records skill-cast hate against this NPC.
func (h *Hostile) AddHate(attacker attackable.Combatant, hate float64) {
	h.brain.AddHate(attacker, hate)
}

// Tick advances the hostile AI clock once.
func (h *Hostile) Tick() {
	h.brain.Tick()
}

// Think runs one hostile AI decision cycle.
func (h *Hostile) Think() {
	h.brain.Think()
}

// SiegeGuard reports whether this NPC is a defensive siege guard.
func (h *Hostile) SiegeGuard() bool {
	return hostileKind(h.Instance) == "SiegeGuard"
}

// AlikeDead reports whether this NPC should be ignored as a live target.
func (h *Hostile) AlikeDead() bool {
	return h.Dead()
}

// Dead reports whether this NPC has died and not yet been revived.
func (h *Hostile) Dead() bool {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	return h.dead
}

// MarkDead transitions this NPC into its dead state. It reports false when
// the NPC was already dead, so a repeated or concurrent kill is a no-op.
func (h *Hostile) MarkDead() bool {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	if h.dead {
		return false
	}
	h.dead = true
	return true
}

// Die runs this NPC's death sequence: the shared once-only dead-state
// transition, then its reward hook. rewards may be nil — the drop and
// experience/SP systems land separately and plug in here once ready. It
// reports whether the death was newly applied by this call.
//
// The caller is responsible for registering the corpse with the decay
// task afterwards (using Instance.Template.CorpseTime as the display
// interval) — Hostile does not hold a reference to that task, so the
// scheduling stays at the orchestration layer that owns it.
func (h *Hostile) Die(killer creature.DeathActor, rewards creature.Rewarder) bool {
	return creature.Die(h, killer, rewards)
}

// Decayed reports whether this NPC's corpse has already been removed from
// the world.
func (h *Hostile) Decayed() bool {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	return h.decayed
}

// Decay removes this NPC's corpse from the world and runs the respawn
// hook, if any. It is idempotent: a repeat call is a no-op, matching the
// once-only guarantee the corpse decay task relies on.
//
// worldState may be nil in tests that do not track live world placement.
// respawn is called after the world removal when non-nil; a live spawn
// runtime is expected to close over its own spawn.State/spawn.Entry
// linkage and call spawn.CalculateRespawnDelay plus spawn.State.SetRespawn
// there, since Hostile itself carries no spawn linkage yet.
func (h *Hostile) Decay(worldState *world.State, respawn func()) bool {
	h.deathMu.Lock()
	if h.decayed {
		h.deathMu.Unlock()
		return false
	}
	h.decayed = true
	h.dead = true
	h.deathMu.Unlock()

	if worldState != nil {
		worldState.Despawn(h)
	}
	if respawn != nil {
		respawn()
	}
	return true
}

// DenyAIAction reports whether this NPC is unable to act.
func (h *Hostile) DenyAIAction() bool {
	return h.AlikeDead()
}

// Knows reports whether target is currently visible to this NPC.
func (h *Hostile) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(h, tracked)
}

// PhysicalAttackRange returns this NPC's melee attack range.
func (h *Hostile) PhysicalAttackRange() int {
	return h.Instance.Template.BaseAttackRange
}

// ReturnHome reports whether this NPC started returning to its spawn.
func (h *Hostile) ReturnHome() bool {
	if h.InTerritory() || !h.brain.Hates().IsEmpty() {
		return false
	}
	if in2DRange(h.location(), h.Instance.Home, h.driftRange()) {
		return false
	}

	h.brain.Threats().ZeroHate()
	h.move.MoveHome(h.Instance.Home)
	return true
}

// InTerritory reports whether this NPC is inside its spawn territory.
func (h *Hostile) InTerritory() bool {
	if !h.Instance.HasHome {
		return true
	}
	return in3DRange(h.location(), h.Instance.Home, h.driftRange())
}

func hostileKind(inst *Instance) InstanceKind {
	if inst.Kind != "" {
		return inst.Kind
	}
	return InstanceKind(inst.Template.Type)
}

func (h *Hostile) location() location.Location {
	x, y, z := h.Position()
	return location.Location{X: x, Y: y, Z: z}
}

func (h *Hostile) driftRange() int {
	if h.Instance.DriftRange > 0 {
		return h.Instance.DriftRange
	}
	return defaultDriftRange
}

func in2DRange(a, b location.Location, radius int) bool {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) <= float64(radius)
}

func in3DRange(a, b location.Location, radius int) bool {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	dz := float64(a.Z - b.Z)
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= float64(radius)
}
