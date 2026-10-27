package formulas

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// oracleRow is one "<formula> <inputs...> = <result>" line of
// testdata/formulas.golden.
type oracleRow struct {
	t    *testing.T
	line int
	name string
	args []string
	want string
}

func (r oracleRow) f(i int) float64 {
	v, err := strconv.ParseFloat(r.args[i], 64)
	if err != nil {
		r.t.Fatalf("line %d arg %d: %v", r.line, i, err)
	}
	return v
}

func (r oracleRow) n(i int) int {
	v, err := strconv.Atoi(r.args[i])
	if err != nil {
		r.t.Fatalf("line %d arg %d: %v", r.line, i, err)
	}
	return v
}

func (r oracleRow) b(i int) bool {
	v, err := strconv.ParseBool(r.args[i])
	if err != nil {
		r.t.Fatalf("line %d arg %d: %v", r.line, i, err)
	}
	return v
}

func (r oracleRow) wantFloat(got float64) {
	want, err := strconv.ParseFloat(r.want, 64)
	if err != nil {
		r.t.Fatalf("line %d result: %v", r.line, err)
	}
	if got != want && !(math.IsNaN(got) && math.IsNaN(want)) {
		r.t.Errorf("line %d %s%v = %v, want %v", r.line, r.name, r.args, got, want)
	}
}

// wantFloatULP accepts a result within maxULP units in the last place of the
// reference: for formulas built on a non-integer power, whose reference
// result only promises 1 ulp of accuracy and which math.Pow does not
// reproduce bit for bit.
func (r oracleRow) wantFloatULP(got float64, maxULP uint64) {
	want, err := strconv.ParseFloat(r.want, 64)
	if err != nil {
		r.t.Fatalf("line %d result: %v", r.line, err)
	}
	a, b := math.Float64bits(got), math.Float64bits(want)
	if a < b {
		a, b = b, a
	}
	if math.Signbit(got) != math.Signbit(want) || a-b > maxULP {
		r.t.Errorf("line %d %s%v = %v, want %v (within %d ulp)", r.line, r.name, r.args, got, want, maxULP)
	}
}

func (r oracleRow) wantString(got string) {
	if got != r.want {
		r.t.Errorf("line %d %s%v = %s, want %s", r.line, r.name, r.args, got, r.want)
	}
}

// powULP bounds how far a math.Pow-based result may sit from the reference
// (#4318).
const powULP = 4

func shield(perfect bool) ShieldDefense {
	if perfect {
		return ShieldPerfect
	}
	return ShieldFailed
}

