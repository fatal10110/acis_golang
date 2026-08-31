package npc

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npcinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

const defaultDriftRange = 200

// partyRangeDefault mirrors players.properties' PartyRange default (1500),
// used by RandomizeHate's canAutoAttack gate. The Party subsystem isn't
// ported yet, so this stays a local default instead of a wired Config
// value; move it there once Party configuration exists.
const partyRangeDefault = 1500

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
	*creature.Live

	Instance *Instance

	brain  *ai.Attackable
	move   ai.MoveController
	world  *world.State
	frames FrameBuilder
	log    zerolog.Logger

	known world.KnownBuffer

	// rewards computes this NPC's drop/experience payout when TakeDamage
	// kills it. It is nil until SetRewarder is called, in which case death
	// still latches but grants nothing — matching Die's own "rewards may be
	// nil" contract.
	rewards creature.Rewarder

	deathMu sync.Mutex
	dead    bool
	decayed bool

	minionsMu sync.RWMutex
	master    *Hostile
	minions   map[int32]*Hostile
	// followSlots are the eight escort points around this NPC, occupied by
	// minion object ids (0 is empty). Only a master uses them.
	followSlots [8]int32
	// lastFollowingLoc is the master's position recorded after this minion's
	// last escort step; hasLastFollow reports whether that snapshot is set.
	lastFollowingLoc location.Location
	hasLastFollow    bool

	regionInactive atomic.Bool
	abnormalEffect atomic.Int32
	running        atomic.Bool

	// geoPathFailCount counts consecutive route-walk moves that couldn't
	// path toward their target node, for task.Walker's teleport-to-start
	// recovery (see GeoPathFailCount).
	geoPathFailCount atomic.Int32

	// raidRelated marks this NPC as tied to a raid encounter (a raid boss
	// or one of its minions), set per-instance rather than derived from
	// the template. See RaidRelated.
	raidRelated atomic.Bool

	spoil          item.SpoilPool
	seed           SeedState
	overhit        overhitState
	corpseDeadline time.Time

	health creature.Health
	hp     float64

	// mpMu guards mp, the live MP value consumed by skill-resource handlers.
	mpMu sync.RWMutex
	mp   float64

	// weapon is this NPC's resolved right-hand weapon kind, recorded by
	// SetWeapon. Nil means unarmed — the common case, since the
	// overwhelming majority of monster templates carry no weapon item id.
	weapon *item.WeaponDetail

	// weaponCrystal is the resolved right-hand weapon's crystal grade,
	// recorded by SetWeapon alongside weapon. CrystalNone when unarmed.
	weaponCrystal item.CrystalType

	// roll draws a uniform integer in [0, n) for MakeAttackHit's hit/crit/
	// damage-spread rolls. It defaults to math/rand's global source; tests
	// substitute a fixed function for deterministic combat outcomes.
	roll func(n int) int

	los LineOfSight

	// shotsMu guards the per-spawn NPC shot counters and charge mask.
	shotsMu            sync.RWMutex
	currentSoulshots   int
	currentSpiritshots int
	shotsMask          int32

	// soulshotRate and spiritshotRate are the template's SoulShotRate/
	// SpiritShotRate AI parameters (percent, 0-100), read-only after
	// construction. RollAttackedShotRecharge uses them together with
	// currentSoulshots/currentSpiritshots to decide whether a landed hit
	// recharges shots.
	soulshotRate   int
	spiritshotRate int

	// statMu guards statCalcs slot creation; each slot's own Calculator
	// then guards its own Mods independently, so a warm read only ever
	// takes statMu's read lock.
	statMu    sync.RWMutex
	statCalcs [stat.Count]*effect.Calculator

	// collisionRadiusOverride is the runtime body-radius override a live
	// effect (e.g. Grow) installs; nil means "use the template value".
	collisionRadiusOverride atomic.Pointer[float64]

	// skillMu guards disabledSkills, this NPC's cast reuse-delay state.
	skillMu        sync.Mutex
	disabledSkills map[int32]time.Time
	maxBuffsAmount atomic.Int32
}

