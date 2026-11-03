package stat

import "testing"

// TestByConstantName: every Stat has its own upper-case constant spelling
// and resolves back from it. The spot checks are the reference enum's
// constants, including its HEAL_EFFECTIVNESS spelling; a data-file spelling
// is not a constant name.
func TestByConstantName(t *testing.T) {
	for s := Stat(0); s < numStats; s++ {
		got, err := ByConstantName(constantNames[s])
		if err != nil || got != s {
			t.Errorf("ByConstantName(%q) = (%v, %v), want %v", constantNames[s], got, err, s)
		}
	}
	for name, want := range map[string]Stat{
		"MAX_HP": MaxHP, "HEAL_EFFECTIVNESS": HealEffectiveness, "P_REUSE": PReuse,
		"PATK_MCREATURES": PAtkMCreatures, "INV_LIM": InvLim, "SKILL_MASTERY": SkillMastery,
	} {
		if got, err := ByConstantName(name); err != nil || got != want {
			t.Errorf("ByConstantName(%q) = (%v, %v), want %v", name, got, err, want)
		}
	}
	for _, name := range []string{"", "maxHp", "max_hp", "HEAL_EFFECTIVENESS"} {
		if _, err := ByConstantName(name); err == nil {
			t.Errorf("ByConstantName(%q) resolved, want an error", name)
		}
	}
}
