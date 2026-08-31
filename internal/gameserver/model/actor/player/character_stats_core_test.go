package player

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type poleKnownCombatant struct {
	world.Presence
	id int32
}

func (c *poleKnownCombatant) ObjectID() int32  { return c.id }
func (c *poleKnownCombatant) SiegeGuard() bool { return false }
func (c *poleKnownCombatant) AlikeDead() bool  { return false }

func TestCharacterPoleAttackConfigAndKnownCombatants(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	live, err := creature.NewLive(location.Location{}, 0, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
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
	c.SetWorld(state)
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
	if got, want := c.PDef(), basePDef*2; !closeFloat(got, want) {
		t.Fatalf("PDef() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MAtk(), baseMAtk+3; !closeFloat(got, want) {
		t.Fatalf("MAtk() with stat func = %v, want %v", got, want)
	}
	if got, want := c.MDef(), baseMDef*2; !closeFloat(got, want) {
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
	if got, want := phys.AttackPower, 5.4; !closeFloat(got, want) {
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

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if got, want := magic.MAtk, 13.286025000000002; !closeFloat(got, want) {
		t.Fatalf("MagicDamageInput MAtk = %v, want %v", got, want)
	}
	if got, want := magic.MDef, 46.080000000000005; !closeFloat(got, want) {
		t.Fatalf("MagicDamageInput MDef = %v, want %v", got, want)
	}
	if magic.SkillPower != 40 || magic.PvPMul != 1 || magic.ElementalMul != 1 {
		t.Fatalf("MagicDamageInput = %+v", magic)
	}

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{Power: 20, SkillType: "MANADAM"})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	if got, want := mana.MAtk, 13.286025000000002; !closeFloat(got, want) {
		t.Fatalf("ManaDamageInput MAtk = %v, want %v", got, want)
	}
	if got, want := mana.MDef, 46.080000000000005; !closeFloat(got, want) {
		t.Fatalf("ManaDamageInput MDef = %v, want %v", got, want)
	}
	if got, want := mana.TargetMaxMp, 38.4; !closeFloat(got, want) {
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
	deathlink, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 100, SkillType: "DEATHLINK"})
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
	deathlink, ok = target.MagicDamageInput(caster, modelskill.Definition{Power: 100, SkillType: "DEATHLINK"})
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

// ---- from character_autoshot_test.go ----
func TestCharacterToggleAutoSoulShotAppliesItemRules(t *testing.T) {
	c := &Character{}

	if status := c.ToggleAutoSoulShot(1463, true, true, false); status != AutoSoulShotToggled {
		t.Fatalf("ToggleAutoSoulShot regular status = %v, want toggled", status)
	}
	if !c.AutoSoulShotEnabled(1463) {
		t.Fatal("regular soulshot was not enabled")
	}
	if status := c.ToggleAutoSoulShot(1463, false, true, false); status != AutoSoulShotToggled {
		t.Fatalf("ToggleAutoSoulShot disable status = %v, want toggled", status)
	}
	if c.AutoSoulShotEnabled(1463) {
		t.Fatal("regular soulshot remained enabled after disable")
	}
	if status := c.ToggleAutoSoulShot(6535, true, true, false); status != AutoSoulShotNoop {
		t.Fatalf("ToggleAutoSoulShot fishing status = %v, want noop", status)
	}
	if c.AutoSoulShotEnabled(6535) {
		t.Fatal("fishing shot was enabled")
	}
	if status := c.ToggleAutoSoulShot(6645, true, true, false); status != AutoSoulShotNeedsSummon {
		t.Fatalf("ToggleAutoSoulShot summon status = %v, want needs summon", status)
	}
	if c.AutoSoulShotEnabled(6645) {
		t.Fatal("summon shot was enabled without a summon")
	}
	if status := c.ToggleAutoSoulShot(1463, true, false, true); status != AutoSoulShotNoop {
		t.Fatalf("ToggleAutoSoulShot missing item status = %v, want noop", status)
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

// ---- from character_cast_test.go ----
// spyCastController records InterruptCastOnDamage calls and StopCast/
// InterruptCast invocations, so a test can pin exactly what a damage- or
// actor-state-driven abort trigger passes onto the live cast controller
// without depending on the real cast package (which already imports this
// one).
type spyCastController struct {
	casting   bool
	magic     bool
	abortable bool

	damageCalls []spyDamageCall
	damageBreak bool

	stopCalls      int
	interruptCalls int
}

type spyDamageCall struct {
	damage       float64
	men          int
	attackCancel float64
	roll         int
	immune       bool
}

func (s *spyCastController) CastingNow() bool          { return s.casting }
func (s *spyCastController) CurrentSkillIsMagic() bool { return s.magic }
func (s *spyCastController) InterruptCast()            { s.interruptCalls++ }
func (s *spyCastController) StopCast()                 { s.stopCalls++ }
func (s *spyCastController) CanAbortCast() bool        { return s.abortable }

func (s *spyCastController) InterruptCastOnDamage(damage float64, men int, attackCancel func(float64) float64, roll int, immune bool) bool {
	cancelled := 0.0
	if attackCancel != nil {
		cancelled = attackCancel(0)
	}
	s.damageCalls = append(s.damageCalls, spyDamageCall{damage: damage, men: men, attackCancel: cancelled, roll: roll, immune: immune})
	return s.damageBreak
}

func TestReduceHPForwardsDamageToCastController(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())
	c.SetHP(100)
	c.SetRollSource(func(int) int { return 42 })
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(30, nil, modelskill.Definition{})

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	got := spy.damageCalls[0]
	if got.damage != 30 {
		t.Fatalf("damage = %v, want 30", got.damage)
	}
	if got.men != tmpl.MEN {
		t.Fatalf("men = %d, want template MEN %d", got.men, tmpl.MEN)
	}
	if got.roll != 42 {
		t.Fatalf("roll = %d, want the injected roll source's 42", got.roll)
	}
	if got.immune {
		t.Fatal("immune = true for a non-invul character, want false")
	}
}

func TestReduceHPSkipsCastControllerOnZeroDamage(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(0, nil, modelskill.Definition{})

	if len(spy.damageCalls) != 0 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 0 for zero damage", len(spy.damageCalls))
	}
}

func TestTakeDamageForwardsDamageToCastController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	c.SetRollSource(zeroRoll)
	spy := &spyCastController{casting: true, magic: false}
	c.SetCastController(spy)

	c.TakeDamage(15, nil)

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 15 {
		t.Fatalf("damage = %v, want 15", got)
	}
}

// TestTakeDamageForwardsZeroDamageToCastController pins
// Formulas.calcCastBreak (Formulas.java:725-753), which has no damage
// guard, and CreatureAttack.java:278's unconditional
// Formulas.calcCastBreak(target, hitHolder._damage) call after a landed
// physical auto-attack: a 0-damage hit (e.g. calcPhysicalAttackDamage
// returning 0 at Formulas.java:392-396, or PDef-overkill) still rolls the
// break chance, clamped to a 1% floor, instead of being skipped.
func TestTakeDamageForwardsZeroDamageToCastController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	c.SetRollSource(zeroRoll)
	spy := &spyCastController{casting: true, magic: false}
	c.SetCastController(spy)

	c.TakeDamage(0, nil)

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1 (zero damage must still roll)", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 0 {
		t.Fatalf("damage = %v, want 0", got)
	}
}

func TestCharacterInterruptCastDelegatesToController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	spy := &spyCastController{}
	c.SetCastController(spy)

	c.InterruptCast()
	if spy.interruptCalls != 1 {
		t.Fatalf("InterruptCast delegated %d times, want 1", spy.interruptCalls)
	}

	c.StopCast()
	if spy.stopCalls != 1 {
		t.Fatalf("StopCast delegated %d times, want 1", spy.stopCalls)
	}
}

func TestCharacterCastDelegatesAreNoOpsWithoutAController(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())

	// None of these may panic on a character with no cast controller wired
	// yet (e.g. an NPC/summon actor type, or a player before its network
	// session attaches one).
	c.InterruptCast()
	c.StopCast()
	if c.CastingNow() {
		t.Fatal("CastingNow() = true with no controller wired, want false")
	}
	if c.CurrentSkillIsMagic() {
		t.Fatal("CurrentSkillIsMagic() = true with no controller wired, want false")
	}
}

func TestCharacterDieStopsInFlightCast(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(1)
	spy := &spyCastController{casting: true}
	c.SetCastController(spy)

	if !c.Die(nil) {
		t.Fatal("Die() = false on a live character, want true")
	}
	if spy.stopCalls != 1 {
		t.Fatalf("StopCast calls on death = %d, want 1", spy.stopCalls)
	}
}

// ---- from character_cc_test.go ----
type ccGeo struct{}

func (ccGeo) CanMove(_, _, _, _, _, _ int) bool { return true }
func (ccGeo) Height(_, _, _ int) int16          { return 0 }

// ccGeo never blocks in these tests, so pathfinding and fall-back queries
// never need a useful answer: return no path and reflect the origin.
func (ccGeo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (ccGeo) Walkable(int, int, int) bool                                 { return true }
func (ccGeo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

// ccFleeTarget satisfies the flee hook a Fear effect's runtime needs, so it
// activates regardless of what its actual effected actor is.
type ccFleeTarget struct{}

func (ccFleeTarget) ObjectID() int32                                    { return 0 }
func (ccFleeTarget) Dead() bool                                         { return false }
func (ccFleeTarget) FleeFrom(effector effect.Participant, distance int) {}

func attachTestLive(t *testing.T, c *Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 0, ccGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
}

func addCharacterEffect(t *testing.T, c *Character, name string) *effect.Effect {
	t.Helper()
	e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("effect.New(%q) error: %v", name, err)
	}
	e.Effected = ccFleeTarget{}
	c.EffectList().Add(e)
	return e
}

func TestCharacterCrowdControlGettersTrackActiveEffectsAndClearOnRemoval(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
		get        func(*Character) bool
	}{
		{"Stunned", "Stun", (*Character).Stunned},
		{"Rooted", "Root", (*Character).Rooted},
		{"Sleeping", "Sleep", (*Character).Sleeping},
		{"Afraid", "Fear", (*Character).Afraid},
		{"ImmobileUntilAttacked", "ImmobileUntilAttacked", (*Character).ImmobileUntilAttacked},
		{"FakeDead", "FakeDeath", (*Character).FakeDead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)

			if tt.get(c) {
				t.Fatalf("%s() = true before any effect is active", tt.name)
			}

			e := addCharacterEffect(t, c, tt.effectName)
			if !tt.get(c) {
				t.Fatalf("%s() = false with the effect active", tt.name)
			}

			c.EffectList().Remove(e)
			if tt.get(c) {
				t.Fatalf("%s() = true after the effect was removed", tt.name)
			}
		})
	}
}

func TestCharacterThrowUpEffectActivatesAndMovesToLanding(t *testing.T) {
	effector := &Character{ID: 1}
	effector.SetLastKnownPosition(location.Location{X: 100, Y: 0, Z: 0}, 0)
	attachThrowUpTestLive(t, effector)

	effected := &Character{ID: 2}
	effected.SetLastKnownPosition(location.Location{}, 0)
	attachThrowUpTestLive(t, effected)

	e, err := effect.New(effect.Skill{ID: 1, FlyRadius: 600}, modelskill.EffectTemplate{Name: "ThrowUp"})
	if err != nil {
		t.Fatal(err)
	}
	e.Effector, e.Effected = effector, effected
	effected.EffectList().Add(e)
	if !effected.Stunned() {
		t.Fatal("ThrowUp was rejected instead of applying its stunned state")
	}

	effected.EffectList().Remove(e)
	if got := effected.CurrentLocation(); got != (location.Location{X: -600, Y: 0, Z: 0}) {
		t.Fatalf("landing = %+v, want {-600 0 0}", got)
	}
}

func attachThrowUpTestLive(t *testing.T, c *Character) {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
}

func TestCharacterParalyzedUnionsManualLockAndActiveEffect(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true on a fresh character")
	}
	if !c.SetParalyzed(true) {
		t.Fatal("SetParalyzed(true) reported no change")
	}
	if !c.Paralyzed() {
		t.Fatal("Paralyzed() = false with only the manual lock set, want true (OR-union)")
	}

	c.SetParalyzed(false)
	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true after the manual lock was cleared and no effect is active")
	}

	e := addCharacterEffect(t, c, "Paralyze")
	if !c.Paralyzed() {
		t.Fatal("Paralyzed() = false with an active paralyze effect and no manual lock")
	}
	c.EffectList().Remove(e)
	if c.Paralyzed() {
		t.Fatal("Paralyzed() = true after the paralyze effect was removed")
	}
}

func TestCharacterAlikeDeadUnionsRealDeathAndFakeDeath(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.maxHP = 100
	c.curHP = 100

	if c.AlikeDead() {
		t.Fatal("AlikeDead() = true on a fresh character")
	}

	e := addCharacterEffect(t, c, "FakeDeath")
	if !c.AlikeDead() {
		t.Fatal("AlikeDead() = false with an active fake-death effect, want true")
	}

	c.EffectList().Remove(e)
	if c.AlikeDead() {
		t.Fatal("AlikeDead() = true after the fake-death effect was removed")
	}

	c.MarkDead()
	if !c.AlikeDead() {
		t.Fatal("AlikeDead() = false on a really-dead character, want true")
	}
}

func TestCharacterRecentFakeDeathTracksGracePeriodAfterMarking(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true before MarkRecentFakeDeath was ever called")
	}

	c.MarkRecentFakeDeath()
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false right after MarkRecentFakeDeath, want true")
	}

	c.recentFakeDeathUntil = time.Now().Add(-time.Second)
	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true after the grace period elapsed")
	}
}

// TestCharacterClearRecentFakeDeathCancelsGrace matches
// Player.clearRecentFakeDeath() zeroing `_recentFakeDeathEndTime`
// (Player.java:2130-2133), called unconditionally from
// PlayerAttack.doAttack (PlayerAttack.java:23) and PlayerCast.doCast
// (PlayerCast.java:184): an attack or completed cast cancels the grace
// immediately instead of letting it run out on its own.
func TestCharacterClearRecentFakeDeathCancelsGrace(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	c.MarkRecentFakeDeath()
	if !c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = false right after MarkRecentFakeDeath, want true")
	}

	c.ClearRecentFakeDeath()
	if c.RecentFakeDeath() {
		t.Fatal("RecentFakeDeath() = true after ClearRecentFakeDeath, want false")
	}
}

func TestCharacterEffectListAndCrowdControlGettersAreSafeBeforeLiveIsAttached(t *testing.T) {
	c := &Character{ID: 1}
	if c.EffectList() != nil {
		t.Fatal("EffectList() = non-nil before Live is attached")
	}
	if c.Stunned() || c.Rooted() || c.Sleeping() || c.Afraid() || c.ImmobileUntilAttacked() || c.Paralyzed() || c.Teleporting() {
		t.Fatal("a crowd-control getter reported true before Live is attached")
	}
}

func TestCharacterTeleportingReportsLiveState(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	if c.Teleporting() {
		t.Fatal("Teleporting() = true on a fresh character")
	}
	if !c.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) reported no change")
	}
	if !c.Teleporting() {
		t.Fatal("Teleporting() = false after SetTeleporting(true)")
	}
	if !c.SetTeleporting(false) {
		t.Fatal("SetTeleporting(false) reported no change")
	}
	if c.Teleporting() {
		t.Fatal("Teleporting() = true after SetTeleporting(false)")
	}
}

// ---- from character_charges_test.go ----
func TestCharacterIncreaseChargesClampsToMax(t *testing.T) {
	c := &Character{ID: 1}

	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 2 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 2, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(2, 5); !ok || c.Charges() != 4 {
		t.Fatalf("after +2: Charges() = %d, ok = %v, want 4, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(3, 5); !ok || c.Charges() != 5 {
		t.Fatalf("after +3 clamped: Charges() = %d, ok = %v, want 5, true", c.Charges(), ok)
	}
	if ok := c.IncreaseCharges(1, 5); ok || c.Charges() != 5 {
		t.Fatalf("at max: Charges() = %d, ok = %v, want 5, false", c.Charges(), ok)
	}
}

func TestCharacterIncreaseChargesNotifiesStatusOnlyAfterSuccessfulAdd(t *testing.T) {
	c := &Character{ID: 1}
	var updates int
	c.SetChargesUpdater(func() { updates++ })

	if !c.IncreaseCharges(5, 5) {
		t.Fatal("IncreaseCharges() = false, want true")
	}
	if updates != 1 {
		t.Fatalf("updates after clamped add = %d, want 1", updates)
	}
	if c.IncreaseCharges(1, 5) {
		t.Fatal("IncreaseCharges() = true at max, want false")
	}
	if updates != 1 {
		t.Fatalf("updates after at-max no-op = %d, want 1", updates)
	}
}

func TestCharacterIncreaseChargesNotifiesForceMessageBeforeStatus(t *testing.T) {
	c := &Character{ID: 1}
	var events []string
	c.SetChargeMessageSender(func(charges int, maxed bool) {
		events = append(events, fmt.Sprintf("message:%d:%t", charges, maxed))
	})
	c.SetChargesUpdater(func() { events = append(events, "status") })

	c.IncreaseCharges(2, 5)
	c.IncreaseCharges(3, 5)
	c.IncreaseCharges(1, 5)

	want := []string{"message:2:false", "status", "message:5:true", "status", "message:5:true"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("charge notifications = %v, want %v", events, want)
	}
}

func TestCharacterDecreaseChargesReportsInsufficientCharges(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(2, 5)

	if ok := c.DecreaseCharges(3); ok || c.Charges() != 2 {
		t.Fatalf("DecreaseCharges(3) over available = ok %v, Charges() %d, want false, 2", ok, c.Charges())
	}
	if ok := c.DecreaseCharges(2); !ok || c.Charges() != 0 {
		t.Fatalf("DecreaseCharges(2) = ok %v, Charges() %d, want true, 0", ok, c.Charges())
	}
}

func TestCharacterDecreaseChargesNotifiesStatusOnlyAfterSuccessfulRemoval(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(2, 5)
	var updates int
	c.SetChargesUpdater(func() { updates++ })

	if !c.DecreaseCharges(1) {
		t.Fatal("DecreaseCharges() = false, want true")
	}
	if updates != 1 {
		t.Fatalf("updates after successful removal = %d, want 1", updates)
	}
	if c.DecreaseCharges(2) {
		t.Fatal("DecreaseCharges() = true with insufficient charges, want false")
	}
	if updates != 1 {
		t.Fatalf("updates after failed removal = %d, want 1", updates)
	}
}

func TestCharacterClearChargesResetsToZero(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(4, 5)

	c.ClearCharges()

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after ClearCharges = %d, want 0", got)
	}
}

func TestCharacterClearChargesNotifiesStatusOnlyWhenChargesChange(t *testing.T) {
	c := &Character{ID: 1}
	c.IncreaseCharges(4, 5)
	var updates int
	c.SetChargesUpdater(func() { updates++ })

	c.ClearCharges()
	if updates != 1 {
		t.Fatalf("updates after clearing charges = %d, want 1", updates)
	}
	c.ClearCharges()
	if updates != 1 {
		t.Fatalf("updates after clearing empty charges = %d, want 1", updates)
	}
}

func TestCharacterDieClearsCharges(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(1)
	c.IncreaseCharges(3, 5)

	c.Die(nil)

	if got := c.Charges(); got != 0 {
		t.Fatalf("Charges() after Die = %d, want 0", got)
	}
}

// ---- from character_cubic_test.go ----
func TestCharacter_CubicListFull_DefaultCapIsOne(t *testing.T) {
	c := &Character{}
	if c.CubicListFull() {
		t.Fatal("CubicListFull() on an empty list = true, want false")
	}
	if _, added := c.AddOrRefreshCubic(cubic.Storm, false); !added {
		t.Fatal("AddOrRefreshCubic() first add reported added=false")
	}
	// With no Cubic Mastery (skill 143), size(1) > level(0): full.
	if !c.CubicListFull() {
		t.Fatal("CubicListFull() after one cubic with no mastery = false, want true")
	}
}

func TestCharacter_CubicListFull_MasteryRaisesCap(t *testing.T) {
	c := &Character{}
	c.SetSkillLevel(cubicMasterySkillID, 1)

	c.AddOrRefreshCubic(cubic.Storm, false)
	if c.CubicListFull() {
		t.Fatal("CubicListFull() after one cubic at mastery level 1 = true, want false")
	}
	c.AddOrRefreshCubic(cubic.Vampiric, false)
	if !c.CubicListFull() {
		t.Fatal("CubicListFull() after two cubics at mastery level 1 = false, want true")
	}
}

func TestCharacter_AddOrRefreshCubic_RefreshReportsNotAdded(t *testing.T) {
	c := &Character{}
	if _, added := c.AddOrRefreshCubic(cubic.Storm, false); !added {
		t.Fatal("first add reported added=false")
	}
	if touched, added := c.AddOrRefreshCubic(cubic.Storm, false); added || !touched {
		t.Fatal("re-adding the same cubic reported added=true or touched=false, want added=false, touched=true (refresh only)")
	}
}

func TestCharacter_CubicIDs_GrantOrder(t *testing.T) {
	c := &Character{}
	c.SetSkillLevel(cubicMasterySkillID, 5)
	c.AddOrRefreshCubic(cubic.Vampiric, false)
	c.AddOrRefreshCubic(cubic.Storm, false)

	want := []int{int(cubic.Vampiric), int(cubic.Storm)}
	if got := c.CubicIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CubicIDs() = %v, want %v", got, want)
	}
}

