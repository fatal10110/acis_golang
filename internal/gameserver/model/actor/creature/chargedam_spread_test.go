package creature

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestRandomDamageMultiplierChargeDamRollsNoSpread pins the spread gate to
// the skill's landing effect type: a CHARGEDAM skill, which sets no
// effectType of its own, rolls no weapon spread; an effect template typed
// otherwise, or another skill type, still rolls it.
func TestRandomDamageMultiplierChargeDamRollsNoSpread(t *testing.T) {
	tests := []struct {
		name string
		def  modelskill.Definition
		want float64
	}{
		{"chargedam", modelskill.Definition{ID: 5, SkillType: "CHARGEDAM"}, 1},
		{"chargedam stun template", modelskill.Definition{ID: 5, SkillType: "CHARGEDAM", Effects: []modelskill.EffectTemplate{{EffectType: "STUN"}}}, 0.8},
		{"pdam", modelskill.Definition{ID: 3, SkillType: "PDAM"}, 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker := &randomDamageTestActor{level: 50, spread: 20, roll: 0}
			if got := RandomDamageMultiplier(attacker, tt.def); got != tt.want {
				t.Fatalf("RandomDamageMultiplier(%s) = %v, want %v", tt.def.SkillType, got, tt.want)
			}
		})
	}
}