// OffensiveFollowLead identifies hostile NPCs that lead moving offensive-follow targets.
func (*Hostile) OffensiveFollowLead() bool { return true }

// CharacterName returns this NPC's display name for character-name packets.
func (h *Hostile) CharacterName() string { return h.Instance.Template.Name }

// Attackable reports whether inst's instance type belongs to the set of
// combat-capable NPC kinds NewHostile accepts. Callers deciding whether to
// build a live Hostile at all (rather than handling NewHostile's error)
// should check this first.
func Attackable(inst *Instance) bool {
	_, ok := hostileInstanceKinds[hostileKind(inst)]
	return ok
}

// skillDefinitions resolves loaded skill definitions for template passives.
type skillDefinitions interface {
	Definition(skill.Ref) (skill.Definition, bool)
}

// NewHostile creates a live attackable NPC wrapper for inst. skills, when
// provided, resolves inst.Template.Passives into stat funcs attached before
// current HP/MP are seeded from the calculated maxima. Folk and other
// non-attackable instance types never reach this constructor, so they never
// gain stats from template passives.
func NewHostile(inst *Instance, live *creature.Live, movement ai.MoveController, attack ai.AttackController, skills ...skillDefinitions) (*Hostile, error) {
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
	if live == nil {
		return nil, errors.New("npc: nil hostile creature")
	}
	if movement == nil {
		return nil, errors.New("npc: nil hostile movement")
	}
	if attack == nil {
		return nil, errors.New("npc: nil hostile attack")
	}
	currentSoulshots, currentSpiritshots, err := shotCounts(inst.Template)
	if err != nil {
		return nil, err
	}
	soulshotRate, spiritshotRate, err := shotRates(inst.Template)
	if err != nil {
		return nil, err
	}

	h := &Hostile{
		Instance:           inst,
		Live:               live,
		move:               movement,
		roll:               rand.Intn,
		currentSoulshots:   currentSoulshots,
		currentSpiritshots: currentSpiritshots,
		soulshotRate:       soulshotRate,
		spiritshotRate:     spiritshotRate,
	}
	h.maxBuffsAmount.Store(maxBuffCount)
	h.health = creature.NewHealth(&h.hp)
	h.running.Store(!inst.WalkMode)
	h.brain = ai.NewAttackable(h, movement, attack)
	var lookup skillDefinitions
	if len(skills) > 0 {
		lookup = skills[0]
	}
	mods, err := effect.TemplatePassiveMods(lookup, inst.Template.Passives)
	if err != nil {
		return nil, fmt.Errorf("npc %d template passives: %w", inst.Template.ID, err)
	}
	h.AddStatFuncs(mods)
	// Seed from calculated Max HP/MP after template passives attach:
	// MaxHpMul/MaxMpMul scale by CON/MEN bonus, and int-truncated maxima
	// match the persisted spawn current-hp/mp contract.
	h.hp = float64(h.MaxHP())
	h.mp = float64(int(h.MaxMPValue()))
	return h, nil
}

func shotCounts(tpl *Template) (soulshots, spiritshots int, err error) {
	if tpl.AIParams == nil {
		return 0, 0, nil
	}
	if soulshots, err = tpl.AIParams.GetIntDefault("SoulShot", 0); err != nil {
		return 0, 0, fmt.Errorf("npc %d: SoulShot AI parameter: %w", tpl.ID, err)
	}
	if spiritshots, err = tpl.AIParams.GetIntDefault("SpiritShot", 0); err != nil {
		return 0, 0, fmt.Errorf("npc %d: SpiritShot AI parameter: %w", tpl.ID, err)
	}
	return soulshots, spiritshots, nil
}

// shotRates reads the template's SoulShotRate/SpiritShotRate AI parameters,
// the percent chance RollAttackedShotRecharge rolls on each landed hit.
func shotRates(tpl *Template) (soulshotRate, spiritshotRate int, err error) {
	if tpl.AIParams == nil {
		return 0, 0, nil
	}
	if soulshotRate, err = tpl.AIParams.GetIntDefault("SoulShotRate", 0); err != nil {
		return 0, 0, fmt.Errorf("npc %d: SoulShotRate AI parameter: %w", tpl.ID, err)
	}
	if spiritshotRate, err = tpl.AIParams.GetIntDefault("SpiritShotRate", 0); err != nil {
		return 0, 0, fmt.Errorf("npc %d: SpiritShotRate AI parameter: %w", tpl.ID, err)
	}
	return soulshotRate, spiritshotRate, nil
}

