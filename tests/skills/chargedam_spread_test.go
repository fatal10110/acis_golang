package skills

import (
	"strings"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestShippedChargeDamSkillsLandAsChargeDam pins the shipped data the
// physical-skill spread gate reads: every shipped CHARGEDAM skill sets no
// effectType of its own, so its landing effect type, which the gate tests,
// falls back to CHARGEDAM and it rolls no weapon spread.
func TestShippedChargeDamSkillsLandAsChargeDam(t *testing.T) {
	for _, id := range []modelskill.ID{5, 6, 7, 9, 17, 35, 54, 261, 284} {
		def := shippedSkill(t, id, 1)
		if def.SkillType != "CHARGEDAM" || def.EffectType != "" {
			t.Fatalf("skill %d = type %q effectType %q, want CHARGEDAM with no effectType", id, def.SkillType, def.EffectType)
		}
		if got := strings.ToUpper(def.LandingEffectType()); got != "CHARGEDAM" {
			t.Fatalf("skill %d landing effect type = %q, want CHARGEDAM", id, got)
		}
	}
}
