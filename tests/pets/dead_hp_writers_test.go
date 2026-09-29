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

// TestDeadPetRejectsMPWriters pins that a dead pet's MP stays where the
// killing hit left it: a restore or cost that passed its liveness check
// before the death is dropped, while a living or revived pet still gains and
// spends MP.
func TestDeadPetRejectsMPWriters(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	owner, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}

	assertLivePetMPWriters(t, pet)

	pet.ReduceHP(pet.MaxHPValue()*2, owner.(attackable.Combatant), modelskill.Definition{})
	if !pet.Dead() {
		t.Fatal("lethal hit left the pet alive")
	}
	mp := pet.MPValue()
	if got := pet.AddMP(5); got != 0 {
		t.Fatalf("dead AddMP(5) = %v, want 0", got)
	}
	if got := pet.ReduceMP(5); got != 0 {
		t.Fatalf("dead ReduceMP(5) = %v, want 0", got)
	}
	if got := pet.MPValue(); got != mp {
		t.Fatalf("dead pet MP = %v after the writers, want unchanged %v", got, mp)
	}

	if !pet.Revive() {
		t.Fatal("Revive() = false for a dead pet")
	}
	assertLivePetMPWriters(t, pet)
}

// petMPWriter is the slice of a pet the MP writer checks drive.
type petMPWriter interface {
	MPValue() float64
	AddMP(float64) float64
	ReduceMP(float64) float64
}

func assertLivePetMPWriters(t *testing.T, pet petMPWriter) {
	t.Helper()
	mp := pet.MPValue()
	if mp < 10 {
		t.Fatalf("fixture pet MP %v leaves no room for the MP writers", mp)
	}
	if got := pet.ReduceMP(10); got != 10 || pet.MPValue() != mp-10 {
		t.Fatalf("live ReduceMP(10) = %v leaving MP %v, want 10 leaving %v", got, pet.MPValue(), mp-10)
	}
	if got := pet.AddMP(5); got != 5 || pet.MPValue() != mp-5 {
		t.Fatalf("live AddMP(5) = %v leaving MP %v, want 5 leaving %v", got, pet.MPValue(), mp-5)
	}
}

// TestDeadPetLevelUpKeepsCorpseVitals pins that a level gained by a pet that
// died after its kill-reward liveness check leaves its corpse HP and MP
// alone: PetStatus.addLevel refills through setHp/setMp, which return while
// the pet is dead. The level itself still rises.
func TestDeadPetLevelUpKeepsCorpseVitals(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)
	owner, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner not in world")
	}

	if got := pet.ReduceMP(10); got != 10 {
		t.Fatalf("live ReduceMP(10) = %v, want 10", got)
	}
	pet.ReduceHP(pet.MaxHPValue()*2, owner.(attackable.Combatant), modelskill.Definition{})
	if !pet.Dead() {
		t.Fatal("lethal hit left the pet alive")
	}
	level, hp, mp := pet.Level(), pet.HP(), pet.MPValue()
	if mp >= pet.MaxMPValue() {
		t.Fatalf("corpse MP %v is already at max %v; the fixture must leave room for a refill", mp, pet.MaxMPValue())
	}

	pet.AddExpAndSp(2*wolfNextLevelExp, 0)
	if pet.Level() <= level {
		t.Fatalf("dead pet level = %d after the exp grant, want above %d", pet.Level(), level)
	}
	if pet.HP() != hp || pet.MPValue() != mp {
		t.Fatalf("dead pet HP/MP = %v/%v after the level-up, want corpse values %v/%v", pet.HP(), pet.MPValue(), hp, mp)
	}
	if !pet.Dead() {
		t.Fatal("level-up revived the pet")
	}
}