// SetWorld records the world registry BroadcastAttack reaches nearby
// observers through. Call it once, after placing this NPC on the grid —
// BroadcastAttack is a no-op until then. This mirrors Decay's worldState
// parameter, which BroadcastAttack has no room for since attack.CreatureActor
// fixes its signature to the snapshot alone.
func (h *Hostile) SetWorld(state *world.State) {
	h.world = state
}

// ForEachKnownCombatantInRadius visits nearby combatants through the world grid.
func (h *Hostile) ForEachKnownCombatantInRadius(radius int, fn func(attackable.Combatant)) {
	if h.world == nil {
		return
	}
	h.world.ForEachKnownInRadius(h, radius, func(candidate world.Tracked) {
		if combatant, ok := candidate.(attackable.Combatant); ok {
			fn(combatant)
		}
	})
}

// SetFrameBuilder records the network-layer hook that translates this NPC's
// broadcast-worthy state changes into wire frames, keeping serverpackets and
// wire-encoding knowledge out of the model layer. Broadcast* is a no-op
// until both SetWorld and SetFrameBuilder have been called.
func (h *Hostile) SetFrameBuilder(b FrameBuilder) {
	h.frames = b
}

// StartAbnormalEffect adds mask to this NPC's client-visible abnormal state.
func (h *Hostile) StartAbnormalEffect(mask int) {
	h.abnormalEffect.Or(int32(mask))
}

// StopAbnormalEffect removes mask from this NPC's client-visible abnormal state.
func (h *Hostile) StopAbnormalEffect(mask int) {
	for {
		current := h.abnormalEffect.Load()
		if h.abnormalEffect.CompareAndSwap(current, current&^int32(mask)) {
			return
		}
	}
}

// AbnormalEffect returns this NPC's client-visible abnormal-effect bitmask.
func (h *Hostile) AbnormalEffect() int {
	return int(h.abnormalEffect.Load())
}

// NPCInfoSnapshot captures this NPC's current client-visible state.
func (h *Hostile) NPCInfoSnapshot() npcinfo.Snapshot {
	tmpl := h.Instance.Template
	x, y, z := h.Position()
	name, title := "", ""
	if tmpl.UsingServerSideName {
		name = tmpl.Name
	}
	if tmpl.UsingServerSideTitle {
		title = tmpl.Title
	}
	return npcinfo.Snapshot{
		ObjectID: h.ObjectID(), TemplateID: tmpl.TemplateID, Attackable: true,
		X: x, Y: y, Z: z, Heading: h.Heading(),
		MAtkSpd: h.MagicAttackSpeed(), PAtkSpd: h.AttackSpeed(),
		RunSpd: h.RunSpeed(), WalkSpd: int(tmpl.WalkSpeed),
		CurrentHP: h.CurrentHP(), MaxHP: int(h.MaxHPValue()),
		CollisionRadius: h.CollisionRadius(), CollisionHeight: tmpl.CollisionHeight,
		RightHand: tmpl.RightHand, LeftHand: tmpl.LeftHand,
		Running: h.Running(), AlikeDead: h.AlikeDead(), SummonAnimation: 2,
		AbnormalEffect: h.AbnormalEffect(), Name: name, Title: title,
	}
}

func (h *Hostile) serverObjectInfoSnapshot() npcinfo.Snapshot {
	snapshot := h.NPCInfoSnapshot()
	snapshot.Name = h.Instance.Template.Name
	return snapshot
}

// UpdateAbnormalEffect re-announces this NPC's current visible state.
func (h *Hostile) UpdateAbnormalEffect() {
	if err := h.broadcastFrame(func() wire.Frame { return h.frames.Info(h.NPCInfoSnapshot()) }); err != nil {
		h.log.Debug().Err(err).Int32("object_id", h.ObjectID()).Msg("broadcast npc abnormal effect")
	}
}

