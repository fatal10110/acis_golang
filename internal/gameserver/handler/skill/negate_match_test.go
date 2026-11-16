package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestNegateMatchesByLevelAndResolvedType checks the NEGATE handler's
// per-type match: at level -1 by skill type or resolved effect type; at a
// set level by resolved effect type and effect abnormal level when the
// candidate has one, else by skill type and abnormal level.
func TestNegateMatchesByLevelAndResolvedType(t *testing.T) {
	mdam := modelskill.Definition{SkillType: "MDAM", AbnormalLevel: -1, EffectAbnormalLevel: -1, Effects: []modelskill.EffectTemplate{{Name: "Debuff"}}}
	pdam := modelskill.Definition{SkillType: "PDAM", AbnormalLevel: -1, EffectAbnormalLevel: -1, Effects: []modelskill.EffectTemplate{{Name: "Debuff"}}}
	poison := func(abnormal int) modelskill.Definition {
		return modelskill.Definition{SkillType: "POISON", AbnormalLevel: abnormal, EffectAbnormalLevel: -1}
	}
	bleedInPoison := modelskill.Definition{SkillType: "POISON", EffectType: "BLEED", AbnormalLevel: 1, EffectAbnormalLevel: 1}
	poisonInDebuff := modelskill.Definition{SkillType: "DEBUFF", EffectType: "POISON", AbnormalLevel: 9, EffectAbnormalLevel: 2}
	for _, tt := range []struct {
		name    string
		def     modelskill.Definition
		negType string
		level   int
		want    bool
	}{
		{"mdam effect under a paralyze negate", mdam, "PARALYZE", -1, true},
		{"pdam effect under a stun negate", pdam, "STUN", -1, true},
		{"pdam effect under a paralyze negate", pdam, "PARALYZE", -1, false},
		{"poison at the level", poison(3), "POISON", 3, true},
		{"poison above the level", poison(4), "POISON", 3, false},
		{"poison without a level", poison(-1), "POISON", 3, true},
		{"effect level picks the effect type", bleedInPoison, "POISON", 3, false},
		{"effect level picks the effect type, any level", bleedInPoison, "POISON", -1, true},
		{"effect type within its effect level", poisonInDebuff, "POISON", 3, true},
		{"effect type above its effect level", poisonInDebuff, "POISON", 1, false},
	} {
		if got := negateMatches(effect.SkillFromDefinition(tt.def), tt.negType, tt.level); got != tt.want {
			t.Errorf("%s: negateMatches(%q, %d) = %v, want %v", tt.name, tt.negType, tt.level, got, tt.want)
		}
	}
}
