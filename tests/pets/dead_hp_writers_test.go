package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestDeadPetRejectsHPWriters pins that a dead pet's HP stays at zero: a heal
// or set that passed its liveness check before the killing hit is dropped,
// while a living pet still heals.
func TestDeadPetRejectsHPWriters(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	owner, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}

	pet.SetHP(10)
	if got := pet.AddHP(5); got != 5 || pet.HP() != 15 {
		t.Fatalf("live AddHP(5) = %v leaving HP %v, want 5 leaving 15", got, pet.HP())
	}

	if pet.Dead() {
		t.Fatal("fixture pet starts dead")
	}
	// The liveness check above is now stale: the pet dies before the writes.
	pet.ReduceHP(pet.MaxHPValue()*2, owner.(attackable.Combatant), modelskill.Definition{})
	if !pet.Dead() {
		t.Fatal("lethal hit left the pet alive")
	}
	if got := pet.AddHP(100); got != 0 {
		t.Fatalf("dead AddHP(100) = %v, want 0", got)
	}
	pet.SetHP(50)
	if got := pet.HP(); got != 0 {
		t.Fatalf("dead pet HP = %v after AddHP/SetHP, want 0", got)
	}
}