// SetLogger records where a broadcast failure from an internally-triggered
// status/death update (not routed through the AI think loop) is logged.
// The zero value discards it.
func (h *Hostile) SetLogger(log zerolog.Logger) {
	h.log = log
}

// SyncPosition moves this NPC's world-grid presence to position. A no-op
// until SetWorld has been called.
func (h *Hostile) SyncPosition(position location.Location) {
	if h.world == nil {
		return
	}
	_ = h.world.Move(h, position.X, position.Y, position.Z)
}

// SetWeapon resolves this NPC's template right-hand item id against items
// and records its weapon kind for AttackType and WeaponReuseDelay. Call it
// once, before exposing this NPC to other goroutines — same constraint as
// SetWorld. A template with no right-hand item id, an unknown item id, or a
// right-hand item that isn't a weapon leaves this NPC unarmed.
func (h *Hostile) SetWeapon(items *item.Table) {
	if items == nil || h.Instance.Template.RightHand == 0 {
		return
	}
	tmpl, ok := items.Get(int32(h.Instance.Template.RightHand))
	if !ok || tmpl.Weapon == nil {
		return
	}
	h.weapon = tmpl.Weapon
	h.weaponCrystal = tmpl.Crystal
}

// SetRollSource overrides the random source MakeAttackHit uses for its
// hit/crit/damage-spread rolls, for deterministic tests.
func (h *Hostile) SetRollSource(f func(n int) int) {
	h.roll = f
}

// SetRewarder records the reward hook TakeDamage passes to Die when its
// damage newly kills this NPC. Call it once, before exposing this NPC to
// other goroutines — same constraint as SetWorld. Leaving it unset keeps
// TakeDamage's kill path reward-free, matching Die's own "rewards may be
// nil" contract.
func (h *Hostile) SetRewarder(rewards creature.Rewarder) {
	h.rewards = rewards
}

// ObjectID returns the world object id assigned to this NPC.
func (h *Hostile) ObjectID() int32 {
	return h.Instance.ObjectID
}

// Master returns the NPC that spawned this minion, if any.
func (h *Hostile) Master() *Hostile {
	h.minionsMu.RLock()
	defer h.minionsMu.RUnlock()
	return h.master
}

// SetMaster records this NPC's spawning master.
func (h *Hostile) SetMaster(master *Hostile) {
	h.minionsMu.Lock()
	h.master = master
	h.minionsMu.Unlock()
}

// AddMinion records a child spawned for this NPC.
func (h *Hostile) AddMinion(minion *Hostile) {
	if minion == nil {
		return
	}
	h.minionsMu.Lock()
	if h.minions == nil {
		h.minions = make(map[int32]*Hostile)
	}
	h.minions[minion.ObjectID()] = minion
	h.minionsMu.Unlock()
}

// RemoveMinion forgets a child that has decayed or been removed.
func (h *Hostile) RemoveMinion(id int32) {
	h.minionsMu.Lock()
	delete(h.minions, id)
	h.minionsMu.Unlock()
}

// Minions returns a stable snapshot of this NPC's current children.
func (h *Hostile) Minions() []*Hostile {
	h.minionsMu.RLock()
	defer h.minionsMu.RUnlock()
	minions := make([]*Hostile, 0, len(h.minions))
	for _, minion := range h.minions {
		minions = append(minions, minion)
	}
	return minions
}

// IsMaster reports whether this NPC has ever recorded a minion spawn.
func (h *Hostile) IsMaster() bool {
	h.minionsMu.RLock()
	defer h.minionsMu.RUnlock()
	return h.minions != nil
}

// AI returns the hostile NPC brain.
func (h *Hostile) AI() *ai.Attackable {
	return h.brain
}

// AddDamageHate records physical threat against this NPC.
func (h *Hostile) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	h.brain.AddDamageHate(attacker, damage, hate)
}

