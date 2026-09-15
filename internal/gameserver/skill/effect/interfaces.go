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
	// FleeFrom starts fleeing from effector and reports whether the actor
	// is able to.
	FleeFrom(effector Actor, distance int) bool
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
	StopCharmOfLuck(*Effect)
	StopPhoenixBlessing(*Effect)
	WeaponGradePenalty() bool
	ReduceDeathPenaltyLevel() int

	CastingNow() bool
	CurrentSkillIsMagic() bool
	InterruptCast()
	StopCast()

	Standing() bool
	SetStanding(bool) bool
	Sit() bool
	StartFakeDeath() bool
	StopFakeDeath() bool
	MarkRecentFakeDeath()
	HPFull() bool

	BroadcastStatus()
	// BroadcastMPStatus pushes a status update that carries MP; only player
	// status broadcasts include it.
	BroadcastMPStatus()
	BroadcastAbnormalEffect()
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
