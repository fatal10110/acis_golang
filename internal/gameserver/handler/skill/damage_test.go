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
	name                 string
	list                 *effect.List
	successOK            bool
	successRate          float64
	reflects             bool
	counterSkillPhysical float64
	lastBss              bool
	lastShield           formulas.ShieldDefense
}

func newDamageEffectFake() *damageEffectFake {
	return &damageEffectFake{skillTarget: &skillTarget{}, list: effect.NewList(nil), successOK: true, successRate: 100}
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
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: d.successRate, Shield: shield}, d.successOK
}

func (d *damageEffectFake) EffectSuccessInput(_ any, _ modelskill.Definition, _ modelskill.EffectTemplate, bss bool, shield formulas.ShieldDefense) (formulas.SkillSuccessInput, bool) {
	d.lastBss = bss
	d.lastShield = shield
	return formulas.SkillSuccessInput{IgnoreResists: true, BaseChance: d.successRate, Shield: shield}, d.successOK
}

func (d *damageEffectFake) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	if d.reflects {
		return formulas.SkillReflectInput{CanBeReflected: true, CastRange: 40, ReflectChance: 100}
	}
	return formulas.SkillReflectInput{}
}

func (d *damageEffectFake) CounterSkillPhysical() float64 { return d.counterSkillPhysical }

func (d *damageEffectFake) ObjectID() int32 { return d.id }

func (d *damageEffectFake) CharacterName() string { return d.name }

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
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

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
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

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
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MDAM effects with failed landing roll = %d, want 0", got)
	}

	target.successOK = true
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("MDAM effects with guaranteed landing roll = %d, want 1", got)
	}

	target.successOK = false
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
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
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("BLOW target effects = %d, want 1", got)
	}
	if !target.lastBss {
		t.Fatal("BLOW landing roll bss = false, want true (Blow.java hardcodes this input)")
	}

	target.successOK = false
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
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

	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("MANADAM effects with guaranteed landing roll = %d, want 1", got)
	}

	target.successOK = false
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MANADAM effects after resisted recast = %d, want 0", got)
	}
}

func TestManadamReflectRedirectsDrainAndEffectsToCaster(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	caster.mp, caster.manaOK = 1000, true
	caster.manaInput = formulas.ManaDamageInput{
		MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
		VulnMul: 1, Affected: true,
	}
	target.mp, target.manaOK, target.reflects = 1000, true, true
	target.manaInput = caster.manaInput
	def := modelskill.Definition{ID: 1234, SkillType: "MANADAM", CanBeReflected: true, Effects: targetEffect()}

	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

	if caster.mp >= 1000 || target.mp != 1000 {
		t.Fatalf("MANADAM reflect mp = %v/%v, want caster drained and target unchanged", caster.mp, target.mp)
	}
	if got := len(caster.EffectList().All()); got != 1 {
		t.Fatalf("MANADAM reflected caster effects = %d, want 1", got)
	}
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MANADAM reflected target effects = %d, want 0", got)
	}
}

func TestDamageSkillResistedRollReportsTargetAndSkill(t *testing.T) {
	for _, tc := range []struct {
		name  string
		skill modelskill.Definition
		setup func(*damageEffectFake)
	}{
		{
			name:  "PDAM icon effect",
			skill: modelskill.Definition{ID: 1230, Level: 1, SkillType: "PDAM", Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 600, Icon: true, EffectPower: 100, EffectPowerSet: true}}},
			setup: func(target *damageEffectFake) {
				target.hp, target.physicalOK = 2000, true
				target.physicalInput = formulas.PhysicalSkillInput{AttackPower: 100, Defence: 40, RandomMul: 1, RaceMul: 1, PvPMul: 1, ElementalMul: 1, WeaponVulnMul: 1}
			},
		},
		{
			name:  "MDAM",
			skill: modelskill.Definition{ID: 1231, Level: 2, SkillType: "MDAM", Effects: targetEffect()},
			setup: func(target *damageEffectFake) {
				target.hp, target.magicOK = 2000, true
				target.magicInput = formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1}
			},
		},
		{
			name:  "BLOW",
			skill: modelskill.Definition{ID: 1232, Level: 3, SkillType: "BLOW", Effects: targetEffect()},
			setup: func(target *damageEffectFake) {
				target.hp, target.blowOK = 2000, true
				target.blowInput = formulas.BlowInput{Landed: true, AttackPower: 100, SkillPower: 50, Defence: 40, RandomMul: 1, PosMul: 1}
			},
		},
		{
			name:  "MANADAM",
			skill: modelskill.Definition{ID: 1233, Level: 4, SkillType: "MANADAM", Effects: targetEffect()},
			setup: func(target *damageEffectFake) {
				target.manaOK = true
				target.manaInput = formulas.ManaDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970, VulnMul: 1, Affected: true}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newDamageEffectFake()
			target := newDamageEffectFake()
			target.name, target.successRate = "Target", 0
			tc.setup(target)

			result, handled := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: tc.skill, Targets: []Actor{target}})
			if !handled {
				t.Fatal("UseResult() handled = false")
			}
			if got := result.Resisted; len(got) != 1 || got[0].TargetName != "Target" || got[0].SkillID != tc.skill.ID || got[0].SkillLevel != tc.skill.Level {
				t.Fatalf("Resisted = %+v, want target and skill", got)
			}
		})
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

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 409, SkillType: "BLOW", Effects: targetEffect()}, Targets: []Actor{target}})
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

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 321, SkillType: "PDAM", Effects: targetEffect()}, Targets: []Actor{target}})
	if target.hp != 1999 {
		t.Fatalf("perfect-shield PDAM hp = %v, want 1999", target.hp)
	}
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("perfect-shield PDAM effects = %d, want 0", got)
	}
}

