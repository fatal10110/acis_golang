package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type effectLandingFake struct {
	list *effect.List
}

func (f *effectLandingFake) EffectList() *effect.List { return f.list }

type effectListOnlyFake struct {
	list *effect.List
}

func (f *effectListOnlyFake) EffectList() *effect.List { return f.list }

func (*effectLandingFake) EffectSuccessInput(_ any, _ modelskill.Definition, tmpl modelskill.EffectTemplate, _ bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
}

func TestApplyEffectsRollsEachConfiguredTemplate(t *testing.T) {
	target := &effectLandingFake{list: effect.NewList(nil)}
	templates := []modelskill.EffectTemplate{
		{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true},
		{Name: "Buff", Time: 60, EffectPower: 0, EffectPowerSet: true},
	}
	applyEffects(nil, target, modelskill.Definition{}, templates)

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("landed effects = %d, want 1", got)
	}
}

func TestApplyEffectsRejectsPerfectShieldBeforeTemplates(t *testing.T) {
	target := &effectLandingFake{list: effect.NewList(nil)}
	applyEffectsWithLanding(nil, target, modelskill.Definition{}, []modelskill.EffectTemplate{{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true}}, formulas.ShieldPerfect, false)

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("landed effects after perfect shield = %d, want 0", got)
	}
}

func TestApplyEffectsRejectsConfiguredTemplateWithoutLandingInput(t *testing.T) {
	target := &effectListOnlyFake{list: effect.NewList(nil)}
	applyEffects(nil, target, modelskill.Definition{}, []modelskill.EffectTemplate{
		{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true},
		{Name: "Buff", Time: 60},
	})

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("landed effects = %d, want 1 unconfigured template", got)
	}
}

func TestActiveEffectFindsAMatchingLiveInstance(t *testing.T) {
	target := newCancelFakeActor(10)
	addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 288})

	if !ActiveEffect(target, 288) {
		t.Fatal("ActiveEffect() = false, want true for a live instance of skill 288")
	}
	if ActiveEffect(target, 99) {
		t.Fatal("ActiveEffect() = true, want false for a skill id with no live instance")
	}
}

func TestActiveEffectOnATargetWithNoEffectListIsFalse(t *testing.T) {
	if ActiveEffect(struct{}{}, 288) {
		t.Fatal("ActiveEffect() = true, want false for a target with no effect list")
	}
}

func TestStopEffectRemovesTheMatchingLiveInstance(t *testing.T) {
	target := newCancelFakeActor(10)
	e := addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 288})
	addBuff(t, target, modelskill.EffectTemplate{Name: "Buff", Time: 60}, effect.Skill{ID: 4})

	StopEffect(target, 288)

	if hasEffect(target.list, e) {
		t.Fatal("skill 288's effect is still active after StopEffect")
	}
	if !ActiveEffect(target, 4) {
		t.Fatal("StopEffect removed an unrelated skill's active effect")
	}
}

func TestStopEffectOnATargetWithNoEffectListIsANoop(t *testing.T) {
	StopEffect(struct{}{}, 288)
}
