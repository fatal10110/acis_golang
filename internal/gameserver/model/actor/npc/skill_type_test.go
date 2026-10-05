package npc

import "testing"

func TestSkillTypeTokens(t *testing.T) {
	t.Parallel()

	// The value order is load-bearing (by-id lookups take the lowest type),
	// so pin its ends and size.
	if skillTypeCount != 141 {
		t.Fatalf("skillTypeCount = %d, want 141", skillTypeCount)
	}
	pins := map[SkillType]string{
		SkillTypePassive:            "PASSIVE",
		SkillTypeAfflictSkill1:      "AFFLICT_SKILL1",
		SkillTypeDDMagic:            "DD_MAGIC",
		SkillTypeSkill01ID:          "SKILL01_ID",
		SkillTypeWShortRangeDDMagic: "W_SHORT_RANGE_DD_MAGIC",
	}
	for typ, name := range pins {
		if typ.String() != name {
			t.Errorf("%d.String() = %q, want %q", typ, typ.String(), name)
		}
	}
	if SkillTypeWShortRangeDDMagic != skillTypeCount-1 {
		t.Errorf("last type = %d, want %d", SkillTypeWShortRangeDDMagic, skillTypeCount-1)
	}

	for i := range skillTypeCount {
		name := i.String()
		if name == "" {
			t.Fatalf("type %d has no token", i)
		}
		got, ok := ParseSkillType(name)
		if !ok || got != i {
			t.Fatalf("ParseSkillType(%q) = %d, %v, want %d, true", name, got, ok, i)
		}
	}

	for _, bad := range []string{"", "passive", "Buff", " BUFF", "NOT_A_TYPE"} {
		if _, ok := ParseSkillType(bad); ok {
			t.Errorf("ParseSkillType(%q) ok, want false", bad)
		}
	}
	if got := skillTypeCount.String(); got != "SkillType(141)" {
		t.Errorf("out-of-range String() = %q", got)
	}
}