// ---- from character_death_exp_karma_test.go ----
// deathExpKarmaTable builds a two-entry level table: level 10 carries the
// penalty parameters under test, and level 11 is the sentinel entry that
// gives level 10's experience band a defined upper bound.
func deathExpKarmaTable(t *testing.T, karmaModifier, expLossAtDeath float64) *LevelTable {
	t.Helper()
	table, err := NewLevelTable(map[int]Level{
		10: {RequiredExpToLevelUp: 1000, KarmaModifier: karmaModifier, ExpLossAtDeath: expLossAtDeath},
		11: {RequiredExpToLevelUp: 3000},
	})
	if err != nil {
		t.Fatalf("NewLevelTable() error = %v", err)
	}
	return table
}

func newDeathExpKarmaCharacter(t *testing.T, karmaModifier, expLossAtDeath float64) *Character {
	c := &Character{ID: 1, CharLevel: 10, Exp: 1500, KarmaPoints: 100}
	c.SetLevelTable(deathExpKarmaTable(t, karmaModifier, expLossAtDeath))
	c.SetAllowDelevel(true)
	c.SetRateKarmaExpLost(2.0)
	return c
}

// TestApplyDeathExpKarmaLossKarmaPositive matches Player.applyDeathPenalty
// (Player.java:2896-2925) and updateKarmaLoss/calculateKarmaLost
// (Player.java:2749-2757, Formulas.java:1267-1270) for a karma-positive
// death: percentLost is scaled by RateKarmaExpLost, the resulting exp loss
// removes the reference's rounded amount, and karma drops by
// floor(lostExp / karmaModifier / 15).
func TestApplyDeathExpKarmaLossKarmaPositive(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	killer := &Character{ID: 2}

	var karmaNotified []int
	c.SetKarmaChangeNotifier(func(karma int) { karmaNotified = append(karmaNotified, karma) })
	var lossNotified [][2]int64
	c.SetExpSpLossNotifier(func(exp int64, sp int) { lossNotified = append(lossNotified, [2]int64{exp, int64(sp)}) })

	c.applyDeathExpKarmaLoss(killer)

	// span = 3000-1000 = 2000; percentLost = 10.0*2.0 = 20.0; lostExp =
	// round(2000*20/100) = 400.
	if want := int64(1100); c.Exp != want {
		t.Fatalf("Exp after death = %d, want %d", c.Exp, want)
	}
	// karmaLost = int(400/2.0/15) = 13.
	if want := 87; c.KarmaPoints != want {
		t.Fatalf("KarmaPoints after death = %d, want %d", c.KarmaPoints, want)
	}
	if len(karmaNotified) != 1 || karmaNotified[0] != 87 {
		t.Fatalf("karma-change notifications = %v, want [87]", karmaNotified)
	}
	if len(lossNotified) != 1 || lossNotified[0] != [2]int64{400, 0} {
		t.Fatalf("exp-loss notifications = %v, want [[400 0]]", lossNotified)
	}
}

// TestApplyDeathExpKarmaLossKarmaZeroSkipsRateAndKarmaLoss matches the
// reference's `if (getKarma() > 0) percentLost *= Config.RATE_KARMA_EXP_LOST`
// (Player.java:2903-2904) and updateKarmaLoss's `getKarma() > 0` guard
// (Player.java:2751): a karma-free death still loses experience at the
// unscaled percentage, and karma stays untouched.
func TestApplyDeathExpKarmaLossKarmaZeroSkipsRateAndKarmaLoss(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.KarmaPoints = 0
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	// percentLost = 10.0 (unscaled); lostExp = round(2000*10/100) = 200.
	if want := int64(1300); c.Exp != want {
		t.Fatalf("Exp after death = %d, want %d", c.Exp, want)
	}
	if c.KarmaPoints != 0 {
		t.Fatalf("KarmaPoints after karma-free death = %d, want 0", c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossKarmaFloorsAtZero matches setKarma's
// `Math.max(0, karma)` clamp (Player.java:1073).
func TestApplyDeathExpKarmaLossKarmaFloorsAtZero(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.KarmaPoints = 5
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.KarmaPoints != 0 {
		t.Fatalf("KarmaPoints after over-large loss = %d, want 0 (floored)", c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossNoKillerIsNoOp matches the reference's
// `if (killer != null)` guard around the whole penalty (Player.java:2615):
// an environmental death costs nothing.
func TestApplyDeathExpKarmaLossNoKillerIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)

	c.applyDeathExpKarmaLoss(nil)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("nil-killer death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossDelevelDisabledIsNoOp matches the caller-side
// `Config.ALLOW_DELEVEL && ...` gate (Player.java:2650): with the config
// off, no death ever applies the penalty regardless of killer or karma.
func TestApplyDeathExpKarmaLossDelevelDisabledIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.SetAllowDelevel(false)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("delevel-disabled death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
}

// TestApplyDeathExpKarmaLossLuckySkillBelowTenIsNoOp and
// TestApplyDeathExpKarmaLossLuckySkillAtTenApplies match
// `!hasSkill(SKILL_LUCKY) || getStatus().getLevel() > 9` (Player.java:2650):
// the Lucky skill exempts a death below level 10, but not from level 10 on.
func TestApplyDeathExpKarmaLossLuckySkillBelowTenIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.CharLevel = 9
	c.SetSkillLevel(int(modelskill.LuckySkillID), 1)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp != 1500 || c.KarmaPoints != 100 {
		t.Fatalf("Lucky-skill sub-10 death changed state: Exp=%d KarmaPoints=%d, want unchanged (1500, 100)", c.Exp, c.KarmaPoints)
	}
}

func TestApplyDeathExpKarmaLossLuckySkillAtTenApplies(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.SetSkillLevel(int(modelskill.LuckySkillID), 1)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.Exp == 1500 {
		t.Fatalf("Lucky-skill level-10 death left Exp unchanged, want the penalty applied")
	}
}

// TestApplyDeathExpKarmaLossSiegeZoneHalvesPercent matches the reference's
// `if (... || isInsideZone(ZoneId.SIEGE)) percentLost /= 4.0`
// (Player.java:2906-2907).
func TestApplyDeathExpKarmaLossSiegeZoneHalvesPercent(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	c.KarmaPoints = 0 // isolate the siege-zone quarter from the karma-rate multiplier.
	c.SetInSiegeZone(true)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	// percentLost = 10.0/4.0 = 2.5; lostExp = round(2000*2.5/100) = 50.
	if want := int64(1450); c.Exp != want {
		t.Fatalf("Exp after siege-zone death = %d, want %d", c.Exp, want)
	}
}

// TestApplyDeathExpKarmaLossSnapshotsExpBeforeDeath matches
// Player.applyDeathPenalty's `setExpBeforeDeath(getStatus().getExp())`
// (Player.java:2919): the pre-loss exp is recorded, not the post-loss exp.
func TestApplyDeathExpKarmaLossSnapshotsExpBeforeDeath(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	killer := &Character{ID: 2}

	c.applyDeathExpKarmaLoss(killer)

	if c.ExpBeforeDeath != 1500 {
		t.Fatalf("ExpBeforeDeath = %d, want 1500 (the pre-loss exp)", c.ExpBeforeDeath)
	}
	if c.Exp != 1100 {
		t.Fatalf("Exp after death = %d, want 1100", c.Exp)
	}
}

// TestRestoreExpAddsPercentOfLostExpAndClearsSnapshot matches
// Player.restoreExp (Player.java:2865-2872).
func TestRestoreExpAddsPercentOfLostExpAndClearsSnapshot(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	killer := &Character{ID: 2}
	c.applyDeathExpKarmaLoss(killer) // Exp: 1500 -> 1100, ExpBeforeDeath = 1500.

	var gained []int64
	c.SetExpSpGainNotifier(func(exp int64, sp int) { gained = append(gained, exp) })

	c.RestoreExp(50)

	// restored = round((1500-1100)*50/100) = 200.
	if want := int64(1300); c.Exp != want {
		t.Fatalf("Exp after RestoreExp(50) = %d, want %d", c.Exp, want)
	}
	if c.ExpBeforeDeath != 0 {
		t.Fatalf("ExpBeforeDeath after RestoreExp = %d, want 0", c.ExpBeforeDeath)
	}
	if len(gained) != 1 || gained[0] != 200 {
		t.Fatalf("exp-gain notifications = %v, want [200]", gained)
	}
}

// TestRestoreExpNoDeathIsNoOp matches restoreExp's
// `if (getExpBeforeDeath() > 0)` guard (Player.java:2867): a character that
// never died has nothing to restore.
func TestRestoreExpNoDeathIsNoOp(t *testing.T) {
	c := newDeathExpKarmaCharacter(t, 2.0, 10.0)

	c.RestoreExp(100)

	if c.Exp != 1500 {
		t.Fatalf("Exp after no-op RestoreExp = %d, want unchanged 1500", c.Exp)
	}
}

// TestDieAwardsKillerKarmaBeforeApplyingVictimsOwnDeathPenalty matches the
// reference's ordering: Playable.doDie (Playable.java:178-183) runs
// onKillUpdatePvPKarma — the source of awardKillerPKKarma/awardKillerPvPKill
// — while the victim's karma is still untouched, and only Player.doDie's
// later applyDeathPenalty call (Player.java:2650) reduces it
// (updateKarmaLoss, Player.java:2921-2925). Character.Die must therefore run
// the killer-reward checks before applyDeathExpKarmaLoss, not after: a
// small-positive-karma victim (3) must still block the "innocent, karma-free
// victim" PK-karma award even though this same death floors that karma to 0
// a moment later.
func TestDieAwardsKillerKarmaBeforeApplyingVictimsOwnDeathPenalty(t *testing.T) {
	victim := newDeathExpKarmaCharacter(t, 2.0, 10.0)
	victim.KarmaPoints = 3
	killer := &Character{ID: 2}

	if !victim.Die(killer) {
		t.Fatal("Die() = false, want true")
	}

	if killer.PKKills != 0 || killer.KarmaPoints != 0 {
		t.Fatalf("killer = (PKKills=%d, KarmaPoints=%d), want (0, 0): PK-karma award must read the victim's pre-penalty karma", killer.PKKills, killer.KarmaPoints)
	}
	if victim.KarmaPoints != 0 {
		t.Fatalf("victim.KarmaPoints after death = %d, want 0 (own death penalty still floors it)", victim.KarmaPoints)
	}
}

// ---- from character_deathpenalty_test.go ----
func TestCharacterSetDeathPenaltyLevelClampsToBounds(t *testing.T) {
	c := &Character{ID: 1}

	c.SetDeathPenaltyLevel(-3)
	if got := c.DeathPenaltyLevel(); got != 0 {
		t.Fatalf("SetDeathPenaltyLevel(-3): DeathPenaltyLevel() = %d, want 0", got)
	}

	c.SetDeathPenaltyLevel(999)
	if got := c.DeathPenaltyLevel(); got != maxDeathPenaltyLevel {
		t.Fatalf("SetDeathPenaltyLevel(999): DeathPenaltyLevel() = %d, want %d", got, maxDeathPenaltyLevel)
	}

	c.SetDeathPenaltyLevel(7)
	if got := c.DeathPenaltyLevel(); got != 7 {
		t.Fatalf("SetDeathPenaltyLevel(7): DeathPenaltyLevel() = %d, want 7", got)
	}
}

func TestCharacterReduceDeathPenaltyLevelDecrementsToFloorZero(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(2)

	if got := c.ReduceDeathPenaltyLevel(); got != 1 {
		t.Fatalf("first ReduceDeathPenaltyLevel() = %d, want 1", got)
	}
	if got := c.ReduceDeathPenaltyLevel(); got != 0 {
		t.Fatalf("second ReduceDeathPenaltyLevel() = %d, want 0", got)
	}
	if got := c.ReduceDeathPenaltyLevel(); got != 0 {
		t.Fatalf("ReduceDeathPenaltyLevel() at zero = %d, want unchanged 0", got)
	}
}

// TestCharacterReduceDeathPenaltyLevelFiresReducedUpdater matches the
// reference's reduceDeathPenaltyBuffLevel (Player.java:6544-6553): the
// packet-layer notification fires with the resulting level on every actual
// decrement (so the caller can tell the S1_ADDED reapply case, level > 0,
// from the LIFTED case, level == 0), and does not fire on the no-op at
// zero (Player.java:6537-6538).
func TestCharacterReduceDeathPenaltyLevelFiresReducedUpdater(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(1)

	var got []int
	c.SetDeathPenaltyReducedUpdater(func(level int) { got = append(got, level) })

	c.ReduceDeathPenaltyLevel() // 1 -> 0: reapply-message branch never fires, LIFTED does.
	c.ReduceDeathPenaltyLevel() // already 0: no-op, updater must not fire again.

	if want := []int{0}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("reduced-updater calls = %v, want %v", got, want)
	}
}

func TestCharacterDeathPenaltySkillUpdaterReplacesLevelOnEachChange(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyChance(100)

	var got [][2]int
	c.SetDeathPenaltySkillUpdater(func(oldLevel, newLevel int) {
		got = append(got, [2]int{oldLevel, newLevel})
	})

	c.RaiseDeathPenaltyLevel(nil, 1)
	c.RaiseDeathPenaltyLevel(nil, 1)
	c.ReduceDeathPenaltyLevel()
	c.ReduceDeathPenaltyLevel()
	c.ReduceDeathPenaltyLevel()

	want := [][2]int{{0, 1}, {1, 2}, {2, 1}, {1, 0}}
	if len(got) != len(want) {
		t.Fatalf("death-penalty skill updates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("death-penalty skill update %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// deathPenaltyKiller is a minimal killer double: a non-Player actor whose
// raid-relation is fixed at construction.
type deathPenaltyKiller struct {
	raidRelated bool
}

func (k deathPenaltyKiller) RaidRelated() bool { return k.raidRelated }

func TestRaiseDeathPenaltyLevelCappedAtMax(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyLevel(maxDeathPenaltyLevel)

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 1); changed || got != maxDeathPenaltyLevel {
		t.Fatalf("RaiseDeathPenaltyLevel() at cap = (%d, %v), want (%d, false)", got, changed, maxDeathPenaltyLevel)
	}
}

func TestRaiseDeathPenaltyLevelRejectsPlayerKiller(t *testing.T) {
	c := &Character{ID: 1}
	killer := &Character{ID: 2}
	c.KarmaPoints = 1000 // karma alone would otherwise pass the gate

	if got, changed := c.RaiseDeathPenaltyLevel(killer, 1); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(playerKiller) = (%d, %v), want (0, false)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelNoKarmaFailsChanceRoll(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyChance(20)

	// roll above the configured chance, no karma: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 21); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(highRoll) = (%d, %v), want (0, false)", got, changed)
	}
	// roll at or below the configured chance, no karma: passes.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 20); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(lowRoll) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelUsesConfiguredChance(t *testing.T) {
	c := &Character{ID: 1}
	c.SetDeathPenaltyChance(0)

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 1); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(configured chance) = (%d, %v), want (0, false)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelKarmaBypassesChanceRoll(t *testing.T) {
	c := &Character{ID: 1}
	c.KarmaPoints = 1

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(karma, highRoll) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelExemptsPvPAndSiegeZones(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Character)
	}{
		{name: "pvp", set: func(c *Character) { c.SetInPvPZone(true) }},
		{name: "siege", set: func(c *Character) { c.SetInSiegeZone(true) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1, KarmaPoints: 1}
			tt.set(c)

			if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100); changed || got != 0 {
				t.Fatalf("RaiseDeathPenaltyLevel() = (%d, %v), want (0, false)", got, changed)
			}
		})
	}
}

// TestRaiseDeathPenaltyLevelFiresRaisedUpdaterOnlyOnPass matches the
// reference's calculateDeathPenaltyBuffLevel (Player.java:6518-6528): the
// packet-layer notification fires with the new level only when the gate
// passes, never on a blocked attempt.
func TestRaiseDeathPenaltyLevelFiresRaisedUpdaterOnlyOnPass(t *testing.T) {
	c := &Character{ID: 1}
	c.KarmaPoints = 1

	var got []int
	c.SetDeathPenaltyRaisedUpdater(func(level int) { got = append(got, level) })

	c.RaiseDeathPenaltyLevel(&Character{ID: 2}, 100) // blocked: player killer, must not fire.
	c.RaiseDeathPenaltyLevel(deathPenaltyKiller{}, 100)

	if want := []int{1}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("raised-updater calls = %v, want %v", got, want)
	}
}

func TestRaiseDeathPenaltyLevelCharmOfLuckBlocksUnidentifiedOrRaidKiller(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.KarmaPoints = 1
	addCharacterEffect(t, c, "CharmOfLuck")

	// nil killer: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(nil, 100); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(charmOfLuck, nilKiller) = (%d, %v), want (0, false)", got, changed)
	}
	// raid-related killer: blocked.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{raidRelated: true}, 100); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(charmOfLuck, raidKiller) = (%d, %v), want (0, false)", got, changed)
	}
	// non-raid, identified killer: passes.
	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{raidRelated: false}, 100); !changed || got != 1 {
		t.Fatalf("RaiseDeathPenaltyLevel(charmOfLuck, mundaneKiller) = (%d, %v), want (1, true)", got, changed)
	}
}

func TestRaiseDeathPenaltyLevelPhoenixBlessingBlocksAlways(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	c.KarmaPoints = 1
	addCharacterEffect(t, c, "PhoenixBless")

	if got, changed := c.RaiseDeathPenaltyLevel(deathPenaltyKiller{raidRelated: false}, 100); changed || got != 0 {
		t.Fatalf("RaiseDeathPenaltyLevel(phoenixBlessed) = (%d, %v), want (0, false)", got, changed)
	}
}

// ---- from character_dot_test.go ----
var (
	_ interface {
		Dead() bool
		HP() float64
		ReduceHPByDOT(float64, effect.Participant, bool)
	} = (*Character)(nil)
	_ interface {
		Dead() bool
		MPValue() float64
		ReduceMP(float64) float64
	} = (*Character)(nil)
)

func TestDamageOverTimeEffectTargetsCharacterAndBroadcastsStatus(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	statusUpdates := 0
	c.SetStatusBroadcaster(func() { statusUpdates++ })
	before := c.HP()

	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "DamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.HP(), before-4; got != want {
		t.Fatalf("HP() = %v, want %v", got, want)
	}
	if statusUpdates != 1 {
		t.Fatalf("status updates = %d, want 1", statusUpdates)
	}
}

// TestManaDamageOverTimeEffectTargetsCharacter pins Finding 2 of the #1088
// closed-PR review: a mana-DOT tick must broadcast a status update carrying
// MP, matching PlayerStatus.broadcastStatusUpdate()'s unconditional CUR_MP
// inclusion (EffectManaDamOverTime.java:35 -> CreatureStatus.reduceMp/setMp,
// CreatureStatus.java:338-355, 274-306 -> the Player override at
// PlayerStatus.java:408-416, which sends CUR_HP+CUR_MP+CUR_CP on every
// call, unlike the generic HP-only, threshold-gated broadcast the base
// Creature/Npc path uses). The generic statusBroadcaster hook is HP-only on
// the wire (network/targeting.go's targetHPAttributes), so this must go
// through the separate MP-carrying broadcaster, not the HP one.
func TestManaDamageOverTimeEffectTargetsCharacter(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}
	mpStatusUpdates := 0
	c.SetMPStatusBroadcaster(func() { mpStatusUpdates++ })
	before := c.MPValue()
	e, err := effect.New(effect.Skill{ID: 1}, skill.EffectTemplate{Name: "ManaDamOverTime", Value: 4})
	if err != nil {
		t.Fatalf("effect.New() error: %v", err)
	}
	e.Effected = c
	if !e.ActionTime() {
		t.Fatal("ActionTime() = false, want true")
	}
	if got, want := c.MPValue(), before-4; got != want {
		t.Fatalf("MPValue() = %v, want %v", got, want)
	}
	if mpStatusUpdates != 1 {
		t.Fatalf("MP status updates = %d, want 1", mpStatusUpdates)
	}
}

// ---- from character_dotnotice_test.go ----
func TestNotifyEffectRemovedDueLackHPAndMP(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "dot", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}

	hpNotices, mpNotices, relaxNotices := 0, 0, 0
	c.SetLackHPNotifier(func() { hpNotices++ })
	c.SetLackMPNotifier(func() { mpNotices++ })
	c.SetRelaxHPFullNotifier(func() { relaxNotices++ })

	c.NotifyEffectRemovedDueLackHP(nil)
	c.NotifyEffectRemovedDueLackMP(nil)
	c.NotifyRelaxDeactivatedHPFull(nil)
	if hpNotices != 1 {
		t.Fatalf("hp notices = %d, want 1", hpNotices)
	}
	if mpNotices != 1 {
		t.Fatalf("mp notices = %d, want 1", mpNotices)
	}
	if relaxNotices != 1 {
		t.Fatalf("relax notices = %d, want 1", relaxNotices)
	}

	// Unwiring the hook (as lifecycle detach does) must not panic.
	c.SetLackHPNotifier(nil)
	c.SetLackMPNotifier(nil)
	c.SetRelaxHPFullNotifier(nil)
	c.NotifyEffectRemovedDueLackHP(nil)
	c.NotifyEffectRemovedDueLackMP(nil)
}

