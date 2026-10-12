package conditions

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// TestCompilePlayerRace: <player race> names a race exactly, and holds only
// for a player effector of that race (the test player is a human).
func TestCompilePlayerRace(t *testing.T) {
	for name, want := range map[string]int{"HUMAN": 0, "ELF": 1, "DARK_ELF": 2, "ORC": 3, "DWARF": 4} {
		if c := mustCompile(t, leaf("player", map[string]string{"race": name})); c != (Race{Race: want}) {
			t.Errorf("race=%q compiled to %#v, want Race{%d}", name, c, want)
		}
	}
	human := &player{}
	runProbes(t, leaf("player", map[string]string{"race": "HUMAN"}), []probe{
		{"human player", human, nil, true},
		{"npc", &npc{}, nil, false},
	})
	runProbes(t, leaf("player", map[string]string{"race": "ELF"}), []probe{
		{"human player", human, nil, false},
	})
}

type statSkill struct {
	stat stat.Stat
	ok   bool
}

func (s statSkill) Stat() (stat.Stat, bool) { return s.stat, s.ok }

// TestCompileSkillStat: <skill stat> reads the data-file stat spelling and
// holds only when the skill under test names that stat; no skill, or a skill
// naming none, fails.
func TestCompileSkillStat(t *testing.T) {
	c := mustCompile(t, leaf("skill", map[string]string{"stat": "maxHp", "bogus": "x"}))
	if c != (SkillStats{Stat: stat.MaxHP}) {
		t.Fatalf("compiled to %#v, want SkillStats{MaxHP}", c)
	}
	p := &player{}
	for _, tt := range []struct {
		name  string
		skill Skill
		want  bool
	}{
		{"same stat", statSkill{stat.MaxHP, true}, true},
		{"other stat", statSkill{stat.PowerAttack, true}, false},
		{"no stat", statSkill{}, false},
		{"no skill", nil, false},
	} {
		if got := c.Test(p, p, tt.skill); got != tt.want {
			t.Errorf("%s: Test = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestEvaluateSkillTestsTheDefinitionStat: the skill under test of a cast
// condition is the definition itself, its stat named by the upper-case
// constant spelling of its "stat" set value.
func TestEvaluateSkillTestsTheDefinitionStat(t *testing.T) {
	clause := modelskill.ConditionClause{Root: leaf("skill", map[string]string{"stat": "maxHp"}), MessageID: 113}
	caster := source{&player{}}
	for _, tt := range []struct {
		stat string
		want bool
	}{
		{"MAX_HP", true},
		{"POWER_ATTACK", false},
		{"maxHp", false},
		{"", false},
	} {
		def := modelskill.Definition{Stat: tt.stat, Conditions: []modelskill.ConditionClause{clause}}
		if _, ok := EvaluateSkill(def, caster, nil); ok != tt.want {
			t.Errorf("stat %q: EvaluateSkill ok = %v, want %v", tt.stat, ok, tt.want)
		}
	}
}
