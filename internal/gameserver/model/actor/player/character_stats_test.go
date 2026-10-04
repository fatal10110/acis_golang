package player

import (
	"math"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type poleKnownCombatant struct {
	attackabletest.Combatant
	world.Presence
	id int32
}

func (c *poleKnownCombatant) ObjectID() int32  { return c.id }
func (*poleKnownCombatant) Kind() actor.Kind   { return actor.KindNPC }
func (c *poleKnownCombatant) SiegeGuard() bool { return false }
func (c *poleKnownCombatant) AlikeDead() bool  { return false }

func TestCharacterPoleAttackConfigAndKnownCombatants(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	live, err := creature.NewLive(location.Location{}, 0, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	live.SetQueue(idleQueue())
	c.Live = live
	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.PowerAttackRange, Op: effect.OpAdd, Value: 25},
		{Stat: stat.PowerAttackAngle, Op: effect.OpSet, Value: 150},
		{Stat: stat.AttackCountMax, Op: effect.OpSet, Value: 4},
	})

	if got := c.PhysicalAttackRange(); got != 65 {
		t.Fatalf("PhysicalAttackRange() = %d, want stat-finalized 65", got)
	}
	if got := c.PoleAttackAngle(); got != 150 {
		t.Fatalf("PoleAttackAngle() = %d, want 150", got)
	}
	if got := c.PoleAttackCountMax(); got != 4 {
		t.Fatalf("PoleAttackCountMax() = %d, want 4", got)
	}

	state := world.New()
	c.world = state
	state.Spawn(c, 0, 0, 0, 0)
	near := &poleKnownCombatant{id: 2}
	far := &poleKnownCombatant{id: 3}
	state.Spawn(near, 50, 0, 0, 0)
	state.Spawn(far, 150, 0, 0, 0)
	var known []int32
	c.ForEachKnownCombatantInRadius(100, func(candidate attackable.Combatant) {
		known = append(known, candidate.ObjectID())
	})
	if !slices.Equal(known, []int32{2}) {
		t.Fatalf("known combatants in radius = %v, want [2]", known)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1}, Type: effect.TypePolearmTargetSingle})
	if got := c.PoleAttackCountMax(); got != 1 {
		t.Fatalf("PoleAttackCountMax() with single-target marker = %d, want 1", got)
	}
}