// TestFormulasMatchReferenceProbe replays testdata/formulas.golden, the
// output of testdata/oracle/FormulasProbe.java (the reference formula bodies
// with creature and skill lookups turned into parameters; see the README
// there), and requires every formula to reproduce each reference result
// bit for bit.
func TestFormulasMatchReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/formulas.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	seen := map[string]int{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 || fields[len(fields)-2] != "=" {
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
		r := oracleRow{t: t, line: line, name: fields[0], args: fields[1 : len(fields)-2], want: fields[len(fields)-1]}
		seen[r.name]++
		switch r.name {
		case "castBreakRate":
			r.wantFloat(CastBreakRate(r.f(0), r.n(1), nil))
		case "castBreaks":
			r.wantString(strconv.FormatBool(CastBreaks(r.f(0), r.n(1))))
		case "timeBetweenAttacks":
			r.wantString(strconv.Itoa(TimeBetweenAttacks(r.n(0))))
		case "atkSpd":
			r.wantString(strconv.Itoa(AtkSpd(r.b(0), r.n(1), r.n(2), r.f(3))))
		case "hitRate":
			r.wantString(strconv.Itoa(HitRate(r.n(0), r.n(1), r.n(2), r.b(3), r.b(4), r.b(5))))
		case "posMul":
			r.wantFloat(PosMul(r.b(0), r.b(1), r.b(2)))
		case "magicSuccessRate":
			r.wantFloatULP(MagicSuccessRate(r.n(0), r.n(1), r.n(2), r.n(3), r.b(4)), powULP)
		case "physicalAttackDamage":
			r.wantFloat(PhysicalAttackDamage(PhysicalAttackInput{
				Defence: r.f(0), AttackPower: r.f(1), Crit: r.b(2), SoulShot: r.b(3),
				PosMul: r.f(4), ElementalMul: r.f(5), RandomMul: r.f(6), RaceMul: r.f(7), WeaponVulnMul: r.f(8), PvPMul: r.f(9),
				CritDamageMul: r.f(10), CritDamagePosMul: r.f(11), CritVulnMul: r.f(12), CritDamageAddBase: r.f(13),
			}))
		case "skillPower":
			r.wantFloatULP(SkillPowerFor(r.args[0], r.f(1), r.f(2)), powULP)
		case "physicalSkillDamage":
			r.wantFloat(PhysicalSkillDamage(PhysicalSkillInput{
				NoDamage: !r.b(0), Shield: shield(r.b(1)), Defence: r.f(2), AttackPower: r.f(3), SkillPower: r.f(4),
				Crit: r.b(5), SoulShot: r.b(6), RandomMul: r.f(7), RaceMul: r.f(8), PvPMul: r.f(9), ElementalMul: r.f(10), WeaponVulnMul: r.f(11),
			}))
		case "blowRate":
			in := BlowRateInput{BaseRate: r.f(0), DexMul: r.f(1), BlowMul: r.f(2), Behind: r.b(3), Front: r.b(4), Backstab: r.b(5)}
			threshold, err := strconv.ParseFloat(r.want, 64)
			if err != nil {
				t.Fatalf("line %d result: %v", line, err)
			}
			fl := int(math.Floor(threshold))
			for _, roll := range []int{0, fl - 1, fl, fl + 1, 999} {
				if roll < 0 || roll > 999 {
					continue
				}
				if got, want := BlowSucceeds(in, roll), threshold > float64(roll); got != want {
					t.Errorf("line %d BlowSucceeds(%+v, %d) = %v, want %v", line, in, roll, got, want)
				}
			}
		case "blowDamage":
			r.wantFloat(BlowDamage(BlowInput{
				Defence: r.f(0), AttackPower: r.f(1), SkillPower: r.f(2), SoulShot: r.b(3), IsPvP: r.b(4),
				RandomMul: r.f(5), PosMul: r.f(6), PvPMul: r.f(7), CritDamageMul: r.f(8), CritDamagePosMul: r.f(9),
				CritVulnMul: r.f(10), DaggerVulnMul: r.f(11), CritDamageAddBase: r.f(12),
			}))
		case "lethalRate":
			r.wantFloat(LethalRate(LethalInput{MagicLevel: r.n(1), AttackerLevel: r.n(2), TargetLevel: r.n(3), LethalMul: r.f(4)}, int(r.f(0))))
		case "magicFailure":
			r.wantString(strconv.Itoa(int(MagicFailureOutcome(r.b(0), r.b(1), r.b(2), r.b(3), r.n(4)))))
		case "magicDamage":
			r.wantFloat(MagicDamage(MagicDamageInput{
				NoDamage: !r.b(0), Shield: shield(r.b(1)), MDef: r.f(2), MAtk: r.f(3), SkillPower: r.f(4),
				SoulShot: r.b(5), BlessedSoulShot: r.b(6), MagicCrit: r.b(7), Failure: MagicFailure(r.n(8)), PvPMul: r.f(9), ElementalMul: r.f(10),
			}))
		case "cubicMagicDamage":
			r.wantFloat(CubicMagicDamage(CubicMagicDamageInput{
				Shield: shield(r.b(0)), MDef: r.f(1), SkillPower: r.f(2), MagicCrit: r.b(3), Failure: MagicFailure(r.n(4)), ElementalMul: r.f(5),
			}))
		case "cubicMAtkModifier":
			r.wantFloat(CubicMAtkModifier(r.f(0), r.f(1), r.b(2)))
		case "manaDamage":
			r.wantFloat(ManaDamage(ManaDamageInput{
				MAtk: r.f(0), MDef: r.f(1), TargetMaxMp: r.f(2), SkillPower: r.f(3), SoulShot: r.b(4), BlessedSoulShot: r.b(5), VulnMul: r.f(6),
			}))
		case "magicAffected":
			r.wantString(strconv.FormatBool(MagicAffected(MagicAffectedInput{Defended: r.b(0), MDef: r.f(1), MAtk: r.f(2), VulnMul: r.f(3)}, r.f(4))))
		case "shieldUse":
			r.wantString(strconv.Itoa(int(ShieldUse(r.f(0), r.n(1), r.b(2), r.b(3), r.n(4), r.n(5)))))
		case "skillSuccessRate":
			r.wantFloat(SkillSuccessRate(SkillSuccessInput{
				Shield: shield(r.b(0)), IgnoreResists: r.b(1), BaseChance: r.f(2), StatModifier: r.f(3), VulnModifier: r.f(4), MAtkModifier: r.f(5), LevelModifier: r.f(6),
			}))
		case "revivePower":
			r.wantFloat(RevivePower(statbonus.WITBonus[r.n(0)], r.f(1)))
		case "cancelRate":
			r.wantFloat(CancelSuccessRate(r.n(0), r.n(1), r.f(2), r.f(3), r.n(4), r.n(5)))
		case "effectCancelRate":
			r.wantString(strconv.Itoa(EffectCancelSuccessRate(r.n(0), r.n(1), r.n(2), r.f(3), r.f(4))))
		case "effectCancelDebuffRate":
			r.wantString(strconv.Itoa(EffectCancelDebuffSuccessRate(r.n(0), r.n(1), r.n(2), r.f(3))))
		case "harvestRate":
			r.wantString(strconv.Itoa(HarvestSuccessRate(r.n(0) - r.n(1))))
		case "sowRate":
			r.wantString(strconv.Itoa(SowSuccessRate(r.n(0), r.n(1), r.n(2), r.b(3))))
		case "doorUnlockRate":
			r.wantString(strconv.Itoa(DoorUnlockRate(r.n(0))))
		case "chestDeluxeRate":
			r.wantString(strconv.Itoa(ChestUnlockDeluxeKeyRate(r.n(0), r.n(1), r.n(2) == 2065)))
		case "chestRate":
			chance, definite, succeeds := ChestUnlockRate(r.n(0), r.n(1))
			got := strconv.Itoa(chance)
			if definite {
				got = strconv.FormatBool(succeeds)
			}
			r.wantString(got)
		case "healAmount":
			r.wantFloat(HealAmount(HealInput{
				Power: r.f(0), Proficiency: r.f(1), Static: r.b(2), MAtk: r.n(3), Scaling: HealShotScaling(r.n(4)),
				Spiritshot: r.b(5), BlessedSpiritshot: r.b(6), SpsCorrection: r.f(7),
			}))
		default:
			t.Fatalf("line %d: unknown formula %q", line, r.name)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 31 {
		t.Fatalf("golden covers %d formulas, want 31", len(seen))
	}
}
