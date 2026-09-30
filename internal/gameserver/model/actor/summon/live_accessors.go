package summon

import (
	"math"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (a *Actor) ObjectID() int32 { return a.id }

// Move returns this summon's lifetime movement state, mirroring
// creature.Live.Move (internal/gameserver/model/actor/creature/live.go):
// the same *move.CreatureMove a wired move.Controller drives, so
// Move().Moving() reflects real in-motion state once InitMovement has run.
func (a *Actor) Move() *move.CreatureMove { return &a.movement }

// InitMovement wires real geodata into this summon's movement state,
// matching creature.NewLive's Init call, moving at baseRunSpeed (the
// template run speed) through the RUN_SPEED stat. Call it once, before
// building a move.Controller over Move() (see GameClientLink.wireSummonAI);
// a summon left uninitialized (no geodata available) keeps a stationary,
// never-moving zero-value CreatureMove.
func (a *Actor) InitMovement(origin location.Location, baseRunSpeed float64, geo move.Geo) error {
	if err := a.movement.Init(origin, a.MoveSpeed(baseRunSpeed), geo); err != nil {
		return err
	}
	a.baseRunSpeed = baseRunSpeed
	a.movementReady.Store(true)
	return nil
}

// refreshMoveSpeed hands the current move speed to the movement
// simulation, re-timing a leg in flight. It does nothing before
// InitMovement.
func (a *Actor) refreshMoveSpeed() {
	if !a.movementReady.Load() {
		return
	}
	a.movement.SetSpeed(a.MoveSpeed(a.baseRunSpeed))
}

// SetQueue makes q, the owner's queue, the queue this summon's work runs on,
// movement arrivals included. Call it once, before the summon is published
// into the world.
func (a *Actor) SetQueue(q *sim.Queue) {
	a.queue.Store(q)
	a.movement.SetQueue(q)
	a.effects.SetQueue(q)
}

// Queue returns the queue this summon's work runs on.
func (a *Actor) Queue() *sim.Queue {
	if a == nil {
		return nil
	}
	return a.queue.Load()
}

// Now reads the clock this summon's queue runs on.
func (a *Actor) Now() time.Time { return a.Queue().Now() }

// Kind reports KindSummon.
func (a *Actor) Kind() actor.Kind { return actor.KindSummon }

// OwnerID returns the owning player's world object id.
func (a *Actor) OwnerID() int32 {
	owner := a.currentOwner()
	if owner == nil {
		return 0
	}
	return owner.ObjectID()
}

// OwnerStillLinked reports whether the owner still identifies this summon as
// their active one. DecayTaskManager cancels a tracked summon corpse when this
// returns false, before its deadline is checked. A servitor's corpse whose
// owner left stays linked to the session that left it, which no longer
// changes its summon.
func (a *Actor) OwnerStillLinked() bool {
	if a == nil || a.world == nil {
		return false
	}
	if !a.isPet && a.OwnerLeft() {
		return true
	}
	active, ok := a.world.Summon(a.OwnerID())
	if !ok {
		return false
	}
	return active.ObjectID() == a.ObjectID()
}

// ControlItemID returns the collar item object id backing this pet.
func (a *Actor) ControlItemID() int32 { return a.controlItemID }

// Level returns the summon's current level.
func (a *Actor) Level() int {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.level
}

// UpdateStatus reports that this summon's current status must be
// republished to its owner's pet window and observers. A vitals setter uses
// BroadcastStatus instead, which also reaches the players targeting it.
func (a *Actor) UpdateStatus() {
	a.emit(event.StatusChanged{})
}

// SyncControlItemEnchant lifts the owner's control item to this pet's
// current level when they differ. It refreshes the owner info window first,
// then persists the item through the inventory pipeline. A matching
// enchant is a no-op.
func (a *Actor) SyncControlItemEnchant() bool {
	if a == nil || !a.isPet || a.controlItemID == 0 {
		return false
	}
	inv := a.ownerInv()
	if inv == nil {
		return false
	}
	inst := inv.ItemByObjectID(a.controlItemID)
	if inst == nil {
		return false
	}
	level := a.Level()
	if inst.Snapshot().EnchantLevel == level {
		return false
	}
	a.emit(event.OwnerInfoChanged{})
	return inv.SetEnchantLevel(inst, level)
}

func (a *Actor) notifyDamage(attacker attackable.Combatant, amount float64) {
	a.emit(event.Damaged{AttackerName: attacker.CharacterName(), Damage: int32(amount)})
}

// IsPet reports whether this live summon is a pet rather than a servitor.
func (a *Actor) IsPet() bool { return a.isPet }

// ExpPenalty is the share of kill exp this servitor withholds from its
// owner; 0 for a pet.
func (a *Actor) ExpPenalty() float32 { return a.expPenalty }

// SummonType returns the client-visible summon type code.
func (a *Actor) SummonType() int {
	if a.isPet {
		return 2
	}
	return 1
}

// NPCID returns the template id backing this summon.
func (a *Actor) NPCID() int { return a.npcID }

// Name returns this summon's display name: the npc template name it was
// spawned with, or a saved pet's own restored name.
func (a *Actor) Name() string {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.name
}

// CharacterName returns this summon's display name for character-name packets.
func (a *Actor) CharacterName() string { return a.Name() }

// SetName overrides this summon's display name, e.g. from a restored save
// row or an owner-issued rename.
func (a *Actor) SetName(name string) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.name = name
}

