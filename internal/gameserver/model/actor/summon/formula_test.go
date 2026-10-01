package summon

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect/effecttest"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from actor_cancel_vulnerability_test.go ----
// TestSummonCancelVulnerabilityAppliesCancelVulnStat proves CANCEL_VULN
// (Formulas.java:949-951) reaches Actor.CancelVulnerability through the live
// stat calculator — see the parallel player-package test for the downstream
// formula behavior this plumbing enables.
func TestSummonCancelVulnerabilityAppliesCancelVulnStat(t *testing.T) {
	a := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Roll: zeroSummonRoll})

	if got := a.CancelVulnerability("CANCEL"); got != 1 {
		t.Fatalf("CancelVulnerability() with no modifier = %v, want 1 (unmodified)", got)
	}

	a.AddStatFuncs([]effect.Mod{{Stat: stat.CancelVuln, Op: effect.OpMul, Value: 0.2}})
	if got := a.CancelVulnerability("CANCEL"); got != 0.2 {
		t.Fatalf("CancelVulnerability() after 0.2x modifier = %v, want 0.2", got)
	}
}

// ---- from formula_golden_test.go ----
// rawPAtk, rawPDef and rawMDef read the finalized stat before the getters
// truncate it, so the pipeline golden stays sensitive to sub-integer float
// drift (func insertion order, association) that truncation would hide.
func rawPAtk(a *Actor) float64 {
	return a.calcStat(stat.PowerAttack, positiveBase(a.combatStats().PAtk))
}

func rawPDef(a *Actor) float64 {
	return a.calcStat(stat.PowerDefence, positiveBase(a.combatStats().PDef))
}

func rawMDef(a *Actor) float64 {
	return a.calcStat(stat.MagicDefence, positiveBase(a.combatStats().MDef))
}

// goldenSummonScenarios is summon.Actor's half of the stat pipeline parity
// oracle described in issue #1527: see the player and npc packages' golden
// tests for the same shape of coverage (same-order attach-sequence
// sensitivity, Set rebasing, attach/detach round-tripping).
func goldenSummonScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}

	{
		a1 := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		a1.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = rawPDef(a1)

		a2 := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1}})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpSub, Value: 1e16}})
		a2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerDefence, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = rawPDef(a2)
	}

	{
		a := mustPet(t, PetConfig{ObjectID: 3, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		a.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = rawMDef(a)
	}

	{
		a := mustPet(t, PetConfig{ObjectID: 4, Level: 44, Stats: stats, Roll: zeroSummonRoll})
		base := rawPAtk(a)
		owner := effect.ModOwnerEffect(&effect.Effect{})
		a.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = rawPAtk(a)
		a.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = rawPAtk(a)
	}

	return out
}

func TestGoldenSummonStatPipelineCapture(t *testing.T) {
	if os.Getenv("ACIS_CAPTURE_GOLDEN") == "" {
		t.Skip("set ACIS_CAPTURE_GOLDEN=1 to (re)capture the golden fixture from the current implementation")
	}
	got := goldenSummonScenarios(t)
	writeSummonGolden(t, "testdata/golden_stats.json", got)
}

func TestGoldenSummonStatPipelineParity(t *testing.T) {
	want := readSummonGolden(t, "testdata/golden_stats.json")
	got := goldenSummonScenarios(t)
	compareSummonGolden(t, want, got)
}

func writeSummonGolden(t testing.TB, path string, values map[string]float64) {
	t.Helper()
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	bitsMap := make(map[string]uint64, len(values))
	for _, k := range keys {
		bitsMap[k] = math.Float64bits(values[k])
	}
	data, err := json.MarshalIndent(bitsMap, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write golden fixture %s: %v", path, err)
	}
}

func readSummonGolden(t testing.TB, path string) map[string]float64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden fixture %s: %v (capture it first with ACIS_CAPTURE_GOLDEN=1)", path, err)
	}
	var bitsMap map[string]uint64
	if err := json.Unmarshal(data, &bitsMap); err != nil {
		t.Fatalf("unmarshal golden fixture %s: %v", path, err)
	}
	out := make(map[string]float64, len(bitsMap))
	for k, v := range bitsMap {
		out[k] = math.Float64frombits(v)
	}
	return out
}

