package event

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CastAborted reports an in-flight cast aborted; Interrupted means the abort
// went through the window-gated interrupt path.
type CastAborted struct{ Interrupted bool }

// CastFinished reports an in-flight cast ending, Interrupted by an abort or
// completed naturally. Broken marks an abort that went through the
// window-gated interrupt path, as CastAborted.Interrupted does. Target is the
// cast's final target when it is a creature, nil otherwise.
type CastFinished struct {
	Interrupted bool
	Broken      bool
	Skill       modelskill.Definition
	Target      attackable.Combatant
}

// CastStopAck reports a cast stop request, whether or not a cast was in
// flight.
type CastStopAck struct{}

// SkillMasteryProc reports a skill-mastery roll succeeding at cast start:
// the cast installs no reuse delay and the caster is told the skill is
// ready to use again.
type SkillMasteryProc struct{}

// AttackFinished reports an attack animation finishing; the actor is free to
// act again. BowReuse marks the finish of a bow's reuse delay rather than of
// a swing.
type AttackFinished struct{ BowReuse bool }

// BowShotFinished reports a player's bow shot ending while the bow's reuse
// delay still runs; AttackFinished with BowReuse follows once the reuse
// ends. A shot with no reuse delay reports that AttackFinished alone. Only
// player attack controllers emit it.
type BowShotFinished struct{}

// AttackRethink reports a hostile NPC's attack reaching a point where its AI
// re-runs desire selection: the hit animation ending or a bow shot landing.
// A finished swing reports AttackFinished instead. Only NPC attack
// controllers emit it.
type AttackRethink struct{}

// ShotsRechargeRequested reports a point where the actor charges its shots
// again from the ones it has set to auto-use: the first hit group of its
// swing reaching the target (Physical), and a skill's cast finalizer
// (Physical for a skill that spends soulshots, Magic for one that spends
// spiritshots), which for a SIGNET_CASTTIME cast also runs at its launch. A player charges its weapon from its own shots, a summon
// from its owner's beast shots. Attack controllers emit it for players and
// summons only. Cast controllers emit it for every caster; an NPC's owner
// drops it, since an NPC's shots recharge from its AI.
type ShotsRechargeRequested struct{ Physical, Magic bool }

// HitDealt reports one of the actor's physical auto-attack hits resolving
// against a target, for the attacking side's damage feedback. Only a player
// or a summon reports it; a summon reports neither a miss nor a hit on its
// own owner. Blocked marks an invulnerable target and Petrified one that is
// also paralyzed.
type HitDealt struct {
	Damage    int
	Crit      bool
	Miss      bool
	Blocked   bool
	Petrified bool
}

// HitLanded reports a physical hit that dealt damage to Target, after the
// damage applied. The receiver runs the chance procs the hit sets off on the
// attacker and on Target. Reflected reports that Target reflected part of
// the damage back on the attacker.
type HitLanded struct {
	Target    attackable.Combatant
	Crit      bool
	Reflected bool
}

// HitDamageApplied reports that one of a player's physical hits has just
// taken its damage off its target, ahead of the damage the target reflects,
// the HP the player absorbs and the procs the hit sets off. It is emitted on
// the player's own queue, so the receiver runs there what the damage left
// owed to the player, such as the side effects of a PK kill the hit made.
// Only player attack controllers emit it.
type HitDamageApplied struct{}

// AttackStanceRequested reports that the actor landed a damaging physical
// hit, or finished an offensive cast whose launch resolved a target: it
// enters its attack stance, or refreshes the one it holds. A summon's stance
// is its owner's.
type AttackStanceRequested struct{}

// Attacked reports that a damaging physical hit or an offensive skill from
// Attacker reached the actor: it enters its attack stance. A summon's stance
// is its owner's.
type Attacked struct{ Attacker attackable.Combatant }

// Evaded reports that the actor evaded a physical auto-attack hit from
// Attacker.
type Evaded struct{ Attacker attackable.Combatant }

// Arrived reports that movement a controller started reached its
// destination.
type Arrived struct{}

// MoveBlocked reports an in-flight move stopped by a newly blocked geodata
// path. The receiver owes observers the stopped-cell correction.
type MoveBlocked struct{}

func (CastAborted) event()            {}
func (CastFinished) event()           {}
func (CastStopAck) event()            {}
func (SkillMasteryProc) event()       {}
func (AttackFinished) event()         {}
func (AttackRethink) event()          {}
func (ShotsRechargeRequested) event() {}
func (BowShotFinished) event()        {}
func (HitDealt) event()               {}
func (HitLanded) event()              {}
func (HitDamageApplied) event()       {}
func (AttackStanceRequested) event()  {}
func (Attacked) event()               {}
func (Evaded) event()                 {}
func (Arrived) event()                {}
func (MoveBlocked) event()            {}
