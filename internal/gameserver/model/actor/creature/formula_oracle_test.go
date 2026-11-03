package creature

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// oracleActor is a FormulaActor built from one reference-probe row: CON and
// MEN, the int M.Atk and M.Def, and one stat func per stat, a multiplier
// (the vulnerability and resistance stats) or an add (the element powers).
type oracleActor struct {
	physicalAttackActor
	con, men   int
	mAtk, mDef float64
	mul, add   map[stat.Stat]float64
}

func (a *oracleActor) CON() int      { return a.con }
func (a *oracleActor) MEN() int      { return a.men }
func (a *oracleActor) MAtk() float64 { return a.mAtk }
func (a *oracleActor) MDef() float64 { return a.mDef }

func (a *oracleActor) CalcStat(s stat.Stat, init float64) float64 {
	if m, ok := a.mul[s]; ok {
		return init * m
	}
	return init + a.add[s]
}

func (a *oracleActor) Against(conditions.Actor, conditions.Skill) SkillStats { return a }

var _ FormulaActor = (*oracleActor)(nil)

// oracleVulnMul is the probe's VULN_MUL: the target's multiplier on each
// vulnerability stat, distinct per stat so a result shows which one applied.
var oracleVulnMul = map[stat.Stat]float64{
	stat.BleedVuln: 0.91, stat.PoisonVuln: 0.83, stat.StunVuln: 0.5, stat.ParalyzeVuln: 0.67, stat.RootVuln: 0.59,
	stat.SleepVuln: 0.47, stat.DerangementVuln: 0.37, stat.DebuffVuln: 0.29, stat.CancelVuln: 1.13,
}

// TestModifiersMatchReferenceProbe replays the creature-level modifier rows
// of the skill formula reference probe (skill/formulas/testdata/oracle) on
// the live-actor resolvers, with each lookup the probe takes as a parameter
// served by an oracleActor, and requires every result bit for bit.
func TestModifiersMatchReferenceProbe(t *testing.T) {
	f, err := os.Open("../../../skill/formulas/testdata/formulas.golden")
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
		name, args, want := fields[0], fields[1:len(fields)-2], fields[len(fields)-1]
		num := func(i int) float64 {
			v, err := strconv.ParseFloat(args[i], 64)
			if err != nil {
				t.Fatalf("line %d arg %d: %v", line, i, err)
			}
			return v
		}
		n := func(i int) int { return int(num(i)) }
		flag := func(i int) bool { return args[i] == "true" }
		element := func(i int) (modelskill.Element, map[stat.Stat]float64) {
			el, err := modelskill.ParseElement(args[i])
			if err != nil {
				t.Fatalf("line %d arg %d: %v", line, i, err)
			}
			mul := map[stat.Stat]float64{}
			if s, ok := ElementResistanceStat(el); ok {
				mul[s] = num(i + 1)
			}
			return el, mul
		}

		var got float64
		switch name {
		case "skillStatModifier":
			got = SkillStatModifier(&oracleActor{con: n(2), men: n(3)}, args[0], flag(1))
		case "skillVulnerability":
			el, mul := element(1)
			for s, m := range oracleVulnMul {
				mul[s] = m
			}
			got = SkillVulnerability(&oracleActor{mul: mul}, args[0], modelskill.Definition{Element: el})
		case "elementalSkillModifier":
			el, mul := element(0)
			got = ElementalSkillModifier(&oracleActor{mul: mul}, modelskill.Definition{Element: el})
		case "levelModifier":
			got = SkillLevelModifier(n(3), n(2), modelskill.Definition{LevelDepend: n(0), MagicLevel: n(1)})
		case "matkModifier":
			got = SkillMAtkModifier(&oracleActor{mDef: num(3)}, &oracleActor{mAtk: num(2)}, modelskill.Definition{Magic: flag(0)}, flag(1))
		case "elementalPhysicalAttackModifier":
			attacker := &oracleActor{add: map[stat.Stat]float64{}}
			target := &oracleActor{mul: map[stat.Stat]float64{}}
			for i, elem := range physicalAttackElements {
				attacker.add[elem.atk] = num(i)
				target.mul[elem.res] = num(len(physicalAttackElements) + i)
			}
			got = ElementalPhysicalAttackModifier(attacker, target)
		default:
			continue
		}
		seen[name]++
		w, err := strconv.ParseFloat(want, 64)
		if err != nil {
			t.Fatalf("line %d result: %v", line, err)
		}
		if got != w {
			t.Errorf("line %d %s%v = %v, want %v", line, name, args, got, w)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 6 {
		t.Fatalf("golden covers %d modifiers, want 6", len(seen))
	}
}
