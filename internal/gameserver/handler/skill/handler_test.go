package skill

import (
	"math"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
)

type recordingHandler struct {
	types []string
	uses  int
}

func (h *recordingHandler) Types() []string { return h.types }

func (h *recordingHandler) Use(Cast) { h.uses++ }

func TestRegistryDispatchesBySkillType(t *testing.T) {
	h := &recordingHandler{types: []string{"HEAL_PERCENT", "MANAHEAL_PERCENT"}}
	registry := NewRegistry(h)

	if _, ok := registry.Handler("heal_percent"); !ok {
		t.Fatal("Handler() did not normalize skill type keys")
	}
	if !registry.Use(Cast{Skill: modelskill.Definition{SkillType: "MANAHEAL_PERCENT"}}) {
		t.Fatal("Use() returned false for a registered skill type")
	}
	if h.uses != 1 {
		t.Fatalf("handler uses = %d, want 1", h.uses)
	}
	if registry.Use(Cast{Skill: modelskill.Definition{SkillType: "NOT_REGISTERED"}}) {
		t.Fatal("Use() returned true for an unregistered skill type")
	}
}

func TestDefaultRegistryHasRepresentativeHandlers(t *testing.T) {
	registry := NewDefaultRegistry()

	for _, skillType := range []string{
		"PDAM", "FATAL", "MDAM", "DEATHLINK", "BLOW", "MANADAM",
		"HEAL", "HEAL_STATIC", "HEAL_PERCENT", "MANAHEAL_PERCENT", "MANAHEAL", "MANARECHARGE",
		"COMBATPOINTHEAL", "BALANCE_LIFE", "REAL_DAMAGE", "GIVE_SP",
		"CPDAMPERCENT", "DUMMY", "BEAST_FEED",
		"SUMMON_CREATURE", "SUMMON_FRIEND", "SUMMON_PARTY", "ERASE",
	} {
		if _, ok := registry.Handler(skillType); !ok {
			t.Fatalf("default registry missing %s", skillType)
		}
	}
}

type skillTarget struct {
	hp, maxHP float64
	mp, maxMP float64
	cp, maxCP float64

	dead         bool
	invulnerable bool
	cursed       bool

	sp       int
	diedBy   any
	recharge float64

	healAmount        float64
	healEffectiveness float64
	healOK            bool

	physicalInput formulas.PhysicalSkillInput
	physicalOK    bool
	magicInput    formulas.MagicDamageInput
	magicOK       bool
	blowInput     formulas.BlowInput
	blowOK        bool
	manaInput     formulas.ManaDamageInput
	manaOK        bool
	lethalInput   formulas.LethalInput
	lethalOK      bool
	lethalPlayer  bool

	raidRelated  bool
	lethalImmune bool

	lethalOutcomes []formulas.LethalOutcome
}

func (t *skillTarget) AlikeDead() bool { return t.dead }
func (t *skillTarget) Dead() bool      { return t.dead }

func (t *skillTarget) Invulnerable() bool { return t.invulnerable }

func (t *skillTarget) CursedWeaponEquipped() bool { return t.cursed }

func (t *skillTarget) RaidRelated() bool { return t.raidRelated }

func (t *skillTarget) Lethalable() bool { return !t.lethalImmune }

func (t *skillTarget) CanBeHealed() bool {
	return !t.dead && !t.invulnerable && !t.cursed
}

func (t *skillTarget) HealAmount(skill modelskill.Definition) (float64, bool) {
	return t.healAmount, t.healOK
}

func (t *skillTarget) HealEffectiveness() float64 {
	if t.healEffectiveness == 0 {
		return 100
	}
	return t.healEffectiveness
}

func (t *skillTarget) HP() float64         { return t.hp }
func (t *skillTarget) MaxHPValue() float64 { return t.maxHP }

func (t *skillTarget) SetHP(v float64) { t.hp = v }

func (t *skillTarget) AddHP(v float64) float64 {
	if t.hp+v > t.maxHP {
		v = t.maxHP - t.hp
	}
	if v == 0 {
		return 0
	}
	t.hp += v
	return v
}

