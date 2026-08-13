package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	petmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func (a *Actor) ObjectID() int32 { return a.id }

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

// DenyAIAction reports whether this summon is unable to act on an owner
// command: dead or out of the owner's control.
func (a *Actor) DenyAIAction() bool { return a.AlikeDead() || a.OutOfControl() }

// Knows reports whether target is currently visible to this summon.
func (a *Actor) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(a, tracked)
}

// PhysicalAttackRange returns this summon's melee attack range.
func (a *Actor) PhysicalAttackRange() int { return a.stats.AttackRange }

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
// its accept/reject decision (busy, cooldown, out of control, MP/mute —
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
// summon.
func (a *Actor) OutOfControl() bool { return a.disabled }

// OwnerCombatant returns the owning player when it can be targeted by AI.
func (a *Actor) OwnerCombatant() attackable.Combatant {
	owner, _ := a.owner.(attackable.Combatant)
	return owner
}

// SetAI attaches the summon intention loop used by accepted commands.
func (a *Actor) SetAI(brain AI) {
	a.brain = brain
}

// CurrentTarget returns the summon target selected by its current command.
func (a *Actor) CurrentTarget() any { return a.target }

// SetTarget updates the summon target without issuing an owner-visible packet.
func (a *Actor) SetTarget(target any) {
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
func (a *Actor) AttackTarget(target any) { a.TryToAttack(target) }

// TryToAttack forwards an attack request to the attached AI when target is
// a live combatant.
func (a *Actor) TryToAttack(target any) {
	combatant, ok := target.(attackable.Combatant)
	if !ok || a.brain == nil {
		return
	}
	a.brain.TryToAttack(combatant)
}

// TryToFollow forwards a follow request to the attached AI when target is
// a live combatant.
func (a *Actor) TryToFollow(target any) {
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
