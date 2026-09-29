package pets

import (
	"testing"

	handlerskill "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestGiveSPGrantsPetSP pins the pet half of GIVE_SP: a live pet target
// takes the skill's power as SP. No shipped GIVE_SP skill targets anything
// but its caster, so this drives the production handler registry directly
// with a real summoned pet.
func TestGiveSPGrantsPetSP(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	before := pet.SP()

	handlerskill.NewDefaultRegistry().Use(handlerskill.Cast{
		Caster:  pet,
		Skill:   modelskill.Definition{ID: 2167, Level: 1, SkillType: "GIVE_SP", Power: 500},
		Targets: []handlerskill.Actor{pet},
	})

	if got := pet.SP() - before; got != 500 {
		t.Fatalf("pet SP gain = %d, want 500", got)
	}
}
