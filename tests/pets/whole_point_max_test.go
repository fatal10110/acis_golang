package pets

import (
	"math"
	"testing"
)

// TestPetRestoresClampToWholePointMaxima pins that a summon's HP/MP maxima
// are whole points, as the reference's int getMaxHp/getMaxMp are: a set above
// the maximum lands on the truncated value and a restore at full applies
// nothing.
func TestPetRestoresClampToWholePointMaxima(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	pet, _ := h.spawnWolf(t)

	maxHP, maxMP := pet.MaxHPValue(), pet.MaxMPValue()
	if maxHP != math.Trunc(maxHP) || maxMP != math.Trunc(maxMP) {
		t.Fatalf("pet max HP/MP = %v/%v, want whole-point values", maxHP, maxMP)
	}
	pet.SetHP(maxHP + 50)
	if got := pet.HP(); got != maxHP {
		t.Fatalf("SetHP(max+50) left HP %v, want %v", got, maxHP)
	}
	if got := pet.AddHP(1); got != 0 {
		t.Fatalf("AddHP(1) at full HP = %v, want 0", got)
	}
	if got := pet.ReduceMP(pet.MPValue()); got <= 0 {
		t.Fatalf("draining the pet's MP applied %v", got)
	}
	if got := pet.AddMP(maxMP + 50); got != maxMP {
		t.Fatalf("AddMP(max+50) on an empty pool = %v, want %v", got, maxMP)
	}
	if got := pet.AddMP(1); got != 0 {
		t.Fatalf("AddMP(1) at full MP = %v, want 0", got)
	}
}