func compareSummonGolden(t testing.TB, want, got map[string]float64) {
	t.Helper()
	for k, w := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("golden case %q missing from current run", k)
			continue
		}
		if math.Float64bits(g) != math.Float64bits(w) {
			t.Errorf("golden case %q = %v (bits %x), want %v (bits %x)", k, g, math.Float64bits(g), w, math.Float64bits(w))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("golden case %q present in current run but not in fixture", k)
		}
	}
}

// ---- from formula_input_test.go ----
func TestSummonFormulaInputsResolveStatsAndResources(t *testing.T) {
	stats := CombatStats{
		STR: 40, CON: 21, DEX: 30, INT: 20, WIT: 43, MEN: 20,
		PAtk: 100, PDef: 50, MAtk: 64, MDef: 40,
		MaxHP: 500, MaxMP: 200, BaseRandomDamage: 5,
	}
	caster := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 44, Stats: stats, Roll: func(n int) int {
		if n == 10000 {
			return 9999
		}
		return 0
	}})
	target := mustPet(t, PetConfig{ObjectID: 2, Level: 44, Stats: stats, Roll: zeroSummonRoll})

	owner := effect.ModOwnerEffect(&effect.Effect{})
	caster.AddStatFuncs([]effect.Mod{
		{Stat: stat.PvPPhysSkillDmg, Op: effect.OpMul, Value: 0.8, Owner: owner},
		{Stat: stat.PvPMagicalDmg, Op: effect.OpMul, Value: 1.3, Owner: owner},
		{Stat: stat.HealProficiency, Op: effect.OpAdd, Value: 11, Owner: owner},
	})
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.FireRes, Op: effect.OpMul, Value: 0.36, Owner: owner},
		{Stat: stat.StunVuln, Op: effect.OpMul, Value: 0.5, Owner: owner},
		{Stat: stat.DaggerWpnVuln, Op: effect.OpMul, Value: 0.8, Owner: owner},
		{Stat: stat.RechargeMPRate, Op: effect.OpMul, Value: 1.5, Owner: owner},
		{Stat: stat.HealEffectiveness, Op: effect.OpMul, Value: 1.2, Owner: owner},
	})

	if got := target.Kind(); got != actor.KindSummon {
		t.Fatalf("Kind() = %v, want summon", got)
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
	}, true)
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

	heal, ok := caster.HealInput(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealInput() ok = false")
	}
	if heal.Scaling != formulas.HealShotScalingMage {
		t.Fatalf("summon HealInput scaling = %v, want mage", heal.Scaling)
	}
	wantHeal := 25.0 + 11 + math.Sqrt(float64(int(caster.MAtk())))
	if got := formulas.HealAmount(heal); !closeSummonFloat(got, wantHeal) {
		t.Fatalf("HealAmount() = %v, want %v", got, wantHeal)
	}
	static, ok := caster.HealInput(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealInput(static) ok = false")
	}
	if got, want := formulas.HealAmount(static), 36.0; !closeSummonFloat(got, want) {
		t.Fatalf("HealAmount(static) = %v, want %v", got, want)
	}
}