// AddAttackDesire queues an attack intention against this NPC.
func (h *Hostile) AddAttackDesire(attacker attackable.Combatant, hate float64) {
	h.brain.AddAttackDesire(attacker, hate)
}

// AddAttackDesireHold queues a stationary attack intention against this NPC.
func (h *Hostile) AddAttackDesireHold(attacker attackable.Combatant, hate float64) {
	h.brain.AddAttackDesireHold(attacker, hate)
}

// RemoveAttackDesire zeroes target's threat hate, drops its queued attack
// desire, and aborts movement.
func (h *Hostile) RemoveAttackDesire(target attackable.Combatant) {
	h.brain.StopAggroHate(target)
	_ = h.move.Stop()
}

// AddCombatDamageHate records attacker's combat damage against this NPC,
// queuing its attack Desire at a flat weight instead of scaling it with the
// damage dealt (see ai.Attackable.AddCombatDamageHate).
func (h *Hostile) AddCombatDamageHate(attacker attackable.Combatant, damage float64) {
	h.brain.AddCombatDamageHate(attacker, damage)
}

// AddHate records skill-cast hate against this NPC.
func (h *Hostile) AddHate(attacker attackable.Combatant, hate float64) {
	h.brain.AddHate(attacker, hate)
}

// AddDefaultHate records the default skill-cast hate against this NPC.
func (h *Hostile) AddDefaultHate(attacker attackable.Combatant) {
	h.brain.AddDefaultHate(attacker)
}

// monsterInstanceKinds is the subset of hostileInstanceKinds whose
// counterpart type is Monster-family, consulted by hostility-redirect
// effects that only accept a Monster-family actor as their target.
var monsterInstanceKinds = map[InstanceKind]struct{}{
	"Chest":           {},
	"FeedableBeast":   {},
	"FestivalMonster": {},
	"GrandBoss":       {},
	"HalishaChest":    {},
	"Monster":         {},
	"RaidBoss":        {},
}

// MonsterKind reports whether this NPC's instance type is Monster-family
// (see monsterInstanceKinds) — as opposed to a Guard, SiegeGuard, or
// FriendlyMonster, which are hostile but not Monster-family.
func (h *Hostile) MonsterKind() bool {
	_, ok := monsterInstanceKinds[hostileKind(h.Instance)]
	return ok
}

// chestKind reports whether this NPC's instance type is specifically the
// lootable Chest kind. HalishaChest is Monster-family but distinct from
// Chest, and is not excluded by this check.
func (h *Hostile) chestKind() bool {
	return hostileKind(h.Instance) == "Chest"
}

// RandomNearbyMonster returns a random other Monster-family NPC known
// within radius units, excluding chests, or ok false if none exist or this
// NPC has no world placement yet.
func (h *Hostile) RandomNearbyMonster(radius int) (attackable.Combatant, bool) {
	if h.world == nil {
		return nil, false
	}
	var candidates []attackable.Combatant
	h.world.ForEachKnownInRadius(h, radius, func(obj world.Tracked) {
		other, ok := obj.(*Hostile)
		if !ok || !other.MonsterKind() || other.chestKind() {
			return
		}
		candidates = append(candidates, other)
	})
	if len(candidates) == 0 {
		return nil, false
	}
	return candidates[h.roll(len(candidates))], true
}

// RandomNearbyCombatant returns a random other attackable NPC known within
// radius units, excluding chests, or ok false if none exist or this NPC
// has no world placement yet. The reference confusion effect also
// considers nearby playable actors as candidates; no playable actor in
// this port exposes itself to this search yet, so only other attackable
// NPCs are ever found here. The search is unwidened by collision radius:
// EffectConfusion.java:43 filters candidates by plain distance2D, not
// MathUtil.checkIfInRange's body-to-body widening.
func (h *Hostile) RandomNearbyCombatant(radius int) (attackable.Combatant, bool) {
	if h.world == nil {
		return nil, false
	}
	var candidates []attackable.Combatant
	h.world.ForEachKnownInPlainRadius(h, radius, func(obj world.Tracked) {
		other, ok := obj.(*Hostile)
		if !ok || other.chestKind() {
			return
		}
		candidates = append(candidates, other)
	})
	if len(candidates) == 0 {
		return nil, false
	}
	return candidates[h.roll(len(candidates))], true
}

