package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func TestCharacterSkillDamageInputsUseElementalSkillModifier(t *testing.T) {
	tmpl := combatTemplate()
	tmpl.MAtk = 25
	tmpl.MDef = 40
	caster := liveCharacter(1, tmpl, combatItems())
	target := liveCharacter(2, tmpl, combatItems())
	owner := &struct{}{}
	target.AddStatFuncs([]basefunc.Func{basefunc.NewMul(owner, stat.FireRes, 0.75, nil)})

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

	caster.SetRollSource(func(int) int { return 7 })
	magic, ok := target.MagicDamageInput(caster, modelskill.Definition{Power: 40, SkillType: "MDAM"})
	if !ok {
		t.Fatal("MagicDamageInput() ok = false")
	}
	if !magic.MagicCrit {
		t.Fatal("MagicDamageInput MagicCrit = false, want true for roll below mCrit rate")
	}

	caster.SetRollSource(func(int) int { return 8 })
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

	in, ok := target.BlowInput(caster, modelskill.Definition{ID: 1, Power: 30, SkillType: "BLOW", BaseLandRate: 1000})
	if !ok {
		t.Fatal("BlowInput() ok = false")
	}
	if !in.Landed {
		t.Fatal("BlowInput().Landed = false, want true for a capped ordinary blow")
	}
}

func TestCharacterBlowInputCarriesShieldDefense(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{}, 0)
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewSet(target, stat.ShieldRate, 100, nil),
		basefunc.NewSet(target, stat.ShieldDefenceAngle, 360, nil),
		basefunc.NewSet(target, stat.ShieldDefence, 30, nil),
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
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewSet(target, stat.ShieldRate, 100, nil),
		basefunc.NewSet(target, stat.ShieldDefenceAngle, 360, nil),
		basefunc.NewSet(target, stat.ShieldDefence, 30, nil),
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
	target.AddStatFuncs([]basefunc.Func{
		basefunc.NewSet(target, stat.ShieldRate, 100, nil),
		basefunc.NewSet(target, stat.ShieldDefenceAngle, 360, nil),
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
	owner := &struct{}{}
	caster.AddStatFuncs([]basefunc.Func{
		basefunc.NewMul(owner, stat.PvPPhysSkillDmg, 0.8, nil),
		basefunc.NewMul(owner, stat.PvPMagicalDmg, 1.3, nil),
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
