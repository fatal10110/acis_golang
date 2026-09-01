package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestShouldIdleWanderIgnoresMovingAttack(t *testing.T) {
	params := commons.NewStatSet()
	params.Set("MovingAttack", 0)
	h := newCombatHostile(t, 1, &Template{ID: 20001, Type: "Monster", AIParams: params})
	if !h.ShouldIdleWander() {
		t.Fatal("ShouldIdleWander() = false with MovingAttack=0, want true")
	}
}

func TestShouldIdleWanderExcludesHoldKinds(t *testing.T) {
	for _, kind := range []InstanceKind{"Guard", "SiegeGuard", "Chest", "HalishaChest"} {
		h, err := NewHostile(&Instance{
			ObjectID: 1,
			Template: &Template{ID: 1, Type: string(kind)},
			Kind:     kind,
		}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
		if err != nil {
			t.Fatalf("NewHostile(%s): %v", kind, err)
		}
		if h.ShouldIdleWander() {
			t.Fatalf("%s ShouldIdleWander() = true, want false", kind)
		}
	}
}
