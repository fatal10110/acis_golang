package npc

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var (
	_ creature.FormulaActor = (*Folk)(nil)
	_ effect.Actor          = (*Folk)(nil)
)

// folkCombat is the state a civilian NPC fights with. Other creatures
// damage, heal and debuff it from their own queues; its regeneration and
// effect ticks run on its own.
type folkCombat struct {
	// world, queue, sink, los, decay, scripts, remover and heldMask are set
	// by Attach before
	// the NPC is published.
	world *world.State
	queue *sim.Queue
	sink  event.Sink
	los   LineOfSight
	decay *task.Decay
	// scripts raises the NPC's script hooks; nil raises none. remover takes
	// it out of the world for DeleteNow; nil decays it with no respawn.
	scripts ScriptHooks
	remover FolkRemover
	// heldMask is the item-type bits of the template's weapon and shield.
	heldMask int32

	effects  *effect.List
	maxBuffs atomic.Int32

	// statMu guards statCalcs slot creation; each Calculator guards its own
	// mods.
	statMu    sync.RWMutex
	statCalcs [stat.Count]*effect.Calculator
	// speedMu orders move-speed hand-offs to a walking NPC's movement.
	speedMu sync.Mutex

	running        atomic.Bool
	inCombat       atomic.Bool
	abnormalEffect atomic.Int32
	invul          atomic.Bool
	immobilized    atomic.Bool

	// vitalsMu guards hp, mp and the death state: dead, decayed and the
	// corpse's decay deadline (zero while none is registered).
	vitalsMu       sync.Mutex
	hp, mp         float64
	dead, decayed  bool
	corpseDeadline time.Time
	// hpBar is the health-bar segment state PublishHP advances.
	hpBar creature.HPBar
	// regen is the HP/MP regeneration task SettleRegen arms on a drop.
	regen creature.Regen
}

// FolkRuntime is what a civilian NPC needs to take part in combat beyond
// its template. Queue is required: the regeneration and attack-stance tasks
// post to the queue of every NPC they track. A nil Sink shows the NPC's
// changes to nobody, and a nil World leaves it knowing nobody.
type FolkRuntime struct {
	World *world.State
	// Queue is the NPC's own queue its regeneration, effects and stance
	// expiry run on.
	Queue *sim.Queue
	Sink  event.Sink
	// Effects is the server's effect-list context.
	Effects effect.Env
	// MaxBuffsAmount is the configured base buff-slot count; zero keeps the
	// shipped default.
	MaxBuffsAmount int
	// AI is the AI task the NPC's spawner put it on, which it leaves when
	// it dies; nil for none.
	AI FolkAI
	// LOS answers the NPC's line of sight; nil sees everything.
	LOS LineOfSight
	// Items resolves the template's held weapon and shield; nil leaves the
	// NPC holding nothing.
	Items *item.Table
	// Decay removes the NPC's corpse once its template corpse time has
	// passed; nil leaves a dead NPC's corpse in place.
	Decay *task.Decay
	// Zones are the zones the NPC's membership follows; nil leaves it in
	// none.
	Zones *zone.Index
	// Slot is the spawn slot that placed the NPC; nil leaves it with no
	// spawn parameters and a script memory of its own.
	Slot SpawnSlot
	// Scripts raises the NPC's script hooks; nil raises none.
	Scripts ScriptHooks
	// Remover takes the NPC out of the world for DeleteNow; nil decays it
	// with no respawn.
	Remover FolkRemover
}

// FolkRemover takes a civilian NPC out of the world at once, with no
// corpse, and answers its spawn as for a decayed corpse.
type FolkRemover interface {
	RemoveFolk(f *Folk)
}

// folkAdmits reports whether a civilian NPC holds e: only plain buffs and
// debuffs land on one; every other effect is dropped before it starts.
func folkAdmits(e *effect.Effect) bool {
	return e.Type == effect.TypeBuff || e.Type == effect.TypeDebuff
}