func TestNotifyMagicFailureHooks(t *testing.T) {
	c, err := NewCharacter(1, humanFighterTemplate(), "acct", "mage", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() error: %v", err)
	}

	failed, resisted, magic := 0, 0, 0
	var resistedName, magicName string
	var resistedID modelskill.ID
	var resistedLevel int
	c.SetMagicFailureNotifiers(func() { failed++ }, func(name string, id modelskill.ID, level int) {
		resisted++
		resistedName, resistedID, resistedLevel = name, id, level
	}, func(name string) {
		magic++
		magicName = name
	})

	c.NotifyAttackFailed()
	c.NotifyResistedSkill("Victim", 1419, 1)
	c.NotifyResistedMagic("Mage")
	if failed != 1 {
		t.Fatalf("attack-failed notices = %d, want 1", failed)
	}
	if resisted != 1 || resistedName != "Victim" || resistedID != 1419 || resistedLevel != 1 {
		t.Fatalf("resisted-skill notice = %d %q %d/%d, want 1 Victim 1419/1", resisted, resistedName, resistedID, resistedLevel)
	}
	if magic != 1 || magicName != "Mage" {
		t.Fatalf("resisted-magic notice = %d %q, want 1 Mage", magic, magicName)
	}

	c.SetMagicFailureNotifiers(nil, nil, nil)
	c.NotifyAttackFailed()
	c.NotifyResistedSkill("Victim", 1419, 1)
	c.NotifyResistedMagic("Mage")
}

// ---- from character_forces_test.go ----
// permissiveGeo is a test-only move.Geo that permits every move, needed
// only because creature.NewLive requires a non-nil Geo.
type permissiveGeo struct{}

func (permissiveGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool { return true }
func (permissiveGeo) Height(x, y, z int) int16                { return int16(z) }
func (permissiveGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return nil, false
}
func (permissiveGeo) Walkable(int, int, int) bool { return true }
func (permissiveGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

func withEffectList(t *testing.T, c *Character) *Character {
	t.Helper()
	live, err := creature.NewLive(location.Location{}, 100, permissiveGeo{}, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Live = live
	return c
}

func TestCharacterSeedPowerReadsActiveSeedEffectLevel(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))

	if got := c.SeedPower(1285); got != 0 {
		t.Fatalf("SeedPower(1285) uncharged = %d, want 0", got)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 1285}, Level: 4, Type: effect.TypeBuff})

	if got := c.SeedPower(1285); got != 4 {
		t.Fatalf("SeedPower(1285) charged = %d, want 4", got)
	}
}

func TestCharacterForceLevelReadsActiveForceEffectLevel(t *testing.T) {
	c := withEffectList(t, liveCharacter(1, combatTemplate(), combatItems()))

	if level, ok := c.ForceLevel(5104); ok || level != 0 {
		t.Fatalf("ForceLevel(5104) inactive = (%d, %v), want (0, false)", level, ok)
	}

	c.EffectList().Add(&effect.Effect{Skill: effect.Skill{ID: 5104}, Level: 2, Type: effect.TypeBuff})

	if level, ok := c.ForceLevel(5104); !ok || level != 2 {
		t.Fatalf("ForceLevel(5104) active = (%d, %v), want (2, true)", level, ok)
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
	updates, refreshes := 0, 0
	c.SetGradePenaltyUpdater(func() { updates++ })
	c.SetItemStatsRefresher(func() { refreshes++ })

	c.RefreshExpertisePenalty()
	if got, want := c.ArmorGradePenalty(), 3; !c.WeaponGradePenalty() || got != want {
		t.Fatalf("penalty = weapon %v armor %d, want weapon true armor %d", c.WeaponGradePenalty(), got, want)
	}
	if got := c.SkillLevel(gradePenaltySkillID); got != 1 {
		t.Fatalf("grade penalty skill level = %d, want 1", got)
	}
	if updates != 1 || refreshes != 1 {
		t.Fatalf("hooks = updates %d refreshes %d, want 1 each", updates, refreshes)
	}

	// Unchanged state neither re-sends packets nor reattaches item passives.
	c.RefreshExpertisePenalty()
	if updates != 1 || refreshes != 1 {
		t.Fatalf("unchanged hooks = updates %d refreshes %d, want 1 each", updates, refreshes)
	}

	c.SetSkillLevel(expertiseSkillID, int(item.CrystalS))
	c.RefreshExpertisePenalty()
	if c.WeaponGradePenalty() || c.ArmorGradePenalty() != 0 || c.HasSkill(gradePenaltySkillID) {
		t.Fatalf("cleared penalty = weapon %v armor %d skill %v", c.WeaponGradePenalty(), c.ArmorGradePenalty(), c.HasSkill(gradePenaltySkillID))
	}
	if updates != 2 || refreshes != 2 {
		t.Fatalf("cleared hooks = updates %d refreshes %d, want 2 each", updates, refreshes)
	}
}

// ---- from character_herb_test.go ----
// TestConsumeHerbReportsWhetherAConsumerTookIt pins the result a herb
// deliverer needs: a detached character consumes nothing, and saying so lets
// the caller deliver the herb another way instead of discarding it.
func TestConsumeHerbReportsWhetherAConsumerTookIt(t *testing.T) {
	c := &Character{ID: 1}

	if c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = true with no consumer wired")
	}

	var consumed []int32
	c.SetHerbConsumer(func(itemID int32) { consumed = append(consumed, itemID) })
	if !c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = false with a consumer wired")
	}
	if len(consumed) != 1 || consumed[0] != 8600 {
		t.Fatalf("consumed = %v, want [8600]", consumed)
	}

	c.SetHerbConsumer(nil)
	if c.ConsumeHerb(8600) {
		t.Fatal("ConsumeHerb() = true after detach")
	}
	if len(consumed) != 1 {
		t.Fatalf("consumed = %v, want no further consumption after detach", consumed)
	}
}

// TestAddRewardItemNotifiesTheUpdateHook pins the delivery half of an
// auto-looted kill reward: the mutation methods stay silent because they also
// serve client requests, so this server-driven caller is the one that has to
// register the inventory with the batching task.
func TestAddRewardItemNotifiesTheUpdateHook(t *testing.T) {
	templates := item.NewTable([]*item.Template{
		{ID: 57, Name: "adena", Kind: item.KindEtcItem, Stackable: true, EtcItem: &item.EtcItemDetail{}},
	})
	c := &Character{ID: 1}
	inv := itemcontainer.RestorePlayerInventory(c.ID, templates, nil)
	c.AttachRuntime(&Template{}, inv)

	notified := 0
	inv.SetUpdateNotifier(func() { notified++ })

	if !c.AddRewardItem(57, 10, 0x30000001) {
		t.Fatal("AddRewardItem() = false for a known stackable template")
	}
	if notified != 1 {
		t.Fatalf("notifier calls = %d, want 1", notified)
	}

	if c.AddRewardItem(9999, 1, 0x30000002) {
		t.Fatal("AddRewardItem() = true for an unknown template")
	}
	if notified != 1 {
		t.Fatalf("notifier calls after a rejected add = %d, want 1", notified)
	}
}

// ---- from character_melee_dot_cp_test.go ----
// TestTakeDamageDrainsCPBeforeHPForPlayableAttacker pins melee auto-attack
// (CreatureAttack.java:263 -> PlayerStatus.reduceHp, PlayerStatus.java:166-184)
// to the same CP-first absorption ReduceHP already applies to skill-cast
// damage (#1143).
func TestTakeDamageDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	defender := liveCharacter(1, combatTemplate(), combatItems())
	defender.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	defender.TakeDamage(50, attacker)

	if defender.HP() != 500 {
		t.Fatalf("HP() = %v, want 500 (fully absorbed by CP)", defender.HP())
	}
	if defender.CP() != 150 {
		t.Fatalf("CP() = %v, want 150", defender.CP())
	}
}

func TestTakeDamageSpillsOverToHPOnceCPExhausted(t *testing.T) {
	defender := liveCharacter(1, combatTemplate(), combatItems())
	defender.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	defender.TakeDamage(50, attacker)

	if defender.CP() != 0 {
		t.Fatalf("CP() = %v, want 0", defender.CP())
	}
	if defender.HP() != 480 {
		t.Fatalf("HP() = %v, want 480 (20 dmg after CP absorbed 30)", defender.HP())
	}
}

func TestTakeDamageSkipsCPAbsorptionForSelfAttacker(t *testing.T) {
	defender := liveCharacter(1, combatTemplate(), combatItems())
	defender.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	defender.TakeDamage(50, defender)

	if defender.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for self-attacker", defender.CP())
	}
	if defender.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", defender.HP())
	}
}

// TestReduceHPByDOTDrainsCPBeforeHPForPlayableAttacker pins DOT damage
// (EffectDamOverTime.java:48 -> PlayerStatus.reduceHp) to the same CP-first
// absorption, not gated on isDOT (#1143).
func TestReduceHPByDOTDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	c.ReduceHPByDOT(50, attacker, true)

	if c.HP() != 500 {
		t.Fatalf("HP() = %v, want 500 (fully absorbed by CP)", c.HP())
	}
	if c.CP() != 150 {
		t.Fatalf("CP() = %v, want 150", c.CP())
	}
}

func TestReduceHPByDOTSpillsOverToHPOnceCPExhausted(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})
	attacker := liveCharacter(2, combatTemplate(), combatItems())

	c.ReduceHPByDOT(50, attacker, true)

	if c.CP() != 0 {
		t.Fatalf("CP() = %v, want 0", c.CP())
	}
	if c.HP() != 480 {
		t.Fatalf("HP() = %v, want 480 (20 dmg after CP absorbed 30)", c.HP())
	}
}

func TestReduceHPByDOTSkipsCPAbsorptionForNonPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHPByDOT(50, reduceHPNpcAttacker{}, true)

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for non-Playable attacker", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

// ---- from character_mount_test.go ----
func TestCharacterMountWyvernTracksControlItemAndNotifies(t *testing.T) {
	c := &Character{}
	if !c.Mount(12621, 77) {
		t.Fatal("Mount() = false, want true")
	}
	if got := c.MountType(); got != 2 {
		t.Fatalf("MountType() = %d, want 2", got)
	}
	if got := c.MountNPCID(); got != 12621 {
		t.Fatalf("MountNPCID() = %d, want 12621", got)
	}
	if got := c.MountObjectID(); got != 77 {
		t.Fatalf("MountObjectID() = %d, want 77", got)
	}
}

// ---- from character_pvpflag_test.go ----
func TestUpdatePvPFlagRefreshesUserInfoOnlyOnChange(t *testing.T) {
	c := &Character{ID: 1}
	updates := 0
	c.SetUserInfoUpdater(func() { updates++ })

	c.UpdatePvPFlag(task.PvPFlagOn)
	if got := c.PvPFlagState(); got != task.PvPFlagOn {
		t.Fatalf("PvPFlagState() = %v, want PvPFlagOn", got)
	}
	if updates != 1 {
		t.Fatalf("updates after first change = %d, want 1", updates)
	}

	c.UpdatePvPFlag(task.PvPFlagOn)
	if updates != 1 {
		t.Fatalf("updates after unchanged state = %d, want 1 (no-op)", updates)
	}

	c.UpdatePvPFlag(task.PvPFlagBlinking)
	if updates != 2 {
		t.Fatalf("updates after second change = %d, want 2", updates)
	}
}

func TestUpdatePvPFlagBroadcastsRelationsOnlyOnChange(t *testing.T) {
	c := &Character{ID: 1}
	broadcasts := 0
	c.SetRelationBroadcaster(func() { broadcasts++ })

	c.UpdatePvPFlag(task.PvPFlagOn)
	if broadcasts != 1 {
		t.Fatalf("broadcasts after first change = %d, want 1", broadcasts)
	}

	c.UpdatePvPFlag(task.PvPFlagOn)
	if broadcasts != 1 {
		t.Fatalf("broadcasts after unchanged state = %d, want 1 (no-op)", broadcasts)
	}

	c.UpdatePvPFlag(task.PvPFlagBlinking)
	if broadcasts != 2 {
		t.Fatalf("broadcasts after second change = %d, want 2", broadcasts)
	}
}

func TestUpdatePvPFlagNoopWithoutRelationBroadcaster(t *testing.T) {
	c := &Character{ID: 1}

	// Should not panic when no hook is wired.
	c.UpdatePvPFlag(task.PvPFlagOn)
}

func TestNotePvPHitFromAttackerFlagsInnocentVictimHit(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.notePvPHitFromAttacker(attacker)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls = %v, want [false] (normal duration)", calls)
	}
}

func TestNotePvPHitFromAttackerSkipsMutualPvPZone(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	attacker.SetInPvPZone(true)
	victim.SetInPvPZone(true)
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	victim.notePvPHitFromAttacker(attacker)

	if called {
		t.Fatal("hook fired for two PvP-zone players")
	}
}

type pvpFlagNPC struct{ guard bool }

func (pvpFlagNPC) ObjectID() int32                { return 4 }
func (pvpFlagNPC) Category() skilltarget.Category { return skilltarget.CategoryAttackable }
func (n pvpFlagNPC) Guard() bool                  { return n.guard }

func TestNotePvPHitFromAttackerUsesFlaggedDurationForOngoingPvPFight(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	victim.pvpFlag = task.PvPFlagOn
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.notePvPHitFromAttacker(attacker)

	if len(calls) != 1 || calls[0] != true {
		t.Fatalf("hook calls = %v, want [true] (PvP-vs-PvP duration)", calls)
	}
}

func TestNotePvPHitFromAttackerUsesNormalDurationWhenAttackerHasKarma(t *testing.T) {
	attacker := &Character{ID: 1, KarmaPoints: 500}
	victim := &Character{ID: 2}
	victim.pvpFlag = task.PvPFlagOn
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.notePvPHitFromAttacker(attacker)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls = %v, want [false]: a karma'd attacker never gets the PvP-vs-PvP duration", calls)
	}
}

func TestNotePvPHitFromAttackerSkipsWhenVictimHasKarma(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2, KarmaPoints: 500}
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	victim.notePvPHitFromAttacker(attacker)

	if called {
		t.Fatal("hook fired, want no-op: hitting a karma'd (PK) victim never flags the attacker")
	}
}

func TestNotePvPHitFromAttackerSkipsNonPlayerAttacker(t *testing.T) {
	victim := &Character{ID: 2}

	// Should not panic and should be a no-op for a non-*Character attacker.
	victim.notePvPHitFromAttacker(npcKiller{id: 99})
}

func TestNotePvPHitFromAttackerSkipsNilAttacker(t *testing.T) {
	victim := &Character{ID: 2}

	victim.notePvPHitFromAttacker(nil)
}

func TestNotePvPHitFromAttackerSkipsSelfHit(t *testing.T) {
	c := &Character{ID: 1}
	called := false
	c.SetPvPFlagHook(func(bool) { called = true })

	c.notePvPHitFromAttacker(c)

	if called {
		t.Fatal("hook fired, want no-op: self-damage never flags the actor")
	}
}

func TestNotePvPHitFromAttackerNoopWithoutHook(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}

	// Should not panic when no hook is wired (e.g. character not attached
	// to a live session).
	victim.notePvPHitFromAttacker(attacker)
}

func TestCharacterNotePvPAttackFlagsInnocentVictim(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	attacker.NotePvPAttack(victim)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls after NotePvPAttack = %v, want [false]", calls)
	}
}

func TestCharacterNotePvPAttackFlagsOwnerOfSummonedTarget(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	attacker.NotePvPAttack(summonKiller{owner: victim})

	if len(calls) != 1 || calls[0] {
		t.Fatalf("hook calls after summoned target = %v, want [false]", calls)
	}
}

func TestCharacterNotePvPSkillTargetsFlagsEligibleNonOffensiveTargets(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	flagged := liveCharacter(2, tmpl, items)
	flagged.UpdatePvPFlag(task.PvPFlagOn)
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	attacker.NotePvPSkillTargets([]creature.DeathActor{flagged, pvpFlagNPC{}}, false, "DUMMY")

	if len(calls) != 2 || calls[0] || calls[1] {
		t.Fatalf("hook calls after NotePvPSkillTargets = %v, want [false false]", calls)
	}
}

func TestCharacterNotePvPSkillTargetsFlagsOwnerOfFlaggedSummon(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	attacker.NotePvPSkillTargets([]creature.DeathActor{summonKiller{owner: victim}}, false, "DUMMY")

	if !called {
		t.Fatal("non-offensive cast at a flagged summon did not flag its owner")
	}
}

// TestCharacterReduceHPByDOTDoesNotFlagAttacker documents the deliberate
// scope cut: a DOT tick continues a skill cast whose own initial hit
// already flagged the attacker in the reference (CreatureCast.java calls
// updatePvPStatus once, at cast time, not per DOT tick), so periodic damage
// must not re-trigger the flag hook here.
func TestCharacterReduceHPByDOTDoesNotFlagAttacker(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	victim.ReduceHPByDOT(10, attacker, true)

	if called {
		t.Fatal("hook fired on ReduceHPByDOT, want no-op: DOT ticks don't re-flag the attacker")
	}
}

// ---- from character_reducehp_cc_test.go ----
// reduceHPPlayableAttacker is a minimal Playable-attacker stub for CP
// absorption tests (a distinct actor from the target, unlike self-damage).
type reduceHPPlayableAttacker struct{}

func (reduceHPPlayableAttacker) ObjectID() int32 { return 99 }
func (reduceHPPlayableAttacker) Dead() bool      { return false }
func (reduceHPPlayableAttacker) Playable() bool  { return true }

// reduceHPNpcAttacker is a non-Playable attacker stub.
type reduceHPNpcAttacker struct{}

func (reduceHPNpcAttacker) ObjectID() int32 { return 98 }
func (reduceHPNpcAttacker) Dead() bool      { return false }
func (reduceHPNpcAttacker) Playable() bool  { return false }

func TestReduceHPDrainsCPBeforeHPForPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{})

	if c.HP() != 500 {
		t.Fatalf("HP() = %v, want 500 (fully absorbed by CP)", c.HP())
	}
	if c.CP() != 150 {
		t.Fatalf("CP() = %v, want 150", c.CP())
	}
}

func TestReduceHPSpillsOverToHPOnceCPExhausted(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 30})

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{})

	if c.CP() != 0 {
		t.Fatalf("CP() = %v, want 0", c.CP())
	}
	if c.HP() != 480 {
		t.Fatalf("HP() = %v, want 480 (20 dmg after CP absorbed 30)", c.HP())
	}
}

func TestReduceHPSkipsCPAbsorptionForNonPlayableAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, reduceHPNpcAttacker{}, modelskill.Definition{})

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

func TestReduceHPSkipsCPAbsorptionForSelfAttacker(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, c, modelskill.Definition{})

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for self-attacker (matches PlayerStatus.java's attacker != _actor gate)", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

func TestReduceHPSkipsCPAbsorptionForDirectHPDamageSkill(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{DirectHPDamage: true})

	if c.CP() != 200 {
		t.Fatalf("CP() = %v, want 200 unchanged for a dmgDirectlyToHp skill (matches PlayerStatus.reduceHp's ignoreCP=skill.getDmgDirectlyToHP())", c.CP())
	}
	if c.HP() != 450 {
		t.Fatalf("HP() = %v, want 450", c.HP())
	}
}

// TestReduceHPBreaksCastOnRawDamageNotCPAbsorbedRemainder pins
// Formulas.calcCastBreak's contract: every Java call site passes the skill's
// raw computed damage, never a CP-reduced remainder. A fully CP-absorbed hit
// (HP untouched) must still forward the full raw damage to the cast
// controller.
func TestReduceHPBreaksCastOnRawDamageNotCPAbsorbedRemainder(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetResourceValues(Resources{MaxHP: 500, CurrentHP: 500, MaxCP: 200, CurrentCP: 200})
	c.SetRollSource(func(int) int { return 42 })
	spy := &spyCastController{casting: true, magic: true}
	c.SetCastController(spy)

	c.ReduceHP(50, reduceHPPlayableAttacker{}, modelskill.Definition{})

	if len(spy.damageCalls) != 1 {
		t.Fatalf("InterruptCastOnDamage calls = %d, want 1", len(spy.damageCalls))
	}
	if got := spy.damageCalls[0].damage; got != 50 {
		t.Fatalf("damage = %v, want 50 (raw damage, not the CP-absorbed remainder of 0)", got)
	}
}

// ---- from character_regenmax_test.go ----
func TestCharacterIsPlayer(t *testing.T) {
	if _, ok := any(&Character{}).(interface{ IsPlayer() bool }); !ok {
		t.Fatal("Character does not identify itself as a player to effects")
	}
}

// ---- from character_state_test.go ----
func TestCharacterSpawnProtectionMakesItInvulnerable(t *testing.T) {
	c := &Character{}
	if c.Invul() {
		t.Fatal("Invul() = true without protection")
	}
	c.SetSpawnProtection(true)
	if !c.SpawnProtected() || !c.Invul() {
		t.Fatal("spawn protection did not make the character invulnerable")
	}
	c.SetSpawnProtection(false)
	if c.SpawnProtected() || c.Invul() {
		t.Fatal("cleared spawn protection left the character invulnerable")
	}
}

func TestCharacterStopFakeDeathDoesNotBroadcastAfterDeath(t *testing.T) {
	c := &Character{ID: 1}
	c.SetStanding(false)
	stances, revives := 0, 0
	c.SetStanceBroadcaster(func(Stance) { stances++ })
	c.SetFakeDeathReviveBroadcaster(func() { revives++ })
	if !c.MarkDead() {
		t.Fatal("MarkDead() = false, want true")
	}

	if c.StopFakeDeath() {
		t.Fatal("StopFakeDeath() = true for a dead character, want false")
	}
	if stances != 0 || revives != 0 {
		t.Fatalf("dead fake-death exit broadcasts = stances:%d revives:%d, want none", stances, revives)
	}
}

