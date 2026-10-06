package npc

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
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

// siegeGuardHomeMoveWeight is the MOVE_TO desire weight SiegeGuard return-home
// queues so it outranks wander and ordinary combat desires.
const siegeGuardHomeMoveWeight = 1_000_000

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
	// spawnBinding is the spawn slot that placed the NPC: its AI parameters
	// and script memory.
	spawnBinding

	brain *ai.Attackable
	move  ai.MoveController
	world *world.State
	// sink receives this NPC's events. Attach installs it before the NPC is
	// published into world.State; nil drops every event.
	sink event.Sink
	log  zerolog.Logger

	// rewards computes this NPC's drop/experience payout when TakeDamage
	// kills it. It is nil until Attach installs one, in which case death
	// still latches but grants nothing — matching Die's own "rewards may be
	// nil" contract.
	rewards creature.Rewarder
	// remover takes this NPC out of the world for DeleteMe; nil despawns it
	// with no respawn. Installed by Attach.
	remover Remover
	// hits watches the hits this NPC registers; nil watches none. Installed
	// by Attach.
	hits HitObserver
	// scripts raises this NPC's script hooks; nil raises none and leaves
	// every built-in reaction on. Installed by Attach.
	scripts ScriptHooks
	// interacted latches the first unlock attempt on a chest.
	interacted atomic.Bool
	// coreAIDisabled turns off this NPC's regular combat behavior; its only
	// effect is AttackDisabled. Box chests and Halisha chests start with it
	// set.
	coreAIDisabled atomic.Bool

	// deathMu guards dead and decayed. The killing hit latches death
	// (TakeDamage → Die → MarkDead) on the attacker's queue.
	deathMu sync.Mutex
	dead    bool
	decayed bool

	// attackedByMu guards attackers recorded by positive physical hits and
	// offensive casts. Rewards read the live attackers' levels at death.
	attackedByMu sync.Mutex
	attackedBy   map[int32]attackable.Combatant

	// minionsMu guards master and the minion list. A minion claims its
	// follow slot under its master's lock from the minion's own queue
	// (claimFollowSlot).
	minionsMu sync.RWMutex
	master    *Hostile
	minions   map[int32]*Hostile
	// followSlots are the eight escort points around this NPC, occupied by
	// minion object ids (0 is empty). Only a master uses them.
	followSlots [escortSlotCount]int32
	// lastFollowingLoc is the master position this minion's last escort step
	// laid the slots out around, taken before the move; hasLastFollow reports
	// whether it is set.
	lastFollowingLoc location.Location
	hasLastFollow    bool

	// inWater reports whether a location lies in a water zone; nil means
	// none does. Set before the NPC is published.
	inWater func(location.Location) bool
	// zones is the NPC's zone membership. SetZones gives it the zone index
	// before the NPC is published.
	zones *zoneMember

	regionInactive atomic.Bool
	abnormalEffect atomic.Int32
	running        atomic.Bool
	// speedMu serializes refreshMoveSpeed's read of the stance and stats
	// with its hand-off to the movement, so the last refresh to run always
	// sets the speed that the latest stance and stat funcs give.
	speedMu sync.Mutex
	// inCombat reports an attack stance, which the stance tracker ends.
	inCombat atomic.Bool

	// geoPathFailCount counts consecutive pathfinding moves that could
	// not resolve a route, for walker teleport-to-start and SiegeGuard
	// return-home recovery (see GeoPathFailCount).
	geoPathFailCount atomic.Int32
	// maxGeoPathFailCount is the geoengine.properties overflow threshold
	// geoPathFailCount wraps at; zero means DefaultMaxGeoPathFailCount.
	maxGeoPathFailCount atomic.Int32

	// raidRelated marks this NPC as tied to a raid encounter (a raid boss
	// or one of its minions), set per-instance rather than derived from
	// the template. See RaidRelated.
	raidRelated atomic.Bool
	// raidMultipliers scale the base defences and regeneration while
	// raidRelated is set; nil leaves them unchanged. See SetRaidMultipliers.
	raidMultipliers atomic.Pointer[RaidMultipliers]
	// aiConfig holds the target-selection switches; nil means
	// DefaultAIConfig. See SetAIConfig.
	aiConfig atomic.Pointer[AIConfig]

	// cast is the live cast controller; see SetCastController.
	cast atomic.Pointer[CastControl]

	spoil          item.SpoilPool
	seed           SeedState
	overhit        overhitState
	corpseDeadline time.Time

	health creature.Health
	hp     float64
	// hpBar is the health-bar segment state PublishHP advances from
	// whichever goroutine changed HP.
	hpBar creature.HPBar
	// regen is the HP/MP regeneration task SettleRegen arms on a drop.
	regen creature.Regen

	// mpMu guards mp, the live MP value consumed by skill-resource handlers.
	// A caster's mana-burn or mana-drain skill reduces it from the caster's
	// queue (ReduceMP).
	mpMu sync.RWMutex
	mp   float64

	// weapon is this NPC's resolved right-hand weapon kind, recorded by
	// Attach. Nil means unarmed — the common case, since the
	// overwhelming majority of monster templates carry no weapon item id.
	weapon *item.WeaponDetail

	// weaponCrystal is the resolved right-hand weapon's crystal grade,
	// resolved by Attach alongside weapon. CrystalNone when unarmed.
	weaponCrystal item.CrystalType

	// offHandMask is the armor-type bit of the shield the template holds
	// in its left hand, resolved by Attach; zero when it holds none.
	offHandMask int32

	// roll draws a uniform integer in [0, n) for MakeAttackHit's hit/crit/
	// damage-spread rolls. It defaults to math/rand's global source; tests
	// substitute a fixed function for deterministic combat outcomes.
	roll func(n int) int

	los LineOfSight

	// shotsMu guards the per-spawn NPC shot counters and charge mask. It is
	// taken from other actors' queues: a hit's shot-recharge roll
	// (registerHit) runs on the attacker's queue, and so does the Think an
	// attacker's first hit runs (thinkIfNoMostHated), which launches this
	// NPC's attacks and casts.
	shotsMu            sync.RWMutex
	currentSoulshots   int
	currentSpiritshots int
	shotsMask          int32

	// statMu guards statCalcs slot creation; each slot's own Calculator
	// then guards its own Mods independently, so a warm read only ever
	// takes statMu's read lock. An attacker's formulas read these stats
	// from the attacker's queue.
	statMu    sync.RWMutex
	statCalcs [stat.Count]*effect.Calculator

	// collisionRadiusOverride is the runtime body-radius override a live
	// effect (e.g. Grow) installs; nil means "use the template value".
	collisionRadiusOverride atomic.Pointer[float64]

	// skillMu guards disabledSkills, this NPC's cast reuse-delay state. The
	// Think an attacker's first hit runs on the attacker's queue
	// (thinkIfNoMostHated) checks and sets reuse delays through the cast
	// controller.
	skillMu        sync.Mutex
	disabledSkills map[int32]time.Time
	maxBuffsAmount atomic.Int32
}

