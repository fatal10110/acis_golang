package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestNegateTypeMatchesResolvedEffectType checks a negate effect matches a
// candidate by its owning skill's resolved effect type: the first typed
// effect template, else the skill's own effectType, else STUN for PDAM and
// PARALYZE for MDAM, else the skill type.
func TestNegateTypeMatchesResolvedEffectType(t *testing.T) {
	for _, tt := range []struct {
		name    string
		def     modelskill.Definition
		negType string
		want    bool
	}{
		{"mdam resolves to paralyze", modelskill.Definition{SkillType: "MDAM", Effects: []modelskill.EffectTemplate{{Name: "Debuff"}}}, "PARALYZE", true},
		{"pdam resolves to stun", modelskill.Definition{SkillType: "PDAM", Effects: []modelskill.EffectTemplate{{Name: "Debuff"}}}, "STUN", true},
		{"pdam is not paralyze", modelskill.Definition{SkillType: "PDAM"}, "PARALYZE", false},
		{"skill-level effect type", modelskill.Definition{SkillType: "DEBUFF", EffectType: "PARALYZE", Effects: []modelskill.EffectTemplate{{Name: "Debuff"}}}, "PARALYZE", true},
		{"first typed template wins", modelskill.Definition{SkillType: "MDAM", EffectType: "ROOT", Effects: []modelskill.EffectTemplate{{Name: "Debuff"}, {Name: "Stun", EffectType: "STUN"}}}, "STUN", true},
		{"own skill type", modelskill.Definition{SkillType: "POISON", EffectType: "BLEED"}, "POISON", true},
	} {
		if got := negateTypeMatches(SkillFromDefinition(tt.def), tt.negType); got != tt.want {
			t.Errorf("%s: negateTypeMatches(%q) = %v, want %v", tt.name, tt.negType, got, tt.want)
		}
	}
}