func TestCharacterAllSkillsDisabledUnionsCrowdControlStates(t *testing.T) {
	tests := []struct {
		name       string
		effectName string
	}{
		{"Stunned", "Stun"},
		{"Sleeping", "Sleep"},
		{"Afraid", "Fear"},
		{"ImmobileUntilAttacked", "ImmobileUntilAttacked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1}
			attachTestLive(t, c)

			if c.AllSkillsDisabled() {
				t.Fatal("AllSkillsDisabled() = true before any lock is active")
			}

			e := addCharacterEffect(t, c, tt.effectName)
			if !c.AllSkillsDisabled() {
				t.Fatalf("AllSkillsDisabled() = false with %s active, want true", tt.effectName)
			}

			c.EffectList().Remove(e)
			if c.AllSkillsDisabled() {
				t.Fatalf("AllSkillsDisabled() = true after %s was removed", tt.effectName)
			}
		})
	}

	t.Run("Paralyzed", func(t *testing.T) {
		c := &Character{ID: 1}
		attachTestLive(t, c)

		c.SetParalyzed(true)
		if !c.AllSkillsDisabled() {
			t.Fatal("AllSkillsDisabled() = false with Paralyzed lock set, want true")
		}
		c.SetParalyzed(false)
		if c.AllSkillsDisabled() {
			t.Fatal("AllSkillsDisabled() = true after the paralyze lock was cleared")
		}
	})
}

func TestCharacterItemDisabledConsultsAllSkillsDisabledOnlyWhenAnItemIsAlreadyTracked(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	e := addCharacterEffect(t, c, "Stun")
	if c.ItemDisabled(1) {
		t.Fatal("ItemDisabled() = true while stunned but no item is tracked as disabled, want false (matches Java's isItemDisabled emptiness short-circuit)")
	}

	c.DisableItem(2, time.Minute)
	if !c.ItemDisabled(1) {
		t.Fatal("ItemDisabled() = false for an untracked id while stunned and another item is disabled, want true")
	}

	c.EffectList().Remove(e)
	if c.ItemDisabled(1) {
		t.Fatal("ItemDisabled() = true for an untracked id once the stun lock clears")
	}
	if !c.ItemDisabled(2) {
		t.Fatal("ItemDisabled() = false for the item still inside its own disable window")
	}
}

func TestCharacterSkillDisabledConsultsAllSkillsDisabledOnlyWhenASkillIsAlreadyTracked(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)

	e := addCharacterEffect(t, c, "Stun")
	if c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = true while stunned but no skill is on cooldown, want false (matches Java's isSkillDisabled emptiness short-circuit)")
	}

	c.DisableSkill(2, time.Minute)
	if !c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = false for an untracked key while stunned and another skill is on cooldown, want true")
	}

	c.EffectList().Remove(e)
	if c.SkillDisabled(1) {
		t.Fatal("SkillDisabled() = true for an untracked key once the stun lock clears")
	}
	if !c.SkillDisabled(2) {
		t.Fatal("SkillDisabled() = false for the skill still inside its own reuse window")
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

// ---- from character_stats_damage_effects_test.go ----
// TestReduceHPStopsSleepAndImmobileUntilAttackedEffects mirrors
// PlayerStatus.reduceHp's unconditional stopEffects(SLEEP)/
// stopEffects(IMMOBILE_UNTIL_ATTACKED) calls on non-HP-consumption damage.
func TestReduceHPStopsSleepAndImmobileUntilAttackedEffects(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	addCharacterEffect(t, c, "ImmobileUntilAttacked")

	c.ReduceHP(10, nil, modelskill.Definition{})

	if c.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHP, want the sleep effect stopped")
	}
	if c.ImmobileUntilAttacked() {
		t.Fatal("ImmobileUntilAttacked() = true after ReduceHP, want the effect stopped")
	}
}

// TestReduceHPStandsUpSittingCharacterUnlessInStoreMode mirrors the
// reference's isSitting() && !isInStoreMode() standUp() gate.
func TestReduceHPStandsUpSittingCharacterUnlessInStoreMode(t *testing.T) {
	tests := []struct {
		name        string
		operating   bool
		wantStanded bool
	}{
		{"stands up out of store mode", false, true},
		{"stays seated in store mode", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			c.Sit()
			c.SetOperating(tt.operating)

			c.ReduceHP(10, nil, modelskill.Definition{})

			if got := c.Standing(); got != tt.wantStanded {
				t.Fatalf("Standing() = %v, want %v", got, tt.wantStanded)
			}
		})
	}
}

// TestReduceHPBreaksStunOnOneInTenRollForNonDOTDamage mirrors
// !isDOT && isStunned() && Rnd.get(10) == 0.
func TestReduceHPBreaksStunOnOneInTenRollForNonDOTDamage(t *testing.T) {
	tests := []struct {
		name      string
		roll      int
		wantStun  bool
		wantAfter bool
	}{
		{"winning roll breaks stun", 0, true, false},
		{"losing roll leaves stun active", 1, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := liveCharacter(1, combatTemplate(), combatItems())
			c.SetHP(100)
			attachTestLive(t, c)
			addCharacterEffect(t, c, "Stun")
			c.SetRollSource(func(int) int { return tt.roll })

			c.ReduceHP(10, nil, modelskill.Definition{})

			if got := c.Stunned(); got != tt.wantAfter {
				t.Fatalf("Stunned() = %v, want %v", got, tt.wantAfter)
			}
		})
	}
}

// TestReduceHPByDOTNeverBreaksStunEvenOnWinningRoll mirrors the reference's
// !isDOT gate for a real damage-over-time skill tick (isDOT=true, e.g.
// Poison/Bleed): it stops SLEEP/IMMOBILE_UNTIL_ATTACKED and stands the
// character up like any other hit, but never rolls to break STUN.
func TestReduceHPByDOTNeverBreaksStunEvenOnWinningRoll(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Stun")
	addCharacterEffect(t, c, "Sleep")
	c.SetRollSource(func(int) int { return 0 })

	c.ReduceHPByDOT(10, nil, true)

	if !c.Stunned() {
		t.Fatal("Stunned() = false after ReduceHPByDOT(isDOT=true), want STUN untouched on a real DOT tick")
	}
	if c.Sleeping() {
		t.Fatal("Sleeping() = true after ReduceHPByDOT, want the sleep effect stopped")
	}
}

// TestReduceHPByDOTBreaksStunWhenNotARealDOTTick covers drowning's exact
// reference call (WaterTaskManager.java: reduceCurrentHp(hp, player, false,
// false, null), isDOT=false): periodic non-attack damage that still allows
// the 1-in-10 STUN-break roll, unlike a real DOT skill tick.
func TestReduceHPByDOTBreaksStunWhenNotARealDOTTick(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Stun")
	c.SetRollSource(func(int) int { return 0 })

	c.ReduceHPByDOT(10, nil, false)

	if c.Stunned() {
		t.Fatal("Stunned() = true after ReduceHPByDOT(isDOT=false) with a winning roll, want STUN broken")
	}
}

// TestReduceHPSkipsDamageEffectsOnAlreadyDeadCharacter mirrors the
// reference's top-of-method isDead() early return: an already-dead
// character (curHP already clamped to 0) must not have its SLEEP effect
// stopped or get stood up by a stray hit landing after death.
func TestReduceHPSkipsDamageEffectsOnAlreadyDeadCharacter(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(0)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.ReduceHP(10, nil, modelskill.Definition{})

	if !c.Sleeping() {
		t.Fatal("Sleeping() = false after ReduceHP on an already-dead character, want the sleep effect untouched")
	}
	if c.Standing() {
		t.Fatal("Standing() = true after ReduceHP on an already-dead character, want it left seated")
	}
}

// TestReduceHPByDOTSkipsDamageEffectsOnAlreadyDeadCharacter is
// ReduceHPByDOT's counterpart to the above.
func TestReduceHPByDOTSkipsDamageEffectsOnAlreadyDeadCharacter(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(0)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.ReduceHPByDOT(10, nil, true)

	if !c.Sleeping() {
		t.Fatal("Sleeping() = false after ReduceHPByDOT on an already-dead character, want the sleep effect untouched")
	}
	if c.Standing() {
		t.Fatal("Standing() = true after ReduceHPByDOT on an already-dead character, want it left seated")
	}
}

// TestTakeDamageAppliesNonConsumptionDamageEffects covers the melee-hit
// entrypoint, which reuses the same non-DOT hook as ReduceHP.
func TestTakeDamageAppliesNonConsumptionDamageEffects(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.TakeDamage(10, nil)

	if c.Sleeping() {
		t.Fatal("Sleeping() = true after TakeDamage, want the sleep effect stopped")
	}
	if !c.Standing() {
		t.Fatal("Standing() = false after TakeDamage, want the character stood up")
	}
}

// TestTakeDamageSkipsDamageEffectsOnZeroDamage covers a zero-damage hit
// (currently unreachable from attack.Controller.deliverHit, which filters
// hit.Damage <= 0 before calling TakeDamage, but TakeDamage is exported and
// exercised directly by other tests): it must not wake, unstun, or stand the
// character up, matching the existing convention ReduceHP/ReduceHPByDOT
// already use for a non-positive amount.
func TestTakeDamageSkipsDamageEffectsOnZeroDamage(t *testing.T) {
	c := liveCharacter(1, combatTemplate(), combatItems())
	c.SetHP(100)
	attachTestLive(t, c)
	addCharacterEffect(t, c, "Sleep")
	c.Sit()

	c.TakeDamage(0, nil)

	if !c.Sleeping() {
		t.Fatal("Sleeping() = false after a zero-damage TakeDamage, want the sleep effect untouched")
	}
	if c.Standing() {
		t.Fatal("Standing() = true after a zero-damage TakeDamage, want it left seated")
	}
}

// ---- from character_stats_damage_test.go ----
func TestCharacterSkillDamageInputsUseElementalSkillModifier(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]effect.Mod{{Stat: stat.FireRes, Op: effect.OpMul, Value: 0.75, Owner: testModOwner()}})

	phys, ok := target.PhysicalSkillInput(caster, modelskill.Definition{Power: 30, SkillType: "PDAM", Element: modelskill.ElementFire})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !closeFloat(phys.ElementalMul, 0.75) {
		t.Fatalf("PhysicalSkillInput ElementalMul = %v, want 0.75", phys.ElementalMul)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Element: modelskill.ElementFire})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !closeFloat(magic.ElementalMul, 0.75) {
		t.Fatalf("MagicDamageInput ElementalMul = %v, want 0.75", magic.ElementalMul)
	}

	neutral, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(neutral) ok = false")
	}
	if !closeFloat(neutral.ElementalMul, 1) {
		t.Fatalf("neutral MagicDamageInput ElementalMul = %v, want 1", neutral.ElementalMul)
	}
}

func TestCharacterMagicDamageInputRollsMagicCritical(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())

	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			return 9999
		}
		return 7
	})
	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = false, want true for roll below mCrit rate")
	}

	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			return 9999
		}
		return 8
	})
	magic, ok = target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() second call ok = false")
	}
	if magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = true, want false for roll at mCrit rate")
	}
}

func TestCharacterBlowInputUsesTargetRelativeHeading(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)

	caster.SetLastKnownPosition(location.Location{X: -80, Y: 0, Z: 0}, 0)
	behind, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput(behind) ok = false")
	}
	if !closeFloat(behind.PosMul, 1.1) {
		t.Fatalf("behind BlowInput PosMul = %v, want 1.1", behind.PosMul)
	}

	caster.SetLastKnownPosition(location.Location{X: 0, Y: 80, Z: 0}, 0)
	side, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput(side) ok = false")
	}
	if !closeFloat(side.PosMul, 1.025) {
		t.Fatalf("side BlowInput PosMul = %v, want 1.025", side.PosMul)
	}

	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	front, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput(front) ok = false")
	}
	if !closeFloat(front.PosMul, 1) {
		t.Fatalf("front BlowInput PosMul = %v, want 1", front.PosMul)
	}
}

func TestCharacterBlowInputCarriesResolvedLandingRoll(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.DEX = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	caster.SetRollSource(func(n int) int {
		if n == 11 {
			return 5
		}
		if n != 1000 {
			t.Fatalf("blow roll bound = %d, want 1000", n)
		}
		return 799
	})

	in, ok := target.BlowInput(caster, modelskill.Definition{ID: 1, Power: 30, SkillType: "BLOW", BaseLandRate: 1000, BaseCritRate: 100})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !in.Landed {
		t.Fatal("BlowInput().Landed = false, want true for a capped ordinary blow")
	}
	if !in.Crit {
		t.Fatal("BlowInput().Crit = false, want true for a successful critical roll")
	}
}

func TestCharacterBlowInputCarriesPhysicalSkillEvasion(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]effect.Mod{{Stat: stat.PSkillEvasion, Op: effect.OpSet, Value: 1, Owner: testModOwner()}})
	caster.SetRollSource(func(n int) int {
		if n != 1000 {
			t.Fatalf("blow roll bound = %d, want 1000", n)
		}
		return 0
	})
	target.SetRollSource(func(n int) int {
		if n != 100 {
			t.Fatalf("skill-evasion roll bound = %d, want 100", n)
		}
		return 0
	})

	in, ok := target.BlowInput(caster, modelskill.Definition{SkillType: "BLOW", BaseLandRate: 1000})
	if !ok || !in.Landed || !in.Evaded {
		t.Fatalf("BlowInput() = %+v, %v; want landed evasion", in, ok)
	}
}

func TestCharacterBlowInputCarriesShieldDefense(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 100, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()},
		{Stat: stat.ShieldDefence, Op: effect.OpSet, Value: 30, Owner: testModOwner()},
	})
	target.SetRollSource(func(n int) int {
		if n != 100 {
			t.Fatalf("shield roll bound = %d, want 100", n)
		}
		return 10
	})

	in, ok := target.BlowInput(caster, modelskill.Definition{SkillType: "BLOW", BaseLandRate: 1000})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if in.Shield != formulas.ShieldSuccess {
		t.Fatalf("BlowInput shield = %v, want ShieldSuccess", in.Shield)
	}
	if want := target.PDef() + target.CalcStat(stat.ShieldDefence, 0); !closeFloat(in.Defence, want) {
		t.Fatalf("BlowInput defence = %v, want %v", in.Defence, want)
	}
}

func TestCharacterPhysicalSkillInputCarriesShieldDefense(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 100, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()},
		{Stat: stat.ShieldDefence, Op: effect.OpSet, Value: 30, Owner: testModOwner()},
	})

	for _, tt := range []struct {
		name    string
		roll    int
		shield  formulas.ShieldDefense
		defence float64
	}{
		{"success", 10, formulas.ShieldSuccess, target.PDef() + target.CalcStat(stat.ShieldDefence, 0)},
		{"perfect", 0, formulas.ShieldPerfect, target.PDef()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target.SetRollSource(func(n int) int {
				if n != 100 {
					t.Fatalf("shield roll bound = %d, want 100", n)
				}
				return tt.roll
			})
			in, ok := target.PhysicalSkillInput(caster, modelskill.Definition{SkillType: "PDAM"})
			if !ok {
				t.Fatal("PhysicalSkillInput() ok = false")
			}
			if in.Shield != tt.shield {
				t.Fatalf("PhysicalSkillInput shield = %v, want %v", in.Shield, tt.shield)
			}
			if !closeFloat(in.Defence, tt.defence) {
				t.Fatalf("PhysicalSkillInput defence = %v, want %v", in.Defence, tt.defence)
			}
		})
	}
}

func TestCharacterBlowInputSkipsShieldRollOnMiss(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetRollSource(func(n int) int {
		if n == 1000 {
			return 999
		}
		return 0
	})
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 100, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()},
	})
	target.SetRollSource(func(n int) int {
		t.Fatalf("shield roll bound = %d, want no shield roll after a miss", n)
		return 0
	})

	in, ok := target.BlowInput(caster, modelskill.Definition{SkillType: "BLOW", BaseLandRate: 1000})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if in.Landed {
		t.Fatal("BlowInput().Landed = true, want false")
	}
	if in.Shield != formulas.ShieldFailed {
		t.Fatalf("BlowInput shield = %v, want ShieldFailed", in.Shield)
	}
}

func TestCharacterDamageInputsRejectInvulnerableTargetAndNoDamagePermission(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	def := modelskill.Definition{Power: 100, Magic: true}

	target.SetSpawnProtection(true)
	if _, ok := target.PhysicalSkillInput(caster, def); ok {
		t.Fatal("PhysicalSkillInput accepted an invulnerable target")
	}
	if _, ok := target.MagicDamageInput(caster, def); ok {
		t.Fatal("MagicDamageInput accepted an invulnerable target")
	}
	if _, ok := target.BlowInput(caster, def); ok {
		t.Fatal("BlowInput accepted an invulnerable target")
	}
	if _, ok := target.ManaDamageInput(caster, def); ok {
		t.Fatal("ManaDamageInput accepted an invulnerable target")
	}

	target.SetSpawnProtection(false)
	caster.SetCanGiveDamage(false)
	if _, ok := target.PhysicalSkillInput(caster, def); ok {
		t.Fatal("PhysicalSkillInput accepted an attacker without damage permission")
	}
	if _, ok := target.MagicDamageInput(caster, def); ok {
		t.Fatal("MagicDamageInput accepted an attacker without damage permission")
	}
	if _, ok := target.BlowInput(caster, def); ok {
		t.Fatal("BlowInput accepted an attacker without damage permission")
	}
	if _, ok := target.ManaDamageInput(caster, def); ok {
		t.Fatal("ManaDamageInput accepted an attacker without damage permission")
	}
}

func TestCharacterLethalInputRejectsAttackerWithoutDamagePermission(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	caster.SetCanGiveDamage(false)
	if _, ok := target.LethalInput(caster, modelskill.Definition{LethalChance1: 100}); ok {
		t.Fatal("LethalInput accepted an attacker without damage permission")
	}
	caster.SetCanGiveDamage(true)
	target.SetSpawnProtection(true)
	if _, ok := target.LethalInput(caster, modelskill.Definition{LethalChance1: 100}); ok {
		t.Fatal("LethalInput accepted an invulnerable target")
	}
}

func TestCharacterReduceHPIgnoresInvulnerableTargetAndNoDamagePermission(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.SetHP(100)
	target.SetSpawnProtection(true)
	target.ReduceHP(10, caster, modelskill.Definition{})
	if got := target.HP(); got != 100 {
		t.Fatalf("HP after invulnerable damage = %v, want 100", got)
	}

	target.SetSpawnProtection(false)
	caster.SetCanGiveDamage(false)
	target.ReduceHP(10, caster, modelskill.Definition{})
	if got := target.HP(); got != 100 {
		t.Fatalf("HP after denied attacker damage = %v, want 100", got)
	}
}

func TestCharacterDamageInputsUseChargedShots(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	items := combatItems()
	soulWeapon := &item.Instance{
		ObjectID: 10, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
		ShotsMask: item.ShotSoul.Mask(),
	}
	soulCaster := liveCharacter(1, tmpl, items, soulWeapon)
	target := liveCharacter(2, tmpl, items)

	phys, ok := target.PhysicalSkillInput(soulCaster, modelskill.Definition{Power: 30, SkillType: "PDAM", SoulShotBoost: 2})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !phys.SoulShot || phys.SkillPower != 60 {
		t.Fatalf("PhysicalSkillInput soulshot = %v skillPower = %v, want true/60", phys.SoulShot, phys.SkillPower)
	}

	fatal, ok := target.PhysicalSkillInput(soulCaster, modelskill.Definition{Power: 30, SkillType: "FATAL", SoulShotBoost: 2})
	if !ok {
		t.Fatal("FATAL PhysicalSkillInput() ok = false")
	}
	wantFatal := formulas.SkillPowerFor("FATAL", 30, soulCaster.HP()/soulCaster.MaxHPValue()) * 2
	if !fatal.SoulShot || !closeFloat(fatal.SkillPower, wantFatal) {
		t.Fatalf("FATAL soulshot = %v skillPower = %v, want true/%v", fatal.SoulShot, fatal.SkillPower, wantFatal)
	}

	blow, ok := target.BlowInput(soulCaster, modelskill.Definition{Power: 30, SkillType: "BLOW", SoulShotBoost: 2})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !blow.SoulShot || blow.SkillPower != 60 {
		t.Fatalf("BlowInput soulshot = %v skillPower = %v, want true/60", blow.SoulShot, blow.SkillPower)
	}

	spiritWeapon := &item.Instance{
		ObjectID: 11, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
		ShotsMask: item.ShotSpirit.Mask(),
	}
	spiritCaster := liveCharacter(3, tmpl, items, spiritWeapon)
	magic, ok := target.MagicDamageInput(spiritCaster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(spirit) ok = false")
	}
	if !magic.SoulShot || magic.BlessedSoulShot {
		t.Fatalf("MagicDamageInput spirit flags = soul %v blessed %v, want true/false", magic.SoulShot, magic.BlessedSoulShot)
	}
	mana, ok := target.ManaDamageInput(spiritCaster, modelskill.Definition{Power: 20, SkillType: "MANADAM"})
	if !ok {
		t.Fatal("ManaDamageInput(spirit) ok = false")
	}
	if !mana.SoulShot || mana.BlessedSoulShot {
		t.Fatalf("ManaDamageInput spirit flags = soul %v blessed %v, want true/false", mana.SoulShot, mana.BlessedSoulShot)
	}

	blessedWeapon := &item.Instance{
		ObjectID: 12, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
		ShotsMask: item.ShotBlessedSpirit.Mask(),
	}
	blessedCaster := liveCharacter(4, tmpl, items, blessedWeapon)
	magic, ok = target.MagicDamageInput(blessedCaster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(blessed) ok = false")
	}
	if magic.SoulShot || !magic.BlessedSoulShot {
		t.Fatalf("MagicDamageInput blessed flags = soul %v blessed %v, want false/true", magic.SoulShot, magic.BlessedSoulShot)
	}
}

