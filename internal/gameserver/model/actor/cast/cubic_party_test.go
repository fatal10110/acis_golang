package cast

import "testing"

type cubicMember struct {
	id      int32
	x, y, z int
}

func (m cubicMember) ObjectID() int32           { return m.id }
func (m cubicMember) Position() (int, int, int) { return m.x, m.y, m.z }

// TestDecideLifeCubicTarget_PartyScan pins Cubic.pickFriendlyTarget's party
// branch: the living member under full HP with the lowest HP ratio
// strictly within 900 of the owner (no collision radii), the first of
// equals, the owner included as a member; the ratio band of the chosen
// member then gates the heal roll.
func TestDecideLifeCubicTarget_PartyScan(t *testing.T) {
	owner := &fakeCubicFireOwner{objectID: 1, hp: 10, maxHP: 1000}
	self := cubicMember{id: 1}
	tests := []struct {
		name    string
		members []LifeCubicMember
		roll    int
		want    int32 // 0: no target
	}{
		{"lowest ratio wins", []LifeCubicMember{
			{Target: self, HPRatio: 0.9},
			{Target: cubicMember{id: 2, x: 100}, HPRatio: 0.5},
			{Target: cubicMember{id: 3, x: 100}, HPRatio: 0.7},
		}, 0, 2},
		{"owner is a member like any other", []LifeCubicMember{
			{Target: self, HPRatio: 0.2},
			{Target: cubicMember{id: 2, x: 100}, HPRatio: 0.5},
		}, 0, 1},
		{"dead member skipped", []LifeCubicMember{
			{Target: self, HPRatio: 0.9},
			{Target: cubicMember{id: 2}, Dead: true, HPRatio: 0},
		}, 0, 1},
		{"first of equal ratios", []LifeCubicMember{
			{Target: cubicMember{id: 2}, HPRatio: 0.5},
			{Target: cubicMember{id: 3}, HPRatio: 0.5},
		}, 0, 2},
		{"member at 900 is out of range", []LifeCubicMember{
			{Target: self, HPRatio: 0.9},
			{Target: cubicMember{id: 2, x: 900}, HPRatio: 0.1},
		}, 0, 1},
		{"member just inside 900 is in range", []LifeCubicMember{
			{Target: self, HPRatio: 0.9},
			{Target: cubicMember{id: 2, x: 899}, HPRatio: 0.1},
		}, 0, 2},
		// The owner's own low HP outside the member list does not matter:
		// in a party only the members' ratios count.
		{"whole party at full HP", []LifeCubicMember{
			{Target: self, HPRatio: 1},
			{Target: cubicMember{id: 2}, HPRatio: 1},
		}, 0, 0},
		{"over 60% needs a roll of at most 13", []LifeCubicMember{{Target: cubicMember{id: 2}, HPRatio: 0.61}}, 14, 0},
		{"60% needs a roll of at most 33", []LifeCubicMember{{Target: cubicMember{id: 2}, HPRatio: 0.6}}, 33, 2},
		{"under 30% needs a roll of at most 53", []LifeCubicMember{{Target: cubicMember{id: 2}, HPRatio: 0.29}}, 53, 2},
		{"under 30% fails a roll of 54", []LifeCubicMember{{Target: cubicMember{id: 2}, HPRatio: 0.29}}, 54, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := *owner
			o.rolls = []int{tt.roll}
			got, ok := DecideLifeCubicTarget(&o, tt.members)
			switch {
			case tt.want == 0 && ok:
				t.Fatalf("target = %d, want none", got.ObjectID())
			case tt.want != 0 && (!ok || got.ObjectID() != tt.want):
				t.Fatalf("target = %v (%v), want %d", got, ok, tt.want)
			}
		})
	}
}

// TestDecideCubicFire_RangeLeavesCollisionOut pins Cubic.pickEnemyTarget's
// range: strictly under 900 from the owner, centre to centre.
func TestDecideCubicFire_RangeLeavesCollisionOut(t *testing.T) {
	for _, tt := range []struct {
		x    int
		want bool
	}{{899, true}, {900, false}} {
		target := &fakeCubicTarget{objectID: 2, x: tt.x}
		owner := &fakeCubicFireOwner{objectID: 1, rolls: []int{0, 0}, target: target}
		if _, _, ok := DecideCubicFire(owner, []int{4049}, 100); ok != tt.want {
			t.Fatalf("target at %d: fired = %v, want %v", tt.x, ok, tt.want)
		}
	}
}
