package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

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

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Element: modelskill.ElementFire}, true)
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !closeFloat(magic.ElementalMul, 0.75) {
		t.Fatalf("MagicDamageInput ElementalMul = %v, want 0.75", magic.ElementalMul)
	}

	neutral, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
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
	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
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
	magic, ok = target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
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

func TestCharacterMagicDamageInputCarriesShieldDefense(t *testing.T) {
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
	caster.SetRollSource(func(n int) int {
		switch n {
		case 1000, 10000:
			return 9999
		default:
			return 0
		}
	})

	for _, tt := range []struct {
		name string
		roll int
		want formulas.ShieldDefense
		mdef float64
	}{
		{"success", 10, formulas.ShieldSuccess, target.MDef() + target.CalcStat(stat.ShieldDefence, 0)},
		{"perfect", 0, formulas.ShieldPerfect, target.MDef()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target.SetRollSource(func(n int) int {
				if n != 100 {
					t.Fatalf("shield roll bound = %d, want 100", n)
				}
				return tt.roll
			})
			in, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
			if !ok {
				t.Fatal("MagicDamageInput() ok = false")
			}
			if in.Shield != tt.want {
				t.Fatalf("MagicDamageInput shield = %v, want %v", in.Shield, tt.want)
			}
			if !closeFloat(in.MDef, tt.mdef) {
				t.Fatalf("MagicDamageInput MDef = %v, want %v", in.MDef, tt.mdef)
			}
		})
	}
}

func TestCharacterMagicDamageInputPerfectShieldSkipsMagicFailure(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 100, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()},
	})
	target.SetRollSource(func(n int) int {
		if n != 100 {
			t.Fatalf("shield roll bound = %d, want 100", n)
		}
		return 0
	})
	magicRolls := 0
	caster.SetRollSource(func(n int) int {
		if n == 10000 {
			magicRolls++
			return 0
		}
		return 9999
	})

	in, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if in.Shield != formulas.ShieldPerfect {
		t.Fatalf("shield = %v, want ShieldPerfect", in.Shield)
	}
	if in.Failure != formulas.MagicFailureNone {
		t.Fatalf("Failure = %v, want MagicFailureNone on perfect shield", in.Failure)
	}
	if magicRolls != 0 {
		t.Fatalf("magic-success rolls = %d, want 0 on perfect shield", magicRolls)
	}
}

func TestCharacterMagicDamageInputShieldIgnoresMagicCrit(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()},
	})
	target.SetRollSource(func(n int) int {
		if n != 100 {
			t.Fatalf("shield roll bound = %d, want 100", n)
		}
		return 30
	})
	caster.SetRollSource(func(n int) int {
		if n == 1000 {
			return 0
		}
		return 9999
	})

	in, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !in.MagicCrit {
		t.Fatal("MagicCrit = false, want true so a leaked isCrit would triple the shield rate")
	}
	if in.Shield != formulas.ShieldFailed {
		t.Fatalf("shield = %v, want ShieldFailed: magic crit must not triple the shield rate", in.Shield)
	}
}