func (t *skillTarget) MaxMPValue() float64 { return t.maxMP }
func (t *skillTarget) MPValue() float64    { return t.mp }

func (t *skillTarget) AddMP(v float64) float64 {
	if t.mp+v > t.maxMP {
		v = t.maxMP - t.mp
	}
	if v == 0 {
		return 0
	}
	t.mp += v
	return v
}

func (t *skillTarget) ReduceMP(v float64) float64 {
	if t.mp-v < 0 {
		v = t.mp
	}
	if v == 0 {
		return 0
	}
	t.mp -= v
	return v
}

func (t *skillTarget) RechargeMP(v float64) float64 { return v * t.recharge }

func (t *skillTarget) CP() float64         { return t.cp }
func (t *skillTarget) MaxCPValue() float64 { return t.maxCP }
func (t *skillTarget) SetCP(v float64) {
	if v < 0 {
		v = 0
	}
	if v > t.maxCP {
		v = t.maxCP
	}
	t.cp = v
}

func (t *skillTarget) AddExpAndSP(exp, sp int) { t.sp += sp }

func (t *skillTarget) Die(killer any) {
	t.dead = true
	t.diedBy = killer
}

func (t *skillTarget) ReduceHP(v float64, attacker any, skill modelskill.Definition) {
	t.hp -= v
}

func (t *skillTarget) PhysicalSkillInput(caster any, skill modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return t.physicalInput, t.physicalOK
}

func (t *skillTarget) MagicDamageInput(caster any, skill modelskill.Definition) (formulas.MagicDamageInput, bool) {
	return t.magicInput, t.magicOK
}

func (t *skillTarget) BlowInput(caster any, skill modelskill.Definition) (formulas.BlowInput, bool) {
	return t.blowInput, t.blowOK
}

func (t *skillTarget) ManaDamageInput(caster any, skill modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return t.manaInput, t.manaOK
}

func (t *skillTarget) LethalInput(caster any, skill modelskill.Definition) (formulas.LethalInput, bool) {
	in := t.lethalInput
	in.Chance1 = skill.LethalChance1
	in.Chance2 = skill.LethalChance2
	in.MagicLevel = skill.MagicLevel
	return in, t.lethalOK
}

func (t *skillTarget) ApplyLethalOutcome(outcome formulas.LethalOutcome, caster any, skill modelskill.Definition) {
	t.lethalOutcomes = append(t.lethalOutcomes, outcome)
	switch outcome {
	case formulas.LethalFull:
		t.hp = 1
		if t.lethalPlayer {
			t.cp = 1
		}
	case formulas.LethalHalf:
		if t.lethalPlayer {
			t.cp = 1
		} else {
			t.hp -= t.hp / 2
		}
	}
}

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestHealPercentRestoresHPOrMP(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{hp: 50, maxHP: 100, mp: 10, maxMP: 50}
	dead := &skillTarget{hp: 10, maxHP: 100, dead: true}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "HEAL_PERCENT", Power: 25},
		Targets: []any{target, dead, "not a creature"},
	})
	if target.hp != 75 {
		t.Fatalf("HEAL_PERCENT hp = %v, want 75", target.hp)
	}
	if dead.hp != 10 {
		t.Fatalf("dead target hp = %v, want unchanged 10", dead.hp)
	}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MANAHEAL_PERCENT", Power: 40},
		Targets: []any{target},
	})
	if target.mp != 30 {
		t.Fatalf("MANAHEAL_PERCENT mp = %v, want 30", target.mp)
	}
}