func TestCharacterStatFuncsAffectCombatStatsAndCanBeRemoved(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 20
	tmpl.MDef = 30
	tmpl.HPRegenTable = []float64{2}
	tmpl.MPRegenTable = []float64{0.9}
	tmpl.CPRegenTable = []float64{2}
	c := liveCharacter(1, tmpl, combatItems())

	basePAtk := c.PAtk()
	basePDef := c.PDef()
	baseMAtk := c.MAtk()
	baseMDef := c.MDef()
	// The multiplier applies to the finalized (untruncated) stat; the getter
	// truncates the product.
	rawPDef := c.calcStat(stat.PowerDefence, tmpl.PDef)
	rawMDef := c.calcStat(stat.MagicDefence, tmpl.MDef)
	baseMaxHP := c.MaxHPValue()
	baseAttackSpeed := c.AttackSpeed()
	baseRunSpeed := c.RunSpeed()
	baseHPRegen := c.HPRegenRate()
	owner := effect.ModOwnerEffect(&effect.Effect{})

	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
		{Stat: stat.PowerDefence, Op: effect.OpMul, Value: 2, Owner: owner},
		{Stat: stat.MagicAttack, Op: effect.OpAdd, Value: 3, Owner: owner},
		{Stat: stat.MagicDefence, Op: effect.OpMul, Value: 2, Owner: owner},
		{Stat: stat.MaxHP, Op: effect.OpMul, Value: 2, Owner: owner},
		{Stat: stat.PowerAttackSpeed, Op: effect.OpAdd, Value: 10, Owner: owner},
		{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: 5, Owner: owner},
		{Stat: stat.RegenerateHPRate, Op: effect.OpAdd, Value: 1, Owner: owner},
	})

	if got, want := c.PAtk(), basePAtk+7; !closeFloat(got, want) {
		t.Fatalf("PAtk() with stat func = %v, want %v", got, want)
	}
	if got, want := c.PDef(), math.Trunc(rawPDef*2); got != want {
		t.Fatalf("PDef() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MAtk(), baseMAtk+3; !closeFloat(got, want) {
		t.Fatalf("MAtk() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MDef(), math.Trunc(rawMDef*2); got != want {
		t.Fatalf("MDef() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MaxHPValue(), baseMaxHP*2; !closeFloat(got, want) {
		t.Fatalf("MaxHPValue() with stat func = %v, want %v", got, want)
	}
	if got, want := c.AttackSpeed(), baseAttackSpeed+10; got != want {
		t.Fatalf("AttackSpeed() with stat func = %v, want %v", got, want)
	}
	if got, want := c.RunSpeed(), baseRunSpeed+5; !closeFloat(got, want) {
		t.Fatalf("RunSpeed() with stat func = %v, want %v", got, want)
	}
	if got, want := c.HPRegenRate(), baseHPRegen+1; !closeFloat(got, want) {
		t.Fatalf("HPRegenRate() with stat func = %v, want %v", got, want)
	}

	c.RemoveStatsByOwner(owner)

	if got := c.PAtk(); !closeFloat(got, basePAtk) {
		t.Fatalf("PAtk() after stat removal = %v, want %v", got, basePAtk)
	}
	if got := c.PDef(); !closeFloat(got, basePDef) {
		t.Fatalf("PDef() after stat removal = %v, want %v", got, basePDef)
	}
	if got := c.MAtk(); !closeFloat(got, baseMAtk) {
		t.Fatalf("MAtk() after stat removal = %v, want %v", got, baseMAtk)
	}
	if got := c.MDef(); !closeFloat(got, baseMDef) {
		t.Fatalf("MDef() after stat removal = %v, want %v", got, baseMDef)
	}
	if got := c.MaxHPValue(); !closeFloat(got, baseMaxHP) {
		t.Fatalf("MaxHPValue() after stat removal = %v, want %v", got, baseMaxHP)
	}
	if got := c.AttackSpeed(); got != baseAttackSpeed {
		t.Fatalf("AttackSpeed() after stat removal = %v, want %v", got, baseAttackSpeed)
	}
	if got := c.RunSpeed(); !closeFloat(got, baseRunSpeed) {
		t.Fatalf("RunSpeed() after stat removal = %v, want %v", got, baseRunSpeed)
	}
	if got := c.HPRegenRate(); !closeFloat(got, baseHPRegen) {
		t.Fatalf("HPRegenRate() after stat removal = %v, want %v", got, baseHPRegen)
	}
}

// TestCharacterRemoveStatsByOwnerZeroValueIsNoop pins the "unowned Mod is
// unremovable" contract carried over from the pre-#1527 Func design, where
// RemoveStatsByOwner returned early on a nil owner (a builtin's owner):
// removal keyed by the zero ModOwner must never sweep every Mod with no
// real owner.
func TestCharacterRemoveStatsByOwnerZeroValueIsNoop(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())
	basePAtk := c.PAtk()
	c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7}})
	withMod := c.PAtk()

	c.RemoveStatsByOwner(effect.ModOwner{})

	if got := c.PAtk(); !closeFloat(got, withMod) {
		t.Fatalf("PAtk() after RemoveStatsByOwner(zero value) = %v, want unchanged %v (unowned Mod must survive)", got, withMod)
	}
	if closeFloat(withMod, basePAtk) {
		t.Fatal("test setup did not actually attach a Mod (withMod == basePAtk)")
	}
}