func TestSummonSkillReflectInputUsesMagicSpecificStat(t *testing.T) {
	target := mustPet(t, PetConfig{ObjectID: 1, Stats: CombatStats{}})
	refOwner := effect.ModOwnerEffect(&effect.Effect{})
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ReflectSkillMagic, Op: effect.OpSet, Value: 17, Owner: refOwner},
		{Stat: stat.ReflectSkillPhysic, Op: effect.OpSet, Value: 29, Owner: refOwner},
	})

	magic := target.SkillReflectInput(modelskill.Definition{Magic: true, CanBeReflected: true, CastRange: 900})
	if magic.ReflectChance != 17 || !magic.CanBeReflected || magic.CastRange != 900 {
		t.Fatalf("magic SkillReflectInput() = %+v", magic)
	}
	if !formulas.SkillReflects(magic, 0) {
		t.Fatal("magic SkillReflectInput() does not reflect")
	}
	physical := target.SkillReflectInput(modelskill.Definition{CanBeReflected: true, CastRange: 40, IgnoreResists: true})
	if physical.ReflectChance != 29 || !physical.IgnoreResists || !physical.CanBeReflected || physical.CastRange != 40 {
		t.Fatalf("physical SkillReflectInput() = %+v", physical)
	}
	if formulas.SkillReflects(physical, 0) {
		t.Fatal("physical SkillReflectInput() reflects despite IgnoreResists")
	}
}

func TestSummonCalcStatFloorsNonPositiveValues(t *testing.T) {
	for _, value := range []float64{0, -1} {
		a := mustPet(t, PetConfig{ObjectID: 1})
		a.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSet, Value: value}})

		if got := a.CalcStat(stat.PowerAttack, 10); got != 1 {
			t.Errorf("CalcStat(PowerAttack, %v) = %v, want 1", value, got)
		}
	}
}

func TestDeadConcurrentReduceHPAndRead(t *testing.T) {
	actor := mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 1}})
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		actor.ReduceHP(1, nil, modelskill.Definition{})
		close(done)
	}()
	<-started
	for range 1000 {
		_ = actor.Dead()
		_ = actor.AlikeDead()
	}
	<-done
}

func closeSummonFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// ---- from formula_speed_test.go ----
func TestActorPAtkSpdHungryHalvesBase(t *testing.T) {
	hungry := mustPet(t, PetConfig{ObjectID: 1, Level: 10, MaxMeal: 100, Fed: 10, HungryLimit: 0.3, Roll: zeroSummonRoll})
	fed := mustPet(t, PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})

	hungryValue := hungry.PAtkSpd(300)
	fedValue := fed.PAtkSpd(300)
	if hungryValue*2 != fedValue {
		t.Fatalf("PAtkSpd(300) hungry=%v fed=%v, want hungry == fed/2 (base halved while under-fed)", hungryValue, fedValue)
	}
}

func TestActorMAtkSpdHungryHalvesBase(t *testing.T) {
	hungry := mustPet(t, PetConfig{ObjectID: 1, Level: 10, MaxMeal: 100, Fed: 10, HungryLimit: 0.3, Roll: zeroSummonRoll})
	fed := mustPet(t, PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})

	hungryValue := hungry.MAtkSpd()
	fedValue := fed.MAtkSpd()
	if hungryValue*2 != fedValue {
		t.Fatalf("MAtkSpd() hungry=%v fed=%v, want hungry == fed/2 (base halved while under-fed)", hungryValue, fedValue)
	}
}

func TestActorCriticalRateCapsAt500(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	if got := pet.CriticalRate(600); got != 500 {
		t.Fatalf("CriticalRate(600) = %v, want capped at 500", got)
	}
}