func TestPdamLethalReportsCasterAndTarget(t *testing.T) {
	caster := newDamageEffectFake()
	caster.id = 1
	target := newDamageEffectFake()
	target.id, target.hp, target.physicalOK = 2, 2000, true
	target.physicalInput = formulas.PhysicalSkillInput{
		AttackPower: 100, SkillPower: 50, Defence: 40,
		RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
	}
	target.lethalOK = true
	target.lethalInput = formulas.LethalInput{AttackerLevel: 1, TargetLevel: 1, LethalMul: 1000}

	result, handled := NewDefaultRegistry().UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "PDAM", LethalChance2: 1},
		Targets: []Actor{target},
	})
	if !handled {
		t.Fatal("UseResult() handled = false")
	}
	if got := result.Lethals; len(got) != 1 || got[0].AttackerID != caster.id || got[0].TargetID != target.id {
		t.Fatalf("Lethals = %+v, want caster %d and target %d", got, caster.id, target.id)
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

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 409, SkillType: "BLOW", Effects: targetEffect()}, Targets: []Actor{target}})
	if target.hp != 2000 {
		t.Fatalf("BLOW miss hp = %v, want unchanged 2000", target.hp)
	}
	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("BLOW miss effects = %d, want 0", got)
	}
}

func TestBlowEvasionSkipsDamageAndReportsParticipants(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	caster.id, caster.name, caster.hp = 1, "Attacker", 2000
	target := newDamageEffectFake()
	target.id, target.name, target.hp = 2, "Defender", 2000
	target.blowOK = true
	target.blowInput = formulas.BlowInput{Landed: true, Evaded: true}

	result, ok := registry.UseResult(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{ID: 409, SkillType: "BLOW", Effects: targetEffect()},
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true")
	}
	if target.hp != 2000 || len(target.EffectList().All()) != 0 {
		t.Fatalf("evaded BLOW target = hp %v effects %d, want unchanged", target.hp, len(target.EffectList().All()))
	}
	if got := result.Dodges; len(got) != 1 || got[0].AttackerID != caster.id || got[0].AttackerName != caster.name || got[0].DefenderID != target.id || got[0].DefenderName != target.name {
		t.Fatalf("Dodges = %+v, want caster and target IDs and names", got)
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
		Targets: []Actor{target},
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
	caster.id, caster.name, caster.hp = 1, "Attacker", 2000
	target := newDamageEffectFake()
	target.id, target.name, target.hp = 2, "Countering NPC", 2000
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
		Targets: []Actor{target},
	})
	if !ok {
		t.Fatal("UseResult() handled = false, want true")
	}
	if got := result.Counterattacks; len(got) != 1 || got[0].AttackerID != caster.id || got[0].AttackerName != caster.name || got[0].DefenderID != target.id || got[0].DefenderName != target.name {
		t.Fatalf("Counterattacks = %+v, want caster and target IDs and names", got)
	}
}

func TestBlowDischargesSoulshotOnlyAfterLanding(t *testing.T) {
	for _, tc := range []struct {
		name        string
		landed      bool
		evaded      bool
		staticReuse bool
		wantCharged bool
	}{
		{name: "landed", landed: true},
		{name: "evaded", landed: true, evaded: true, wantCharged: true},
		{name: "missed", wantCharged: true},
		{name: "static reuse", landed: true, staticReuse: true, wantCharged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caster := newShotCaster()
			caster.charged[modelitem.ShotSoul] = true
			target := newDamageEffectFake()
			target.blowOK = true
			target.blowInput = formulas.BlowInput{Landed: tc.landed, Evaded: tc.evaded}

			NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "BLOW", StaticReuse: tc.staticReuse}, Targets: []Actor{target}})

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

func TestMdamReflectAppliesEffectsUnconditionallyWithNoLandingRoll(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.magicOK = true
	target.magicInput = formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1}
	target.reflects = true
	target.successOK = false

	def := modelskill.Definition{ID: 1231, SkillType: "MDAM", Effects: targetEffect()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("MDAM reflected target effects = %d, want 0", got)
	}
	if got := len(caster.EffectList().All()); got != 1 {
		t.Fatalf("MDAM reflected caster effects with failed landing roll = %d, want 1 (Mdam.java's reflect branch never rolls)", got)
	}
}

