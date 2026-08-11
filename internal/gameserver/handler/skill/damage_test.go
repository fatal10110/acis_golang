package skill

import (
	"testing"

	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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
	id                   int32
	list                 *effect.List
	successOK            bool
	reflects             bool
	counterSkillPhysical float64
	lastBss              bool
	lastShield           formulas.ShieldDefense
}

func newDamageEffectFake() *damageEffectFake {
	return &damageEffectFake{skillTarget: &skillTarget{}, list: effect.NewList(nil), successOK: true}
}

type shotCaster struct {
	*damageEffectFake
	charged map[modelitem.ShotKind]bool
}

func newShotCaster() *shotCaster {
	return &shotCaster{damageEffectFake: newDamageEffectFake(), charged: make(map[modelitem.ShotKind]bool)}
}

func (c *shotCaster) ChargedShot(kind modelitem.ShotKind) bool { return c.charged[kind] }
func (c *shotCaster) SetChargedShot(kind modelitem.ShotKind, charged bool) {
	c.charged[kind] = charged
}

func (d *damageEffectFake) EffectList() *effect.List { return d.list }

func (d *damageEffectFake) SkillSuccessInput(caster any, def modelskill.Definition, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	d.lastShield = shield
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: 100, Shield: shield}, d.successOK
}

func (d *damageEffectFake) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	if d.reflects {
		return formulas.SkillReflectInput{CanBeReflected: true, CastRange: 40, ReflectChance: 100}
	}
	return formulas.SkillReflectInput{}
}

func (d *damageEffectFake) CounterSkillPhysical() float64 { return d.counterSkillPhysical }

func (d *damageEffectFake) ObjectID() int32 { return d.id }

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

func TestManadamRemovesEffectsBeforeResistedRecast(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.manaOK = true
	target.manaInput = formulas.ManaDamageInput{
		MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
		VulnMul: 1, Affected: true,
	}
	def := modelskill.Definition{ID: 1234, SkillType: "MANADAM", Effects: targetEffect()}

	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})
	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("MANADAM effects with guaranteed landing roll = %d, want 1", got)
	}

	target.successOK = false
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []any{target}})
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MANADAM effects after resisted recast = %d, want 0", got)
	}
}

func TestBlowUsesOneShieldOutcomeForDamageAndEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.blowOK = true
	target.blowInput = formulas.BlowInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, PosMul: 1.2,
		CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		Landed: true, Shield: formulas.ShieldPerfect,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 409, SkillType: "BLOW", Effects: targetEffect()}, Targets: []any{target}})
	if target.hp != 1999 {
		t.Fatalf("perfect-shield BLOW hp = %v, want 1999", target.hp)
	}
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("perfect-shield BLOW effects = %d, want 0", got)
	}
	if target.lastShield != formulas.ShieldPerfect {
		t.Fatalf("BLOW effect shield = %v, want ShieldPerfect", target.lastShield)
	}
}

func TestPdamPerfectShieldBlocksDamageAndEffects(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.physicalOK = true
	target.physicalInput = formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		Shield: formulas.ShieldPerfect,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 321, SkillType: "PDAM", Effects: targetEffect()}, Targets: []any{target}})
	if target.hp != 1999 {
		t.Fatalf("perfect-shield PDAM hp = %v, want 1999", target.hp)
	}
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("perfect-shield PDAM effects = %d, want 0", got)
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

func TestBlowCounterSkillDamagesCasterInsteadOfTarget(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	caster.hp = 2000
	target := newDamageEffectFake()
	target.hp = 2000
	target.counterSkillPhysical = 50
	target.blowOK = true
	target.blowInput = formulas.BlowInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, PosMul: 1.2,
		CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		Landed: true,
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 409, SkillType: "BLOW", CanBeReflected: true},
		Targets: []any{target},
	})

	wantDamage := float64(int(formulas.BlowDamage(target.blowInput))) * .5
	if got := caster.hp; got != 2000-wantDamage {
		t.Fatalf("countered BLOW caster hp = %v, want %v", got, 2000-wantDamage)
	}
	if got := target.hp; got != 2000 {
		t.Fatalf("countered BLOW target hp = %v, want unchanged 2000", got)
	}
}

func TestBlowCounterSkillReportsCounterattackParticipants(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	caster.id, caster.hp = 1, 2000
	target := newDamageEffectFake()
	target.id, target.hp = 2, 2000
	target.counterSkillPhysical = 50
	target.blowOK = true
	target.blowInput = formulas.BlowInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, PosMul: 1.2,
		CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		Landed: true,
	}

	result, ok := registry.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 409, SkillType: "BLOW", CanBeReflected: true},
		Targets: []any{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true")
	}
	if got := result.Counterattacks; len(got) != 1 || got[0].AttackerID != caster.id || got[0].DefenderID != target.id {
		t.Fatalf("Counterattacks = %+v, want caster and target IDs", got)
	}
}

func TestBlowDischargesSoulshotOnlyAfterLanding(t *testing.T) {
	for _, tc := range []struct {
		name        string
		landed      bool
		staticReuse bool
		wantCharged bool
	}{
		{name: "landed", landed: true},
		{name: "missed", wantCharged: true},
		{name: "static reuse", landed: true, staticReuse: true, wantCharged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newShotCaster()
			caster.charged[modelitem.ShotSoul] = true
			target := newDamageEffectFake()
			target.blowOK = true
			target.blowInput = formulas.BlowInput{Landed: tc.landed}

			NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "BLOW", StaticReuse: tc.staticReuse}, Targets: []any{target}})

			if got := caster.ChargedShot(modelitem.ShotSoul); got != tc.wantCharged {
				t.Fatalf("SoulshotCharged() = %v, want %v", got, tc.wantCharged)
			}
		})
	}
}

func TestManadamDischargesLoadedSpiritshotAfterCast(t *testing.T) {
	for _, tc := range []struct {
		name        string
		kind        modelitem.ShotKind
		staticReuse bool
		wantCharged bool
	}{
		{name: "spirit", kind: modelitem.ShotSpirit},
		{name: "blessed spirit", kind: modelitem.ShotBlessedSpirit},
		{name: "static reuse", kind: modelitem.ShotBlessedSpirit, staticReuse: true, wantCharged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newShotCaster()
			caster.charged[tc.kind] = true

			NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM", StaticReuse: tc.staticReuse}})

			if got := caster.ChargedShot(tc.kind); got != tc.wantCharged {
				t.Fatalf("ChargedShot(%v) = %v, want %v", tc.kind, got, tc.wantCharged)
			}
		})
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
