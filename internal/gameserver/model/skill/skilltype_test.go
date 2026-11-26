package skill

import "testing"

// TestParseSkillType pins the loader's skill type match: every tag parses
// to itself, and the match is exact.
func TestParseSkillType(t *testing.T) {
	t.Parallel()
	if got := len(skillTypes); got != 95 {
		t.Errorf("skill type tags = %d, want 95", got)
	}
	for tag := range skillTypes {
		if got, ok := ParseSkillType(string(tag)); !ok || got != tag {
			t.Errorf("ParseSkillType(%q) = %q, %v; want the tag", tag, got, ok)
		}
	}
	for _, name := range []string{"", "summon", " SUMMON", "SUMON", "Buff"} {
		if _, ok := ParseSkillType(name); ok || KnownSkillType(name) {
			t.Errorf("ParseSkillType(%q) accepted, want rejected", name)
		}
	}
	if !KnownSkillType("SUMMON_FRIEND") {
		t.Error(`KnownSkillType("SUMMON_FRIEND") = false, want true`)
	}
}

func TestSkillTypeNormalized(t *testing.T) {
	for in, want := range map[SkillType]SkillType{
		"BUFF":          SkillTypeBuff,
		" heal_static ": SkillTypeHealStatic,
		"Pdam\t":        SkillTypePDam,
		"":              "",
	} {
		if got := in.Normalized(); got != want {
			t.Errorf("%q.Normalized() = %q, want %q", in, got, want)
		}
	}
}
