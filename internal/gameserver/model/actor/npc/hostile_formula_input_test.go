package npc

import (
	"math"
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestHostileFormulaInputsResolveStatsAndRaceMultiplier(t *testing.T) {
	caster := newCombatHostile(t, 1, &Template{
		ID: 1, Type: "Monster", Level: 1,
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		HPMax: 500, MPMax: 200, CritRate: 4, BaseRandomDamage: 5,
	})
	target := newCombatHostile(t, 2, &Template{
		ID: 2, Type: "Monster", Level: 1, Race: RaceBeast,
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		HPMax: 500, MPMax: 200, CritRate: 4,
	})
	caster.SetRollSource(zeroRoll)
	target.SetRollSource(zeroRoll)

	owner := &struct{}{}
	caster.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(owner, stat.PAtkBeasts, 24, nil),
		basefunc.NewAdd(owner, stat.HealProficiency, 11, nil),
	})
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewAdd(owner, stat.PDefBeasts, 4, nil),
		basefunc.NewMul(owner, stat.FireRes, 0.36, nil),
		basefunc.NewMul(owner, stat.StunVuln, 0.5, nil),
		basefunc.NewMul(owner, stat.DaggerWpnVuln, 0.8, nil),
		basefunc.NewMul(owner, stat.RechargeMPRate, 1.5, nil),
		basefunc.NewMul(owner, stat.HealEffectiveness, 1.2, nil),
	})

	if got := target.Category(); got != skilltarget.CategoryAttackable {
		t.Fatalf("Category() = %v, want attackable", got)
	}
	if target.Playable() {
		t.Fatal("Playable() = true for an attackable NPC")
	}

	phys, ok := target.PhysicalSkillInput(caster, modelskill.Definition{
		Power:        30,
		SkillType:    "PDAM",
		Element:      modelskill.ElementFire,
		BaseCritRate: 1,
	})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if got, want := phys.AttackPower, 108.0; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("PhysicalSkillInput AttackPower = %v, want %v", got, want)
	}
	if got, want := phys.Defence, 45.0; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("PhysicalSkillInput Defence = %v, want %v", got, want)
	}
	if !phys.Crit {
		t.Fatal("PhysicalSkillInput Crit = false, want true with zero roll")
	}
	if got, want := phys.RandomMul, 0.95; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RandomMul = %v, want %v", got, want)
	}
	if got, want := phys.ElementalMul, 0.36; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("PhysicalSkillInput ElementalMul = %v, want %v", got, want)
	}
	if got, want := phys.RaceMul, 1.2; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RaceMul = %v, want %v", got, want)
	}
	if phys.PvPMul != 1 || phys.WeaponVulnMul != 1 {
		t.Fatalf("PhysicalSkillInput neutral PvP/weapon multipliers = %+v", phys)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{
		Power:     40,
		SkillType: "MDAM",
		Magic:     true,
		Element:   modelskill.ElementFire,
	})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if magic.MAtk <= 0 || magic.MDef <= 0 {
		t.Fatalf("MagicDamageInput non-positive attack/defence = %+v", magic)
	}
	if !magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = false, want true with zero roll")
	}
	if magic.PvPMul != 1 || !closeNPCFormulaFloat(magic.ElementalMul, 0.36) {
		t.Fatalf("MagicDamageInput multipliers = %+v", magic)
	}

	blow, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if blow.IsPvP {
		t.Fatal("BlowInput IsPvP = true for NPC-vs-NPC")
	}
	if got, want := blow.RandomMul, 0.95; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("BlowInput RandomMul = %v, want %v", got, want)
	}
	if got, want := blow.DaggerVulnMul, 0.8; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("BlowInput DaggerVulnMul = %v, want %v", got, want)
	}

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{
		Power:     20,
		SkillType: "MANADAM",
		Element:   modelskill.ElementFire,
	})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	if mana.MAtk <= 0 || mana.MDef <= 0 || mana.TargetMaxMp <= 0 {
		t.Fatalf("ManaDamageInput non-positive values = %+v", mana)
	}
	if got, want := mana.VulnMul, 0.6; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("ManaDamageInput VulnMul = %v, want %v", got, want)
	}

	success, ok := target.SkillSuccessInput(caster, modelskill.Definition{
		SkillType:    "STUN",
		EffectType:   "STUN",
		Magic:        true,
		BaseLandRate: 50,
		Element:      modelskill.ElementFire,
	}, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	if success.BaseChance != 50 || success.Shield != formulas.ShieldPerfect {
		t.Fatalf("SkillSuccessInput base/shield = %+v", success)
	}
	if got, want := success.VulnModifier, 0.3; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("SkillSuccessInput VulnModifier = %v, want %v", got, want)
	}
	if rate := formulas.SkillSuccessRate(success); rate != 0 {
		t.Fatalf("SkillSuccessRate() = %v, want 0 for perfect shield", rate)
	}

	target.SetHP(100)
	if got := target.AddHP(25); got != 25 {
		t.Fatalf("AddHP() = %v, want 25", got)
	}
	target.ReduceHP(20.5, caster, modelskill.Definition{SkillType: "PDAM"})
	if got, want := target.HP(), 104.5; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("HP after ReduceHP = %v, want %v", got, want)
	}
	mp := target.MPValue()
	if got := target.ReduceMP(15); got != 15 {
		t.Fatalf("ReduceMP() = %v, want 15", got)
	}
	if got := target.AddMP(10); got != 10 {
		t.Fatalf("AddMP() = %v, want 10", got)
	}
	if got, want := target.MPValue(), mp-5; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("MP after ReduceMP/AddMP = %v, want %v", got, want)
	}
	if !target.CanBeHealed() || target.Invul() || target.Invulnerable() {
		t.Fatalf("healing/invulnerability flags: CanBeHealed=%v Invul=%v Invulnerable=%v", target.CanBeHealed(), target.Invul(), target.Invulnerable())
	}
	if got, want := target.HealEffectiveness(), 120.0; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("HealEffectiveness() = %v, want %v", got, want)
	}
	if got, want := target.RechargeMP(10), 15.0; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("RechargeMP() = %v, want %v", got, want)
	}

	heal, ok := caster.HealAmount(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealAmount() ok = false")
	}
	wantHeal := 25.0 + 11 + math.Sqrt(float64(int(caster.MAtk())))
	if !closeNPCFormulaFloat(heal, wantHeal) {
		t.Fatalf("HealAmount() = %v, want %v", heal, wantHeal)
	}
	static, ok := caster.HealAmount(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealAmount(static) ok = false")
	}
	if got, want := static, 36.0; !closeNPCFormulaFloat(got, want) {
		t.Fatalf("HealAmount(static) = %v, want %v", got, want)
	}
}

func closeNPCFormulaFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
