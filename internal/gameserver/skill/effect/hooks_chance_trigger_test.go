package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestNewChanceSkillTriggerRejectsUnknownTriggerType(t *testing.T) {
	if _, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", ChanceType: "BOGUS", ActivationChance: 50}); err == nil {
		t.Fatal("New() error = nil, want an error for an unknown chanceType")
	}
}

func TestNewChanceSkillTriggerAcceptsAnAbsentChanceType(t *testing.T) {
	if _, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", TriggeredID: 5144}); err != nil {
		t.Fatalf("New() error = %v, want nil for an absent chanceType", err)
	}
}
type chanceTriggerFakeActor struct {
	tracked []*Effect
}

func (a *chanceTriggerFakeActor) ObjectID() int32 { return 0 }

func (a *chanceTriggerFakeActor) Dead() bool { return false }

func (a *chanceTriggerFakeActor) AddChanceTrigger(e *Effect) {
	a.tracked = append(a.tracked, e)
}

func (a *chanceTriggerFakeActor) RemoveChanceTrigger(e *Effect) {
	for i, cur := range a.tracked {
		if cur == e {
			a.tracked = append(a.tracked[:i], a.tracked[i+1:]...)
			return
		}
	}
}

func TestChanceSkillTriggerInstallsAndRemovesOnTarget(t *testing.T) {
	target := &chanceTriggerFakeActor{}
	e, err := New(Skill{}, modelskill.EffectTemplate{
		Name: "ChanceSkillTrigger", Time: 60, TriggeredID: 5144,
		ChanceType: "ON_ATTACKED", ActivationChance: 80,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	e.Effected = target

	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true")
	}
	if len(target.tracked) != 1 || target.tracked[0] != e {
		t.Fatalf("tracked after OnStart = %+v, want [e]", target.tracked)
	}

	e.OnExit(e)
	if len(target.tracked) != 0 {
		t.Fatalf("tracked after OnExit = %+v, want empty", target.tracked)
	}
}

func TestChanceSkillTriggerOnATargetWithNoTrackingIsANoop(t *testing.T) {
	e, err := New(Skill{}, modelskill.EffectTemplate{Name: "ChanceSkillTrigger", ChanceType: "ON_HIT", ActivationChance: 50})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if !e.OnStart(e) {
		t.Fatal("OnStart() = false, want true even without a tracking target")
	}
	e.OnExit(e)
}