// IsNamed reports whether this pet has a player-assigned custom name, as
// opposed to falling back to its npc template's name (Pet.getName() != null
// in the reference).
func (a *Actor) IsNamed() bool {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.named
}

// SetNamed marks whether this pet has a player-assigned custom name.
func (a *Actor) SetNamed(named bool) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.named = named
}

// PetState returns the collar id and durable state for a live pet. Name is
// persisted only once the pet has been explicitly named (IsNamed): the
// npc-template fallback name must never round-trip through storage as a
// saved custom name, or a later restore would misread "has a saved display
// name" as "was explicitly named" (Pet.getName() != null in the reference
// stays null until an actual rename).
func (a *Actor) PetState() (int32, petmodel.State, bool) {
	if a == nil || !a.isPet || a.controlItemID == 0 {
		return 0, petmodel.State{}, false
	}
	a.statusMu.RLock()
	name := ""
	if a.named {
		name = a.name
	}
	state := petmodel.State{Name: name, Level: a.level, Exp: a.exp, SP: a.sp, Fed: a.fed}
	a.statusMu.RUnlock()
	a.vitals.mu.RLock()
	state.CurHP, state.CurMP = a.vitals.hp, a.vitals.mp
	a.vitals.mu.RUnlock()
	return a.controlItemID, state, true
}

// ScaledExpGain returns rawExp multiplied by this pet's configured
// experience rate.
func (a *Actor) ScaledExpGain(rawExp int64) int64 {
	if a == nil || !a.isPet {
		return 0
	}
	if a.petConfig == nil {
		return petmodel.DefaultConfig().ScaledExpGain(a.npcID, rawExp)
	}
	return a.petConfig.ScaledExpGain(a.npcID, rawExp)
}

func (a *Actor) ExpType() int {
	if a == nil || !a.isPet {
		return 0
	}
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.expType
}

// CanReceiveKillReward reports whether this pet meets the reference's
// maximum-experience, life, and owner-distance reward gate.
func (a *Actor) CanReceiveKillReward(partyRange int) bool {
	if a == nil || !a.isPet || a.Dead() {
		return false
	}
	owner := a.currentOwner()
	if owner == nil {
		return false
	}
	a.statusMu.RLock()
	max, ok := int64(0), false
	if a.growth != nil {
		if row, found := a.growth.Levels[petMaxLevel]; found {
			max, ok = row.MaxExp, true
		}
	}
	exp := a.exp
	a.statusMu.RUnlock()
	if !ok || exp > max+10_000 {
		return false
	}
	ax, ay, az := a.Position()
	ox, oy, oz := owner.Position()
	return location.In3DRadius(ax, ay, az, ox, oy, oz, partyRange)
}