func TestCharacterDamageInputsUsePvPMultipliers(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	caster.AddStatFuncs([]effect.Mod{
		{Stat: stat.PvPPhysSkillDmg, Op: effect.OpMul, Value: 0.8, Owner: testModOwner()},
		{Stat: stat.PvPMagicalDmg, Op: effect.OpMul, Value: 1.3, Owner: testModOwner()},
	})

	phys, ok := target.PhysicalSkillInput(caster, modelskill.Definition{Power: 30, SkillType: "PDAM"})
	if !ok {
		t.Fatal("PhysicalSkillInput() ok = false")
	}
	if !closeFloat(phys.PvPMul, 0.8) {
		t.Fatalf("PhysicalSkillInput PvPMul = %v, want 0.8", phys.PvPMul)
	}

	blow, ok := target.BlowInput(caster, modelskill.Definition{Power: 30, SkillType: "BLOW"})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !blow.IsPvP || !closeFloat(blow.PvPMul, 0.8) {
		t.Fatalf("BlowInput PvP = %v mul %v, want true/0.8", blow.IsPvP, blow.PvPMul)
	}

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Magic: true})
	if !ok {
		t.Fatal("MagicDamageInput(magic) ok = false")
	}
	if !closeFloat(magic.PvPMul, 1.3) {
		t.Fatalf("MagicDamageInput magic PvPMul = %v, want 1.3", magic.PvPMul)
	}

	physicalMagic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput(physical skill type) ok = false")
	}
	if !closeFloat(physicalMagic.PvPMul, 0.8) {
		t.Fatalf("MagicDamageInput physical PvPMul = %v, want 0.8", physicalMagic.PvPMul)
	}
}

// ---- from character_stats_golden_test.go ----
// goldenPlayerScenarios computes the stat pipeline parity oracle for
// player.Character: actor x stat x active-modifier-set, including several
// same-order funcs attached/detached in different sequences (float addition
// is not associative, and AddFunc's insertion order is load-bearing — see
// issue #1527), a Set rebase, and an item-driven Enchant. Every case is
// deterministic given fresh actors, so re-running this against a rewritten
// stat pipeline must reproduce bit-identical float64 values.
func goldenPlayerScenarios(t testing.TB) map[string]float64 {
	t.Helper()
	out := make(map[string]float64)

	// Same-order (30: Add/Sub) funcs attached in two different sequences,
	// using precision-sensitive values so the stable-insertion-order
	// contract is actually observable in the result, not just documented.
	{
		tmpl := combatTemplate()
		c := liveCharacter(1, tmpl, combatItems())
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1e16}})
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSub, Value: 1e16}})
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1}})
		out["order30_forward"] = c.PAtk()

		tmpl2 := combatTemplate()
		c2 := liveCharacter(2, tmpl2, combatItems())
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1}})
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpSub, Value: 1e16}})
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1e16}})
		out["order30_reverse"] = c2.PAtk()
	}

	// Set rebasing: a *Set at order 0 must replace `base` for every func
	// that runs after it, including the builtin at order 10.
	{
		tmpl := combatTemplate()
		c := liveCharacter(3, tmpl, combatItems())
		c.AddStatFuncs([]effect.Mod{
			{Stat: stat.MagicDefence, Op: effect.OpSet, Value: 500},
			{Stat: stat.MagicDefence, Op: effect.OpBaseMul, Value: 0.5},
		})
		out["set_rebase_mdef"] = c.MDef()
	}

	// Attach then detach: value must return exactly to the pre-attach
	// value, including through the builtin finalize step.
	{
		tmpl := combatTemplate()
		c := liveCharacter(4, tmpl, combatItems())
		base := c.PAtk()
		owner := effect.ModOwnerEffect(&effect.Effect{})
		c.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 7, Owner: owner},
			{Stat: stat.PowerAttack, Op: effect.OpMul, Value: 1.25, Owner: owner},
		})
		out["attach_detach_before"] = base
		out["attach_detach_during"] = c.PAtk()
		c.RemoveStatsByOwner(owner)
		out["attach_detach_after"] = c.PAtk()
	}

	// Mixed orders across several stats at once (BaseAdd, Mul, Add, AddMul,
	// SubDiv), each stat also carrying its own builtin at order 10.
	{
		tmpl := combatTemplate()
		tmpl.MAtk = 20
		tmpl.RunSpeed = 100
		c := liveCharacter(5, tmpl, combatItems())
		c.AddStatFuncs([]effect.Mod{
			{Stat: stat.PowerDefence, Op: effect.OpBaseAdd, Value: 4},
			{Stat: stat.PowerDefence, Op: effect.OpMul, Value: 1.1},
			{Stat: stat.MagicAttack, Op: effect.OpAdd, Value: 6},
			{Stat: stat.MagicAttack, Op: effect.OpAddMul, Value: 10}, // -10%
			{Stat: stat.RunSpeed, Op: effect.OpSubDiv, Value: 20},    // /(1-0.2)
		})
		out["mixed_pdef"] = c.PDef()
		out["mixed_matk"] = c.MAtk()
		out["mixed_runspeed"] = c.RunSpeed()
	}

	// Enchant: item-driven, no configured Value, math keyed off the
	// item's live EnchantLevel/Weapon/Crystal.
	{
		tmpl := combatTemplate()
		items := item.NewTable([]*item.Template{
			{ID: 50, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalS,
				Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		})
		inst := &item.Instance{ObjectID: 900, TemplateID: 50, Location: item.LocationPaperdoll, LocationData: 0, EnchantLevel: 7}
		c := liveCharacter(6, tmpl, items, inst)
		tmplRef, _ := items.Get(50)
		owner := effect.ModOwnerItem(effect.ItemOwner{Inst: inst, Tmpl: tmplRef})
		c.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpEnchant, Owner: owner}})
		out["enchant_patk_s_over3"] = c.PAtk()

		inst2 := &item.Instance{ObjectID: 901, TemplateID: 50, Location: item.LocationPaperdoll, LocationData: 0, EnchantLevel: 2}
		c2 := liveCharacter(7, tmpl, items, inst2)
		tmplRef2, _ := items.Get(50)
		owner2 := effect.ModOwnerItem(effect.ItemOwner{Inst: inst2, Tmpl: tmplRef2})
		c2.AddStatFuncs([]effect.Mod{{Stat: stat.PowerAttack, Op: effect.OpEnchant, Owner: owner2}})
		out["enchant_patk_s_under3"] = c2.PAtk()
	}

	return out
}

func TestGoldenPlayerStatPipelineCapture(t *testing.T) {
	if os.Getenv("ACIS_CAPTURE_GOLDEN") == "" {
		t.Skip("set ACIS_CAPTURE_GOLDEN=1 to (re)capture the golden fixture from the current implementation")
	}
	got := goldenPlayerScenarios(t)
	writeGolden(t, "testdata/golden_stats.json", got)
}

func TestGoldenPlayerStatPipelineParity(t *testing.T) {
	want := readGolden(t, "testdata/golden_stats.json")
	got := goldenPlayerScenarios(t)
	compareGolden(t, want, got)
}

func writeGolden(t testing.TB, path string, values map[string]float64) {
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

func readGolden(t testing.TB, path string) map[string]float64 {
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

func compareGolden(t testing.TB, want, got map[string]float64) {
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

// ---- from character_stats_heal_test.go ----
func TestCharacterHealAmountUsesMagicAttackAndHealProficiency(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 49
	c := liveCharacter(1, tmpl, combatItems())
	c.AddStatFuncs([]effect.Mod{{Stat: stat.HealProficiency, Op: effect.OpAdd, Value: 11, Owner: testModOwner()}})

	amount, ok := c.HealAmount(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealAmount() ok = false")
	}
	if want := 41.099019513592786; !closeFloat(amount, want) {
		t.Fatalf("HealAmount() = %v, want %v", amount, want)
	}

	static, ok := c.HealAmount(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok {
		t.Fatal("HealAmount(HEAL_STATIC) ok = false")
	}
	if want := 25.0 + 11; !closeFloat(static, want) {
		t.Fatalf("HealAmount(HEAL_STATIC) = %v, want %v", static, want)
	}
}

// ---- from character_stats_shield_test.go ----
type shieldDefenseResolver interface {
	ShieldDefense(caster creature.DeathActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense
}

func TestCharacterShieldDefenseUsesLiveShieldStatsFacingAndRoll(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
	})

	src, ok := any(target).(shieldDefenseResolver)
	if !ok {
		t.Fatal("Character must resolve live shield defense")
	}

	tests := []struct {
		name string
		roll int
		want formulas.ShieldDefense
	}{
		{name: "perfect block", roll: 0, want: formulas.ShieldPerfect},
		{name: "ordinary block", roll: 5, want: formulas.ShieldSuccess},
		{name: "failed block", roll: 99, want: formulas.ShieldFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target.SetRollSource(func(n int) int {
				if n != 100 {
					t.Fatalf("shield roll bound = %d, want 100", n)
				}
				return tt.roll
			})
			if got := src.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != tt.want {
				t.Fatalf("ShieldDefense() = %v, want %v", got, tt.want)
			}
		})
	}

	caster.SetLastKnownPosition(location.Location{X: -80, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]effect.Mod{{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()}})
	target.SetRollSource(func(int) int { return 0 })
	if got := src.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != formulas.ShieldPerfect {
		t.Fatalf("ShieldDefense() with 360-degree stat = %v, want ShieldPerfect", got)
	}
}

// TestCharacterShieldDefenseUsesConfiguredPerfectShieldBlockRate proves a
// non-default PerfectShieldBlockRate (players.properties) changes the
// perfect-block roll threshold, matching Formulas.java:859.
func TestCharacterShieldDefenseUsesConfiguredPerfectShieldBlockRate(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
	})
	target.SetRollSource(func(int) int { return 10 })

	// Default rate 5: roll 10 is >= rate, so an ordinary block, not perfect.
	if got := target.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != formulas.ShieldSuccess {
		t.Fatalf("ShieldDefense() with default rate = %v, want ShieldSuccess", got)
	}

	// Configuring PerfectShieldBlockRate to 15 pushes the same roll (10)
	// under the threshold, upgrading the block to perfect.
	target.SetPerfectShieldBlockRate(15)
	if got := target.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != formulas.ShieldPerfect {
		t.Fatalf("ShieldDefense() with configured rate 15 = %v, want ShieldPerfect", got)
	}
}

func TestCharacterShieldDefenseGatesEquipStatsAndFacing(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	def := modelskill.Definition{SkillType: "STUN"}

	tests := []struct {
		name      string
		equipped  []*item.Instance
		rate      float64
		angle     float64
		casterLoc location.Location
		def       modelskill.Definition
	}{
		{
			name:      "no shield equipped",
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "left hand is not armor",
			equipped:  []*item.Instance{equippedArrow()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "left hand armor is not a shield",
			equipped:  []*item.Instance{equippedLightArmor()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "zero shield rate",
			equipped:  []*item.Instance{equippedShield()},
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "outside shield angle",
			equipped:  []*item.Instance{equippedShield()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: -80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "skill ignores shield",
			equipped:  []*item.Instance{equippedShield()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       modelskill.Definition{SkillType: "STUN", IgnoreShield: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := liveCharacter(1, tmpl, items)
			target := liveCharacter(2, tmpl, items, tt.equipped...)
			caster.SetLastKnownPosition(tt.casterLoc, 0)
			target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
			target.SetRollSource(func(int) int { return 0 })
			target.AddStatFuncs([]effect.Mod{
				{Stat: stat.ShieldRate, Op: effect.OpSet, Value: tt.rate, Owner: testModOwner()},
				{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: tt.angle, Owner: testModOwner()},
			})

			src, ok := any(target).(shieldDefenseResolver)
			if !ok {
				t.Fatal("Character must resolve live shield defense")
			}
			if got := src.ShieldDefense(caster, tt.def, false); got != formulas.ShieldFailed {
				t.Fatalf("ShieldDefense() = %v, want ShieldFailed", got)
			}
		})
	}
}

func TestCharacterShieldDefenseNotifiesDefendingPlayerBySDefOnly(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	def := modelskill.Definition{SkillType: "STUN"}

	tests := []struct {
		name        string
		roll        int
		wantSuccess bool
		wantPerfect bool
	}{
		{name: "perfect block notifies excellent message", roll: 0, wantPerfect: true},
		{name: "ordinary block notifies success message", roll: 5, wantSuccess: true},
		{name: "failed block sends no message", roll: 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := liveCharacter(1, tmpl, items)
			target := liveCharacter(2, tmpl, items, equippedShield())
			caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
			target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
			target.AddStatFuncs([]effect.Mod{
				{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: testModOwner()},
				{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
			})
			target.SetRollSource(func(int) int { return tt.roll })

			var gotSuccess, gotPerfect bool
			target.SetShieldBlockNotifiers(func() { gotSuccess = true }, func() { gotPerfect = true })

			target.ShieldDefense(caster, def, false)

			if gotSuccess != tt.wantSuccess {
				t.Fatalf("shield block success notice fired = %v, want %v", gotSuccess, tt.wantSuccess)
			}
			if gotPerfect != tt.wantPerfect {
				t.Fatalf("shield block perfect notice fired = %v, want %v", gotPerfect, tt.wantPerfect)
			}
		})
	}
}

func shieldDefenseItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
		{ID: 4, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
		{ID: 5, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
	})
}

func equippedShield() *item.Instance {
	return &item.Instance{ObjectID: 30, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

func equippedArrow() *item.Instance {
	return &item.Instance{ObjectID: 40, TemplateID: 4, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

func equippedLightArmor() *item.Instance {
	return &item.Instance{ObjectID: 50, TemplateID: 5, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

// ---- from character_stats_skillsuccess_test.go ----
func TestCharacterSkillSuccessInputUsesStatsAndCasterMagicAttack(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.CharLevel = 44
	target.AddStatFuncs([]effect.Mod{{Stat: stat.StunVuln, Op: effect.OpMul, Value: 0.5, Owner: testModOwner()}})
	def := modelskill.Definition{
		SkillType:    "STUN",
		EffectType:   "STUN",
		Magic:        true,
		MagicLevel:   40,
		LevelDepend:  2,
		BaseLandRate: 50,
	}

	in, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}

	if in.BaseChance != 50 {
		t.Fatalf("BaseChance = %v, want 50", in.BaseChance)
	}
	if want := 0.7430194910023464; !closeFloat(in.StatModifier, want) {
		t.Fatalf("StatModifier = %v, want %v", in.StatModifier, want)
	}
	if !closeFloat(in.VulnModifier, 0.5) {
		t.Fatalf("VulnModifier = %v, want 0.5", in.VulnModifier)
	}
	if want := 0.9420817669172932; !closeFloat(in.MAtkModifier, want) {
		t.Fatalf("MAtkModifier = %v, want %v", in.MAtkModifier, want)
	}
	if want := 1 + 0.01*(float64(def.MagicLevel+def.LevelDepend-target.CharLevel)); !closeFloat(in.LevelModifier, want) {
		t.Fatalf("LevelModifier = %v, want %v", in.LevelModifier, want)
	}
}

func TestCharacterSkillReflectInputUsesMagicSpecificStat(t *testing.T) {
	target := liveCharacter(1, combatTemplate(), combatItems())
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ReflectSkillMagic, Op: effect.OpSet, Value: 17, Owner: testModOwner()},
		{Stat: stat.ReflectSkillPhysic, Op: effect.OpSet, Value: 29, Owner: testModOwner()},
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

func TestCharacterEffectSuccessInputRespectsTemplateResistance(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]effect.Mod{{Stat: stat.StunVuln, Op: effect.OpMul, Value: 0.5, Owner: testModOwner()}})
	def := modelskill.Definition{IgnoreResists: true, Magic: true, MagicLevel: 40, EffectType: "ROOT"}

	in, ok := target.EffectSuccessInput(caster, def, modelskill.EffectTemplate{EffectPower: 50, EffectType: "STUN"}, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("EffectSuccessInput() ok = false")
	}
	if in.IgnoreResists {
		t.Fatal("EffectSuccessInput() ignored template resistance")
	}
	if in.BaseChance != 50 || !closeFloat(in.VulnModifier, 0.5) {
		t.Fatalf("EffectSuccessInput() = %+v, want template power and STUN resistance", in)
	}
}

func TestCharacterSkillSuccessInputFoldsElementalResistanceIntoVulnerability(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.FireRes, Op: effect.OpMul, Value: 0.36, Owner: testModOwner()},
		{Stat: stat.StunVuln, Op: effect.OpMul, Value: 0.5, Owner: testModOwner()},
	})

	in, ok := target.SkillSuccessInput(caster, modelskill.Definition{
		SkillType:    "STUN",
		EffectType:   "STUN",
		Element:      modelskill.ElementFire,
		BaseLandRate: 50,
	}, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}

	// Java folds sqrt(elemental resistance) in as the vulnerability base
	// before applying the stat-specific (here STUN) vulnerability on top:
	// sqrt(0.36) * 0.5 = 0.3.
	if want := 0.3; !closeFloat(in.VulnModifier, want) {
		t.Fatalf("VulnModifier = %v, want %v", in.VulnModifier, want)
	}
}

func TestCharacterManaDamageInputFoldsElementalResistanceIntoVulnerability(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]effect.Mod{{Stat: stat.FireRes, Op: effect.OpMul, Value: 0.36, Owner: testModOwner()}})

	mana, ok := target.ManaDamageInput(caster, modelskill.Definition{
		SkillType: "MANADAM",
		Element:   modelskill.ElementFire,
		Power:     20,
	})
	if !ok {
		t.Fatal("ManaDamageInput() ok = false")
	}
	// MANADAM has no matching vulnerability case (see the STUN/POISON/...
	// switch), so Java's calcSkillVulnerability returns the elemental base
	// unchanged: sqrt(0.36) = 0.6.
	if want := 0.6; !closeFloat(mana.VulnMul, want) {
		t.Fatalf("VulnMul = %v, want %v", mana.VulnMul, want)
	}
}

func TestCharacterSkillSuccessInputDoesNotFallbackToSkillType(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	target.AddStatFuncs([]effect.Mod{{Stat: stat.StunVuln, Op: effect.OpMul, Value: 0.5, Owner: testModOwner()}})

	in, ok := target.SkillSuccessInput(caster, modelskill.Definition{
		SkillType:    "STUN",
		Magic:        true,
		BaseLandRate: 50,
	}, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}

	if in.StatModifier != 1 {
		t.Fatalf("StatModifier = %v, want 1 without EffectType", in.StatModifier)
	}
	if in.VulnModifier != 1 {
		t.Fatalf("VulnModifier = %v, want 1 without EffectType", in.VulnModifier)
	}
}

// TestCharacterSkillSuccessInputQuadruplesMAtkOnBlessedSpiritshot asserts a
// blessed-spiritshot charge quadruples the caster's effective magic attack
// before the square root is taken, exactly doubling the resulting modifier
// (sqrt(4x) == 2*sqrt(x)) relative to an uncharged cast against the same
// stats used by TestCharacterSkillSuccessInputUsesStatsAndCasterMagicAttack.
func TestCharacterSkillSuccessInputQuadruplesMAtkOnBlessedSpiritshot(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 100
	tmpl.MDef = 50
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	def := modelskill.Definition{SkillType: "STUN", EffectType: "STUN", Magic: true, BaseLandRate: 50}

	without, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=false) ok = false")
	}
	with, ok := target.SkillSuccessInput(caster, def, true, formulas.ShieldFailed)
	if !ok {
		t.Fatal("SkillSuccessInput(bss=true) ok = false")
	}

	if want := without.MAtkModifier * 2; !closeFloat(with.MAtkModifier, want) {
		t.Fatalf("MAtkModifier with bss = %v, want %v (2x the uncharged modifier)", with.MAtkModifier, want)
	}
}

// TestCharacterSkillSuccessInputCarriesShieldOutcome asserts the
// already-resolved shield-block outcome passed into SkillSuccessInput
// reaches the returned formula input unchanged, and that a perfect block
// fails the landing roll outright through the real formulas pipeline.
func TestCharacterSkillSuccessInputCarriesShieldOutcome(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	def := modelskill.Definition{SkillType: "STUN", BaseLandRate: 100, IgnoreResists: true}

	in, ok := target.SkillSuccessInput(caster, def, false, formulas.ShieldPerfect)
	if !ok {
		t.Fatal("SkillSuccessInput() ok = false")
	}
	if in.Shield != formulas.ShieldPerfect {
		t.Fatalf("Shield = %v, want ShieldPerfect", in.Shield)
	}
	if rate := formulas.SkillSuccessRate(in); rate != 0 {
		t.Fatalf("SkillSuccessRate() = %v, want 0 for a perfect block despite IgnoreResists", rate)
	}
}