// initCombat settles the NPC's stats from its template passives mods and
// fills its HP and MP.
func (f *Folk) initCombat(mods []effect.Mod) {
	f.effects = effect.NewList(f, effect.WithAdmission(folkAdmits))
	f.maxBuffs.Store(maxBuffCount)
	f.running.Store(!f.Instance.WalkMode)
	f.AttachStatFuncs(mods)
	f.hp = float64(f.MaxHP())
	f.mp = math.Trunc(f.MaxMPValue())
	f.hpBar.Calibrate(float64(f.MaxHP()))
}

// Attach installs rt. Call it once, before the NPC is published. It
// refuses a runtime without a queue.
func (f *Folk) Attach(rt FolkRuntime) error {
	if rt.Queue == nil {
		return errors.New("npc: folk runtime needs a queue")
	}
	f.world, f.queue, f.sink, f.los, f.decay = rt.World, rt.Queue, rt.Sink, rt.LOS, rt.Decay
	f.scripts, f.remover = rt.Scripts, rt.Remover
	f.zones.ix = rt.Zones
	f.cast.ai = rt.AI
	f.bindSpawn(rt.Slot)
	f.heldMask = templateHeldMask(f.Instance.Template, rt.Items)
	if rt.MaxBuffsAmount > 0 {
		f.maxBuffs.Store(int32(rt.MaxBuffsAmount))
	}
	f.effects = effect.NewList(f, effect.WithEnv(rt.Effects), effect.WithAdmission(folkAdmits))
	f.effects.SetQueue(rt.Queue)
	return nil
}

// Queue returns the queue the NPC's regeneration and effects run on.
func (f *Folk) Queue() *sim.Queue { return f.queue }

func (f *Folk) emit(ev event.Event) {
	if f.sink != nil {
		f.sink.Emit(ev)
	}
}

// OnInactiveRegion stops every effect the NPC holds once no player is near
// its region. The departing player's goroutine calls it, so the stop runs
// on the NPC's own queue, and only if the region is still inactive then.
func (f *Folk) OnInactiveRegion() {
	if f.queue == nil {
		return
	}
	f.queue.Post(func() {
		if f.world != nil {
			if placed, active := f.world.RegionActivity(f); !placed || active {
				return
			}
		}
		f.effects.StopAll()
	})
}

// CharacterName returns the template name.
func (f *Folk) CharacterName() string { return f.Instance.Name() }

// Karma reports 0: NPCs carry no PK karma.
func (f *Folk) Karma() int { return 0 }

// AlikeDead reports whether the NPC is dead: it never feigns death.
func (f *Folk) AlikeDead() bool { return f.Dead() }

// FakeDeath reports false: NPCs never feign death.
func (f *Folk) FakeDeath() bool { return false }

// RecentFakeDeath reports false: NPCs never feign death.
func (f *Folk) RecentFakeDeath() bool { return false }

// MovementDisabled reports a template that cannot move, an immobilized or
// dead NPC, or a teleport under way.
func (f *Folk) MovementDisabled() bool {
	return !f.Instance.Template.CanMove || f.immobilized.Load() || f.Dead() || (f.motion != nil && f.motion.teleporting.Load())
}

// SilentMoving reports whether an effect lets the NPC move unseen.
func (f *Folk) SilentMoving() bool { return f.effects.IsAffected(effect.FlagSilentMove) }

// Knows reports whether other is in the NPC's known list. An invisible
// player or its summon is never known.
func (f *Folk) Knows(other attackable.Combatant) bool {
	tracked, ok := other.(world.Tracked)
	return ok && world.Knows(f, tracked) && !attackable.HiddenActingPlayer(other)
}

// SpawnProtected reports false: spawn protection is a player state.
func (f *Folk) SpawnProtected() bool { return false }

// CanGiveDamage reports true: only access levels revoke damage.
func (f *Folk) CanGiveDamage() bool { return true }