// Exp returns this pet's durable total experience.
func (a *Actor) Exp() int64 {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.exp
}

// SP returns this pet's durable skill points.
func (a *Actor) SP() int {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.sp
}

// socialActionLevelUp is the level-up animation a pet plays for everyone
// around it when its level increases.
const socialActionLevelUp = 15

// maxPetSP is the largest SP a pet can hold: SP is a 32-bit field in the
// pets table and in every pet packet.
const maxPetSP = math.MaxInt32

// AddExpAndSp grants a pet its raw kill-reward share. Experience uses the
// pet-specific configured rate; SP is deliberately unscaled. Each amount is
// applied independently and skipped when negative, and SP stops at maxPetSP.
// A grant where neither amount applies (negative exp, and SP that is
// negative or meets a full SP pool) changes nothing and tells nobody.
// Otherwise a level increase refreshes the owner, restores vitals and
// publishes them as a vitals change (BroadcastStatus), then broadcasts the
// level-up animation, before the owner is told the exp earned. A grant
// that stays inside the level tells the owner the exp earned and nothing
// else: neither the pet window nor the observers are refreshed.
func (a *Actor) AddExpAndSp(rawExp int64, sp int) {
	if a == nil || !a.isPet {
		return
	}
	expGain := a.ScaledExpGain(rawExp)
	a.statusMu.Lock()
	expApplied := expGain >= 0
	if expApplied {
		a.exp += expGain
	}
	spApplied := sp >= 0 && a.sp < maxPetSP
	if spApplied {
		a.sp += min(sp, maxPetSP-a.sp)
	}
	if !expApplied && !spApplied {
		a.statusMu.Unlock()
		return
	}
	leveled := a.refreshGrowthLocked()
	a.statusMu.Unlock()
	if leveled {
		// The level-up restores full HP through a vitals set, which also
		// refreshes the health bar of every player targeting the pet.
		a.resetVitals()
		a.SyncControlItemEnchant()
		a.BroadcastStatus()
		a.emit(event.SocialAction{ID: socialActionLevelUp})
	}
	a.emit(event.ExpGained{Exp: expGain})
}

// petMaxLevel is the first level a pet can never hold, the same sentinel
// the player level table ends on. Pet growth tables carry a row for it only
// so the top level's experience span and the kill-reward gate have a
// ceiling; petRealMaxLevel is the highest level a pet can reach.
const (
	petMaxLevel     = 81
	petRealMaxLevel = petMaxLevel - 1
)

// refreshGrowthLocked raises a's level to the highest one its experience
// has reached and applies that level's growth row, reporting whether the
// level changed. Experience that reaches past petRealMaxLevel moves the level
// not at all, even when it also crosses lower thresholds on the way: the
// whole step is refused and the pet keeps its level. statusMu must be held.
func (a *Actor) refreshGrowthLocked() bool {
	if a.growth == nil {
		return false
	}
	oldLevel := a.level
	level := a.level
	for {
		next, ok := a.growth.Levels[level+1]
		if !ok || a.exp < next.MaxExp {
			break
		}
		level++
	}
	if level <= petRealMaxLevel {
		a.level = level
	}
	row, ok := a.growth.Levels[a.level]
	if !ok {
		return a.level != oldLevel
	}
	a.expType = row.ExpType
	a.maxMeal = row.MaxMeal
	a.mealInBattle = row.MealInBattle
	a.mealInNormal = row.MealInNormal
	a.stats.PAtk = row.PAtk
	a.stats.PDef = row.PDef
	a.stats.MAtk = row.MAtk
	a.stats.MDef = row.MDef
	a.stats.MaxHP = row.MaxHP
	a.stats.MaxMP = row.MaxMP
	a.stats.SSCount = row.SSCount
	a.stats.SPSCount = row.SPSCount
	return a.level != oldLevel
}

// resetVitals fills a's HP and MP for a level-up. A dead pet keeps its
// corpse values: a level gained while dead must not raise it.
func (a *Actor) resetVitals() {
	hp, mp := a.MaxHPValue(), a.MaxMPValue()
	a.vitals.mu.Lock()
	if !a.dead {
		a.vitals.hp, a.vitals.mp = hp, mp
	}
	a.vitals.mu.Unlock()
}