// ---- from character_stats_test.go ----
func closeFloat(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// testModOwner returns a fresh, disposable stat-Mod owner identity for
// tests that need one only to attach a Mod, never to exercise
// RemoveStatsByOwner's identity matching.
func testModOwner() effect.ModOwner {
	return effect.ModOwnerEffect(&effect.Effect{})
}

// ---- from character_summon_test.go ----
type spySummonSpawner struct {
	calls []spySpawnCall
	ok    bool
}

type spySpawnCall struct {
	owner *Character
	item  *item.Instance
	def   modelskill.Definition
}

func (s *spySummonSpawner) SpawnPet(owner *Character, controlItem *item.Instance) bool {
	s.calls = append(s.calls, spySpawnCall{owner: owner, item: controlItem})
	return s.ok
}

func (s *spySummonSpawner) SpawnServitor(owner *Character, def modelskill.Definition) bool {
	s.calls = append(s.calls, spySpawnCall{owner: owner, def: def})
	return s.ok
}

func TestCharacterSummonCreatureDelegatesToSpawner(t *testing.T) {
	c := &Character{}
	spy := &spySummonSpawner{ok: true}
	c.SetSummonSpawner(spy)
	inst := &item.Instance{ObjectID: 500, TemplateID: 91000}

	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, inst)

	if len(spy.calls) != 1 {
		t.Fatalf("SpawnPet calls = %d, want 1", len(spy.calls))
	}
	if spy.calls[0].owner != c {
		t.Fatalf("SpawnPet owner = %v, want %v", spy.calls[0].owner, c)
	}
	if spy.calls[0].item != inst {
		t.Fatalf("SpawnPet item = %v, want %v", spy.calls[0].item, inst)
	}
}

func TestCharacterSummonServitorDelegatesToSpawner(t *testing.T) {
	c := &Character{}
	spy := &spySummonSpawner{ok: true}
	c.SetSummonSpawner(spy)

	c.SummonServitor(modelskill.Definition{NpcID: 14848})

	if len(spy.calls) != 1 {
		t.Fatalf("SpawnServitor calls = %d, want 1", len(spy.calls))
	}
	if spy.calls[0].owner != c || spy.calls[0].def.NpcID != 14848 {
		t.Fatalf("SpawnServitor call = %+v, want owner and NpcID 14848", spy.calls[0])
	}
}

func TestCharacterSummonCreatureNoopsWithoutSpawner(t *testing.T) {
	c := &Character{}
	// No SetSummonSpawner call: must not panic, matching Java's item==nil
	// early return.
	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, &item.Instance{})
}

func TestCharacterSummonCreatureNoopsOnNonItemArg(t *testing.T) {
	c := &Character{}
	spy := &spySummonSpawner{ok: true}
	c.SetSummonSpawner(spy)

	// A cast-interrupted skill can reach the handler with no item
	// (handler/skill/summon.go's own doc comment); SummonCreature must
	// drop that silently, matching Java's checkedItem==nil early return.
	c.SummonCreature(modelskill.Definition{ID: 2046, Level: 1}, nil)

	if len(spy.calls) != 0 {
		t.Fatalf("SpawnPet calls = %d, want 0 for a non-item cast.Item", len(spy.calls))
	}
}

// ---- from character_target_test.go ----
func targetCharacter(id int32) *Character {
	return &Character{ID: id, Name: "target", CharLevel: 1}
}

func TestCharacterTargetRoundTrips(t *testing.T) {
	c := targetCharacter(1)
	if got := c.Target(); got != nil {
		t.Fatalf("Target() = %v, want nil before any selection", got)
	}

	other := targetCharacter(2)
	c.SetTargetTracked(other)
	if got := c.Target(); got != world.Tracked(other) {
		t.Fatalf("Target() = %v, want %v", got, other)
	}

	c.SetTargetTracked(nil)
	if got := c.Target(); got != nil {
		t.Fatalf("Target() = %v, want nil after clearing", got)
	}
}

// TestCharacterRetargetableOnAggressionRetargetsWhenNotAlreadyTargetingCaster
// exercises the retargetableOnAggression contract the AGGDEBUFF continuous
// handler consults: a playable not currently targeting the caster gets
// retargeted onto them via SetTarget, not attacked.
func TestCharacterRetargetableOnAggressionRetargetsWhenNotAlreadyTargetingCaster(t *testing.T) {
	caster := targetCharacter(1)
	other := targetCharacter(3)
	target := targetCharacter(2)
	target.SetTargetTracked(other)

	var attacked bool
	target.SetAttackTargetHook(func(world.Tracked) { attacked = true })

	if got := target.CurrentTarget(); got != world.Tracked(other) {
		t.Fatalf("CurrentTarget() = %v, want %v", got, other)
	}
	target.SetTarget(caster)

	if got := target.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() after SetTarget = %v, want caster", got)
	}
	if attacked {
		t.Fatal("a playable not already targeting the caster must be retargeted, not attacked")
	}
}

// TestCharacterRetargetableOnAggressionAttacksWhenAlreadyTargetingCaster
// exercises the other branch: a playable already targeting the caster is
// provoked into attacking them through the AttackTarget hook instead of
// being retargeted.
func TestCharacterRetargetableOnAggressionAttacksWhenAlreadyTargetingCaster(t *testing.T) {
	caster := targetCharacter(1)
	target := targetCharacter(2)
	target.SetTargetTracked(caster)

	var attackedWith any
	target.SetAttackTargetHook(func(t world.Tracked) { attackedWith = t })

	target.AttackTarget(caster)

	if attackedWith != world.Tracked(caster) {
		t.Fatalf("AttackTarget hook called with %v, want caster", attackedWith)
	}
	if got := target.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() = %v, want unchanged caster (attack, not retarget)", got)
	}
}

func TestCharacterTryToAttackDelegatesToAttackTarget(t *testing.T) {
	caster := targetCharacter(1)
	target := targetCharacter(2)
	var attackedWith any
	target.SetAttackTargetHook(func(t world.Tracked) { attackedWith = t })

	target.TryToAttack(caster)

	if attackedWith != world.Tracked(caster) {
		t.Fatalf("attack target hook called with %v, want caster", attackedWith)
	}
}

// ---- from character_test.go ----
func humanFighterTemplate() *Template {
	return &Template{
		ID:        0,
		BaseLevel: 1,
		CON:       43,
		MEN:       25,
		HPTable:   []float64{80, 91.83},
		MPTable:   []float64{30, 35.46},
		CPTable:   []float64{32, 36.732},
		Spawns: []location.Location{
			{X: 1, Y: 2, Z: 3},
		},
	}
}

func TestNewCharacter(t *testing.T) {
	tmpl := humanFighterTemplate()

	c, err := NewCharacter(0x10000001, tmpl, "acct1", "Newbie", 1, 2, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}

	if c.ID != 0x10000001 || c.AccountName != "acct1" || c.Name != "Newbie" {
		t.Fatalf("NewCharacter() identity = %+v", c)
	}
	if c.ClassID != 0 || c.BaseClassID != 0 {
		t.Errorf("ClassID/BaseClassID = %d/%d, want 0/0", c.ClassID, c.BaseClassID)
	}
	if c.Race != RaceHuman {
		t.Errorf("Race = %v, want %v", c.Race, RaceHuman)
	}
	if c.Sex != SexMale {
		t.Errorf("Sex = %v, want %v", c.Sex, SexMale)
	}
	if c.CharLevel != 1 {
		t.Errorf("Level = %d, want 1", c.CharLevel)
	}
	res := c.ResourceValues()
	if res.MaxHP != tmpl.HPTable[0] {
		t.Errorf("stored max HP base = %v, want raw table value %v", res.MaxHP, tmpl.HPTable[0])
	}
	if want := float64(int(tmpl.HPTable[0] * statbonus.CONBonus[tmpl.CON])); res.CurrentHP != want {
		t.Errorf("CurrentHP = %v, want computed max %v", res.CurrentHP, want)
	}
	if res.MaxMP != tmpl.MPTable[0] {
		t.Errorf("stored max MP base = %v, want raw table value %v", res.MaxMP, tmpl.MPTable[0])
	}
	if want := float64(int(tmpl.MPTable[0] * statbonus.MENBonus[tmpl.MEN])); res.CurrentMP != want {
		t.Errorf("CurrentMP = %v, want computed max %v", res.CurrentMP, want)
	}
	if res.MaxCP != tmpl.CPTable[0] {
		t.Errorf("stored max CP base = %v, want raw table value %v", res.MaxCP, tmpl.CPTable[0])
	}
	if res.CurrentCP != 0 {
		t.Errorf("CurrentCP = %v, want 0", res.CurrentCP)
	}
	if c.HairStyle != 1 || c.HairColor != 2 || c.Face != 0 {
		t.Errorf("appearance = hairStyle=%d hairColor=%d face=%d, want 1/2/0", c.HairStyle, c.HairColor, c.Face)
	}
	if c.Location != tmpl.Spawns[0] {
		t.Errorf("Location = %+v, want %+v", c.Location, tmpl.Spawns[0])
	}
	if c.AccessLevel != defaultAccessLevel {
		t.Errorf("AccessLevel = %d, want %d", c.AccessLevel, defaultAccessLevel)
	}
}

// TestNewCharacterVitalsApplyBonusOnce pins that a freshly created
// character's computed maxima fold the CON/MEN bonus exactly once: the raw
// level-table bases stored at creation are finalized through the live stat
// calculator without pre-multiplication, and no current value starts above
// its own maximum.
func TestNewCharacterVitalsApplyBonusOnce(t *testing.T) {
	tmpl := humanFighterTemplate()

	c, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	c.AttachRuntime(tmpl, nil)

	res := c.ResourceValues()
	if want := tmpl.HPTable[0] * statbonus.CONBonus[tmpl.CON]; res.MaxHP != want {
		t.Errorf("MaxHPValue() = %v, want table*CONBonus applied once = %v", res.MaxHP, want)
	}
	if want := tmpl.MPTable[0] * statbonus.MENBonus[tmpl.MEN]; res.MaxMP != want {
		t.Errorf("MaxMPValue() = %v, want table*MENBonus applied once = %v", res.MaxMP, want)
	}
	if want := tmpl.CPTable[0] * statbonus.CONBonus[tmpl.CON]; res.MaxCP != want {
		t.Errorf("MaxCPValue() = %v, want table*CONBonus applied once = %v", res.MaxCP, want)
	}
	if res.CurrentHP > res.MaxHP || res.CurrentMP > res.MaxMP || res.CurrentCP > res.MaxCP {
		t.Errorf("current vitals %+v exceed their maxima on a fresh character", res)
	}
}

func TestNewCharacter_NilTemplate(t *testing.T) {
	if _, err := NewCharacter(1, nil, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with nil template: want error, got nil")
	}
}

// TestRestoreVitalsRebasesFromTemplate pins the restore boundary for
// persisted vitals: a characters row stores finalized max snapshots (Save
// writes ResourceValues), so restoring them verbatim into the raw base fields
// would re-apply the CON/MEN finalize on every read and compound across
// save→load cycles. RestoreVitals must re-derive the bases from the class
// level tables — keeping each cycle's computed maxima identical — while the
// current values survive untouched.
func TestRestoreVitalsRebasesFromTemplate(t *testing.T) {
	tmpl := humanFighterTemplate()

	restored, err := NewCharacter(1, tmpl, "acct1", "Restored", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	restored.CharLevel = 2

	finalHP := tmpl.HPTable[1] * statbonus.CONBonus[tmpl.CON]
	finalMP := tmpl.MPTable[1] * statbonus.MENBonus[tmpl.MEN]
	restored.SetResourceValues(Resources{
		MaxHP: finalHP, CurrentHP: finalHP / 2,
		MaxCP: tmpl.CPTable[1] * statbonus.CONBonus[tmpl.CON], CurrentCP: 0,
		MaxMP: finalMP, CurrentMP: finalMP / 2,
	})
	restored.AttachRuntime(tmpl, nil)

	restored.RestoreVitals(tmpl)

	res := restored.ResourceValues()
	if want := tmpl.HPTable[1] * statbonus.CONBonus[tmpl.CON]; res.MaxHP != want {
		t.Errorf("MaxHPValue() = %v, want table*CONBonus applied once = %v", res.MaxHP, want)
	}
	if want := tmpl.MPTable[1] * statbonus.MENBonus[tmpl.MEN]; res.MaxMP != want {
		t.Errorf("MaxMPValue() = %v, want table*MENBonus applied once = %v", res.MaxMP, want)
	}
	if want := tmpl.CPTable[1] * statbonus.CONBonus[tmpl.CON]; res.MaxCP != want {
		t.Errorf("MaxCPValue() = %v, want table*CONBonus applied once = %v", res.MaxCP, want)
	}
	if want := finalHP / 2; res.CurrentHP != want {
		t.Errorf("CurrentHP = %v, want restored value preserved = %v", res.CurrentHP, want)
	}
	if want := finalMP / 2; res.CurrentMP != want {
		t.Errorf("CurrentMP = %v, want restored value preserved = %v", res.CurrentMP, want)
	}

	// A second save→load cycle must land on exactly the same maxima and
	// currents: snapshot is what Save persists, replayed through the restore
	// boundary.
	snapshot := restored.ResourceValues()
	recycled, err := NewCharacter(1, tmpl, "acct1", "Restored", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	recycled.CharLevel = 2
	recycled.SetResourceValues(snapshot)
	recycled.AttachRuntime(tmpl, nil)
	recycled.RestoreVitals(tmpl)

	if next := recycled.ResourceValues(); next != snapshot {
		t.Errorf("second round trip = %+v, want unchanged %+v", next, snapshot)
	}

	// Levels without a table row leave the stored bases untouched, matching
	// AddLevel's refill convention, and a nil template is a no-op.
	recycled.CharLevel = len(tmpl.HPTable) + 1
	stale := recycled.ResourceValues()
	recycled.RestoreVitals(tmpl)
	if after := recycled.ResourceValues(); after.MaxHP != stale.MaxHP || after.MaxMP != stale.MaxMP || after.MaxCP != stale.MaxCP {
		t.Errorf("RestoreVitals beyond the table changed maxima %+v → %+v, want untouched", stale, after)
	}
	recycled.CharLevel = 2
	recycled.RestoreVitals(nil)
	if after := recycled.ResourceValues(); after.MaxHP != stale.MaxHP {
		t.Errorf("RestoreVitals(nil) changed MaxHP to %v, want untouched %v", after.MaxHP, stale.MaxHP)
	}
}

func TestNewCharacter_UnknownClass(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.ID = 9999
	if _, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with unknown class id: want error, got nil")
	}
}

func TestNewCharacter_MissingLevelTables(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.HPTable = nil
	if _, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale); err == nil {
		t.Fatal("NewCharacter() with no HP table: want error, got nil")
	}
}

func TestNewCharacter_NoSpawnsLeavesZeroPosition(t *testing.T) {
	tmpl := humanFighterTemplate()
	tmpl.Spawns = nil

	c, err := NewCharacter(1, tmpl, "acct1", "Newbie", 0, 0, 0, SexMale)
	if err != nil {
		t.Fatalf("NewCharacter() unexpected error: %v", err)
	}
	if c.Location != (location.Location{}) {
		t.Errorf("Location = %+v, want zero value", c.Location)
	}
}

// ---- from character_vitals_test.go ----
func TestCharacterResourcesAreNotExportedFields(t *testing.T) {
	typ := reflect.TypeOf(Character{})
	for _, name := range []string{"MaxHP", "CurHP", "MaxMP", "CurMP", "MaxCP", "CurCP"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("Character exports mutable resource field %s", name)
		}
	}
}

func TestCharacterVitals(t *testing.T) {
	ch := &Character{}
	ch.SetResourceValues(Resources{CurrentHP: 12.9, CurrentMP: 7.1})

	got := ch.Vitals()
	want := Vitals{HP: 12, MP: 7}
	if got != want {
		t.Fatalf("Vitals() = %+v, want %+v", got, want)
	}
}

func TestHPFull(t *testing.T) {
	ch := &Character{}
	ch.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 99})
	if ch.HPFull() {
		t.Fatal("HPFull() = true below maximum")
	}
	ch.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 100})
	if !ch.HPFull() {
		t.Fatal("HPFull() = false at maximum")
	}
}

func TestVitalsChangesTo(t *testing.T) {
	before := Vitals{HP: 100, MP: 50}

	got := before.ChangesTo(Vitals{HP: 75, MP: 50})
	want := VitalsChange{HP: 75, HPChanged: true, MP: 50}
	if got != want {
		t.Fatalf("ChangesTo() = %+v, want %+v", got, want)
	}
	if !got.Changed() {
		t.Fatal("Changed() = false, want true")
	}

	got = before.ChangesTo(before)
	want = VitalsChange{HP: 100, MP: 50}
	if got != want {
		t.Fatalf("ChangesTo(unchanged) = %+v, want %+v", got, want)
	}
	if got.Changed() {
		t.Fatal("Changed() unchanged = true, want false")
	}
}

// ---- from character_weight_penalty_test.go ----
func TestRefreshWeightPenalty(t *testing.T) {
	tests := []struct {
		name  string
		ratio float64
		want  int
	}{
		{"below half", .499, 0},
		{"half", .5, 1},
		{"two thirds", .666, 2},
		{"four fifths", .8, 3},
		{"full", 1, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
			c := &Character{}
			c.AttachRuntime(&Template{CON: 20}, inv)
			c.SetWeightLimitMultiplier(1)
			inv.AddNew(1, int(math.Ceil(float64(c.WeightLimit())*tc.ratio)), 1)
			inv.UpdateWeight()
			c.RefreshWeightPenalty()
			if got := c.WeightPenalty(); got != tc.want {
				t.Fatalf("WeightPenalty() = %d, want %d (limit %d weight %d)", got, tc.want, c.WeightLimit(), c.CurrentWeight())
			}
		})
	}
}

func TestRefreshWeightPenaltyChangesOnlyOnBandChange(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.SetWeightLimitMultiplier(1)
	inv.AddNew(1, c.WeightLimit()/2, 1)
	inv.UpdateWeight()
	updates := 0
	c.SetWeightPenaltyUpdater(func() { updates++ })

	c.RefreshWeightPenalty()
	c.RefreshWeightPenalty()
	if updates != 1 {
		t.Fatalf("updates after unchanged refresh = %d, want 1", updates)
	}

	inv.AddNew(1, c.WeightLimit()/6, 2)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	if got, want := c.WeightPenalty(), 2; got != want {
		t.Fatalf("WeightPenalty() = %d, want %d", got, want)
	}
	if updates != 2 {
		t.Fatalf("updates after band change = %d, want 2", updates)
	}
}

// TestWeightPenaltySpeedMultiplier pins the per-band speed multiplier to
// WeightPenalty.java:5-9 (NONE 1, LEVEL_1 1, LEVEL_2 0.5, LEVEL_3 0.5,
// LEVEL_4 0). LEVEL_4 must be 0, not 0.5 — a fully overloaded reference
// player cannot move (PlayerStatus.getMoveSpeed, PlayerStatus.java:944-947).
func TestWeightPenaltySpeedMultiplier(t *testing.T) {
	tests := []struct {
		band int
		want float64
	}{
		{0, 1}, {1, 1}, {2, .5}, {3, .5}, {4, 0},
	}
	for _, tc := range tests {
		c := &Character{}
		c.stateMu.Lock()
		c.weightPenalty = tc.band
		c.stateMu.Unlock()
		if got := c.weightPenaltySpeedMultiplier(); got != tc.want {
			t.Errorf("band %d: weightPenaltySpeedMultiplier() = %v, want %v", tc.band, got, tc.want)
		}
	}
}

// TestAddLevelRefreshesWeightPenalty pins PlayerStatus.addLevel's direct
// call to _actor.refreshWeightPenalty() on every level change
// (PlayerStatus.java:644, before the UserInfo send at :648). The band is
// forced stale beforehand so a passing test proves AddLevel actually
// recomputed it rather than leaving it untouched.
func TestAddLevelRefreshesWeightPenalty(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{CharLevel: 1}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.SetWeightLimitMultiplier(1)
	inv.AddNew(1, c.WeightLimit(), 1) // full overload -> band 4
	inv.UpdateWeight()

	c.stateMu.Lock()
	c.weightPenalty = 0 // stale: as if never refreshed
	c.stateMu.Unlock()

	table, err := NewLevelTable(map[int]Level{1: {RequiredExpToLevelUp: 0}, 2: {RequiredExpToLevelUp: 100}, 3: {RequiredExpToLevelUp: 200}})
	if err != nil {
		t.Fatalf("NewLevelTable: %v", err)
	}
	c.AddLevel(table, nil, 1)

	if got, want := c.WeightPenalty(), 4; got != want {
		t.Fatalf("WeightPenalty() after AddLevel = %d, want %d (stale band never recomputed)", got, want)
	}
}

func TestRefreshWeightPenaltyKeepsStateWhenLimitIsZero(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindEtcItem, Weight: 1, Stackable: true, EtcItem: &item.EtcItemDetail{}}}))
	c := &Character{}
	c.AttachRuntime(&Template{CON: 20}, inv)
	c.SetWeightLimitMultiplier(1)
	inv.AddNew(1, c.WeightLimit(), 1)
	inv.UpdateWeight()
	c.RefreshWeightPenalty()
	c.SetWeightLimitMultiplier(0)
	c.RefreshWeightPenalty()
	if got, want := c.WeightPenalty(), 4; got != want {
		t.Fatalf("WeightPenalty() after zero limit = %d, want %d", got, want)
	}
}

