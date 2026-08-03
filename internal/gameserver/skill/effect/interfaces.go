package effect

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

type dotTarget interface {
	Dead() bool
	HP() float64
	ReduceHPByDOT(damage float64, effector any, isDOT bool)
}

// statusBroadcaster is implemented by an actor that can push its current
// vitals to the clients watching it.
type statusBroadcaster interface {
	BroadcastStatus()
}

// mpStatusBroadcaster is implemented only by actors whose status broadcast
// includes MP, matching PlayerStatus.broadcastStatusUpdate()'s unconditional
// CUR_MP inclusion (CreatureStatus.java's Player override) — the generic
// Creature/Npc path sends HP only, so no NPC-side actor implements this.
type mpStatusBroadcaster interface {
	BroadcastMPStatus()
}

type regenMaxSender interface {
	SendRegenMax(count, period int32, hpRegen float64)
}

type lackHPNotifier interface {
	NotifyEffectRemovedDueLackHP(*Effect)
}

type mpDotTarget interface {
	Dead() bool
	MPValue() float64
	ReduceMP(amount float64) float64
}

type lackMPNotifier interface {
	NotifyEffectRemovedDueLackMP(*Effect)
}

// recentFakeDeathMarker is implemented by an actor that tracks its own
// stand-up grace period after Fake Death exits; an actor without one gets
// no grace-period tracking (fake death still applies while the effect is
// active regardless).
type recentFakeDeathMarker interface {
	MarkRecentFakeDeath()
}

type aborter interface {
	AbortAll(force bool)
}

type idleTarget interface {
	TryToIdle()
}

type moveStopper interface {
	StopMove()
}

type abnormalUpdater interface {
	UpdateAbnormalEffect()
}

type thinkTarget interface {
	Think()
}

type afraidTarget interface {
	Afraid() bool
}

type fearImmuneTarget interface {
	FearImmune() bool
}

type playableTarget interface {
	Playable() bool
}

type fleeTarget interface {
	FleeFrom(effector any, distance int)
}

type effectStopper interface {
	StopEffects(Type)
}

// raidTarget optionally reports whether a target is a raid boss or minion;
// a target without one is treated as not raid-related.
type raidTarget interface {
	RaidRelated() bool
}

// castInterrupter is implemented by an actor whose in-progress cast can be
// checked and forcibly interrupted.
type castInterrupter interface {
	CastingNow() bool
	InterruptCast()
}

// magicCastTarget is implemented by an actor whose in-progress cast can be
// checked for its magic/physical nature and stopped.
type magicCastTarget interface {
	CastingNow() bool
	CurrentSkillIsMagic() bool
	StopCast()
}

// castStopper is implemented by an actor whose in-progress cast can be
// unconditionally stopped.
type castStopper interface {
	StopCast()
}

// targetClearer is implemented by an actor that can drop its current
// target and abandon any attack in progress against it.
type targetClearer interface {
	ClearTarget()
	StopAttack()
}

// invulnerabilityTarget is implemented by an actor whose damage
// invulnerability can be toggled.
type invulnerabilityTarget interface {
	SetInvul(bool)
}

// immobilizeTarget is implemented by an actor whose movement-lock flag can
// be toggled. SetImmobilized reports whether the flag actually changed;
// this hook ignores that report.
type immobilizeTarget interface {
	SetImmobilized(bool) bool
}

// manaHealTarget is implemented by an actor whose mana pool can be checked
// for healing eligibility and restored. AddMP reports the amount actually
// applied (e.g. clamped to the actor's max MP).
type manaHealTarget interface {
	CanBeHealed() bool
	AddMP(amount float64) float64
}

// instantHealTarget is implemented by an actor whose HP can be checked for
// healing eligibility and restored. AddHP reports the amount actually
// applied (e.g. clamped to the actor's max HP).
type instantHealTarget interface {
	CanBeHealed() bool
	AddHP(amount float64) float64
}

// healProficiencyTarget optionally reports the additive bonus a heal
// effect's base value is boosted by before the percentage from
// healEffectivenessTarget is applied; absent, it defaults to 0.
type healProficiencyTarget interface {
	HealProficiency() float64
}

// healEffectivenessTarget optionally reports the percentage a heal amount
// is scaled by; absent, it defaults to 100.
type healEffectivenessTarget interface {
	HealEffectiveness() float64
}

