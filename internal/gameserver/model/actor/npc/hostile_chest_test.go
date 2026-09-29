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

// TestCoreAIDisabledChestsCannotAttack pins the NPC-only attack gate term:
// box chests (Chest kind, ids 18265-18298) and every Halisha chest start with
// their core AI disabled, so they never open a physical attack; a Chest id
// outside the box range and an ordinary monster still can. A script can
// toggle the flag either way.
func TestCoreAIDisabledChestsCannotAttack(t *testing.T) {
	cases := []struct {
		name     string
		id       int
		kind     InstanceKind
		disabled bool
	}{
		{"first box", 18265, "Chest", true},
		{"last box", 18298, "Chest", true},
		{"mimic chest", 21671, "Chest", false},
		{"halisha chest", 18256, "HalishaChest", true},
		{"monster with a box id", 18265, "Monster", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			h, err := NewHostile(&Instance{ObjectID: 1, Template: &Template{ID: tt.id, Type: string(tt.kind)}, Kind: tt.kind}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
			if err != nil {
				t.Fatal(err)
			}
			if got := h.AttackDisabled(); got != tt.disabled {
				t.Fatalf("AttackDisabled() = %v, want %v", got, tt.disabled)
			}
			h.DisableCoreAI(!tt.disabled)
			if got := h.AttackDisabled(); got == tt.disabled {
				t.Fatalf("AttackDisabled() = %v after DisableCoreAI(%v), want %v", got, !tt.disabled, !tt.disabled)
			}
		})
	}
}