// OffensiveFollowLead identifies hostile NPCs that lead moving offensive-follow targets.
func (*Hostile) OffensiveFollowLead() bool { return true }

// CharacterName returns this NPC's display name for character-name packets.
func (h *Hostile) CharacterName() string { return h.Instance.Name() }

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

	h := &Hostile{
		Instance:           inst,
		spawnBinding:       newSpawnBinding(inst.Template),
		Live:               live,
		move:               movement,
		roll:               rand.Intn,
		currentSoulshots:   currentSoulshots,
		currentSpiritshots: currentSpiritshots,
	}
	h.zones = newZoneMember(h)
	h.initSeedState()
	h.maxBuffsAmount.Store(maxBuffCount)
	// Raid and grand bosses are raid-related from construction; minions
	// are marked at spawn (see SetRaidRelated).
	h.raidRelated.Store(h.RaidBoss())
	h.coreAIDisabled.Store(h.Box() || kind == "HalishaChest")
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
	// The live movement starts at the template speed; move at the stat-
	// finalized one from the first step.
	h.refreshMoveSpeed()
	// Seed from calculated Max HP/MP after template passives attach:
	// MaxHpMul/MaxMpMul scale by CON/MEN bonus, and int-truncated maxima
	// match the persisted spawn current-hp/mp contract.
	h.hp = float64(h.MaxHP())
	h.mp = float64(int(h.MaxMPValue()))
	h.hpBar.Calibrate(float64(h.MaxHP()))
	return h, nil
}