// RaidRelated reports false.
func (f *Folk) RaidRelated() bool { return false }

// SiegeGuard reports false.
func (f *Folk) SiegeGuard() bool { return false }

// Guard reports false: a civilian NPC is no guard.
func (f *Folk) Guard() bool { return false }

// Attackable reports false: a civilian NPC keeps no hate or aggro, so the
// aggro controls below do nothing.
func (f *Folk) Attackable() bool { return false }

// Running reports the NPC's run stance: it spawns running, or walking
// for a route walker in walk mode; a hit or a cast desire it acts on
// switches it to run, and its AI going idle back to walk.
func (f *Folk) Running() bool { return f.running.Load() }

// InCombat reports whether the NPC holds an attack stance.
func (f *Folk) InCombat() bool { return f.inCombat.Load() }

// SetInCombat records the attack stance and reports whether it changed.
// The stance tracker clears it when the stance expires.
func (f *Folk) SetInCombat(inCombat bool) bool { return f.inCombat.Swap(inCombat) != inCombat }

// NotifyAttacked reports a damaging hit or an offensive skill reaching the
// NPC: it enters its attack stance.
func (f *Folk) NotifyAttacked(attacker attackable.Combatant) {
	f.emit(event.Attacked{Attacker: attacker})
}

// NotifyEvaded reports a missed hit, which the NPC does not react to.
func (f *Folk) NotifyEvaded(attackable.Combatant) {}

// BroadcastAutoAttackStop reports that the attack stance expired.
func (f *Folk) BroadcastAutoAttackStop() { f.emit(event.AutoAttackStopped{}) }

// forceRunStance switches a walking NPC to its run stance, as any hit
// does: the movement speeds up, and observers see the stance change and
// the NPC's info again.
func (f *Folk) forceRunStance() {
	if !f.running.CompareAndSwap(false, true) {
		return
	}
	f.refreshMoveSpeed()
	if f.MoveSpeed() != 0 {
		f.emit(event.MoveTypeChanged{Running: true})
	}
	f.emit(event.NPCInfoChanged{})
}

// forceWalkStance switches a running NPC to its walk stance, as its AI
// does when it idles: the movement slows down, and observers see the stance
// change and the NPC's info again.
func (f *Folk) forceWalkStance() {
	if !f.running.CompareAndSwap(true, false) {
		return
	}
	f.refreshMoveSpeed()
	if f.MoveSpeed() != 0 {
		f.emit(event.MoveTypeChanged{Running: false})
	}
	f.emit(event.NPCInfoChanged{})
}

// Roll draws a uniform random integer in [0, n).
func (f *Folk) Roll(n int) int {
	if n <= 0 {
		return 0
	}
	return rand.Intn(n)
}

// AbnormalEffect returns the NPC's client-visible abnormal-effect bits.
func (f *Folk) AbnormalEffect() int {
	return int(f.abnormalEffect.Load()) | f.effects.CrowdControlAbnormalEffect()
}

// StartAbnormalEffect adds mask to the NPC's abnormal state.
func (f *Folk) StartAbnormalEffect(mask int) { f.abnormalEffect.Or(int32(mask)) }

// StopAbnormalEffect removes mask from the NPC's abnormal state.
func (f *Folk) StopAbnormalEffect(mask int) { f.abnormalEffect.And(^int32(mask)) }

// UpdateAbnormalEffect shows observers the NPC's info again.
func (f *Folk) UpdateAbnormalEffect() { f.emit(event.AbnormalEffectChanged{}) }

// ServerObjectInfoSnapshot is NPCInfoSnapshot with the server-side name
// always shown, the view an NPC that cannot move is announced with.
func (f *Folk) ServerObjectInfoSnapshot() npcinfo.Snapshot {
	s := f.NPCInfoSnapshot()
	s.Name = f.Instance.Name()
	return s
}
