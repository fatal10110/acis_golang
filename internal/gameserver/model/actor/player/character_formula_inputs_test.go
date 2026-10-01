package player

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ---- from character_stats_heal_test.go ----
func TestCharacterHealInputUsesMagicAttackAndHealProficiency(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 49
	c := liveCharacter(1, tmpl, combatItems())
	c.AddStatFuncs([]effect.Mod{{Stat: stat.HealProficiency, Op: effect.OpAdd, Value: 11, Owner: testModOwner()}})

	in, ok := c.HealInput(modelskill.Definition{SkillType: "HEAL", Power: 25})
	if !ok {
		t.Fatal("HealInput() ok = false")
	}
	if in.Power != 25 || in.Proficiency != 11 || in.Static || in.MAtk != int(c.MAtk()) {
		t.Fatalf("HealInput() = %+v, want power 25, proficiency 11, M.Atk %d", in, int(c.MAtk()))
	}
	if amount, want := formulas.HealAmount(in), 41.099019513592786; !closeFloat(amount, want) {
		t.Fatalf("HealAmount() = %v, want %v", amount, want)
	}
	// Class 0 is a fighter: a charged shot does not scale its M.Atk term.
	if in.Scaling != formulas.HealShotScalingNone {
		t.Fatalf("fighter HealInput scaling = %v, want none", in.Scaling)
	}

	static, ok := c.HealInput(modelskill.Definition{SkillType: "HEAL_STATIC", Power: 25})
	if !ok || !static.Static {
		t.Fatalf("HealInput(HEAL_STATIC) = %+v, %v, want static", static, ok)
	}
	if got, want := formulas.HealAmount(static), 25.0+11; !closeFloat(got, want) {
		t.Fatalf("HealAmount(HEAL_STATIC) = %v, want %v", got, want)
	}

	// Human Mystic (10) and its cleric line are mage classes.
	for _, classID := range []int{10, 15, 16} {
		c.SetClassID(classID)
		mage, _ := c.HealInput(modelskill.Definition{SkillType: "HEAL", Power: 25})
		if mage.Scaling != formulas.HealShotScalingMage {
			t.Fatalf("class %d HealInput scaling = %v, want mage", classID, mage.Scaling)
		}
	}
}

// ---- from character_stats_shield_test.go ----
type shieldDefenseResolver interface {
	ShieldDefense(caster creature.FormulaActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense
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
	target.perfectShieldBlockRate = 15
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

			rec := recordEvents(target)

			target.ShieldDefense(caster, def, false)

			var gotSuccess, gotPerfect bool
			for _, e := range event.Of[event.ShieldBlocked](rec) {
				gotSuccess = gotSuccess || !e.Perfect
				gotPerfect = gotPerfect || e.Perfect
			}

			if gotSuccess != tt.wantSuccess {
				t.Fatalf("shield block success notice fired = %v, want %v", gotSuccess, tt.wantSuccess)
			}
			if gotPerfect != tt.wantPerfect {
				t.Fatalf("shield block perfect notice fired = %v, want %v", gotPerfect, tt.wantPerfect)
			}
		})
	}
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
	// sqrt(M.Atk) / M.Def * 11 over the truncated stats: M.Atk 53.1441 -> 53,
	// M.Def 85.12 -> 85.
	if want := math.Sqrt(53) / 85 * 11; !closeFloat(in.MAtkModifier, want) {
		t.Fatalf("MAtkModifier = %v, want %v", in.MAtkModifier, want)
	}
	if want := 1 + 0.01*float64(def.MagicLevel+def.LevelDepend-target.CharLevel); !closeFloat(in.LevelModifier, want) {
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