// StopMostHatedTarget clears this NPC's physical threat against whichever
// attacker currently sits at the top of its threat table, without
// dropping that attacker's entry. A confusion effect uses this to drop
// its forced redirect once the effect ends.
func (h *Hostile) StopMostHatedTarget() {
	if most, ok := h.brain.Threats().MostHated(); ok {
		h.brain.StopAggroHate(most.Attacker)
	}
}

// ReduceAllAggroHate subtracts amount from every threat entry and may
// return this NPC to peace when no attacker remains most-hated.
func (h *Hostile) ReduceAllAggroHate(amount float64) {
	h.brain.ReduceAllAggroHate(amount)
}

// StopAggroHate zeroes target's threat hate and may return this NPC to
// peace when no attacker remains most-hated.
func (h *Hostile) StopAggroHate(attacker attackable.Combatant) {
	h.brain.StopAggroHate(attacker)
}

// StopHateList drops target from the skill-cast hate table.
func (h *Hostile) StopHateList(attacker attackable.Combatant) {
	h.brain.Hates().StopHate(attacker)
}

// ClearAggroTables drops every threat and skill-cast hate entry.
func (h *Hostile) ClearAggroTables() {
	h.brain.Threats().Clear()
	h.brain.Hates().Clear()
}

// RandomizeHate ports Npc.java's AggroList.randomizeAttack(), the behavior
// behind EffectRandomizeHate: swaps a random valid attacker into the
// most-hated slot ahead of the current target, gated by the same
// canAutoAttack(target, PARTY_RANGE, true) rule reconsiderTarget uses.
// Reports whether a swap happened.
func (h *Hostile) RandomizeHate() bool {
	return h.brain.RandomizeHate(func(target attackable.Combatant) bool {
		return h.AutoAttackTargetValid(target, partyRangeDefault, true)
	}, h.roll)
}

// Tick advances the hostile AI clock once.
func (h *Hostile) Tick() {
	if !h.canRunAI() {
		return
	}
	h.brain.Tick()
}

// Think runs one hostile AI decision cycle.
func (h *Hostile) Think() error {
	if !h.canRunAI() {
		return nil
	}
	return h.brain.Think()
}

// OnInactiveRegion applies the hostile-NPC reset that aCis runs when the
// owning world region deactivates.
func (h *Hostile) OnInactiveRegion() {
	h.enterInactiveRegion()
}

// OnActiveRegion clears the deactivation latch once players wake the region.
func (h *Hostile) OnActiveRegion() {
	h.regionInactive.Store(false)
}

// SleepWhenRegionInactive reports whether the AI task should pause this NPC
// while no player is near its region. noSleepMode NPCs and off-territory NPCs
// keep ticking, matching the oracle's deactivation exemption.
func (h *Hostile) SleepWhenRegionInactive() bool {
	return !h.Instance.Template.NoSleepMode && h.InTerritory()
}

func (h *Hostile) canRunAI() bool {
	if h.world == nil {
		h.regionInactive.Store(false)
		return true
	}
	placed, active := h.world.RegionActivity(h)
	if !placed {
		h.regionInactive.Store(false)
		return false
	}
	if !active {
		h.enterInactiveRegion()
		return !h.SleepWhenRegionInactive()
	}
	h.regionInactive.Store(false)
	return true
}

func (h *Hostile) enterInactiveRegion() {
	if h.regionInactive.CompareAndSwap(false, true) {
		h.EffectList().StopAll()
		h.brain.SetBackToPeace()
	}
}

// SiegeGuard reports whether this NPC is a defensive siege guard.
func (h *Hostile) SiegeGuard() bool {
	return hostileKind(h.Instance) == "SiegeGuard"
}

// RaidRelated reports whether this NPC is tied to a raid encounter (a raid
// boss or one of its minions). A raid-related NPC sees through silent
// movement in AutoAttackTargetValid regardless of its template's own
// concealment-detection setting. False until SetRaidRelated marks it.
func (h *Hostile) RaidRelated() bool {
	return h.raidRelated.Load()
}