// shotCounts reads the template's own SoulShot/SpiritShot AI parameters,
// the shots the NPC spawns with. A spawn's parameters do not change them.
func shotCounts(tpl *Template) (soulshots, spiritshots int, err error) {
	ss, err := tpl.AIParams.Int("SoulShot", 0)
	if err != nil {
		return 0, 0, fmt.Errorf("npc %d: %w", tpl.ID, err)
	}
	sps, err := tpl.AIParams.Int("SpiritShot", 0)
	if err != nil {
		return 0, 0, fmt.Errorf("npc %d: %w", tpl.ID, err)
	}
	return int(ss), int(sps), nil
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

// Runtime is everything a Hostile needs to act in the live world beyond its
// template and controllers. A nil dependency leaves the matching behavior
// off: no world means no spatial queries, no LOS means CanSee is permissive,
// no Items leaves the NPC unarmed, no Rewards makes a kill reward-free, and
// no Sink drops every event.
type Runtime struct {
	World   *world.State
	LOS     LineOfSight
	Log     zerolog.Logger
	Items   *item.Table
	Rewards creature.Rewarder
	Sink    event.Sink
	Remover Remover
	// Hits watches every hit this NPC registers; nil watches none.
	Hits HitObserver
	// Scripts raises the NPC's script hooks; nil raises none.
	Scripts ScriptHooks
	// Slot is the spawn slot that placed the NPC; nil leaves it with no
	// spawn parameters and a script memory of its own.
	Slot SpawnSlot
}

// Attach installs rt. Call it once, before exposing this NPC to other
// goroutines. Items resolves the template's right-hand item id into the
// weapon kind AttackType and WeaponReuseDelay read; a template with no
// right-hand item id, an unknown id, or a non-weapon item leaves the NPC
// unarmed. It resolves the left-hand item id the same way into the shield
// HeldItemTypeMask reports.
func (h *Hostile) Attach(rt Runtime) {
	h.world = rt.World
	h.los = rt.LOS
	h.log = rt.Log
	h.rewards = rt.Rewards
	h.sink = rt.Sink
	h.remover = rt.Remover
	h.hits = rt.Hits
	h.scripts = rt.Scripts
	h.bindSpawn(rt.Slot)
	if rt.Items == nil {
		return
	}
	if id := h.Instance.Template.LeftHand; id != 0 {
		if tmpl, ok := rt.Items.Get(int32(id)); ok && tmpl.Kind == item.KindArmor && tmpl.Armor != nil {
			h.offHandMask = tmpl.Armor.Type.Mask()
		}
	}
	if h.Instance.Template.RightHand == 0 {
		return
	}
	tmpl, ok := rt.Items.Get(int32(h.Instance.Template.RightHand))
	if !ok || tmpl.Weapon == nil {
		return
	}
	h.weapon = tmpl.Weapon
	h.weaponCrystal = tmpl.Crystal
}

// HeldItemTypeMask returns the item-type bits of this NPC's right-hand
// weapon and of the armor, a shield, in its left hand. An unarmed NPC holds
// no weapon bit at all, not a fist's.
func (h *Hostile) HeldItemTypeMask() int32 {
	mask := h.offHandMask
	if h.weapon != nil {
		mask |= h.weapon.Type.Mask()
	}
	return mask
}

func (h *Hostile) emit(e event.Event) {
	if h.sink != nil {
		h.sink.Emit(e)
	}
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

// AbnormalEffect returns this NPC's client-visible abnormal-effect bitmask:
// the stored visual bits plus the ones its live crowd-control state
// implies.
func (h *Hostile) AbnormalEffect() int {
	return int(h.abnormalEffect.Load()) | h.EffectList().CrowdControlAbnormalEffect()
}

// NPCInfoSnapshot captures this NPC's current client-visible state.
func (h *Hostile) NPCInfoSnapshot() npcinfo.Snapshot {
	tmpl := h.Instance.Template
	x, y, z := h.Position()
	name, title := "", ""
	if tmpl.UsingServerSideName {
		name = h.Instance.Name()
	}
	if tmpl.UsingServerSideTitle {
		title = h.Instance.Title()
	}
	pAtkSpd := h.AttackSpeed()
	return npcinfo.Snapshot{
		ObjectID: h.ObjectID(), TemplateID: tmpl.TemplateID, Attackable: true,
		X: x, Y: y, Z: z, Heading: h.Heading(),
		MAtkSpd: h.MagicAttackSpeed(), PAtkSpd: pAtkSpd,
		RunSpd: int(tmpl.RunSpeed), WalkSpd: int(tmpl.WalkSpeed),
		MoveMultiplier: float64(h.MovementSpeedMultiplier()), AtkSpdMultiplier: npcinfo.AttackSpeedMultiplier(pAtkSpd, tmpl.AtkSpd),
		MoveType:  h.zones.moveType(),
		CurrentHP: h.CurrentHP(), MaxHP: int(h.MaxHPValue()),
		CollisionRadius: h.CollisionRadius(), CollisionHeight: tmpl.CollisionHeight,
		RightHand: tmpl.RightHand, LeftHand: tmpl.LeftHand,
		Running: h.Running(), InCombat: h.InCombat(), AlikeDead: h.AlikeDead(), SummonAnimation: 2,
		AbnormalEffect: h.AbnormalEffect(), Name: name, Title: title,
	}
}

// ServerObjectInfoSnapshot is NPCInfoSnapshot with the template's server-side
// name always shown, the view an immobile NPC is announced with.
func (h *Hostile) ServerObjectInfoSnapshot() npcinfo.Snapshot {
	snapshot := h.NPCInfoSnapshot()
	snapshot.Name = h.Instance.Name()
	return snapshot
}

// UpdateAbnormalEffect re-announces this NPC's current visible state.
func (h *Hostile) UpdateAbnormalEffect() {
	h.emit(event.AbnormalEffectChanged{})
}

// SyncPosition moves this NPC's world-grid presence to position, a movement
// step. A no-op until Attach installs a world.
func (h *Hostile) SyncPosition(position location.Location) {
	h.relocate(position, false)
}

// relocate moves this NPC's world-grid presence to position and reports the
// move to its zones: a movement step, or with placed a position set outside
// movement.
func (h *Hostile) relocate(position location.Location, placed bool) {
	if h.world == nil {
		return
	}
	previous := h.location()
	_ = h.world.Move(h, position.X, position.Y, position.Z)
	if placed {
		h.zones.place(previous)
		return
	}
	h.zones.step(previous)
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

// Kind reports KindNPC.
func (h *Hostile) Kind() actor.Kind { return actor.KindNPC }

// Unlockable reports whether this hostile NPC is a chest.
func (h *Hostile) Unlockable() bool { return hostileKind(h.Instance) == "Chest" }

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

// ClearMinions forgets every child, leaving them in the world; the NPC
// stays a master.
func (h *Hostile) ClearMinions() {
	h.minionsMu.Lock()
	h.minions = make(map[int32]*Hostile)
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
	h.move.Stop()
}

// AddCombatDamageHate records attacker's combat damage against this NPC,
// queuing its ATTACKED-event attack Desire at attackedHateWeight's
// approximation of the per-script attacked-hate formula (see
// ai.Attackable.AddCombatDamageHate). A hit adds it only for an NPC with no
// bound behavior.
func (h *Hostile) AddCombatDamageHate(attacker attackable.Combatant, damage float64) {
	h.brain.AddCombatDamageHate(attacker, damage, h.attackedHateWeight(attacker, damage))
}

// AddHate records skill-cast hate against this NPC.
func (h *Hostile) AddHate(attacker attackable.Combatant, hate float64) {
	h.brain.AddHate(attacker, hate)
}

// AddDefaultHate records the default skill-cast hate against this NPC.
func (h *Hostile) AddDefaultHate(attacker attackable.Combatant) {
	h.brain.AddDefaultHate(attacker)
}

// RecordAttacker keeps a creature that physically hit or cast an offensive
// skill at this NPC, independently of damage and hate accounting.
func (h *Hostile) RecordAttacker(attacker attackable.Combatant) {
	if attacker == nil || attacker.ObjectID() == h.ObjectID() {
		return
	}
	h.attackedByMu.Lock()
	if h.attackedBy == nil {
		h.attackedBy = make(map[int32]attackable.Combatant)
	}
	h.attackedBy[attacker.ObjectID()] = attacker
	h.attackedByMu.Unlock()
}

// HighestAttackerLevel returns the highest current level in the attacked-by
// set, or fallback when no attack has been recorded.
func (h *Hostile) HighestAttackerLevel(fallback int) int {
	h.attackedByMu.Lock()
	attackers := make([]attackable.Combatant, 0, len(h.attackedBy))
	for _, attacker := range h.attackedBy {
		attackers = append(attackers, attacker)
	}
	h.attackedByMu.Unlock()
	if len(attackers) == 0 {
		return fallback
	}
	level := 0
	for _, attacker := range attackers {
		level = max(level, attacker.Level())
	}
	return level
}

func (h *Hostile) clearAttackers() {
	h.attackedByMu.Lock()
	h.attackedBy = nil
	h.attackedByMu.Unlock()
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

// FolkOrGuard reports whether this NPC is a town Guard. Folk-family NPCs
// are not Hostile, so Guard is the only kind that can qualify here.
func (h *Hostile) FolkOrGuard() bool {
	return hostileKind(h.Instance) == "Guard"
}

// FeedableBeast reports whether this NPC is of the FeedableBeast kind, the
// only target a beast spice is fed to.
func (h *Hostile) FeedableBeast() bool {
	return hostileKind(h.Instance) == "FeedableBeast"
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

// RandomNearbyCombatant returns a random confusion target known within
// radius units of this NPC; see RandomConfusionTarget.
func (h *Hostile) RandomNearbyCombatant(radius int) (attackable.Combatant, bool) {
	target, ok := RandomConfusionTarget(h.world, h, radius, h.roll)
	if !ok {
		return nil, false
	}
	combatant, ok := target.(attackable.Combatant)
	return combatant, ok
}

// RandomConfusionTarget returns a random creature a confused self may turn
// on: an attackable NPC other than a lootable chest, a player, or a summon,
// known within radius units of self. Every result is also an
// attackable.Combatant. Distance is measured point to point on the
// horizontal plane: height differences are ignored and collision radii do
// not widen the search. ok is false when none is in range or self has no
// world placement yet. roll draws a uniform integer in [0, n).
func RandomConfusionTarget(w *world.State, self world.Tracked, radius int, roll func(n int) int) (world.Tracked, bool) {
	if w == nil {
		return nil, false
	}
	var candidates []world.Tracked
	w.ForEachKnownIn2DRadius(self, radius, func(obj world.Tracked) {
		if other, ok := obj.(*Hostile); ok {
			if !other.chestKind() {
				candidates = append(candidates, other)
			}
			return
		}
		if _, ok := obj.(attackable.Combatant); ok && obj.Kind().Playable() {
			candidates = append(candidates, obj)
		}
	})
	if len(candidates) == 0 {
		return nil, false
	}
	return candidates[roll(len(candidates))], true
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

// AggroHate returns this NPC's threat-table hate against attacker, or 0
// when attacker is not listed.
func (h *Hostile) AggroHate(attacker attackable.Combatant) float64 {
	if h == nil || h.brain == nil || attacker == nil {
		return 0
	}
	return h.brain.Threats().Hate(attacker)
}

// NpcID returns this NPC's template id.
func (h *Hostile) NpcID() int {
	return h.Instance.Template.ID
}

var (
	_ creature.RaidCurseTarget    = (*Hostile)(nil)
	_ creature.RaidCurseSkillRaid = (*Hostile)(nil)
)

// StopHateList drops target from the skill-cast hate table.
func (h *Hostile) StopHateList(attacker attackable.Combatant) {
	h.brain.Hates().StopHate(attacker)
}

// ClearAggroTables drops every threat and skill-cast hate entry.
func (h *Hostile) ClearAggroTables() {
	h.brain.Threats().Clear()
	h.brain.Hates().Clear()
}

// RandomizeHate is attack randomization, the behavior behind the
// randomize-hate effect: swaps a random valid attacker into the most-hated
// slot ahead of the current target, gated by the same AutoAttackTargetValid
// rule ReconsiderTarget uses, here at party range with peaceful targets
// allowed.
// Reports whether a swap happened.
func (h *Hostile) RandomizeHate() bool {
	return h.brain.RandomizeHate(func(target attackable.Combatant) bool {
		return h.AutoAttackTargetValid(target, partyRangeDefault, true)
	}, h.roll)
}

// LifeTime returns the number of AI cycles h has completed since it
// spawned, zero again once it dies. Safe from any goroutine.
func (h *Hostile) LifeTime() int32 { return h.brain.LifeTime() }

// Tick advances the hostile AI clock once.
func (h *Hostile) Tick() {
	if !h.canRunAI() {
		return
	}
	h.brain.Tick()
}

// AtHookPoint is where h's AI loop gives the behavior bound to h's template
// its turn. No behavior binds a hook point yet, so it does nothing.
func (h *Hostile) AtHookPoint(ai.HookPoint) {}

// Think continues the hostile AI's current intention after a bow's reuse
// ending or a control effect ending. It never idles on an empty desire
// queue: RunAI and TickThink do.
func (h *Hostile) Think() error {
	if !h.canRunAI() {
		return nil
	}
	return h.brain.Think()
}

// RunAI re-runs the hostile AI's desire selection on an event: the hit
// animation ending, a bow shot landing or a completed cast. Unlike Think it idles an actor whose desire queue ran empty.
func (h *Hostile) RunAI() error {
	if !h.canRunAI() {
		return nil
	}
	return h.brain.RunAI()
}

// AttackFinished runs the hostile AI on a finished swing: desire selection
// as RunAI, and for an out-of-control NPC, which selects nothing, one
// continue step of its current intention.
func (h *Hostile) AttackFinished() error {
	if !h.canRunAI() {
		return nil
	}
	return h.brain.AttackFinished()
}

// CastFinished ends the AI's hold on a cast that completed or was aborted:
// the desire that drove it is dropped, and only a completed cast re-runs
// desire selection.
func (h *Hostile) CastFinished(interrupted bool) error {
	h.brain.ClearCurrentDesire()
	if interrupted {
		return nil
	}
	return h.RunAI()
}

// TickThink runs one periodic AI cycle, including empty-queue idle abort
// after the first cycle.
func (h *Hostile) TickThink() error {
	if !h.canRunAI() {
		return nil
	}
	return h.brain.TickThink()
}

// OnInactiveRegion applies the hostile-NPC reset when the owning world region
// deactivates. The player whose departure deactivated the region calls it on
// that player's queue, so the reset is posted to this NPC's queue and applies
// only if the NPC is still placed in an inactive region when it runs.
func (h *Hostile) OnInactiveRegion() {
	reset := func() {
		if h.world != nil {
			if placed, active := h.world.RegionActivity(h); !placed || active {
				return
			}
		}
		h.enterInactiveRegion()
	}
	h.Queue().Post(reset)
}

// OnActiveRegion clears the deactivation latch once players wake the region.
func (h *Hostile) OnActiveRegion() {
	h.regionInactive.Store(false)
}

// SleepWhenRegionInactive reports whether the AI task should pause this NPC
// while no player is near its region. noSleepMode NPCs and off-territory NPCs
// keep ticking: they are exempt from deactivation.
func (h *Hostile) SleepWhenRegionInactive() bool {
	return !h.Instance.Template.NoSleepMode && h.InTerritory()
}

// AISleeping reports whether the AI task leaves the NPC alone: it is dead,
// out of the world, or in an inactive region it sleeps in.
func (h *Hostile) AISleeping() bool {
	if h.Dead() {
		return true
	}
	if h.world == nil {
		return false
	}
	placed, active := h.world.RegionActivity(h)
	return !placed || (!active && h.SleepWhenRegionInactive())
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
		h.clearAttackers()
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
// concealment-detection setting, and can't be struck by a lethal hit.
func (h *Hostile) RaidRelated() bool {
	return h.raidRelated.Load()
}

// SetRaidRelated marks or clears this NPC's raid-encounter association.
// Raid and grand bosses start marked; a Monster-family private spawned for a
// raid boss master is marked by the spawner (MinionSpawn.doSpawn).
func (h *Hostile) SetRaidRelated(v bool) {
	h.raidRelated.Store(v)
}

// RaidBoss reports whether this NPC is a raid or grand boss itself, as
// opposed to RaidRelated, which also covers its minions.
func (h *Hostile) RaidBoss() bool {
	switch hostileKind(h.Instance) {
	case "RaidBoss", "GrandBoss":
		return true
	}
	return false
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

// MarkDead clears HP and transitions this NPC into its dead state. It reports false when
// the NPC was already dead, so a repeated or concurrent kill is a no-op.
func (h *Hostile) MarkDead() bool {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	if h.dead {
		return false
	}
	h.health.SetCurrent(0)
	h.dead = true
	return true
}

// Die runs this NPC's death sequence: the once-only dead-state
// transition, the strip of every effect that does not last through death,
// then its reward hook. rewards may be nil — the drop and
// experience/SP systems land separately and plug in here once ready. It
// reports whether the death was newly applied by this call.
//
// The caller is responsible for registering the corpse with the decay
// task afterwards (using Instance.Template.CorpseTime as the display
// interval) — Hostile does not hold a reference to that task, so the
// scheduling stays at the orchestration layer that owns it.
func (h *Hostile) Die(killer attackable.Combatant, rewards creature.Rewarder) bool {
	if !h.MarkDead() {
		return false
	}
	h.BroadcastStatus()
	h.AbortAll(true)
	h.brain.ResetLifeTime()
	// A death strip ends each effect's stat change silently; the status
	// broadcast below is the only refresh observers get.
	h.EffectList().StopAllExceptThoseThatLastThroughDeath()
	if rewards != nil {
		rewards.CalculateRewards(killer)
	}
	h.BroadcastStatus()
	h.clearAttackers()
	h.BroadcastDie()
	if h.RaidBoss() && killedByPlayer(killer) {
		h.emit(event.RaidBossKilled{})
	}
	h.raiseDying(killer)
	return true
}

// killedByPlayer reports whether killer acts for a player: a player, or a
// summon with an owner.
func killedByPlayer(killer attackable.Combatant) bool {
	if killer == nil {
		return false
	}
	switch killer.Kind() {
	case actor.KindPlayer:
		return true
	case actor.KindSummon:
		_, ok := killer.Owner()
		return ok
	}
	return false
}

// Decayed reports whether this NPC's corpse has already been removed from
// the world.
func (h *Hostile) Decayed() bool {
	h.deathMu.Lock()
	defer h.deathMu.Unlock()
	return h.decayed
}

// Decay removes this NPC's corpse from the world, stops every effect it
// still holds and runs the respawn hook, if any. Its decayed script hooks
// run first, while it is still in the world. It is idempotent: a repeat
// call is a no-op, matching the once-only guarantee the corpse decay task
// relies on.
//
// worldState may be nil in tests that do not track live world placement.
// respawn is called after the world removal when non-nil; a live spawn
// runtime is expected to close over its own spawn.State/spawn.Entry
// linkage and call spawn.CalculateRespawnDelay plus spawn.State.SetRespawn
// there, since Hostile itself carries no spawn linkage yet.
func (h *Hostile) Decay(worldState *world.State, respawn func()) bool {
	return h.DecayWithRespawn(worldState, func(int32) func() { return respawn })
}

// DecayWithRespawn is Decay with the respawn hook resolved by respawnFor,
// called with this NPC's object id only once this call has won the decay
// and before its decayed hooks run. Of several concurrent decays, only the
// winner claims the spawn's respawn, so it is armed exactly once.
// respawnFor may be nil.
func (h *Hostile) DecayWithRespawn(worldState *world.State, respawnFor func(id int32) func()) bool {
	h.deathMu.Lock()
	if h.decayed {
		h.deathMu.Unlock()
		return false
	}
	h.decayed = true
	h.dead = true
	h.corpseDeadline = time.Time{}
	h.deathMu.Unlock()
	var respawn func()
	if respawnFor != nil {
		respawn = respawnFor(h.ObjectID())
	}
	h.raiseDecayed()

	// The NPC leaves its zones while its observers still know it.
	h.zones.leave(h.location())
	if worldState != nil {
		worldState.Despawn(h)
	}
	// End whatever outlived the death strip (effects that last through
	// death, or anything on a living NPC removed at once) with their exit
	// hooks, after the world removal so nobody sees them go. Untrack then
	// keeps a straggler tick or skill task already on this queue from
	// registering the list with task.Effects again.
	h.EffectList().StopAll()
	h.EffectList().Untrack()
	if respawn != nil {
		respawn()
	}
	// A respawn is a new Hostile on a new queue; this one takes no more work.
	h.Queue().Close()
	return true
}

// DenyAIAction reports whether this NPC is unable to act: dead, teleporting,
// or held by a crowd-control effect.
func (h *Hostile) DenyAIAction() bool {
	return h.AlikeDead() || h.Stunned() || h.ImmobileUntilAttacked() || h.Sleeping() || h.Paralyzed() || h.Teleporting() || h.Afraid()
}

// OutOfControl reports whether this NPC's AI cannot choose a new intention:
// DenyAIAction, or confused. Confusion counts once its start has run, so
// the attack desire a confusion start queues is still selected at once.
func (h *Hostile) OutOfControl() bool {
	return h.DenyAIAction() || h.EffectList().StartedAffected(effect.FlagConfused)
}

// Knows reports whether target is currently visible to this NPC.
// attackable stays a leaf, so a Combatant is not statically a world object;
// one that is not on the grid is never known, nor is an invisible player
// or its summon.
func (h *Hostile) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(h, tracked) && !attackable.HiddenActingPlayer(target)
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
	if hostileKind(h.Instance) == "GrandBoss" {
		return false
	}
	if h.SiegeGuard() {
		return h.returnHomeOutsideDriftRange()
	}
	if h.InTerritory() || !h.brain.Hates().IsEmpty() {
		return false
	}
	return h.returnHomeOutsideDriftRange()
}

// InTerritory reports whether this NPC is inside its spawn territory.
// A living private uses its master's territory. A dead master is treated
// as unlinked, so the private stays in-territory for the corpse window.
// Maker NPCs with a resolved territory use banned-then-allowed polygon
// containment. A nil maker, or a maker with no territories, uses a strict
// 200-unit 3D sphere around Home.
func (h *Hostile) InTerritory() bool {
	if master := h.Master(); master != nil {
		if master.Dead() {
			return true
		}
		return master.InTerritory()
	}
	if maker := h.Instance.Maker; maker != nil && len(maker.Territories) > 0 {
		loc := h.location()
		if maker.ContainsBanned(loc) {
			return false
		}
		return maker.Contains(loc)
	}
	if !h.Instance.HasHome {
		return true
	}
	return h.location().In3DRadius(h.Instance.Home, defaultDriftRange)
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

// RestoreSpawnHeadingIfAtHome faces the spawn heading when this NPC has
// landed exactly on its spawn point. Escort FOLLOW arrivals skip the
// caller, so a minion whose master is standing on the minion spawn keeps
// the heading it had while following.
func (h *Hostile) RestoreSpawnHeadingIfAtHome() {
	if h.Instance == nil || !h.Instance.HasHome {
		return
	}
	if h.location() != h.Instance.Home {
		return
	}
	h.SetHeading(h.Instance.SpawnHeading)
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
	if !h.Instance.HasHome || h.location().In2DRadius(h.Instance.Home, h.driftRange()) {
		return false
	}
	h.brain.Threats().ZeroHate()
	if h.SiegeGuard() {
		h.ForceRunStance()
	} else {
		h.ForceWalkStance()
	}
	if h.SiegeGuard() {
		if h.GeoPathFailCount() >= move.HomeGeoFailLimit {
			_ = h.move.MoveHome(h.Instance.Home)
			return true
		}
		// AddMoveToDesire drops an unreachable or movement-disabled home.
		// Count only a reachability miss as a path failure so the teleport
		// recovery above still trips; a root expires on its own and must
		// not burn fail count.
		if !h.brain.AddMoveToDesire(h.Instance.Home, siegeGuardHomeMoveWeight) && !h.MovementDisabled() {
			h.AddGeoPathFailCount()
		}
		return true
	}
	if h.GeoPathFailCount() >= move.HomeGeoFailLimit {
		_ = h.move.MoveHome(h.Instance.Home)
		return true
	}
	if !h.MovementDisabled() {
		_ = h.move.MoveHome(h.Instance.Home)
	}
	h.scheduleWanderRecheck()
	return true
}

func (h *Hostile) scheduleWanderRecheck() {
	speed := float32(h.MoveSpeed())
	if speed <= 0 {
		return
	}
	delay := time.Duration(int32(float32(1500+h.roll(1001))*(100/speed))) * time.Millisecond
	recheck := func() {
		if h.brain.CurrentIntention() != ai.IntentionWander || h.MovementDisabled() {
			return
		}
		position := h.location()
		distance := min(int(h.CollisionRadius())*2, 50)
		radians := (location.HeadingDegrees(h.Heading()) + 180) * math.Pi / 180
		_, _ = h.move.MoveToLocation(location.Location{
			X: position.X + int(float64(distance)*math.Cos(radians)),
			Y: position.Y + int(float64(distance)*math.Sin(radians)),
			Z: position.Z,
		})
	}
	h.Queue().After(delay, recheck)
}