// TestActorCriticalRateTruncatesBeforeCap pins the boundary from
// CreatureStatus.getCriticalHit (CreatureStatus.java:551-553):
// `Math.min((int) calcStat(...), 500)`. A base critical rate of 8.48
// finalizes to 84.8 (base*10, no DEX bonus for summons); truncating first
// yields the int 84, not the untruncated 84.8.
func TestActorCriticalRateTruncatesBeforeCap(t *testing.T) {
	pet := mustPet(t, PetConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	if got := pet.CriticalRate(8.48); got != 84 {
		t.Fatalf("CriticalRate(8.48) = %v, want truncated to 84", got)
	}
}

func TestActorMAtkSpdServitorNeverHalved(t *testing.T) {
	// A servitor has no feeding state at all (isPet is false), so its
	// magic attack speed must equal a well-fed pet's, never a hungry one's.
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 10, Roll: zeroSummonRoll})
	fed := mustPet(t, PetConfig{ObjectID: 2, Level: 10, MaxMeal: 100, Fed: 100, HungryLimit: 0.3, Roll: zeroSummonRoll})
	if servitor.MAtkSpd() != fed.MAtkSpd() {
		t.Fatalf("MAtkSpd() servitor=%v, want unhalved (matching a well-fed pet) %v", servitor.MAtkSpd(), fed.MAtkSpd())
	}
}

// ---- from lethal_test.go ----
type deniedLethalCaster struct{ *Actor }

func (deniedLethalCaster) CanGiveDamage() bool { return false }

func TestActorLethalSurfaceBuildsInputAndAppliesOutcomes(t *testing.T) {
	caster := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500}})
	target := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 45, Stats: CombatStats{MaxHP: 500}})
	skill := modelskill.Definition{LethalChance1: 30, LethalChance2: 10, MagicLevel: 40}

	in, ok := target.LethalInput(caster, skill)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 || in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1 {
		t.Fatalf("LethalInput() = %+v, want skill fields and 40/45/1 actor values", in)
	}

	hp := target.MaxHPValue()
	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, skill)
	if got := target.HP(); got != hp/2 {
		t.Fatalf("half lethal HP = %v, want %v", got, hp/2)
	}

	target.SetHP(hp)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, skill)
	if got := target.HP(); got != 1 {
		t.Fatalf("full lethal HP = %v, want 1", got)
	}
}

func TestActorLethalInputRejectsGuardedDamage(t *testing.T) {
	caster := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500}})
	target := mustServitor(t, ServitorConfig{ObjectID: 2, Level: 45, Stats: CombatStats{MaxHP: 500}})
	skill := modelskill.Definition{LethalChance1: 30}
	target.SetInvul(true)
	if _, ok := target.LethalInput(caster, skill); ok {
		t.Fatal("LethalInput accepted an invulnerable summon")
	}
	target.SetInvul(false)
	if _, ok := target.LethalInput(deniedLethalCaster{caster}, skill); ok {
		t.Fatal("LethalInput accepted an attacker without damage permission")
	}
}

// ---- from shots_test.go ----
func TestSummonChargedShotStateAndCounts(t *testing.T) {
	servitor := mustServitor(t, ServitorConfig{ObjectID: 1, Level: 40, Stats: CombatStats{MaxHP: 500, MaxMP: 200, SSCount: 5, SPSCount: 3}, Roll: zeroSummonRoll})

	if servitor.SoulshotCharged() || servitor.SpiritshotCharged() || servitor.BlessedSpiritshotCharged() {
		t.Fatal("new summon should carry no shot charge")
	}
	if servitor.SSCount() != 5 {
		t.Fatalf("SSCount() = %d, want 5", servitor.SSCount())
	}
	if servitor.SPSCount() != 3 {
		t.Fatalf("SPSCount() = %d, want 3", servitor.SPSCount())
	}

	servitor.SetChargedShot(item.ShotSoul, true)
	if !servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after SetChargedShot(ShotSoul, true)")
	}
	if servitor.SpiritshotCharged() || servitor.BlessedSpiritshotCharged() {
		t.Fatal("charging soulshot must not charge spiritshot kinds")
	}

	servitor.SetChargedShot(item.ShotSoul, false)
	if servitor.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = true after SetChargedShot(ShotSoul, false)")
	}
}

// ---- from status_update_test.go ----
type namedDamageAttacker struct {
	world.Presence
	effecttest.Actor
	name string
}

func (a *namedDamageAttacker) CharacterName() string { return a.name }