// SetRaidRelated marks or clears this NPC's raid-encounter association.
func (h *Hostile) SetRaidRelated(v bool) {
	h.raidRelated.Store(v)
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
	if !creature.Die(h, killer, rewards) {
		return false
	}
	if err := h.BroadcastDie(); err != nil {
		h.log.Warn().Err(err).Msg("npc: die broadcast")
	}
	return true
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
	h.corpseDeadline = time.Time{}
	h.deathMu.Unlock()

	if worldState != nil {
		worldState.Despawn(h)
	}
	if respawn != nil {
		respawn()
	}
	return true
}

// DenyAIAction reports whether this NPC is unable to act: dead, teleporting,
// or held by a crowd-control effect.
func (h *Hostile) DenyAIAction() bool {
	return h.AlikeDead() || h.Stunned() || h.ImmobileUntilAttacked() || h.Sleeping() || h.Paralyzed() || h.Teleporting() || h.Afraid()
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

// PoleAttackAngle returns the finalized forward cone used by pole attacks.
func (h *Hostile) PoleAttackAngle() int {
	return int(h.calcStat(stat.PowerAttackAngle, 120))
}

// PoleAttackCountMax returns the primary-inclusive pole target cap.
func (h *Hostile) PoleAttackCountMax() int {
	for _, active := range h.EffectList().All() {
		if active.Type == effect.TypePolearmTargetSingle {
			return 1
		}
	}
	return int(h.calcStat(stat.AttackCountMax, 0))
}

// ReturnHome reports whether this NPC started returning to its spawn.
func (h *Hostile) ReturnHome() bool {
	if h.SiegeGuard() {
		return h.returnHomeOutsideDriftRange()
	}
	if h.InTerritory() || !h.brain.Hates().IsEmpty() {
		return false
	}
	return h.returnHomeOutsideDriftRange()
}

// InTerritory reports whether this NPC is inside its spawn territory.
func (h *Hostile) InTerritory() bool {
	if !h.Instance.HasHome {
		return true
	}
	return h.location().In3DRange(h.Instance.Home, defaultDriftRange)
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

// IsMoving reports whether this NPC has an in-flight movement request.
func (h *Hostile) IsMoving() bool { return h.Move().Moving() }

func (h *Hostile) driftRange() int {
	if kind := hostileKind(h.Instance); kind == "Guard" || kind == "SiegeGuard" {
		return 20
	}
	if h.Instance.DriftRange > 0 {
		return h.Instance.DriftRange
	}
	return defaultDriftRange
}

func (h *Hostile) returnHomeOutsideDriftRange() bool {
	if !h.Instance.HasHome || in2DRange(h.location(), h.Instance.Home, h.driftRange()) {
		return false
	}
	h.brain.Threats().ZeroHate()
	if h.SiegeGuard() {
		h.ForceRunStance()
	} else {
		h.ForceWalkStance()
	}
	_ = h.move.MoveHome(h.Instance.Home)
	h.scheduleWanderRecheck()
	return true
}

func (h *Hostile) scheduleWanderRecheck() {
	mover, ok := h.move.(interface {
		MoveToLocation(location.Location) (bool, error)
	})
	if !ok || h.moveSpeed() <= 0 {
		return
	}
	delay := time.Duration(float64(1500+h.roll(1001))*100/float64(h.moveSpeed())) * time.Millisecond
	time.AfterFunc(delay, func() {
		if h.brain.CurrentIntention() != ai.IntentionWander {
			return
		}
		position := h.location()
		distance := min(int(h.CollisionRadius())*2, 50)
		radians := (location.HeadingDegrees(h.Heading()) + 180) * math.Pi / 180
		_, _ = mover.MoveToLocation(location.Location{
			X: position.X + int(float64(distance)*math.Cos(radians)),
			Y: position.Y + int(float64(distance)*math.Sin(radians)),
			Z: position.Z,
		})
	})
}

func in2DRange(a, b location.Location, radius int) bool {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) <= float64(radius)
}
