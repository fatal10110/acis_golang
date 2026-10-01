package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from apply_test.go ----
type effectLandingFake struct {
	neutralCreature
	world.Presence
	fakeActor
	list    *effect.List
	x, y, z int
	invul   bool
}

func (f *effectLandingFake) EffectList() *effect.List { return f.list }

func (f *effectLandingFake) Position() (int, int, int) { return f.x, f.y, f.z }

func (f *effectLandingFake) Invul() bool { return f.invul }

type damagePermissionFake struct {
	neutralCreature
	world.Presence
	fakeActor
	allow bool
}

func (f *damagePermissionFake) CanGiveDamage() bool { return f.allow }
func (*damagePermissionFake) Heading() int          { return 0 }

// combatantOnlyCaster is a combatant that is not a formula caster: the shape
// a new combatant kind has before it grows the formula surface.
type combatantOnlyCaster struct {
	world.Presence
	fakeActor
	allow bool
}

var _ attackable.Combatant = (*combatantOnlyCaster)(nil)

func (f *combatantOnlyCaster) CanGiveDamage() bool { return f.allow }

type positionedFakeActor struct {
	neutralCreature
	world.Presence
	fakeActor
	x, y, z int
}

func (f *positionedFakeActor) Position() (int, int, int) { return f.x, f.y, f.z }

type effectListOnlyFake struct {
	world.Presence
	fakeActor
	list *effect.List
}

func (f *effectListOnlyFake) EffectList() *effect.List { return f.list }

func (*effectLandingFake) EffectSuccessInput(_ creature.FormulaActor, _ modelskill.Definition, tmpl modelskill.EffectTemplate, _ bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	return formulas.SkillSuccessInput{BaseChance: tmpl.EffectPower, IgnoreResists: true, Shield: shield}, true
}

func TestApplyEffectsRollsEachConfiguredTemplate(t *testing.T) {
	target := &effectLandingFake{list: newTestList(nil)}
	templates := []modelskill.EffectTemplate{
		{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true},
		{Name: "Buff", Time: 60, EffectPower: 0, EffectPowerSet: true},
	}
	applyEffects(nil, target, modelskill.Definition{}, templates)

	if got := len(target.list.All()); got != 1 {
		t.Fatalf("landed effects = %d, want 1", got)
	}
}

func TestApplyEffectsEffectRangeAtLanding(t *testing.T) {
	configured := []modelskill.EffectTemplate{{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true}}
	tests := []struct {
		name      string
		caster    effect.Actor
		target    effect.Actor
		templates []modelskill.EffectTemplate
		want      int
	}{
		{
			name:      "outside range",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), x: 101},
			templates: configured,
		},
		{
			name:      "at range",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), x: 100},
			templates: configured,
		},
		{
			name:      "inside range",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), x: 99},
			templates: configured,
			want:      1,
		},
		{
			name:      "self cast",
			caster:    &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target:    &effectLandingFake{fakeActor: fakeActor{objectID: 1}, list: newTestList(nil), x: 1000},
			templates: configured,
			want:      1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEffects(tt.caster, tt.target, modelskill.Definition{EffectRange: 100}, tt.templates)
			if got := len(tt.target.EffectList().All()); got != tt.want {
				t.Fatalf("landed effects = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyEffectsRejectsPerfectShieldBeforeTemplates(t *testing.T) {
	target := &effectLandingFake{list: newTestList(nil)}
	applyEffectsWithLanding(nil, target, modelskill.Definition{}, []modelskill.EffectTemplate{{Name: "Buff", Time: 60, EffectPower: 100, EffectPowerSet: true}}, formulas.ShieldPerfect, false)

	if got := len(target.list.All()); got != 0 {
		t.Fatalf("landed effects after perfect shield = %d, want 0", got)
	}
}

func TestApplyEffectsRefusesOffensiveAndDebuffOnInvulOrDeniedDamage(t *testing.T) {
	if _, ok := effect.Actor(&combatantOnlyCaster{}).(creature.FormulaActor); ok {
		t.Fatal("combatantOnlyCaster must not satisfy creature.FormulaActor")
	}
	landing := []modelskill.EffectTemplate{{Name: "Buff", Time: 60}}
	tests := []struct {
		name   string
		caster effect.Actor
		target *effectLandingFake
		def    modelskill.Definition
		want   int
	}{
		{
			name:   "offensive vs invul",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "debuff vs invul",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Debuff: true},
		},
		{
			name:   "buff vs invul still lands",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			want:   1,
		},
		{
			name:   "self offensive on invul still lands",
			caster: &positionedFakeActor{fakeActor: fakeActor{objectID: 1}},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 1}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Offensive: true},
			want:   1,
		},
		{
			name:   "nil caster still refuses invul offensive",
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil), invul: true},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "offensive when caster cannot give damage",
			caster: &damagePermissionFake{fakeActor: fakeActor{objectID: 1}, allow: false},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "buff when caster cannot give damage still lands",
			caster: &damagePermissionFake{fakeActor: fakeActor{objectID: 1}, allow: false},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			want:   1,
		},
		{
			name:   "offensive when a non-formula combatant cannot give damage",
			caster: &combatantOnlyCaster{fakeActor: fakeActor{objectID: 1}, allow: false},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
		},
		{
			name:   "offensive when a non-formula combatant may give damage",
			caster: &combatantOnlyCaster{fakeActor: fakeActor{objectID: 1}, allow: true},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
			want:   1,
		},
		{
			name:   "offensive when caster may give damage",
			caster: &damagePermissionFake{fakeActor: fakeActor{objectID: 1}, allow: true},
			target: &effectLandingFake{fakeActor: fakeActor{objectID: 2}, list: newTestList(nil)},
			def:    modelskill.Definition{Offensive: true},
			want:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyEffects(tt.caster, tt.target, tt.def, landing)
			if got := len(tt.target.list.All()); got != tt.want {
				t.Fatalf("landed effects = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyEffectsRejectsConfiguredTemplateWithoutLandingInput(t *testing.T) {
	target := &effectListOnlyFake{list: newTestList(nil)}
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
	if ActiveEffect(&placedFakeActor{}, 288) {
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
	StopEffect(&placedFakeActor{}, 288)
}

func (*effectLandingFake) Kind() actor.Kind { return actor.KindNPC }

func (*positionedFakeActor) Kind() actor.Kind { return actor.KindNPC }

var (
	_ Creature = (*effectLandingFake)(nil)
	_ Creature = (*positionedFakeActor)(nil)
	_ Creature = (*damagePermissionFake)(nil)
)