func TestCharacterFormulaInputsResolveLiveStats(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	skill := modelskill.Definition{Power: 30, SkillType: "PDAM"}

	phys, ok := target.PhysicalSkillInput(caster, skill)
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	// P.Atk 5.4, M.Atk 13.286025 and M.Def 46.08 finalize fractional; every
	// formula input reads the truncated int.
	if got, want := phys.AttackPower, 5.0; got != want {
		t.Fatalf("PhysicalSkillInput AttackPower = %v, want %v", got, want)
	}
	if got, want := phys.SkillPower, float64(skill.Power); !closeFloat(got, want) {
		t.Fatalf("PhysicalSkillInput SkillPower = %v, want %v", got, want)
	}
	if got, want := phys.Defence, 45.0; !closeFloat(got, want) {
		t.Fatalf("PhysicalSkillInput Defence = %v, want %v", got, want)
	}
	if phys.RandomMul != 1 || phys.ElementalMul != 1 || phys.RaceMul != 1 || phys.WeaponVulnMul != 1 || phys.PvPMul != 1 {
		t.Fatalf("PhysicalSkillInput neutral multipliers = %+v", phys)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if got, want := magic.MAtk, 13.0; got != want {
		t.Fatalf("MagicDamageInput MAtk = %v, want %v", got, want)
	}
	if got, want := magic.MDef, 46.0; got != want {
		t.Fatalf("MagicDamageInput MDef = %v, want %v", got, want)
	}
	if magic.SkillPower != 40 || magic.PvPMul != 1 || magic.ElementalMul != 1 {
		t.Fatalf("MagicDamageInput = %+v", magic)
	}

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{Power: 20, SkillType: "MANADAM"})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	if got, want := mana.MAtk, 13.0; got != want {
		t.Fatalf("ManaDamageInput MAtk = %v, want %v", got, want)
	}
	if got, want := mana.MDef, 46.0; got != want {
		t.Fatalf("ManaDamageInput MDef = %v, want %v", got, want)
	}
	if got, want := mana.TargetMaxMp, 38.0; got != want {
		t.Fatalf("ManaDamageInput TargetMaxMp = %v, want %v", got, want)
	}
	if mana.SkillPower != 20 || mana.VulnMul != 1 {
		t.Fatalf("ManaDamageInput = %+v", mana)
	}

	hpRatio := caster.HP() / caster.MaxHPValue()
	fatal, ok := target.PhysicalSkillInput(caster, modelskill.Definition{Power: 100, SkillType: "FATAL"})
	if !ok {
		t.Fatal("FATAL PhysicalSkillInput() ok = false")
	}
	if got, want := fatal.SkillPower, formulas.SkillPowerFor("FATAL", 100, hpRatio); !closeFloat(got, want) {
		t.Fatalf("FATAL SkillPower at HP ratio %v = %v, want %v", hpRatio, got, want)
	}
	deathlink, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 100, SkillType: "DEATHLINK"}, true)
	if !ok {
		t.Fatal("DEATHLINK MagicDamageInput() ok = false")
	}
	if got, want := deathlink.SkillPower, formulas.SkillPowerFor("DEATHLINK", 100, hpRatio); !closeFloat(got, want) {
		t.Fatalf("DEATHLINK SkillPower at HP ratio %v = %v, want %v", hpRatio, got, want)
	}

	caster.SetHP(caster.MaxHPValue() * 0.1)
	hpRatio = caster.HP() / caster.MaxHPValue()
	fatal, ok = target.PhysicalSkillInput(caster, modelskill.Definition{Power: 100, SkillType: "FATAL"})
	if !ok {
		t.Fatal("FATAL PhysicalSkillInput() at low HP ok = false")
	}
	if got, want := fatal.SkillPower, formulas.SkillPowerFor("FATAL", 100, hpRatio); !closeFloat(got, want) {
		t.Fatalf("FATAL SkillPower at HP ratio %v = %v, want %v", hpRatio, got, want)
	}
	deathlink, ok = target.MagicDamageInput(caster, modelskill.Definition{Power: 100, SkillType: "DEATHLINK"}, true)
	if !ok {
		t.Fatal("DEATHLINK MagicDamageInput() at low HP ok = false")
	}
	if got, want := deathlink.SkillPower, formulas.SkillPowerFor("DEATHLINK", 100, hpRatio); !closeFloat(got, want) {
		t.Fatalf("DEATHLINK SkillPower at HP ratio %v = %v, want %v", hpRatio, got, want)
	}
	pdam, ok := target.PhysicalSkillInput(caster, modelskill.Definition{Power: 30, SkillType: "PDAM"})
	if !ok {
		t.Fatal("PDAM PhysicalSkillInput() at low HP ok = false")
	}
	if got, want := pdam.SkillPower, float64(30); !closeFloat(got, want) {
		t.Fatalf("PDAM SkillPower at low HP = %v, want %v", got, want)
	}
	mana, ok = target.ManaDamageInput(caster, modelskill.Definition{Power: 20, SkillType: "MANADAM"})
	if !ok {
		t.Fatal("ManaDamageInput() at low HP ok = false")
	}
	if mana.SkillPower != 20 {
		t.Fatalf("MANADAM SkillPower at low HP = %v, want 20", mana.SkillPower)
	}
}

