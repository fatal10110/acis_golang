package npc

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestWarriorIdleWanderIgnoresMovingAttack(t *testing.T) {
	// Warrior.onNoDesire (id 20001) does not read MovingAttack. MonsterBehavior
	// descendants that ship MovingAttack=0 (21394/21395) are omitted from the
	// table instead — see TestIdleWanderParamsIssueCases.
	params := commons.NewStatSet()
	params.Set("MovingAttack", 0)
	h := newCombatHostile(t, 1, &Template{ID: 20001, Type: "Monster", AIParams: params})
	timer, weight, ok := h.IdleWander()
	if !ok || timer != 5 || weight != 5 {
		t.Fatalf("IdleWander() = (%d, %v, %v), want (5, 5, true)", timer, weight, ok)
	}
}

func TestIdleWanderParamsIssueCases(t *testing.T) {
	cases := []struct {
		id         int
		wantOK     bool
		wantTimer  int
		wantWeight float64
	}{
		{20001, true, 5, 5},
		{20002, true, 5, 20},
		{27102, false, 0, 0},
		{12774, false, 0, 0},
		{31845, true, 5, 5},
		{31844, true, 5, 5},
		{31853, true, 5, 5},
		{31032, true, 5, 5},
		{22199, true, 5, 5},
		{22200, true, 5, 5},
		{22215, true, 5, 5},
		{22214, false, 0, 0},
		{29003, true, 40, 20},
		// PortraitSpirit has no NO_DESIRE handler; spawn-time wander is onCreated.
		{29050, false, 0, 0},
		{29051, false, 0, 0},
		// MonsterBehavior.onNoDesire skips wander when MovingAttack is 0.
		{21394, false, 0, 0},
		{21395, false, 0, 0},
		// Awake-state wander is not table-shaped.
		{29020, false, 0, 0},
		{29028, false, 0, 0},
		{29066, false, 0, 0},
		{100, false, 0, 0},
		{1, false, 0, 0},
	}
	for _, tc := range cases {
		timer, weight, ok := IdleWanderParams(tc.id)
		if ok != tc.wantOK || timer != tc.wantTimer || weight != tc.wantWeight {
			t.Errorf("IdleWanderParams(%d) = (%d, %v, %v), want (%d, %v, %v)",
				tc.id, timer, weight, ok, tc.wantTimer, tc.wantWeight, tc.wantOK)
		}
	}
}

func TestIdleWanderUsesTemplateIDNotKind(t *testing.T) {
	guard, err := NewHostile(&Instance{
		ObjectID: 1,
		Template: &Template{ID: 31845, Type: "Guard"},
		Kind:     "Guard",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatalf("NewHostile(Guard 31845): %v", err)
	}
	if _, _, ok := guard.IdleWander(); !ok {
		t.Fatal("Guard 31845 IdleWander ok=false, want true")
	}

	hold, err := NewHostile(&Instance{
		ObjectID: 2,
		Template: &Template{ID: 27102, Type: "Monster"},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatalf("NewHostile(Monster 27102): %v", err)
	}
	if _, _, ok := hold.IdleWander(); ok {
		t.Fatal("Monster 27102 IdleWander ok=true, want false")
	}
}

func TestIdleWanderTableSize(t *testing.T) {
	got := len(idleWanderIDs) + len(idleWanderSpecial)
	const want = 2526
	if got != want {
		t.Fatalf("idle wander id count = %d, want %d", got, want)
	}
	if !slices.IsSorted(idleWanderIDs) {
		t.Error("idleWanderIDs must stay sorted for BinarySearch")
	}
	for id := range idleWanderSpecial {
		if _, found := slices.BinarySearch(idleWanderIDs, id); found {
			t.Errorf("id %d in both idleWanderIDs and idleWanderSpecial", id)
		}
	}
}