// rechargeRateTarget optionally adjusts a base MP-restore amount by the
// actor's recharge rate before it is applied; absent, the base amount is
// used unadjusted.
type rechargeRateTarget interface {
	RechargeMP(base float64) float64
}

// targetRedirectTarget is implemented by an actor whose current target can
// be read or replaced, or turned into an attack.
type targetRedirectTarget interface {
	CurrentTarget() any
	SetTarget(any)
	TryToAttack(any)
}

// headingTarget is implemented by an actor whose facing can be read or set.
type headingTarget interface {
	Heading() int
	SetHeading(int)
}

// bluffExemptTarget optionally reports whether an actor ignores a
// facing-redirect effect (some non-combatant and event-specific actors do);
// absent, the actor is not exempt.
type bluffExemptTarget interface {
	BluffExempt() bool
}

// playerTarget optionally reports whether an actor is specifically a
// player, as opposed to any other playable (pet, summon, ...); absent, the
// actor is treated as not a player.
type playerTarget interface {
	IsPlayer() bool
}

// charmOfLuckStopper is implemented by an actor that reacts to its Charm of
// Luck buff ending.
type charmOfLuckStopper interface {
	StopCharmOfLuck(*Effect)
}

// phoenixBlessStopper is implemented by an actor that reacts to its Phoenix
// Blessing buff ending.
type phoenixBlessStopper interface {
	StopPhoenixBlessing(*Effect)
}

// skillIDEffectStopper is implemented by an actor whose active effects can
// be stopped by owning skill id.
type skillIDEffectStopper interface {
	StopSkillEffectsByID(id modelskill.ID)
}

// deadChecker reports whether an actor is dead, consulted by a cancel-family
// effect before it strips anything.
type deadChecker interface {
	Dead() bool
}

// effectListOwner is implemented by an actor whose active effect list can be
// inspected and stripped directly, for cancel- and negate-family effects
// that act on other effects rather than the actor's stats.
type effectListOwner interface {
	EffectList() *List
}

// cancelVulnerabilitySource optionally supplies an actor's already-resolved
// vulnerability multiplier for a classification tag; an actor without one is
// treated as unmodified (1.0).
type cancelVulnerabilitySource interface {
	CancelVulnerability(classification string) float64
}

// objectIDTarget optionally reports an actor's world object id.
type objectIDTarget interface {
	ObjectID() int32
}

// levelTarget optionally reports an actor's level.
type levelTarget interface {
	Level() int
}

// spoilCaster is implemented by an actor whose identity and level a spoil
// effect's magic-resist roll needs.
type spoilCaster interface {
	objectIDTarget
	levelTarget
}

// spoilTarget is implemented by a monster-kind actor whose spoil pool can
// be checked and marked, mirroring the SPOIL skill-type handler's own
// target surface for a caster-applied spoil effect.
type spoilTarget interface {
	deadChecker
	levelTarget
	SpoilPool() *item.SpoilPool
}

// weaponGradePenalized optionally reports whether the caster's equipped
// weapon grade is insufficient for the skill being cast (a flat magic-
// resist penalty); a caster without one is treated as unpenalized.
type weaponGradePenalized interface {
	WeaponGradePenalty() bool
}

// standingTarget optionally reports whether an actor is currently standing
// rather than sitting; an actor without one is treated as not standing, so
// a rest-family effect's seated-check never rejects it.
type standingTarget interface {
	Standing() bool
}

// sitTarget is implemented by an actor whose sit/stand state can be set.
type sitTarget interface {
	SetStanding(bool) bool
}

type stanceTarget interface {
	Sit() bool
}

type fakeDeathStanceTarget interface {
	StartFakeDeath() bool
	StopFakeDeath() bool
}

// hpFullTarget optionally reports whether an actor's HP is already at (or
// within the reference effect's own +1 rounding tolerance of) its maximum,
// consulted by Relax before it drains MP on a tick; an actor without one is
// treated as never full.
type hpFullTarget interface {
	HPFull() bool
}

// summonOwnerTarget is implemented by a summon actor whose owning player's
// object id can be read, to confirm an effector is that summon's own
// owner.
type summonOwnerTarget interface {
	OwnerID() int32
}