func TestCharacterCalcStatFloorsNonPositiveValues(t *testing.T) {
	for _, value := range []float64{0, -1} {
		c := liveCharacter(1, combatTemplate(), combatItems())
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSet, Value: value}})

		if got := c.CalcStat(stat.PowerAttack, 10); got != 1 {
			t.Errorf("CalcStat(PowerAttack, %v) = %v, want 1", value, got)
		}
	}
}

func TestCharacterLethalInputAndOutcomes(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.SetResourceValues(Resources{
		MaxHP: 500, CurrentHP: 500,
		MaxMP: 30, CurrentMP: 30,
		MaxCP: 200, CurrentCP: 200,
	})
	caster.CharLevel = 40
	target.CharLevel = 45
	caster.AddStatFuncs([]effect.Mod{
		{Stat: stat.LethalRate, Op: effect.OpMul, Value: 1.5},
	})

	skill := modelskill.Definition{MagicLevel: 40, LethalChance1: 30, LethalChance2: 10}
	in, ok := target.LethalInput(caster, skill)
	if !ok {
		t.Fatal("LethalInput() ok = false")
	}
	if in.Chance1 != 30 || in.Chance2 != 10 || in.MagicLevel != 40 {
		t.Fatalf("LethalInput skill fields = %+v, want chances 30/10 and magic level 40", in)
	}
	if in.AttackerLevel != 40 || in.TargetLevel != 45 || in.LethalMul != 1.5 {
		t.Fatalf("LethalInput actor fields = %+v, want attacker 40 target 45 lethal mul 1.5", in)
	}

	target.SetHP(500)
	target.SetCP(200)
	target.ApplyLethalOutcome(formulas.LethalHalf, caster, skill)
	if target.HP() != 500 || target.CP() != 1 {
		t.Fatalf("half lethal hp/cp = %v/%v, want 500/1", target.HP(), target.CP())
	}

	target.SetHP(500)
	target.SetCP(200)
	target.ApplyLethalOutcome(formulas.LethalFull, caster, skill)
	if target.HP() != 1 || target.CP() != 1 {
		t.Fatalf("full lethal hp/cp = %v/%v, want 1/1", target.HP(), target.CP())
	}
}

// ---- from character_cancel_vulnerability_test.go ----
// TestCharacterCancelVulnerabilityAppliesCancelVulnStat proves the CANCEL_VULN
// stat (Formulas.java:949-951) reaches Character.CancelVulnerability through
// the live stat calculator, and that the result actually changes both the
// targeted Cancel skill's clamped rate (formulas.CancelSuccessRate, consumed
// by handler/skill's cancelHandler) and the CancelDebuff effect's clamped
// rate (formulas.EffectCancelDebuffSuccessRate, consumed by
// skill/effect's cancelDebuffStart) — before #1602 neither handler's roll
// ever saw a modified rate because no production actor implemented
// CancelVulnerability.
func TestCharacterCancelVulnerabilityAppliesCancelVulnStat(t *testing.T) {
	target := liveCharacter(1, combatTemplate(), combatItems())

	if got := target.CancelVulnerability("CANCEL"); got != 1 {
		t.Fatalf("CancelVulnerability() with no modifier = %v, want 1 (unmodified)", got)
	}

	target.AddStatFuncs([]effect.Mod{{Stat: stat.CancelVuln, Op: effect.OpMul, Value: 0.2, Owner: testModOwner()}})

	got := target.CancelVulnerability("CANCEL")
	if got != 0.2 {
		t.Fatalf("CancelVulnerability() after 0.2x modifier = %v, want 0.2", got)
	}

	// Targeted Cancel skill: same inputs, only vuln differs.
	baseCancelRate := formulas.CancelSuccessRate(600, 0, 50, 1, 25, 75)
	modCancelRate := formulas.CancelSuccessRate(600, 0, 50, got, 25, 75)
	if modCancelRate >= baseCancelRate {
		t.Fatalf("CancelSuccessRate with vuln=0.2 (%v) must be lower than with vuln=1 (%v)", modCancelRate, baseCancelRate)
	}

	// CancelDebuff effect: same inputs, only vuln differs.
	baseDebuffRate := formulas.EffectCancelDebuffSuccessRate(60, 40, 600, 1)
	modDebuffRate := formulas.EffectCancelDebuffSuccessRate(60, 40, 600, got)
	if modDebuffRate >= baseDebuffRate {
		t.Fatalf("EffectCancelDebuffSuccessRate with vuln=0.2 (%v) must be lower than with vuln=1 (%v)", modDebuffRate, baseDebuffRate)
	}
}