func TestReduceHPUpdatesStatusAfterDirectAndDOTDamage(t *testing.T) {
	for _, damage := range []struct {
		name  string
		apply func(*Actor)
	}{
		{"direct", func(a *Actor) { a.ReduceHP(10, nil, modelskill.Definition{}) }},
		{"dot", func(a *Actor) { a.ReduceHPByDOT(10, nil, true) }},
	} {
		t.Run(damage.name, func(t *testing.T) {
			a := mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}})
			rec := &event.Recorder{}
			a.Attach(Runtime{Sink: rec})

			damage.apply(a)

			if updates := event.Count[event.HPChanged](rec); updates != 1 {
				t.Fatalf("status updates = %d, want 1", updates)
			}
		})
	}
}

// TestVitalsMutatorsUpdateStatusOncePerChange pins that every HP/MP restore
// or MP payment that changes a summon's vitals republishes its status once,
// and one that changes nothing stays silent. Regeneration changes both
// resources but republishes once for the pair.
func TestVitalsMutatorsUpdateStatusOncePerChange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*Actor)
		apply func(*Actor) float64
		want  int
	}{
		{"add hp", func(a *Actor) { a.SetHP(50) }, func(a *Actor) float64 { return a.AddHP(10) }, 1},
		{"add hp at max", nil, func(a *Actor) float64 { return a.AddHP(10) }, 0},
		{"add mp", func(a *Actor) { a.reduceMP(50) }, func(a *Actor) float64 { return a.AddMP(10) }, 1},
		{"add mp at max", nil, func(a *Actor) float64 { return a.AddMP(10) }, 0},
		{"reduce mp", nil, func(a *Actor) float64 { return a.ReduceMP(10) }, 1},
		{"reduce empty mp", func(a *Actor) { a.reduceMP(200) }, func(a *Actor) float64 { return a.ReduceMP(10) }, 0},
		{"regen both", func(a *Actor) { a.SetHP(50); a.reduceMP(50) }, func(a *Actor) float64 { a.TickRegen(); return 1 }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100, MaxMP: 100}})
			if tc.setup != nil {
				tc.setup(a)
			}
			rec := &event.Recorder{}
			a.Attach(Runtime{Sink: rec})

			applied := tc.apply(a)

			if (applied > 0) != (tc.want > 0) {
				t.Fatalf("applied = %v, want a change only when %d updates are expected", applied, tc.want)
			}
			if updates := event.Count[event.HPChanged](rec); updates != tc.want {
				t.Fatalf("status updates = %d, want %d", updates, tc.want)
			}
		})
	}
}

func TestReduceHPNotifiesKnownDirectAttackerOnly(t *testing.T) {
	for _, tc := range []struct {
		name   string
		new    func() *Actor
		apply  func(*Actor, *namedDamageAttacker)
		called bool
	}{
		{"pet direct", func() *Actor { return mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker *namedDamageAttacker) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, true},
		{"servitor direct", func() *Actor { return mustServitor(t, ServitorConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker *namedDamageAttacker) { a.ReduceHP(12.9, attacker, modelskill.Definition{}) }, true},
		{"dot", func() *Actor { return mustPet(t, PetConfig{Stats: CombatStats{MaxHP: 100}}) }, func(a *Actor, attacker *namedDamageAttacker) { a.ReduceHPByDOT(12.9, attacker, true) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := tc.new()
			rec := &event.Recorder{}
			a.Attach(Runtime{Sink: rec})
			attacker := &namedDamageAttacker{name: "Attacker"}
			tc.apply(a, attacker)
			got := event.Of[event.Damaged](rec)
			if tc.called {
				if len(got) != 1 || got[0] != (event.Damaged{AttackerName: "Attacker", Damage: 12}) {
					t.Fatalf("notifications = %+v, want [{Attacker 12}]", got)
				}
			} else if len(got) != 0 {
				t.Fatalf("notifications = %d, want 0", len(got))
			}
		})
	}
}
