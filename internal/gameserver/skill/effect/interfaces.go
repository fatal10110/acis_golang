package effect

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Actor is every effect participant: Effect.Effector and Effect.Effected.
// Players, NPCs, summons and signet effect points implement every method; a
// method that does not apply to a kind returns the neutral value documented
// at its implementation. Kind-specific surfaces live on PlayerActor, NPCActor
// and SummonActor.
type Actor interface {
	world.Tracked
	Dead() bool
	Level() int
	CharacterName() string
	Position() (x, y, z int)
	Heading() int
	SetHeading(int)
	RaidRelated() bool

	EffectList() *List
	// CancelVulnerability is the resolved vulnerability multiplier for a
	// cancel classification tag; 1 when unmodified.
	CancelVulnerability(classification string) float64
	// UpdateAbnormalEffect re-announces the actor's appearance (its
	// abnormal visual bits among them) to itself and its observers.
	UpdateAbnormalEffect()
	StartAbnormalEffect(mask int)
	StopAbnormalEffect(mask int)
	StopEffects(Type)
	StopSkillEffectsByID(id modelskill.ID)
	AddChanceTrigger(e *Effect)
	RemoveChanceTrigger(e *Effect)

	HP() float64
	ReduceHPByDOT(damage float64, effector Actor, isDOT bool)
	MPValue() float64
	// ReduceMP reports the amount actually removed.
	ReduceMP(amount float64) float64
	CanBeHealed() bool
	// AddHP and AddMP report the amount actually applied.
	AddHP(amount float64) float64
	AddMP(amount float64) float64
	// HealProficiency is the additive heal-power bonus; 0 when none.
	HealProficiency() float64
	// HealEffectiveness is the percentage a heal is scaled by; 100 when
	// unmodified.
	HealEffectiveness() float64
	// RechargeMP adjusts a base MP-restore amount by the recharge rate.
	RechargeMP(base float64) float64
	// BroadcastStatus republishes the actor's current vitals to whoever
	// follows them: a player's own bars, a summon's owner pet window and
	// observers, an NPC's targeters. A summon's and an NPC's AddHP, AddMP and
	// ReduceMP already do this themselves; a player's do not.
	BroadcastStatus()

	AbortAll(force bool)
	StopMove()
	TryToIdle()
	ClearTarget()
	StopAttack()
	// SetImmobilized reports whether the movement lock actually changed.
	SetImmobilized(bool) bool
	// SetInvul reports whether the invulnerability flag actually changed.
	SetInvul(bool) bool
	Afraid() bool
	FearImmune() bool
	// FleeFrom runs the actor distance units directly away from effector,
	// as far as the actor's own movement rules let it. A nil or self
	// effector, or a distance under 10, does nothing.
	FleeFrom(effector Actor, distance int)
	// BluffExempt reports whether the actor ignores facing-redirect effects.
	BluffExempt() bool

	// ValidLocation corrects a knockback destination against geodata.
	ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location
	FlyTo(dest location.Location, flight modelskill.Flight)
	SetXYZ(x, y, z int)
	BroadcastPosition()
}

// PlayerActor is the player-only effect surface.
type PlayerActor interface {
	Actor

	IncreaseCharges(count, max int) bool
	CurrentTarget() world.Tracked
	SetTarget(world.Tracked)
	TryToAttack(world.Tracked)
	// WakeAI re-evaluates the player's current intention once.
	WakeAI()
	// StopCharmOfLuck, StopPhoenixBlessing and StopProtectionBlessing run
	// when that blessing ends on the player; observers see its appearance
	// refreshed.
	StopCharmOfLuck(*Effect)
	StopPhoenixBlessing(*Effect)
	StopProtectionBlessing(*Effect)
	// BroadcastEtcStatus sends the player's status-window flags to the
	// player and its observers.
	BroadcastEtcStatus()
	WeaponGradePenalty() bool
	ReduceDeathPenaltyLevel() int

	CasterActor

	Standing() bool
	SetStanding(bool) bool
	Sit() bool
	StartFakeDeath() bool
	StopFakeDeath() bool
	MarkRecentFakeDeath()
	HPFull() bool

	SendRegenMax(count, period int32, hpRegen float64)
	NotifyEffectRemovedDueLackHP(*Effect)
	NotifyEffectRemovedDueLackMP(*Effect)
	NotifyRelaxDeactivatedHPFull(*Effect)
	NotifyHPRestored(healerName string, amount int, byOther bool)
	NotifyMPRestored(healerName string, amount int, byOther bool)
	NotifySpoilAlready()
	NotifySpoilSuccess()
}

// NPCActor is the NPC-only effect surface: hate, aggro redirection, spoil
// state and runtime collision size.
type NPCActor interface {
	Actor

	AddDamageHate(attacker attackable.Combatant, damage, hate float64)
	AddAttackDesire(attacker attackable.Combatant, hate float64)
	MonsterKind() bool
	RandomNearbyMonster(radius int) (attackable.Combatant, bool)
	RandomNearbyCombatant(radius int) (attackable.Combatant, bool)
	RandomizeHate() bool
	StopMostHatedTarget()
	Think() error
	SpoilPool() *item.SpoilPool
	CollisionRadius() float64
	SetCollisionRadius(radius float64)
	ResetCollisionRadius()
}

// SummonActor is the summon-only effect surface.
type SummonActor interface {
	Actor

	OwnerID() int32
	// OwnerObject returns the controlling player's world object.
	OwnerObject() (world.Tracked, bool)
	TryToAttack(world.Tracked)
	TryToFollow(world.Tracked)
	// RandomConfusionTarget returns a random creature within radius a
	// confused summon may turn on.
	RandomConfusionTarget(radius int) (world.Tracked, bool)
	// Think wakes the summon's AI to continue its current intention.
	Think() error
	// StopCharmOfLuck and StopPhoenixBlessing run when that blessing ends
	// on the summon; observers see its appearance refreshed.
	StopCharmOfLuck(*Effect)
	StopPhoenixBlessing(*Effect)
	// StopProtectionBlessing runs when a Blessing of Protection loses its
	// stack group's head on the summon; observers see its appearance
	// refreshed.
	StopProtectionBlessing(*Effect)
}

// CasterActor is the cast surface of every kind that casts: players, NPCs
// and summons.
type CasterActor interface {
	CastingNow() bool
	// CurrentSkillIsMagic reports whether the cast in flight is a magic
	// skill; false when nothing is being cast.
	CurrentSkillIsMagic() bool
	// InterruptCast aborts the cast only while it is still inside its
	// interrupt window; the caster's own client (a summon's owner) reads
	// CASTING_INTERRUPTED.
	InterruptCast()
	// StopCast aborts the cast unconditionally.
	StopCast()
}

func asCaster(a Actor) (CasterActor, bool) {
	c, ok := a.(CasterActor)
	return c, ok
}

func asPlayer(a Actor) (PlayerActor, bool) {
	p, ok := a.(PlayerActor)
	return p, ok
}

func asNPC(a Actor) (NPCActor, bool) {
	n, ok := a.(NPCActor)
	return n, ok
}

func asSummon(a Actor) (SummonActor, bool) {
	s, ok := a.(SummonActor)
	return s, ok
}

// growRadiusScale is EffectGrow.onStart()'s collision-radius multiplier.
const growRadiusScale = 1.19