// ---- from character_grade_penalty_stats_test.go ----
func TestGradePenaltyAppliesToDependentStats(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())

	baseRunSpeed := c.RunSpeed()
	baseSwimSpeed := c.SwimSpeed()
	baseMAtkSpd := c.MagicAttackSpeed()
	baseEvasion := c.Evasion()
	baseAccuracy := c.Accuracy()

	c.armorGradePenalty = 4
	c.weaponGradePenalty = true

	wantRunSpeed := baseRunSpeed * math.Pow(0.84, 4)
	if got := c.RunSpeed(); !closeFloat(got, wantRunSpeed) {
		t.Fatalf("RunSpeed() with armor penalty 4 = %v, want %v", got, wantRunSpeed)
	}

	wantSwimSpeed := baseSwimSpeed * math.Pow(0.84, 4)
	if got := c.SwimSpeed(); !closeFloat(got, wantSwimSpeed) {
		t.Fatalf("SwimSpeed() with armor penalty 4 = %v, want %v", got, wantSwimSpeed)
	}

	wantMAtkSpd := int(float64(baseMAtkSpd) * math.Pow(0.84, 4))
	if got := c.MagicAttackSpeed(); got != wantMAtkSpd {
		t.Fatalf("MagicAttackSpeed() with armor penalty 4 = %v, want %v", got, wantMAtkSpd)
	}

	wantEvasion := baseEvasion - 8
	if got := c.Evasion(); got != wantEvasion {
		t.Fatalf("Evasion() with armor penalty 4 = %v, want %v", got, wantEvasion)
	}

	wantAccuracy := baseAccuracy - 20
	if got := c.Accuracy(); got != wantAccuracy {
		t.Fatalf("Accuracy() with weapon penalty = %v, want %v", got, wantAccuracy)
	}

	c.armorGradePenalty = 0
	c.weaponGradePenalty = false

	if got := c.RunSpeed(); !closeFloat(got, baseRunSpeed) {
		t.Fatalf("RunSpeed() with no penalty = %v, want %v", got, baseRunSpeed)
	}
	if got := c.SwimSpeed(); !closeFloat(got, baseSwimSpeed) {
		t.Fatalf("SwimSpeed() with no penalty = %v, want %v", got, baseSwimSpeed)
	}
	if got := c.MagicAttackSpeed(); got != baseMAtkSpd {
		t.Fatalf("MagicAttackSpeed() with no penalty = %v, want %v", got, baseMAtkSpd)
	}
	if got := c.Evasion(); got != baseEvasion {
		t.Fatalf("Evasion() with no penalty = %v, want %v", got, baseEvasion)
	}
	if got := c.Accuracy(); got != baseAccuracy {
		t.Fatalf("Accuracy() with no penalty = %v, want %v", got, baseAccuracy)
	}
}