func TestHealRestoresResolvedAmount(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{healAmount: 80, healOK: true}
	target := &skillTarget{hp: 50, maxHP: 200, healEffectiveness: 125}
	dead := &skillTarget{hp: 10, maxHP: 100, dead: true}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "HEAL", Power: 30},
		Targets: []any{target, dead, "not a creature"},
	}) {
		t.Fatal("Use() returned false for HEAL")
	}
	if target.hp != 150 {
		t.Fatalf("HEAL hp = %v, want 150", target.hp)
	}
	if dead.hp != 10 {
		t.Fatalf("dead target hp = %v, want unchanged 10", dead.hp)
	}

	caster.healAmount = 500
	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "HEAL_STATIC", Power: 30},
		Targets: []any{target},
	})
	if target.hp != 200 {
		t.Fatalf("HEAL_STATIC hp = %v, want clamped to 200", target.hp)
	}
}

func TestManaHealAndRecharge(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{mp: 70, maxMP: 100, recharge: 0.5}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MANAHEAL", Power: 50},
		Targets: []any{target},
	})
	if target.mp != 100 {
		t.Fatalf("MANAHEAL mp = %v, want clamped to 100", target.mp)
	}

	target.mp = 10
	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "MANARECHARGE", Power: 50},
		Targets: []any{target},
	})
	if target.mp != 35 {
		t.Fatalf("MANARECHARGE mp = %v, want 35", target.mp)
	}
}

func TestCombatPointHealClampsAndSkipsInvalidTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	target := &skillTarget{cp: 80, maxCP: 100}
	dead := &skillTarget{cp: 1, maxCP: 100, dead: true}
	invulnerable := &skillTarget{cp: 1, maxCP: 100, invulnerable: true}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "COMBATPOINTHEAL", Power: 40},
		Targets: []any{target, dead, invulnerable},
	})
	if target.cp != 100 {
		t.Fatalf("cp = %v, want clamped to 100", target.cp)
	}
	if dead.cp != 1 || invulnerable.cp != 1 {
		t.Fatalf("invalid target cp changed: dead=%v invulnerable=%v", dead.cp, invulnerable.cp)
	}
}

func TestCPDamagePercentReducesCurrentCP(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{cp: 80, maxCP: 100}
	dead := &skillTarget{cp: 80, maxCP: 100, dead: true}
	invulnerable := &skillTarget{cp: 80, maxCP: 100, invulnerable: true}

	if !registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "CPDAMPERCENT", Power: 35},
		Targets: []any{target, dead, invulnerable, "not a player"},
	}) {
		t.Fatal("Use() returned false for CPDAMPERCENT")
	}
	if target.cp != 52 {
		t.Fatalf("CPDAMPERCENT cp = %v, want 52", target.cp)
	}
	if dead.cp != 80 || invulnerable.cp != 80 {
		t.Fatalf("invalid target cp changed: dead=%v invulnerable=%v", dead.cp, invulnerable.cp)
	}
}

func TestBalanceLifeEqualizesLivingTargets(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	a := &skillTarget{hp: 20, maxHP: 100}
	b := &skillTarget{hp: 80, maxHP: 200}
	dead := &skillTarget{hp: 1, maxHP: 100, dead: true}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BALANCE_LIFE"},
		Targets: []any{a, b, dead},
	})

	if !almost(a.hp, 100.0/3.0) || !almost(b.hp, 200.0/3.0) {
		t.Fatalf("balanced hp = %v/%v, want one-third of max hp", a.hp, b.hp)
	}
	if dead.hp != 1 {
		t.Fatalf("dead hp = %v, want unchanged 1", dead.hp)
	}
}

func TestGiveSPRealDamageAndDummy(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{hp: 25, maxHP: 100}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "GIVE_SP", Power: 42.9},
		Targets: []any{target},
	})
	if target.sp != 42 {
		t.Fatalf("sp = %d, want truncated skill power 42", target.sp)
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "REAL_DAMAGE", Power: 10},
		Targets: []any{target},
	})
	if target.hp != 15 || target.dead {
		t.Fatalf("after nonlethal real damage hp=%v dead=%v, want 15/false", target.hp, target.dead)
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "REAL_DAMAGE", Power: 20},
		Targets: []any{target},
	})
	if !target.dead || target.diedBy != caster {
		t.Fatalf("lethal real damage dead=%v diedBy=%p, want caster %p", target.dead, target.diedBy, caster)
	}

	registry.Use(Cast{
		Skill:   modelskill.Definition{SkillType: "DUMMY", Power: 1000},
		Targets: []any{target},
	})
	if target.hp != 15 {
		t.Fatalf("dummy changed hp to %v, want unchanged 15", target.hp)
	}
}

