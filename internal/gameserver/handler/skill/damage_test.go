package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

// damageEffectFake wraps skillTarget (already shaped for PDAM/MDAM/BLOW
// formula inputs) with the extra surface applyPdamEffects/applyMdamEffects/
// applyBlowEffects probe for: an effect list, a guaranteed-success landing
// roll by default, and an opt-in reflect.
type damageEffectFake struct {
	*skillTarget
	list      *effect.List
	successOK bool
	reflects  bool
	lastBss   bool
}

func newDamageEffectFake() *damageEffectFake {
	return &damageEffectFake{skillTarget: &skillTarget{}, list: effect.NewList(nil), successOK: true}
}

func (d *damageEffectFake) EffectList() *effect.List { return d.list }

func (d *damageEffectFake) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100}, d.successOK
}

func (d *damageEffectFake) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	if d.reflects {
		return formulas.SkillReflectInput{CanBeReflected: true, CastRange: 40, ReflectChance: 100}
	}
	return formulas.SkillReflectInput{}
}

func targetEffect() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Time: 600}}
}

func TestPdamAppliesTargetAndSelfEffectsUnconditionally(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.physicalOK = true
	target.physicalInput = formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 60,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}

	def := modelskill.Definition{ID: 321, SkillType: "PDAM", Effects: targetEffect(), SelfEffects: buffEffect()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})

	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("PDAM target effects = %d, want 1", got)
	}
	if got := len(caster.EffectList().All()); got != 1 {
		t.Fatalf("PDAM caster self-effects = %d, want 1", got)
	}
}

func TestPdamSkipsEffectsWhenTargetBlocksDebuff(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.physicalOK = true
	blocker, err := effect.New(effect.Skill{ID: 999}, modelskill.EffectTemplate{Name: "BlockDebuff", Time: 600})
	if err != nil {
		t.Fatalf("effect.New(BlockDebuff) error: %v", err)
	}
	target.EffectList().Add(blocker)

	def := modelskill.Definition{ID: 321, SkillType: "PDAM", Effects: targetEffect()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})

	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("PDAM target effects with BLOCK_DEBUFF = %d, want 1 (only the blocker)", got)
	}
}

func TestMdamAppliesEffectsOnlyOnDamageAndLandingRoll(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.magicOK = true
	target.magicInput = formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1}
	target.successOK = false

	def := modelskill.Definition{ID: 1231, SkillType: "MDAM", Effects: targetEffect()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MDAM effects with failed landing roll = %d, want 0", got)
	}

	target.successOK = true
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})
	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("MDAM effects with guaranteed landing roll = %d, want 1", got)
	}

	target.successOK = false
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MDAM effects after resisted recast = %d, want 0", got)
	}
}

func TestBlowAppliesEffectsWithForcedBlessedSpiritshotInput(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.blowOK = true
	target.blowInput = formulas.BlowInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, PosMul: 1.2,
		CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		Landed: true,
	}

	def := modelskill.Definition{ID: 409, SkillType: "BLOW", Effects: targetEffect()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})

	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("BLOW target effects = %d, want 1", got)
	}
	if !target.lastBss {
		t.Fatal("BLOW landing roll bss = false, want true (Blow.java hardcodes this input)")
	}

	target.successOK = false
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("BLOW effects after resisted recast = %d, want 0", got)
	}
}

func TestBlowMissSkipsDamageAndEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.blowOK = true
	target.blowInput = formulas.BlowInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, PosMul: 1.2,
		CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		Landed: false,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 409, SkillType: "BLOW", Effects: targetEffect()}, Targets: []any{target}})
	if target.hp != 2000 {
		t.Fatalf("BLOW miss hp = %v, want unchanged 2000", target.hp)
	}
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("BLOW miss effects = %d, want 0", got)
	}
}

func TestDamageSkillEffectsRedirectOntoCasterOnReflect(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.physicalOK = true
	target.physicalInput = formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 60,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}
	target.reflects = true

	def := modelskill.Definition{ID: 321, SkillType: "PDAM", Effects: targetEffect()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})

	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("PDAM reflected target effects = %d, want 0", got)
	}
	if got := len(caster.EffectList().All()); got != 1 {
		t.Fatalf("PDAM reflected caster effects = %d, want 1", got)
	}
}