// CanWearPetItem reports whether this pet can equip tmpl.
func (a *Actor) CanWearPetItem(tmpl *item.Template) bool {
	if a == nil || tmpl == nil {
		return false
	}
	switch a.npcID {
	case 12311, 12312, 12313:
		return tmpl.Slot == item.SlotHatchling
	case 12077:
		return tmpl.Slot == item.SlotWolf
	case 12526, 12527, 12528:
		return tmpl.Slot == item.SlotStrider
	case 12780, 12781, 12782:
		return tmpl.Slot == item.SlotBabyPet
	default:
		return false
	}
}

// Dead reports whether the summon is dead.
func (a *Actor) Dead() bool {
	a.vitals.mu.RLock()
	defer a.vitals.mu.RUnlock()
	return a.dead
}

// AlikeDead reports whether this summon is dead, satisfying
// attackable.Combatant so a summon can be targeted by its own owner-
// commanded skills (e.g. a self-cast special skill).
func (a *Actor) AlikeDead() bool { return a.Dead() }

// SiegeGuard always reports false: pets and servitors are never defensive
// siege guards.
func (a *Actor) SiegeGuard() bool { return false }

// siegeSummonNPCIDs are the Siege Golem, Hog Cannon, and Swoop Cannon
// servitor templates SiegeSummon.java identifies (SIEGE_GOLEM_ID,
// HOG_CANNON_ID, SWOOP_CANNON_ID), the summons the ERASE skill exempts
// (Disablers.java: `!(targetCreature instanceof SiegeSummon)`).
var siegeSummonNPCIDs = map[int]struct{}{
	14737: {},
	14768: {},
	14839: {},
}

// SiegeSummon reports whether this servitor is a siege-assault summon,
// exempt from the ERASE skill.
func (a *Actor) SiegeSummon() bool {
	_, ok := siegeSummonNPCIDs[a.npcID]
	return ok
}

// SummonOwner returns this summon's owning player.
func (a *Actor) SummonOwner() Owner { return a.currentOwner() }

// UnSummon despawns this summon as an owner-directed removal, matching
// Java's Summon.unSummon(Player owner) (this actor already knows its own
// owner, so the parameter only satisfies that contract).
func (a *Actor) UnSummon(Owner) { a.Unsummon() }

// DenyAIAction reports whether this summon cannot act now.
// See effectHeld for the effects it counts while AbortAll runs.
func (a *Actor) DenyAIAction() bool {
	return a.AlikeDead() || a.paralyzedLock() || a.Teleporting() || a.effectHeld(effect.AIDenyFlags)
}

// Paralyzed reports whether this summon is temporarily paralyzed or carries
// an active paralyze effect.
func (a *Actor) Paralyzed() bool {
	return a.paralyzedLock() || a.effects.IsAffected(effect.FlagParalyzed)
}

// paralyzedLock reports the summon's transient paralysis lock (see
// SetParalyzed), leaving paralyze effects out.
func (a *Actor) paralyzedLock() bool {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.paralyzed
}

// SetParalyzed sets or clears this summon's transient paralysis lock.
func (a *Actor) SetParalyzed(v bool) bool {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.paralyzed == v {
		return false
	}
	a.paralyzed = v
	return true
}

// Immobilized reports whether this summon's movement-lock flag is set, e.g.
// by ImobilePetBuff. Distinct from the FlagRooted effect.
func (a *Actor) Immobilized() bool {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.immobilized
}

