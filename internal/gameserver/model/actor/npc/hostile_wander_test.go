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
		{29003, true, 40, 20},
		{29050, false, 0, 0},
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

func TestShouldIdleWanderUsesTemplateIDNotKind(t *testing.T) {
	guard, err := NewHostile(&Instance{
		ObjectID: 1,
		Template: &Template{ID: 31845, Type: "Guard"},
		Kind:     "Guard",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatalf("NewHostile(Guard 31845): %v", err)
	}
	if !guard.ShouldIdleWander() {
		t.Fatal("Guard 31845 ShouldIdleWander() = false, want true")
	}

	hold, err := NewHostile(&Instance{
		ObjectID: 2,
		Template: &Template{ID: 27102, Type: "Monster"},
		Kind:     "Monster",
	}, newHostileLive(t), &hostileMove{}, &hostileAttack{})
	if err != nil {
		t.Fatalf("NewHostile(Monster 27102): %v", err)
	}
	if hold.ShouldIdleWander() {
		t.Fatal("Monster 27102 ShouldIdleWander() = true, want false")
	}
}

func TestIdleWanderTableSize(t *testing.T) {
	got := len(idleWanderIDs) + len(idleWanderSpecial)
	const want = 2509
	if got != want {
		t.Fatalf("idle wander id count = %d, want %d", got, want)
	}
}
