package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (a *Actor) ObjectID() int32 { return a.id }

// Move returns this summon's lifetime movement state, mirroring
// creature.Live.Move (internal/gameserver/model/actor/creature/live.go):
// the same *move.CreatureMove a wired move.Controller drives, so
// Move().Moving() reflects real in-motion state once InitMovement has run.
func (a *Actor) Move() *move.CreatureMove { return &a.movement }

// InitMovement wires real geodata/speed into this summon's movement state,
// matching creature.NewLive's Init call. Call it once, before building a
// move.Controller over Move() (see GameClientLink.wireSummonAI); a summon
// left uninitialized (no geodata available) keeps a stationary, never-moving
// zero-value CreatureMove.
func (a *Actor) InitMovement(origin location.Location, speed float64, geo move.Geo) error {
	return a.movement.Init(origin, speed, geo)
}

// ActingPlayer returns the owner for player-attributed outcomes.
func (a *Actor) ActingPlayer() creature.DeathActor {
	owner, _ := a.owner.(creature.DeathActor)
	return owner
}

// OwnerID returns the owning player's world object id.
func (a *Actor) OwnerID() int32 {
	if a.owner == nil {
		return 0
	}
	return a.owner.ObjectID()
}

// ControlItemID returns the collar item object id backing this pet.
func (a *Actor) ControlItemID() int32 { return a.controlItemID }

// Level returns the summon's current level.
func (a *Actor) Level() int {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.level
}

// SetStatusUpdater records the runtime hook that publishes this summon's
// changed status to connected clients.
func (a *Actor) SetStatusUpdater(update func()) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.statusUpdater = update
}

// UpdateStatus publishes this summon's current status through its runtime hook.
func (a *Actor) UpdateStatus() {
	a.statusMu.RLock()
	update := a.statusUpdater
	a.statusMu.RUnlock()
	if update != nil {
		update()
	}
}

// SetDamageNotifier records the runtime hook for owner-facing direct damage feedback.
func (a *Actor) SetDamageNotifier(notify func(string, int32)) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.damageNotifier = notify
}

// SetExpNotifier records the owner-facing pet experience feedback hook.
func (a *Actor) SetExpNotifier(notify func(int64)) {
	a.statusMu.Lock()
	defer a.statusMu.Unlock()
	a.expNotifier = notify
}

func (a *Actor) notifyDamage(attacker any, amount float64) {
	named, ok := attacker.(interface{ CharacterName() string })
	if !ok {
		return
	}
	a.statusMu.RLock()
	notify := a.damageNotifier
	a.statusMu.RUnlock()
	if notify != nil {
		notify(named.CharacterName(), int32(amount))
	}
}

// IsPet reports whether this live summon is a pet rather than a servitor.
func (a *Actor) IsPet() bool { return a.isPet }

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
	if a == nil || !a.isPet || a.Dead() || a.owner == nil {
		return false
	}
	a.statusMu.RLock()
	max, ok := int64(0), false
	if a.growth != nil {
		if row, found := a.growth.Levels[81]; found {
			max, ok = row.MaxExp, true
		}
	}
	exp := a.exp
	a.statusMu.RUnlock()
	if !ok || exp > max+10_000 {
		return false
	}
	ax, ay, az := a.Position()
	ox, oy, oz := a.owner.Position()
	return location.In3DRange(ax, ay, az, ox, oy, oz, partyRange)
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

// AddExpAndSp grants a pet its raw kill-reward share. Experience uses the
// pet-specific configured rate; SP is deliberately unscaled.
func (a *Actor) AddExpAndSp(rawExp int64, sp int) {
	if a == nil || !a.isPet {
		return
	}
	expGain := a.ScaledExpGain(rawExp)
	a.statusMu.Lock()
	a.exp += expGain
	a.sp += sp
	leveled := a.refreshGrowthLocked()
	a.statusMu.Unlock()
	if leveled {
		a.resetVitals()
	}
	a.UpdateStatus()
	a.statusMu.RLock()
	notify := a.expNotifier
	a.statusMu.RUnlock()
	if notify != nil {
		notify(expGain)
	}
}