func TestPhysicalMagicBlowAndManaDamageHandlersUseFormulaInputs(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp: 2000,
		mp: 100,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 60,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		},
		physicalOK: true,
		magicInput: formulas.MagicDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20,
			PvPMul: 1, ElementalMul: 1,
		},
		magicOK: true,
		blowInput: formulas.BlowInput{
			AttackPower: 100, SkillPower: 50, Defence: 40,
			RandomMul: 1, PosMul: 1.2,
			CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		},
		blowOK: true,
		manaInput: formulas.ManaDamageInput{
			MAtk: 400, MDef: 50, SkillPower: 20, TargetMaxMp: 970,
			VulnMul: 1,
		},
		manaOK: true,
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "PDAM"}, Targets: []any{target}})
	if !almost(target.hp, 2000-192.5) {
		t.Fatalf("PDAM hp = %v, want %v", target.hp, 2000-192.5)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MDAM"}, Targets: []any{target}})
	if !almost(target.hp, 2000-192.5-728) {
		t.Fatalf("MDAM hp = %v, want %v", target.hp, 2000-192.5-728)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "BLOW"}, Targets: []any{target}})
	if !almost(target.hp, 2000-192.5-728-577) {
		t.Fatalf("BLOW hp = %v, want %v", target.hp, 2000-192.5-728-577)
	}

	registry.Use(Cast{Caster: caster, Skill: modelskill.Definition{SkillType: "MANADAM"}, Targets: []any{target}})
	if target.mp != 20 {
		t.Fatalf("MANADAM mp = %v, want 20", target.mp)
	}
}

func TestPhysicalAndBlowHandlersResolveLethalHits(t *testing.T) {
	registry := NewDefaultRegistry()
	caster := &skillTarget{}
	target := &skillTarget{
		hp:           2000,
		cp:           300,
		lethalPlayer: true,
		physicalInput: formulas.PhysicalSkillInput{
			AttackPower: 100, SkillPower: 50, Defence: 60,
			RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1, ElementalMul: 1,
		},
		physicalOK: true,
		blowInput: formulas.BlowInput{
			AttackPower: 100, SkillPower: 50, Defence: 40,
			RandomMul: 1, PosMul: 1.2,
			CritDamageMul: 1.5, CritDamagePosMul: 1, CritVulnMul: 1, DaggerVulnMul: 1, CritDamageAddBase: 5,
		},
		blowOK: true,
		lethalInput: formulas.LethalInput{
			AttackerLevel: 40,
			TargetLevel:   40,
			LethalMul:     1,
		},
		lethalOK: true,
	}

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "PDAM", LethalChance2: 100},
		Targets: []any{target},
	})
	if target.hp != 1 || target.cp != 1 {
		t.Fatalf("PDAM lethal2 hp/cp = %v/%v, want 1/1", target.hp, target.cp)
	}
	if len(target.lethalOutcomes) != 1 || target.lethalOutcomes[0] != formulas.LethalFull {
		t.Fatalf("PDAM lethal outcomes = %v, want [LethalFull]", target.lethalOutcomes)
	}

	target.hp = 2000
	target.cp = 300
	target.lethalOutcomes = nil

	registry.Use(Cast{
		Caster:  caster,
		Skill:   modelskill.Definition{SkillType: "BLOW", LethalChance1: 100},
		Targets: []any{target},
	})
	if !almost(target.hp, 1423) || target.cp != 1 {
		t.Fatalf("BLOW lethal1 hp/cp = %v/%v, want 1423/1", target.hp, target.cp)
	}
	if len(target.lethalOutcomes) != 1 || target.lethalOutcomes[0] != formulas.LethalHalf {
		t.Fatalf("BLOW lethal outcomes = %v, want [LethalHalf]", target.lethalOutcomes)
	}
}
