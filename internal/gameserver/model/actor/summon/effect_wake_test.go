package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// thinkCountingAI is a summon AI that only counts Think calls.
type thinkCountingAI struct{ thinks int }

func (a *thinkCountingAI) Think()                                                  { a.thinks++ }
func (*thinkCountingAI) TryToAttack(attackable.Combatant) bool                     { return false }
func (*thinkCountingAI) TryToFollow(attackable.Combatant) bool                     { return false }
func (*thinkCountingAI) TryToIdle()                                                {}
func (*thinkCountingAI) WaitOutIdle() bool                                         { return false }
func (*thinkCountingAI) FinishedAttack(attackable.Combatant) bool                  { return false }
func (*thinkCountingAI) FinishedCasting(attackable.Combatant) bool                 { return false }
func (*thinkCountingAI) CastStopped(attackable.Combatant) (bool, bool)             { return false, false }
func (*thinkCountingAI) TryToCast(attackable.Combatant, modelskill.Ref, bool) bool { return false }
func (*thinkCountingAI) AbortAll()                                                 {}
func (*thinkCountingAI) FollowInstead(attackable.Combatant)                        {}
func (*thinkCountingAI) TryToMoveTo(location.Location) bool                        { return false }
func (*thinkCountingAI) StepAside(location.Location) bool                          { return false }
func (*thinkCountingAI) StopMove()                                                 {}
func (*thinkCountingAI) StopAttack()                                               {}
func (*thinkCountingAI) AttackingNow() bool                                        { return false }

// TestSummonCrowdControlExitWakesAI drives Root, Sleep and Paralyze through
// the summon's real effect list: EffectRoot/EffectSleep/EffectParalyze.onExit
// notify THINK for every non-Player effected, so each removal must wake the
// summon AI exactly once, and applying the effect must not.
func TestSummonCrowdControlExitWakesAI(t *testing.T) {
	for _, name := range []string{"Root", "Sleep", "Paralyze"} {
		t.Run(name, func(t *testing.T) {
			brain := &thinkCountingAI{}
			servitor := mustServitor(t, ServitorConfig{ObjectID: 1})
			servitor.Attach(Runtime{AI: brain})

			e, err := effect.New(effect.Skill{ID: 1201, Level: 1, SkillType: "DEBUFF"}, modelskill.EffectTemplate{Name: name, Count: 1, Time: 30})
			if err != nil {
				t.Fatalf("effect.New(%s) error = %v", name, err)
			}
			e.Effector = servitor
			e.Effected = servitor

			servitor.EffectList().Add(e)
			if !e.InUse() {
				t.Fatalf("%s did not start on the summon", name)
			}
			if brain.thinks != 0 {
				t.Fatalf("Think() calls after applying %s = %d, want 0", name, brain.thinks)
			}

			servitor.EffectList().Remove(e)
			if brain.thinks != 1 {
				t.Fatalf("Think() calls after %s ended = %d, want 1", name, brain.thinks)
			}
		})
	}
}

// TestSummonThinkWithoutAIIsNoop covers a summon whose AI was never attached.
func TestSummonThinkWithoutAIIsNoop(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1})
	if err := servitor.Think(); err != nil {
		t.Fatalf("Think() error = %v, want nil", err)
	}
}
