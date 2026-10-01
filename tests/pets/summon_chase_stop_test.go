package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestPetAttackChaseStopsWithinAttackRange pins the summon chase pawn walk
// (SummonMove.offensiveFollowTask, SummonMove.java:41-66, sets _pawn/_offset;
// CreatureMove.updatePosition, CreatureMove.java:387, ends the walk on its
// last geo leg once within _offset of the pawn, 2D): a pet sent against a
// monster that stands still stops at the first step strictly within its
// attack range of the monster, not on the monster's cell.
func TestPetAttackChaseStopsWithinAttackRange(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	px, py, pz := h.srv.PlayerPosition(t, h.ownerID)
	placePet(t, pet, location.Location{X: px, Y: py, Z: pz})
	target := location.Location{X: px + 600, Y: py, Z: pz}
	hostile := h.srv.SpawnHostileNPCAt(t, target)
	drainUntilQuiet(t, h.client)

	h.client.Send(encodeAction(hostile.ObjectID(), int32(px), int32(py), int32(pz), false))
	drainUntilQuiet(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, false))
	drainUntilQuiet(t, h.client)
	if !pet.IsMoving() {
		t.Fatal("pet did not start chasing the out-of-range monster")
	}
	for i := 0; pet.IsMoving(); i++ {
		if i == 300 {
			t.Fatal("chase still under way after 30 s of position updates")
		}
		h.srv.TickPositions()
	}

	x, y, z := pet.Position()
	pos := location.Location{X: x, Y: y, Z: z}
	reach := pet.PhysicalAttackRange()
	step := int(pet.Move().Speed()/10) + 1
	if !pos.In2DRadius(target, reach) || pos.In2DRadius(target, reach-step) {
		t.Fatalf("chase ended at %+v, %.1f from the monster; want the first step within the attack range %d",
			pos, pos.Distance2D(target), reach)
	}
	drainUntilQuiet(t, h.client)
}