func (a *Actor) refreshGrowthLocked() bool {
	if a.growth == nil {
		return false
	}
	oldLevel := a.level
	for {
		next, ok := a.growth.Levels[a.level+1]
		if !ok || a.exp < next.MaxExp {
			break
		}
		a.level++
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

func (a *Actor) resetVitals() {
	hp, mp := a.MaxHPValue(), a.MaxMPValue()
	a.vitals.mu.Lock()
	a.vitals.hp, a.vitals.mp = hp, mp
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
func (a *Actor) SummonOwner() Owner { return a.owner }

// UnSummon despawns this summon as an owner-directed removal, matching
// Java's Summon.unSummon(Player owner) (this actor already knows its own
// owner, so the parameter only satisfies that contract).
func (a *Actor) UnSummon(Owner) { a.Unsummon() }

// DenyAIAction reports whether this summon cannot act now.
func (a *Actor) DenyAIAction() bool {
	return a.AlikeDead() || a.Paralyzed() || a.Teleporting() || a.effects.IsAffected(
		effect.FlagStunned|effect.FlagMeditating|effect.FlagSleep|effect.FlagFear,
	)
}

// Paralyzed reports whether this summon is temporarily paralyzed or carries
// an active paralyze effect.
func (a *Actor) Paralyzed() bool {
	a.stateMu.RLock()
	paralyzed := a.paralyzed
	a.stateMu.RUnlock()
	return paralyzed || a.effects.IsAffected(effect.FlagParalyzed)
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
func (a *Actor) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(a, tracked)
}

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
	if a.owner != nil {
		ownerLevel = a.owner.LevelValue()
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
// true even if the AI goes on to reject it.
func (a *Actor) TryUseSkill(skillID int, target attackable.Combatant) bool {
	ref, ok := a.GetSkill(skillID)
	if !ok || !a.CanUseSkill() || a.brain == nil {
		return false
	}
	a.brain.TryToCast(target, ref)
	return true
}

// OutOfControl reports whether the owner cannot currently command this
// summon, matching Summon.isOutOfControl (Summon.java:296-298):
// super.isOutOfControl() || isBetrayed().
func (a *Actor) OutOfControl() bool {
	return a.disabled || a.effects.IsAffected(effect.FlagBetrayed)
}

// OwnerCombatant returns the owning player when it can be targeted by AI.
func (a *Actor) OwnerCombatant() attackable.Combatant {
	owner, _ := a.owner.(attackable.Combatant)
	return owner
}

// SetAI attaches the summon intention loop used by accepted commands.
func (a *Actor) SetAI(brain AI) {
	a.brain = brain
}

// SetOnDespawn records runtime cleanup that must run exactly when this summon
// leaves world state. It is configured before the summon is published.
func (a *Actor) SetOnDespawn(f func()) { a.onDespawn = f }

// CurrentTarget returns the summon target selected by its current command.
func (a *Actor) CurrentTarget() worldobject.Object { return a.target }

// SetTarget updates the summon target without issuing an owner-visible packet.
func (a *Actor) SetTarget(target worldobject.Object) {
	if target == nil {
		a.target = nil
		return
	}
	tracked, ok := target.(world.Tracked)
	if ok {
		a.target = tracked
	}
}

// AttackTarget forwards an aggression-triggered attack to the summon AI.
func (a *Actor) AttackTarget(target worldobject.Object) { a.TryToAttack(target) }

// TryToAttack forwards an attack request to the attached AI when target is
// a live combatant.
func (a *Actor) TryToAttack(target worldobject.Object) {
	combatant, ok := target.(attackable.Combatant)
	if !ok || a.brain == nil {
		return
	}
	a.brain.TryToAttack(combatant)
}

// TryToFollow forwards a follow request to the attached AI when target is
// a live combatant.
func (a *Actor) TryToFollow(target worldobject.Object) {
	combatant, ok := target.(attackable.Combatant)
	if !ok || a.brain == nil {
		return
	}
	a.brain.TryToFollow(combatant)
}

// TryToIdle cancels the attached AI's current intention.
func (a *Actor) TryToIdle() {
	a.intent = IntentIdle
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

// Fed returns a pet's current meal gauge.
func (a *Actor) Fed() int {
	a.statusMu.RLock()
	defer a.statusMu.RUnlock()
	return a.fed
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
func (a *Actor) FollowActive() bool { return a.followActive }

// Intent returns the live action this actor is currently pursuing.
func (a *Actor) Intent() Intent { return a.intent }

// ApplyCommand resolves and applies an owner-issued control command.