// ---- from class_test.go ----
func TestClassLevel(t *testing.T) {
	tests := []struct {
		id        int
		wantLevel int
		wantOK    bool
	}{
		{id: 0, wantLevel: 0, wantOK: true},   // base
		{id: 1, wantLevel: 1, wantOK: true},   // first occupation change
		{id: 2, wantLevel: 2, wantOK: true},   // second occupation change
		{id: 88, wantLevel: 3, wantOK: true},  // third class
		{id: 118, wantLevel: 3, wantOK: true}, // third class, last id
		{id: 999, wantLevel: 0, wantOK: false},
		{id: 58, wantLevel: 0, wantOK: false}, // reserved gap
	}
	for _, tt := range tests {
		level, ok := ClassLevel(tt.id)
		if level != tt.wantLevel || ok != tt.wantOK {
			t.Errorf("ClassLevel(%d) = (%d, %v), want (%d, %v)", tt.id, level, ok, tt.wantLevel, tt.wantOK)
		}
	}
}

// ---- from fixtures_test.go ----
func zeroRoll(int) int { return 0 }

func combatTemplate() *Template {
	return &Template{
		ID: 0, FistsItemID: 1,
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 5, PDef: 50,
		CollisionRadius: 9, CollisionHeight: 23,
		HPTable: []float64{100}, MPTable: []float64{30}, CPTable: []float64{0},
		Spawns: []location.Location{{X: 0, Y: 0, Z: 0}},
	}
}

func combatItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 5},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 300},
		}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD, Weapon: &item.WeaponDetail{Type: item.WeaponSword, ReuseDelay: 1200, RandomDamage: 0}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 100},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 433},
			{Op: item.FuncSet, Stat: "rCrit", Value: 4},
		}},
		{ID: 3, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD, Weapon: &item.WeaponDetail{
			Type: item.WeaponSword, SoulshotCount: 2, SpiritshotCount: 1,
		}},
	})
}

func liveCharacter(id int32, tmpl *Template, items *item.Table, equipped ...*item.Instance) *Character {
	c := &Character{
		ID: id, Name: "char", ClassID: tmpl.ID, BaseClassID: tmpl.ID,
		Race: RaceHuman, Sex: SexMale, CharLevel: 1,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	c.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 30, CurrentMP: 30})
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, items, equipped))
	c.SetRollSource(zeroRoll)
	c.SetPerfectShieldBlockRate(5)
	return c
}

// reads the last-known state the way a save or a range check would. Run
// with -race.
func TestCharacterPositionAccessIsRaceFree(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SyncPosition(location.Location{X: i, Y: 0, Z: 0})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SetLastKnownPosition(location.Location{X: -i, Y: 0, Z: 0}, i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.CurrentLocation()
			_ = c.CurrentHeading()
		}
	}()

	wg.Wait()
}

// npcKiller is a minimal non-player DeathActor double.
type npcKiller struct{ id int32 }

func (k npcKiller) ObjectID() int32 { return k.id }

type summonKiller struct{ owner creature.DeathActor }

func (k summonKiller) ObjectID() int32                   { return 3 }
func (k summonKiller) ActingPlayer() creature.DeathActor { return k.owner }

// ---- from level_test.go ----
func TestLevelTable_Levels(t *testing.T) {
	table, err := NewLevelTable(map[int]Level{
		3: {RequiredExpToLevelUp: 300},
		1: {RequiredExpToLevelUp: 100},
		2: {RequiredExpToLevelUp: 200},
	})
	if err != nil {
		t.Fatalf("NewLevelTable() error: %v", err)
	}

	levels := table.Levels()
	if len(levels) != table.Count() {
		t.Fatalf("Levels() returned %d entries, Count() = %d", len(levels), table.Count())
	}
	if !sort.IntsAreSorted(levels) {
		t.Fatalf("Levels() not sorted ascending: %v", levels)
	}
	if want := []int{1, 2, 3}; !equalInts(levels, want) {
		t.Fatalf("Levels() = %v, want %v", levels, want)
	}
}

// ---- from progression_test.go ----
// realPlayerLevelExp holds the requiredExpToLevelUp value for every level
// 1-81 of the shipped player level table (level 81 is the sentinel entry
// that closes level 80's experience band). Values generated by running the
// reference level-progression logic against the shipped data file.
var realPlayerLevelExp = []int64{
	0, 68, 363, 1168, 2884, 6038, 11287, 19423, 31378, 48229,
	71201, 101676, 141192, 191452, 254327, 331864, 426284, 539995, 675590, 835854,
	1023775, 1242536, 1495531, 1786365, 2118860, 2497059, 2925229, 3407873, 3949727, 4555766,
	5231213, 5981539, 6812472, 7729999, 8740372, 9850111, 11066012, 12395149, 13844879, 15422851,
	17137002, 18995573, 21007103, 23180442, 25524751, 28049509, 30764519, 33679907, 36806133, 40153995,
	45524865, 51262204, 57383682, 63907585, 70852742, 80700339, 91162131, 102265326, 114038008, 126509030,
	146307211, 167243291, 189363788, 212716741, 237351413, 271973532, 308441375, 346825235, 387197529, 429632402,
	474205751, 532692055, 606319094, 696376867, 804219972, 931275828, 1151275834, 1511275834, 2099275834, 4200000000,
	6299994999,
}

func realLevelTable(t *testing.T) *LevelTable {
	t.Helper()
	levels := make(map[int]Level, len(realPlayerLevelExp))
	for i, exp := range realPlayerLevelExp {
		levels[i+1] = Level{RequiredExpToLevelUp: exp}
	}
	table, err := NewLevelTable(levels)
	if err != nil {
		t.Fatalf("realLevelTable: %v", err)
	}
	return table
}

// levelStepTemplate returns a template whose HP/MP/CP tables carry a
// distinct, easily recognizable value per level, so a test can assert
// which table row a level-up resync picked up.
func levelStepTemplate(levels int) *Template {
	hp := make([]float64, levels)
	mp := make([]float64, levels)
	cp := make([]float64, levels)
	for i := 0; i < levels; i++ {
		hp[i] = 100 + float64(i)*10
		mp[i] = 50 + float64(i)*5
		cp[i] = 20 + float64(i)*2
	}
	return &Template{HPTable: hp, MPTable: mp, CPTable: cp}
}

func newProgressionCharacter() *Character {
	tmpl := levelStepTemplate(81)
	c := &Character{
		CharLevel: 1,
	}
	c.SetResourceValues(Resources{
		MaxHP: tmpl.HPTable[0], CurrentHP: tmpl.HPTable[0],
		MaxMP: tmpl.MPTable[0], CurrentMP: tmpl.MPTable[0],
		MaxCP: tmpl.CPTable[0], CurrentCP: tmpl.CPTable[0],
	})
	return c
}

// TestCharacter_AddExpAndSp compares Go's exp/sp/level accumulation
// against expected values generated by running the reference
// exp/sp/level-progression logic (PlayableStatus/PlayerStatus's addExp,
// addLevel, addSp) against the shipped player level table.
func TestCharacter_AddExpAndSp(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	tests := []struct {
		name        string
		exp         int64
		sp          int
		wantLevel   int
		wantExp     int64
		wantSP      int
		wantLeveled bool
		wantMaxHP   float64
		wantMaxMP   float64
		wantMaxCP   float64
	}{
		{
			name: "small exp below next threshold stays at level 1", exp: 50, sp: 0,
			wantLevel: 1, wantExp: 50, wantSP: 0, wantLeveled: false,
			wantMaxHP: 100, wantMaxMP: 50, wantMaxCP: 20,
		},
		{
			name: "exp exactly at level 2 threshold levels up", exp: 68, sp: 0,
			wantLevel: 2, wantExp: 68, wantSP: 0, wantLeveled: true,
			wantMaxHP: 110, wantMaxMP: 55, wantMaxCP: 22,
		},
		{
			name: "multi-level jump from 1 to 5", exp: 2884, sp: 0,
			wantLevel: 5, wantExp: 2884, wantSP: 0, wantLeveled: true,
			wantMaxHP: 140, wantMaxMP: 70, wantMaxCP: 28,
		},
		{
			name: "sp only, no exp added", exp: 0, sp: 500,
			wantLevel: 1, wantExp: 0, wantSP: 500, wantLeveled: false,
			wantMaxHP: 100, wantMaxMP: 50, wantMaxCP: 20,
		},
		{
			name: "exp and sp together crossing into level 3", exp: 363, sp: 1200,
			wantLevel: 3, wantExp: 363, wantSP: 1200, wantLeveled: true,
			wantMaxHP: 120, wantMaxMP: 60, wantMaxCP: 24,
		},
		{
			name: "huge exp caps just below the highest level's band", exp: 4611686018427387903, sp: 0,
			wantLevel: 80, wantExp: 6299994998, wantSP: 0, wantLeveled: true,
			wantMaxHP: 890, wantMaxMP: 445, wantMaxCP: 178,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newProgressionCharacter()
			leveled := c.AddExpAndSp(table, tmpl, tt.exp, tt.sp)

			if leveled != tt.wantLeveled {
				t.Errorf("AddExpAndSp() leveled = %v, want %v", leveled, tt.wantLeveled)
			}
			if c.CharLevel != tt.wantLevel {
				t.Errorf("Level = %d, want %d", c.CharLevel, tt.wantLevel)
			}
			if c.Exp != tt.wantExp {
				t.Errorf("Exp = %d, want %d", c.Exp, tt.wantExp)
			}
			if c.SP != tt.wantSP {
				t.Errorf("SP = %d, want %d", c.SP, tt.wantSP)
			}
			res := c.ResourceValues()
			if res.MaxHP != tt.wantMaxHP || res.CurrentHP != tt.wantMaxHP {
				t.Errorf("MaxHP/CurHP = %v/%v, want %v", res.MaxHP, res.CurrentHP, tt.wantMaxHP)
			}
			if res.MaxMP != tt.wantMaxMP || res.CurrentMP != tt.wantMaxMP {
				t.Errorf("MaxMP/CurMP = %v/%v, want %v", res.MaxMP, res.CurrentMP, tt.wantMaxMP)
			}
			if res.MaxCP != tt.wantMaxCP || res.CurrentCP != tt.wantMaxCP {
				t.Errorf("MaxCP/CurCP = %v/%v, want %v", res.MaxCP, res.CurrentCP, tt.wantMaxCP)
			}
		})
	}
}

// TestCharacter_AddSp_Saturates matches the reference SP accumulator's
// 32-bit signed integer ceiling.
func TestCharacter_AddSp_Saturates(t *testing.T) {
	c := newProgressionCharacter()
	c.SP = maxSP - 5

	c.AddSp(100)

	if c.SP != maxSP {
		t.Errorf("SP = %d, want %d", c.SP, maxSP)
	}
}

// TestCharacter_AddExp_OverflowGuard matches the reference accumulator's
// no-op behavior when an addition would overflow experience negative.
func TestCharacter_AddExp_OverflowGuard(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	c := newProgressionCharacter()
	c.CharLevel = 80
	c.Exp = 6000000000

	leveled := c.AddExp(table, tmpl, 1<<63-1)

	if leveled {
		t.Errorf("AddExp() leveled = true, want false (overflow no-op)")
	}
	if c.Exp != 6000000000 {
		t.Errorf("Exp = %d, want unchanged 6000000000", c.Exp)
	}
	if c.CharLevel != 80 {
		t.Errorf("Level = %d, want unchanged 80", c.CharLevel)
	}
}

// TestCharacter_RemoveExpAndSp compares Go's exp/sp/level removal against
// values generated by running the reference removeExp/removeSp/addLevel
// logic against the shipped player level table.
func TestCharacter_RemoveExpAndSp(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	t.Run("removing exp delevels and does not refill HP/MP/CP", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddExpAndSp(table, tmpl, 363, 0) // level 3, exp 363, HP/MP/CP at level 3's row

		c.RemoveExpAndSp(table, tmpl, 300, 0) // exp -> 63, under level 2's 68 threshold

		if c.CharLevel != 1 {
			t.Errorf("Level = %d, want 1", c.CharLevel)
		}
		if c.Exp != 63 {
			t.Errorf("Exp = %d, want 63", c.Exp)
		}
		// A delevel never refills HP/MP/CP: the level-3 row values persist.
		res := c.ResourceValues()
		if res.MaxHP != 120 || res.MaxMP != 60 || res.MaxCP != 24 {
			t.Errorf("MaxHP/MP/CP = %v/%v/%v, want 120/60/24 (unrefilled)", res.MaxHP, res.MaxMP, res.MaxCP)
		}
	})

	t.Run("removing more exp than held floors at 1, not 0", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddExpAndSp(table, tmpl, 50, 0)

		c.RemoveExpAndSp(table, tmpl, 1000, 0)

		if c.Exp != 1 {
			t.Errorf("Exp = %d, want 1", c.Exp)
		}
		if c.CharLevel != 1 {
			t.Errorf("Level = %d, want 1", c.CharLevel)
		}
	})

	t.Run("removing more sp than held floors at 0", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddExpAndSp(table, tmpl, 0, 30)

		c.RemoveExpAndSp(table, tmpl, 0, 100)

		if c.SP != 0 {
			t.Errorf("SP = %d, want 0", c.SP)
		}
	})
}

// TestCharacter_AddLevel_RefusesPastRealMax matches the reference
// behavior: attempting to add a level past the level table's real max
// leaves the character entirely untouched instead of clamping.
func TestCharacter_AddLevel_RefusesPastRealMax(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	c := newProgressionCharacter()
	c.CharLevel = table.RealMaxLevel()
	c.Exp = table.RequiredExpForLevel(table.RealMaxLevel())

	leveled := c.AddLevel(table, tmpl, 1)

	if leveled {
		t.Errorf("AddLevel() = true, want false at the real max level")
	}
	if c.CharLevel != table.RealMaxLevel() {
		t.Errorf("Level = %d, want unchanged %d", c.CharLevel, table.RealMaxLevel())
	}
}

// TestCharacter_AddLevel_Direct exercises AddLevel outside the
// exp-accumulation path (e.g. a level granted directly rather than earned
// through experience).
func TestCharacter_AddLevel_Direct(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	c := newProgressionCharacter()
	leveled := c.AddLevel(table, tmpl, 1)

	if !leveled {
		t.Fatal("AddLevel(+1) from level 1: want leveled up")
	}
	if c.CharLevel != 2 {
		t.Errorf("Level = %d, want 2", c.CharLevel)
	}
	if c.Exp != 68 {
		t.Errorf("Exp = %d, want 68 (resynced to level 2's threshold)", c.Exp)
	}
	res := c.ResourceValues()
	if res.MaxHP != 110 || res.MaxMP != 55 || res.MaxCP != 22 {
		t.Errorf("MaxHP/MP/CP = %v/%v/%v, want 110/55/22", res.MaxHP, res.MaxMP, res.MaxCP)
	}
}

func TestCharacter_AddLevel_RefillsStatCalculatedVitals(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)
	c := newProgressionCharacter()
	c.AttachRuntime(tmpl, nil)
	c.AddStatFuncs([]effect.Mod{
		{Stat: stat.MaxHP, Op: effect.OpMul, Value: 2, Owner: testModOwner()},
		{Stat: stat.MaxMP, Op: effect.OpMul, Value: 2, Owner: testModOwner()},
		{Stat: stat.MaxCP, Op: effect.OpMul, Value: 2, Owner: testModOwner()},
	})

	c.AddLevel(table, tmpl, 1)

	if got := c.ResourceValues(); got.CurrentHP != got.MaxHP || got.CurrentMP != got.MaxMP || got.CurrentCP != got.MaxCP {
		t.Fatalf("ResourceValues() after stat-modified level-up = %+v, want every current value at its stat-calculated max", got)
	}
}

// TestCharacter_AddLevel_NilTemplateSkipsRefill covers the case where a
// caller doesn't have (or need) the profession template on hand: the level
// and experience still update, but HP/MP/CP are left alone.
func TestCharacter_AddLevel_NilTemplateSkipsRefill(t *testing.T) {
	table := realLevelTable(t)

	c := newProgressionCharacter()
	leveled := c.AddLevel(table, nil, 1)

	if !leveled {
		t.Fatal("AddLevel(+1) with nil template: want leveled up")
	}
	if c.CharLevel != 2 {
		t.Errorf("Level = %d, want 2", c.CharLevel)
	}
	if res := c.ResourceValues(); res.MaxHP != 100 {
		t.Errorf("MaxHP = %v, want unchanged 100 (no template to resync from)", res.MaxHP)
	}
}

func TestLevelTable_RequiredExpForLevel(t *testing.T) {
	table := realLevelTable(t)

	if got := table.RequiredExpForLevel(3); got != 363 {
		t.Errorf("RequiredExpForLevel(3) = %d, want 363", got)
	}
	if got := table.RequiredExpForLevel(9999); got != 0 {
		t.Errorf("RequiredExpForLevel(9999) = %d, want 0 (absent entry)", got)
	}
}

func TestLevelTable_ExpSpanAtLevel(t *testing.T) {
	table := realLevelTable(t)

	if got, want := table.ExpSpanAtLevel(1), int64(68); got != want {
		t.Errorf("ExpSpanAtLevel(1) = %d, want %d", got, want)
	}
	// At/above the level cap, the span falls back to the top band's width
	// (level 80 to level 81's threshold).
	wantTopSpan := realPlayerLevelExp[80] - realPlayerLevelExp[79]
	if got := table.ExpSpanAtLevel(table.RealMaxLevel()); got != wantTopSpan {
		t.Errorf("ExpSpanAtLevel(realMaxLevel) = %d, want %d", got, wantTopSpan)
	}
	if got := table.ExpSpanAtLevel(table.MaxLevel()); got != wantTopSpan {
		t.Errorf("ExpSpanAtLevel(maxLevel) = %d, want %d", got, wantTopSpan)
	}
}

// TestAddExpAndSpNotifiesGain pins the reward message an addition owes the
// player. The hook fires once per attempt that was not fully rejected —
// including one adding nothing — and carries the raw requested amounts, which
// are what the message reports.
func TestAddExpAndSpNotifiesGain(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	type gain struct {
		exp int64
		sp  int
	}
	tests := []struct {
		name string
		exp  int64
		sp   int
		want []gain
	}{
		{name: "experience only", exp: 100, sp: 0, want: []gain{{100, 0}}},
		{name: "sp only", exp: 0, sp: 25, want: []gain{{0, 25}}},
		{name: "both", exp: 100, sp: 25, want: []gain{{100, 25}}},
		{name: "neither", exp: 0, sp: 0, want: []gain{{0, 0}}},
		{name: "both rejected", exp: -1, sp: -1, want: nil},
		{name: "experience rejected", exp: -1, sp: 25, want: []gain{{-1, 25}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newProgressionCharacter()
			var got []gain
			c.SetExpSpGainNotifier(func(exp int64, sp int) {
				got = append(got, gain{exp, sp})
			})
			c.AddExpAndSp(table, tmpl, tc.exp, tc.sp)
			if len(got) != len(tc.want) {
				t.Fatalf("gain notifications = %v, want %v", got, tc.want)
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("gain notification %d = %v, want %v", i, got[i], want)
				}
			}
		})
	}
}

// TestAddExpAndSpNotifiesGainAtSPCeiling pins the one case where an addition
// stays silent despite a non-negative SP amount: SP already at the ceiling
// with no experience added leaves nothing to report.
func TestAddExpAndSpNotifiesGainAtSPCeiling(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	c := newProgressionCharacter()
	c.SP = maxSP
	notifications := 0
	c.SetExpSpGainNotifier(func(int64, int) { notifications++ })
	c.AddExpAndSp(table, tmpl, -1, 25)
	if notifications != 0 {
		t.Errorf("gain notifications = %d, want 0", notifications)
	}
}

// TestRewardExpAndSpNotifiesGain covers the table-less reward fallback, which
// applies SP only and must still report it.
func TestRewardExpAndSpNotifiesGain(t *testing.T) {
	c := newProgressionCharacter()
	var got []int
	c.SetExpSpGainNotifier(func(exp int64, sp int) {
		if exp != 0 {
			t.Errorf("gain notification exp = %d, want 0", exp)
		}
		got = append(got, sp)
	})
	c.RewardExpAndSp(nil, 100, 25)
	if len(got) != 1 || got[0] != 25 {
		t.Errorf("gain notifications = %v, want [25]", got)
	}
}

// TestAddLevelBroadcastsLevelUp pins the level-up animation and message: they
// belong to an actual increase, never to a level drop.
func TestAddLevelBroadcastsLevelUp(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	t.Run("increase", func(t *testing.T) {
		c := newProgressionCharacter()
		broadcasts := 0
		c.SetLevelUpBroadcaster(func() { broadcasts++ })
		if !c.AddLevel(table, tmpl, 1) {
			t.Fatal("AddLevel(1) = false, want true")
		}
		if broadcasts != 1 {
			t.Errorf("level-up broadcasts = %d, want 1", broadcasts)
		}
	})

	t.Run("decrease", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddLevel(table, tmpl, 5)
		broadcasts := 0
		c.SetLevelUpBroadcaster(func() { broadcasts++ })
		if c.AddLevel(table, tmpl, -1) {
			t.Fatal("AddLevel(-1) = true, want false")
		}
		if broadcasts != 0 {
			t.Errorf("level-up broadcasts = %d, want 0", broadcasts)
		}
	})
}

