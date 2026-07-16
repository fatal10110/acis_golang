package staticobject

import "testing"

type testChairUser struct {
	x, y, z  int
	dead     bool
	standing bool
}

func (u testChairUser) Position() (int, int, int) { return u.x, u.y, u.z }
func (u testChairUser) AlikeDead() bool           { return u.dead }
func (u testChairUser) Standing() bool            { return u.standing }

type testChairObject struct {
	x, y, z int
	typ     int
	busy    bool
}

func (o *testChairObject) Position() (int, int, int) { return o.x, o.y, o.z }
func (o *testChairObject) Type() int                 { return o.typ }
func (o *testChairObject) SetBusy(busy bool) bool {
	if o.busy == busy {
		return false
	}
	o.busy = busy
	return true
}

func TestClaimChairRequiresEligibleUserAndChair(t *testing.T) {
	user := testChairUser{standing: true}
	chair := &testChairObject{x: 100, typ: ChairType}

	if !ClaimChair(user, chair, ChairInteractionDistance) {
		t.Fatal("ClaimChair returned false for standing user in range of free chair")
	}
	if !chair.busy {
		t.Fatal("ClaimChair did not mark the chair busy")
	}
	if ClaimChair(user, chair, ChairInteractionDistance) {
		t.Fatal("ClaimChair returned true for an already busy chair")
	}

	tests := []struct {
		name  string
		user  testChairUser
		chair *testChairObject
	}{
		{"dead user", testChairUser{standing: true, dead: true}, &testChairObject{typ: ChairType}},
		{"sitting user", testChairUser{}, &testChairObject{typ: ChairType}},
		{"wrong type", testChairUser{standing: true}, &testChairObject{typ: 2}},
		{"too far", testChairUser{standing: true}, &testChairObject{x: ChairInteractionDistance + 1, typ: ChairType}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ClaimChair(tt.user, tt.chair, ChairInteractionDistance) {
				t.Fatal("ClaimChair returned true, want false")
			}
			if tt.chair.busy {
				t.Fatal("ClaimChair marked an invalid chair busy")
			}
		})
	}
}
