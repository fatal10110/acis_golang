package effect

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// confusionFake is a minimal Actor + moveStopper + nearbyCombatTarget +
// hateRaiser + attackDesireRaiser double for exercising confusionStart in
// isolation, counting calls instead of reading back threat-table hate (which
// saturates at ThreatTable's maxThreatValue and so cannot distinguish one
// write from a doubled one at math.MaxInt32).
type confusionFake struct {
	world.Presence
	neutralActor
	candidate           attackable.Combatant
	addDamageHateCalls  int
	addAttackDesireHate float64
	addAttackDesireN    int
}

func (f *confusionFake) ObjectID() int32 { return 1 }
func (f *confusionFake) Dead() bool      { return false }
func (f *confusionFake) StopMove()       {}
func (f *confusionFake) RandomNearbyCombatant(radius int) (attackable.Combatant, bool) {
	return f.candidate, f.candidate != nil
}

func (f *confusionFake) AddDamageHate(attacker attackable.Combatant, damage, hate float64) {
	f.addDamageHateCalls++
}

func (f *confusionFake) AddAttackDesire(attacker attackable.Combatant, hate float64) {
	f.addAttackDesireN++
	f.addAttackDesireHate = hate
}

type confusionCandidate struct {
	attackabletest.Combatant
	id int32
}

func (c confusionCandidate) ObjectID() int32  { return c.id }
func (c confusionCandidate) SiegeGuard() bool { return false }
func (c confusionCandidate) AlikeDead() bool  { return false }

// TestConfusionStartDoesNotDoubleCountHate pins #2340's review finding:
// AddAttackDesire now feeds the threat table itself, so confusionStart's
// separate AddDamageHate call — a workaround for the pre-fix gap, like the
// one removed from queuePartyAttack — would double the hate. Java's
// EffectConfusion.onStart (EffectConfusion.java:49-50) makes a single
// addAttackDesire(target, Integer.MAX_VALUE) call with no separate
// addDamageHate.
func TestConfusionStartDoesNotDoubleCountHate(t *testing.T) {
	target := &confusionFake{candidate: confusionCandidate{id: 2}}
	e := &Effect{Effected: target}

	if !confusionStart(e) {
		t.Fatal("confusionStart() = false, want true")
	}
	if target.addDamageHateCalls != 0 {
		t.Fatalf("AddDamageHate calls = %d, want 0 (Java makes no separate call)", target.addDamageHateCalls)
	}
	if target.addAttackDesireN != 1 {
		t.Fatalf("AddAttackDesire calls = %d, want 1", target.addAttackDesireN)
	}
	if target.addAttackDesireHate != math.MaxInt32 {
		t.Fatalf("AddAttackDesire hate = %v, want %v", target.addAttackDesireHate, math.MaxInt32)
	}
}

func (confusionCandidate) Kind() actor.Kind { return actor.KindNPC }

func (*confusionFake) Kind() actor.Kind { return actor.KindNPC }

func (confusionCandidate) Heading() int { return 0 }

func (confusionCandidate) Position() (x, y, z int) { return 0, 0, 0 }

func (f *confusionFake) MonsterKind() bool                                    { return false }
func (f *confusionFake) RandomNearbyMonster(int) (attackable.Combatant, bool) { return nil, false }
func (f *confusionFake) RandomizeHate() bool                                  { return false }
func (f *confusionFake) StopMostHatedTarget()                                 {}
func (f *confusionFake) Think() error                                         { return nil }
func (f *confusionFake) SpoilPool() *item.SpoilPool                           { return nil }
func (f *confusionFake) CollisionRadius() float64                             { return 0 }
func (f *confusionFake) SetCollisionRadius(float64)                           {}
func (f *confusionFake) ResetCollisionRadius()                                {}

var _ NPCActor = (*confusionFake)(nil)