func mdamEffectPowerTemplate() []modelskill.EffectTemplate {
	return []modelskill.EffectTemplate{{Name: "Buff", Time: 600, EffectPower: 100, EffectPowerSet: true}}
}

func TestMdamReflectHardcodesBssFalseForEffectLanding(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.magicOK = true
	target.magicInput = formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1, BlessedSoulShot: true}
	target.reflects = true

	def := modelskill.Definition{ID: 1231, SkillType: "MDAM", Effects: mdamEffectPowerTemplate()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

	if caster.lastBss {
		t.Fatalf("MDAM reflect branch EffectSuccessInput bss = true, want false (Mdam.java's 2-arg getEffects hardcodes false)")
	}
}

func TestMdamNonReflectPassesRealBssForEffectLanding(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp = 2000
	target.magicOK = true
	target.magicInput = formulas.MagicDamageInput{MAtk: 400, MDef: 50, SkillPower: 20, PvPMul: 1, ElementalMul: 1, BlessedSoulShot: true}

	def := modelskill.Definition{ID: 1231, SkillType: "MDAM", Effects: mdamEffectPowerTemplate()}
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

	if !target.lastBss {
		t.Fatalf("MDAM non-reflect branch EffectSuccessInput bss = false, want true (caster's real blessed-spiritshot charge)")
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
	registry.Use(Cast{Caster: caster, Skill: def, Targets: []Actor{target}})

	if got := len(target.EffectList().All()); got != 0 {
		t.Fatalf("PDAM reflected target effects = %d, want 0", got)
	}
	if got := len(caster.EffectList().All()); got != 1 {
		t.Fatalf("PDAM reflected caster effects = %d, want 1", got)
	}
}

type chargeCaster struct {
	*damageEffectFake
	charges int
}

func (c *chargeCaster) Charges() int { return c.charges }

func TestPdamAndChargeDamCounterSkillDamageCaster(t *testing.T) {
	input := formulas.PhysicalSkillInput{AttackPower: 100, SkillPower: 50, Defence: 40, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1}
	for _, tc := range []struct {
		name   string
		caster func() (Actor, *damageEffectFake)
		skill  modelskill.Definition
		mul    float64
	}{
		{"PDAM", func() (Actor, *damageEffectFake) { caster := newDamageEffectFake(); return caster, caster }, modelskill.Definition{ID: 321, SkillType: "PDAM", CanBeReflected: true, Effects: targetEffect()}, 1},
		{"CHARGEDAM", func() (Actor, *damageEffectFake) {
			caster := &chargeCaster{damageEffectFake: newDamageEffectFake(), charges: 2}
			return caster, caster.damageEffectFake
		}, modelskill.Definition{ID: 214, SkillType: "CHARGEDAM", CanBeReflected: true, NumCharges: 1, Effects: targetEffect()}, 1.4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, outcome := range []struct {
				name          string
				reflects      bool
				casterEffects int
			}{
				{name: "counter only"},
				{name: "counter and reflect", reflects: true, casterEffects: 1},
			} {
				t.Run(outcome.name, func(t *testing.T) {
					caster, health := tc.caster()
					health.hp = 2000
					target := newDamageEffectFake()
					target.hp, target.physicalOK, target.physicalInput, target.counterSkillPhysical = 2000, true, input, 50
					target.reflects = outcome.reflects

					result, _ := NewDefaultRegistry().UseResult(Cast{Caster: caster, Skill: tc.skill, Targets: []Actor{target}})

					want := formulas.PhysicalSkillDamage(input) * tc.mul * .5
					if health.hp != 2000-want || target.hp != 2000 || len(result.Counterattacks) != 1 || len(health.EffectList().All()) != outcome.casterEffects || len(target.EffectList().All()) != 1-outcome.casterEffects {
						t.Fatalf("counter %s hp = %v/%v, reports/effects = %d/%d/%d; want %v/2000, 1/%d/%d", tc.name, health.hp, target.hp, len(result.Counterattacks), len(health.EffectList().All()), len(target.EffectList().All()), 2000-want, outcome.casterEffects, 1-outcome.casterEffects)
					}
				})
			}
		})
	}
}

func TestBlowCounterAndReflectKeepsEffectsOnTarget(t *testing.T) {
	caster := newDamageEffectFake()
	target := newDamageEffectFake()
	target.hp, target.blowOK, target.counterSkillPhysical, target.reflects = 2000, true, 50, true
	target.blowInput = formulas.BlowInput{AttackPower: 100, SkillPower: 50, Defence: 40, RandomMul: 1, PosMul: 1.2, CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, Landed: true}

	NewDefaultRegistry().Use(Cast{Caster: caster, Skill: modelskill.Definition{ID: 409, SkillType: "BLOW", CanBeReflected: true, Effects: targetEffect()}, Targets: []Actor{target}})

	if got := len(target.EffectList().All()); got != 1 {
		t.Fatalf("combined-counter BLOW target effects = %d, want 1", got)
	}
	if got := len(caster.EffectList().All()); got != 0 {
		t.Fatalf("combined-counter BLOW caster effects = %d, want 0", got)
	}
}