// hateRaiser is implemented by an actor whose physical-attack threat can be
// raised against another combatant.
type hateRaiser interface {
	AddDamageHate(attacker attackable.Combatant, damage, hate float64)
}

// monsterKindTarget optionally reports whether an actor is specifically a
// Monster-family hostile NPC (as opposed to a guard, siege guard, or
// friendly monster); an actor without one is treated as not Monster-family.
type monsterKindTarget interface {
	MonsterKind() bool
}

// nearbyMonsterFinder is implemented by an actor that can pick a random
// other Monster-family actor within radius units of itself, for an effect
// that redirects one monster's aggression onto another; an actor without
// one finds no candidate.
type nearbyMonsterFinder interface {
	RandomNearbyMonster(radius int) (attackable.Combatant, bool)
}

// nearbyCombatTarget is implemented by an actor that can pick a random
// other attackable actor within radius units of itself, for an effect that
// redirects its own aggression onto a random bystander; an actor without
// one finds no candidate.
type nearbyCombatTarget interface {
	RandomNearbyCombatant(radius int) (attackable.Combatant, bool)
}

// hateRandomizer is implemented by an actor whose physical threat table can
// swap a random valid attacker into the most-hated slot ahead of its
// current top-hate attacker.
type hateRandomizer interface {
	RandomizeHate() bool
}

// mostHatedResetter is implemented by an actor whose physical threat table
// can have its current top target's hate cleared, used to drop a confusion
// effect's forced redirect once the effect ends.
type mostHatedResetter interface {
	StopMostHatedTarget()
}

// summonOwnerCombatant is implemented by a summon that can expose its
// owning player as an AI target.
type summonOwnerCombatant interface {
	OwnerCombatant() attackable.Combatant
}

// summonOwnerAttacker is implemented by a summon that can redirect its AI
// toward and away from its owner.
type summonOwnerAttacker interface {
	TryToAttack(any)
	TryToFollow(any)
}

type chargesTarget interface {
	IncreaseCharges(count, max int) bool
}

// increaseChargesStart adds the template's charge amount, capped at the
// template's count (repurposed here as the max-charges cap, not a tick
// count), matching the reference's one-shot onStart call into
// Player.increaseCharges. It always reports success: whether the target
// was already at the cap is the target method's own no-op/system-message
// concern, not this effect's.

type chanceTriggerTarget interface {
	AddChanceTrigger(e *Effect)
	RemoveChanceTrigger(e *Effect)
}

type growTarget interface {
	CollisionRadius() float64
	SetCollisionRadius(radius float64)
	ResetCollisionRadius()
}

// growRadiusScale is EffectGrow.onStart()'s collision-radius multiplier.
const growRadiusScale = 1.19

// growStart ports EffectGrow.onStart(): rejects a target that isn't
// Npc-shaped (a player target never grows), otherwise scales its collision
// radius by growRadiusScale and refreshes its visible state.

type recoveryTarget interface {
	ReduceDeathPenaltyLevel() int
}

// recoveryStart lowers the target's death-penalty debuff level by one,
// rejecting a target that isn't player-shaped. Reapplying the debuff skill
// at its new level and refreshing the client's status window are the live
// character's own concern, not this effect's.

type flightPosition interface {
	X() int
	Y() int
	Z() int
}

// flightResolver corrects a knockback's raw destination against geodata,
// the same terrain-walk primitive ordinary movement resolution uses.
type flightResolver interface {
	ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location
}

// flightMover carries out a knockback's client-visible flight and its
// server-authoritative landing. SetXYZ and BroadcastPosition reuse the
// jumpCaster naming from handler/skill/teleport.go so one actor
// implementation satisfies both.
type flightMover interface {
	FlyTo(dest location.Location, flight modelskill.Flight)
	SetXYZ(x, y, z int)
	BroadcastPosition()
}

// throwUpStart computes a knockback's landing point and starts the
// client-visible flight. The target is always aborted first, even when the
// distance gate below rejects the effect outright — that ordering matches
// the reference behavior, where the abort is unconditional and the range
// check only guards whether the flight itself happens.
//
// The destination pivots on the effector's position, not the effected's:
// the target is pushed further along the effector-to-effected line. Z is
// left at the effected's current height even after the X/Y geo correction
// below — the reference implementation never corrects Z for this effect,
// a known approximation preserved here rather than fixed.