// ---- from character_grade_penalty_test.go ----
func TestRefreshExpertisePenalty(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalB, Weapon: &item.WeaponDetail{}},
		{ID: 2, Kind: item.KindArmor, Slot: item.SlotFullArmor, Crystal: item.CrystalC, Armor: &item.ArmorDetail{}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotNeck, Crystal: item.CrystalC, Armor: &item.ArmorDetail{}},
		{ID: 4, Kind: item.KindEtcItem, Slot: item.SlotLHand, Crystal: item.CrystalS, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
	})
	inv := itemcontainer.NewPlayerInventory(1, templates)
	for _, id := range []int32{1, 2, 3, 4} {
		inst := inv.AddNew(id, 1, 100+id)
		tmpl, _ := templates.Get(id)
		inv.EquipItem(inst, tmpl)
	}

	c := &Character{ID: 1}
	c.AttachRuntime(nil, inv)
	c.SetSkillLevel(expertiseSkillID, 1)
	rec := recordEvents(c)

	c.RefreshExpertisePenalty()
	if got, want := c.ArmorGradePenalty(), 3; !c.WeaponGradePenalty() || got != want {
		t.Fatalf("penalty = weapon %v armor %d, want weapon true armor %d", c.WeaponGradePenalty(), got, want)
	}
	if got := c.SkillLevel(gradePenaltySkillID); got != 1 {
		t.Fatalf("grade penalty skill level = %d, want 1", got)
	}
	if updates := event.Count[event.GradePenaltyChanged](rec); updates != 1 {
		t.Fatalf("grade penalty events = %d, want 1", updates)
	}

	// Unchanged state neither re-sends packets nor reattaches item passives.
	c.RefreshExpertisePenalty()
	if updates := event.Count[event.GradePenaltyChanged](rec); updates != 1 {
		t.Fatalf("unchanged grade penalty events = %d, want 1", updates)
	}

	c.SetSkillLevel(expertiseSkillID, int(item.CrystalS))
	c.RefreshExpertisePenalty()
	if c.WeaponGradePenalty() || c.ArmorGradePenalty() != 0 || c.HasSkill(gradePenaltySkillID) {
		t.Fatalf("cleared penalty = weapon %v armor %d skill %v", c.WeaponGradePenalty(), c.ArmorGradePenalty(), c.HasSkill(gradePenaltySkillID))
	}
	if updates := event.Count[event.GradePenaltyChanged](rec); updates != 2 {
		t.Fatalf("cleared grade penalty events = %d, want 2", updates)
	}
}

// ---- from character_stats_concurrency_test.go ----
// TestCharacterStatPipelineConcurrentReadsAndMutationsAreRaceFree exercises
// #1527's concurrency requirement at the Character level: statCalcs' array
// of lazily created *effect.Calculator slots must stay safe under
// concurrent CalcStat reads (some touching a Stat for the first time,
// racing the lazy slot creation) and concurrent AddStatFuncs/
// RemoveStatsByOwner writers. Run under -race; this test asserts nothing
// about the numeric outcome, which is inherently nondeterministic here.
func TestCharacterStatPipelineConcurrentReadsAndMutationsAreRaceFree(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())
	owner := effect.ModOwnerEffect(&effect.Effect{})

	stats := []stat.Stat{stat.PowerAttack, stat.PowerDefence, stat.MagicAttack, stat.MagicDefence, stat.RunSpeed}

	var readers, writers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func(i int) {
			defer readers.Done()
			s := stats[i%len(stats)]
			for {
				select {
				case <-stop:
					return
				default:
					c.CalcStat(s, 100)
				}
			}
		}(i)
	}

	for i := 0; i < 2; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for j := 0; j < 200; j++ {
				c.AddStatFuncs([]effect.Mod{
					{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1, Owner: owner},
					{Stat: stat.MagicAttack, Op: effect.OpMul, Value: 1.1, Owner: owner},
				})
				c.RemoveStatsByOwner(owner)
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}

// TestMovementSpeedMultiplier pins CreatureStatus.getMovementSpeedMultiplier
// (CreatureStatus.java:784-790) on land: the modified move speed over the
// template run or walk speed the run mode picks. Unbuffed, it is the DEX
// run-speed bonus (FuncMoveSpeed); an armor grade penalty scales it by 0.84
// per level and a full weight penalty stops it (PlayerStatus.java:944-952).
func TestMovementSpeedMultiplier(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.RunSpeed, tmpl.WalkSpeed = 120, 80
	c := liveCharacter(1, tmpl, combatItems())
	dex := float32(statbonus.DEXBonus[tmpl.DEX])
	closeTo := func(got, want float32) bool { return math.Abs(float64(got-want)) < 1e-5 }

	if got := c.MovementSpeedMultiplier(); !closeTo(got, dex) {
		t.Fatalf("running multiplier = %v, want DEX bonus %v", got, dex)
	}
	c.SetRunning(false)
	if got := c.MovementSpeedMultiplier(); !closeTo(got, dex) {
		t.Fatalf("walking multiplier = %v, want DEX bonus %v", got, dex)
	}
	c.armorGradePenalty = 2
	if got, want := c.MovementSpeedMultiplier(), dex*0.84*0.84; !closeTo(got, want) {
		t.Fatalf("multiplier with armor penalty 2 = %v, want %v", got, want)
	}
	c.armorGradePenalty = 0
	c.weightPenalty = 4
	if got := c.MovementSpeedMultiplier(); got != 0 {
		t.Fatalf("multiplier with full weight penalty = %v, want 0", got)
	}
}

