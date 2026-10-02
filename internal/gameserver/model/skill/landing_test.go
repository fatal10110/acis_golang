package skill

import "testing"

// TestLandingPowerAndEffectType pins L2Skill.getEffectPower and
// getEffectType (L2Skill.java:554-622): an effect template's positive
// effect power or effect type comes first, then the skill's own, then the
// PDAM/MDAM fallbacks, then the skill's power (kept only within (0, 100])
// or its skill type. The cubic skills are their datapack definitions.
func TestLandingPowerAndEffectType(t *testing.T) {
	tests := []struct {
		name      string
		def       Definition
		wantPower float64
		wantType  string
	}{
		{"4049 Cubic Drain (MDAM)", Definition{SkillType: "MDAM", Power: 2399}, 20, "PARALYZE"},
		{"4050 Cubic DD (DRAIN)", Definition{SkillType: "DRAIN", Power: 543}, 20, "DRAIN"},
		{"4052 Poison", Definition{SkillType: "POISON", Power: 70, Effects: []EffectTemplate{{Name: "DamOverTime", EffectPower: -1}}}, 70, "POISON"},
		{"4164 Paralysis", Definition{SkillType: "PARALYZE", Power: 15}, 15, "PARALYZE"},
		{"5115 Cubic Hate", Definition{SkillType: "AGGDAMAGE", Power: 80}, 80, "AGGDAMAGE"},
		{"PDAM fallback", Definition{SkillType: "PDAM", Power: 50}, 20, "STUN"},
		{"power over 100", Definition{SkillType: "DEBUFF", Power: 101}, 20, "DEBUFF"},
		{"power exactly 100", Definition{SkillType: "DEBUFF", Power: 100}, 100, "DEBUFF"},
		{"no power", Definition{SkillType: "ROOT"}, 20, "ROOT"},
		{"skill effectPower and effectType", Definition{SkillType: "MDAM", Power: 80, EffectPower: 35, EffectType: "STUN"}, 35, "STUN"},
		{
			"template power and type first",
			Definition{SkillType: "MDAM", EffectPower: 35, EffectType: "STUN", Effects: []EffectTemplate{
				{EffectPower: 0},
				{EffectPower: 60, EffectType: "SLEEP"},
				{EffectPower: 70, EffectType: "ROOT"},
			}},
			60, "SLEEP",
		},
	}
	for _, tt := range tests {
		if got := tt.def.LandingPower(); got != tt.wantPower {
			t.Errorf("%s: LandingPower() = %v, want %v", tt.name, got, tt.wantPower)
		}
		if got := tt.def.LandingEffectType(); got != tt.wantType {
			t.Errorf("%s: LandingEffectType() = %q, want %q", tt.name, got, tt.wantType)
		}
	}
}
