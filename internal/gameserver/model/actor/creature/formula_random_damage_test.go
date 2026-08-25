package creature

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// randomDamageTestActor is a minimal FormulaActor stub for exercising
// RandomDamageMultiplier in isolation.
type randomDamageTestActor struct {
	level         int
	spread        int
	roll          int
	attackType    item.WeaponType
	pSkillEvasion float64
}

func (a *randomDamageTestActor) Position() (int, int, int)      { return 0, 0, 0 }
func (a *randomDamageTestActor) ObjectID() int32                { return 1 }
func (a *randomDamageTestActor) Heading() int                   { return 0 }
func (a *randomDamageTestActor) Level() int                     { return a.level }
func (a *randomDamageTestActor) STR() int                       { return 0 }
func (a *randomDamageTestActor) CON() int                       { return 0 }
func (a *randomDamageTestActor) DEX() int                       { return 0 }
func (a *randomDamageTestActor) INT() int                       { return 0 }
func (a *randomDamageTestActor) WIT() int                       { return 0 }
func (a *randomDamageTestActor) MEN() int                       { return 0 }
func (a *randomDamageTestActor) PAtk() float64                  { return 0 }
func (a *randomDamageTestActor) PDef() float64                  { return 0 }
func (a *randomDamageTestActor) MAtk() float64                  { return 0 }
func (a *randomDamageTestActor) MDef() float64                  { return 0 }
func (a *randomDamageTestActor) MagicCriticalRate() float64     { return 0 }
func (a *randomDamageTestActor) AttackType() item.WeaponType    { return a.attackType }
func (a *randomDamageTestActor) SoulshotCharged() bool          { return false }
func (a *randomDamageTestActor) SpiritshotCharged() bool        { return false }
func (a *randomDamageTestActor) BlessedSpiritshotCharged() bool { return false }
func (a *randomDamageTestActor) CalcStat(kind stat.Stat, base float64) float64 {
	if kind == stat.PSkillEvasion {
		return a.pSkillEvasion
	}
	return base
}
func (a *randomDamageTestActor) RandomDamageSpread() int { return a.spread }
func (a *randomDamageTestActor) Roll(int) int            { return a.roll }

var _ FormulaActor = (*randomDamageTestActor)(nil)

func TestResolvePhysicalSkillInputSkipsEvasionForUnarmedAndBow(t *testing.T) {
	target := &randomDamageTestActor{pSkillEvasion: 100}
	for _, attackType := range []item.WeaponType{item.WeaponFist, item.WeaponBow} {
		in, ok := ResolvePhysicalSkillInput(&randomDamageTestActor{attackType: attackType}, target, modelskill.Definition{}, false, 1)
		if !ok || in.Evaded {
			t.Fatalf("AttackType %v evaded = %v, ok = %v; want false, true", attackType, in.Evaded, ok)
		}
	}

	in, ok := ResolvePhysicalSkillInput(&randomDamageTestActor{attackType: item.WeaponSword}, target, modelskill.Definition{}, false, 1)
	if !ok || !in.Evaded {
		t.Fatalf("sword evaded = %v, ok = %v; want true, true", in.Evaded, ok)
	}
}

// Mirrors Creature.getRandomDamageMultiplier (Creature.java:1699-1710):
// weaponless attackers (spread == -1, the sentinel) roll `5 + sqrt(level)`
// spread, not a fixed 1x.
func TestRandomDamageMultiplierWeaponlessUsesLevelSpread(t *testing.T) {
	// level 50 -> spread = 5 + int(sqrt(50)) = 5 + 7 = 12
	attacker := &randomDamageTestActor{level: 50, spread: -1, roll: 2*12 + 1 - 1} // max roll
	got := RandomDamageMultiplier(attacker, modelskill.Definition{})
	want := 1 + float64(12)/100
	if got != want {
		t.Fatalf("RandomDamageMultiplier() = %v, want %v (max-roll bound for level 50 weaponless)", got, want)
	}

	attacker = &randomDamageTestActor{level: 50, spread: -1, roll: 0} // min roll
	got = RandomDamageMultiplier(attacker, modelskill.Definition{})
	want = 1 - float64(12)/100
	if got != want {
		t.Fatalf("RandomDamageMultiplier() = %v, want %v (min-roll bound for level 50 weaponless)", got, want)
	}
}

// A weapon-supplied spread still takes priority over the level fallback.
func TestRandomDamageMultiplierWeaponSpreadTakesPriority(t *testing.T) {
	attacker := &randomDamageTestActor{level: 50, spread: 20, roll: 0}
	got := RandomDamageMultiplier(attacker, modelskill.Definition{})
	want := 1 - float64(20)/100
	if got != want {
		t.Fatalf("RandomDamageMultiplier() = %v, want %v (weapon spread should win over level fallback)", got, want)
	}
}

// A weapon with an explicit or defaulted 0 random-damage spread (e.g. item
// 8763 "Elrokian Trap", which has no random_damage attribute) must resolve
// to a neutral 1x, NOT the level fallback — Java's gate is
// `activeWeapon != null`, not "spread > 0" (Creature.java:1699-1709).
func TestRandomDamageMultiplierZeroSpreadWeaponStaysNeutral(t *testing.T) {
	attacker := &randomDamageTestActor{level: 50, spread: 0, roll: 0}
	got := RandomDamageMultiplier(attacker, modelskill.Definition{})
	if got != 1 {
		t.Fatalf("RandomDamageMultiplier() = %v, want 1 (0-spread weapon must not trigger the level fallback)", got)
	}
}