// nearSpeed compares a float32-narrowed move speed against its float64
// oracle.
func nearSpeed(got, want float64) bool { return math.Abs(got-want) < 1e-4 }

// TestMovementSpeedMultiplierInWaterAndSwamp pins
// CreatureStatus.getMovementSpeedMultiplier (CreatureStatus.java:784-790)
// over PlayerStatus.getMoveSpeed (PlayerStatus.java:931-955): in water the
// numerator is the swim speed while the base stays the run or walk speed,
// and a swamp scales the base speed by (100 + move_bonus) / 100 before the
// RUN_SPEED stat.
func TestMovementSpeedMultiplierInWaterAndSwamp(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.RunSpeed, tmpl.WalkSpeed, tmpl.SwimSpeed = 120, 80, 50
	c := liveCharacter(1, tmpl, combatItems())
	dex := statbonus.DEXBonus[tmpl.DEX]
	closeTo := func(got float32, want float64) bool { return math.Abs(float64(got)-want) < 1e-5 }

	c.SetInWater(true)
	if got, want := c.MoveSpeed(), 50*dex; !nearSpeed(got, want) {
		t.Fatalf("MoveSpeed() in water = %v, want the swim speed %v", got, want)
	}
	if got, want := c.MovementSpeedMultiplier(), 50*dex/120; !closeTo(got, want) {
		t.Fatalf("running multiplier in water = %v, want %v", got, want)
	}
	c.SetRunning(false)
	if got, want := c.MovementSpeedMultiplier(), 50*dex/80; !closeTo(got, want) {
		t.Fatalf("walking multiplier in water = %v, want %v", got, want)
	}

	c.SetInWater(false)
	c.SetRunning(true)
	c.SetSwampMoveBonus(-80)
	if got, want := c.RunSpeed(), 120*0.2*dex; !nearSpeed(got, want) {
		t.Fatalf("RunSpeed() in a -80 swamp = %v, want %v", got, want)
	}
	if got, want := c.WalkSpeed(), 80*0.2*dex; !nearSpeed(got, want) {
		t.Fatalf("WalkSpeed() in a -80 swamp = %v, want %v", got, want)
	}
	if got, want := c.MovementSpeedMultiplier(), 0.2*dex; !closeTo(got, want) {
		t.Fatalf("running multiplier in a -80 swamp = %v, want %v", got, want)
	}
	c.SetInWater(true)
	if got, want := c.MovementSpeedMultiplier(), 50*0.2*dex/120; !closeTo(got, want) {
		t.Fatalf("running multiplier swimming in a -80 swamp = %v, want %v", got, want)
	}
	c.SetInWater(false)
	c.SetSwampMoveBonus(0)
	if got := c.MovementSpeedMultiplier(); !closeTo(got, dex) {
		t.Fatalf("multiplier after leaving the swamp = %v, want DEX bonus %v", got, dex)
	}
}

// TestRunSpeedStatFuncsRefreshLiveSpeed: a RUN_SPEED func (Wind Walk's
// flat add) raises the multiplier, the live movement speed follows it, and
// the change is reported so the client can be told
// (Creature.broadcastModifiedStats, Creature.java:1243-1253).
func TestRunSpeedStatFuncsRefreshLiveSpeed(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.RunSpeed, tmpl.WalkSpeed = 120, 80
	c := attachIdleLive(t, liveCharacter(1, tmpl, combatItems()))
	rec := recordEvents(c)
	dex := statbonus.DEXBonus[tmpl.DEX]
	owner := effect.ModOwnerSkill(modelskill.Ref{ID: 1204, Level: 2})

	c.AddStatFuncs([]effect.Mod{{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: 33, Owner: owner}})
	want := 120*dex + 33
	if got := c.Move().Speed(); !nearSpeed(got, want) {
		t.Fatalf("live move speed after the buff = %v, want %v", got, want)
	}
	if got, wantMult := c.MovementSpeedMultiplier(), want/120; math.Abs(float64(got)-wantMult) > 1e-5 {
		t.Fatalf("multiplier after the buff = %v, want %v", got, wantMult)
	}
	if got := event.Count[event.RunSpeedChanged](rec); got != 1 {
		t.Fatalf("RunSpeedChanged after the buff = %d, want 1", got)
	}

	c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner}})
	if got := event.Count[event.RunSpeedChanged](rec); got != 1 {
		t.Fatalf("RunSpeedChanged after a P.Atk func = %d, want still 1", got)
	}

	c.SetRunning(false)
	if got := c.Move().Speed(); !nearSpeed(got, 80*dex+33) {
		t.Fatalf("live move speed walking = %v, want %v", got, 80*dex+33)
	}
	c.SetRunning(true)
	c.RemoveStatsByOwner(owner)
	if got := c.Move().Speed(); !nearSpeed(got, 120*dex) {
		t.Fatalf("live move speed after the buff ends = %v, want %v", got, 120*dex)
	}
	if got := event.Count[event.RunSpeedChanged](rec); got != 2 {
		t.Fatalf("RunSpeedChanged after the buff ends = %d, want 2", got)
	}
}