// TestAddLevelRefreshesLevelEntitlements pins the refresh a level change
// owes: what a level entitles a character to is re-derived, so a drop has to
// run the same refresh a gain does, and the UserInfo that follows has to
// describe the already-refreshed character.
func TestAddLevelRefreshesLevelEntitlements(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	for _, tc := range []struct {
		name  string
		delta int
	}{
		{"increase", 1},
		{"decrease", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newProgressionCharacter()
			c.AddLevel(table, tmpl, 20)

			var order []string
			c.SetLevelRefresher(func() { order = append(order, "refresh") })
			c.SetUserInfoUpdater(func() { order = append(order, "userinfo") })

			c.AddLevel(table, tmpl, tc.delta)
			if want := []string{"refresh", "userinfo"}; !slices.Equal(order, want) {
				t.Errorf("hook calls = %v, want %v", order, want)
			}
		})
	}

	// A refused change — one that would push past the real max level —
	// leaves the character alone, so it owes neither hook.
	t.Run("refused", func(t *testing.T) {
		c := newProgressionCharacter()
		refreshes, updates := 0, 0
		c.SetLevelRefresher(func() { refreshes++ })
		c.SetUserInfoUpdater(func() { updates++ })
		if c.AddLevel(table, tmpl, table.RealMaxLevel()+1) {
			t.Fatal("AddLevel past the real max reported an increase")
		}
		if refreshes != 0 || updates != 0 {
			t.Errorf("refreshes = %d, UserInfo updates = %d, want 0 and 0", refreshes, updates)
		}
	})
}

// TestRemoveExpAndSpNotifiesLoss pins the decrease messages and the status
// broadcast a level-dropping removal owes nearby clients.
func TestRemoveExpAndSpNotifiesLoss(t *testing.T) {
	table := realLevelTable(t)
	tmpl := levelStepTemplate(81)

	t.Run("without level drop", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddExpAndSp(table, tmpl, table.RequiredExpForLevel(10)+1000, 1000)
		var lost [][2]int64
		broadcasts := 0
		c.SetExpSpLossNotifier(func(exp int64, sp int) {
			lost = append(lost, [2]int64{exp, int64(sp)})
		})
		c.SetStatusBroadcaster(func() { broadcasts++ })
		c.RemoveExpAndSp(table, tmpl, 10, 25)
		if len(lost) != 1 || lost[0] != [2]int64{10, 25} {
			t.Errorf("loss notifications = %v, want [[10 25]]", lost)
		}
		if broadcasts != 0 {
			t.Errorf("status broadcasts = %d, want 0", broadcasts)
		}
	})

	t.Run("with level drop", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddExpAndSp(table, tmpl, table.RequiredExpForLevel(10), 1000)
		before := c.CharLevel
		notifications, broadcasts := 0, 0
		c.SetExpSpLossNotifier(func(int64, int) { notifications++ })
		c.SetStatusBroadcaster(func() { broadcasts++ })
		c.RemoveExpAndSp(table, tmpl, c.Exp, 0)
		if c.CharLevel >= before {
			t.Fatalf("CharLevel = %d, want below %d", c.CharLevel, before)
		}
		if notifications != 1 {
			t.Errorf("loss notifications = %d, want 1", notifications)
		}
		if broadcasts != 1 {
			t.Errorf("status broadcasts = %d, want 1", broadcasts)
		}
	})

	t.Run("nothing removed", func(t *testing.T) {
		c := newProgressionCharacter()
		notifications := 0
		c.SetExpSpLossNotifier(func(int64, int) { notifications++ })
		c.RemoveExpAndSp(table, tmpl, 0, 0)
		if notifications != 0 {
			t.Errorf("loss notifications = %d, want 0", notifications)
		}
	})
}

// TestRewardExpAndSpUpdatesUserInfo pins the client notification a kill
// reward owes the player: without it the client keeps showing the
// experience, SP and level it was last told about.
func TestRewardExpAndSpUpdatesUserInfo(t *testing.T) {
	table := realLevelTable(t)

	t.Run("with level table, no level gained", func(t *testing.T) {
		c := newProgressionCharacter()
		c.AddExpAndSp(table, nil, table.RequiredExpForLevel(10), 0)
		updates := 0
		c.SetUserInfoUpdater(func() { updates++ })
		c.RewardExpAndSp(table, 1, 10)
		if updates != 1 {
			t.Errorf("UserInfo updates = %d, want 1", updates)
		}
	})

	// A reward that levels the character sends UserInfo twice: the level
	// change pushes one describing the new level, and the experience add
	// pushes its own afterwards. Both are self-only and descriptive, so the
	// second restates the first rather than contradicting it.
	t.Run("with level table, level gained", func(t *testing.T) {
		c := newProgressionCharacter()
		updates := 0
		c.SetUserInfoUpdater(func() { updates++ })
		if !c.RewardExpAndSp(table, table.RequiredExpForLevel(2), 10) {
			t.Fatal("RewardExpAndSp did not report a level increase")
		}
		if updates != 2 {
			t.Errorf("UserInfo updates = %d, want 2", updates)
		}
	})

	t.Run("without level table", func(t *testing.T) {
		c := newProgressionCharacter()
		updates := 0
		c.SetUserInfoUpdater(func() { updates++ })
		c.RewardExpAndSp(nil, 100, 10)
		if updates != 1 {
			t.Errorf("UserInfo updates = %d, want 1", updates)
		}
	})
}

// ---- from race_test.go ----
func TestClassRace(t *testing.T) {
	tests := []struct {
		classID int
		want    Race
	}{
		{0, RaceHuman},    // Human Fighter (root)
		{9, RaceHuman},    // human fighter line, 2nd tier
		{93, RaceHuman},   // human fighter line, 3rd tier, multiple parent hops to root
		{10, RaceHuman},   // Human Mystic (root)
		{18, RaceElf},     // Elven Fighter (root)
		{25, RaceElf},     // Elven Mystic (root)
		{31, RaceDarkElf}, // Dark Fighter (root)
		{38, RaceDarkElf}, // Dark Mystic (root)
		{44, RaceOrc},     // Orc Fighter (root)
		{49, RaceOrc},     // Orc Mystic (root)
		{53, RaceDwarf},   // Dwarven Fighter (root)
		{57, RaceDwarf},   // Warsmith, dwarf line, 2nd tier
		{118, RaceDwarf},  // 3rd tier id parented under the dwarf line
	}
	for _, tt := range tests {
		got, ok := ClassRace(tt.classID)
		if !ok {
			t.Errorf("ClassRace(%d) reported unknown, want %v", tt.classID, tt.want)
			continue
		}
		if got != tt.want {
			t.Errorf("ClassRace(%d) = %v, want %v", tt.classID, got, tt.want)
		}
	}
}

func TestRaceBreathMultiplier(t *testing.T) {
	tests := map[Race]float64{
		RaceHuman:   1,
		RaceElf:     1.5,
		RaceDarkElf: 1.5,
		RaceOrc:     0.9,
		RaceDwarf:   0.8,
	}
	for race, want := range tests {
		if got := race.BreathMultiplier(); got != want {
			t.Errorf("%v BreathMultiplier() = %v, want %v", race, got, want)
		}
	}
}

func TestClassMage(t *testing.T) {
	for _, classID := range []int{10, 25, 38, 49, 94} {
		if !ClassMage(classID) {
			t.Errorf("ClassMage(%d) = false, want true", classID)
		}
	}
	for _, classID := range []int{0, 18, 31, 44, 53, 93} {
		if ClassMage(classID) {
			t.Errorf("ClassMage(%d) = true, want false", classID)
		}
	}
}

func TestClassRace_UnknownID(t *testing.T) {
	if _, ok := ClassRace(9999); ok {
		t.Error("ClassRace(9999) reported known, want unknown")
	}
	// 58-87 are reserved dummy ids with no profession behind them.
	if _, ok := ClassRace(70); ok {
		t.Error("ClassRace(70) reported known, want unknown")
	}
}

// ---- from reward_test.go ----
// Expected values were generated by running the reference server's kill
// reward formula (damage-share split, then the level-difference falloff)
// against these same inputs in a standalone probe, not derived by hand from
// the Go implementation under test.
func TestKillRewardExpAndSp(t *testing.T) {
	tests := []struct {
		name                string
		expReward, spReward float64
		levelDiff           int
		damage, totalDamage float64
		wantExp             int64
		wantSp              int
	}{
		{"level match, sole attacker", 1000, 100, 0, 100, 100, 1000, 100},
		{"attacker under-leveled, no bonus", 1000, 100, -3, 100, 100, 1000, 100},
		{"exactly at threshold, no falloff", 1000, 100, 5, 100, 100, 1000, 100},
		{"one level past threshold", 1000, 100, 6, 100, 100, 833, 83},
		{"two levels past threshold", 1000, 100, 7, 100, 100, 694, 69},
		{"five levels past threshold", 1000, 100, 10, 100, 100, 401, 40},
		{"fifteen levels past threshold", 1000, 100, 20, 100, 100, 64, 6},
		{"far past threshold, falls to zero", 1000, 100, 50, 100, 100, 0, 0},
		{"30% damage share", 5000, 500, 0, 30, 100, 1500, 150},
		{"25% damage share", 5000, 500, 0, 25, 100, 1250, 125},
		{"60% damage share with falloff", 5000, 500, 8, 60, 100, 1736, 173},
		{"uneven share and reward values", 12345, 6789, 3, 40, 137, 3604, 1982},
		{"zero reward template", 0, 0, 0, 100, 100, 0, 0},
		{"sp-only reward is zero, exp untouched", 1000, 0, 0, 100, 100, 1000, 0},
		{"exp-only reward is zero, sp zeroed with it", 0, 100, 0, 100, 100, 0, 0},
		{"tiny share far past threshold", 1000, 100, 100, 1, 1, 0, 0},
		{"large reward one level past threshold", 123456, 7654, 6, 999, 1000, 102777, 6371},
		{"exp just below int32 ceiling", 2147483646, 100, 0, 100, 100, 2147483646, 100},
		{"exp just above int32 ceiling saturates", 2147483648, 100, 0, 100, 100, 2147483647, 100},
		{"sp just above int32 ceiling saturates", 1000, 2147483648, 0, 100, 100, 1000, 2147483647},
		{"zero total damage yields no reward", 1000, 100, 0, 0, 0, 0, 0},
		{"negative total damage yields no reward", 1000, 100, 0, 0, -1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExp, gotSp := KillRewardExpAndSp(tt.expReward, tt.spReward, tt.damage, tt.totalDamage, tt.levelDiff)
			if gotExp != tt.wantExp || gotSp != tt.wantSp {
				t.Errorf("KillRewardExpAndSp(%v, %v, %v, %v, %v) = (%d, %d), want (%d, %d)",
					tt.expReward, tt.spReward, tt.damage, tt.totalDamage, tt.levelDiff,
					gotExp, gotSp, tt.wantExp, tt.wantSp)
			}
		})
	}
}

// TestKillRewardExpAndSp_FullShareMatchesTemplate documents the acceptance
// criterion that a sole attacker dealing all the damage, at or below the
// falloff threshold, receives the template's full reward.
func TestKillRewardExpAndSp_FullShareMatchesTemplate(t *testing.T) {
	exp, sp := KillRewardExpAndSp(4000, 250, 77, 77, 0)
	if exp != 4000 || sp != 250 {
		t.Errorf("got (%d, %d), want (4000, 250)", exp, sp)
	}
}

// ---- from sex_test.go ----
func TestParseSex(t *testing.T) {
	tests := []struct {
		in      int32
		want    Sex
		wantErr bool
	}{
		{0, SexMale, false},
		{1, SexFemale, false},
		{2, 0, true},
		{-1, 0, true},
	}
	for _, tt := range tests {
		got, err := ParseSex(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseSex(%d) = %v, nil; want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSex(%d) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseSex(%d) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// ---- from template_test.go ----
func TestTemplateTable_All(t *testing.T) {
	// 0, 10 and 18 are base professions (classParent maps them to -1), so
	// NewTemplateTable needs no other entries to resolve them.
	table, err := NewTemplateTable(map[int]*Template{
		18: {ID: 18},
		0:  {ID: 0},
		10: {ID: 10},
	})
	if err != nil {
		t.Fatalf("NewTemplateTable() error: %v", err)
	}

	all := table.All()
	if len(all) != table.Count() {
		t.Fatalf("All() returned %d templates, Count() = %d", len(all), table.Count())
	}

	var ids []int
	for _, tpl := range all {
		ids = append(ids, tpl.ID)
	}
	if !sort.IntsAreSorted(ids) {
		t.Fatalf("All() not sorted ascending by ID: %v", ids)
	}
	if want := []int{0, 10, 18}; !equalInts(ids, want) {
		t.Fatalf("All() ids = %v, want %v", ids, want)
	}
}

func TestSkillGrantCorrectedCost(t *testing.T) {
	if got := (SkillGrant{Cost: -1}).CorrectedCost(); got != 0 {
		t.Fatalf("CorrectedCost(-1) = %d, want 0", got)
	}
	if got := (SkillGrant{Cost: 50}).CorrectedCost(); got != 50 {
		t.Fatalf("CorrectedCost(50) = %d, want 50", got)
	}
}

func TestTemplateSkillLearning(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 2, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 3, MinLevel: 10, Cost: 370},
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 1405, Level: 1, MinLevel: 5, Cost: -1},
	}}

	if got, ok := tmpl.FindSkillGrant(3, 2); !ok || got.Level != 2 {
		t.Fatalf("FindSkillGrant(3, 2) = %+v, %v; want level 2", got, ok)
	}
	if _, ok := tmpl.FindSkillGrant(3, 4); ok {
		t.Fatal("FindSkillGrant(3, 4) found a missing grant")
	}

	available := tmpl.AvailableSkillGrants(5, SkillLevels{3: 0})
	want := []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 1405, Level: 1, MinLevel: 5, Cost: -1},
	}
	if !equalSkillGrants(available, want) {
		t.Fatalf("AvailableSkillGrants(level 5, known none) = %+v, want %+v", available, want)
	}

	available = tmpl.AvailableSkillGrants(5, SkillLevels{3: 1, 1405: 1})
	want = []SkillGrant{{SkillID: 3, Level: 2, MinLevel: 5, Cost: 50}}
	if !equalSkillGrants(available, want) {
		t.Fatalf("AvailableSkillGrants(level 5, known 3:1) = %+v, want %+v", available, want)
	}

	if got := tmpl.RequiredLevelForNextSkillGrant(5); got != 10 {
		t.Fatalf("RequiredLevelForNextSkillGrant(level 5) = %d, want 10", got)
	}
	if got := tmpl.RequiredLevelForNextSkillGrant(10); got != 0 {
		t.Fatalf("RequiredLevelForNextSkillGrant(level 10) = %d, want 0", got)
	}

	grant, status := tmpl.CheckSkillLearn(5, 49, SkillLevels{}, 3, 1)
	if status != LearnNeedsSP || grant.CorrectedCost() != 50 {
		t.Fatalf("CheckSkillLearn(not enough SP) = %+v, %v; want cost 50 and LearnNeedsSP", grant, status)
	}

	grant, status = tmpl.CheckSkillLearn(5, 50, SkillLevels{}, 3, 1)
	if status != LearnAllowed || grant.SkillID != 3 || grant.Level != 1 {
		t.Fatalf("CheckSkillLearn(enough SP) = %+v, %v; want skill 3 level 1 and LearnAllowed", grant, status)
	}

	grant, status = tmpl.CheckSkillLearn(5, 0, SkillLevels{}, 1405, 1)
	if status != LearnAllowed || grant.CorrectedCost() != 0 {
		t.Fatalf("CheckSkillLearn(corrected zero cost) = %+v, %v; want allowed zero-cost grant", grant, status)
	}

	if _, status = tmpl.CheckSkillLearn(5, 1000, SkillLevels{3: 0}, 3, 2); status != LearnUnavailable {
		t.Fatalf("CheckSkillLearn(skipped previous level) = %v, want LearnUnavailable", status)
	}
}

// TestTemplateSkillLearningIgnoresMinLevelAndZeroCost pins that CheckSkillLearn
// does not reject on MinLevel or a zero Cost: RequestAcquireSkill.java's
// general-learn case (lines 59-101) locates the node via
// PlayerTemplate.findSkill, which ignores minLvl (PlayerTemplate.java:193-196),
// and only rejects on known-level continuity and SP vs getCorrectedCost() — it
// has no level or cost==0 gate of its own.
func TestTemplateSkillLearningIgnoresMinLevelAndZeroCost(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 20, Cost: 50},
		{SkillID: 249, Level: 1, MinLevel: 1, Cost: 0},
	}}

	if _, status := tmpl.CheckSkillLearn(1, 0, SkillLevels{}, 3, 1); status != LearnNeedsSP {
		t.Fatalf("CheckSkillLearn(character level 1 below grant MinLevel 20, 0 SP) = %v, want LearnNeedsSP (no MinLevel gate)", status)
	}
	grant, status := tmpl.CheckSkillLearn(1, 50, SkillLevels{}, 3, 1)
	if status != LearnAllowed || grant.SkillID != 3 {
		t.Fatalf("CheckSkillLearn(character level 1 below grant MinLevel 20, enough SP) = %+v, %v; want LearnAllowed", grant, status)
	}

	grant, status = tmpl.CheckSkillLearn(1, 0, SkillLevels{}, 249, 1)
	if status != LearnAllowed || grant.SkillID != 249 || grant.CorrectedCost() != 0 {
		t.Fatalf("CheckSkillLearn(cost-0 grant requested manually) = %+v, %v; want LearnAllowed (no cost==0 gate)", grant, status)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTemplateAutoGetSkillGrants pins which grants a level hands over for
// free: only an exact cost of 0, only the highest level unlocked per skill,
// and only where the character is not already at or above that level.
func TestTemplateAutoGetSkillGrants(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 249, Level: 3, MinLevel: 20, Cost: 0},
		// Bought, and the -1 variant that merely displays a price of 0.
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 1405, Level: 1, MinLevel: 5, Cost: -1},
	}}

	got := tmpl.AutoGetSkillGrants(10, SkillLevels{})
	want := []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
	}
	if !equalSkillGrants(got, want) {
		t.Fatalf("AutoGetSkillGrants(level 10, known none) = %+v, want %+v", got, want)
	}

	got = tmpl.AutoGetSkillGrants(10, SkillLevels{194: 1, 249: 2})
	if len(got) != 0 {
		t.Fatalf("AutoGetSkillGrants(level 10, already granted) = %+v, want none", got)
	}

	got = tmpl.AutoGetSkillGrants(10, SkillLevels{249: 1})
	want = []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
	}
	if !equalSkillGrants(got, want) {
		t.Fatalf("AutoGetSkillGrants(level 10, known 249:1) = %+v, want %+v", got, want)
	}
}

func TestTemplateAllAvailableSkillGrants(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 194, Level: 1, MinLevel: 1, Cost: 0},
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 249, Level: 1, MinLevel: 5, Cost: 0},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
		{SkillID: 3, Level: 2, MinLevel: 10, Cost: -1},
		{SkillID: 249, Level: 3, MinLevel: 20, Cost: 0},
	}}

	got := tmpl.AllAvailableSkillGrants(10, SkillLevels{194: 1, 249: 1})
	want := []SkillGrant{
		{SkillID: 3, Level: 2, MinLevel: 10, Cost: -1},
		{SkillID: 249, Level: 2, MinLevel: 10, Cost: 0},
	}
	if !equalSkillGrants(got, want) {
		t.Fatalf("AllAvailableSkillGrants(level 10, known 194:1/249:1) = %+v, want %+v", got, want)
	}
}

// TestTemplateReachableSkillGrants pins the nine-level slack every skill but
// expertise keeps, so a small level loss does not strip skills the character
// legitimately learned.
func TestTemplateReachableSkillGrants(t *testing.T) {
	tmpl := &Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
		{SkillID: 3, Level: 2, MinLevel: 20, Cost: 50},
		{SkillID: 239, Level: 1, MinLevel: 20, Cost: 0},
		{SkillID: 239, Level: 2, MinLevel: 40, Cost: 0},
	}}

	// At level 15 the lookahead reaches skill 3's level-20 grant, but
	// expertise stays pinned to the level itself and so has no grant yet.
	reachable := tmpl.ReachableSkillGrants(15)
	if got, ok := reachable[3]; !ok || got.Level != 2 {
		t.Fatalf("ReachableSkillGrants(15)[3] = %+v, %v; want level 2", got, ok)
	}
	if got, ok := reachable[239]; ok {
		t.Fatalf("ReachableSkillGrants(15)[239] = %+v; want no expertise grant", got)
	}

	if got, ok := tmpl.ReachableSkillGrants(20)[239]; !ok || got.Level != 1 {
		t.Fatalf("ReachableSkillGrants(20)[239] = %+v, %v; want level 1", got, ok)
	}
	if got, ok := tmpl.ReachableSkillGrants(40)[239]; !ok || got.Level != 2 {
		t.Fatalf("ReachableSkillGrants(40)[239] = %+v, %v; want level 2", got, ok)
	}

	// One level short of the lookahead, skill 3 falls back to its lower
	// grant rather than dropping out entirely.
	if got, ok := tmpl.ReachableSkillGrants(10)[3]; !ok || got.Level != 1 {
		t.Fatalf("ReachableSkillGrants(10)[3] = %+v, %v; want level 1", got, ok)
	}
	// Far enough below every grant, nothing is reachable at all.
	if reachable := (&Template{Skills: []SkillGrant{
		{SkillID: 3, Level: 1, MinLevel: 20, Cost: 50},
	}}).ReachableSkillGrants(10); len(reachable) != 0 {
		t.Fatalf("ReachableSkillGrants(10) = %+v, want none", reachable)
	}

	if !tmpl.GrantsSkill(3) {
		t.Error("GrantsSkill(3) = false, want true")
	}
	if tmpl.GrantsSkill(4267) {
		t.Error("GrantsSkill(4267) = true, want false for a skill the line never grants")
	}
}

func equalSkillGrants(a, b []SkillGrant) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