func TestCharacterMagicDamageInputFailureOutcomes(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()

	for _, tt := range []struct {
		name      string
		tgtLevel  int
		first     int
		second    int
		want      formulas.MagicFailure
		wantRolls int
		off       bool
	}{
		// MagicFailures=false skips the resist roll entirely.
		{name: "switch off", tgtLevel: 11, first: 0, second: 0, want: formulas.MagicFailureNone, wantRolls: 0, off: true},
		{name: "half", tgtLevel: 1, first: 0, second: 9999, want: formulas.MagicFailureHalf, wantRolls: 2},
		{name: "full second miss", tgtLevel: 1, first: 0, second: 0, want: formulas.MagicFailureFull, wantRolls: 2},
		{name: "full past gap", tgtLevel: 11, first: 0, second: 9999, want: formulas.MagicFailureFull, wantRolls: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caster := liveCharacter(1, tmpl, items)
			target := liveCharacter(2, tmpl, items)
			target.CharLevel = tt.tgtLevel
			rolls := 0
			caster.SetRollSource(func(n int) int {
				if n == 10000 {
					rolls++
					if rolls == 1 {
						return tt.first
					}
					return tt.second
				}
				if n == 1000 {
					return 0 // magic critical
				}
				return 9999
			})
			in, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, !tt.off)
			if !ok {
				t.Fatal("MagicDamageInput() ok = false")
			}
			if in.Failure != tt.want {
				t.Fatalf("Failure = %v, want %v", in.Failure, tt.want)
			}
			if rolls != tt.wantRolls {
				t.Fatalf("magic-success rolls = %d, want %d", rolls, tt.wantRolls)
			}
			// The rolled critical survives a resist: the damage formula
			// skips it, but the caster's damage feedback still reports it.
			if !in.MagicCrit {
				t.Fatal("MagicCrit = false, want the rolled critical kept")
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

// TestCharacterDamageInputsAcceptInvulnerableTargetAndResolveNoDamagePermission
// pins issue #2333: a skill-damage formula is never gated on the target's
// own invulnerability, which the HP reduction checks after hate registers,
// so an invulnerable target still yields a computed input. MANADAM is the
// target-side exception: it refuses an invulnerable target up front.
//
// On the attacker side (issue #2740) a damage-denied attacker's physical
// and magic inputs still resolve, flagged NoDamage so the formula yields 0
// before any shield or magic-failure outcome; the blow and mana inputs have
// no attacker-permission gate at all.
func TestCharacterDamageInputsAcceptInvulnerableTargetAndResolveNoDamagePermission(t *testing.T) {
	tmpl := combatTemplate()
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	def := modelskill.Definition{Power: 100, Magic: true}

	target.SetSpawnProtection(true)
	if _, ok := target.PhysicalSkillInput(caster, def); !ok {
		t.Fatal("PhysicalSkillInput rejected an invulnerable target")
	}
	if _, ok := target.MagicDamageInput(caster, def, true); !ok {
		t.Fatal("MagicDamageInput rejected an invulnerable target")
	}
	if _, ok := target.BlowInput(caster, def); !ok {
		t.Fatal("BlowInput rejected an invulnerable target")
	}
	if _, ok := target.ManaDamageInput(caster, def); ok {
		t.Fatal("ManaDamageInput accepted an invulnerable target")
	}

	target.SetSpawnProtection(false)
	caster.SetCanGiveDamage(false)
	physical, ok := target.PhysicalSkillInput(caster, def)
	if !ok || !physical.NoDamage || formulas.PhysicalSkillDamage(physical) != 0 {
		t.Fatalf("PhysicalSkillInput from a damage-denied attacker = %+v, %v; want a resolved zero-damage input", physical, ok)
	}
	magic, ok := target.MagicDamageInput(caster, def, true)
	if !ok || !magic.NoDamage || magic.Failure != formulas.MagicFailureNone || formulas.MagicDamage(magic) != 0 {
		t.Fatalf("MagicDamageInput from a damage-denied attacker = %+v, %v; want a resolved zero-damage input with no failure roll", magic, ok)
	}
	// The blow formula has no damage-permission gate: the target's HP
	// reduction refuses the damage instead.
	if _, ok := target.BlowInput(caster, def); !ok {
		t.Fatal("BlowInput rejected an attacker without damage permission")
	}
	if _, ok := target.ManaDamageInput(caster, def); !ok {
		t.Fatal("ManaDamageInput rejected an attacker without damage permission")
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

// TestSelfDamageSkipsOwnDamagePermission gives a character no damage
// permission: its own damage — drowning, its own damage-over-time tick, a
// hit or skill it lands on itself, each through a wrapper — still lands,
// while another attacker's gated damage does not.
func TestSelfDamageSkipsOwnDamagePermission(t *testing.T) {
	tmpl := combatTemplate()
	for _, tc := range []struct {
		name string
		hit  func(c *Character, attacker wrappedSelf)
	}{
		{"drowning", func(c *Character, a wrappedSelf) { c.ReduceHPByDOT(10, a, false) }},
		{"damage over time", func(c *Character, a wrappedSelf) { c.ReduceHPByDOT(10, a, true) }},
		{"skill", func(c *Character, a wrappedSelf) { c.ReduceHP(10, a, modelskill.Definition{}) }},
		{"hit", func(c *Character, a wrappedSelf) { c.TakeDamage(10, a) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := liveCharacter(1, tmpl, combatItems())
			c.SetHP(100)
			c.SetCanGiveDamage(false)
			tc.hit(c, wrappedSelf{c})
			if got := c.HP(); got != 90 {
				t.Fatalf("HP after own damage without damage permission = %v, want 90", got)
			}
		})
	}

	other := liveCharacter(1, tmpl, combatItems())
	c := liveCharacter(2, tmpl, combatItems())
	c.SetHP(100)
	other.SetCanGiveDamage(false)
	c.ReduceHPByDOT(10, other, true)
	c.TakeDamage(10, other)
	if got := c.HP(); got != 100 {
		t.Fatalf("HP after another attacker's damage without permission = %v, want 100", got)
	}
}

// TestInvulnerableCharacterTakesOnlyItsOwnDamageOverTime makes a character
// invulnerable: its own damage-over-time tick still lands, through a
// wrapper; its own drowning, another attacker's tick and a fall do not.
func TestInvulnerableCharacterTakesOnlyItsOwnDamageOverTime(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(2, tmpl, combatItems())
	other := liveCharacter(1, tmpl, combatItems())
	c.SetHP(100)
	c.SetSpawnProtection(true)

	c.ReduceHPByDOT(10, wrappedSelf{c}, false)
	c.ReduceHPByDOT(10, other, true)
	c.ReduceHPByDOT(10, nil, true)
	if got := c.HP(); got != 100 {
		t.Fatalf("HP after drowning, another's tick and a fall while invulnerable = %v, want 100", got)
	}
	c.ReduceHPByDOT(10, wrappedSelf{c}, true)
	if got := c.HP(); got != 90 {
		t.Fatalf("HP after its own damage-over-time tick while invulnerable = %v, want 90", got)
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
	magic, ok := target.MagicDamageInput(spiritCaster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
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
	magic, ok = target.MagicDamageInput(blessedCaster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
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

	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM", Magic: true}, true)
	if !ok {
		t.Fatal("MagicDamageInput(magic) ok = false")
	}
	if !closeFloat(magic.PvPMul, 1.3) {
		t.Fatalf("MagicDamageInput magic PvPMul = %v, want 1.3", magic.PvPMul)
	}

	physicalMagic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"}, true)
	if !ok {
		t.Fatal("MagicDamageInput(physical skill type) ok = false")
	}
	if !closeFloat(physicalMagic.PvPMul, 0.8) {
		t.Fatalf("MagicDamageInput physical PvPMul = %v, want 0.8", physicalMagic.PvPMul)
	}
}
