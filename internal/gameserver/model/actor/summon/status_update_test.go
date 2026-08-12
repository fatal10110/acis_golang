package summon

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestReduceHPUpdatesStatusAfterDirectAndDOTDamage(t *testing.T) {
	for _, damage := range []struct {
		name  string
		apply func(*Actor)
	}{
		{"direct", func(a *Actor) { a.ReduceHP(10, nil, modelskill.Definition{}) }},
		{"dot", func(a *Actor) { a.ReduceHPByDOT(10, nil, true) }},
	} {
		t.Run(damage.name, func(t *testing.T) {
			a := NewPet(PetConfig{Stats: CombatStats{MaxHP: 100}})
			updates := 0
			a.SetStatusUpdater(func() { updates++ })

			damage.apply(a)

			if updates != 1 {
				t.Fatalf("status updates = %d, want 1", updates)
			}
		})
	}
}