// SetImmobilized sets or clears this summon's movement-lock flag and reports
// whether the flag actually changed. Every set, including one while the lock
// is already held, records the current follow mode and drops a following
// summon out of follow mode, idling it; every clear restores the follow mode
// the last set recorded, whatever the owner toggled in between. Before any
// set has run, a clear restores following. Two stacked locks therefore end
// in the follow mode the summon had when the second one landed.
//
// Effect hooks call this from the applying actor's queue or the effect
// list's expiry tick. Every value it touches has its own guard (stateMu,
// the atomic followOff, the AI loop's mutex), and the follow change runs
// after stateMu is released because it takes stateMu itself.
func (a *Actor) SetImmobilized(v bool) bool {
	a.stateMu.Lock()
	changed := a.immobilized != v
	a.immobilized = v
	if v {
		a.unfollowBeforeImmobilized = a.followOff.Load()
	}
	following := !a.unfollowBeforeImmobilized
	a.stateMu.Unlock()

	if !v {
		a.setFollowStatus(following)
	} else if following {
		a.setFollowStatus(false)
	}
	return changed
}

// Teleporting reports whether this summon is in a teleport transition.
func (a *Actor) Teleporting() bool {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.teleporting
}

// SetTeleporting sets or clears this summon's teleport transition.
func (a *Actor) SetTeleporting(v bool) bool {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.teleporting == v {
		return false
	}
	a.teleporting = v
	return true
}

// Knows reports whether target is currently visible to this summon.
// attackable stays a leaf, so a Combatant is not statically a world object;
// one that is not on the grid is never known.
func (a *Actor) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(a, tracked)
}

// OwnsOffensiveFollowTicker reports that summon AI already rechecks its
// attack/cast intention on the reference follow cadence.
func (*Actor) OwnsOffensiveFollowTicker() bool { return true }

// PhysicalAttackRange returns this summon's melee attack range.
func (a *Actor) PhysicalAttackRange() int { return a.combatStats().AttackRange }

// GetSkill returns the skill this summon's npc template grants at skillID,
// matching Java's Summon.getSkill. ok is false when the template doesn't
// grant that skill id at all.
func (a *Actor) GetSkill(skillID int) (modelskill.Ref, bool) {
	level, ok := a.skills[skillID]
	if !ok {
		return modelskill.Ref{}, false
	}
	return modelskill.Ref{ID: modelskill.ID(skillID), Level: level}, true
}

// CanUseSkill reports whether the owner may currently command this summon
// to use one of its special skills. Matching Java's
// RequestActionUse.useSkill, this only gates a pet on the owner-vs-pet
// level gap; it does not check out-of-control state the way movement
// commands do, and servitors have no gate at all.
func (a *Actor) CanUseSkill() bool {
	if !a.isPet {
		return true
	}
	ownerLevel := 0
	if owner := a.currentOwner(); owner != nil {
		ownerLevel = owner.LevelValue()
	}
	return a.Level()-ownerLevel <= 20
}

// TryUseSkill dispatches an owner-commanded special-skill cast: resolves
// skillID against this summon's own skill catalog, checks the level gate,
// then forwards to the attached AI. Matching Java's useSkill
// (RequestActionUse.java:453-472), the AI's tryToCast is fire-and-forget:
// its accept/reject decision (busy, cooldown, MP/mute —
// PlayableAI.java:297, void return) does not feed back into the result
// here. TryUseSkill returns false only wherever Java's useSkill would
// (unknown skill, level gap, no attached AI); a dispatched cast reports
// true even if the AI goes on to reject it. ctrl is the command's
// forced-use modifier.
func (a *Actor) TryUseSkill(skillID int, target attackable.Combatant, ctrl bool) bool {
	ref, ok := a.GetSkill(skillID)
	if !ok || !a.CanUseSkill() || a.brain == nil {
		return false
	}
	a.brain.TryToCast(target, ref, ctrl)
	return true
}

// OutOfControl reports whether the owner cannot currently command this
// summon, matching Summon.isOutOfControl (Summon.java:296-298):
// super.isOutOfControl() || isBetrayed().
func (a *Actor) OutOfControl() bool {
	return a.disabled || a.Betrayed()
}

// InCombat reports the owner's attack-stance state, matching
// Summon.isInCombat (Summon.java:302-305): _owner != null && _owner.isInCombat().
func (a *Actor) InCombat() bool {
	owner := a.currentOwner()
	return owner != nil && owner.InCombat()
}

