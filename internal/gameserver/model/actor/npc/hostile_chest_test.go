package npc

import "testing"

func TestChestClaimInteractionOnlyOnce(t *testing.T) {
	chest := newCombatHostile(t, 1, &Template{ID: 18265, Type: "Chest"})
	chest.Instance.Kind = "Chest"
	if !chest.Box() {
		t.Fatal("chest 18265 is not a box")
	}
	if chest.Interacted() {
		t.Fatal("Interacted() = true before any claim")
	}
	if !chest.ClaimInteraction() {
		t.Fatal("first ClaimInteraction() = false, want true")
	}
	if chest.ClaimInteraction() {
		t.Fatal("second ClaimInteraction() = true, want false")
	}
	if !chest.Interacted() {
		t.Fatal("Interacted() = false after a claim")
	}
}
