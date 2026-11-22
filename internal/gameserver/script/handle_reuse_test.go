package script

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	creaturelive "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// reuseGeo is geodata the test never asks: the hostile never moves.
type reuseGeo struct{ move.Geo }

type reuseMove struct{}

func (reuseMove) MaybeStartOffensiveFollow(attackable.Combatant, int) (bool, error) {
	return false, nil
}
func (reuseMove) MoveHome(location.Location) error                       { return nil }
func (reuseMove) Stop()                                                  {}
func (reuseMove) CancelFollow()                                          {}
func (reuseMove) CanMoveTo(location.Location) bool                       { return true }
func (reuseMove) MoveToLocation(location.Location) (bool, error)         { return false, nil }
func (reuseMove) MoveStraightToLocation(location.Location) (bool, error) { return false, nil }

type reuseAttack struct{}

func (reuseAttack) BowCoolingDown() bool                { return false }
func (reuseAttack) AttackingNow() bool                  { return false }
func (reuseAttack) CanAttack(attackable.Combatant) bool { return false }
func (reuseAttack) DoAttack(attackable.Combatant)       {}
func (reuseAttack) Stop()                               {}

// Every hook on a hostile NPC shares one handle, kept on the NPC: a hook
// dispatch makes no handle of its own.
func TestHostileHandleIsKeptAcrossHooks(t *testing.T) {
	live, err := creaturelive.NewLive(location.Location{}, 100, reuseGeo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h, err := npc.NewHostile(&npc.Instance{ObjectID: 7, Kind: "Monster", Template: &npc.Template{ID: 20001, Type: "Monster", HPMax: 100}}, live, reuseMove{}, reuseAttack{})
	if err != nil {
		t.Fatal(err)
	}
	first := NewNPC(h)
	if first == nil || first.self != attackable.Combatant(h) {
		t.Fatalf("NewNPC(h) = %+v, want a handle on h", first)
	}
	if again, of := NewNPC(h), NPCOf(h); again != first || of != first {
		t.Fatal("a second hook made another handle for the same hostile")
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = NewNPC(h) }); allocs != 0 {
		t.Fatalf("a hook's hostile handle allocates %v times, want 0", allocs)
	}
}