// IsAttackingNow reports whether this summon's own attack cycle is
// currently in flight, matching CreatureAttack.isAttackingNow
// (CreatureAttack.java:56-59) as read via pet.getAttack()/servitor.getAttack()
// — the summon's own attack component, not the owner's.
func (a *Actor) IsAttackingNow() bool {
	return a.brain != nil && a.brain.AttackingNow()
}

// CurrentTarget returns the summon target selected by its current command.
func (a *Actor) CurrentTarget() world.Tracked {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.target
}

// SetTarget updates the summon target without issuing an owner-visible packet.
func (a *Actor) SetTarget(target world.Tracked) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	a.target = target
}

// AttackTarget forwards an aggression-triggered attack to the summon AI.
func (a *Actor) AttackTarget(target world.Tracked) { a.TryToAttack(target) }

// TryToAttack forwards an attack request to the attached AI when target is
// a live combatant.
func (a *Actor) TryToAttack(target world.Tracked) {
	combatant, ok := target.(attackable.Combatant)
	if !ok || a.brain == nil {
		return
	}
	a.brain.TryToAttack(combatant)
}

// TryToFollow forwards a follow request to the attached AI when target is
// a live combatant.
func (a *Actor) TryToFollow(target world.Tracked) {
	combatant, ok := target.(attackable.Combatant)
	if !ok || a.brain == nil {
		return
	}
	a.brain.TryToFollow(combatant)
}

// TryToIdle sends the summon idle the way an effect, an interrupted action
// or the owner's Stop command does. A summon that could not take AI actions
// before the effect in progress (if any) landed keeps its intentions. One
// mid-swing or mid-cast only drops what it had queued: the swing or cast
// ending then goes on as it would with nothing queued. The owner is told
// nothing either way.
func (a *Actor) TryToIdle() {
	if a.aiDeniedBeforeEffect() {
		return
	}
	if a.brain != nil && a.brain.WaitOutIdle() {
		return
	}
	a.goIdle()
}

// goIdle makes the summon idle at once: one that follows its owner goes
// back to following it, and picks the walk up again once it can move.
func (a *Actor) goIdle() {
	follow := a.idleFollow()
	if follow == nil || a.brain == nil {
		a.idle()
		return
	}
	a.setIntent(IntentFollowOwner)
	a.brain.FollowInstead(follow)
}

// idleFollow is who an idle summon follows: its owner while follow is on,
// otherwise nil.
func (a *Actor) idleFollow() attackable.Combatant {
	owner := a.currentOwner()
	if a.followOff.Load() || owner == nil {
		return nil
	}
	return owner
}

// FinishedAttack moves the attached AI on once a swing ends: to the queued
// intention, on with the attack, or idle when the summon cannot keep
// attacking its target.
func (a *Actor) FinishedAttack() {
	if a.brain == nil {
		return
	}
	follow := a.idleFollow()
	if !a.brain.FinishedAttack(follow) {
		return
	}
	if follow != nil {
		a.setIntent(IntentFollowOwner)
	} else {
		a.setIntent(IntentIdle)
	}
}

// Betrayed reports whether this summon has been turned on its owner.
func (a *Actor) Betrayed() bool {
	return a.effects.IsAffected(effect.FlagBetrayed)
}

// FinishedCasting moves the attached AI on once a cast completes: to the
// queued intention, back to the attack the cast replaced, or else idle. The
// AI applies the idle itself, in the same critical section that decides it.
func (a *Actor) FinishedCasting() {
	if a.brain == nil {
		return
	}
	follow := a.idleFollow()
	if a.brain.FinishedCasting(follow) {
		a.noteIdled(follow)
	}
}

// CastStopped moves the attached AI on once a cast is stopped before it
// completes, as FinishedCasting does, and then sends the summon idle the
// way every stop of its cast does (see TryToIdle): a resumed attack whose
// swing already started carries on, anything else ends following the
// owner. A cast stopped by AbortAll is left to AbortAll's caller.
func (a *Actor) CastStopped() {
	if a.brain == nil {
		return
	}
	follow := a.idleFollow()
	idled, handled := a.brain.CastStopped(follow)
	if !handled {
		return
	}
	if idled {
		a.noteIdled(follow)
	}
	a.TryToIdle()
}

