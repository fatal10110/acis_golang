package npc

import "testing"

func TestHostileCollisionRadiusOverride(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CollisionRadius: 9})

	if got := h.CollisionRadius(); got != 9 {
		t.Fatalf("CollisionRadius() before override = %v, want template value 9", got)
	}

	h.SetCollisionRadius(9 * 1.19)
	if got := h.CollisionRadius(); got != 9*1.19 {
		t.Fatalf("CollisionRadius() after SetCollisionRadius = %v, want %v", got, 9*1.19)
	}

	h.ResetCollisionRadius()
	if got := h.CollisionRadius(); got != 9 {
		t.Fatalf("CollisionRadius() after ResetCollisionRadius = %v, want template value 9", got)
	}
}
