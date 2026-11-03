package skill

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestRegistrySetMagicFailuresReachesNextCast pins that SetMagicFailures
// puts the switch in force for the MDAM and DRAIN casts after it, on the
// registry already built.
func TestRegistrySetMagicFailuresReachesNextCast(t *testing.T) {
	registry := NewDefaultRegistryWithSignet(nil, true, nil, SignetDeps{})
	for _, on := range []bool{false, true} {
		registry.SetMagicFailures(on)
		for _, skillType := range []string{"MDAM", "DRAIN"} {
			target := &skillTarget{hp: 2000}
			registry.Use(Cast{Caster: &skillTarget{hp: 2000}, Skill: modelskill.Definition{SkillType: skillType}, Targets: []Actor{target}})
			if target.magicFailures == nil || *target.magicFailures != on {
				t.Fatalf("%s after SetMagicFailures(%v): MagicDamageInput magicFailures = %v", skillType, on, target.magicFailures)
			}
		}
	}
}
