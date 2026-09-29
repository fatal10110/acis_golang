package event

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CastAborted reports an in-flight cast aborted; Interrupted means the abort
// went through the window-gated interrupt path.
type CastAborted struct{ Interrupted bool }

// CastFinished reports an in-flight cast ending, Interrupted by an abort or
// completed naturally.
type CastFinished struct {
	Interrupted bool
	Skill       modelskill.Definition
}

// CastStopAck reports a cast stop request, whether or not a cast was in
// flight.
type CastStopAck struct{}

// SkillMasteryProc reports a skill-mastery roll succeeding at cast start:
// the cast installs no reuse delay and the caster is told the skill is
// ready to use again.
type SkillMasteryProc struct{}

// AttackStarted reports an attack animation starting, before its hits are
// scheduled.
type AttackStarted struct{}

// AttackFinished reports an attack animation finishing; the actor is free to
// act again. BowReuse marks the finish of a bow's reuse delay rather than of
// a swing.
type AttackFinished struct{ BowReuse bool }

// AttackRethink reports a hostile NPC's attack reaching a point where its AI
// re-runs desire selection: the hit animation ending or a bow shot landing.
// A finished swing reports AttackFinished instead. Only NPC attack
// controllers emit it.
type AttackRethink struct{}

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
// attacker and on Target.
type HitLanded struct {
	Target attackable.Combatant
	Crit   bool
}

// Arrived reports that movement a controller started reached its
// destination.
type Arrived struct{}

// MoveBlocked reports an in-flight move stopped by a newly blocked geodata
// path. The receiver owes observers the stopped-cell correction.
type MoveBlocked struct{}

func (CastAborted) event()      {}
func (CastFinished) event()     {}
func (CastStopAck) event()      {}
func (SkillMasteryProc) event() {}
func (AttackStarted) event()    {}
func (AttackFinished) event()   {}
func (AttackRethink) event()    {}
func (HitDealt) event()         {}
func (HitLanded) event()        {}
func (Arrived) event()          {}
func (MoveBlocked) event()      {}
