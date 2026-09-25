package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestPetDamageRemainderIsHalfPointThreshold pins that a summon dies on a
// remainder under half a point and survives at or above it, on both the
// direct-hit and the damage-over-time route. Fractional HP is ordinary; only
// the packet surface truncates it.
func TestPetDamageRemainderIsHalfPointThreshold(t *testing.T) {
	t.Parallel()
	const damage = 10
	routes := map[string]func(pet *summon.Actor, owner any){
		"hit": func(pet *summon.Actor, owner any) {
			pet.ReduceHP(damage, owner.(attackable.Combatant), modelskill.Definition{})
		},
		"damage over time": func(pet *summon.Actor, owner any) {
			pet.ReduceHPByDOT(damage, owner.(effect.Actor), true)
		},
	}
	for route, apply := range routes {
		for _, test := range []struct {
			name      string
			remainder float64
			wantDead  bool
		}{
			{"below half point", 0.4, true},
			{"at half point", 0.5, false},
		} {
			t.Run(route+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				h := bootOwnerWithCollar(t)
				pet, _ := h.spawnWolf(t)
				owner, ok := h.srv.State.Player(h.ownerID)
				if !ok {
					t.Fatal("owner not in world")
				}
				pet.SetHP(damage + test.remainder)
				apply(pet, owner)
				if dead := pet.Dead(); dead != test.wantDead {
					t.Fatalf("pet dead after a hit leaving %v HP = %v, want %v", test.remainder, dead, test.wantDead)
				}
			})
		}
	}
}