// TestStatFuncsReportModifiedStats pins the non-RUN_SPEED branch of
// Creature.broadcastModifiedStats (Creature.java:1213-1261): any other stat
// func change reports the self view stale, with one P.Atk./cast speed value
// per changed func on those stats, read at change time; a batch touching
// RUN_SPEED reports only the full refresh. Fist P.Atk. speed 300 * DEX 30
// bonus 1.1 = 330, * 1.33 = 438.9; cast speed 333 * WIT 11 bonus 0.64 =
// 213.12, * 1.2 = 255.74.
func TestStatFuncsReportModifiedStats(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	rec := recordEvents(c)
	might := effect.ModOwnerSkill(modelskill.Ref{ID: 1068, Level: 1})
	haste := effect.ModOwnerSkill(modelskill.Ref{ID: 1086, Level: 1})
	windWalk := effect.ModOwnerSkill(modelskill.Ref{ID: 1204, Level: 1})

	c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.2, Owner: might}})
	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.PowerAttackSpeed, Op: effect.OpMul, Value: 1.33, Owner: haste},
		{Stat: stat.MagicAttackSpeed, Op: effect.OpMul, Value: 1.2, Owner: haste},
	})
	c.RemoveStatsByOwner(haste)
	c.RemoveStatsByOwner(haste)
	c.AddStatFuncs(nil)

	got := event.Of[event.StatsModified](rec)
	want := []event.StatsModified{
		{},
		{Attrs: []event.StatusAttr{{Kind: event.StatusPhysicalSpeed, Value: 438}, {Kind: event.StatusMagicSpeed, Value: 255}}},
		{Attrs: []event.StatusAttr{{Kind: event.StatusPhysicalSpeed, Value: 330}, {Kind: event.StatusMagicSpeed, Value: 213}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StatsModified = %+v, want %+v", got, want)
	}

	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.PowerAttackSpeed, Op: effect.OpMul, Value: 1.33, Owner: windWalk},
		{Stat: stat.RunSpeed, Op: effect.OpAdd, Value: 33, Owner: windWalk},
	})
	c.RemoveStatsByOwner(windWalk)
	if n := event.Count[event.StatsModified](rec); n != len(want) {
		t.Fatalf("StatsModified after RUN_SPEED batches = %d, want still %d", n, len(want))
	}
	if n := event.Count[event.RunSpeedChanged](rec); n != 2 {
		t.Fatalf("RunSpeedChanged = %d, want 2", n)
	}
}

// TestAttackSpeedMultiplier pins CreatureStatus.getAttackSpeedMultiplier
// (CreatureStatus.java:795-801): (float) (1.1 * P.Atk. speed / 300), the
// player templates' base P.Atk. speed.
func TestAttackSpeedMultiplier(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	base := c.AttackSpeed()
	if got, want := c.AttackSpeedMultiplier(), float32(1.1*float64(base)/300); got != want {
		t.Fatalf("multiplier at P.Atk. speed %d = %v, want %v", base, got, want)
	}
	c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttackSpeed, Op: effect.OpMul, Value: 1.2}})
	hasted := c.AttackSpeed()
	if hasted == base {
		t.Fatal("P.Atk. speed func changed nothing")
	}
	if got, want := c.AttackSpeedMultiplier(), float32(1.1*float64(hasted)/300); got != want {
		t.Fatalf("multiplier at P.Atk. speed %d = %v, want %v", hasted, got, want)
	}
}