// noteIdled records the intent of a summon its AI just sent idle, following
// follow when non-nil.
func (a *Actor) noteIdled(follow attackable.Combatant) {
	if follow != nil {
		a.setIntent(IntentFollowOwner)
	} else {
		a.setIntent(IntentIdle)
	}
}

// Think wakes the attached AI to continue its current intention, as an
// ending sleep, root or paralysis does. The AI logs its own broadcast
// errors, so this never returns one.
func (a *Actor) Think() error {
	if a.brain != nil {
		a.brain.Think()
	}
	return nil
}

// setFollowStatus turns following the owner on or off. On, the summon heads
// back to its owner; off, it goes idle where it stands.
func (a *Actor) setFollowStatus(follow bool) {
	a.followOff.Store(!follow)
	if !follow {
		a.idle()
		return
	}
	a.setIntent(IntentFollowOwner)
	a.TryToFollow(a.currentOwner())
}

// idle cancels the attached AI's current intention without falling back to
// following the owner.
func (a *Actor) idle() {
	a.setIntent(IntentIdle)
	if a.brain != nil {
		a.brain.TryToIdle()
	}
}

// PetInventory returns the pet's inventory, or nil for servitors.
func (a *Actor) PetInventory() *itemcontainer.Inventory {
	if !a.isPet {
		return nil
	}
	return a.petInventory
}

// HeldItemTypeMask returns the item-type bit of the weapon a pet holds in
// its right hand. A servitor, and a pet holding none, hold no weapon; no
// summon holds a shield.
func (a *Actor) HeldItemTypeMask() int32 {
	inv := a.PetInventory()
	if inv == nil {
		return 0
	}
	inst := inv.ItemAt(itemcontainer.RHand)
	if inst == nil {
		return 0
	}
	tmpl, ok := inv.Templates().Get(inst.TemplateID)
	if !ok || tmpl == nil || tmpl.Weapon == nil {
		return 0
	}
	return tmpl.Weapon.Type.Mask()
}

// Fed returns a pet's current meal gauge.
func (a *Actor) Fed() int {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.fed
}

// CanEatFood reports whether itemID matches one of this pet's configured
// food template ids, matching PetTemplate.canEatFood.
func (a *Actor) CanEatFood(itemID int32) bool {
	return itemID == a.food1 || itemID == a.food2
}

// AddFed increases a pet's meal gauge by amount, capped at maxMeal, and
// reports the resulting fed value along with whether the pet is still below
// the auto-feed threshold, matching Pet.checkAutoFeedState after
// PetFoods.useFood's setCurrentFed update.
func (a *Actor) AddFed(amount int) (fed int, stillHungry bool) {
	a.statusMu.Lock()
	a.fed += amount
	if a.fed > a.maxMeal {
		a.fed = a.maxMeal
	}
	a.belowUnsummonLimit = petmodel.BelowShare(a.fed, a.maxMeal, a.unsummonLimit)
	fed = a.fed
	stillHungry = petmodel.BelowShare(fed, a.maxMeal, a.autoFeedLimit)
	a.statusMu.Unlock()
	return fed, stillHungry
}

// Lifetime returns a servitor's current time-remaining/total-lifetime state,
// the servitor analogue of a pet's Fed/maxMeal (Servitor.getTimeRemaining/
// getTotalLifeTime, mirrored by PetInfo.java:26-30's non-Pet branch).
func (a *Actor) Lifetime() LifetimeState {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.lifetime
}

// FollowActive reports whether this actor is following its owner.
func (a *Actor) FollowActive() bool { return !a.followOff.Load() }

// Intent returns the live action this actor is currently pursuing.
func (a *Actor) Intent() Intent {
	a.stateMu.RLock()
	defer a.stateMu.RUnlock()
	return a.intent
}

func (a *Actor) setIntent(intent Intent) {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	a.intent = intent
}

// ApplyCommand resolves and applies an owner-issued control command.
