package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// reflect sources wired to a guaranteed-success roll by default.
type continuousFake struct {
	id                int32
	dead, invul       bool
	playable          bool
	attackableFlag    bool
	cursed            bool
	bss               bool
	list              *effect.List
	successOK         bool
	reflectOK         bool
	successInput      formulas.SkillSuccessInput
	skillReflectInput formulas.SkillReflectInput

	// recordSuccessInput, when set, is called with every SkillSuccessInput
	// invocation's raw arguments, letting tests assert on the resolved
	// caster/shield state without duplicating checkSkillSuccess's logic.
	recordSuccessInput func(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense)

	// aggression-event recording: which optional surface fired, and with
	// what arguments.
	aggressionSource  any
	aggressionPower   int
	currentTarget     worldobject.Object
	setTargetCalls    []worldobject.Object
	attackTargetCalls []worldobject.Object
}

func newContinuousFake(id int32) *continuousFake {
	return &continuousFake{
		id:           id,
		list:         effect.NewList(nil),
		successOK:    true,
		successInput: formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100},
		reflectOK:    true,
	}
}

func (f *continuousFake) ObjectID() int32                { return f.id }
func (*continuousFake) CharacterName() string            { return "Target" }
func (f *continuousFake) Dead() bool                     { return f.dead }
func (f *continuousFake) Invul() bool                    { return f.invul }
func (f *continuousFake) Playable() bool                 { return f.playable }
func (f *continuousFake) Attackable() bool               { return f.attackableFlag }
func (f *continuousFake) CursedWeaponEquipped() bool     { return f.cursed }
func (f *continuousFake) EffectList() *effect.List       { return f.list }
func (f *continuousFake) BlessedSpiritshotCharged() bool { return f.bss }

func (f *continuousFake) SkillSuccessInput(caster creature.DeathActor, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	if f.recordSuccessInput != nil {
		f.recordSuccessInput(caster, def, bss, shield)
	}
	return f.successInput, f.successOK
}

func (f *continuousFake) SkillReflectInput(modelskill.Definition) formulas.SkillReflectInput {
	return f.skillReflectInput
}

func (f *continuousFake) NotifyAggression(source creature.DeathActor, power int) {
	f.aggressionSource = source
	f.aggressionPower = power
}

func (f *continuousFake) CurrentTarget() worldobject.Object { return f.currentTarget }

func (f *continuousFake) SetTarget(target worldobject.Object) {
	f.setTargetCalls = append(f.setTargetCalls, target)
}

func (f *continuousFake) AttackTarget(target worldobject.Object) {
	f.attackTargetCalls = append(f.attackTargetCalls, target)
}

// addContinuousEffect seeds target's list with one effect of the given effect
// type, used to pre-arm BLOCK_BUFF / BLOCK_BUFF immunity.
func addContinuousEffect(t *testing.T, target *continuousFake, effectType string) {
	t.Helper()
	e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "Buff", Time: 600, EffectType: effectType})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = target
	target.list.Add(e)
}

func buffEffect() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Time: 600}}
}

type continuousDefinitions map[modelskill.Ref]modelskill.Definition

func (d continuousDefinitions) Definition(ref modelskill.Ref) (modelskill.Definition, bool) {
	def, ok := d[ref]
	return def, ok
}

func (d continuousDefinitions) MaxLevel(id modelskill.ID) int {
	max := 0
	for ref := range d {
		if ref.ID == id && ref.Level > max {
			max = ref.Level
		}
	}
	return max
}

func TestContinuousRegistryHasAllHandledTypes(t *testing.T) {
	registry := NewDefaultRegistry()
	for _, typ := range []string{
		"BUFF", "DEBUFF", "DOT", "MDOT", "POISON", "BLEED",
		"HOT", "MPHOT", "FEAR", "CONT", "WEAKNESS", "REFLECT",
		"AGGDEBUFF", "FUSION",
	} {
		if _, ok := registry.Handler(typ); !ok {
			t.Errorf("continuous handler missing registered skill type %q", typ)
		}
	}
}
