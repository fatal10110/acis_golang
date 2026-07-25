package summon

import (
	"math"
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestSummonFormulaInputsResolveStatsAndResources(t *testing.T) {
	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}
	caster := NewServitor(ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})
	target := NewPet(PetConfig{ObjectID: 2, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	owner := &struct{}{}
	caster.AddStatFuncs([]basefunc.Func{
		basefunc.NewMul(owner, stat.PvPPhysSkillDmg, 0.8, nil),
		basefunc.NewMul(owner, stat.PvPMagicalDmg, 1.3, nil),
		basefunc.NewAdd(owner, stat.HealProficiency, 11, nil),
	})
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewMul(owner, stat.FireRes, 0.36, nil),
		basefunc.NewMul(owner, stat.StunVuln, 0.5, nil),
		basefunc.NewMul(owner, stat.DaggerWpnVuln, 0.8, nil),
		basefunc.NewMul(owner, stat.RechargeMPRate, 1.5, nil),
		basefunc.NewMul(owner, stat.HealEffectiveness, 1.2, nil),
	})

	if got := target.Category(); got != skilltarget.CategoryPlayable {
		t.Fatalf("Category() = %v, want playable", got)
	}
	if !target.Playable() {
		t.Fatal("Playable() = false for a pet")
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
	if !phys.Crit {
		t.Fatal("PhysicalSkillInput Crit = false, want true with zero roll")
	}
	if got, want := phys.RandomMul, 0.95; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RandomMul = %v, want %v", got, want)
	}
	if got, want := phys.RaceMul, 1.0; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput RaceMul = %v, want %v", got, want)
	}
	if got, want := phys.PvPMul, 0.8; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput PvPMul = %v, want %v", got, want)
	}
	if got, want := phys.ElementalMul, 0.36; !closeSummonFloat(got, want) {
		t.Fatalf("PhysicalSkillInput ElementalMul = %v, want %v", got, want)
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
	if !magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = false, want true with zero roll")
	}
	if got, want := magic.PvPMul, 1.3; !closeSummonFloat(got, want) {
		t.Fatalf("MagicDamageInput PvPMul = %v, want %v", got, want)
	}
	if got, want := magic.ElementalMul, 0.36; !closeSummonFloat(got, want) {
		t.Fatalf("MagicDamageInput ElementalMul = %v, want %v", got, want)
	}

	blow, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !blow.IsPvP {
		t.Fatal("BlowInput IsPvP = false for summon-vs-pet")
	}
	if got, want := blow.RandomMul, 0.95; !closeSummonFloat(got, want) {
		t.Fatalf("BlowInput RandomMul = %v, want %v", got, want)
	}
	if got, want := blow.DaggerVulnMul, 0.8; !closeSummonFloat(got, want) {
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
	if got, want := mana.VulnMul, 0.6; !closeSummonFloat(got, want) {
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
	if got, want := success.VulnModifier, 0.3; !closeSummonFloat(got, want) {
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
	if got, want := target.HP(), 104.5; !closeSummonFloat(got, want) {
		t.Fatalf("HP after ReduceHP = %v, want %v", got, want)
	}
	mp := target.MPValue()
	if got := target.ReduceMP(15); got != 15 {
		t.Fatalf("ReduceMP() = %v, want 15", got)
	}
	if got := target.AddMP(10); got != 10 {
		t.Fatalf("AddMP() = %v, want 10", got)
	}
	if got, want := target.MPValue(), mp-5; !closeSummonFloat(got, want) {
		t.Fatalf("MP after ReduceMP/AddMP = %v, want %v", got, want)
	}
	if !target.CanBeHealed() || target.Invul() || target.Invulnerable() {
		t.Fatalf("healing/invulnerability flags: CanBeHealed=%v Invul=%v Invulnerable=%v", target.CanBeHealed(), target.Invul(), target.Invulnerable())
	}
	if got, want := target.HealEffectiveness(), 120.0; !closeSummonFloat(got, want) {
		t.Fatalf("HealEffectiveness() = %v, want %v", got, want)
	}
	if got, want := target.RechargeMP(10), 15.0; !closeSummonFloat(got, want) {
		t.Fatalf("RechargeMP() = %v, want %v", got, want)
	}

	heal, ok := caster.HealAmount(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealAmount() ok = false")
	}
	wantHeal := 25.0 + 11 + math.Sqrt(float64(int(caster.MAtk())))
	if !closeSummonFloat(heal, wantHeal) {
		t.Fatalf("HealAmount() = %v, want %v", heal, wantHeal)
	}
	static, ok := caster.HealAmount(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealAmount(static) ok = false")
	}
	if got, want := static, 36.0; !closeSummonFloat(got, want) {
		t.Fatalf("HealAmount(static) = %v, want %v", got, want)
	}
}

func zeroSummonRoll(int) int { return 0 }

func closeSummonFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
